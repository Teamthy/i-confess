package api

import (
	"net/http"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/httpx"
)

func (h *Handler) adminSetUserRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !isValidAdminRole(req.Role) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid admin role")
		return
	}

	// Verify user exists
	if _, err := h.users.ByID(r.Context(), req.UserID); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "user not found")
		return
	}

	if err := h.users.SetAdminRole(r.Context(), req.UserID, req.Role); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to set admin role")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "admin role set successfully"})
}

// adminRemoveUserRole removes a user's admin privileges.
func (h *Handler) adminRemoveUserRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.users.RemoveAdminRole(r.Context(), req.UserID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to remove admin role")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "admin role removed successfully"})
}

// adminListAdmins returns all users with admin roles.
func (h *Handler) adminListAdmins(w http.ResponseWriter, r *http.Request) {
	admins, err := h.users.ListAllAdmins(r.Context(), 100)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load admins")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, admins)
}

// adminSetUserStatus sets a user's account status (active, suspended, deleted, etc).
func (h *Handler) adminSetUserStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Status string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !isValidUserStatus(req.Status) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid user status")
		return
	}

	if err := h.users.SetStatus(r.Context(), req.UserID, req.Status); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to set user status")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "user status updated successfully"})
}

// Helper functions
func isValidAdminRole(role string) bool {
	// Use RBAC system roles plus legacy admin
	if _, ok := auth.SystemRoles[role]; ok {
		return true
	}
	// Also allow custom roles that exist in DB will be checked elsewhere, but for this helper we allow any non-empty
	// that matches role name pattern - the RBAC store will validate existence
	if role == "admin" {
		return true
	}
	return false
}

func isValidUserStatus(status string) bool {
	switch status {
	case "active", "suspended", "deleted":
		return true
	}
	return false
}

// ---------- Content ----------
