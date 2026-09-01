package email

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Postmark
// ---------------------------------------------------------------------------

// Postmark delivers through the Postmark transactional API.
//
// Chosen as the concrete adapter because transactional-only providers keep
// authentication mail out of the same reputation pool as marketing sends, which
// is what keeps verification emails landing in inboxes. The Sender interface
// means swapping to SES or Resend is one file.
type Postmark struct {
	Token   string
	From    string
	BaseURL string
	Client  *http.Client
}

// NewPostmark builds an adapter with production-sane defaults.
func NewPostmark(token, from string) *Postmark {
	return &Postmark{
		Token: token, From: from,
		BaseURL: "https://api.postmarkapp.com",
		// Short: a worker retries, so a slow provider should free the goroutine
		// rather than hold it for minutes.
		Client: &http.Client{Timeout: 20 * time.Second},
	}
}

// Name implements Sender.
func (p *Postmark) Name() string { return "postmark" }

type postmarkRequest struct {
	From          string `json:"From"`
	To            string `json:"To"`
	Subject       string `json:"Subject"`
	TextBody      string `json:"TextBody"`
	HTMLBody      string `json:"HtmlBody,omitempty"`
	MessageStream string `json:"MessageStream"`
	Tag           string `json:"Tag,omitempty"`
}

// Send implements Sender.
func (p *Postmark) Send(ctx context.Context, msg Message) error {
	if p.Token == "" {
		return Permanent("postmark token is not configured")
	}
	if !plausibleAddress(msg.To) {
		// Malformed addresses are permanent: retrying cannot fix them, and
		// repeated hard failures damage sender reputation.
		return Permanent("refusing to send to malformed address")
	}

	body, err := json.Marshal(postmarkRequest{
		From: p.From, To: msg.To, Subject: msg.Subject,
		TextBody: msg.Text, HTMLBody: msg.HTML,
		// Transactional stream: authentication mail must never share a stream
		// with broadcast sends.
		MessageStream: "outbound",
		Tag:           msg.Tag,
	})
	if err != nil {
		return Permanent("encode request: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(p.BaseURL, "/")+"/email", bytes.NewReader(body))
	if err != nil {
		return Permanent("build request: %v", err)
	}
	req.Header.Set("X-Postmark-Server-Token", p.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return Retryable("provider request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		return nil
	}
	// Bounded read: the body may be large or hostile, and it can echo the
	// message content, so it is never logged verbatim by callers.
	snippet, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
	switch {
	case res.StatusCode == http.StatusTooManyRequests, res.StatusCode >= 500:
		return Retryable("provider returned %d", res.StatusCode)
	case res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden:
		// Credential problems need an operator, not another attempt.
		return Permanent("provider rejected credentials (%d)", res.StatusCode)
	default:
		return Permanent("provider returned %d: %s", res.StatusCode, strings.TrimSpace(string(snippet)))
	}
}

// plausibleAddress is a cheap sanity check, not RFC 5322 validation. Real
// validation is delivery: an address either accepts mail or it does not.
func plausibleAddress(addr string) bool {
	addr = strings.TrimSpace(addr)
	if len(addr) < 3 || len(addr) > 320 {
		return false
	}
	at := strings.IndexByte(addr, '@')
	if at <= 0 || at == len(addr)-1 {
		return false
	}
	if strings.Count(addr, "@") != 1 {
		return false
	}
	if strings.ContainsAny(addr, " \t\r\n<>\",") {
		return false
	}
	return strings.Contains(addr[at:], ".")
}

// ---------------------------------------------------------------------------
// Log sender (development)
// ---------------------------------------------------------------------------

// LogSender writes messages to the log instead of delivering them.
//
// It exists so the full flow is exercisable without a provider account. It logs
// the link because a developer needs it to complete the flow locally — which is
// exactly why it must never be selected in production, and Config.Validate
// refuses to start if it is.
type LogSender struct{}

// Name implements Sender.
func (LogSender) Name() string { return "log" }

// Send implements Sender.
func (LogSender) Send(_ context.Context, msg Message) error {
	log.Printf("email(dev) to=%s tag=%s subject=%q\n%s",
		MaskAddress(msg.To), msg.Tag, msg.Subject, msg.Text)
	return nil
}

// ---------------------------------------------------------------------------
// Memory sender (tests)
// ---------------------------------------------------------------------------

// MemorySender captures messages for assertions.
type MemorySender struct {
	mu   sync.Mutex
	sent []Message
	// Err, when set, is returned by every Send.
	Err error
}

// NewMemorySender creates a capturing sender.
func NewMemorySender() *MemorySender { return &MemorySender{} }

// Name implements Sender.
func (m *MemorySender) Name() string { return "memory" }

// Send implements Sender.
func (m *MemorySender) Send(_ context.Context, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	m.sent = append(m.sent, msg)
	return nil
}

// Sent returns a copy of captured messages.
func (m *MemorySender) Sent() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Message, len(m.sent))
	copy(out, m.sent)
	return out
}

// LastTo returns the most recent message sent to an address.
func (m *MemorySender) LastTo(addr string) (Message, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.sent) - 1; i >= 0; i-- {
		if m.sent[i].To == addr {
			return m.sent[i], true
		}
	}
	return Message{}, false
}

// Reset clears captured messages.
func (m *MemorySender) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = nil
}
