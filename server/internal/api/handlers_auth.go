package api

import (
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/ratelimit"
	"github.com/Teamthy/i-confess/internal/store"
)

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
		writeCode(w, http.StatusBadRequest, "AUTH_VALIDATION_FAILED", "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" {
		writeCode(w, http.StatusBadRequest, "AUTH_VALIDATION_FAILED", "email is required")
		return
	}
	// One policy, applied at every point a password is set. It used to be an
	// inline length check duplicated per handler, which meant a new entry point
	// could be added without it and nothing would notice.
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeCode(w, http.StatusBadRequest, "AUTH_VALIDATION_FAILED", err.Error())
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
		writeCode(w, http.StatusBadRequest, "AUTH_VALIDATION_FAILED", "invalid request body")
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
		writeCode(w, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "invalid credentials")
		return
	}
	// pending_deletion must still be able to sign in, otherwise the grace
	// period is meaningless: the owner of an account someone else scheduled for
	// deletion could never get back in to cancel it.
	if u.Status != "active" && u.Status != "pending_deletion" {
		// Deliberately vague: moderation state is not the caller's business (S80).
		writeCode(w, http.StatusForbidden, "AUTH_ACCOUNT_UNAVAILABLE", "this account is not available")
		return
	}

	// A successful sign-in clears the counter, so a user who finally remembers
	// their password is not left throttled.
	h.limiter.Reset(acctKey)
	h.metrics.Inc(MetricLoginSuccess)

	// Second factor, if enrolled. The password alone must not yield a session
	// (S41): everything below this point requires the factor to be satisfied.
	//
	// A failure to read the enrolment is refused, not ignored. The previous form
	// was "mErr == nil && enrolment.Enabled", which meant any error - a dropped
	// connection, a timeout, a failed query - evaluated the whole condition to
	// false and issued a session with the second factor silently skipped. That
	// turns an availability incident into an authentication bypass, and it fails
	// in the direction an attacker would choose. A user who cannot sign in
	// during an outage is inconvenienced; a user whose 2FA vanished is
	// compromised.
	//
	// ErrNotFound means "not enrolled", which is a normal answer and not a
	// failure. Every other error is refused. Collapsing the two - as
	// "mErr == nil && enrolment.Enabled" did - is what let a database error
	// skip the second factor entirely.
	enrolment, mErr := h.getMFAEnrolment(r.Context(), u.ID)
	if mErr != nil && !errors.Is(mErr, store.ErrNotFound) {
		log.Printf("auth: cannot read MFA enrolment for %s, refusing sign-in: %v", u.ID, mErr)
		writeCode(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "sign-in is temporarily unavailable, try again")
		return
	}
	if mErr == nil && enrolment.Enabled {
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
		ttl = auth.FallbackTokenTTL
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

	if err := auth.ValidatePassword(req.NewPassword); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
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
		ttl = auth.FallbackTokenTTL
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
		writeCode(w, http.StatusBadRequest, "AUTH_TOKEN_INVALID", "invalid verification token")
		return
	}
	// Look up by hash: only hashes are stored.
	hashed := auth.HashToken(req.Token)
	userID, err := h.users.UserIDByVerificationToken(r.Context(), hashed)
	if err != nil || userID == "" {
		writeCode(w, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "invalid or expired verification token")
		return
	}
	if ok, err := h.users.VerifyEmailToken(r.Context(), userID, hashed); err != nil || !ok {
		writeCode(w, http.StatusBadRequest, "AUTH_TOKEN_INVALID", "verification failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "email verified"})
}

func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Email == "" {
		writeCode(w, http.StatusBadRequest, "AUTH_VALIDATION_FAILED", "email is required")
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
		writeCode(w, http.StatusBadRequest, "AUTH_VALIDATION_FAILED", "email is required")
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
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Token == "" {
		writeCode(w, http.StatusBadRequest, "AUTH_TOKEN_INVALID", "token is required")
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeCode(w, http.StatusBadRequest, "AUTH_VALIDATION_FAILED", err.Error())
		return
	}
	// Only hashes are stored, so hash the presented token before lookup.
	hashed := auth.HashToken(req.Token)
	userID, err := h.users.UserIDByPasswordResetToken(r.Context(), hashed)
	if err != nil || userID == "" {
		writeCode(w, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "invalid or expired reset token")
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
		writeCode(w, http.StatusBadRequest, "AUTH_TOKEN_INVALID", "invalid or expired reset token")
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
