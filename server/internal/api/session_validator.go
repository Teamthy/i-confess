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

// emailFor looks up an address for a security notification. Best effort: a
// failed lookup must not prevent the revocation itself.
func (v sessionValidator) emailFor(ctx context.Context, userID string) string {
	if u, err := v.h.users.ByID(ctx, userID); err == nil {
		return u.Email
	}
	return ""
}

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
	case !st.Exists:
		return auth.SessionState{Reason: auth.ReasonSessionRevoked}
	case st.Revoked:
		// Distinguish an ordinary revocation from a REPLAYED rotated session.
		// If this session was superseded by rotation and is still being
		// presented, someone kept a copy of it. There is no way to tell
		// whether the replay is the attacker or the legitimate client, so the
		// entire descendant chain is revoked and both must re-authenticate
		// (PRD S23).
		if replacedBy, _, rerr := v.h.users.SessionRotationState(ctx, sessionID); rerr == nil && replacedBy != "" {
			n, _ := v.h.users.RevokeSessionFamily(ctx, userID, sessionID)
			log.Printf("auth: refresh-token reuse detected for user=%s session=%s; revoked %d sessions in the family",
				userID, sessionID, n)
			v.h.notifySecurityEvent(v.emailFor(ctx, userID),
				"You were signed out for security",
				"A sign-in token from this account was reused, which can mean it was copied. "+
					"Everything has been signed out. Please sign in again and change your password.")
			return auth.SessionState{Reason: auth.ReasonTokenReused}
		}
		return auth.SessionState{Reason: auth.ReasonSessionRevoked}
	case st.Expired:
		return auth.SessionState{Reason: auth.ReasonSessionExpired}
	}

	return auth.SessionState{Valid: true, Role: state.Role}
}
