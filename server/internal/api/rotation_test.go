package api

import (
	"net/http"
	"testing"
)

// Refresh rotation and reuse detection (§23, §76).
//
// This is the property that makes a stolen refresh token survivable: the theft
// is *detected* the next time either party uses their copy.

func TestRefreshRotatesTheSession(t *testing.T) {
	a := newAuthHarness(t)
	first, _ := a.register(t, "rot1@test.com")

	code, out := a.do("POST", "/auth/refresh", first, nil)
	if code != http.StatusOK {
		t.Fatalf("refresh: %d %v", code, out)
	}
	second, _ := out["token"].(string)
	if second == "" {
		t.Fatal("refresh issued no token")
	}
	if second == first {
		t.Fatal("refresh returned the same token; the session was not rotated")
	}

	// The new token works.
	if code, _ := a.do("GET", "/me", second, nil); code != http.StatusOK {
		t.Fatalf("rotated token does not work: %d", code)
	}
}

// The old token must stop working the moment it is rotated away. Otherwise
// rotation is cosmetic and a copied token keeps its full lifetime.
func TestRotatedTokenIsRetired(t *testing.T) {
	a := newAuthHarness(t)
	first, _ := a.register(t, "rot2@test.com")

	_, out := a.do("POST", "/auth/refresh", first, nil)
	second, _ := out["token"].(string)

	// Confirm the new token works BEFORE touching the old one: presenting the
	// retired token is itself a reuse event that revokes the whole family, so
	// checking it first would invalidate the successor and mask the result.
	if code, _ := a.do("GET", "/me", second, nil); code != http.StatusOK {
		t.Fatal("the rotated token does not work")
	}
	if code, _ := a.do("GET", "/me", first, nil); code != http.StatusUnauthorized {
		t.Fatalf("the retired token still authenticates: %d", code)
	}
}

// The headline security property: replaying a rotated token reveals that
// someone kept a copy, and there is no way to tell whether the replay is the
// attacker or the victim. Both must be signed out.
func TestReuseOfRotatedTokenRevokesTheWholeFamily(t *testing.T) {
	a := newAuthHarness(t)

	// The attacker's copy.
	stolen, _ := a.register(t, "rot3@test.com")

	// The legitimate client rotates normally.
	_, out := a.do("POST", "/auth/refresh", stolen, nil)
	live, _ := out["token"].(string)
	if code, _ := a.do("GET", "/me", live, nil); code != http.StatusOK {
		t.Fatal("legitimate token should work after rotation")
	}

	// The attacker replays the token they copied before rotation.
	code, body := a.do("GET", "/me", stolen, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("replayed token was accepted: %d", code)
	}
	if body["code"] != "AUTH_TOKEN_REUSED" {
		t.Fatalf("reuse was not identified as such: code = %v", body["code"])
	}

	// And the legitimate session is now dead too. That is deliberate: we
	// cannot tell which party is the thief, so neither keeps access.
	if code, _ := a.do("GET", "/me", live, nil); code != http.StatusUnauthorized {
		t.Fatalf("the live session survived a detected token theft: %d", code)
	}
}

// Reuse detection must not be triggered by ordinary logout.
func TestLogoutIsNotMistakenForReuse(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "rot4@test.com")

	a.do("POST", "/auth/logout", token, nil)

	code, body := a.do("GET", "/me", token, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("logged-out token works: %d", code)
	}
	if body["code"] == "AUTH_TOKEN_REUSED" {
		t.Fatal("an ordinary logout was reported as token theft, which would alarm users for no reason")
	}
}

// A compromise on one device must not sign the user out everywhere: the family
// is the rotation chain, not the whole account.
func TestReuseDoesNotAffectUnrelatedDevices(t *testing.T) {
	a := newAuthHarness(t)
	phone, _ := a.register(t, "rot5@test.com")

	// A separate login represents a different device with its own chain.
	_, out := a.do("POST", "/auth/login", "", map[string]string{
		"email": "rot5@test.com", "password": "password123",
	})
	laptop, _ := out["token"].(string)
	if laptop == "" {
		t.Fatal("second login failed")
	}

	// The phone's chain is compromised and detected.
	a.do("POST", "/auth/refresh", phone, nil)
	a.do("GET", "/me", phone, nil) // replay triggers family revocation

	// The laptop, a different chain, keeps working.
	if code, _ := a.do("GET", "/me", laptop, nil); code != http.StatusOK {
		t.Fatalf("an unrelated device was signed out by another device's compromise: %d", code)
	}
}

// Rotation must survive repeated use: a long-lived client refreshes many times.
func TestRepeatedRotationKeepsWorking(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "rot6@test.com")

	for i := 0; i < 5; i++ {
		code, out := a.do("POST", "/auth/refresh", token, nil)
		if code != http.StatusOK {
			t.Fatalf("refresh %d failed: %d %v", i, code, out)
		}
		next, _ := out["token"].(string)
		if next == "" {
			t.Fatalf("refresh %d issued no token", i)
		}
		token = next

		if code, _ := a.do("GET", "/me", token, nil); code != http.StatusOK {
			t.Fatalf("token from refresh %d does not work", i)
		}
	}
}
