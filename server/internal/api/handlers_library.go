package api

import (
	"errors"
	"net/http"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/sessions"
	"github.com/Teamthy/i-confess/internal/store"
)

func (h *Handler) addFavorite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || !validEntityType(req.EntityType) || req.EntityID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid favorite")
		return
	}
	f, err := h.eng.AddFavorite(r.Context(), h.userID(r), req.EntityType, req.EntityID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to add favorite")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, f)
}

func (h *Handler) removeFavorite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.EntityID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid favorite")
		return
	}
	if err := h.eng.RemoveFavorite(r.Context(), h.userID(r), req.EntityType, req.EntityID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to remove favorite")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listFavorites returns the caller's favourites with display names resolved.
//
// The bare table rows carry only (entity_type, entity_id), which a library
// screen cannot render as anything but opaque ids — which is exactly what the
// favourites tab showed before PHASE 27. An optional `type` narrows the list
// to one kind of entity.
func (h *Handler) listFavorites(w http.ResponseWriter, r *http.Request) {
	entityType := r.URL.Query().Get("type")
	// A filter the vocabulary does not contain would silently return nothing,
	// which reads to the caller as "you have no favourites" rather than "that
	// is not a thing you can favourite".
	if entityType != "" && !validEntityType(entityType) {
		writeCode(w, http.StatusBadRequest, "VALIDATION_FAILED",
			"type must be one of confession, category, session, voice")
		return
	}
	list, err := h.eng.ListFavoritesDetailed(r.Context(), h.userID(r), entityType)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load favorites")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	list, err := h.eng.History(r.Context(), h.userID(r), 50)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load history")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) recordPlayback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID       string `json:"session_id"`
		ConfessionID    string `json:"confession_id"`
		DurationSeconds int    `json:"duration_seconds"`
		Completed       bool   `json:"completed"`
		Skipped         bool   `json:"skipped"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Prevent forged playback records:
	// A client request alone must not prove that audio played or completed.
	if req.SessionID != "" {
		sess, err := h.sess.ByID(r.Context(), req.SessionID)
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
		// Refuse forged completion of a session that has never started playback and has no listened duration
		if req.Completed && req.DurationSeconds <= 0 && sess.StartedAt == "" && sess.Status != string(sessions.Completed) && sess.Status != string(sessions.Active) && sess.Status != string(sessions.Paused) && sess.Status != string(sessions.Interrupted) {
			httpx.WriteJSON(w, http.StatusConflict, map[string]any{
				"error": "a session can only be completed after playback has started",
				"code":  "INVALID_TRANSITION",
			})
			return
		}
	}

	rec := &models.PlaybackRecord{
		UserID:          h.userID(r),
		SessionID:       req.SessionID,
		ConfessionID:    req.ConfessionID,
		DurationSeconds: req.DurationSeconds,
		Completed:       req.Completed,
		Skipped:         req.Skipped,
	}
	if err := h.eng.RecordPlayback(r.Context(), rec); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to record playback")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, rec)
}

// ---------- User confessions ----------
