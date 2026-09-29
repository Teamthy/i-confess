package api

import (
	"errors"
	"net/http"

	"github.com/Teamthy/i-confess/internal/engine"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/sessions"
	"github.com/Teamthy/i-confess/internal/store"
)

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CategoryIDs     []string           `json:"category_ids"`
		CategoryWeights map[string]float64 `json:"category_weights"`
		DurationSeconds int                `json:"duration_seconds"`
		DurationPreset  string             `json:"duration_preset"`
		Strategy        string             `json:"strategy"`
		VoiceID         string             `json:"voice_id"`
		Title           string             `json:"title"`
		Description     string             `json:"description"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.CategoryIDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "at least one category is required")
		return
	}
	// The builder sends either an explicit length or a preset name. Presets
	// exist so that deep links and schedules can say "30 minutes" without
	// hardcoding 1800 on the client.
	if req.DurationSeconds == 0 && req.DurationPreset != "" {
		if engine.IsCustomPreset(req.DurationPreset) {
			httpx.WriteError(w, http.StatusBadRequest, "custom duration_preset requires duration_seconds")
			return
		}
		secs, ok := engine.PresetFor(req.DurationPreset)
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "unknown duration_preset — use 10m|15m|30m|45m|60m|90m|120m|180m, or send duration_seconds")
			return
		}
		req.DurationSeconds = secs
	}
	if req.DurationSeconds < 60 || req.DurationSeconds > 3*3600 {
		httpx.WriteError(w, http.StatusBadRequest, "duration must be between 1 minute and 3 hours")
		return
	}
	if req.Strategy != "" && !engine.IsValidStrategy(req.Strategy) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid strategy — use EXACT|CLOSEST|UNDER|OVER|BALANCED")
		return
	}

	// Session length is a plan capability (PRD S25/S26). Enforced server-side:
	// the client is never the authority on what it may request.
	ent := h.entitlementsFor(r.Context(), h.userID(r))
	if max := ent.MaxSessionSeconds(); req.DurationSeconds > max {
		writePlanLimit(w, ent)
		return
	}

	// Favorites + recency for content selection (§18)
	favs, _ := h.eng.ListFavorites(r.Context(), h.userID(r), "confession")
	favMap := map[string]bool{}
	for _, f := range favs {
		favMap[f.EntityID] = true
	}
	history, _ := h.eng.History(r.Context(), h.userID(r), 20)
	recentMap := map[string]bool{}
	for i, rec := range history {
		if i >= 10 {
			break
		}
		if rec.ConfessionID != "" {
			recentMap[rec.ConfessionID] = true
		}
	}

	sess, err := h.engn.Build(r.Context(), engine.Request{
		UserID:          h.userID(r),
		CategoryIDs:     req.CategoryIDs,
		CategoryWeights: req.CategoryWeights,
		DurationSeconds: req.DurationSeconds,
		Strategy:        engine.NormalizeStrategy(req.Strategy),
		VoiceID:         req.VoiceID,
		FavoriteIDs:     favMap,
		RecentIDs:       recentMap,

		// Re-stated for the engine even though this handler already returned
		// 402 above. The handler check gives the better error message; this
		// one is the guarantee that survives a future edit to the handler.
		MaxDurationSeconds: ent.MaxSessionSeconds(),
	})
	if errors.Is(err, engine.ErrNoVoice) {
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":  "no available active voice",
			"code":   "VOICE_UNAVAILABLE",
			"reason": "voice_unavailable",
		})
		return
	}
	if errors.Is(err, engine.ErrNoContent) {
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":  "no published content available for the selected categories and voice",
			"code":   "CONTENT_UNAVAILABLE",
			"reason": "content_unavailable",
		})
		return
	}
	if errors.Is(err, engine.ErrDurationTooShort) || errors.Is(err, engine.ErrDurationTooLong) {
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":  err.Error(),
			"code":   "DURATION_OUT_OF_BOUNDS",
			"reason": "duration_out_of_bounds",
		})
		return
	}
	if errors.Is(err, engine.ErrDurationExceedsPlan) {
		writePlanLimit(w, ent)
		return
	}
	if errors.Is(err, engine.ErrNoExactFit) {
		// Distinct from "no content": the library has material for these
		// categories, it just cannot land exactly on the requested length
		// without cutting a confession short, which we do not do.
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":  "no combination of complete confessions matches that exact length",
			"code":   "EXACT_DURATION_UNAVAILABLE",
			"reason": "exact_duration_unavailable",
			"hint":   "choose a nearby length, or use the balanced strategy",
		})
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to build session")
		return
	}
	// Optional metadata — client can name a session (long-form §22)
	if req.Title != "" {
		sess.Title = req.Title
	}
	if req.Description != "" {
		sess.Description = req.Description
	}
	if err := h.sess.Create(r.Context(), sess); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save session")
		return
	}
	h.signSessionAudio(r.Context(), sess, ent)
	httpx.WriteJSON(w, http.StatusCreated, sess)
}

func (h *Handler) getSession(w http.ResponseWriter, r *http.Request) {
	sess, err := h.sess.ByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load session")
		return
	}
	if sess.UserID != h.userID(r) {
		httpx.WriteError(w, http.StatusForbidden, "not your session")
		return
	}
	// Re-evaluated on every read: a plan can lapse between creating a session
	// and playing it.
	h.signSessionAudio(r.Context(), sess, h.entitlementsFor(r.Context(), h.userID(r)))
	httpx.WriteJSON(w, http.StatusOK, sess)
}

// updateSessionStatus moves a session along its lifecycle.
//
// The transition is validated against the state machine in the sessions
// package, not against a list of allowed values. That distinction is the whole
// point: completion is a product metric the business reports on, and without a
// transition check a client could PATCH a never-played session straight to
// COMPLETED. The state machine makes that unreachable rather than merely
// discouraged — see sessions.TestCompletionRequiresPlaybackOnEveryPath.
//
// An illegal move returns 409 with a reason the client can show the user; an
// unrecognised status string returns 400.
func (h *Handler) updateSessionStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := h.sess.ByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	if sess.UserID != h.userID(r) {
		httpx.WriteError(w, http.StatusForbidden, "not your session")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validSessionStatus(req.Status) {
		httpx.WriteError(w, http.StatusBadRequest, "unknown session status")
		return
	}
	to, _ := sessions.Parse(req.Status)

	// Rows written before the canonical vocabulary was introduced still hold
	// the legacy spellings; Normalize folds them onto the same states.
	from, ok := sessions.Parse(sess.Status)
	if !ok {
		// A persisted status we do not recognise means the row is corrupt
		// or from a schema we no longer understand. Refuse to guess.
		httpx.WriteError(w, http.StatusConflict, "session is in an unrecognised state")
		return
	}
	if !sessions.CanTransition(from, to) {
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error": sessions.Reason(from, to),
			"from":  string(from),
			"to":    string(to),
			"code":  "INVALID_TRANSITION",
		})
		return
	}
	if err := h.sess.UpdateStatus(r.Context(), id, string(to)); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update session")
		return
	}
	// Return the canonical state so clients converge on one vocabulary.
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": string(to)})
}

// validSessionStatus reports whether s names a session state the API accepts,
// including the legacy spellings still sent by older clients.
func validSessionStatus(s string) bool {
	_, ok := sessions.Parse(s)
	return ok
}

// ---------- Schedules ----------
