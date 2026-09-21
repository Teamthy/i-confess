package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/playapi"
)

// Tests for Google Play verification (IC-003).
//
// The finding these tests close: the previous "production" verifier never
// called Google. It unmarshalled the string the client sent and granted premium
// whenever it contained a purchase_token, so `{"purchase_token":"anything"}`
// was a free subscription. Every test below asserts a decision that must come
// from Google's answer rather than from the request.
//
// The conversation with Google's API is tested in internal/playapi; what is
// tested here is what the answer means.

const (
	testPlayPackage = "app.iconfess"
	testPlayMonthly = "premium_monthly"
	testPlayAnnual  = "premium_annual"
	testPlayToken   = "purchase-token-abc123"
)

// fakePlay is a PlayPurchaseClient that records what it was asked.
type fakePlay struct {
	mu        sync.Mutex
	asked     []string
	purchase  *playapi.Subscription
	returnNil bool
	err       error
	callCount int
}

func (f *fakePlay) Subscription(_ context.Context, purchaseToken string) (*playapi.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, purchaseToken)
	f.callCount++
	if f.err != nil {
		return nil, f.err
	}
	if f.returnNil {
		return nil, nil
	}
	if f.purchase == nil {
		return nil, fmt.Errorf("%w: no purchase configured", playapi.ErrTokenInvalid)
	}
	return f.purchase, nil
}

func (f *fakePlay) tokens() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.asked...)
}

func (f *fakePlay) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callCount
}

// purchase builds a Google answer.
func purchase(state, productID string, expires time.Time, autoRenew *bool) *playapi.Subscription {
	item := playapi.LineItem{ProductID: productID, ExpiryTime: expires.UTC()}
	if autoRenew != nil {
		item.AutoRenewSet = true
		item.AutoRenew = *autoRenew
	}
	return &playapi.Subscription{
		State:                state,
		AcknowledgementState: playapi.AckAcknowledged,
		LatestOrderID:        "GPA.3333-4137-0319-36762",
		LineItems:            []playapi.LineItem{item},
	}
}

// rowEntitles asks the model the same question the store asks when it resolves
// a user's plan, so these tests assert what will actually be enforced rather
// than only what the verifier returned.
func rowEntitles(status, expiresAt string, now time.Time) bool {
	row := models.Subscription{Status: status, EndsAt: expiresAt}
	return row.Entitled(now)
}

func newPlayVerifier(t *testing.T, client PlayPurchaseClient, mutate func(*GooglePlayConfig)) *GooglePlayVerifier {
	t.Helper()

	cfg := GooglePlayConfig{
		PackageName: testPlayPackage,
		Plans: map[string]string{
			testPlayMonthly: "monthly",
			testPlayAnnual:  "annual",
		},
		Purchases: client,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	verifier, err := NewGooglePlayVerifier(cfg)
	if err != nil {
		t.Fatalf("build verifier: %v", err)
	}
	return verifier
}

// ---------------------------------------------------------------------------
// The exploit
// ---------------------------------------------------------------------------

// TestGoogleVerifierRejectsTheClientSuppliedJSONThatPreviouslyGrantedPremium is
// the regression test for IC-003 on the Play side.
//
// The exact payload that used to grant premium is sent, with Google answering
// that the token does not exist. It must be refused, and Google must have been
// asked: a rejection that never checked is not verification.
func TestGoogleVerifierRejectsTheClientSuppliedJSONThatPreviouslyGrantedPremium(t *testing.T) {
	client := &fakePlay{err: fmt.Errorf("%w: unknown token", playapi.ErrTokenInvalid)}
	verifier := newPlayVerifier(t, client, nil)

	forged := `{"purchase_token":"anything","product_id":"` + testPlayAnnual + `","expiry_time_millis":9999999999999}`
	got, err := verifier.Verify(context.Background(), "google", forged)
	if err == nil && got.Valid {
		t.Fatal("a client-supplied purchase token was accepted - premium is free again")
	}
	if !errors.Is(err, ErrInvalidReceipt) {
		t.Fatalf("error = %v, want ErrInvalidReceipt", err)
	}
	// The token from the envelope is what must be checked, not the whole blob.
	if tokens := client.tokens(); len(tokens) != 1 || tokens[0] != "anything" {
		t.Fatalf("asked Google about %v, want the token from the envelope", tokens)
	}
}

// A client that lies about the product must not get the better plan, and one
// that lies about the expiry must not extend it.
func TestGoogleVerifierTakesPlanAndExpiryFromGoogleNotTheClient(t *testing.T) {
	expires := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	verifier := newPlayVerifier(t, &fakePlay{
		purchase: purchase(playapi.StateActive, testPlayMonthly, expires, nil),
	}, nil)

	lie := fmt.Sprintf(`{"purchase_token":"%s","product_id":"%s","expiry_time_millis":%d}`,
		testPlayToken, testPlayAnnual, time.Now().Add(10*365*24*time.Hour).UnixMilli())

	got, err := verifier.Verify(context.Background(), "google", lie)
	if err != nil || !got.Valid {
		t.Fatalf("genuine subscription rejected: %v %+v", err, got)
	}
	if got.PlanID != "monthly" {
		t.Errorf("plan = %q, want monthly from Google rather than the claimed annual", got.PlanID)
	}
	if got.ProductID != testPlayMonthly {
		t.Errorf("product = %q, want Google's %q", got.ProductID, testPlayMonthly)
	}
	if want := expires.Format(time.RFC3339); got.ExpiresAt != want {
		t.Errorf("expires_at = %q, want Google's %q", got.ExpiresAt, want)
	}
}

// ---------------------------------------------------------------------------
// Lifecycle states
// ---------------------------------------------------------------------------

func TestGoogleVerifierAcceptsAnActiveSubscription(t *testing.T) {
	autoRenew := true
	verifier := newPlayVerifier(t, &fakePlay{
		purchase: purchase(playapi.StateActive, testPlayMonthly, time.Now().Add(20*24*time.Hour), &autoRenew),
	}, nil)

	got, err := verifier.Verify(context.Background(), "google", testPlayToken)
	if err != nil || !got.Valid {
		t.Fatalf("active subscription rejected: %v %+v", err, got)
	}
	if got.State != models.SubscriptionActive {
		t.Errorf("state = %q, want active", got.State)
	}
	if got.AutoRenew == nil || !*got.AutoRenew {
		t.Errorf("auto-renew not captured: %+v", got.AutoRenew)
	}
	if got.NeedsAcknowledgement {
		t.Error("an acknowledged purchase was flagged for acknowledgement")
	}
	if got.TransactionID != "GPA.3333-4137-0319-36762" {
		t.Errorf("order id not captured: %q", got.TransactionID)
	}
}

// The grace period exists so a failed payment does not immediately cut a paying
// customer off. Revoking here is the most common cause of "you cancelled my
// subscription" complaints from users who never cancelled.
func TestGoogleVerifierKeepsEntitlementThroughTheGracePeriod(t *testing.T) {
	autoRenew := true
	verifier := newPlayVerifier(t, &fakePlay{
		purchase: purchase(playapi.StateInGracePeriod, testPlayMonthly, time.Now().Add(5*24*time.Hour), &autoRenew),
	}, nil)

	got, err := verifier.Verify(context.Background(), "google", testPlayToken)
	if err != nil || !got.Valid {
		t.Fatalf("a subscription in its grace period lost entitlement: %v %+v", err, got)
	}
	if got.State != models.SubscriptionGrace {
		t.Errorf("state = %q, want grace", got.State)
	}
	if !rowEntitles(got.State, got.ExpiresAt, time.Now()) {
		t.Error("the recorded row would not entitle the user during the grace period")
	}
}

// SUBSCRIPTION_STATE_CANCELED means "cancelled, but the paid period has not
// ended". Access continues until the expiry Google reports. Treating CANCELED
// as EXPIRED revokes access the customer paid for.
func TestGoogleVerifierKeepsEntitlementWhileCancelledButPaid(t *testing.T) {
	autoRenew := false
	verifier := newPlayVerifier(t, &fakePlay{
		purchase: purchase(playapi.StateCanceled, testPlayMonthly, time.Now().Add(10*24*time.Hour), &autoRenew),
	}, nil)

	got, err := verifier.Verify(context.Background(), "google", testPlayToken)
	if err != nil || !got.Valid {
		t.Fatalf("a cancelled-but-paid subscription lost entitlement early: %v %+v", err, got)
	}
	if got.State != models.SubscriptionCancelled {
		t.Errorf("state = %q, want cancelled", got.State)
	}
	if got.AutoRenew == nil || *got.AutoRenew {
		t.Errorf("auto-renew should be false for a cancelled subscription, got %+v", got.AutoRenew)
	}
	// The stored row must entitle, because the paid period is still running.
	if !rowEntitles(got.State, got.ExpiresAt, time.Now()) {
		t.Error("a cancelled subscription inside its paid period does not entitle the user")
	}
	// ...and must stop entitling once the period ends.
	if rowEntitles(got.State, got.ExpiresAt, time.Now().Add(11*24*time.Hour)) {
		t.Error("a cancelled subscription still entitles after its paid period ends")
	}
}

// ...and when the paid period really is over, the entitlement ends.
func TestGoogleVerifierRevokesWhenThePaidPeriodEnds(t *testing.T) {
	autoRenew := false
	cases := map[string]*playapi.Subscription{
		"cancelled and past expiry": purchase(playapi.StateCanceled, testPlayMonthly, time.Now().Add(-24*time.Hour), &autoRenew),
		"expired":                   purchase(playapi.StateExpired, testPlayMonthly, time.Now().Add(-24*time.Hour), &autoRenew),
		"on hold":                   purchase(playapi.StateOnHold, testPlayMonthly, time.Now().Add(-24*time.Hour), &autoRenew),
		"paused":                    purchase(playapi.StatePaused, testPlayMonthly, time.Now().Add(24*time.Hour), &autoRenew),
		"pending payment":           purchase(playapi.StatePending, testPlayMonthly, time.Now().Add(24*time.Hour), &autoRenew),
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			verifier := newPlayVerifier(t, &fakePlay{purchase: answer}, nil)

			got, err := verifier.Verify(context.Background(), "google", testPlayToken)
			if err != nil {
				t.Fatalf("genuine but non-entitling state returned an error: %v", err)
			}
			if got.Valid {
				t.Fatalf("%s still granted premium", name)
			}
			if got.Detail == "" {
				t.Error("refusal carries no detail")
			}
			// Whatever the store said, the recorded row must not entitle.
			if rowEntitles(got.State, got.ExpiresAt, time.Now()) {
				t.Errorf("%s produced a row that still entitles the user (status %q, ends %q)",
					name, got.State, got.ExpiresAt)
			}
		})
	}
}

// An unknown state must not fall through to "entitled". Google adds states; the
// safe default for one this server has never seen is no access.
func TestGoogleVerifierRefusesAnUnknownState(t *testing.T) {
	verifier := newPlayVerifier(t, &fakePlay{
		purchase: purchase("SUBSCRIPTION_STATE_SOMETHING_NEW", testPlayMonthly, time.Now().Add(24*time.Hour), nil),
	}, nil)

	got, err := verifier.Verify(context.Background(), "google", testPlayToken)
	if err == nil && got.Valid {
		t.Fatal("an unknown subscription state granted premium")
	}
	if err == nil {
		t.Fatal("expected an error for an unknown state")
	}
	if !errors.Is(err, ErrProviderError) {
		t.Errorf("error = %v, want ErrProviderError so an operator sees it", err)
	}
}

// An upgrade leaves the superseded line item in the response. The one with time
// left is the one the customer is paying for.
func TestGoogleVerifierPicksTheLineItemThatStillHasTime(t *testing.T) {
	verifier := newPlayVerifier(t, &fakePlay{purchase: &playapi.Subscription{
		State: playapi.StateActive,
		LineItems: []playapi.LineItem{
			{ProductID: testPlayMonthly, ExpiryTime: time.Now().Add(-48 * time.Hour)},
			{ProductID: testPlayAnnual, ExpiryTime: time.Now().Add(300 * 24 * time.Hour)},
		},
	}}, nil)

	got, err := verifier.Verify(context.Background(), "google", testPlayToken)
	if err != nil || !got.Valid {
		t.Fatalf("upgraded subscription rejected: %v %+v", err, got)
	}
	if got.PlanID != "annual" {
		t.Errorf("plan = %q, want annual (the line item with time left)", got.PlanID)
	}
}

// A purchase that is not acknowledged is refunded by Google after three days,
// so the flag has to reach the caller.
func TestGoogleVerifierFlagsAnUnacknowledgedPurchase(t *testing.T) {
	p := purchase(playapi.StateActive, testPlayMonthly, time.Now().Add(24*time.Hour), nil)
	p.AcknowledgementState = playapi.AckPending
	verifier := newPlayVerifier(t, &fakePlay{purchase: p}, nil)

	got, err := verifier.Verify(context.Background(), "google", testPlayToken)
	if err != nil || !got.Valid {
		t.Fatalf("valid subscription rejected: %v %+v", err, got)
	}
	if !got.NeedsAcknowledgement {
		t.Error("an unacknowledged purchase was not flagged - it will be refunded in three days")
	}
}

func TestGoogleVerifierRefusesAnEmptyProviderResponse(t *testing.T) {
	verifier := newPlayVerifier(t, &fakePlay{returnNil: true}, nil)
	_, err := verifier.Verify(context.Background(), "google", testPlayToken)
	if !errors.Is(err, ErrProviderError) {
		t.Fatalf("empty provider response error = %v, want ErrProviderError", err)
	}
}

// A product this server does not sell must not grant anything, however healthy
// the subscription is.
func TestGoogleVerifierRejectsAnUnmappedProduct(t *testing.T) {
	verifier := newPlayVerifier(t, &fakePlay{
		purchase: purchase(playapi.StateActive, "some_other_sku", time.Now().Add(24*time.Hour), nil),
	}, nil)

	if got, err := verifier.Verify(context.Background(), "google", testPlayToken); err == nil && got.Valid {
		t.Fatal("an unmapped Play product granted a plan")
	}
}

// A prepaid plan has no auto-renewingPlan object at all, which is not the same
// as auto-renewal switched off.
func TestGoogleVerifierDistinguishesAbsentAutoRenew(t *testing.T) {
	verifier := newPlayVerifier(t, &fakePlay{
		purchase: purchase(playapi.StateActive, testPlayMonthly, time.Now().Add(24*time.Hour), nil),
	}, nil)

	got, err := verifier.Verify(context.Background(), "google", testPlayToken)
	if err != nil || !got.Valid {
		t.Fatalf("prepaid subscription rejected: %v %+v", err, got)
	}
	if got.AutoRenew != nil {
		t.Errorf("absent auto-renew recorded as %v, want unknown", *got.AutoRenew)
	}
}

// ---------------------------------------------------------------------------
// Failure classification
// ---------------------------------------------------------------------------

// An operator problem must never be reported to a paying customer as "your
// receipt is invalid": that sends them to support with a problem support cannot
// see, and it hides a broken deployment.
func TestGoogleVerifierSeparatesProviderFaultsFromBadReceipts(t *testing.T) {
	careless := map[string]error{
		"unauthorized": fmt.Errorf("%w: 403", playapi.ErrUnauthorized),
		"unavailable":  fmt.Errorf("%w: 503", playapi.ErrUnavailable),
		"unknown":      errors.New("something else entirely"),
	}
	for name, err := range careless {
		t.Run(name, func(t *testing.T) {
			verifier := newPlayVerifier(t, &fakePlay{err: err}, nil)

			_, verr := verifier.Verify(context.Background(), "google", testPlayToken)
			if verr == nil {
				t.Fatal("expected an error")
			}
			if errors.Is(verr, ErrInvalidReceipt) {
				t.Fatalf("%s was reported as an invalid receipt; it is our fault, not the caller's: %v", name, verr)
			}
			if !errors.Is(verr, ErrProviderError) {
				t.Fatalf("%s: error = %v, want ErrProviderError", name, verr)
			}
		})
	}

	verifier := newPlayVerifier(t, &fakePlay{err: fmt.Errorf("%w: unknown token", playapi.ErrTokenInvalid)}, nil)
	if _, err := verifier.Verify(context.Background(), "google", testPlayToken); !errors.Is(err, ErrInvalidReceipt) {
		t.Errorf("a rejected token: error = %v, want ErrInvalidReceipt", err)
	}
}

// Without credentials nothing can be verified, and nothing may be granted. This
// is the fail-closed property the previous implementation lacked.
func TestGoogleVerifierNeverGrantsWithoutCredentials(t *testing.T) {
	client := &fakePlay{err: fmt.Errorf("%w: no service account configured", playapi.ErrUnauthorized)}
	verifier := newPlayVerifier(t, client, nil)

	got, err := verifier.Verify(context.Background(), "google", testPlayToken)
	if err == nil && got.Valid {
		t.Fatal("granted premium without credentials")
	}
	if !errors.Is(err, ErrProviderError) {
		t.Fatalf("error = %v, want ErrProviderError", err)
	}
}

func TestGoogleVerifierRejectsAnotherProvider(t *testing.T) {
	verifier := newPlayVerifier(t, &fakePlay{}, nil)
	for _, provider := range []string{"apple", "", "stripe"} {
		if got, err := verifier.Verify(context.Background(), provider, testPlayToken); err == nil && got.Valid {
			t.Errorf("provider %q was accepted by the Play verifier", provider)
		}
	}
	// A mismatched provider must be decided locally, without asking Google.
	if calls := verifier.cfg.Purchases.(*fakePlay).calls(); calls != 0 {
		t.Errorf("Google was queried %d times for a provider it does not serve", calls)
	}
}

func TestGoogleVerifierRejectsAnEmptyReceipt(t *testing.T) {
	client := &fakePlay{purchase: purchase(playapi.StateActive, testPlayMonthly, time.Now().Add(time.Hour), nil)}
	verifier := newPlayVerifier(t, client, nil)

	for _, receipt := range []string{"", "   ", "{}", `{"purchase_token":""}`} {
		if got, err := verifier.Verify(context.Background(), "google", receipt); err == nil && got.Valid {
			t.Errorf("receipt %q was accepted", receipt)
		}
	}
	if calls := client.calls(); calls != 0 {
		t.Errorf("Google was queried %d times for an empty receipt", calls)
	}
}

// A bare purchase token is the normal shape (in_app_purchase hands the app the
// token itself). Both spellings of the JSON envelope are tolerated because
// clients wrap it differently.
func TestGoogleVerifierAcceptsTheTokenShapesClientsSend(t *testing.T) {
	for _, receipt := range []string{
		testPlayToken,
		`{"purchase_token":"` + testPlayToken + `"}`,
		`{"purchaseToken":"` + testPlayToken + `"}`,
		`{"packageName":"app.iconfess","purchaseToken":"` + testPlayToken + `","productId":"x"}`,
	} {
		client := &fakePlay{purchase: purchase(playapi.StateActive, testPlayMonthly, time.Now().Add(time.Hour), nil)}
		verifier := newPlayVerifier(t, client, nil)

		if _, err := verifier.Verify(context.Background(), "google", receipt); err != nil {
			t.Errorf("receipt %q was rejected: %v", receipt, err)
		}
		tokens := client.tokens()
		if len(tokens) != 1 || tokens[0] != testPlayToken {
			t.Errorf("receipt %q asked about %v, want [%s]", receipt, tokens, testPlayToken)
		}
	}
}

func TestNewGooglePlayVerifierRefusesIncompleteConfiguration(t *testing.T) {
	plans := map[string]string{testPlayMonthly: "monthly"}
	client := &fakePlay{}
	cases := map[string]GooglePlayConfig{
		"no package":     {Plans: plans, Purchases: client},
		"no plans":       {PackageName: testPlayPackage, Purchases: client},
		"no credentials": {PackageName: testPlayPackage, Plans: plans},
	}
	for name, cfg := range cases {
		if _, err := NewGooglePlayVerifier(cfg); err == nil {
			t.Errorf("%s: verifier was constructed", name)
		} else if !errors.Is(err, ErrUnconfigured) {
			t.Errorf("%s: error = %v, want ErrUnconfigured", name, err)
		}
	}
}

// The verifier must not be talked into a plan by anything except the configured
// product map.
func TestGoogleVerifierIgnoresProductIDsFromTheClient(t *testing.T) {
	verifier := newPlayVerifier(t, &fakePlay{
		purchase: purchase(playapi.StateActive, testPlayMonthly, time.Now().Add(time.Hour), nil),
	}, nil)

	// A product id in the receipt that maps to the annual plan must not be used.
	receipt := `{"purchase_token":"` + testPlayToken + `","product_id":"` + testPlayAnnual + `"}`
	got, err := verifier.Verify(context.Background(), "google", receipt)
	if err != nil || !got.Valid {
		t.Fatalf("genuine subscription rejected: %v %+v", err, got)
	}
	if got.PlanID != "monthly" {
		t.Errorf("plan = %q, want monthly from Google's line items", got.PlanID)
	}
}

// The Play environment is reported so test purchases can be told apart from
// revenue.
func TestGoogleVerifierReportsTestPurchases(t *testing.T) {
	p := purchase(playapi.StateActive, testPlayMonthly, time.Now().Add(time.Hour), nil)
	p.TestPurchase = true
	verifier := newPlayVerifier(t, &fakePlay{purchase: p}, nil)

	got, err := verifier.Verify(context.Background(), "google", testPlayToken)
	if err != nil || !got.Valid {
		t.Fatalf("test purchase rejected: %v %+v", err, got)
	}
	if !strings.EqualFold(got.Environment, "test") {
		t.Errorf("environment = %q, want Test", got.Environment)
	}
}
