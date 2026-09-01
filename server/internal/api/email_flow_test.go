package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/email"
)

// End-to-end email flows (§12, §15, §32, §52, §57, §58).
//
// Before this slice tokens were generated correctly but never delivered, so
// verification and reset were non-functional end to end. These tests drive the
// real queue and templates through a capturing sender.

func newMailHarness(t *testing.T) (*authHarness, *email.MemorySender) {
	t.Helper()
	a := newAuthHarness(t)

	sender := email.NewMemorySender()
	q := email.NewQueue(sender, 32)
	q.Backoff = func(int) time.Duration { return time.Millisecond }
	q.Start(context.Background(), 2)
	t.Cleanup(q.Stop)

	a.h.SetMailer(q, email.Config{
		AppName: "i-confess", BaseURL: "https://iconfess.app",
		FromAddress: "no-reply@iconfess.app", SupportAddress: "support@iconfess.app",
	})
	return a, sender
}

// tokenFromLink pulls the ?token= value out of the first link in a body.
func tokenFromLink(t *testing.T, body, path string) string {
	t.Helper()
	i := strings.Index(body, "https://iconfess.app"+path)
	if i < 0 {
		t.Fatalf("no %s link in body:\n%s", path, body)
	}
	rest := body[i:]
	if j := strings.IndexAny(rest, " \n\r\"<"); j > 0 {
		rest = rest[:j]
	}
	u, err := url.Parse(rest)
	if err != nil {
		t.Fatalf("unparseable link %q: %v", rest, err)
	}
	tok := u.Query().Get("token")
	if tok == "" {
		t.Fatalf("link carries no token: %s", rest)
	}
	return tok
}

func waitForMail(t *testing.T, s *email.MemorySender, addr string) email.Message {
	t.Helper()
	return waitForTaggedMail(t, s, addr, "")
}

// waitForTaggedMail waits for a message to an address, optionally of a specific
// tag. Filtering by tag matters because registration also emits a verification
// mail, and a test asserting on a security alert must not match it by accident.
func waitForTaggedMail(t *testing.T, s *email.MemorySender, addr, tag string) email.Message {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, m := range s.Sent() {
			if m.To == addr && (tag == "" || m.Tag == tag) {
				return m
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no %s email delivered to %s", tag, addr)
	return email.Message{}
}

// Registering must actually send a verification email, and the link in it must
// verify the account.
func TestRegistrationSendsWorkingVerificationLink(t *testing.T) {
	a, sender := newMailHarness(t)

	if code, _ := a.do("POST", "/auth/register", "", map[string]string{
		"email": "verify@example.com", "password": "password123",
		"display_name": "Grace", "timezone": "Africa/Lagos",
	}); code != http.StatusOK {
		t.Fatalf("register: %d", code)
	}

	msg := waitForMail(t, sender, "verify@example.com")
	if msg.Tag != "email_verification" {
		t.Fatalf("tag = %q", msg.Tag)
	}

	token := tokenFromLink(t, msg.Text, "/verify-email")
	if code, _ := a.do("POST", "/auth/verify-email", "", map[string]string{"token": token}); code != http.StatusOK {
		t.Fatalf("verify with emailed token: %d", code)
	}

	// Single use (§16).
	if code, _ := a.do("POST", "/auth/verify-email", "", map[string]string{"token": token}); code == http.StatusOK {
		t.Fatal("verification token was reusable")
	}
}

// The whole recovery journey, driven only by what arrives in the inbox.
func TestPasswordResetJourneyViaEmail(t *testing.T) {
	a, sender := newMailHarness(t)
	a.register(t, "recover@example.com")
	sender.Reset()

	if code, _ := a.do("POST", "/auth/request-password-reset", "", map[string]string{
		"email": "recover@example.com",
	}); code != http.StatusOK {
		t.Fatalf("request reset: %d", code)
	}

	msg := waitForTaggedMail(t, sender, "recover@example.com", "password_reset")
	token := tokenFromLink(t, msg.Text, "/reset-password")

	if code, _ := a.do("POST", "/auth/reset-password", "", map[string]string{
		"token": token, "password": "a-completely-new-password",
	}); code != http.StatusOK {
		t.Fatalf("reset: %d", code)
	}
	if code, _ := a.do("POST", "/auth/login", "", map[string]string{
		"email": "recover@example.com", "password": "a-completely-new-password",
	}); code != http.StatusOK {
		t.Fatalf("login with the new password: %d", code)
	}
}

// An unknown address must produce the same HTTP response as a known one, and
// must not generate mail to a stranger's inbox (§33, §65).
func TestResetForUnknownAddressSendsNothing(t *testing.T) {
	a, sender := newMailHarness(t)
	a.register(t, "known2@example.com")
	sender.Reset()

	codeKnown, bodyKnown := a.do("POST", "/auth/request-password-reset", "", map[string]string{
		"email": "known2@example.com",
	})
	codeUnknown, bodyUnknown := a.do("POST", "/auth/request-password-reset", "", map[string]string{
		"email": "stranger@example.com",
	})

	if codeKnown != codeUnknown || bodyKnown["message"] != bodyUnknown["message"] {
		t.Fatalf("responses differ: %d/%v vs %d/%v",
			codeKnown, bodyKnown["message"], codeUnknown, bodyUnknown["message"])
	}

	time.Sleep(100 * time.Millisecond)
	for _, m := range sender.Sent() {
		if m.To == "stranger@example.com" {
			t.Fatal("sent reset mail to an address with no account")
		}
	}
}

// Registering with an address that already exists must tell the real owner,
// not the caller (§65).
func TestDuplicateRegistrationNotifiesOwnerNotCaller(t *testing.T) {
	a, sender := newMailHarness(t)
	a.register(t, "owner@example.com")
	sender.Reset()

	code, out := a.do("POST", "/auth/register", "", map[string]string{
		"email": "owner@example.com", "password": "attacker-chosen",
	})
	if code != http.StatusOK {
		t.Fatalf("duplicate registration: got %d, want a neutral 200", code)
	}
	if _, hasToken := out["token"]; hasToken {
		t.Fatal("duplicate registration returned a session token")
	}

	msg := waitForTaggedMail(t, sender, "owner@example.com", "security_alert")
	// The alert must not hand over an actionable credential.
	if strings.Contains(msg.Text, "token=") {
		t.Fatal("security alert contained a token")
	}
}

// A password change must notify the account owner (§52).
func TestPasswordChangeSendsSecurityAlert(t *testing.T) {
	a, sender := newMailHarness(t)
	token, _ := a.register(t, "alert@example.com")
	sender.Reset()

	if code, _ := a.do("POST", "/auth/change-password", token, map[string]string{
		"current_password": "password123", "new_password": "something-else-entirely",
	}); code != http.StatusOK {
		t.Fatalf("change password: %d", code)
	}

	msg := waitForTaggedMail(t, sender, "alert@example.com", "security_alert")
	if !strings.Contains(strings.ToLower(msg.Text), "password") {
		t.Fatalf("alert does not mention the password change:\n%s", msg.Text)
	}
}

// Registration must not wait on the mail provider (§58).
func TestRegistrationDoesNotBlockOnSlowProvider(t *testing.T) {
	a := newAuthHarness(t)

	slow := &slowSender{delay: 2 * time.Second}
	q := email.NewQueue(slow, 8)
	q.Start(context.Background(), 1)
	defer q.Stop()
	a.h.SetMailer(q, email.Config{AppName: "i-confess", BaseURL: "https://iconfess.app"})

	start := time.Now()
	if code, _ := a.do("POST", "/auth/register", "", map[string]string{
		"email": "slow@example.com", "password": "password123",
	}); code != http.StatusOK {
		t.Fatalf("register: %d", code)
	}
	// bcrypt dominates the request; the 2s provider delay must not be included.
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("registration blocked on the mail provider: took %s", elapsed)
	}
}

// A mail outage must not fail registration: the account exists and the user can
// use "resend".
func TestRegistrationSucceedsWhenMailProviderIsDown(t *testing.T) {
	a := newAuthHarness(t)

	broken := email.NewMemorySender()
	broken.Err = email.Retryable("provider unreachable")
	q := email.NewQueue(broken, 8)
	q.MaxAttempts = 1
	q.Start(context.Background(), 1)
	defer q.Stop()
	a.h.SetMailer(q, email.Config{AppName: "i-confess", BaseURL: "https://iconfess.app"})

	code, out := a.do("POST", "/auth/register", "", map[string]string{
		"email": "outage@example.com", "password": "password123",
	})
	if code != http.StatusOK {
		t.Fatalf("registration failed during a mail outage: %d", code)
	}
	if _, ok := out["token"]; !ok {
		t.Fatal("no session issued despite successful registration")
	}
}

type slowSender struct{ delay time.Duration }

func (s *slowSender) Name() string { return "slow" }
func (s *slowSender) Send(ctx context.Context, _ email.Message) error {
	select {
	case <-time.After(s.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
