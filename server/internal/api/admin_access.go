package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/content"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/store"
)

// adminAccess reports the caller's role and the modules it may open, read
// from the same matrix the router enforces (PHASE 44, master-plan 40). The
// admin command centre builds its navigation from this response. It is not a
// permission check — every route still enforces its own module — it is the
// map, so a client never has to guess which doors are locked.
func (h *Handler) adminAccess(w http.ResponseWriter, r *http.Request) {
	c := auth.FromContext(r)
	if c == nil || c.Role == "" {
		httpx.WriteError(w, http.StatusForbidden, "admin access required")
		return
	}
	held := auth.ModulesFor(c.Role)
	modules := make([]map[string]any, 0, len(held))
	for _, m := range auth.Modules() {
		access, ok := held[m]
		if !ok {
			continue
		}
		modules = append(modules, map[string]any{
			"module": string(m),
			"access": string(access),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id":     c.Sub,
		"email":       c.Email,
		"role":        c.Role,
		"super_admin": c.Role == auth.RoleSuperAdmin,
		"modules":     modules,
	})
}

// adminGetTheologicalReview reads the review record on a confession.
func (h *Handler) adminGetTheologicalReview(w http.ResponseWriter, r *http.Request) {
	rec, err := h.cont.TheologicalReview(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "confession not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to read review")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, rec)
}

// adminRecordTheologicalReview records a reviewer's outcome (directive §22).
//
// The outcome is one of content.ReviewOutcomes(). Sending text back requires
// notes: a reviewer who says "needs revision" without saying what must change
// has not reviewed it. The reviewer of record is the caller's email, which is
// what the audit trail already uses, so the two agree. Recording the outcome
// does not move the lifecycle — that stays the content admin's PATCH — but the
// theological_review → audio_production edge is gated on a "reviewed" outcome,
// so the outcome is load-bearing, not advisory.
func (h *Handler) adminRecordTheologicalReview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status string `json:"status"`
		Notes  string `json:"notes"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeCode(w, http.StatusBadRequest, "REVIEW_INVALID", "invalid request body")
		return
	}
	req.Status = strings.TrimSpace(req.Status)
	req.Notes = strings.TrimSpace(req.Notes)
	if !content.ValidReviewOutcome(req.Status) {
		writeCode(w, http.StatusBadRequest, "REVIEW_INVALID",
			"status must be reviewed or needs_revision")
		return
	}
	if req.Status == string(content.ReviewNeedsRevision) && req.Notes == "" {
		writeCode(w, http.StatusBadRequest, "REVIEW_NOTES_REQUIRED",
			"sending a confession back requires notes saying what must change")
		return
	}
	if len(req.Notes) > 4000 {
		writeCode(w, http.StatusBadRequest, "REVIEW_INVALID", "notes must be 4000 characters or fewer")
		return
	}
	reviewer := ""
	if c := auth.FromContext(r); c != nil {
		reviewer = c.Email
	}
	rec, err := h.cont.RecordTheologicalReview(r.Context(), r.PathValue("id"), req.Status, reviewer, req.Notes)
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "confession not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to record review")
		return
	}
	h.recordAudit(r, "theological_review_"+req.Status, "confession", r.PathValue("id"), req.Notes, req.Status)
	httpx.WriteJSON(w, http.StatusOK, rec)
}

// adminAnalytics is the analytics module's overview: events, the trial funnel,
// subscriptions by status and session activity for a window (?days=30 by
// default, 1–365). It is the first route the analytics_admin role has ever
// held; the per-listener funnel already existed under /subscriptions/trial/
// engagement, but nothing showed the population.
func (h *Handler) adminAnalytics(w http.ResponseWriter, r *http.Request) {
	days := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 365 {
			writeCode(w, http.StatusBadRequest, "ANALYTICS_INVALID", "days must be between 1 and 365")
			return
		}
		days = n
	}
	until := time.Now().UTC()
	out, err := h.analytics.Overview(r.Context(), until.AddDate(0, 0, -days), until)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to compute analytics")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"days": days, "overview": out})
}
