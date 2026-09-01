package api

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

// Collections, devices, notifications and data export (§35–§37, §45, §46, §49).
//
// Every handler derives the user from the authenticated session. None accepts a
// user id from the path or body (§72).

// ---------------------------------------------------------------------------
// Collections
// ---------------------------------------------------------------------------

func (h *Handler) listCollections2(w http.ResponseWriter, r *http.Request) {
	cols, err := h.library.Collections(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load collections")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cols)
}

func (h *Handler) createCollection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Visibility  string `json:"visibility"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "invalid request body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || utf8.RuneCountInString(name) > 80 {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "name is required and must be 80 characters or fewer")
		return
	}
	if utf8.RuneCountInString(req.Description) > 500 {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "description must be 500 characters or fewer")
		return
	}
	// An unspecified or unrecognised visibility becomes private, never public.
	visibility := models.VisibilityPrivate
	if v := req.Visibility; validVisibility(v) {
		visibility = v
	}

	c := &models.UserCollection{
		UserID: h.userID(r), Name: name,
		Description: strings.TrimSpace(req.Description), Visibility: visibility,
	}
	if err := h.library.CreateCollection(r.Context(), c); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create collection")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (h *Handler) getCollection(w http.ResponseWriter, r *http.Request) {
	c, err := h.library.Collection(r.Context(), h.userID(r), r.PathValue("id"))
	if err != nil {
		writeCollectionError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (h *Handler) updateCollection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Visibility  *string `json:"visibility"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "invalid request body")
		return
	}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" || utf8.RuneCountInString(n) > 80 {
			writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "name must be 1-80 characters")
			return
		}
		req.Name = &n
	}
	if req.Visibility != nil && !validVisibility(*req.Visibility) {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID",
			"visibility must be private, unlisted or public")
		return
	}

	if err := h.library.UpdateCollection(r.Context(), h.userID(r), r.PathValue("id"),
		req.Name, req.Description, req.Visibility); err != nil {
		writeCollectionError(w, err)
		return
	}
	h.getCollection(w, r)
}

func (h *Handler) deleteCollection(w http.ResponseWriter, r *http.Request) {
	if err := h.library.DeleteCollection(r.Context(), h.userID(r), r.PathValue("id")); err != nil {
		writeCollectionError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "collection deleted"})
}

func (h *Handler) addCollectionItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConfessionID string `json:"confession_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.ConfessionID == "" {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "confession_id is required")
		return
	}
	// Verify the confession exists, so a collection cannot accumulate dangling
	// references that break every later read.
	if _, err := h.cont.ConfessionByID(r.Context(), req.ConfessionID); err != nil {
		writeCode(w, http.StatusUnprocessableEntity, "RESOURCE_NOT_FOUND", "unknown confession")
		return
	}
	if err := h.library.AddCollectionItem(r.Context(), h.userID(r), r.PathValue("id"), req.ConfessionID); err != nil {
		writeCollectionError(w, err)
		return
	}
	h.getCollection(w, r)
}

func (h *Handler) removeCollectionItem(w http.ResponseWriter, r *http.Request) {
	if err := h.library.RemoveCollectionItem(r.Context(), h.userID(r),
		r.PathValue("id"), r.PathValue("confessionId")); err != nil {
		writeCollectionError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "item removed"})
}

func (h *Handler) reorderCollection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConfessionIDs []string `json:"confession_ids"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "invalid request body")
		return
	}
	if err := h.library.ReorderCollection(r.Context(), h.userID(r), r.PathValue("id"), req.ConfessionIDs); err != nil {
		writeCollectionError(w, err)
		return
	}
	h.getCollection(w, r)
}

// writeCollectionError maps store errors to responses.
//
// A forbidden resource is reported as 404, not 403: telling an attacker that a
// collection exists but is not theirs confirms the id is real (§71).
func writeCollectionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrForbidden):
		writeCode(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "collection not found")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "collection operation failed")
	}
}

func validVisibility(v string) bool {
	switch v {
	case models.VisibilityPrivate, models.VisibilityUnlisted, models.VisibilityPublic:
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Devices
// ---------------------------------------------------------------------------

func (h *Handler) registerDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceID   string `json:"device_id"`
		Platform   string `json:"platform"`
		DeviceName string `json:"device_name"`
		AppVersion string `json:"app_version"`
		PushToken  string `json:"push_token"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || strings.TrimSpace(req.DeviceID) == "" {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "device_id is required")
		return
	}
	if req.Platform != "" && !oneOf(req.Platform, "ios", "android", "web") {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "platform must be ios, android or web")
		return
	}
	// The push token is accepted but not echoed back: it is a delivery
	// credential, not profile data.
	if err := h.library.RegisterDevice(r.Context(), h.userID(r), &models.UserDevice{
		DeviceID: strings.TrimSpace(req.DeviceID), Platform: req.Platform,
		Name: req.DeviceName, AppVersion: req.AppVersion,
	}); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to register device")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "device registered"})
}

func (h *Handler) listDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.library.Devices(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load devices")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, devices)
}

func (h *Handler) revokeDevice(w http.ResponseWriter, r *http.Request) {
	err := h.library.RevokeDevice(r.Context(), h.userID(r), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "device not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to remove device")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "device removed"})
}

// ---------------------------------------------------------------------------
// Notification preferences
// ---------------------------------------------------------------------------

func (h *Handler) getNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	p, err := h.library.NotificationPreferences(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load notification preferences")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"preferences": p,
		// Made explicit so a user is not misled into thinking they can silence
		// security mail from here (§46).
		"note": "Security notifications are always sent and cannot be disabled.",
	})
}

func (h *Handler) updateNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ScheduledSessions *bool `json:"scheduled_sessions"`
		NewContent        *bool `json:"new_content"`
		Recommendations   *bool `json:"recommendations"`
		ProductUpdates    *bool `json:"product_updates"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "invalid request body")
		return
	}
	p, err := h.library.UpdateNotificationPreferences(r.Context(), h.userID(r),
		req.ScheduledSessions, req.NewContent, req.Recommendations, req.ProductUpdates)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update notification preferences")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

// ---------------------------------------------------------------------------
// Data export (§49)
// ---------------------------------------------------------------------------

// exportData returns everything the platform holds about the caller.
//
// Deliberately excluded: password hashes, session tokens, one-time tokens and
// provider credentials. An export is a copy of the user's own data, not a dump
// of the security material protecting it — a leaked export must not be a
// credential.
func (h *Handler) exportData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := h.userID(r)

	u, err := h.users.ByID(ctx, userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to build export")
		return
	}
	profile, _ := h.profiles.Profile(ctx, userID)
	prefs, _ := h.profiles.Preferences(ctx, userID)
	interests, _ := h.profiles.Interests(ctx, userID)
	notifications, _ := h.library.NotificationPreferences(ctx, userID)
	collections, _ := h.library.Collections(ctx, userID)
	devices, _ := h.library.Devices(ctx, userID)
	identities, _ := h.users.ListIdentities(ctx, userID)
	userConfessions, _ := h.eng.ListUserConfessions(ctx, userID)
	favorites, _ := h.eng.ListFavorites(ctx, userID, "")
	sessions, _ := h.sess.ListByUser(ctx, userID, 500)
	schedules, _ := h.sched.ListByUser(ctx, userID)

	// Attachment disposition so a browser saves the file rather than rendering
	// personal data inline.
	w.Header().Set("Content-Disposition", `attachment; filename="iconfess-export.json"`)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"account": map[string]any{
			"id": u.ID, "email": u.Email, "display_name": u.DisplayName,
			"timezone": u.Timezone, "status": u.Status, "created_at": u.CreatedAt,
		},
		"profile":                  profile,
		"preferences":              prefs,
		"notification_preferences": notifications,
		"interests":                interests,
		"collections":              collections,
		"devices":                  devices,
		"sign_in_methods":          identities,
		"favorites":                favorites,
		"sessions":                 sessions,
		"schedules":                schedules,
		"my_confessions":           userConfessions,
		"excluded": []string{
			"password hashes",
			"session and refresh tokens",
			"email verification and password reset tokens",
			"third-party provider credentials",
		},
	})
}
