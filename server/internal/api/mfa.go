package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/mfa"
	"github.com/Teamthy/i-confess/internal/ratelimit"
	"github.com/Teamthy/i-confess/internal/store"
)

// Multi-factor authentication (§41, §42, §81).
//
// Enrolment is two-step on purpose: the server issues a secret, and the factor
// only becomes active once the user proves they can generate a code from it.
// A one-step "enable" would let someone lock themselves out with a mis-scanned
// QR code, which turns a security feature into a support burden.

// beginMFAEnrolment issues a new secret and its provisioning URI.
//
// The secret is generated server-side. The previous implementation accepted a
// client-supplied secret, which meant an attacker could enrol a factor they
// controlled onto someone else's account.
func (h *Handler) beginMFAEnrolment(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)

	// Re-enrolling while enabled would silently invalidate the working factor
	// if the new one is never confirmed.
	if e, err := h.users.MFAEnrolmentFor(r.Context(), userID); err == nil && e.Enabled {
		writeCode(w, http.StatusConflict, "MFA_ALREADY_ENABLED",
			"two-factor authentication is already on; turn it off first to re-enrol")
		return
	}

	secret, err := mfa.GenerateSecret()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to start enrolment")
		return
	}
	if err := h.users.BeginMFAEnrolment(r.Context(), userID, secret); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to start enrolment")
		return
	}

	email := ""
	if c := auth.FromContext(r); c != nil {
		email = c.Email
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"secret":            secret,
		"provisioning_uri":  mfa.ProvisioningURI(secret, email, h.mailCfg.AppName),
		"digits":            mfa.Digits,
		"period_seconds":    int(mfa.Period.Seconds()),
		"next":              "Scan the QR code, then confirm with a code to finish turning this on.",
		"recovery_codes_at": "confirm",
	})
}

// confirmMFAEnrolment activates the factor and returns recovery codes.
func (h *Handler) confirmMFAEnrolment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Code == "" {
		writeCode(w, http.StatusBadRequest, "MFA_CODE_REQUIRED", "a verification code is required")
		return
	}
	userID := h.userID(r)

	enrolment, err := h.users.MFAEnrolmentFor(r.Context(), userID)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusConflict, "MFA_NOT_STARTED", "start enrolment first")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load enrolment")
		return
	}
	if enrolment.Enabled {
		writeCode(w, http.StatusConflict, "MFA_ALREADY_ENABLED", "two-factor authentication is already on")
		return
	}

	// Throttle: without a limit, six digits is 10^6 and a script would walk it.
	if ok, retry := h.limiter.Allow("mfa:confirm:"+userID, mfaAttemptRule); !ok {
		writeRetry(w, retry)
		return
	}

	counter, ok := mfa.Verify(enrolment.Secret, req.Code, time.Now())
	if !ok {
		writeCode(w, http.StatusUnauthorized, "MFA_CODE_INVALID", "that code is not valid")
		return
	}

	plaintext, hashes, err := mfa.GenerateRecoveryCodes()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to generate recovery codes")
		return
	}
	if err := h.users.ConfirmMFA(r.Context(), userID, hashes, counter); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to enable two-factor authentication")
		return
	}
	h.limiter.Reset("mfa:confirm:" + userID)

	if c := auth.FromContext(r); c != nil {
		h.notifySecurityEvent(c.Email, "Two-factor authentication was turned on",
			"If you did not do this, change your password immediately.")
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"enabled": true,
		// Shown exactly once. They are stored only as hashes, so the platform
		// genuinely cannot show them again.
		"recovery_codes": plaintext,
		"warning": "Save these recovery codes now. They are shown once and cannot be retrieved later. " +
			"Each can be used once if you lose your authenticator.",
	})
}

// disableMFA turns the factor off.
//
// Requires the password and a current code (or a recovery code): removing a
// second factor is exactly what an attacker with a stolen session wants to do
// first.
func (h *Handler) disableMFA(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "invalid request body")
		return
	}
	userID := h.userID(r)

	u, err := h.users.ByID(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load account")
		return
	}
	if hasPassword, _ := h.users.HasPassword(r.Context(), userID); hasPassword {
		_, hash, err := h.users.ByEmail(r.Context(), u.Email)
		if err != nil || !auth.CheckPassword(hash, req.Password) {
			writeCode(w, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "your password is required")
			return
		}
	}

	enrolment, err := h.users.MFAEnrolmentFor(r.Context(), userID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !enrolment.Enabled) {
		writeCode(w, http.StatusConflict, "MFA_NOT_ENABLED", "two-factor authentication is not on")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load enrolment")
		return
	}

	if ok, retry := h.limiter.Allow("mfa:disable:"+userID, mfaAttemptRule); !ok {
		writeRetry(w, retry)
		return
	}
	if _, ok := h.verifySecondFactor(r, userID, enrolment, req.Code); !ok {
		writeCode(w, http.StatusUnauthorized, "MFA_CODE_INVALID", "a valid code is required")
		return
	}

	if err := h.users.DisableMFA(r.Context(), userID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to disable two-factor authentication")
		return
	}
	h.notifySecurityEvent(u.Email, "Two-factor authentication was turned off",
		"If you did not do this, your account may be compromised. Change your password immediately.")

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "two-factor authentication disabled"})
}

// mfaStatus reports whether the factor is on and how many recovery codes remain.
func (h *Handler) mfaStatus(w http.ResponseWriter, r *http.Request) {
	e, err := h.users.MFAEnrolmentFor(r.Context(), h.userID(r))
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"enabled": false, "pending": false})
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load status")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"enabled": e.Enabled,
		// A started-but-unconfirmed enrolment is surfaced so the UI can offer
		// to resume rather than silently starting over.
		"pending":                  !e.Enabled && e.Secret != "",
		"recovery_codes_remaining": len(e.RecoveryHashes),
	})
}

// verifySecondFactor accepts either a TOTP code or a recovery code.
//
// Returns whether the factor was satisfied. Recovery codes are consumed on use
// so each works exactly once.
func (h *Handler) verifySecondFactor(r *http.Request, userID string, e *store.MFAEnrolment, code string) (usedRecovery bool, ok bool) {
	if code == "" {
		return false, false
	}

	if counter, valid := mfa.Verify(e.Secret, code, time.Now()); valid {
		// Reject replay within the same window: a code observed over someone's
		// shoulder stays valid for up to 30 seconds otherwise.
		if counter <= e.LastCounter {
			return false, false
		}
		_ = h.users.RecordMFACounter(r.Context(), userID, counter)
		return false, true
	}

	if idx, matched := mfa.MatchRecoveryCode(code, e.RecoveryHashes); matched {
		remaining := make([]string, 0, len(e.RecoveryHashes)-1)
		remaining = append(remaining, e.RecoveryHashes[:idx]...)
		remaining = append(remaining, e.RecoveryHashes[idx+1:]...)
		_ = h.users.ConsumeRecoveryCode(r.Context(), userID, remaining)
		return true, true
	}
	return false, false
}

// mfaAttemptRule throttles second-factor guesses. Six digits is only 10^6, so
// without a limit a script walks the space; eight tries a minute is generous
// for someone retyping from a phone.
var mfaAttemptRule = ratelimit.Rule{Burst: 8, Window: time.Minute}

// writeRetry emits a throttling response.
func writeRetry(w http.ResponseWriter, retry time.Duration) {
	ratelimit.TooManyRequests(w, retry)
}
