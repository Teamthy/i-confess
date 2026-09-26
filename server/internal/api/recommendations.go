package api

import (
	"net/http"
	"time"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/personalization"
)

// recommendations ranks the catalogue for the caller from the seven listener
// signals of master-plan item 34 (PHASE 43).
//
// Before this phase the endpoint read one thing — explicit interests — and
// reported `personalized: true` whenever the listener had typed any in. It
// could not tell a listener who finishes every morning session from one who
// skips half of what they start, because it never looked. It now reads what
// the listener did through SignalStore and hands the evidence to
// personalization.Rank, which is deterministic and unit-tested signal by
// signal. Nothing here learns; the rules are constants.
//
// Two preferences gate it, both the listener's own (PRD S14):
//
//   - personalization_enabled=false: behavioural signals are not read at all.
//     The catalogue is ranked on explicit interests only, exactly as before,
//     and `personalized` is false.
//   - recommendations_enabled=false: the endpoint answers with the catalogue in
//     its own order and no reasons, so a client that renders the response
//     still has something to show and nothing that was inferred.
//
// The response keeps the v1 shape (categories, confessions, count,
// personalized) so the shipped mobile client keeps working, and adds the
// evidence: `signals`, `listen_again`, `suggested_duration_seconds`,
// `daypart` and per-item `reasons`.
func (h *Handler) recommendations(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	ctx := r.Context()

	cats, err := h.cont.ListCategories(ctx, false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load categories")
		return
	}

	behavioural, inferred := true, true
	if prefs, err := h.profiles.Preferences(ctx, userID); err == nil && prefs != nil {
		behavioural = prefs.PersonalizationEnabled
		inferred = prefs.RecommendationsEnabled
	}

	in := personalization.Input{
		Categories:  cats,
		Confessions: map[string][]models.Confession{},
		Interests:   map[string]float64{},
		Now:         time.Now().UTC(),
	}
	if inferred {
		if interests, err := h.profiles.Interests(ctx, userID); err == nil {
			for _, it := range interests {
				in.Interests[it.CategoryID] += it.Weight
			}
		}
	}
	if inferred && behavioural {
		tz := ""
		if p, err := h.profiles.Profile(ctx, userID); err == nil && p != nil {
			tz = p.Timezone
		}
		sig, err := h.signals.Signals(ctx, userID, tz)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "failed to load listening signals")
			return
		}
		in.Signals = sig
	}
	// The ranking only needs confessions for categories it will surface, but
	// the repeat-listening rail can name a confession from any category, so
	// every category is loaded. The per-category read is the cached, published
	// projection the catalogue already serves.
	for _, c := range cats {
		confs, err := h.cont.ConfessionsByCategory(ctx, c.ID, true)
		if err != nil {
			continue
		}
		in.Confessions[c.ID] = confs
	}

	rec := personalization.Rank(in)

	categories := make([]models.Category, 0, len(rec.Categories))
	categoryReasons := map[string][]string{}
	for _, rc := range rec.Categories {
		categories = append(categories, rc.Category)
		if len(rc.Reasons) > 0 {
			categoryReasons[rc.Category.ID] = rc.Reasons
		}
	}
	confessions := make([]models.Confession, 0, len(rec.Confessions))
	confessionReasons := map[string][]string{}
	for _, rc := range rec.Confessions {
		confessions = append(confessions, rc.Confession)
		if len(rc.Reasons) > 0 {
			confessionReasons[rc.Confession.ID] = rc.Reasons
		}
	}
	again := make([]map[string]any, 0, len(rec.ListenAgain))
	for _, rc := range rec.ListenAgain {
		again = append(again, map[string]any{
			"confession": rc.Confession,
			"times":      int(rc.Score),
		})
	}

	rate, sample := in.Signals.CompletionRate()
	present := make([]string, 0, len(rec.Present))
	for _, s := range rec.Present {
		present = append(present, string(s))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"categories":   categories,
		"confessions":  confessions,
		"count":        len(categories) + len(confessions),
		"personalized": len(rec.Present) > 0,
		"listen_again": again,
		"reasons": map[string]any{
			"categories":  categoryReasons,
			"confessions": confessionReasons,
		},
		"suggested_duration_seconds": rec.SuggestedDurationSeconds,
		"daypart":                    rec.Daypart,
		"signals": map[string]any{
			"present":                  present,
			"categories_listened_to":   len(in.Signals.CategoriesListenedTo()),
			"completion_rate":          rate,
			"completion_sample":        sample,
			"preferred_daypart":        rec.PreferredDaypart,
			"typical_duration_seconds": in.Signals.TypicalDuration(),
			"favourites": len(in.Signals.Favourites.Confessions) +
				len(in.Signals.Favourites.Categories) + len(in.Signals.Favourites.Voices),
			"skips":            len(in.Signals.SkipsByConfession()),
			"repeat_listening": len(in.Signals.RepeatListens()),
		},
		"preferences": map[string]bool{
			"personalization_enabled": behavioural,
			"recommendations_enabled": inferred,
		},
	})
}
