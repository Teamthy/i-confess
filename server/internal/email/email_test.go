package email

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		AppName: "i-confess", BaseURL: "https://iconfess.app",
		FromAddress: "no-reply@iconfess.app", SupportAddress: "support@iconfess.app",
	}
}

// The token must reach the user only through the link, and must not appear in
// the subject where it would be visible in notifications and mail lists.
func TestVerificationMessageCarriesTokenOnlyInLink(t *testing.T) {
	const token = "abc123-secret-token"
	msg := testConfig().VerificationMessage("user@example.com", token)

	if strings.Contains(msg.Subject, token) {
		t.Fatalf("token leaked into subject: %q", msg.Subject)
	}
	if !strings.Contains(msg.Text, "https://iconfess.app/verify-email?token="+token) {
		t.Fatalf("verification link missing from text body:\n%s", msg.Text)
	}
	if !strings.Contains(msg.HTML, token) {
		t.Fatal("verification link missing from html body")
	}
	if msg.Tag != "email_verification" {
		t.Fatalf("tag = %q", msg.Tag)
	}
}

func TestPasswordResetMessageStatesExpiryAndNoOpPath(t *testing.T) {
	msg := testConfig().PasswordResetMessage("user@example.com", "tok")

	// A user who did not request this must be told they need do nothing —
	// otherwise the mail itself provokes a panicked password change.
	if !strings.Contains(strings.ToLower(msg.Text), "did not request") {
		t.Fatalf("reset mail lacks a no-op reassurance:\n%s", msg.Text)
	}
	if !strings.Contains(msg.Text, "expires") {
		t.Fatal("reset mail does not state an expiry")
	}
}

// A security alert must not contain an actionable link: that is what makes
// security mail a phishing template.
func TestSecurityAlertCarriesNoActionLink(t *testing.T) {
	msg := testConfig().SecurityAlertMessage("user@example.com",
		"Your password was changed", "This happened just now.")

	if strings.Contains(msg.Text, "token=") {
		t.Fatal("security alert contains a token")
	}
	if strings.Contains(msg.HTML, "/reset-password") || strings.Contains(msg.HTML, "/verify-email") {
		t.Fatal("security alert contains an action link")
	}
}

// Interpolated values must be escaped: templates outlive the assumption that
// their inputs are trusted.
func TestHTMLIsEscaped(t *testing.T) {
	c := testConfig()
	c.AppName = `Evil<script>alert(1)</script>`
	msg := c.VerificationMessage("user@example.com", "tok")

	if strings.Contains(msg.HTML, "<script>") {
		t.Fatalf("unescaped markup reached the html body:\n%s", msg.HTML)
	}
}

func TestMaskAddress(t *testing.T) {
	cases := map[string]string{
		"grace@example.com": "g***@example.com",
		"a@b.co":            "***",
		"":                  "***",
	}
	for in, want := range cases {
		if got := MaskAddress(in); got != want {
			t.Fatalf("MaskAddress(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Postmark adapter
// ---------------------------------------------------------------------------

func TestPostmarkSendsTransactionalStream(t *testing.T) {
	var gotToken, gotStream, gotTo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Postmark-Server-Token")
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		s := string(body)
		if strings.Contains(s, `"MessageStream":"outbound"`) {
			gotStream = "outbound"
		}
		if strings.Contains(s, "user@example.com") {
			gotTo = "user@example.com"
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewPostmark("tok-123", "no-reply@iconfess.app")
	p.BaseURL = srv.URL

	if err := p.Send(context.Background(), Message{
		To: "user@example.com", Subject: "s", Text: "t", Tag: "email_verification",
	}); err != nil {
		t.Fatal(err)
	}
	if gotToken != "tok-123" {
		t.Fatalf("token header = %q", gotToken)
	}
	// Authentication mail must not share a stream with broadcast sends.
	if gotStream != "outbound" {
		t.Fatal("message was not sent on the transactional stream")
	}
	if gotTo == "" {
		t.Fatal("recipient missing from request")
	}
}

func TestPostmarkErrorClassification(t *testing.T) {
	cases := []struct {
		status    int
		retryable bool
	}{
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusUnprocessableEntity, false},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", tc.status)
		}))
		p := NewPostmark("tok", "from@x.com")
		p.BaseURL = srv.URL
		err := p.Send(context.Background(), Message{To: "u@example.com", Text: "t"})
		srv.Close()

		if err == nil {
			t.Fatalf("status %d: expected an error", tc.status)
		}
		if IsRetryable(err) != tc.retryable {
			t.Fatalf("status %d: retryable = %v, want %v", tc.status, IsRetryable(err), tc.retryable)
		}
	}
}

func TestPostmarkRejectsMalformedAddressWithoutCallingOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("provider must not be contacted for a malformed address")
	}))
	defer srv.Close()

	p := NewPostmark("tok", "from@x.com")
	p.BaseURL = srv.URL

	for _, bad := range []string{"", "nope", "a@", "@b.com", "a b@c.com", "two@@at.com", "no-dot@localhost"} {
		err := p.Send(context.Background(), Message{To: bad, Text: "t"})
		if err == nil {
			t.Fatalf("malformed address accepted: %q", bad)
		}
		if IsRetryable(err) {
			t.Fatalf("malformed address %q classified retryable", bad)
		}
	}
}

// ---------------------------------------------------------------------------
// Queue
// ---------------------------------------------------------------------------

func TestQueueDeliversAsynchronously(t *testing.T) {
	sender := NewMemorySender()
	q := NewQueue(sender, 16)
	q.Start(context.Background(), 2)
	defer q.Stop()

	q.Enqueue(testConfig().VerificationMessage("a@example.com", "tok-a"))
	q.Enqueue(testConfig().PasswordResetMessage("b@example.com", "tok-b"))

	if !q.Drain(2 * time.Second) {
		t.Fatal("queue did not drain")
	}
	if n := len(sender.Sent()); n != 2 {
		t.Fatalf("sent %d messages, want 2", n)
	}
	if _, ok := sender.LastTo("a@example.com"); !ok {
		t.Fatal("verification message not delivered")
	}
}

// Enqueue must never block a request handler, even when nothing is draining.
func TestEnqueueNeverBlocks(t *testing.T) {
	sender := NewMemorySender()
	q := NewQueue(sender, 2) // deliberately tiny, no workers started

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			q.Enqueue(Message{To: "u@example.com", Text: "t"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue blocked when the buffer was full")
	}

	m := q.Metrics()
	if m.Dropped == 0 {
		t.Fatal("expected overflow to be counted as dropped")
	}
}

// A transient failure is retried; the message eventually lands.
func TestQueueRetriesTransientFailures(t *testing.T) {
	flaky := &flakySender{failFirst: 2}
	q := NewQueue(flaky, 8)
	q.MaxAttempts = 5
	q.Backoff = func(int) time.Duration { return 5 * time.Millisecond }
	q.Start(context.Background(), 1)
	defer q.Stop()

	q.Enqueue(Message{To: "u@example.com", Text: "t", Tag: "test"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if flaky.succeeded() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("message never delivered after retries (attempts: %d)", flaky.calls())
}

// A permanent failure must not be retried: repeated hard bounces damage sender
// reputation for every other user.
func TestQueueDoesNotRetryPermanentFailures(t *testing.T) {
	perm := &flakySender{alwaysPermanent: true}
	q := NewQueue(perm, 8)
	q.Backoff = func(int) time.Duration { return time.Millisecond }
	q.Start(context.Background(), 1)
	defer q.Stop()

	q.Enqueue(Message{To: "u@example.com", Text: "t"})
	time.Sleep(200 * time.Millisecond)

	if n := perm.calls(); n != 1 {
		t.Fatalf("permanent failure attempted %d times, want 1", n)
	}
	if q.Metrics().Failed != 1 {
		t.Fatalf("permanent failure not counted: %+v", q.Metrics())
	}
}

// Retries are bounded, or a permanently broken provider produces an infinite
// loop of attempts.
func TestQueueGivesUpAfterMaxAttempts(t *testing.T) {
	always := &flakySender{failFirst: 1 << 30}
	q := NewQueue(always, 8)
	q.MaxAttempts = 3
	q.Backoff = func(int) time.Duration { return time.Millisecond }
	q.Start(context.Background(), 1)
	defer q.Stop()

	q.Enqueue(Message{To: "u@example.com", Text: "t"})
	time.Sleep(400 * time.Millisecond)

	if n := always.calls(); n > 3 {
		t.Fatalf("attempted %d times, want at most MaxAttempts=3", n)
	}
	if q.Metrics().Discarded == 0 {
		t.Fatalf("give-up not counted: %+v", q.Metrics())
	}
}

func TestStopIsIdempotent(t *testing.T) {
	q := NewQueue(NewMemorySender(), 4)
	q.Start(context.Background(), 1)
	q.Stop()
	q.Stop() // must not panic
}

// flakySender fails a set number of times, then succeeds.
type flakySender struct {
	mu              sync.Mutex
	n               int
	failFirst       int
	alwaysPermanent bool
	ok              bool
}

func (f *flakySender) Name() string { return "flaky" }

func (f *flakySender) Send(_ context.Context, _ Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	if f.alwaysPermanent {
		return Permanent("bad address")
	}
	if f.n <= f.failFirst {
		return Retryable("upstream 503")
	}
	f.ok = true
	return nil
}

func (f *flakySender) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

func (f *flakySender) succeeded() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ok
}
