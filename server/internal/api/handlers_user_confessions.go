package api

import (
	"net/http"
	"strings"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/moderation"
)

func (h *Handler) createUserConfession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title      string `json:"title"`
		Text       string `json:"text"`
		CategoryID string `json:"category_id"`
		Visibility string `json:"visibility"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" || req.Text == "" {
		httpx.WriteError(w, http.StatusBadRequest, "title and text are required")
		return
	}
	// PRIVATE is the default and the floor (PRD §22): asking for a public
	// audience is allowed at creation, but the moderation review is what
	// actually publishes - visibility is the author's intent, not the outcome.
	visibility := req.Visibility
	if visibility == "" {
		visibility = moderation.VisibilityPrivate
	}
	if !moderation.ValidVisibility(visibility) {
		httpx.WriteError(w, http.StatusBadRequest,
			"visibility must be one of: "+strings.Join(moderation.UGCVisibilities(), ", "))
		return
	}
	uc := &models.UserConfession{
		UserID:     h.userID(r),
		Title:      req.Title,
		Text:       req.Text,
		CategoryID: req.CategoryID,
		IsPrivate:  visibility == moderation.VisibilityPrivate,
		Status:     string(moderation.UGCDraft),
		Visibility: visibility,
	}
	if err := h.eng.CreateUserConfession(r.Context(), uc); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create confession")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, uc)
}

func (h *Handler) listUserConfessions(w http.ResponseWriter, r *http.Request) {
	list, err := h.eng.ListUserConfessions(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confessions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func validEntityType(t string) bool {
	switch t {
	case "confession", "category", "session", "voice":
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Session management (PRD S30, S31, S54)
// ---------------------------------------------------------------------------

// listSessions returns the caller's live sign-ins for the security screen.
//
// It deliberately reports no geolocation: an IP-derived city is frequently
// wrong and turns a security feature into a source of false alarms. Device and
// last-used time are enough to recognise a session (S31).
