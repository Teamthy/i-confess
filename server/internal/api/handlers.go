package api

import (
	"database/sql"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/deletion"
	"github.com/Teamthy/i-confess/internal/email"
	"github.com/Teamthy/i-confess/internal/engine"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/oauth"
	"github.com/Teamthy/i-confess/internal/ratelimit"
	"github.com/Teamthy/i-confess/internal/scheduler"
	"github.com/Teamthy/i-confess/internal/sessions"
	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/voice"
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
	// signer mints short-lived audio URLs. Audio bytes never pass through this
	// API (PRD S11); the client is handed a signed CDN link instead.
	signer storage.ObjectStorage
	// mediaHandler is the development-only signed audio origin.
	mediaHandler http.Handler
	// pipeline renders text to audio. Nil when synthesis is unconfigured, in
	// which case generation endpoints report 503 rather than failing obscurely.
	pipeline *voice.Pipeline
	// vrights stores voice authorization metadata.
	vrights *store.VoiceRightsStore
	db      *sql.DB
	// devTokenSink receives one-time tokens in development and tests. Nil in
	// production, where tokens go only to the email queue.
	devTokenSink func(purpose, email, token string)
	// limiter throttles authentication abuse (S20, S21).
	limiter ratelimit.Enforcer
	// profiles owns profile, preferences and interests (S5, S14, S20).
	profiles *store.ProfileStore
	// mail delivers transactional authentication email asynchronously, so a
	// slow provider cannot fail a registration (S57, S58).
	mail    *email.Queue
	mailCfg email.Config
	// verifiers validate third-party identity tokens (S36, S37).
	verifiers map[string]oauth.Verifier
	// library owns collections, devices and notification preferences.
	library *store.LibraryStore
	// deletion performs account erasure under an explicit retention policy.
	deletion *deletion.Service
	// dispatcher delivers scheduled-session reminders (S19, S47).
	dispatcher *scheduler.Dispatcher
	// downloads manages offline licences (S28).
	downloads *store.DownloadStore
	// metrics counts security-relevant events for alerting (S83, S84).
	metrics *AuthMetrics
	// routes records every registered endpoint, so the API spec is generated
	// from the same calls that serve traffic and cannot drift.
	routes *routeRecorder
}

// SetLimiter installs a rate limiter. Production passes a Redis-backed
// enforcer so limits hold across replicas; without it each instance would allow
// the full burst independently.
func (h *Handler) SetLimiter(e ratelimit.Enforcer) {
	if e != nil {
		h.limiter = e
	}
}

// SetMailer installs the transactional email queue and templates.
func (h *Handler) SetMailer(q *email.Queue, cfg email.Config) {
	h.mail, h.mailCfg = q, cfg
}

// SetDevTokenSink installs a development/test hook for one-time tokens.
func (h *Handler) SetDevTokenSink(f func(purpose, email, token string)) { h.devTokenSink = f }

type Config struct {
	JWTSecret string
	TokenTTL  string
}

func NewHandler(cfg Config, db *sql.DB) *Handler {
	return &Handler{
		cfg:       cfg,
		users:     store.NewUserStore(db),
		cont:      store.NewContentStore(db),
		audio:     store.NewAudioStore(db),
		sess:      store.NewSessionStore(db),
		sched:     store.NewScheduleStore(db),
		eng:       store.NewEngagementStore(db),
		queue:     jobs.NewMemoryQueue(),
		vrights:   store.NewVoiceRightsStore(db),
		db:        db,
		limiter:   ratelimit.New(),
		profiles:  store.NewProfileStore(db),
		verifiers: map[string]oauth.Verifier{},
		library:   store.NewLibraryStore(db),
		deletion:  deletion.NewService(db),
		downloads: store.NewDownloadStore(db),
		metrics:   NewAuthMetrics(),
		routes:    &routeRecorder{},
	}
}

// auditRights records a change to a voice licence. Failures are logged and
// swallowed: the rights change itself already succeeded, and losing an audit
// line must not roll it back.
func (h *Handler) auditRights(r *http.Request, voiceID string, aiGranted bool, attestation string) {
	action := "voice_rights_updated"
	if aiGranted {
		action = "voice_rights_ai_generation_granted"
	}
	if attestation == "" {
		attestation = "no attestation supplied"
	}
	// Persisted, not merely logged: a rights dispute needs a queryable record,
	// and log retention is measured in days while a licence decision matters
	// for years (S51).
	h.recordAudit(r, action, "voice", voiceID, attestation, "success")
}

// usersDB exposes the underlying database handle for tests that need to
// manipulate subscription state directly.
func (h *Handler) usersDB() *sql.DB { return h.db }

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
	// An existing address must not be confirmed to the caller (S65). Instead of
	// 409 (which turns registration into a membership oracle), the response is
	// indistinguishable from a fresh signup and the real owner is emailed that
	// someone tried to register with their address.
	if existing, _, err := h.users.ByEmail(r.Context(), req.Email); err == nil && existing != nil {
		h.notifySecurityEvent(existing.Email,
			"Someone tried to create an account with your email",
			"If this was you, you already have an account and can sign in or reset your password.")
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"message": "Check your email to continue setting up your account.",
			"pending": true,
		})
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

	// Issue and send the verification link. Failure to create the token must
	// not fail registration: the account exists and "resend" can recover it.
	if plaintext, hashed, terr := auth.NewOneTimeToken(); terr == nil {
		if err := h.users.CreateVerificationToken(r.Context(), u.ID, "email", hashed, 60); err == nil {
			h.deliverToken("email_verification", u.Email, plaintext)
		} else {
			log.Printf("auth: failed to store verification token for %s: %v", u.ID, err)
		}
	}

	h.issueToken(w, r, u)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		// Code carries the second factor when the client already has it, so a
		// user with MFA can sign in with one round trip.
		Code string `json:"code"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	// Throttle per account as well as per IP. Keying on the account stops a
	// distributed attack spreading guesses across many addresses; keying on the
	// address stops one host grinding through many accounts. Neither ever locks
	// the account, which would let anyone lock anyone else out (S20, S21).
	acctKey := "login:acct:" + req.Email
	if ok, retry := h.limiter.Allow(acctKey, ratelimit.LoginPerAccount); !ok {
		ratelimit.TooManyRequests(w, retry)
		return
	}

	u, hash, err := h.users.ByEmail(r.Context(), req.Email)
	if err != nil || !auth.CheckPassword(hash, req.Password) {
		h.metrics.Inc(MetricLoginFailure)
		// One generic message for both "no such account" and "wrong password",
		// so the endpoint cannot be used to discover who has an account (S19).
		httpx.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	// pending_deletion must still be able to sign in, otherwise the grace
	// period is meaningless: the owner of an account someone else scheduled for
	// deletion could never get back in to cancel it.
	if u.Status != "active" && u.Status != "pending_deletion" {
		// Deliberately vague: moderation state is not the caller's business (S80).
		httpx.WriteError(w, http.StatusForbidden, "this account is not available")
		return
	}

	// A successful sign-in clears the counter, so a user who finally remembers
	// their password is not left throttled.
	h.limiter.Reset(acctKey)
	h.metrics.Inc(MetricLoginSuccess)

	// Second factor, if enrolled. The password alone must not yield a session
	// (S41): everything below this point requires the factor to be satisfied.
	if enrolment, mErr := h.users.MFAEnrolmentFor(r.Context(), u.ID); mErr == nil && enrolment.Enabled {
		if req.Code == "" {
			// Signals the client to prompt. Deliberately not an error: the
			// credentials were correct, the login is simply incomplete.
			h.metrics.Inc(MetricMFAChallenge)
			httpx.WriteJSON(w, http.StatusOK, map[string]any{
				"mfa_required": true,
				"message":      "Enter the code from your authenticator app.",
			})
			return
		}
		if ok, retry := h.limiter.Allow("mfa:login:"+u.ID, mfaAttemptRule); !ok {
			ratelimit.TooManyRequests(w, retry)
			return
		}
		usedRecovery, ok := h.verifySecondFactor(r, u.ID, enrolment, req.Code)
		if !ok {
			h.metrics.Inc(MetricMFAFailure)
			httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "that code is not valid", "code": "MFA_CODE_INVALID",
			})
			return
		}
		h.limiter.Reset("mfa:login:" + u.ID)
		if usedRecovery {
			// Using a recovery code means the authenticator is probably gone,
			// which is worth telling the owner about.
			h.notifySecurityEvent(u.Email, "A recovery code was used to sign in",
				"If this was not you, change your password and review your devices.")
		}
	}

	h.issueToken(w, r, u)
}

// issueToken opens a server-side session and returns an access token bound to
// it. The binding is what makes logout, suspension and password changes able to
// invalidate a token that has already been handed out (PRD S22, S28, S54).
func (h *Handler) issueToken(w http.ResponseWriter, r *http.Request, u *models.User) {
	role, _ := h.users.AdminRole(r.Context(), u.ID)

	ttl, err := time.ParseDuration(h.cfg.TokenTTL)
	if err != nil {
		ttl = 720 * time.Hour
	}
	sessionID, err := h.users.CreateAuthSession(r.Context(), u.ID,
		r.Header.Get("X-Platform"), r.UserAgent(), clientIP(r), ttl)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	tok, err := auth.SignSessionToken(h.cfg.JWTSecret, h.cfg.TokenTTL, u.ID, u.Email, role, sessionID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"token": tok,
		"user":  u,
	})
}

// clientIP extracts the caller address for session metadata. It reads
// X-Forwarded-For only because the service runs behind a trusted proxy; the
// value is descriptive metadata for the security screen and is never used for
// an authorization decision.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
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

	// The usual reason to change a password is that someone else may have it,
	// so every other session is ended. The caller keeps theirs, to avoid the
	// hostile UX of being signed out of the device they just used (§34, §35).
	sessionID := ""
	if c := auth.FromContext(r); c != nil {
		sessionID = c.SessionID
	}
	if err := h.users.RevokeSessionsExcept(r.Context(), userID, sessionID); err != nil {
		log.Printf("auth: failed to revoke sessions after password change for %s: %v", userID, err)
	}

	h.notifySecurityEvent(u.Email, "Your password was changed",
		"If you made this change, no action is needed. You have been signed out on all other devices.")

	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "password updated successfully",
		"note":    "You have been signed out on all other devices.",
	})
}

// refreshToken rotates the session and issues a new access token (PRD S23).
//
// Rotation, not renewal: the presented session is retired and replaced. That is
// what makes theft detectable — if the retired session is ever presented again,
// someone kept a copy, and ValidateSession revokes the whole family.
func (h *Handler) refreshToken(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r)
	if claims == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Role is re-read rather than copied from the old token: a demoted admin
	// must not be able to refresh their way into keeping privileges (S56).
	role, _ := h.users.AdminRole(r.Context(), claims.Sub)

	ttl, err := time.ParseDuration(h.cfg.TokenTTL)
	if err != nil {
		ttl = 720 * time.Hour
	}

	sessionID := claims.SessionID
	if sessionID != "" {
		rotated, rerr := h.users.RotateSession(r.Context(), claims.Sub, sessionID,
			r.Header.Get("X-Platform"), r.UserAgent(), clientIP(r), ttl)
		if rerr != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "failed to rotate session")
			return
		}
		sessionID = rotated
	}

	tok, err := auth.SignSessionToken(h.cfg.JWTSecret, h.cfg.TokenTTL,
		claims.Sub, claims.Email, role, sessionID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"token": tok})
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Token == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid verification token")
		return
	}
	// Look up by hash: only hashes are stored.
	hashed := auth.HashToken(req.Token)
	userID, err := h.users.UserIDByVerificationToken(r.Context(), hashed)
	if err != nil || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid or expired verification token")
		return
	}
	if ok, err := h.users.VerifyEmailToken(r.Context(), userID, hashed); err != nil || !ok {
		httpx.WriteError(w, http.StatusBadRequest, "verification failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "email verified"})
}

func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Email == "" {
		httpx.WriteError(w, http.StatusBadRequest, "email is required")
		return
	}

	// Always answer identically, whether or not the address is known. Any
	// difference in status, body or wording is an account-enumeration oracle
	// (S17, S65).
	const neutral = "If an account exists for that email, a verification link has been sent."

	user, _, err := h.users.ByEmail(r.Context(), strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil || user == nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": neutral})
		return
	}

	plaintext, hash, err := auth.NewOneTimeToken()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create verification token")
		return
	}
	if err := h.users.CreateVerificationToken(r.Context(), user.ID, "email", hash, 60); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create verification token")
		return
	}

	// The link is delivered by email. In development the token is logged so the
	// flow is testable; it must never appear in the HTTP response, which would
	// hand it to anyone who can guess an address.
	h.deliverToken("email_verification", user.Email, plaintext)

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": neutral})
}

func (h *Handler) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Email == "" {
		httpx.WriteError(w, http.StatusBadRequest, "email is required")
		return
	}

	// Identical response either way (S33, S65). Previously an unknown address
	// returned 404, which confirmed which emails were registered.
	const neutral = "If an account exists for that email, a reset link has been sent."

	user, _, err := h.users.ByEmail(r.Context(), strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil || user == nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": neutral})
		return
	}

	// Cryptographically random, stored only as a hash. The previous token was
	// "reset-" + user id: guessable by anyone who learned an id, which made
	// password reset an account-takeover primitive (S34).
	plaintext, hash, err := auth.NewOneTimeToken()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create reset token")
		return
	}
	if err := h.users.CreatePasswordReset(r.Context(), user.ID, hash, 30); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create reset token")
		return
	}

	h.deliverToken("password_reset", user.Email, plaintext)

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": neutral})
}

// deliverToken hands a one-time token to the delivery channel.
//
// Delivery is asynchronous (S58): the queue accepts the message and returns
// immediately, so SMTP latency never becomes authentication latency. The token
// is never written to a production log and never returned in a response.
func (h *Handler) deliverToken(purpose, email_, token string) {
	// Test hook, when set, takes precedence so suites can assert on tokens
	// without a provider.
	if h.devTokenSink != nil {
		h.devTokenSink(purpose, email_, token)
		return
	}
	if h.mail == nil {
		log.Printf("auth: %s token issued for %s (email delivery not configured)",
			purpose, maskEmail(email_))
		return
	}
	switch purpose {
	case "email_verification":
		h.mail.Enqueue(h.mailCfg.VerificationMessage(email_, token))
	case "password_reset":
		h.mail.Enqueue(h.mailCfg.PasswordResetMessage(email_, token))
	default:
		log.Printf("auth: unknown token purpose %q, not sending", purpose)
	}
}

// notifySecurityEvent tells a user about a meaningful account change (S52).
//
// Failures are ignored on purpose: the action already succeeded, and a mail
// outage must not roll back a password change.
func (h *Handler) notifySecurityEvent(addr, event, detail string) {
	if h.mail == nil || addr == "" {
		return
	}
	h.mail.Enqueue(h.mailCfg.SecurityAlertMessage(addr, event, detail))
}

// maskEmail redacts an address for logs (S70).
func maskEmail(email string) string {
	at := strings.IndexByte(email, '@')
	if at <= 1 {
		return "***"
	}
	return email[:1] + "***" + email[at:]
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
	// Only hashes are stored, so hash the presented token before lookup.
	hashed := auth.HashToken(req.Token)
	userID, err := h.users.UserIDByPasswordResetToken(r.Context(), hashed)
	if err != nil || userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid or expired reset token")
		return
	}
	newHash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := h.users.ResetPassword(r.Context(), userID, hashed, newHash); err != nil {
		// Do not surface the store error: it can distinguish "already used"
		// from "expired", which leaks token state.
		httpx.WriteError(w, http.StatusBadRequest, "invalid or expired reset token")
		return
	}

	// A reset is a recovery action: whoever held the old password may be an
	// attacker, so every existing session is ended (S34).
	if err := h.users.RevokeAllSessions(r.Context(), userID); err != nil {
		log.Printf("auth: failed to revoke sessions after password reset for %s: %v", userID, err)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "password reset successfully",
		"note":    "You have been signed out on all devices.",
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r)
	if claims == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	// Logout ends this session only; other devices stay signed in. Ending all
	// of them is a separate, explicit action (S28 vs S29).
	if claims.SessionID != "" {
		if err := h.users.RevokeAuthSession(r.Context(), claims.Sub, claims.SessionID); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "failed to revoke session")
			return
		}
	} else if err := h.users.RevokeAllSessions(r.Context(), claims.Sub); err != nil {
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
		DurationPreset  string   `json:"duration_preset"`
		Strategy        string   `json:"strategy"`
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
			httpx.WriteError(w, http.StatusBadRequest, "unknown duration_preset")
			return
		}
		req.DurationSeconds = secs
	}
	if req.DurationSeconds < 60 || req.DurationSeconds > 3*3600 {
		httpx.WriteError(w, http.StatusBadRequest, "duration must be between 1 minute and 3 hours")
		return
	}
	if req.Strategy != "" && !engine.IsValidStrategy(req.Strategy) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid strategy")
		return
	}

	// Session length is a plan capability (PRD S25/S26). Enforced server-side:
	// the client is never the authority on what it may request.
	ent := h.entitlementsFor(r.Context(), h.userID(r))
	if max := ent.MaxSessionSeconds(); req.DurationSeconds > max {
		httpx.WriteJSON(w, http.StatusPaymentRequired, map[string]any{
			"error":       "session length exceeds your plan limit",
			"reason":      "session_duration_exceeds_plan_limit",
			"max_seconds": max,
			"plan":        ent.Plan,
		})
		return
	}

	sess, err := h.engn.Build(r.Context(), engine.Request{
		UserID:          h.userID(r),
		CategoryIDs:     req.CategoryIDs,
		DurationSeconds: req.DurationSeconds,
		Strategy:        engine.NormalizeStrategy(req.Strategy),
		VoiceID:         req.VoiceID,
	})
	if errors.Is(err, engine.ErrNoContent) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, "no content available for the selected categories and voice")
		return
	}
	if errors.Is(err, engine.ErrNoExactFit) {
		// Distinct from "no content": the library has material for these
		// categories, it just cannot land exactly on the requested length
		// without cutting a confession short, which we do not do.
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":  "no combination of complete confessions matches that exact length",
			"reason": "exact_duration_unavailable",
			"hint":   "choose a nearby length, or use the balanced strategy",
		})
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
		httpx.WriteError(w, http.StatusConflict, sessions.Reason(from, to))
		return
	}
	if err := h.sess.UpdateStatus(r.Context(), id, string(to)); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update session")
		return
	}
	// Return the canonical state so clients converge on one vocabulary.
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": string(to)})
}

func (h *Handler) listMySessions(w http.ResponseWriter, r *http.Request) {
	sess, err := h.sess.ListByUser(r.Context(), h.userID(r), 50)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load sessions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sess)
}

// validSessionStatus reports whether s names a session state the API accepts,
// including the legacy spellings still sent by older clients.
func validSessionStatus(s string) bool {
	_, ok := sessions.Parse(s)
	return ok
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

// ---------------------------------------------------------------------------
// Session management (PRD S30, S31, S54)
// ---------------------------------------------------------------------------

// listSessions returns the caller's live sign-ins for the security screen.
//
// It deliberately reports no geolocation: an IP-derived city is frequently
// wrong and turns a security feature into a source of false alarms. Device and
// last-used time are enough to recognise a session (S31).
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
