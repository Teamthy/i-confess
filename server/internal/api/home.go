package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Teamthy/i-confess/internal/engine"
	"github.com/Teamthy/i-confess/internal/entitlements"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/search"
)

// home is the landing feed (§14): what to listen to next, and what the
// listener was in the middle of.
//
// It is deliberately assembled from data the product already has rather than
// from a generic "featured content" block. A listener opening the app after a
// three-day gap should land on their own interrupted session, not on a
// marketing carousel.
func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)

	out := map[string]any{}

	if userID != "" {
		sessions, err := h.sess.ListByUser(r.Context(), userID, 12)
		if err == nil {
			out["continue"] = resumable(sessions)
			out["recent_sessions"] = sessions
		}
		if collections, err := h.library.Collections(r.Context(), userID); err == nil {
			out["collections"] = collections
		}
	}

	// Categories are always served from the backend (§7): the client renders
	// whatever this returns and never carries its own list.
	if cats, err := h.cont.ListCategories(r.Context(), false); err == nil {
		out["categories"] = cats
	}

	httpx.WriteJSON(w, http.StatusOK, out)
}

// resumable finds the session worth offering as "continue listening": the most
// recent one that is still open. Completed, cancelled and expired sessions are
// finished business and offering them is just noise.
func resumable(sessions []models.Session) *models.Session {
	var best *models.Session
	var bestAt time.Time
	for i := range sessions {
		s := &sessions[i]
		switch s.Status {
		case "ACTIVE", "PAUSED", "INTERRUPTED", "READY", "SCHEDULED", "STARTING":
		default:
			continue
		}
		at, err := time.Parse(time.RFC3339, s.CreatedAt)
		if err != nil {
			continue
		}
		if best == nil || at.After(bestAt) {
			best, bestAt = s, at
		}
	}
	return best
}

// triggerSchedule builds a session from a schedule immediately (§14, §19).
//
// This is the manual equivalent of the scheduler firing at the appointed time:
// the same session gets built, so "start it now" and "wait for 6am" produce
// the same artefact.
func (h *Handler) triggerSchedule(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)
	sched, err := h.sched.ByID(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "schedule not found")
		return
	}
	if sched.UserID != userID {
		httpx.WriteError(w, http.StatusForbidden, "not your schedule")
		return
	}

	cats := sched.CategoryIDs
	if len(cats) == 0 {
		if list, err := h.cont.ListCategories(r.Context(), false); err == nil {
			for _, c := range list {
				cats = append(cats, c.ID)
			}
		}
	}
	duration := sched.DurationSeconds
	if duration <= 0 {
		duration = 10 * 60
	}

	// Entitlement is evaluated here, when the session is actually built, and
	// not only when the schedule was created. A schedule is a saved intention
	// that fires later: the plan in force at 6am is the plan that applies at
	// 6am. Checking at creation alone would let a user subscribe, save a
	// three-hour schedule, downgrade, and keep receiving three-hour sessions.
	//
	// This was the PHASE 13 finding. The ad-hoc path returned 402 for a
	// duration over the plan limit and this path built the session anyway,
	// because the check lived in one handler rather than in the engine.
	ent := h.entitlementsFor(r.Context(), userID)
	if max := ent.MaxSessionSeconds(); duration > max {
		writePlanLimit(w, ent)
		return
	}

	// Same builder, same inputs as an ad-hoc session: a schedule is a saved
	// set of choices, not a separate code path that can drift.
	sess, err := h.engn.Build(r.Context(), engine.Request{
		UserID:          userID,
		CategoryIDs:     cats,
		VoiceID:         sched.VoiceID,
		DurationSeconds: duration,
		Strategy:        engine.NormalizeStrategy(""),

		MaxDurationSeconds: ent.MaxSessionSeconds(),
	})
	if errors.Is(err, engine.ErrDurationExceedsPlan) {
		writePlanLimit(w, ent)
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to build session")
		return
	}
	if err := h.sess.Create(r.Context(), sess); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save session")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, sess)
}

// searchAll serves the v1 search route. The v1 tree registered the SearchStore
// itself where a handler belongs; this is the handler.
func (h *Handler) searchAll(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 20
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	types := q["type"]
	if len(types) == 0 {
		types = []string{"confession", "category", "collection", "voice"}
	}
	results, err := h.search.Search(r.Context(), search.SearchRequest{
		Query:         q.Get("q"),
		Types:         types,
		Limit:         limit,
		PublishedOnly: true,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "search failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": results, "count": len(results)})
}

// The last 501 stubs on this surface were removed in PHASE 31: the moderation
// queue, user-confession review and confession QA now have a store and live in
// internal/api/moderation.go. The original six are all real; see
// docs/AUDIT-2026-09-05.md for the audit that listed them.

func (h *Handler) recommendations(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Load signals for deterministic personalization (v1):
	// - explicit interests (what user said)
	// - recent sessions (what user actually did)
	// - all categories (fallback)
	cats, err := h.cont.ListCategories(r.Context(), false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load categories")
		return
	}

	// Build interest map
	interestWeight := map[string]float64{}
	if interests, err := h.profiles.Interests(r.Context(), userID); err == nil {
		for _, it := range interests {
			interestWeight[it.CategoryID] = interestWeight[it.CategoryID] + float64(it.Weight)
		}
	}

	// Build recency map from sessions
	recentCount := map[string]int{}
	if sessions, err := h.sess.ListByUser(r.Context(), userID, 20); err == nil {
		for _, s := range sessions {
			// Sessions don't directly store category_ids in the list view,
			// but we can count via items' categories if available, or use a
			// simple heuristic: if session has items, count first item's category
			// via confession lookup is expensive, so we count session types.
			// For v1, we use session creation recency as a signal to boost
			// diversity, not exact category affinity.
			_ = s
		}
		// For v1, we don't have category breakdown in ListByUser without items.
		// We keep recentCount empty and rely on interests; future versions can
		// join session_items -> confessions -> categories.
	}

	// Score categories: interests first, then alphabetical for determinism
	type scored struct {
		cat   models.Category
		score float64
	}
	scoredCats := make([]scored, 0, len(cats))
	for _, c := range cats {
		score := interestWeight[c.ID]*10 + float64(recentCount[c.ID])
		// Boost non-premium for free users? No, show all but mark premium.
		// Deterministic tie-breaker: sort_order then name.
		scoredCats = append(scoredCats, scored{cat: c, score: score})
	}

	// Sort by score desc, then sort_order asc, then name asc for determinism
	// (so same interests always yield same recommendations, required for v1).
	for i := 0; i < len(scoredCats); i++ {
		for j := i + 1; j < len(scoredCats); j++ {
			si, sj := scoredCats[i], scoredCats[j]
			less := false
			if si.score != sj.score {
				less = si.score < sj.score
			} else if si.cat.SortOrder != sj.cat.SortOrder {
				less = si.cat.SortOrder > sj.cat.SortOrder
			} else {
				less = si.cat.Name > sj.cat.Name
			}
			if less {
				scoredCats[i], scoredCats[j] = scoredCats[j], scoredCats[i]
			}
		}
	}

	// Top 6 categories
	topN := 6
	if len(scoredCats) < topN {
		topN = len(scoredCats)
	}
	recommendedCats := make([]models.Category, 0, topN)
	recommendedCatIDs := make([]string, 0, topN)
	for i := 0; i < topN; i++ {
		recommendedCats = append(recommendedCats, scoredCats[i].cat)
		recommendedCatIDs = append(recommendedCatIDs, scoredCats[i].cat.ID)
	}

	// Gather confessions from top categories (2 each, max 12)
	var recommendedConfessions []models.Confession
	for _, catID := range recommendedCatIDs {
		confs, err := h.cont.ConfessionsByCategory(r.Context(), catID, true)
		if err != nil || len(confs) == 0 {
			continue
		}
		// Take up to 2 per category, deterministic by sort (already ordered by sort_order)
		take := 2
		if len(confs) < take {
			take = len(confs)
		}
		recommendedConfessions = append(recommendedConfessions, confs[:take]...)
		if len(recommendedConfessions) >= 12 {
			break
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"categories":   recommendedCats,
		"confessions":  recommendedConfessions,
		"count":        len(recommendedCats) + len(recommendedConfessions),
		"personalized": len(interestWeight) > 0,
	})
}

// getSubscription reports the caller's own plan and its status.
//
// This answered 501 with a comment claiming the subscription package had "no
// constructors". It did: UserStore.Subscription and entitlements.FromPlan were
// both already in use elsewhere in this package. The 501 was not a missing
// dependency, it was an unwritten function.
func (h *Handler) getSubscription(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	plan, status, err := h.users.SubscriptionState(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not load subscription")
		return
	}
	ent := entitlements.FromPlan(plan)
	// "active" means a live paid subscription, not merely that the row's status
	// column reads active. A free user has a subscription row too, and a client
	// that shows "Premium" because a boolean said true is worse than one that
	// shows nothing. The test that caught this had a free user with an active
	// row and expected active to be false.
	active := status == "active" && ent.Plan == entitlements.PlanPremium
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"plan":                plan,
		"status":              status,
		"active":              active,
		"max_session_seconds": ent.MaxSessionSeconds(),
	})
}

// getEntitlements reports what the caller may do, as decided by the server.
//
// Section 35: the client never asserts its own plan. It reads this and obeys
// it, and the API re-checks on every privileged action regardless.
func (h *Handler) getEntitlements(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	ent := h.entitlementsFor(r.Context(), userID)
	view := entitlementView(ent)
	view["playback_ttl_seconds"] = int(ent.PlaybackTTL().Seconds())
	httpx.WriteJSON(w, http.StatusOK, view)
}
