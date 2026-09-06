package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/deletion"
)

// Account deletion over HTTP (§40, §50, §84).

// Requesting deletion must sign the account out everywhere immediately.
func TestDeletionRequestEndsAllSessions(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "del1@test.com")

	if code, _ := a.do("GET", "/me", token, nil); code != http.StatusOK {
		t.Fatalf("precondition: %d", code)
	}

	code, out := a.do("POST", "/me/deletion", token, map[string]any{
		"password": "password123", "confirm": "DELETE", "reason": "finished",
	})
	if code != http.StatusOK {
		t.Fatalf("request deletion: %d %v", code, out)
	}

	// The session must be dead at once, not at the end of the grace period.
	if code, _ := a.do("GET", "/me", token, nil); code != http.StatusUnauthorized {
		t.Fatalf("session survived a deletion request: %d", code)
	}
}

// Re-authentication is required: a live session alone is not enough proof for
// an irreversible action (§40).
func TestDeletionRequiresPassword(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "del2@test.com")

	code, out := a.do("POST", "/me/deletion", token, map[string]any{
		"password": "wrong-password", "confirm": "DELETE",
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("wrong password accepted: %d %v", code, out)
	}
	// The account is untouched.
	if code, _ := a.do("GET", "/me", token, nil); code != http.StatusOK {
		t.Fatalf("account was affected by a failed deletion attempt: %d", code)
	}
}

// An accidental or replayed POST must not destroy an account.
func TestDeletionRequiresExplicitConfirmation(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "del3@test.com")

	for _, body := range []map[string]any{
		{"password": "password123"},
		{"password": "password123", "confirm": "yes"},
		{"password": "password123", "confirm": "delete"},
	} {
		code, out := a.do("POST", "/me/deletion", token, body)
		if code != http.StatusBadRequest {
			t.Fatalf("unconfirmed deletion accepted: %d %v", code, out)
		}
		if out["code"] != "DELETION_NOT_CONFIRMED" {
			t.Fatalf("code = %v", out["code"])
		}
	}
	if code, _ := a.do("GET", "/me", token, nil); code != http.StatusOK {
		t.Fatal("account affected despite unconfirmed requests")
	}
}

// The grace period is what makes an attacker-triggered deletion survivable.
func TestDeletionCanBeCancelledDuringGracePeriod(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "del4@test.com")

	a.do("POST", "/me/deletion", token, map[string]any{
		"password": "password123", "confirm": "DELETE",
	})

	// Signing in again is possible during the grace period, which is how the
	// owner recovers the account.
	code, out := a.do("POST", "/auth/login", "", map[string]string{
		"email": "del4@test.com", "password": "password123",
	})
	if code != http.StatusOK {
		t.Fatalf("owner cannot sign in to cancel: %d %v", code, out)
	}
	fresh, _ := out["token"].(string)

	if code, _ := a.do("DELETE", "/me/deletion", fresh, nil); code != http.StatusOK {
		t.Fatalf("cancel: %d", code)
	}
	if code, _ := a.do("GET", "/me", fresh, nil); code != http.StatusOK {
		t.Fatalf("account not usable after cancelling: %d", code)
	}
}

// The user must be told what is retained before they commit, not after.
func TestDeletionPreviewDisclosesRetention(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "del5@test.com")

	code, out := a.do("GET", "/me/deletion", token, nil)
	if code != http.StatusOK {
		t.Fatalf("preview: %d", code)
	}
	retained, _ := out["will_be_retained"].([]any)
	if len(retained) == 0 {
		t.Fatal("preview does not disclose what is retained")
	}
	deleted, _ := out["will_be_deleted"].([]any)
	if len(deleted) == 0 {
		t.Fatal("preview does not say what is deleted")
	}
	if out["grace_period_days"] == nil {
		t.Fatal("preview does not state the grace period")
	}
}

// The owner must be emailed, because that is how they discover a deletion they
// did not request.
func TestDeletionNotifiesTheOwner(t *testing.T) {
	a, sender := newMailHarness(t)
	token, _ := a.register(t, "del6@test.com")
	sender.Reset()

	a.do("POST", "/me/deletion", token, map[string]any{
		"password": "password123", "confirm": "DELETE",
	})

	msg := waitForTaggedMail(t, sender, "del6@test.com", "security_alert")
	if !strings.Contains(strings.ToLower(msg.Text), "delet") {
		t.Fatalf("alert does not mention deletion:\n%s", msg.Text)
	}
	// The alert must not carry an actionable credential.
	if strings.Contains(msg.Text, "token=") {
		t.Fatal("deletion alert contained a token")
	}
}

// The end-to-end property that matters: once erased, the credentials no longer
// work and the data is gone.
func TestErasedAccountCannotAuthenticate(t *testing.T) {
	a := newAuthHarness(t)
	token, userID := a.register(t, "erased@test.com")
	a.do("POST", "/me/collections", token, map[string]any{"name": "Gone Soon"})

	a.do("POST", "/me/deletion", token, map[string]any{
		"password": "password123", "confirm": "DELETE",
	})

	// Erase directly, as the sweeper would once the grace period elapsed.
	svc := deletion.NewService(a.h.db)
	if _, err := svc.Erase(context.Background(), userID); err != nil {
		t.Fatal(err)
	}

	// The old session is dead.
	if code, _ := a.do("GET", "/me", token, nil); code != http.StatusUnauthorized {
		t.Fatalf("erased account still authenticates: %d", code)
	}
	// The password no longer works.
	if code, _ := a.do("POST", "/auth/login", "", map[string]string{
		"email": "erased@test.com", "password": "password123",
	}); code == http.StatusOK {
		t.Fatal("erased account can still sign in")
	}
	// And the email is free to register again, since the original was
	// tombstoned rather than left occupying the unique index.
	if code, _ := a.do("POST", "/auth/register", "", map[string]string{
		"email": "erased@test.com", "password": "brand-new-password",
	}); code != http.StatusOK {
		t.Fatalf("the address is permanently unusable after erasure: %d", code)
	}
}

// A new account on a reused address must not inherit the old one's data.
func TestReRegisteringAfterErasureStartsFresh(t *testing.T) {
	a := newAuthHarness(t)
	token, userID := a.register(t, "reuse@test.com")
	a.do("POST", "/me/collections", token, map[string]any{"name": "OldSecretCollection"})
	a.do("POST", "/me/deletion", token, map[string]any{
		"password": "password123", "confirm": "DELETE",
	})

	svc := deletion.NewService(a.h.db)
	if _, err := svc.Erase(context.Background(), userID); err != nil {
		t.Fatal(err)
	}

	code, out := a.do("POST", "/auth/register", "", map[string]string{
		"email": "reuse@test.com", "password": "another-password",
	})
	if code != http.StatusOK {
		t.Fatalf("re-register: %d", code)
	}
	fresh, _ := out["token"].(string)
	if fresh == "" {
		t.Fatal("no session for the new account")
	}

	_, list := a.doList("GET", "/me/collections", fresh)
	if len(list) != 0 {
		t.Fatalf("new account inherited %d collections from the erased one", len(list))
	}
}

// Deletion endpoints require authentication.
func TestDeletionEndpointsRequireAuth(t *testing.T) {
	a := newAuthHarness(t)
	if code, _ := a.do("GET", "/me/deletion", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("preview without auth: %d", code)
	}
	if code, _ := a.do("POST", "/me/deletion", "", map[string]any{"confirm": "DELETE"}); code != http.StatusUnauthorized {
		t.Fatalf("request without auth: %d", code)
	}
}

// One user must not be able to delete another.
func TestCannotDeleteAnotherUsersAccount(t *testing.T) {
	a := newAuthHarness(t)
	alice, _ := a.register(t, "alice-del@test.com")
	bob, _ := a.register(t, "bob-del@test.com")

	// Bob supplies Alice's id; identity comes from the token, so this deletes
	// Bob's own account at most.
	a.do("POST", "/me/deletion", bob, map[string]any{
		"password": "password123", "confirm": "DELETE", "user_id": "alice",
	})

	if code, _ := a.do("GET", "/me", alice, nil); code != http.StatusOK {
		t.Fatalf("alice's account was affected by bob's request: %d", code)
	}
}

// Immediate erasure is not available to ordinary users or non-super admins.
func TestAdminEraseRequiresSuperAdmin(t *testing.T) {
	a := newAuthHarness(t)
	user, _ := a.register(t, "nosy-del@test.com")
	_, victimID := a.register(t, "victim-del@test.com")

	code, _ := a.do("POST", "/admin/users/"+victimID+"/erase", user, map[string]any{"confirm": "ERASE"})
	if code != http.StatusForbidden {
		t.Fatalf("ordinary user reached admin erasure: %d", code)
	}
	if code, _ := a.do("POST", "/admin/users/"+victimID+"/erase", "", map[string]any{"confirm": "ERASE"}); code != http.StatusForbidden {
		t.Fatalf("anonymous reached admin erasure: %d", code)
	}
}

// The sweeper must only touch accounts past the grace period.
func TestSweepOnlyErasesExpiredRequests(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "sweep@test.com")
	a.do("POST", "/me/deletion", token, map[string]any{
		"password": "password123", "confirm": "DELETE",
	})

	n, err := a.h.RunDeletionSweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("sweep erased %d accounts still inside the grace period", n)
	}
}
