package api

import (
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

	// Same builder, same inputs as an ad-hoc session: a schedule is a saved
	// set of choices, not a separate code path that can drift.
	sess, err := h.engn.Build(r.Context(), engine.Request{
		UserID:          userID,
		CategoryIDs:     cats,
		VoiceID:         sched.VoiceID,
		DurationSeconds: duration,
		Strategy:        engine.NormalizeStrategy(""),
	})
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

// --- Not yet implemented -------------------------------------------------
//
// These five routes are registered in the v1 tree but their services are not
// wired into the Handler yet: recommendations, billing and entitlements exist
// as standalone packages with no constructors, and the moderation queue has no
// store at all. They answer 501 rather than a fabricated response, so a client
// integrating against this API learns the truth immediately instead of
// discovering it in production.
//
// Two of the original six - subscription and entitlements - were removed in
// PHASE 08. Their packages existed and were already used elsewhere in this
// package; the comment describing them as having "no constructors" was simply
// wrong, and a 501 behind a false explanation outlived the reason for it.
//
// Tracked in docs/AUDIT-2026-09-05.md.

func notImplemented(what string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusNotImplemented, map[string]any{
			"error": what + " is not implemented yet",
			"code":  "NOT_IMPLEMENTED",
		})
	}
}

func (h *Handler) recommendations(w http.ResponseWriter, r *http.Request) {
	notImplemented("recommendations")(w, r)
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

func (h *Handler) adminQAConfession(w http.ResponseWriter, r *http.Request) {
	notImplemented("confession QA")(w, r)
}

func (h *Handler) adminListModerationQueue(w http.ResponseWriter, r *http.Request) {
	notImplemented("the moderation queue")(w, r)
}

func (h *Handler) adminReviewUserConfession(w http.ResponseWriter, r *http.Request) {
	notImplemented("user confession review")(w, r)
}
