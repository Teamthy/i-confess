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
