package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/Teamthy/i-confess/internal/oauth"
)

// Social sign-in and account linking (§36, §37, §38).
//
// The linking rules are where account takeover lives, so each branch has a
// test. Token verification itself is covered in internal/oauth.

// stubVerifier returns a fixed identity, standing in for a real provider whose
// signature has already been checked.
type stubVerifier struct {
	provider string
	identity *oauth.Identity
	err      error
	calls    int
}

func (s *stubVerifier) Name() string { return s.provider }
func (s *stubVerifier) Verify(_ context.Context, token string) (*oauth.Identity, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.identity, nil
}

func newSocialHarness(t *testing.T, id *oauth.Identity) (*authHarness, *stubVerifier) {
	t.Helper()
	a := newAuthHarness(t)
	v := &stubVerifier{provider: "google", identity: id}
	a.h.SetVerifiers(map[string]oauth.Verifier{"google": v})
	return a, v
}

func googleIdentity(sub, email string, verified bool) *oauth.Identity {
	return &oauth.Identity{
		Provider: "google", Subject: sub, Email: email,
		EmailVerified: verified, Name: "Grace",
	}
}

// A first-time provider sign-in creates an account and signs the user in.
func TestSocialSignInCreatesAccount(t *testing.T) {
	a, _ := newSocialHarness(t, googleIdentity("google-1", "new@example.com", true))

	code, out := a.do("POST", "/auth/social/google", "", map[string]string{"id_token": "tok"})
	if code != http.StatusOK {
		t.Fatalf("social sign-in: %d %v", code, out)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatal("no session issued")
	}
	if code, _ := a.do("GET", "/me", token, nil); code != http.StatusOK {
		t.Fatalf("issued session does not work: %d", code)
	}
}

// Signing in twice must return the same account, not create a duplicate.
func TestRepeatSocialSignInReusesAccount(t *testing.T) {
	a, _ := newSocialHarness(t, googleIdentity("google-1", "same@example.com", true))

	_, first := a.do("POST", "/auth/social/google", "", map[string]string{"id_token": "tok"})
	_, second := a.do("POST", "/auth/social/google", "", map[string]string{"id_token": "tok"})

	u1, _ := first["user"].(map[string]any)
	u2, _ := second["user"].(map[string]any)
	if u1["id"] != u2["id"] {
		t.Fatalf("second sign-in created a different account: %v vs %v", u1["id"], u2["id"])
	}
}

// A provider that vouches for the address may link to an existing account: the
// provider's verification is equivalent proof to our own.
func TestVerifiedProviderEmailLinksToExistingAccount(t *testing.T) {
	a, _ := newSocialHarness(t, googleIdentity("google-1", "existing@example.com", true))
	_, userID := a.register(t, "existing@example.com")

	code, out := a.do("POST", "/auth/social/google", "", map[string]string{"id_token": "tok"})
	if code != http.StatusOK {
		t.Fatalf("verified link refused: %d %v", code, out)
	}
	u, _ := out["user"].(map[string]any)
	if u["id"] != userID {
		t.Fatalf("linked to the wrong account: %v, want %v", u["id"], userID)
	}
}

// The critical one: an UNVERIFIED provider email must never claim an existing
// account. Otherwise anyone able to mint an unverified assertion for a known
// address takes that account over.
func TestUnverifiedProviderEmailCannotClaimExistingAccount(t *testing.T) {
	a, _ := newSocialHarness(t, googleIdentity("attacker-sub", "victim@example.com", false))
	victimToken, _ := a.register(t, "victim@example.com")

	code, out := a.do("POST", "/auth/social/google", "", map[string]string{"id_token": "tok"})
	if code == http.StatusOK {
		t.Fatalf("unverified provider email took over an existing account: %v", out)
	}
	if code != http.StatusConflict {
		t.Fatalf("got %d, want 409", code)
	}
	if out["code"] != "AUTH_LINK_REQUIRES_SIGN_IN" {
		t.Fatalf("code = %v", out["code"])
	}
	// The victim's account is untouched.
	if code, _ := a.do("GET", "/me", victimToken, nil); code != http.StatusOK {
		t.Fatalf("victim's session was disturbed: %d", code)
	}
}

// Two different provider subjects with the same email are two different people
// only until one is verified; the subject, not the email, is the join key.
func TestDifferentSubjectsAreDifferentAccounts(t *testing.T) {
	a, v := newSocialHarness(t, googleIdentity("subject-A", "a@example.com", true))

	_, first := a.do("POST", "/auth/social/google", "", map[string]string{"id_token": "tok"})
	uA, _ := first["user"].(map[string]any)

	v.identity = googleIdentity("subject-B", "b@example.com", true)
	_, second := a.do("POST", "/auth/social/google", "", map[string]string{"id_token": "tok"})
	uB, _ := second["user"].(map[string]any)

	if uA["id"] == uB["id"] {
		t.Fatal("two different provider subjects resolved to one account")
	}
}

// A disabled provider must report that clearly rather than failing obscurely.
func TestDisabledProviderReturns501(t *testing.T) {
	a := newAuthHarness(t)
	code, out := a.do("POST", "/auth/social/apple", "", map[string]string{"id_token": "tok"})
	if code != http.StatusNotImplemented {
		t.Fatalf("got %d, want 501", code)
	}
	if out["code"] != "AUTH_PROVIDER_DISABLED" {
		t.Fatalf("code = %v", out["code"])
	}
}

// A rejected token must not create or resolve an account.
func TestInvalidIdentityTokenIsRejected(t *testing.T) {
	a, v := newSocialHarness(t, nil)
	v.err = oauth.ErrInvalidToken

	code, out := a.do("POST", "/auth/social/google", "", map[string]string{"id_token": "forged"})
	if code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", code)
	}
	if _, hasToken := out["token"]; hasToken {
		t.Fatal("a session was issued for an invalid identity token")
	}
}

// A provider outage is our problem, not a credential failure, and must be
// distinguishable so clients retry rather than prompting for a password.
func TestProviderOutageIsNotAuthFailure(t *testing.T) {
	a, v := newSocialHarness(t, nil)
	v.err = oauth.ErrProviderUnavai

	code, out := a.do("POST", "/auth/social/google", "", map[string]string{"id_token": "tok"})
	if code != http.StatusBadGateway {
		t.Fatalf("got %d, want 502", code)
	}
	if out["code"] != "AUTH_PROVIDER_UNAVAILABLE" {
		t.Fatalf("code = %v", out["code"])
	}
}

// Linking from an authenticated session is the safe path (§38).
func TestLinkFromAuthenticatedSession(t *testing.T) {
	a, _ := newSocialHarness(t, googleIdentity("google-9", "linkme@example.com", true))
	token, _ := a.register(t, "local@example.com")

	if code, out := a.do("POST", "/auth/identities/google", token, map[string]string{"id_token": "tok"}); code != http.StatusOK {
		t.Fatalf("link: %d %v", code, out)
	}

	code, out := a.do("GET", "/auth/identities", token, nil)
	if code != http.StatusOK {
		t.Fatalf("list identities: %d", code)
	}
	ids, _ := out["identities"].([]any)
	if len(ids) != 1 {
		t.Fatalf("expected 1 linked identity, got %d", len(ids))
	}
	if out["password"] != true {
		t.Fatal("account should still report a password credential")
	}
}

// One person's provider account must not be transferable to another user.
func TestCannotLinkIdentityOwnedByAnotherUser(t *testing.T) {
	a, _ := newSocialHarness(t, googleIdentity("shared-sub", "owner@example.com", true))

	// First user links it.
	first, _ := a.register(t, "first@example.com")
	if code, _ := a.do("POST", "/auth/identities/google", first, map[string]string{"id_token": "tok"}); code != http.StatusOK {
		t.Fatal("first link failed")
	}

	// Second user presents the same provider identity.
	second, _ := a.register(t, "second@example.com")
	code, out := a.do("POST", "/auth/identities/google", second, map[string]string{"id_token": "tok"})
	if code != http.StatusConflict {
		t.Fatalf("got %d, want 409", code)
	}
	if out["code"] != "AUTH_IDENTITY_IN_USE" {
		t.Fatalf("code = %v", out["code"])
	}
}

// Removing the last credential would lock a user out of their own account.
func TestCannotUnlinkLastCredential(t *testing.T) {
	a, _ := newSocialHarness(t, googleIdentity("only-cred", "federated@example.com", true))

	// Sign in with the provider: this account has no password.
	code, out := a.do("POST", "/auth/social/google", "", map[string]string{"id_token": "tok"})
	if code != http.StatusOK {
		t.Fatalf("social sign-in: %d", code)
	}
	token, _ := out["token"].(string)

	code, body := a.do("DELETE", "/auth/identities/google", token, nil)
	if code != http.StatusConflict {
		t.Fatalf("got %d, want 409 refusing to remove the only credential", code)
	}
	if body["code"] != "AUTH_LAST_CREDENTIAL" {
		t.Fatalf("code = %v", body["code"])
	}

	// It is still usable.
	if code, _ := a.do("GET", "/me", token, nil); code != http.StatusOK {
		t.Fatalf("account became unusable: %d", code)
	}
}

// Unlinking is fine when a password remains.
func TestUnlinkAllowedWhenPasswordRemains(t *testing.T) {
	a, _ := newSocialHarness(t, googleIdentity("removable", "keep@example.com", true))
	token, _ := a.register(t, "keep@example.com")

	a.do("POST", "/auth/identities/google", token, map[string]string{"id_token": "tok"})
	if code, out := a.do("DELETE", "/auth/identities/google", token, nil); code != http.StatusOK {
		t.Fatalf("unlink: %d %v", code, out)
	}

	_, out := a.do("GET", "/auth/identities", token, nil)
	ids, _ := out["identities"].([]any)
	if len(ids) != 0 {
		t.Fatalf("identity was not removed: %v", ids)
	}
}

// A federated account with no password must not be sign-in-able with an empty
// password through the ordinary login endpoint.
func TestFederatedAccountCannotLoginWithEmptyPassword(t *testing.T) {
	a, _ := newSocialHarness(t, googleIdentity("nopass", "nopass@example.com", true))
	a.do("POST", "/auth/social/google", "", map[string]string{"id_token": "tok"})

	for _, pw := range []string{"", " ", "test-passphrase-2026"} {
		code, _ := a.do("POST", "/auth/login", "", map[string]string{
			"email": "nopass@example.com", "password": pw,
		})
		if code == http.StatusOK {
			t.Fatalf("password login succeeded against a federated account (password %q)", pw)
		}
	}
}

// Identity endpoints require authentication.
func TestIdentityEndpointsRequireAuth(t *testing.T) {
	a, _ := newSocialHarness(t, googleIdentity("x", "x@example.com", true))
	if code, _ := a.do("GET", "/auth/identities", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("list identities without a token: %d", code)
	}
	if code, _ := a.do("POST", "/auth/identities/google", "", map[string]string{"id_token": "t"}); code != http.StatusUnauthorized {
		t.Fatalf("link without a token: %d", code)
	}
}
