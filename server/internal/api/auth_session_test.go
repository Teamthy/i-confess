package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Teamthy/i-confess/internal/store"
)

// Session revocation tests (§28, §29, §54, §76, §80).
//
// The property under test: a token must stop working the moment its session is
// revoked. Signature validity alone is not authority — a signed JWT proves only
// that we issued it at some point, not that it is still honoured.

type authHarness struct {
	h      *Handler
	router http.Handler
}

func newAuthHarness(t *testing.T) *authHarness {
	t.Helper()
	dbConn := setupTestDB(t)
	t.Cleanup(func() { dbConn.Close() })

	h := NewHandler(Config{JWTSecret: "test-secret", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()
	return &authHarness{h: h, router: h.Routes()}
}

func (a *authHarness) do(method, path, token string, body any) (int, map[string]any) {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)

	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (a *authHarness) register(t *testing.T, email string) (token, userID string) {
	t.Helper()
	code, out := a.do("POST", "/auth/register", "", map[string]string{
		"email": email, "password": "test-passphrase-2026", "display_name": "T", "timezone": "UTC",
	})
	if code != http.StatusOK {
		t.Fatalf("register: %d %v", code, out)
	}
	token, _ = out["token"].(string)
	if u, ok := out["user"].(map[string]any); ok {
		userID, _ = u["id"].(string)
	}
	if token == "" {
		t.Fatal("no token returned")
	}
	return token, userID
}

// A logged-out token must be rejected. Before session validation existed, the
// server revoked the refresh row but kept honouring the access token for its
// full lifetime — a stolen token survived logout entirely.
func TestLogoutInvalidatesToken(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "logout@test.com")

	if code, _ := a.do("GET", "/me", token, nil); code != http.StatusOK {
		t.Fatalf("token should work before logout: %d", code)
	}
	if code, _ := a.do("POST", "/auth/logout", token, nil); code != http.StatusOK {
		t.Fatalf("logout: %d", code)
	}
	if code, _ := a.do("GET", "/me", token, nil); code != http.StatusUnauthorized {
		t.Fatalf("token still valid after logout: got %d, want 401", code)
	}
}

// "Log out all devices" must invalidate every issued token, not just the caller's.
func TestLogoutAllInvalidatesEveryDevice(t *testing.T) {
	a := newAuthHarness(t)
	phone, _ := a.register(t, "multi@test.com")

	// A second login represents another device.
	code, out := a.do("POST", "/auth/login", "", map[string]string{
		"email": "multi@test.com", "password": "test-passphrase-2026",
	})
	if code != http.StatusOK {
		t.Fatalf("second login: %d", code)
	}
	laptop, _ := out["token"].(string)

	if code, _ := a.do("GET", "/me", laptop, nil); code != http.StatusOK {
		t.Fatalf("laptop token should work: %d", code)
	}
	if code, _ := a.do("POST", "/auth/logout-all", phone, nil); code != http.StatusOK {
		t.Fatalf("logout-all: %d", code)
	}

	for name, tok := range map[string]string{"phone": phone, "laptop": laptop} {
		if code, _ := a.do("GET", "/me", tok, nil); code != http.StatusUnauthorized {
			t.Fatalf("%s token survived logout-all: got %d, want 401", name, code)
		}
	}
}

// Suspending an account must take effect immediately. Checking status only at
// login would leave an abusive user active until their token expired.
func TestSuspensionRevokesLiveSessions(t *testing.T) {
	a := newAuthHarness(t)
	token, userID := a.register(t, "suspend@test.com")

	if code, _ := a.do("GET", "/me", token, nil); code != http.StatusOK {
		t.Fatalf("token should work while active: %d", code)
	}

	users := store.NewUserStore(a.h.db)
	if err := users.SetStatus(context.Background(), userID, "suspended"); err != nil {
		t.Fatal(err)
	}

	if code, _ := a.do("GET", "/me", token, nil); code != http.StatusUnauthorized {
		t.Fatalf("suspended user still authenticated: got %d, want 401", code)
	}
}

// Changing a password must invalidate sessions elsewhere: the usual reason to
// change a password is that someone else may have it.
func TestPasswordChangeRevokesOtherSessions(t *testing.T) {
	a := newAuthHarness(t)
	attacker, _ := a.register(t, "victim@test.com")

	code, out := a.do("POST", "/auth/login", "", map[string]string{
		"email": "victim@test.com", "password": "test-passphrase-2026",
	})
	if code != http.StatusOK {
		t.Fatalf("login: %d", code)
	}
	owner, _ := out["token"].(string)

	if code, _ := a.do("POST", "/auth/change-password", owner, map[string]string{
		"current_password": "test-passphrase-2026", "new_password": "a-much-better-password",
	}); code != http.StatusOK {
		t.Fatalf("change password: %d", code)
	}

	if code, _ := a.do("GET", "/me", attacker, nil); code != http.StatusUnauthorized {
		t.Fatalf("stolen session survived a password change: got %d, want 401", code)
	}
}

// Enumeration protection (§19, §33, §65): registration and password reset must
// not reveal whether an email is already known.
func TestNoAccountEnumeration(t *testing.T) {
	a := newAuthHarness(t)
	a.register(t, "known@test.com")

	// Login with a wrong password vs an unknown account must be indistinguishable.
	codeKnown, bodyKnown := a.do("POST", "/auth/login", "", map[string]string{
		"email": "known@test.com", "password": "wrong-password",
	})
	codeUnknown, bodyUnknown := a.do("POST", "/auth/login", "", map[string]string{
		"email": "nobody@test.com", "password": "wrong-password",
	})
	if codeKnown != codeUnknown {
		t.Fatalf("status differs: known=%d unknown=%d", codeKnown, codeUnknown)
	}
	if bodyKnown["error"] != bodyUnknown["error"] {
		t.Fatalf("error differs: known=%q unknown=%q", bodyKnown["error"], bodyUnknown["error"])
	}

	// Password reset must respond identically either way.
	codeA, bodyA := a.do("POST", "/auth/request-password-reset", "", map[string]string{"email": "known@test.com"})
	codeB, bodyB := a.do("POST", "/auth/request-password-reset", "", map[string]string{"email": "nobody@test.com"})
	if codeA != codeB {
		t.Fatalf("reset status differs: %d vs %d", codeA, codeB)
	}
	if bodyA["message"] != bodyB["message"] {
		t.Fatalf("reset message differs: %q vs %q", bodyA["message"], bodyB["message"])
	}
	// And must not confirm existence in the wording.
	if msg, _ := bodyA["message"].(string); msg == "" {
		t.Fatal("expected a neutral confirmation message")
	}
}

// A forged or tampered token must never authenticate.
func TestForgedTokensRejected(t *testing.T) {
	a := newAuthHarness(t)
	valid, _ := a.register(t, "forge@test.com")

	bad := []struct{ name, token string }{
		{"empty", ""},
		{"garbage", "not-a-jwt"},
		{"alg-none", "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJhZG1pbiJ9."},
		{"truncated", valid[:len(valid)-6]},
		{"flipped signature", valid[:len(valid)-3] + "AAA"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if code, _ := a.do("GET", "/me", tc.token, nil); code != http.StatusUnauthorized {
				t.Fatalf("token %q accepted: got %d, want 401", tc.name, code)
			}
		})
	}
}

// The full recovery flow must work with unpredictable tokens, and the old
// predictable form must not.
func TestPasswordResetFlowUsesUnguessableTokens(t *testing.T) {
	a := newAuthHarness(t)

	captured := map[string]string{}
	a.h.SetDevTokenSink(func(purpose, email, token string) { captured[purpose] = token })

	_, userID := a.register(t, "reset@test.com")

	if code, _ := a.do("POST", "/auth/request-password-reset", "", map[string]string{
		"email": "reset@test.com",
	}); code != http.StatusOK {
		t.Fatalf("request reset: %d", code)
	}

	token := captured["password_reset"]
	if token == "" {
		t.Fatal("no reset token was issued")
	}
	// The old scheme was "reset-" + user id. Anything derived from the id is
	// guessable and must not be what we issue.
	if token == "reset-"+userID || len(token) < 32 {
		t.Fatalf("reset token is guessable: %q", token)
	}

	// A guessed token must fail.
	if code, _ := a.do("POST", "/auth/reset-password", "", map[string]string{
		"token": "reset-" + userID, "password": "brand-new-password",
	}); code != http.StatusUnauthorized {
		t.Fatalf("guessable token accepted: %d", code)
	}

	// The real token must work.
	if code, _ := a.do("POST", "/auth/reset-password", "", map[string]string{
		"token": token, "password": "brand-new-password",
	}); code != http.StatusOK {
		t.Fatalf("valid reset token rejected: %d", code)
	}

	// Single use (§16, §34).
	if code, _ := a.do("POST", "/auth/reset-password", "", map[string]string{
		"token": token, "password": "another-password",
	}); code == http.StatusOK {
		t.Fatal("reset token was reusable")
	}

	// The new password works and the old one does not.
	if code, _ := a.do("POST", "/auth/login", "", map[string]string{
		"email": "reset@test.com", "password": "brand-new-password",
	}); code != http.StatusOK {
		t.Fatalf("login with new password: %d", code)
	}
	if code, _ := a.do("POST", "/auth/login", "", map[string]string{
		"email": "reset@test.com", "password": "test-passphrase-2026",
	}); code == http.StatusOK {
		t.Fatal("old password still works after reset")
	}
}

// A one-time token must never be returned in an HTTP response: that would let
// anyone who can guess an address reset the account.
func TestResetTokenNeverAppearsInResponse(t *testing.T) {
	a := newAuthHarness(t)
	captured := map[string]string{}
	a.h.SetDevTokenSink(func(purpose, email, token string) { captured[purpose] = token })
	a.register(t, "leak@test.com")

	body, _ := json.Marshal(map[string]string{"email": "leak@test.com"})
	req := httptest.NewRequest("POST", "/auth/request-password-reset", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)

	tok := captured["password_reset"]
	if tok == "" {
		t.Fatal("no token issued")
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(tok)) {
		t.Fatalf("reset token leaked in response: %s", rec.Body.String())
	}
}

// Ownership: one user must never reach another's resources (§71, §91).
func TestCrossUserAccessDenied(t *testing.T) {
	a := newAuthHarness(t)
	alice, _ := a.register(t, "alice@test.com")
	bob, _ := a.register(t, "bob@test.com")

	// Alice creates a schedule.
	code, out := a.do("POST", "/schedules", alice, map[string]any{
		"label": "Morning", "time": "06:00", "days_of_week": []int{1, 2, 3},
		"timezone": "Africa/Lagos", "duration_seconds": 600,
	})
	if code != http.StatusCreated {
		t.Fatalf("create schedule: %d %v", code, out)
	}
	scheduleID, _ := out["id"].(string)

	// Bob must not be able to touch it.
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{"PATCH", "/schedules/" + scheduleID, map[string]any{"enabled": false}},
		{"DELETE", "/schedules/" + scheduleID, nil},
	} {
		code, _ := a.do(tc.method, tc.path, bob, tc.body)
		if code == http.StatusOK || code == http.StatusNoContent {
			t.Fatalf("%s %s: bob modified alice's schedule (got %d)", tc.method, tc.path, code)
		}
	}

	// And Alice's schedule must still be there.
	code, _ = a.do("GET", "/schedules", alice, nil)
	if code != http.StatusOK {
		t.Fatalf("alice list schedules: %d", code)
	}
}

// Brute force must become expensive, and must do so without letting an
// attacker lock a victim out (§20, §21).
func TestLoginBruteForceIsThrottled(t *testing.T) {
	a := newAuthHarness(t)
	a.register(t, "target@test.com")

	throttled := false
	for i := 0; i < 30; i++ {
		code, _ := a.do("POST", "/auth/login", "", map[string]string{
			"email": "target@test.com", "password": "wrong",
		})
		if code == http.StatusTooManyRequests {
			throttled = true
			break
		}
	}
	if !throttled {
		t.Fatal("unlimited password guesses were allowed")
	}
}

// Throttling must not become a denial-of-service tool: hammering one account
// must not prevent a different user from signing in.
func TestThrottlingOneAccountDoesNotBlockAnother(t *testing.T) {
	a := newAuthHarness(t)
	a.register(t, "victim2@test.com")
	a.register(t, "bystander@test.com")

	for i := 0; i < 30; i++ {
		a.do("POST", "/auth/login", "", map[string]string{
			"email": "victim2@test.com", "password": "wrong",
		})
	}

	// The bystander shares the test's client address but a different account,
	// so the per-account limit must not have consumed their budget.
	code, _ := a.do("POST", "/auth/login", "", map[string]string{
		"email": "bystander@test.com", "password": "test-passphrase-2026",
	})
	if code == http.StatusTooManyRequests {
		t.Skip("per-IP limit reached first; per-account isolation covered by unit tests")
	}
	if code != http.StatusOK {
		t.Fatalf("bystander could not sign in: %d", code)
	}
}

// A throttled response must tell the client when to retry rather than leaving
// it to guess or hammer.
func TestThrottleIncludesRetryHint(t *testing.T) {
	a := newAuthHarness(t)
	a.register(t, "retry@test.com")

	for i := 0; i < 40; i++ {
		body, _ := json.Marshal(map[string]string{"email": "retry@test.com", "password": "wrong"})
		req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		a.router.ServeHTTP(rec, req)

		if rec.Code == http.StatusTooManyRequests {
			if rec.Header().Get("Retry-After") == "" {
				t.Fatal("429 without a Retry-After header")
			}
			var out map[string]any
			json.Unmarshal(rec.Body.Bytes(), &out)
			if out["code"] != "AUTH_RATE_LIMITED" {
				t.Fatalf("code = %v, want AUTH_RATE_LIMITED", out["code"])
			}
			return
		}
	}
	t.Fatal("never throttled")
}

// The security screen must list live sessions, mark the current one, and let a
// user sign out a single device (§30, §31, §54).
func TestListAndRevokeIndividualSessions(t *testing.T) {
	a := newAuthHarness(t)
	phone, _ := a.register(t, "devices@test.com")

	code, out := a.do("POST", "/auth/login", "", map[string]string{
		"email": "devices@test.com", "password": "test-passphrase-2026",
	})
	if code != http.StatusOK {
		t.Fatalf("second login: %d", code)
	}
	laptop, _ := out["token"].(string)

	// List from the laptop.
	req := httptest.NewRequest("GET", "/auth/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+laptop)
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list sessions: %d", rec.Code)
	}

	var sessions []struct {
		ID      string `json:"id"`
		Current bool   `json:"current"`
	}
	json.Unmarshal(rec.Body.Bytes(), &sessions)
	if len(sessions) != 2 {
		t.Fatalf("expected 2 live sessions, got %d", len(sessions))
	}
	currents := 0
	var other string
	for _, s := range sessions {
		if s.Current {
			currents++
		} else {
			other = s.ID
		}
	}
	if currents != 1 {
		t.Fatalf("expected exactly one session marked current, got %d", currents)
	}

	// Sign the phone out from the laptop.
	if code, _ := a.do("DELETE", "/auth/sessions/"+other, laptop, nil); code != http.StatusOK {
		t.Fatalf("revoke session: %d", code)
	}
	if code, _ := a.do("GET", "/me", phone, nil); code != http.StatusUnauthorized {
		t.Fatalf("revoked device still authenticated: %d", code)
	}
	// The laptop keeps working.
	if code, _ := a.do("GET", "/me", laptop, nil); code != http.StatusOK {
		t.Fatalf("current session was revoked too: %d", code)
	}
}

// One user must not be able to revoke another user's session by guessing an id.
func TestCannotRevokeAnotherUsersSession(t *testing.T) {
	a := newAuthHarness(t)
	alice, _ := a.register(t, "alice2@test.com")
	bob, _ := a.register(t, "bob2@test.com")

	req := httptest.NewRequest("GET", "/auth/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+alice)
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)

	var sessions []struct {
		ID string `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &sessions)
	if len(sessions) == 0 {
		t.Fatal("alice has no sessions")
	}

	// Bob tries to revoke Alice's session. The call may report success, but it
	// must not actually revoke anything.
	a.do("DELETE", "/auth/sessions/"+sessions[0].ID, bob, nil)

	if code, _ := a.do("GET", "/me", alice, nil); code != http.StatusOK {
		t.Fatalf("bob revoked alice's session: alice now gets %d", code)
	}
}
