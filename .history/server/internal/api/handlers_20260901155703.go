package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/engine"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

// Handler bundles all stores and config needed by the API.
type Handler struct {
	cfg   Config
	users *store.UserStore
	cont  *store.ContentStore
	audio *store.AudioStore
	sess  *store.SessionStore
	sched *store.ScheduleStore
	eng   *store.EngagementStore
	engn  *engine.Engine
	queue *jobs.MemoryQueue
}

type Config struct {
	JWTSecret string
	TokenTTL  string
}

func NewHandler(cfg Config, db *sql.DB) *Handler {
	return &Handler{
		cfg:   cfg,
		users: store.NewUserStore(db),
		cont:  store.NewContentStore(db),
		audio: store.NewAudioStore(db),
		sess:  store.NewSessionStore(db),
		sched: store.NewScheduleStore(db),
		eng:   store.NewEngagementStore(db),
		queue: jobs.NewMemoryQueue(),
	}
}

// BuildEngine wires the session engine after handler construction.
func (h *Handler) BuildEngine() {
	h.engn = engine.New(h.cont, h.audio, h.users)
}

// GetQueue returns the job queue for external wiring (e.g., workers).
func (h *Handler) GetQueue() *jobs.MemoryQueue {
	return h.queue
}

func (h *Handler) userID(r *http.Request) string {
	if c := auth.FromContext(r); c != nil {
		return c.Sub
	}
	if token := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(token, "Bearer ") {
		claims, err := auth.ParseToken(h.cfg.JWTSecret, strings.TrimPrefix(token, "Bearer "))
		if err == nil && claims != nil {
			return claims.Sub
		}
	}
	return ""
}

// ---------- Auth ----------

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"display_name"`
		Timezone string `json:"timezone"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || len(req.Password) < 8 {
		httpx.WriteError(w, http.StatusBadRequest, "email and a password of at least 8 characters are required")
		return
	}
	if _, _, err := h.users.ByEmail(r.Context(), req.Email); err == nil {
		httpx.WriteError(w, http.StatusConflict, "an account with this email already exists")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	tz := req.Timezone
	if tz == "" {
		tz = "UTC"
	}
	u, err := h.users.Create(r.Context(), req.Email, hash, req.Name, tz)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create account")
		return
	}
	h.issueToken(w, r, u)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	u, hash, err := h.users.ByEmail(r.Context(), req.Email)
	if err != nil || !auth.CheckPassword(hash, req.Password) {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if u.Status != "active" {
		httpx.WriteError(w, http.StatusForbidden, "account is disabled")
		return
	}
	h.issueToken(w, r, u)
}

func (h *Handler) issueToken(w http.ResponseWriter, r *http.Request, u *models.User) {
	role, _ := h.users.AdminRole(r.Context(), u.ID)
	tok, err := auth.SignToken(h.cfg.JWTSecret, h.cfg.TokenTTL, u.ID, u.Email, role)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"token": tok,
		"user":  u,
	})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	u, err := h.users.ByID(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	plan, _ := h.users.Subscription(r.Context(), u.ID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": u, "plan": plan})
}

// changePassword updates the user's password.
func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.NewPassword) < 8 {
		httpx.WriteError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}

	userID := h.userID(r)
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	u, err := h.users.ByID(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "user not found")
		return
	}

	_, hash, err := h.users.ByEmail(r.Context(), u.Email)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to verify current password")
		return
	}

	if !auth.CheckPassword(hash, req.CurrentPassword) {
		httpx.WriteError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to hash new password")
		return
	}

	if err := h.users.UpdatePassword(r.Context(), userID, newHash); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "password updated successfully"})
}

// refreshToken issues a new access token for an authenticated user.
func (h *Handler) refreshToken(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r)
	if claims == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	role, _ := h.users.AdminRole(r.Context(), claims.Sub)
	tok, err := auth.SignToken(h.cfg.JWTSecret, h.cfg.TokenTTL, claims.Sub, claims.Email, role)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"token": tok})
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct{ Token string `json:"token"` }
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Token == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid verification token")
		return
	}
	userID, err := h.users.UserIDByVerificationToken(r.Context(), req.Token)
	if err != nil || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid or expired verification token")
		return
	}
	if ok, err := h.users.VerifyEmailToken(r.Context(), userID, req.Token); err != nil || !ok {
		httpx.WriteError(w, http.StatusBadRequest, "verification failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "email verified"})
}

func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	var req struct{ Email string `json:"email"` }
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Email == "" {
		httpx.WriteError(w, http.StatusBadRequest, "email is required")
		return
	}
	user, hash, err := h.users.ByEmail(r.Context(), strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil || user == nil {
		httpx.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	if hash == "" {
		httpx.WriteError(w, http.StatusInternalServerError, "user record is invalid")
		return
	}
	if err := h.users.CreateVerificationToken(r.Context(), user.ID, "email", "verify-"+user.ID, 60); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create verification token")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "verification email sent"})
}

func (h *Handler) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req struct{ Email string `json:"email"` }
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Email == "" {
		httpx.WriteError(w, http.StatusBadRequest, "email is required")
		return
	}
	user, _, err := h.users.ByEmail(r.Context(), strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil || user == nil {
		httpx.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	if err := h.users.CreatePasswordReset(r.Context(), user.ID, "reset-"+user.ID, 30); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create reset token")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "password reset requested"})
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Token == "" || len(req.Password) < 8 {
		httpx.WriteError(w, http.StatusBadRequest, "token and a password of at least 8 characters are required")
		return
	}
	userID, err := h.users.UserIDByPasswordResetToken(r.Context(), req.Token)
	if err != nil || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid or expired reset token")
		return
	}
	newHash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := h.users.ResetPassword(r.Context(), userID, req.Token, newHash); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "password reset successfully"})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r)
	if claims == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if err := h.users.RevokeAllSessions(r.Context(), claims.Sub); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to revoke sessions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (h *Handler) logoutAll(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r)
	if claims == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if err := h.users.RevokeAllSessions(r.Context(), claims.Sub); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to revoke all sessions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "all sessions revoked"})
}

func (h *Handler) recordConsent(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r)
	if claims == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var req struct {
		Category string `json:"category"`
		Granted  bool   `json:"granted"`
		Version  string `json:"version"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Category == "" {
		httpx.WriteError(w, http.StatusBadRequest, "category and consent state are required")
		return
	}
	if err := h.users.RecordConsent(r.Context(), claims.Sub, req.Category, req.Version, r.RemoteAddr, r.UserAgent(), req.Granted); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to record consent")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "consent recorded"})
}

func (h *Handler) enableMFA(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r)
	if claims == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var req struct {
		Secret string `json:"secret"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Secret == "" {
		httpx.WriteError(w, http.StatusBadRequest, "mfa secret is required")
		return
	}
	if err := h.users.EnableMFA(r.Context(), claims.Sub, req.Secret); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to enable MFA")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "MFA enabled"})
}

func (h *Handler) securityEvent(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r)
	if claims == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var req struct {
		EventType string            `json:"event_type"`
		Metadata  map[string]string `json:"metadata"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.EventType == "" {
		httpx.WriteError(w, http.StatusBadRequest, "event_type is required")
		return
	}
	if err := h.users.RecordSecurityEvent(r.Context(), claims.Sub, req.EventType, r.RemoteAddr, r.UserAgent(), req.Metadata); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to record security event")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "security event recorded"})
}

// ---------- Admin User Management ----------

// adminSetUserRole sets or updates a user's admin role.
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
	switch role {
	case auth.RoleSuperAdmin, auth.RoleContentAdmin, auth.RoleAudioProducer,
		auth.RoleTheologicalRev, auth.RoleSupportAdmin, auth.RoleAnalyticsAdmin:
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

func isValidSubscriptionPlan(plan string) bool {
	switch plan {
	case "free", "premium":
		return true
	}
	return false
}

func isValidSubscriptionStatus(status string) bool {
	switch status {
	case "active", "cancelled", "expired":
		return true
	}
	return false
}

// ---------- Content ----------

func (h *Handler) listCollections(w http.ResponseWriter, r *http.Request) {
	cols, err := h.cont.ListCollections(r.Context(), false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load collections")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cols)
}

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.cont.ListCategories(r.Context(), false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load categories")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cats)
}

func (h *Handler) categoryConfessions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	confs, err := h.cont.ConfessionsByCategory(r.Context(), id, true)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confessions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, confs)
}

func (h *Handler) getConfession(w http.ResponseWriter, r *http.Request) {
	c, err := h.cont.ConfessionByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "confession not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confession")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (h *Handler) listVoices(w http.ResponseWriter, r *http.Request) {
	voices, err := h.audio.ListVoices(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load voices")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, voices)
}

// ---------- Sessions ----------

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CategoryIDs     []string `json:"category_ids"`
		DurationSeconds int      `json:"duration_seconds"`
		VoiceID         string   `json:"voice_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.CategoryIDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "at least one category is required")
		return
	}
	if req.DurationSeconds < 60 || req.DurationSeconds > 3*3600 {
		httpx.WriteError(w, http.StatusBadRequest, "duration must be between 1 minute and 3 hours")
		return
	}
	sess, err := h.engn.Build(r.Context(), engine.Request{
		UserID:          h.userID(r),
		CategoryIDs:     req.CategoryIDs,
		DurationSeconds: req.DurationSeconds,
		VoiceID:         req.VoiceID,
	})
	if errors.Is(err, engine.ErrNoContent) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, "no content available for the selected categories and voice")
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
	httpx.WriteJSON(w, http.StatusOK, sess)
}

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
	if err := httpx.DecodeJSON(r, &req); err != nil || !validSessionStatus(req.Status) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid status")
		return
	}
	if err := h.sess.UpdateStatus(r.Context(), id, req.Status); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update session")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": req.Status})
}

func (h *Handler) listMySessions(w http.ResponseWriter, r *http.Request) {
	sess, err := h.sess.ListByUser(r.Context(), h.userID(r), 50)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load sessions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sess)
}

func validSessionStatus(s string) bool {
	switch s {
	case "created", "playing", "completed", "abandoned":
		return true
	}
	return false
}

// ---------- Schedules ----------

func (h *Handler) listSchedules(w http.ResponseWriter, r *http.Request) {
	list, err := h.sched.ListByUser(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load schedules")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) createSchedule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label           string   `json:"label"`
		Time            string   `json:"time"`
		DaysOfWeek      []int    `json:"days_of_week"`
		Timezone        string   `json:"timezone"`
		DurationSeconds int      `json:"duration_seconds"`
		VoiceID         string   `json:"voice_id"`
		CategoryIDs     []string `json:"category_ids"`
		Enabled         *bool    `json:"enabled"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Label == "" || req.Time == "" {
		httpx.WriteError(w, http.StatusBadRequest, "label and time are required")
		return
	}
	if _, err := time.Parse("15:04", req.Time); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "time must be HH:MM")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.DurationSeconds == 0 {
		req.DurationSeconds = 1800
	}
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	sc := &models.Schedule{
		UserID:          h.userID(r),
		Label:           req.Label,
		Time:            req.Time,
		DaysOfWeek:      req.DaysOfWeek,
		Timezone:        req.Timezone,
		DurationSeconds: req.DurationSeconds,
		VoiceID:         req.VoiceID,
		CategoryIDs:     req.CategoryIDs,
		Enabled:         enabled,
	}
	if err := h.sched.Create(r.Context(), sc); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create schedule")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, sc)
}

func (h *Handler) updateSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sc, err := h.sched.ByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "schedule not found")
		return
	}
	if sc.UserID != h.userID(r) {
		httpx.WriteError(w, http.StatusForbidden, "not your schedule")
		return
	}
	var req struct {
		Label           *string  `json:"label"`
		Time            *string  `json:"time"`
		DaysOfWeek      []int    `json:"days_of_week"`
		Timezone        *string  `json:"timezone"`
		DurationSeconds *int     `json:"duration_seconds"`
		VoiceID         *string  `json:"voice_id"`
		CategoryIDs     []string `json:"category_ids"`
		Enabled         *bool    `json:"enabled"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Label != nil {
		sc.Label = *req.Label
	}
	if req.Time != nil {
		if _, err := time.Parse("15:04", *req.Time); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "time must be HH:MM")
			return
		}
		sc.Time = *req.Time
	}
	if req.DaysOfWeek != nil {
		sc.DaysOfWeek = req.DaysOfWeek
	}
	if req.Timezone != nil {
		sc.Timezone = *req.Timezone
	}
	if req.DurationSeconds != nil {
		sc.DurationSeconds = *req.DurationSeconds
	}
	if req.VoiceID != nil {
		sc.VoiceID = *req.VoiceID
	}
	if req.CategoryIDs != nil {
		sc.CategoryIDs = req.CategoryIDs
	}
	if req.Enabled != nil {
		sc.Enabled = *req.Enabled
	}
	if err := h.sched.Update(r.Context(), sc); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update schedule")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sc)
}

func (h *Handler) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	err := h.sched.Delete(r.Context(), r.PathValue("id"), h.userID(r))
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "schedule not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to delete schedule")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- Engagement ----------

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

func (h *Handler) listFavorites(w http.ResponseWriter, r *http.Request) {
	list, err := h.eng.ListFavorites(r.Context(), h.userID(r), r.URL.Query().Get("type"))
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

func (h *Handler) createUserConfession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title      string `json:"title"`
		Text       string `json:"text"`
		CategoryID string `json:"category_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" || req.Text == "" {
		httpx.WriteError(w, http.StatusBadRequest, "title and text are required")
		return
	}
	uc := &models.UserConfession{
		UserID:     h.userID(r),
		Title:      req.Title,
		Text:       req.Text,
		CategoryID: req.CategoryID,
		IsPrivate:  true, // private by default (PRD Â§22)
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
