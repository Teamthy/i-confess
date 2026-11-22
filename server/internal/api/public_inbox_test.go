package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/email"
)

// The site's newsletter and contact forms used to say "Subscribed" and
// "Message sent" while storing nothing.
func TestNewsletterAndContactAreStoredAndForwarded(t *testing.T) {
	a := newAuthHarness(t)
	sent := email.NewMemorySender()
	q := email.NewQueue(sent, 8)
	q.Start(context.Background(), 1)
	defer q.Stop()
	a.h.SetMailer(q, email.Config{AppName: "I CONFESS", BaseURL: "https://iconfess.app", SupportAddress: "support@iconfess.app"})

	if code, _ := a.do("POST", "/newsletter", "", map[string]string{"email": "not-an-address"}); code != http.StatusBadRequest {
		t.Fatalf("invalid email: %d, want 400", code)
	}
	for i := 0; i < 2; i++ { // idempotent: a second sign-up is not an error
		if code, out := a.do("POST", "/newsletter", "", map[string]string{"email": "Reader@Example.com"}); code != http.StatusAccepted {
			t.Fatalf("subscribe #%d: %d %v", i+1, code, out)
		}
	}
	var n int
	if err := a.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM newsletter_subscriptions WHERE email='reader@example.com'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("subscriptions = %d (%v), want exactly 1", n, err)
	}

	if code, _ := a.do("POST", "/contact", "", map[string]string{"name": "Ada", "email": "ada@example.com", "message": " "}); code != http.StatusBadRequest {
		t.Fatalf("empty message: %d, want 400", code)
	}
	code, out := a.do("POST", "/contact", "", map[string]string{"name": "Ada", "email": "ada@example.com", "message": "The Day 2 evening session would not start."})
	if code != http.StatusAccepted || out["reference"] == "" {
		t.Fatalf("contact: %d %v", code, out)
	}
	if err := a.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM contact_messages WHERE email='ada@example.com' AND status='new'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("contact messages = %d (%v), want 1", n, err)
	}
	if !q.Drain(2 * time.Second) {
		t.Fatal("mail queue did not drain")
	}
	msg, ok := sent.LastTo("support@iconfess.app")
	if !ok || !strings.Contains(msg.Text, "ada@example.com") || !strings.Contains(msg.Text, "Day 2 evening") {
		t.Fatalf("support was not forwarded the message: %+v", msg)
	}
}

// Every sign-up gets a way off the list: the welcome mail carries a link
// whose token unsubscribes that address and nothing else.
func TestNewsletterUnsubscribeLink(t *testing.T) {
	a := newAuthHarness(t)
	var token string
	a.h.SetDevTokenSink(func(purpose, _, tok string) {
		if purpose == "newsletter_unsubscribe" {
			token = tok
		}
	})
	if code, _ := a.do("POST", "/newsletter", "", map[string]string{"email": "leaver@example.com"}); code != http.StatusAccepted {
		t.Fatalf("subscribe: %d", code)
	}
	if token == "" {
		t.Fatal("no unsubscribe token was issued")
	}
	unsubscribed := func() bool {
		var at *string
		if err := a.db.QueryRowContext(context.Background(),
			`SELECT unsubscribed_at FROM newsletter_subscriptions WHERE email='leaver@example.com'`).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at != nil
	}

	if code, _ := a.do("POST", "/v1/newsletter/unsubscribe", "", map[string]string{"token": "not-the-token"}); code != http.StatusNotFound {
		t.Fatalf("wrong token: %d, want 404", code)
	}
	if unsubscribed() {
		t.Fatal("a wrong token unsubscribed the address")
	}
	for i := 0; i < 2; i++ { // the link may be clicked twice
		if code, out := a.do("POST", "/newsletter/unsubscribe", "", map[string]string{"token": token}); code != http.StatusOK {
			t.Fatalf("unsubscribe #%d: %d %v", i+1, code, out)
		}
	}
	if !unsubscribed() {
		t.Fatal("address is still subscribed")
	}

	// Signing up again re-subscribes and rotates the token: the old link is dead.
	old := token
	if code, _ := a.do("POST", "/newsletter", "", map[string]string{"email": "leaver@example.com"}); code != http.StatusAccepted || unsubscribed() {
		t.Fatalf("re-subscribe: %d, unsubscribed=%v", code, unsubscribed())
	}
	if code, _ := a.do("POST", "/newsletter/unsubscribe", "", map[string]string{"token": old}); code != http.StatusNotFound {
		t.Fatalf("old token after re-subscribe: %d, want 404", code)
	}
}

func TestNewsletterWelcomeMailCarriesUnsubscribeLink(t *testing.T) {
	a := newAuthHarness(t)
	a.h.SetDevTokenSink(nil)
	sent := email.NewMemorySender()
	q := email.NewQueue(sent, 8)
	q.Start(context.Background(), 1)
	defer q.Stop()
	a.h.SetMailer(q, email.Config{AppName: "I CONFESS", BaseURL: "https://iconfess.app"})
	if code, _ := a.do("POST", "/newsletter", "", map[string]string{"email": "welcome@example.com"}); code != http.StatusAccepted {
		t.Fatalf("subscribe: %d", code)
	}
	if !q.Drain(2 * time.Second) {
		t.Fatal("mail queue did not drain")
	}
	msg, ok := sent.LastTo("welcome@example.com")
	if !ok || !strings.Contains(msg.Text, "https://iconfess.app/newsletter/unsubscribe?token=") {
		t.Fatalf("welcome mail has no unsubscribe link: %+v", msg)
	}
}
