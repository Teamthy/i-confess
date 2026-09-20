package playapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// Tests for the Play Developer API wire protocol.
//
// These pin the parts that only exist on the wire: the URL, the bearer token,
// Google's JSON field names and the HTTP status classification. Billing's tests
// cover what the decoded answer means, so a change to Google's API shows up
// here rather than as a wrong entitlement.

const testPackage = "app.iconfess"

type recorder struct {
	mu       sync.Mutex
	paths    []string
	rawURIs  []string
	auths    []string
	requests int
	status   int
	body     string
}

func (r *recorder) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.paths = append(r.paths, req.URL.Path)
		r.rawURIs = append(r.rawURIs, req.RequestURI)
		r.auths = append(r.auths, req.Header.Get("Authorization"))
		r.requests++
		status, body := r.status, r.body
		r.mu.Unlock()

		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func (r *recorder) snapshot() (paths, rawURIs, auths []string, requests int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...), append([]string(nil), r.rawURIs...),
		append([]string(nil), r.auths...), r.requests
}

func newTestClient(t *testing.T, rec *recorder, mutate func(*Client)) *Client {
	t.Helper()

	srv := httptest.NewServer(rec.handler())
	t.Cleanup(srv.Close)

	client := &Client{
		PackageName: testPackage,
		TokenSource: func(context.Context) (string, error) { return "access-token", nil },
		BaseURL:     srv.URL,
	}
	if mutate != nil {
		mutate(client)
	}
	return client
}

func purchaseJSON(state, productID string, expires time.Time, autoRenew *bool) string {
	item := map[string]any{
		"productId":  productID,
		"expiryTime": expires.UTC().Format(time.RFC3339),
	}
	if autoRenew != nil {
		item["autoRenewingPlan"] = map[string]any{"autoRenewEnabled": *autoRenew}
	}
	payload := map[string]any{
		"kind":                 "androidpublisher#subscriptionPurchaseV2",
		"subscriptionState":    state,
		"acknowledgementState": AckAcknowledged,
		"latestOrderId":        "GPA.3333-4137-0319-36762",
		"lineItems":            []any{item},
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

// The request must go to the documented endpoint, with the token in the path
// and the access token in the header. A client that asks the wrong question
// gets a plausible answer to a different one.
func TestSubscriptionRequestsTheDocumentedEndpoint(t *testing.T) {
	rec := &recorder{body: purchaseJSON(StateActive, "premium_monthly", time.Now().Add(time.Hour), nil)}
	client := newTestClient(t, rec, nil)

	// A token with characters that mean something in a URL: a purchase token is
	// opaque, so interpolating it into a path unescaped lets it change which
	// endpoint is called.
	const token = "token/with odd+chars?and#hash"
	if _, err := client.Subscription(context.Background(), token); err != nil {
		t.Fatalf("fetch: %v", err)
	}

	_, rawURIs, auths, requests := rec.snapshot()
	if requests != 1 {
		t.Fatalf("%d requests, want 1", requests)
	}
	want := "/androidpublisher/v3/applications/" + testPackage +
		"/purchases/subscriptionsv2/tokens/" + url.PathEscape(token)
	if rawURIs[0] != want {
		t.Errorf("request URI = %q, want %q", rawURIs[0], want)
	}
	// The escaping is the point: without it, the token's own "/" and "?" would
	// rewrite the request.
	if strings.Contains(rawURIs[0], "/tokens/token/") || strings.Contains(rawURIs[0], "?and") {
		t.Errorf("the token was interpolated unescaped: %q", rawURIs[0])
	}
	if !strings.Contains(rawURIs[0], "token%2Fwith") {
		t.Errorf("the token's slash was not escaped: %q", rawURIs[0])
	}
	if auths[0] != "Bearer access-token" {
		t.Errorf("authorization = %q", auths[0])
	}
}

func TestSubscriptionDecodesTheResponse(t *testing.T) {
	autoRenew := true
	expires := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)
	rec := &recorder{body: purchaseJSON(StateInGracePeriod, "premium_annual", expires, &autoRenew)}
	client := newTestClient(t, rec, nil)

	got, err := client.Subscription(context.Background(), "tok")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got.State != StateInGracePeriod {
		t.Errorf("state = %q", got.State)
	}
	if got.AcknowledgementState != AckAcknowledged {
		t.Errorf("ack = %q", got.AcknowledgementState)
	}
	if got.LatestOrderID == "" || got.LinkedPurchaseToken != "" {
		t.Errorf("order/linked ids decoded wrong: %+v", got)
	}
	if len(got.LineItems) != 1 {
		t.Fatalf("line items = %d, want 1", len(got.LineItems))
	}
	item := got.LineItems[0]
	if item.ProductID != "premium_annual" {
		t.Errorf("product = %q", item.ProductID)
	}
	if !item.ExpiryTime.Equal(expires) {
		t.Errorf("expiry = %v, want %v", item.ExpiryTime, expires)
	}
	if !item.AutoRenewSet || !item.AutoRenew {
		t.Errorf("auto-renew = %+v, want set and true", item)
	}
}

// A prepaid plan has no autoRenewingPlan at all. Absent must stay absent: false
// would read as "cancelled".
func TestSubscriptionDistinguishesAbsentAutoRenew(t *testing.T) {
	rec := &recorder{body: purchaseJSON(StateActive, "prepaid_plan", time.Now().Add(time.Hour), nil)}
	client := newTestClient(t, rec, nil)

	got, err := client.Subscription(context.Background(), "tok")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got.LineItems[0].AutoRenewSet {
		t.Error("absent autoRenewingPlan was reported as set")
	}
}

// Google's timestamps may carry fractional seconds and non-Z offsets. Both are
// valid RFC 3339 and must not be rejected.
func TestSubscriptionParsesGoogleTimestamps(t *testing.T) {
	body := `{"subscriptionState":"` + StateActive + `","lineItems":[` +
		`{"productId":"a","expiryTime":"2026-10-20T09:00:00.123Z"},` +
		`{"productId":"b","expiryTime":"2026-10-20T10:00:00+01:00"}]}`
	rec := &recorder{body: body}
	client := newTestClient(t, rec, nil)

	got, err := client.Subscription(context.Background(), "tok")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got.LineItems) != 2 {
		t.Fatalf("line items = %d", len(got.LineItems))
	}
	if got.LineItems[0].ExpiryTime.IsZero() || got.LineItems[1].ExpiryTime.IsZero() {
		t.Error("a valid timestamp was not parsed")
	}
	if !got.LineItems[1].ExpiryTime.Equal(time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("offset timestamp parsed as %v, want 09:00Z", got.LineItems[1].ExpiryTime)
	}
}

// A malformed expiry must be an error, not a silent zero time: the zero time is
// in the past, so it would read as "expired" and quietly revoke a paying user.
func TestSubscriptionRejectsAnUnparsableExpiry(t *testing.T) {
	body := `{"subscriptionState":"` + StateActive + `","lineItems":[{"productId":"a","expiryTime":"next tuesday"}]}`
	client := newTestClient(t, &recorder{body: body}, nil)

	if _, err := client.Subscription(context.Background(), "tok"); err == nil {
		t.Fatal("an unparsable expiry was accepted")
	} else if !errors.Is(err, ErrUnavailable) {
		t.Errorf("error = %v, want ErrUnavailable", err)
	}
}

// Status classification decides whether a customer is told "your purchase is
// bad" or an operator is told the deployment is broken.
func TestSubscriptionClassifiesHTTPStatus(t *testing.T) {
	cases := map[int]error{
		http.StatusBadRequest:          ErrTokenInvalid,
		http.StatusNotFound:            ErrTokenInvalid,
		http.StatusGone:                ErrTokenInvalid,
		http.StatusUnauthorized:        ErrUnauthorized,
		http.StatusForbidden:           ErrUnauthorized,
		http.StatusTooManyRequests:     ErrUnavailable,
		http.StatusInternalServerError: ErrUnavailable,
		http.StatusBadGateway:          ErrUnavailable,
		http.StatusServiceUnavailable:  ErrUnavailable,
	}
	for status, want := range cases {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := newTestClient(t, &recorder{status: status, body: `{"error":{"message":"nope"}}`}, nil)
			_, err := client.Subscription(context.Background(), "tok")
			if !errors.Is(err, want) {
				t.Fatalf("status %d: error = %v, want %v", status, err, want)
			}
		})
	}
}

// Without credentials no request may be made, and nothing may be granted.
func TestSubscriptionDoesNotCallTheAPIFailureWithoutCredentials(t *testing.T) {
	rec := &recorder{body: purchaseJSON(StateActive, "premium_monthly", time.Now().Add(time.Hour), nil)}
	client := newTestClient(t, rec, func(c *Client) {
		c.TokenSource = func(context.Context) (string, error) {
			return "", errors.New("service account missing")
		}
	})

	_, err := client.Subscription(context.Background(), "tok")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v, want ErrUnauthorized", err)
	}
	if _, _, _, requests := rec.snapshot(); requests != 0 {
		t.Errorf("%d requests were made without a token", requests)
	}
}

func TestSubscriptionRejectsAnEmptyTokenWithoutCallingTheAPI(t *testing.T) {
	rec := &recorder{}
	client := newTestClient(t, rec, nil)

	if _, err := client.Subscription(context.Background(), "  "); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("error = %v, want ErrTokenInvalid", err)
	}
	if _, _, _, requests := rec.snapshot(); requests != 0 {
		t.Errorf("%d requests were made for an empty token", requests)
	}
}

func TestSubscriptionReportsATestPurchase(t *testing.T) {
	body := `{"subscriptionState":"` + StateActive + `","testPurchase":{},` +
		`"lineItems":[{"productId":"a","expiryTime":"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `"}]}`
	client := newTestClient(t, &recorder{body: body}, nil)

	got, err := client.Subscription(context.Background(), "tok")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !got.TestPurchase {
		t.Error("a licensed-tester purchase was not reported as a test purchase")
	}
}

// An upstream error body is echoed into the message, so it must be bounded and
// single-line: it lands in logs and in an operator's terminal.
func TestSubscriptionTruncatesTheUpstreamErrorBody(t *testing.T) {
	client := newTestClient(t, &recorder{
		status: http.StatusBadGateway,
		body:   strings.Repeat("x", 5000) + "\nsecond line",
	}, nil)

	_, err := client.Subscription(context.Background(), "tok")
	if err == nil {
		t.Fatal("expected an error")
	}
	if len(err.Error()) > 400 {
		t.Errorf("error message is %d bytes, want it bounded", len(err.Error()))
	}
	if strings.Contains(err.Error(), "\n") {
		t.Error("error message contains a newline")
	}
}
