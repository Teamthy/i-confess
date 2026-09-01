package push

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// APNs
// ---------------------------------------------------------------------------

// APNs delivers through Apple Push Notification service over HTTP/2.
//
// Token-based authentication (a signed JWT) rather than certificates: one key
// serves every app and environment, and it does not expire annually, so
// nobody has to remember to rotate a .p12 before notifications silently stop.
type APNs struct {
	// TeamID and KeyID identify the signing key in the Apple developer account.
	TeamID string
	KeyID  string
	// Topic is the app bundle id.
	Topic string
	// key is the parsed .p8 private key.
	key *ecdsa.PrivateKey

	BaseURL string
	Client  *http.Client

	mu       sync.Mutex
	cached   string
	cachedAt time.Time
}

// NewAPNs parses a .p8 signing key and builds a client.
func NewAPNs(p8PEM, teamID, keyID, topic string, production bool) (*APNs, error) {
	block, _ := pem.Decode([]byte(p8PEM))
	if block == nil {
		return nil, fmt.Errorf("apns: key is not valid PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("apns: parse key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("apns: key is not ECDSA")
	}

	base := "https://api.sandbox.push.apple.com"
	if production {
		base = "https://api.push.apple.com"
	}
	return &APNs{
		TeamID: teamID, KeyID: keyID, Topic: topic, key: key,
		BaseURL: base,
		Client:  &http.Client{Timeout: 20 * time.Second},
	}, nil
}

// Name implements Sender.
func (a *APNs) Name() string { return ProviderAPNs }

// authToken returns a cached provider JWT.
//
// Apple rejects tokens refreshed more than once per 20 minutes and expires
// them after 60, so it is regenerated on a 45-minute cycle: comfortably inside
// the expiry, comfortably outside the rate limit.
func (a *APNs) authToken() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cached != "" && time.Since(a.cachedAt) < 45*time.Minute {
		return a.cached, nil
	}

	header := base64URL(fmt.Sprintf(`{"alg":"ES256","kid":%q}`, a.KeyID))
	claims := base64URL(fmt.Sprintf(`{"iss":%q,"iat":%d}`, a.TeamID, time.Now().Unix()))
	signingInput := header + "." + claims

	sig, err := signES256(a.key, signingInput)
	if err != nil {
		return "", err
	}
	a.cached = signingInput + "." + sig
	a.cachedAt = time.Now()
	return a.cached, nil
}

type apnsPayload struct {
	APS  apnsAPS           `json:"aps"`
	Data map[string]string `json:"data,omitempty"`
}

type apnsAPS struct {
	Alert apnsAlert `json:"alert"`
	Sound string    `json:"sound,omitempty"`
	// ThreadID groups a user's reminders in Notification Center rather than
	// stacking them as unrelated alerts.
	ThreadID string `json:"thread-id,omitempty"`
}

type apnsAlert struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Send implements Sender.
func (a *APNs) Send(ctx context.Context, n Notification) error {
	if n.Token == "" {
		return Permanent("empty device token")
	}
	token, err := a.authToken()
	if err != nil {
		return Retryable("build auth token: %v", err)
	}

	sound := n.Sound
	if sound == "" {
		sound = "default"
	}
	body, err := json.Marshal(apnsPayload{
		APS: apnsAPS{
			Alert:    apnsAlert{Title: n.Title, Body: n.Body},
			Sound:    sound,
			ThreadID: n.CollapseKey,
		},
		Data: n.Data,
	})
	if err != nil {
		return Permanent("encode payload: %v", err)
	}

	url := strings.TrimRight(a.BaseURL, "/") + "/3/device/" + n.Token
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Permanent("build request: %v", err)
	}
	req.Header.Set("authorization", "bearer "+token)
	req.Header.Set("apns-topic", a.Topic)
	req.Header.Set("apns-push-type", "alert")
	// A reminder for a 6 AM session is worthless at 9 AM, so it expires rather
	// than being stored and delivered late.
	req.Header.Set("apns-expiration", fmt.Sprintf("%d", time.Now().Add(2*time.Hour).Unix()))
	if n.CollapseKey != "" {
		req.Header.Set("apns-collapse-id", truncate(n.CollapseKey, 64))
	}

	res, err := a.Client.Do(req)
	if err != nil {
		return Retryable("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		return nil
	}
	snippet, _ := io.ReadAll(io.LimitReader(res.Body, 512))
	reason := apnsReason(snippet)

	switch {
	// Apple reports a dead device as 410 Gone, or 400 with BadDeviceToken.
	case res.StatusCode == http.StatusGone,
		reason == "BadDeviceToken", reason == "Unregistered":
		return InvalidToken("apns rejected the token: %s", reason)
	case res.StatusCode == http.StatusTooManyRequests, res.StatusCode >= 500:
		return Retryable("apns returned %d: %s", res.StatusCode, reason)
	case res.StatusCode == http.StatusForbidden:
		// Almost always a bad signing key or wrong team id: an operator has to
		// fix it, so retrying only hides the cause.
		return Permanent("apns rejected credentials: %s", reason)
	default:
		return Permanent("apns returned %d: %s", res.StatusCode, reason)
	}
}

func apnsReason(body []byte) string {
	var r struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &r); err == nil && r.Reason != "" {
		return r.Reason
	}
	return strings.TrimSpace(string(body))
}

// signES256 produces a JWS signature over the signing input.
func signES256(key *ecdsa.PrivateKey, signingInput string) (string, error) {
	digest := sha256Sum([]byte(signingInput))
	r, s, err := ecdsaSign(key, digest)
	if err != nil {
		return "", err
	}
	// JWS wants fixed-width big-endian r||s, not the ASN.1 form crypto/ecdsa
	// marshals by default. Getting this wrong yields a 403 that looks like a
	// credential problem.
	keyBytes := (key.Curve.Params().BitSize + 7) / 8
	out := make([]byte, 2*keyBytes)
	copyRightAligned(out[:keyBytes], r)
	copyRightAligned(out[keyBytes:], s)
	return base64.RawURLEncoding.EncodeToString(out), nil
}

func copyRightAligned(dst []byte, v *big.Int) {
	b := v.Bytes()
	copy(dst[len(dst)-len(b):], b)
}

func base64URL(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

// ---------------------------------------------------------------------------
// FCM
// ---------------------------------------------------------------------------

// FCM delivers through Firebase Cloud Messaging HTTP v1.
type FCM struct {
	ProjectID string
	// AccessToken supplies a short-lived OAuth2 token. Injected rather than
	// minted here so credential handling stays in one place and tests do not
	// need a Google service account.
	AccessToken func(ctx context.Context) (string, error)
	BaseURL     string
	Client      *http.Client
}

// NewFCM builds a client.
func NewFCM(projectID string, tokenSource func(ctx context.Context) (string, error)) *FCM {
	return &FCM{
		ProjectID: projectID, AccessToken: tokenSource,
		BaseURL: "https://fcm.googleapis.com",
		Client:  &http.Client{Timeout: 20 * time.Second},
	}
}

// Name implements Sender.
func (f *FCM) Name() string { return ProviderFCM }

type fcmRequest struct {
	Message fcmMessage `json:"message"`
}

type fcmMessage struct {
	Token        string            `json:"token"`
	Notification fcmNotification   `json:"notification"`
	Data         map[string]string `json:"data,omitempty"`
	Android      *fcmAndroid       `json:"android,omitempty"`
}

type fcmNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type fcmAndroid struct {
	CollapseKey string `json:"collapse_key,omitempty"`
	Priority    string `json:"priority,omitempty"`
	TTL         string `json:"ttl,omitempty"`
}

// Send implements Sender.
func (f *FCM) Send(ctx context.Context, n Notification) error {
	if n.Token == "" {
		return Permanent("empty device token")
	}
	if f.AccessToken == nil {
		return Permanent("fcm credentials are not configured")
	}
	token, err := f.AccessToken(ctx)
	if err != nil {
		return Retryable("obtain access token: %v", err)
	}

	body, err := json.Marshal(fcmRequest{Message: fcmMessage{
		Token:        n.Token,
		Notification: fcmNotification{Title: n.Title, Body: n.Body},
		Data:         n.Data,
		Android: &fcmAndroid{
			CollapseKey: n.CollapseKey,
			// A devotional reminder is time-sensitive; delivering it hours
			// late is worse than not delivering it.
			Priority: "high",
			TTL:      "7200s",
		},
	}})
	if err != nil {
		return Permanent("encode payload: %v", err)
	}

	url := fmt.Sprintf("%s/v1/projects/%s/messages:send",
		strings.TrimRight(f.BaseURL, "/"), f.ProjectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Permanent("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res, err := f.Client.Do(req)
	if err != nil {
		return Retryable("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		return nil
	}
	snippet, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
	msg := strings.TrimSpace(string(snippet))

	switch {
	// FCM reports a dead token as 404 UNREGISTERED, or 400 INVALID_ARGUMENT
	// when the token is malformed.
	case res.StatusCode == http.StatusNotFound,
		strings.Contains(msg, "UNREGISTERED"),
		strings.Contains(msg, "INVALID_ARGUMENT") && strings.Contains(msg, "token"):
		return InvalidToken("fcm rejected the token")
	case res.StatusCode == http.StatusTooManyRequests, res.StatusCode >= 500:
		return Retryable("fcm returned %d", res.StatusCode)
	case res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden:
		return Permanent("fcm rejected credentials (%d)", res.StatusCode)
	default:
		return Permanent("fcm returned %d: %s", res.StatusCode, truncate(msg, 200))
	}
}

// ---------------------------------------------------------------------------
// Development and test senders
// ---------------------------------------------------------------------------

// LogSender prints notifications instead of delivering them.
type LogSender struct{}

// Name implements Sender.
func (LogSender) Name() string { return "log" }

// Send implements Sender.
func (LogSender) Send(_ context.Context, n Notification) error {
	log.Printf("push(dev) platform=%s token=%s title=%q body=%q data=%v",
		n.Platform, maskToken(n.Token), n.Title, n.Body, n.Data)
	return nil
}

// MemorySender captures notifications for assertions.
type MemorySender struct {
	mu   sync.Mutex
	sent []Notification
	// Err, when set, is returned by every Send.
	Err error
}

// NewMemorySender creates a capturing sender.
func NewMemorySender() *MemorySender { return &MemorySender{} }

// Name implements Sender.
func (m *MemorySender) Name() string { return "memory" }

// Send implements Sender.
func (m *MemorySender) Send(_ context.Context, n Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	m.sent = append(m.sent, n)
	return nil
}

// Sent returns a copy of captured notifications.
func (m *MemorySender) Sent() []Notification {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Notification, len(m.sent))
	copy(out, m.sent)
	return out
}

// Reset clears captured notifications.
func (m *MemorySender) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = nil
}

// maskToken redacts a device token for logs (§70).
func maskToken(t string) string {
	if len(t) <= 8 {
		return "***"
	}
	return t[:4] + "***" + t[len(t)-4:]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
