package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/playapi"
)

// Google Play receipt verification (IC-003).
//
// The previous "production" implementation unmarshalled the string the client
// sent and granted premium whenever it contained a non-empty purchase_token:
// `{"purchase_token":"anything"}` was a free subscription. A purchase token is
// a claim by the client like any other, and the only way to learn whether it
// names a real subscription is to ask Google.
//
// This file holds the decision: which states entitle, for how long, and what a
// refusal means. The conversation with Google's servers lives in
// internal/playapi, because a rule about entitlement should be expressible
// without an HTTP client — and because internal/billing is domain logic, which
// the arch tests forbid from importing net/http.

// PlayPurchaseClient fetches the current state of a Play purchase token.
//
// An interface rather than a concrete client so this package's tests need no
// server, and so a future provider (Amazon, Stripe) can satisfy the same shape.
type PlayPurchaseClient interface {
	Subscription(ctx context.Context, purchaseToken string) (*playapi.Subscription, error)
}

// GooglePlayConfig configures the Play verifier.
type GooglePlayConfig struct {
	// PackageName is the applicationId the subscription must belong to.
	// Required for the same reason as AppleConfig.BundleID: the verify endpoint
	// is reachable by anyone.
	PackageName string
	// Plans maps a Play product id to a plan in this server's catalogue. An
	// unmapped product grants nothing.
	Plans map[string]string
	// Purchases is the Play Developer API client. Required: a verifier that
	// cannot ask Google cannot decide anything.
	Purchases PlayPurchaseClient
	// Now is injectable for tests.
	Now func() time.Time
}

func (c GooglePlayConfig) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

// GooglePlayVerifier verifies Play subscriptions against Google's API.
type GooglePlayVerifier struct {
	cfg GooglePlayConfig
}

// NewGooglePlayVerifier builds a verifier, refusing a configuration that could
// not reach a verdict.
func NewGooglePlayVerifier(cfg GooglePlayConfig) (*GooglePlayVerifier, error) {
	if strings.TrimSpace(cfg.PackageName) == "" {
		return nil, fmt.Errorf("%w: GOOGLE_PLAY_PACKAGE_NAME is required", ErrUnconfigured)
	}
	if len(cfg.Plans) == 0 {
		return nil, fmt.Errorf("%w: no Play product ids are mapped to plans", ErrUnconfigured)
	}
	if cfg.Purchases == nil {
		return nil, fmt.Errorf("%w: Google Play verification needs a service account", ErrUnconfigured)
	}
	return &GooglePlayVerifier{cfg: cfg}, nil
}

// Verify implements Verifier.
func (v *GooglePlayVerifier) Verify(ctx context.Context, provider, receipt string) (Verification, error) {
	if p := strings.ToLower(strings.TrimSpace(provider)); p != "google" {
		return Verification{}, fmt.Errorf("%w: provider %q is not handled by the Play verifier",
			ErrInvalidReceipt, provider)
	}

	token, err := extractPurchaseToken(receipt)
	if err != nil {
		return Verification{}, err
	}

	purchase, err := v.cfg.Purchases.Subscription(ctx, token)
	if err != nil {
		return Verification{}, translatePlayError(err)
	}
	return v.evaluate(purchase)
}

// translatePlayError keeps the caller's fault and ours apart.
//
// Reporting a missing service account as "invalid receipt" is how a broken
// deployment looks like a stream of fraudulent users: the customer is told
// their purchase is bad, support cannot see anything wrong, and nobody looks at
// the deployment.
func translatePlayError(err error) error {
	switch {
	case errors.Is(err, playapi.ErrTokenInvalid):
		return fmt.Errorf("%w: %v", ErrInvalidReceipt, err)
	default:
		// ErrUnauthorized, ErrUnavailable, or anything a future client adds:
		// the receipt is not the problem.
		return fmt.Errorf("%w: %v", ErrProviderError, err)
	}
}

// extractPurchaseToken accepts either a bare purchase token or the JSON blob a
// Play Billing client hands the app.
//
// Accepting the blob is input decoding, not a trust decision: the token that
// comes out of it is still checked with Google before it grants anything. A
// fabricated token is rejected by the API, which is the property the previous
// implementation lacked.
func extractPurchaseToken(receipt string) (string, error) {
	receipt = strings.TrimSpace(receipt)
	if receipt == "" {
		return "", fmt.Errorf("%w: google receipt is empty", ErrInvalidReceipt)
	}
	if !strings.HasPrefix(receipt, "{") {
		return receipt, nil
	}

	var envelope struct {
		PurchaseToken      string `json:"purchase_token"`
		PurchaseTokenCamel string `json:"purchaseToken"`
	}
	if err := json.Unmarshal([]byte(receipt), &envelope); err != nil {
		return "", fmt.Errorf("%w: google receipt is neither a purchase token nor valid JSON",
			ErrInvalidReceipt)
	}
	switch {
	case envelope.PurchaseToken != "":
		return envelope.PurchaseToken, nil
	case envelope.PurchaseTokenCamel != "":
		return envelope.PurchaseTokenCamel, nil
	default:
		return "", fmt.Errorf("%w: google receipt contains no purchase token", ErrInvalidReceipt)
	}
}

// evaluate turns Google's answer into a grant or a refusal.
func (v *GooglePlayVerifier) evaluate(p *playapi.Subscription) (Verification, error) {
	now := v.cfg.now()

	// The line item that runs furthest into the future decides the plan: an
	// upgrade can leave the superseded line item in the list, and the one the
	// customer is paying for is the one with time left.
	var (
		plan          string
		expires       time.Time
		autoRenew     *bool
		productID     string
		foundMappable bool
	)
	for _, item := range p.LineItems {
		mapped, ok := v.cfg.Plans[item.ProductID]
		if !ok {
			continue
		}
		foundMappable = true
		if plan == "" || item.ExpiryTime.After(expires) {
			plan, expires, productID = mapped, item.ExpiryTime, item.ProductID
			if item.AutoRenewSet {
				value := item.AutoRenew
				autoRenew = &value
			} else {
				// Absent is not false: a prepaid plan has no auto-renewal at
				// all, and recording that as "switched off" would make a
				// healthy subscription look cancelled.
				autoRenew = nil
			}
		}
	}
	if !foundMappable {
		return Verification{}, fmt.Errorf(
			"%w: no Play product in this purchase is mapped to a plan on this server (%s)",
			ErrInvalidReceipt, strings.Join(playProductIDs(p.LineItems), ", "))
	}

	out := Verification{
		Provider:              "google",
		ProductID:             productID,
		TransactionID:         p.LatestOrderID,
		OriginalTransactionID: p.LinkedPurchaseToken,
		AutoRenew:             autoRenew,
		Environment:           playEnvironment(p),
		NeedsAcknowledgement:  p.AcknowledgementState == playapi.AckPending,
	}
	if expires.IsZero() {
		return Verification{}, fmt.Errorf("%w: Play returned no expiry for the mapped product", ErrProviderError)
	}
	out.ExpiresAt = expires.Format(time.RFC3339)

	switch p.State {
	case playapi.StateActive:
		out.State = models.SubscriptionActive
	case playapi.StateInGracePeriod:
		// The store is retrying a failed payment and access continues. Not
		// 'active': the payment did fail, and an operator looking at the row
		// needs to see that a dunning process is running.
		out.State = models.SubscriptionGrace
	case playapi.StateCanceled:
		// Cancelled, but the paid period has not ended. No renewal is coming
		// and access continues until the expiry — the clock below decides.
		out.State = models.SubscriptionCancelled
	case playapi.StateOnHold, playapi.StatePaused, playapi.StatePending:
		out.Valid = false
		out.State = models.SubscriptionSuspended
		out.Detail = "google: subscription is not delivering access (" + p.State + ")"
		return out, nil
	case playapi.StateExpired:
		out.Valid = false
		out.State = models.SubscriptionExpired
		out.Detail = "google: subscription has expired"
		return out, nil
	case playapi.StateUnspecified, "":
		return Verification{}, fmt.Errorf("%w: Play returned no subscription state", ErrProviderError)
	default:
		// An unrecognised state is a state this server has never seen. The safe
		// reading is "not entitled", and it is worth an operator knowing that
		// Google has added something.
		return Verification{}, fmt.Errorf("%w: Play returned unknown subscription state %q",
			ErrProviderError, p.State)
	}

	if !expires.After(now) {
		out.Valid = false
		out.State = models.SubscriptionExpired
		out.Detail = "google: subscription period has ended"
		return out, nil
	}

	out.Valid = true
	out.PlanID = plan
	out.Detail = "google: verified Play subscription " + productID
	return out, nil
}

func playProductIDs(items []playapi.LineItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.ProductID != "" {
			out = append(out, item.ProductID)
		}
	}
	if len(out) == 0 {
		return []string{"<no line items>"}
	}
	return out
}

// playEnvironment distinguishes a licensed-tester purchase from a real one, so
// test purchases can be kept out of revenue reporting.
func playEnvironment(p *playapi.Subscription) string {
	if p.TestPurchase {
		return "Test"
	}
	return "Production"
}
