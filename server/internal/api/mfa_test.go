package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/mfa"
)

// MFA over HTTP (§41, §42, §81).

// enrolMFA runs the two-step enrolment and returns the secret and recovery codes.
func enrolMFA(t *testing.T, a *authHarness, token string) (secret string, recovery []string) {
	t.Helper()

	code, out := a.do("POST", "/auth/mfa/begin", token, nil)
	if code != http.StatusOK {
		t.Fatalf("begin enrolment: %d %v", code, out)
	}
	secret, _ = out["secret"].(string)
	if secret == "" {
		t.Fatal("no secret issued")
	}

	totp, err := mfa.Code(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	code, out = a.do("POST", "/auth/mfa/confirm", token, map[string]string{"code": totp})
	if code != http.StatusOK {
		t.Fatalf("confirm enrolment: %d %v", code, out)
	}
	for _, c := range out["recovery_codes"].([]any) {
		recovery = append(recovery, c.(string))
	}
	return secret, recovery
}

// The secret must come from the server. The previous implementation accepted a
// client-supplied secret, which let an attacker enrol a factor they controlled.
func TestEnrolmentSecretIsServerGenerated(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "mfa1@test.com")

	_, out := a.do("POST", "/auth/mfa/begin", token, map[string]string{
		"secret": "ATTACKERCONTROLLEDSECRET",
	})
	if out["secret"] == "ATTACKERCONTROLLEDSECRET" {
		t.Fatal("the server accepted a client-supplied MFA secret")
	}
	if uri, _ := out["provisioning_uri"].(string); uri == "" {
		t.Fatal("no provisioning URI returned")
	}
}

// Enrolment must not activate until the user proves they can produce a code,
// or a mis-scanned QR code locks them out.
func TestEnrolmentRequiresProofBeforeActivation(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "mfa2@test.com")

	a.do("POST", "/auth/mfa/begin", token, nil)

	_, status := a.do("GET", "/auth/mfa", token, nil)
	if status["enabled"] != false {
		t.Fatal("MFA became active before the code was verified")
	}
	if status["pending"] != true {
		t.Fatal("a started enrolment is not reported as pending")
	}

	// A wrong code must not activate it.
	if code, _ := a.do("POST", "/auth/mfa/confirm", token, map[string]string{"code": "000000"}); code != http.StatusUnauthorized {
		t.Fatalf("wrong confirmation code accepted: %d", code)
	}
	_, status = a.do("GET", "/auth/mfa", token, nil)
	if status["enabled"] != false {
		t.Fatal("MFA activated despite a failed confirmation")
	}
}

// The headline property: a correct password alone must not yield a session.
func TestLoginRequiresSecondFactorOnceEnrolled(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "mfa3@test.com")
	secret, _ := enrolMFA(t, a, token)

	code, out := a.do("POST", "/auth/login", "", map[string]string{
		"email": "mfa3@test.com", "password": "password123",
	})
	if code != http.StatusOK {
		t.Fatalf("login: %d", code)
	}
	if _, gotToken := out["token"]; gotToken {
		t.Fatal("password alone issued a session despite MFA being enabled")
	}
	if out["mfa_required"] != true {
		t.Fatalf("server did not signal that a second factor is needed: %v", out)
	}

	// The enrolment code was consumed, so use the next window's code: a TOTP
	// code is single-use by design, which is what the replay guard enforces.
	totp, _ := mfa.Code(secret, time.Now().Add(mfa.Period))
	code, out = a.do("POST", "/auth/login", "", map[string]string{
		"email": "mfa3@test.com", "password": "password123", "code": totp,
	})
	if code != http.StatusOK {
		t.Fatalf("login with code: %d %v", code, out)
	}
	session, _ := out["token"].(string)
	if session == "" {
		t.Fatal("no session issued after a valid second factor")
	}
	if c, _ := a.do("GET", "/me", session, nil); c != http.StatusOK {
		t.Fatalf("issued session does not work: %d", c)
	}
}

func TestLoginRejectsWrongSecondFactor(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "mfa4@test.com")
	enrolMFA(t, a, token)

	code, out := a.do("POST", "/auth/login", "", map[string]string{
		"email": "mfa4@test.com", "password": "password123", "code": "123456",
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("wrong code accepted: %d", code)
	}
	if _, gotToken := out["token"]; gotToken {
		t.Fatal("a session was issued with an invalid code")
	}
}

// A recovery code works when the authenticator is gone, and works only once.
func TestRecoveryCodeSignsInOnce(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "mfa5@test.com")
	_, recovery := enrolMFA(t, a, token)

	if len(recovery) < 5 {
		t.Fatalf("expected a set of recovery codes, got %d", len(recovery))
	}

	code, out := a.do("POST", "/auth/login", "", map[string]string{
		"email": "mfa5@test.com", "password": "password123", "code": recovery[0],
	})
	if code != http.StatusOK {
		t.Fatalf("recovery code rejected: %d %v", code, out)
	}
	if _, ok := out["token"]; !ok {
		t.Fatal("no session issued for a valid recovery code")
	}

	// The same code must not work twice.
	code, _ = a.do("POST", "/auth/login", "", map[string]string{
		"email": "mfa5@test.com", "password": "password123", "code": recovery[0],
	})
	if code == http.StatusOK {
		t.Fatal("a recovery code was accepted twice")
	}

	// A different one still works.
	code, _ = a.do("POST", "/auth/login", "", map[string]string{
		"email": "mfa5@test.com", "password": "password123", "code": recovery[1],
	})
	if code != http.StatusOK {
		t.Fatalf("second recovery code rejected: %d", code)
	}
}

// Recovery codes are shown once and stored only as hashes, so the platform
// genuinely cannot reveal them again.
func TestRecoveryCodesAreNotRetrievable(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "mfa6@test.com")
	_, recovery := enrolMFA(t, a, token)

	_, status := a.do("GET", "/auth/mfa", token, nil)
	for _, c := range recovery {
		if status["recovery_codes"] != nil {
			t.Fatal("status endpoint returns recovery codes")
		}
		_ = c
	}
	if status["recovery_codes_remaining"] == nil {
		t.Fatal("status should report how many codes remain")
	}
}

// Removing a second factor is what an attacker with a stolen session does
// first, so it needs the password and a current code.
func TestDisableRequiresPasswordAndCode(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "mfa7@test.com")
	secret, _ := enrolMFA(t, a, token)

	if code, _ := a.do("POST", "/auth/mfa/disable", token, map[string]string{
		"password": "wrong", "code": "123456",
	}); code != http.StatusUnauthorized {
		t.Fatalf("wrong password accepted: %d", code)
	}
	if code, _ := a.do("POST", "/auth/mfa/disable", token, map[string]string{
		"password": "password123", "code": "000000",
	}); code != http.StatusUnauthorized {
		t.Fatalf("wrong code accepted: %d", code)
	}

	// Still on.
	_, status := a.do("GET", "/auth/mfa", token, nil)
	if status["enabled"] != true {
		t.Fatal("MFA was disabled by a failed attempt")
	}

	// With both, it comes off.
	// Next window: the enrolment code was consumed, and Skew is one step so a
	// code further ahead than that is correctly outside the accepted range.
	totp, _ := mfa.Code(secret, time.Now().Add(mfa.Period))
	code, out := a.do("POST", "/auth/mfa/disable", token, map[string]string{
		"password": "password123", "code": totp,
	})
	if code != http.StatusOK {
		t.Fatalf("disable with valid credentials: %d %v", code, out)
	}
	_, status = a.do("GET", "/auth/mfa", token, nil)
	if status["enabled"] != false {
		t.Fatal("MFA still enabled after a successful disable")
	}

	// And login no longer asks for a factor.
	_, loginOut := a.do("POST", "/auth/login", "", map[string]string{
		"email": "mfa7@test.com", "password": "password123",
	})
	if _, ok := loginOut["token"]; !ok {
		t.Fatal("login still requires a second factor after disabling")
	}
}

// Re-enrolling while active would invalidate a working factor if the new one
// is never confirmed.
func TestCannotReEnrolWhileEnabled(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "mfa8@test.com")
	enrolMFA(t, a, token)

	code, out := a.do("POST", "/auth/mfa/begin", token, nil)
	if code != http.StatusConflict {
		t.Fatalf("re-enrolment allowed while enabled: %d", code)
	}
	if out["code"] != "MFA_ALREADY_ENABLED" {
		t.Fatalf("code = %v", out["code"])
	}
}

// Six digits is only 10^6, so guessing must be throttled.
func TestSecondFactorGuessesAreThrottled(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "mfa9@test.com")
	enrolMFA(t, a, token)

	throttled := false
	for i := 0; i < 30; i++ {
		code, _ := a.do("POST", "/auth/login", "", map[string]string{
			"email": "mfa9@test.com", "password": "password123", "code": "000000",
		})
		if code == http.StatusTooManyRequests {
			throttled = true
			break
		}
	}
	if !throttled {
		t.Fatal("unlimited second-factor guesses were allowed")
	}
}

func TestMFAEndpointsRequireAuth(t *testing.T) {
	a := newAuthHarness(t)
	for _, path := range []string{"/auth/mfa", "/auth/mfa/begin", "/auth/mfa/confirm", "/auth/mfa/disable"} {
		method := "POST"
		if path == "/auth/mfa" {
			method = "GET"
		}
		if code, _ := a.do(method, path, "", map[string]string{"code": "123456"}); code != http.StatusUnauthorized {
			t.Fatalf("%s %s without auth: got %d, want 401", method, path, code)
		}
	}
}
