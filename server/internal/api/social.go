package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/oauth"
	"github.com/Teamthy/i-confess/internal/store"
)

// Social sign-in (§36, §37, §38, §64).
//
// The rule that governs every branch below: an identity assertion is believed
// only after its signature is verified against the provider's published keys.
// Nothing here trusts an email, subject or name supplied by the client.

// SetVerifiers installs the configured identity providers.
func (h *Handler) SetVerifiers(v map[string]oauth.Verifier) { h.verifiers = v }

// socialSignIn exchanges a provider identity token for an application session.
func (h *Handler) socialSignIn(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")

	verifier, ok := h.verifiers[provider]
	if !ok || verifier == nil {
		httpx.WriteJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "this sign-in method is not enabled",
			"code":  "AUTH_PROVIDER_DISABLED",
		})
		return
	}

	var req struct {
		IDToken string `json:"id_token"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.IDToken == "" {
		writeCode(w, http.StatusBadRequest, "AUTH_TOKEN_INVALID", "id_token is required")
		return
	}

	identity, err := verifier.Verify(r.Context(), req.IDToken)
	if errors.Is(err, oauth.ErrProviderUnavai) {
		// The provider being down is our availability problem, not a bad
		// credential, so it must not be reported as an auth failure.
		httpx.WriteJSON(w, http.StatusBadGateway, map[string]string{
			"error": "sign-in provider is temporarily unavailable",
			"code":  "AUTH_PROVIDER_UNAVAILABLE",
		})
		return
	}
	if err != nil {
		log.Printf("auth: %s identity token rejected: %v", provider, err)
		writeCode(w, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "could not verify that sign-in")
		return
	}

	u, err := h.resolveFederatedUser(r.Context(), identity)
	if err != nil {
		if errors.Is(err, errLinkRequiresProof) {
			// An unverified provider email must never claim an existing local
			// account: otherwise anyone who can mint an unverified assertion
			// for a known address takes it over (§38).
			httpx.WriteJSON(w, http.StatusConflict, map[string]string{
				"error": "an account already uses this email; sign in with your password and link this provider from Settings",
				"code":  "AUTH_LINK_REQUIRES_SIGN_IN",
			})
			return
		}
		log.Printf("auth: federated sign-in failed for %s: %v", provider, err)
		httpx.WriteError(w, http.StatusInternalServerError, "sign-in failed")
		return
	}

	if u.Status != "active" {
		httpx.WriteError(w, http.StatusForbidden, "this account is not available")
		return
	}

	h.issueToken(w, r, u)
}

// errLinkRequiresProof signals that automatic linking was refused.
var errLinkRequiresProof = errors.New("linking requires an authenticated session")

// resolveFederatedUser maps a verified identity to an account.
//
// The order matters:
//
//  1. A known (provider, subject) is the account. This is the only stable key.
//  2. Otherwise, a *verified* provider email may link to an existing account —
//     the provider has vouched for the address, which is equivalent proof to
//     our own email verification.
//  3. An *unverified* provider email may not link. It is refused, and the user
//     is told to sign in first. Linking on an unverified address is the classic
//     OAuth account-takeover path.
//  4. Otherwise a new account is created.
func (h *Handler) resolveFederatedUser(ctx context.Context, id *oauth.Identity) (*models.User, error) {
	userID, err := h.users.UserIDByIdentity(ctx, id.Provider, id.Subject)
	if err == nil {
		return h.users.ByID(ctx, userID)
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	if id.Email != "" {
		existing, _, err := h.users.ByEmail(ctx, id.Email)
		if err == nil && existing != nil {
			if !id.EmailVerified {
				return nil, errLinkRequiresProof
			}
			if err := h.users.LinkIdentity(ctx, existing.ID, id.Provider, id.Subject, id.Email, id.EmailVerified); err != nil {
				return nil, err
			}
			h.notifySecurityEvent(existing.Email,
				"A new sign-in method was added to your account",
				"You can now sign in with "+id.Provider+". If this was not you, change your password immediately.")
			return existing, nil
		}
	}

	// New account. Apple relay addresses are stored as-is: they deliver mail
	// and are the only address we are given.
	email := id.Email
	name := strings.TrimSpace(id.Name)
	u, err := h.users.CreateFederated(ctx, email, name, "UTC", id.EmailVerified)
	if err != nil {
		return nil, err
	}
	if err := h.users.LinkIdentity(ctx, u.ID, id.Provider, id.Subject, email, id.EmailVerified); err != nil {
		return nil, err
	}
	return u, nil
}

// listIdentities shows which providers are linked to the caller's account.
func (h *Handler) listIdentities(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)

	identities, err := h.users.ListIdentities(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load sign-in methods")
		return
	}
	hasPassword, err := h.users.HasPassword(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load sign-in methods")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"password":   hasPassword,
		"identities": identities,
	})
}

// linkIdentity attaches a provider to the caller's already-authenticated
// account. Requiring an existing session is what makes linking safe: the user
// has proven they own this account before a new credential is attached (§38).
func (h *Handler) linkIdentity(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	verifier, ok := h.verifiers[provider]
	if !ok || verifier == nil {
		httpx.WriteJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "this sign-in method is not enabled",
			"code":  "AUTH_PROVIDER_DISABLED",
		})
		return
	}

	var req struct {
		IDToken string `json:"id_token"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.IDToken == "" {
		writeCode(w, http.StatusBadRequest, "AUTH_TOKEN_INVALID", "id_token is required")
		return
	}

	identity, err := verifier.Verify(r.Context(), req.IDToken)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "could not verify that sign-in")
		return
	}

	userID := h.userID(r)

	// If this provider identity already belongs to someone else, refuse. A
	// silent re-point would hand one person's Google account to another user.
	if owner, err := h.users.UserIDByIdentity(r.Context(), identity.Provider, identity.Subject); err == nil && owner != userID {
		httpx.WriteJSON(w, http.StatusConflict, map[string]string{
			"error": "that " + provider + " account is already linked to another user",
			"code":  "AUTH_IDENTITY_IN_USE",
		})
		return
	}

	if err := h.users.LinkIdentity(r.Context(), userID, identity.Provider, identity.Subject, identity.Email, identity.EmailVerified); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to link sign-in method")
		return
	}

	if c := auth.FromContext(r); c != nil {
		h.notifySecurityEvent(c.Email,
			"A new sign-in method was added to your account",
			"You can now sign in with "+provider+". If this was not you, change your password immediately.")
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": provider + " linked"})
}

// unlinkIdentity detaches a provider.
//
// It refuses to remove the last remaining credential, which would leave the
// user unable to sign in to their own account.
func (h *Handler) unlinkIdentity(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	userID := h.userID(r)

	hasPassword, err := h.users.HasPassword(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load sign-in methods")
		return
	}
	identities, err := h.users.ListIdentities(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load sign-in methods")
		return
	}

	remaining := 0
	for _, i := range identities {
		if i.Provider != provider {
			remaining++
		}
	}
	if !hasPassword && remaining == 0 {
		httpx.WriteJSON(w, http.StatusConflict, map[string]string{
			"error": "set a password before removing your only sign-in method",
			"code":  "AUTH_LAST_CREDENTIAL",
		})
		return
	}

	if err := h.users.UnlinkIdentity(r.Context(), userID, provider); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to unlink sign-in method")
		return
	}
	if c := auth.FromContext(r); c != nil {
		h.notifySecurityEvent(c.Email,
			"A sign-in method was removed from your account",
			provider+" can no longer be used to sign in. If this was not you, change your password immediately.")
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": provider + " unlinked"})
}
