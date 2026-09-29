package api

import (
	"net/http"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/httpx"
)

func (h *Handler) listAuthSessions(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r)
	if claims == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	authSessions, err := h.users.ListAuthSessions(r.Context(), claims.Sub)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load sessions")
		return
	}
	for i := range authSessions {
		authSessions[i].Current = authSessions[i].ID == claims.SessionID
	}
	httpx.WriteJSON(w, http.StatusOK, authSessions)
}

// revokeAuthSession signs out one device.
func (h *Handler) revokeAuthSession(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r)
	if claims == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "session id is required")
		return
	}
	// Scoped to the caller in SQL, so one user cannot revoke another's session
	// even by guessing an id (S71).
	if err := h.users.RevokeAuthSession(r.Context(), claims.Sub, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to revoke session")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "session revoked"})
}
