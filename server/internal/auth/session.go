package auth

import (
	"context"
	"net/http"

	"github.com/Teamthy/i-confess/internal/httpx"
)

// Server-side session validation (§3, §22, §28, §29, §54, §56, §80).
//
// A signed JWT proves only that we issued it at some point. It does not prove
// the session is still honoured, that the account is still active, or that the
// user has not logged out. Treating signature validity as authority is what
// makes "log out all devices" a lie and leaves a stolen token working until it
// expires.
//
// Every authenticated request therefore consults a SessionValidator. The cost
// is one indexed lookup; the alternative is tokens that cannot be revoked.

// SessionState is the outcome of validating a token's session.
type SessionState struct {
	Valid bool
	// Reason is a stable machine-readable code the client maps to a message.
	Reason string
	// Role is the authoritative admin role, read at request time rather than
	// trusted from the token: a demoted admin must lose access immediately (§56).
	Role string
}

// Stable reason codes (§49).
const (
	ReasonSessionRevoked   = "AUTH_SESSION_REVOKED"
	ReasonSessionExpired   = "AUTH_SESSION_EXPIRED"
	ReasonAccountSuspended = "AUTH_ACCOUNT_SUSPENDED"
	ReasonAccountDeleted   = "AUTH_ACCOUNT_DELETED"
	ReasonTokenInvalid     = "AUTH_TOKEN_INVALID"
	// ReasonTokenReused signals that a rotated session was presented again,
	// which indicates the token was copied (PRD S49).
	ReasonTokenReused = "AUTH_TOKEN_REUSED"
)

// SessionValidator answers whether a token's session is still honoured.
//
// Implementations must be cheap: this runs on every authenticated request.
type SessionValidator interface {
	// ValidateSession checks the session id and account status for a user.
	// An empty sessionID means the token predates session binding.
	ValidateSession(ctx context.Context, userID, sessionID string) SessionState
}

// MiddlewareWithSessions authenticates a request and confirms the session is
// still live. This should be preferred over Middleware everywhere.
func MiddlewareWithSessions(secret string, v SessionValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := fromRequest(secret, r)
			if err != nil {
				unauthorized(w, ReasonTokenInvalid, "unauthorized")
				return
			}
			if v != nil {
				state := v.ValidateSession(r.Context(), c.Sub, c.SessionID)
				if !state.Valid {
					unauthorized(w, state.Reason, reasonMessage(state.Reason))
					return
				}
				// The database is authoritative for role, not the token.
				c.Role = state.Role
			}
			next.ServeHTTP(w, withClaims(r, c))
		})
	}
}

// RequireRoleWithSessions restricts access to named roles and validates the
// session, so a revoked or demoted admin loses access immediately.
//
// SUPER_ADMIN is admitted everywhere; every other role must be listed.
func RequireRoleWithSessions(secret string, v SessionValidator, allowed ...string) func(http.Handler) http.Handler {
	permitted := make(map[string]bool, len(allowed)+1)
	permitted[RoleSuperAdmin] = true
	for _, role := range allowed {
		permitted[role] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := fromRequest(secret, r)
			if err != nil {
				forbidden(w, "admin access required")
				return
			}
			role := c.Role
			if v != nil {
				state := v.ValidateSession(r.Context(), c.Sub, c.SessionID)
				if !state.Valid {
					unauthorized(w, state.Reason, reasonMessage(state.Reason))
					return
				}
				role = state.Role
			}
			if role == "" || !permitted[role] {
				// Deliberately does not reveal which roles would suffice.
				forbidden(w, "your role does not permit this action")
				return
			}
			c.Role = role
			next.ServeHTTP(w, withClaims(r, c))
		})
	}
}

func unauthorized(w http.ResponseWriter, code, msg string) {
	if code == "" {
		code = ReasonTokenInvalid
	}
	httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{
		"error": msg, "code": code,
	})
}

func forbidden(w http.ResponseWriter, msg string) {
	httpx.WriteJSON(w, http.StatusForbidden, map[string]string{
		"error": msg, "code": "AUTH_INSUFFICIENT_PERMISSION",
	})
}

// reasonMessage maps a code to a message safe to show a user. It never
// discloses moderation detail (§80).
func reasonMessage(code string) string {
	switch code {
	case ReasonSessionRevoked:
		return "your session has ended, please sign in again"
	case ReasonSessionExpired:
		return "your session has expired, please sign in again"
	case ReasonTokenReused:
		return "you were signed out for security, please sign in again"
	case ReasonAccountSuspended:
		return "this account is not available"
	case ReasonAccountDeleted:
		return "this account is not available"
	default:
		return "unauthorized"
	}
}
