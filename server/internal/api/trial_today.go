package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/Teamthy/i-confess/internal/billing"
	"github.com/Teamthy/i-confess/internal/engine"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
	trialdomain "github.com/Teamthy/i-confess/internal/trial"
)

// startTrialToday builds today's trial-journey session(s) — G-48.
//
// The journey is guided, not automatic: nothing is built or played until the
// listener taps the day's card, and then the server does the work the day
// declares, so the client never has to reproduce it:
//
//   - Day 2 (SessionCount 2) builds two sessions, morning and evening, one
//     category each;
//   - Day 3 (Personalized) builds from the listener's own interests;
//   - Day 5 (PremiumVoice) selects an available Premium voice, and says so
//     plainly when none is ready rather than pretending;
//   - Day 6 (Custom) builds nothing: it returns the builder, prefilled;
//   - every other day builds one session from the day's categories.
//
// Idempotent per (user, day, slot): a retry returns the sessions already built
// for today; a concurrent duplicate gets 409 TRIAL_DAY_PREPARING. Plan limits,
// voice entitlement and content availability go through the same engine and
// error mapping as POST /sessions.
func (h *Handler) startTrialToday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := h.userID(r)
	now := time.Now().UTC()

	current, err := h.trials.Refresh(ctx, userID, now)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load trial")
		return
	}
	dayNum := trialDay(current, now)
	if current == nil || (current.State != trialdomain.Active && current.State != trialdomain.Expiring) || dayNum < 1 {
		writeCode(w, http.StatusConflict, "TRIAL_NOT_ACTIVE", "there is no trial day to start")
		return
	}
	day := h.personalizedJourney(r, userID)[dayNum-1]

	cats, err := h.cont.ListCategories(ctx, false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load categories")
		return
	}
	idBySlug := make(map[string]string, len(cats))
	for _, c := range cats {
		idBySlug[c.Slug] = c.ID
	}
	var catIDs, catSlugs []string
	for _, slug := range day.Categories {
		if id, ok := idBySlug[slug]; ok {
			catIDs = append(catIDs, id)
			catSlugs = append(catSlugs, slug)
		}
	}
	if len(catIDs) == 0 {
		writeCode(w, http.StatusUnprocessableEntity, "CONTENT_UNAVAILABLE", "today's categories are not published")
		return
	}

	ent := h.entitlementsFor(ctx, userID)
	duration := day.Duration
	if max := ent.MaxSessionSeconds(); duration > max {
		duration = max
	}

	resp := map[string]any{"day": day.Day, "intent": day.Intent, "title": day.Title}

	// Day 6 hands the listener the builder. Building for them would defeat
	// the day, so the response is the prefill only.
	if day.Custom {
		resp["action"] = "open_builder"
		// Both forms: ids for API clients, slugs for routes that are keyed by
		// slug (the web builder's ?cat=).
		resp["builder"] = map[string]any{"category_ids": catIDs, "category_slugs": catSlugs, "duration_seconds": duration}
		resp["sessions"] = []any{}
		httpx.WriteJSON(w, http.StatusOK, resp)
		return
	}

	voiceID := ""
	if day.PremiumVoice {
		voiceID = h.premiumVoiceID(ctx)
		resp["premium_voice_available"] = voiceID != ""
	}

	slots := day.SessionCount
	if slots < 1 {
		slots = 1
	}
	existing, err := h.trials.DaySlots(ctx, userID, day.Day)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load today's sessions")
		return
	}

	built := make([]map[string]any, 0, slots)
	for slot := 1; slot <= slots; slot++ {
		slotCats := catIDs
		if slots > 1 && len(catIDs) >= slots {
			slotCats = []string{catIDs[slot-1]}
		}
		label := slotLabel(slots, slot)

		if prior, ok := existing[slot]; ok && prior.SessionID == "" {
			// Another request is building this slot, or one died while doing
			// so. A claim older than the longest build is abandoned and is
			// taken over rather than blocking the day forever.
			took, err := h.trials.TakeOverStaleDaySlot(ctx, userID, day.Day, slot, now.Add(-trialSlotClaimTTL), now)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "failed to prepare today's session")
				return
			}
			if !took {
				writeCode(w, http.StatusConflict, "TRIAL_DAY_PREPARING", "today's session is still being prepared; try again in a moment")
				return
			}
		} else if ok {
			if sess, err := h.sess.ByID(ctx, prior.SessionID); err == nil && sess.UserID == userID {
				h.signSessionAudio(ctx, sess, ent)
				built = append(built, map[string]any{"slot": slot, "label": label, "session": sess})
				continue
			}
			// The listener deleted that session; build a replacement into the
			// slot this account already owns.
		} else {
			claimed, err := h.trials.ClaimDaySlot(ctx, current.ID, userID, day.Day, slot, now)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "failed to prepare today's session")
				return
			}
			if !claimed {
				writeCode(w, http.StatusConflict, "TRIAL_DAY_PREPARING", "today's session is still being prepared; try again in a moment")
				return
			}
		}

		sess, err := h.buildTrialSession(ctx, userID, day, label, slotCats, duration, voiceID, ent)
		if err != nil {
			h.releaseTrialSlot(ctx, userID, day.Day, slot)
			writeEngineError(w, err, ent)
			return
		}
		if err := h.trials.AttachDaySession(ctx, userID, day.Day, slot, sess.ID, now); err != nil {
			h.releaseTrialSlot(ctx, userID, day.Day, slot)
			httpx.WriteError(w, http.StatusInternalServerError, "failed to record today's session")
			return
		}
		h.signSessionAudio(ctx, sess, ent)
		built = append(built, map[string]any{"slot": slot, "label": label, "session": sess})
	}

	resp["action"] = "play"
	if day.Summary {
		resp["action"] = "review"
	}
	resp["sessions"] = built
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// trialSlotClaimTTL is how long an unattached claim is trusted to belong to a
// build still in progress. A build takes seconds; anything older was abandoned.
const trialSlotClaimTTL = 2 * time.Minute

// releaseTrialSlot drops a claim whose build or attach failed. It runs on a
// context that outlives the request: when the failure *is* the client going
// away, the request context is already cancelled and a release on it would
// fail too, leaving the slot claimed.
func (h *Handler) releaseTrialSlot(ctx context.Context, userID string, day, slot int) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := h.trials.ReleaseDaySlot(rctx, userID, day, slot); err != nil {
		log.Printf("trial today: release slot %d/%d: %v", day, slot, err)
	}
}

// buildTrialSession builds and saves one journey session through the same
// engine path as POST /sessions.
func (h *Handler) buildTrialSession(ctx context.Context, userID string, day billing.TrialDay, label string, catIDs []string, duration int, voiceID string, ent interface{ MaxSessionSeconds() int }) (*models.Session, error) {
	fav, recent := h.selectionHints(ctx, userID)
	sess, err := h.engn.Build(ctx, engine.Request{
		UserID:             userID,
		CategoryIDs:        catIDs,
		DurationSeconds:    duration,
		Strategy:           engine.NormalizeStrategy(""),
		VoiceID:            voiceID,
		FavoriteIDs:        fav,
		RecentIDs:          recent,
		MaxDurationSeconds: ent.MaxSessionSeconds(),
	})
	if err != nil {
		return nil, err
	}
	sess.Title = day.Title
	if label != "" {
		sess.Title += " · " + label
	}
	sess.Description = day.Description
	if err := h.sess.Create(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// premiumVoiceID returns the first Premium voice that can actually be played,
// or "" when none is ready. Availability uses the engine's own rule (active
// and playable), so the day never names a voice the engine would refuse.
func (h *Handler) premiumVoiceID(ctx context.Context) string {
	voices, err := h.audio.ListVoices(ctx)
	if err != nil {
		return ""
	}
	for _, v := range voices {
		if v.Premium && v.Status == "active" && v.Playable {
			return v.ID
		}
	}
	return ""
}

func slotLabel(slots, slot int) string {
	if slots != 2 {
		return ""
	}
	if slot == 1 {
		return "Morning"
	}
	return "Evening"
}
