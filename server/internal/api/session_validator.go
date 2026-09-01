package api

import (
	"context"
	"log"

	"github.com/Teamthy/i-confess/internal/auth"
)

// sessionValidator answers, on every authenticated request, whether a token's
// session is still honoured and what the account's authoritative role is.
//
// This is the enforcement point for §3 ("the backend is the ultimate
// authority"): the token carries claims, but the database decides.
type sessionValidator struct{ h *Handler }

// ValidateSession implements auth.SessionValidator.
func (v sessionValidator) ValidateSession(ctx context.Context, userID, sessionID string) auth.SessionState {
	// Account status first: a suspended or deleted account must lose access
	// immediately, regardless of how healthy its sessions look (§80).
	state, err := v.h.users.AccountStateFor(ctx, userID)
	if err != nil {
		// Fail closed. An unavailable database must not become an open door.
		log.Printf("auth: account state lookup failed for %s: %v", userID, err)
		return auth.SessionState{Reason: auth.ReasonTokenInvalid}
	}
	switch state.Status {
	case "active":
	// An account awaiting deletion stays usable during the grace period so its
	// owner can sign in and cancel. Erasure, not the request, is what ends
	// access for good.
	case "pending_deletion":
	case "deleted":
		return auth.SessionState{Reason: auth.ReasonAccountDeleted}
	default:
		// suspended, locked, pending_verification and anything unrecognised.
		return auth.SessionState{Reason: auth.ReasonAccountSuspended}
	}

	// A token with no session id predates session binding. Refusing it forces
	// one re-login rather than leaving a permanently unrevocable credential in
	// circulation.
	if sessionID == "" {
		return auth.SessionState{Reason: auth.ReasonSessionRevoked}
	}

	st, err := v.h.users.AuthSessionStatus(ctx, sessionID)
	if err != nil {
		log.Printf("auth: session lookup failed for %s: %v", sessionID, err)
		return auth.SessionState{Reason: auth.ReasonTokenInvalid}
	}
	switch {
	case !st.Exists, st.Revoked:
		return auth.SessionState{Reason: auth.ReasonSessionRevoked}
	case st.Expired:
		return auth.SessionState{Reason: auth.ReasonSessionExpired}
	}

	return auth.SessionState{Valid: true, Role: state.Role}
}
