// Package playapi is an outbound client for the Google Play Developer API.
//
// It exists because internal/billing must not import net/http: the arch test
// states the rule, and the reason behind it holds here — the decision about
// what a subscription state entitles is domain logic, and a rule expressed
// through an HTTP client cannot be reused by a job that has no client at all.
// This package owns the wire: URLs, headers, Google's JSON and Google's state
// vocabulary. Billing owns what those states mean.
//
// The API is purchases.subscriptionsv2.get, the successor to
// purchases.subscriptions.get (deprecated 2025-05-21, shut down 2027-08-31).
package playapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BaseURL is the Play Developer API root.
const BaseURL = "https://androidpublisher.googleapis.com"

// SubscriptionState values, as they appear on the wire.
//
// The two that are easy to get wrong:
//
//   - StateCanceled means "cancelled, but not expired yet". The user cancelled
//     and keeps access until the line item's expiry. Reading it as expired
//     revokes access the customer paid for.
//   - StateInGracePeriod means "payment failed, Google is retrying". The expiry
//     is extended into the future, so entitlement continues.
const (
	StateActive        = "SUBSCRIPTION_STATE_ACTIVE"
	StateInGracePeriod = "SUBSCRIPTION_STATE_IN_GRACE_PERIOD"
	StateOnHold        = "SUBSCRIPTION_STATE_ON_HOLD"
	StatePaused        = "SUBSCRIPTION_STATE_PAUSED"
	StateCanceled      = "SUBSCRIPTION_STATE_CANCELED"
	StateExpired       = "SUBSCRIPTION_STATE_EXPIRED"
	StatePending       = "SUBSCRIPTION_STATE_PENDING"
	StateUnspecified   = "SUBSCRIPTION_STATE_UNSPECIFIED"
)

// Acknowledgement states. Play refunds a purchase that is not acknowledged
// within three days, so this is the difference between taking money and keeping
// it.
const (
	AckPending      = "ACKNOWLEDGEMENT_STATE_PENDING"
	AckAcknowledged = "ACKNOWLEDGEMENT_STATE_ACKNOWLEDGED"
	AckUnspecified  = "ACKNOWLEDGEMENT_STATE_UNSPECIFIED"
)

// Errors a caller must distinguish: a bad purchase token is the user's problem,
// the other two are ours.
var (
	// ErrTokenInvalid means Google does not recognise the token: unknown,
	// malformed, or older than the 60 days a token stays queryable after
	// expiry.
	ErrTokenInvalid = errors.New("play: purchase token is not valid")
	// ErrUnauthorized means our service account is not allowed to read
	// purchases: not linked in the Play Console, or the API is not enabled.
	ErrUnauthorized = errors.New("play: credentials rejected")
	// ErrUnavailable means a transient upstream fault.
	ErrUnavailable = errors.New("play: api unavailable")
)

// LineItem is one purchased subscription. An upgrade can leave two line items
// in the response, which is why the expiry is per item rather than one field on
// the purchase.
type LineItem struct {
	ProductID string
	// ExpiryTime is zero when Google sent no expiry for the item.
	ExpiryTime   time.Time
	AutoRenewSet bool
	AutoRenew    bool
}

// Subscription is the subset of SubscriptionPurchaseV2 this server acts on.
type Subscription struct {
	State                string
	AcknowledgementState string
	LatestOrderID        string
	LinkedPurchaseToken  string
	LineItems            []LineItem
	// TestPurchase marks a licensed-tester purchase.
	TestPurchase bool
}

// Client talks to the Play Developer API.
type Client struct {
	// PackageName is the applicationId.
	PackageName string
	// TokenSource returns an OAuth2 access token with the androidpublisher
	// scope. Required.
	TokenSource func(ctx context.Context) (string, error)
	// BaseURL overrides the API root. Tests point this at an httptest server.
	BaseURL string
	// HTTPClient is optional; a bounded default is used otherwise.
	HTTPClient *http.Client
}

func (c *Client) baseURL() string {
	if strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return BaseURL
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// rawSubscription mirrors the JSON. ExpiryTime stays a string so a malformed
// timestamp is reported as a malformed timestamp rather than silently becoming
// the zero time, which would read as "expired".
type rawSubscription struct {
	Kind                 string        `json:"kind"`
	SubscriptionState    string        `json:"subscriptionState"`
	AcknowledgementState string        `json:"acknowledgementState"`
	LatestOrderID        string        `json:"latestOrderId"`
	LinkedPurchaseToken  string        `json:"linkedPurchaseToken"`
	RegionCode           string        `json:"regionCode"`
	TestPurchase         *struct{}     `json:"testPurchase"`
	LineItems            []rawLineItem `json:"lineItems"`
}

type rawLineItem struct {
	ProductID        string `json:"productId"`
	ExpiryTime       string `json:"expiryTime"`
	AutoRenewingPlan *struct {
		AutoRenewEnabled *bool `json:"autoRenewEnabled"`
	} `json:"autoRenewingPlan"`
	PrepaidPlan *struct {
		AllowExtendAfterTime string `json:"allowExtendAfterTime"`
	} `json:"prepaidPlan"`
}

// Subscription fetches the current state of a purchase token.
func (c *Client) Subscription(ctx context.Context, purchaseToken string) (*Subscription, error) {
	purchaseToken = strings.TrimSpace(purchaseToken)
	if purchaseToken == "" {
		return nil, fmt.Errorf("%w: empty purchase token", ErrTokenInvalid)
	}
	if c.TokenSource == nil {
		return nil, fmt.Errorf("%w: no token source configured", ErrUnauthorized)
	}

	accessToken, err := c.TokenSource(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: obtain credentials: %v", ErrUnauthorized, err)
	}
	if accessToken == "" {
		return nil, fmt.Errorf("%w: empty access token", ErrUnauthorized)
	}

	endpoint := fmt.Sprintf("%s/androidpublisher/v3/applications/%s/purchases/subscriptionsv2/tokens/%s",
		c.baseURL(), url.PathEscape(c.PackageName), url.PathEscape(purchaseToken))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("play: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = res.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))

	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusBadRequest, http.StatusNotFound, http.StatusGone:
		return nil, fmt.Errorf("%w: Play Developer API rejected the token (%d): %s",
			ErrTokenInvalid, res.StatusCode, snippet(body))
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("%w: Play Developer API returned %d: %s",
			ErrUnauthorized, res.StatusCode, snippet(body))
	default:
		return nil, fmt.Errorf("%w: Play Developer API returned %d: %s",
			ErrUnavailable, res.StatusCode, snippet(body))
	}

	var raw rawSubscription
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", ErrUnavailable, err)
	}

	out := &Subscription{
		State:                raw.SubscriptionState,
		AcknowledgementState: raw.AcknowledgementState,
		LatestOrderID:        raw.LatestOrderID,
		LinkedPurchaseToken:  raw.LinkedPurchaseToken,
		TestPurchase:         raw.TestPurchase != nil,
	}
	for _, item := range raw.LineItems {
		line := LineItem{ProductID: item.ProductID}
		if strings.TrimSpace(item.ExpiryTime) != "" {
			parsed, err := time.Parse(time.RFC3339, item.ExpiryTime)
			if err != nil {
				return nil, fmt.Errorf("%w: line item %q has an unparsable expiry %q",
					ErrUnavailable, item.ProductID, item.ExpiryTime)
			}
			line.ExpiryTime = parsed.UTC()
		}
		if item.AutoRenewingPlan != nil && item.AutoRenewingPlan.AutoRenewEnabled != nil {
			line.AutoRenewSet = true
			line.AutoRenew = *item.AutoRenewingPlan.AutoRenewEnabled
		}
		out.LineItems = append(out.LineItems, line)
	}
	return out, nil
}

// Acknowledge tells Google this server has accepted a purchase.
//
// Play refunds an unacknowledged purchase after three days. That is not a
// billing edge case: it is the difference between a subscription that pays and
// one that is silently refunded three days after it is bought, every time,
// because nothing in this server ever called this endpoint.
//
// It is idempotent on Google's side. A purchase that is already acknowledged
// answers 400 with a body saying so, which is treated as success: the
// customer's entitlement is exactly what it would be either way, and failing
// the request would make a retry look like an outage.
func (c *Client) Acknowledge(ctx context.Context, productID, purchaseToken string) error {
	productID = strings.TrimSpace(productID)
	purchaseToken = strings.TrimSpace(purchaseToken)
	if productID == "" || purchaseToken == "" {
		return fmt.Errorf("%w: acknowledgement needs a product id and a purchase token", ErrTokenInvalid)
	}
	if c.TokenSource == nil {
		return fmt.Errorf("%w: no token source configured", ErrUnauthorized)
	}
	accessToken, err := c.TokenSource(ctx)
	if err != nil {
		return fmt.Errorf("%w: obtain credentials: %v", ErrUnauthorized, err)
	}

	endpoint := fmt.Sprintf("%s/androidpublisher/v3/applications/%s/purchases/subscriptions/%s/tokens/%s:acknowledge",
		c.baseURL(), url.PathEscape(c.PackageName), url.PathEscape(productID), url.PathEscape(purchaseToken))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return fmt.Errorf("play: build acknowledge request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))

	switch res.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusCreated:
		return nil
	case http.StatusBadRequest:
		// Already acknowledged, or a token Google will never accept. Only the
		// first is benign, and Google distinguishes them in the body - which
		// says "The purchase has already been acknowledged." in some API
		// versions and "Purchase already acknowledged" in others, so both words
		// are matched rather than one exact sentence.
		lower := strings.ToLower(string(body))
		if strings.Contains(lower, "already") && strings.Contains(lower, "acknowledg") {
			return nil
		}
		return fmt.Errorf("%w: acknowledge rejected (%d): %s", ErrTokenInvalid, res.StatusCode, snippet(body))
	case http.StatusNotFound, http.StatusGone:
		return fmt.Errorf("%w: acknowledge rejected (%d): %s", ErrTokenInvalid, res.StatusCode, snippet(body))
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: acknowledge returned %d: %s", ErrUnauthorized, res.StatusCode, snippet(body))
	default:
		return fmt.Errorf("%w: acknowledge returned %d: %s", ErrUnavailable, res.StatusCode, snippet(body))
	}
}

// snippet trims an upstream error body for a log line or an error message.
func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	if s == "" {
		return "<empty body>"
	}
	return s
}
