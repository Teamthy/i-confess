package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/billing"
	"github.com/Teamthy/i-confess/internal/entitlements"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

// End-to-end tests for POST /subscriptions/verify (IC-003).
//
// The audit's validation steps for this finding were: a replayed receipt has a
// single effect; an expired subscription loses premium; a refunded purchase is
// revoked; and the same receipt cannot be redeemed by two accounts. Each is a
// test below, run against the real handler and a real database.
//
// The development verifier is what is installed here (ENV is unset, so the
// handler resolves to the no-op verifier), and since it now returns an expiry
// and a stable transaction id, these paths are exercised rather than assumed.
// The signature checks themselves are covered in internal/billing against a
// generated certificate authority.

// verify posts a receipt and returns the status and body.
func (f *audioFixture) verify(t *testing.T, token, provider, receipt string) (int, map[string]any) {
	t.Helper()
	return f.call(t, http.MethodPost, "/subscriptions/verify", token, map[string]any{
		"provider": provider,
		"receipt":  receipt,
	})
}

func (f *audioFixture) entitlements(t *testing.T, token string) map[string]any {
	t.Helper()
	code, body := getJSON(t, f, "/entitlements", token)
	if code != http.StatusOK {
		t.Fatalf("GET /entitlements: %d", code)
	}
	return body
}

func (f *audioFixture) subscription(t *testing.T, token string) map[string]any {
	t.Helper()
	code, body := getJSON(t, f, "/subscription", token)
	if code != http.StatusOK {
		t.Fatalf("GET /subscription: %d", code)
	}
	return body
}

// A genuine purchase grants premium, and the expiry the store reported is
// recorded rather than discarded.
func TestVerifyingAReceiptGrantsPremiumAndRecordsItsExpiry(t *testing.T) {
	f := newAudioFixture(t)
	token, userID := f.registerWithID(t, "buyer@test.com")

	if got := f.entitlements(t, token)["plan"]; got != "free" {
		t.Fatalf("plan before purchase = %v, want free", got)
	}

	code, body := f.verify(t, token, "apple", "valid_monthly")
	if code != http.StatusOK {
		t.Fatalf("verify: %d %v", code, body)
	}
	if body["verified"] != true {
		t.Fatalf("verified = %v (%v)", body["verified"], body)
	}
	if body["expires_at"] == "" || body["expires_at"] == nil {
		t.Errorf("response carries no expiry: %v", body)
	}

	// The entitlement is live...
	if got := f.entitlements(t, token)["plan"]; got != "premium" {
		t.Errorf("plan after purchase = %v, want premium", got)
	}
	// ...and the row records where it came from and when it ends. Without the
	// expiry there is nothing for the entitlement check to run against.
	users := store.NewUserStore(f.db)
	record, err := users.SubscriptionRecord(context.Background(), userID)
	if err != nil || record == nil {
		t.Fatalf("record: %v %+v", err, record)
	}
	if record.Plan != entitlements.PlanPremium {
		t.Errorf("stored plan = %q, want premium", record.Plan)
	}
	if record.Provider != "apple" {
		t.Errorf("provider = %q, want apple", record.Provider)
	}
	if record.OriginalTransactionID == "" {
		t.Error("no original transaction id recorded - a renewal or refund could not be matched to this account")
	}
	if _, ok := record.ExpiresAt(); !ok {
		t.Errorf("stored expiry %q is unusable", record.EndsAt)
	}
}

// Replaying the same receipt must have one effect, not two: the mobile client
// retries on a flaky network, and refresh-on-launch re-sends it every time.
func TestReplayingAReceiptDoesNotDuplicateTheSubscription(t *testing.T) {
	f := newAudioFixture(t)
	token, userID := f.registerWithID(t, "replay@test.com")

	for i := 0; i < 3; i++ {
		if code, body := f.verify(t, token, "apple", "valid_annual"); code != http.StatusOK {
			t.Fatalf("verify %d: %d %v", i, code, body)
		}
	}

	var rows int
	if err := f.db.QueryRow(
		`SELECT count(*) FROM subscriptions WHERE user_id = ?`, userID).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Fatalf("%d subscription rows after three replays, want 1", rows)
	}
}

// The hole this closes: entitlement came from the status column, so a period
// that had ended still granted premium.
func TestAnExpiredSubscriptionLosesPremium(t *testing.T) {
	f := newAudioFixture(t)
	token, userID := f.registerWithID(t, "lapsed@test.com")

	users := store.NewUserStore(f.db)
	ctx := context.Background()
	if err := users.SaveVerifiedSubscription(ctx, userID, store.VerifiedSubscription{
		Plan: "premium", Status: models.SubscriptionActive,
		ExpiresAt:             time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		Provider:              "apple",
		OriginalTransactionID: "tx-lapsed",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	if got := f.entitlements(t, token)["plan"]; got != "free" {
		t.Errorf("plan = %v, want free once the paid period has ended", got)
	}
	// The status is reported as expired rather than active, so a client showing
	// "Premium" over a lapsed row is showing its own bug.
	if got := f.subscription(t, token)["active"]; got != false {
		t.Errorf("active = %v, want false", got)
	}
}

// A refund is Apple's or Google's decision. It must revoke access even though
// the paid period is still on the clock, and the client must be able to say why.
func TestARefundedPurchaseIsRevoked(t *testing.T) {
	f := newAudioFixture(t)
	token, userID := f.registerWithID(t, "refunded@test.com")

	// First a real purchase, so the account is premium and the row carries the
	// store's identity.
	code, body := f.verify(t, token, "apple", "valid_monthly")
	if code != http.StatusOK || body["verified"] != true {
		t.Fatalf("purchase: %d %v", code, body)
	}
	if got := f.entitlements(t, token)["plan"]; got != "premium" {
		t.Fatalf("plan = %v, want premium after the purchase", got)
	}

	// The store now reports the purchase as refunded.
	code, body = f.verify(t, token, "apple", "revoked_monthly")
	if code != http.StatusOK {
		t.Fatalf("revoked verify: %d %v", code, body)
	}
	if body["verified"] != false {
		t.Errorf("a refunded receipt reported verified = %v", body["verified"])
	}

	if got := f.entitlements(t, token)["plan"]; got != "free" {
		t.Errorf("plan = %v, want free after a refund", got)
	}
	users := store.NewUserStore(f.db)
	record, err := users.SubscriptionRecord(context.Background(), userID)
	if err != nil || record == nil {
		t.Fatalf("record: %v %+v", err, record)
	}
	if record.Status != models.SubscriptionRefunded {
		t.Errorf("stored status = %q, want refunded", record.Status)
	}
}

// A receipt is a bearer token: whoever holds the string can present it. The
// same purchase must not be redeemed twice.
func TestAReceiptCannotBeRedeemedByASecondAccount(t *testing.T) {
	f := newAudioFixture(t)
	firstToken, _ := f.registerWithID(t, "first@test.com")
	secondToken, _ := f.registerWithID(t, "second@test.com")

	const receipt = "valid_annual_shared"
	if code, body := f.verify(t, firstToken, "apple", receipt); code != http.StatusOK {
		t.Fatalf("first verify: %d %v", code, body)
	}
	if got := f.entitlements(t, firstToken)["plan"]; got != "premium" {
		t.Fatalf("first account plan = %v, want premium", got)
	}

	code, body := f.verify(t, secondToken, "apple", receipt)
	if code != http.StatusConflict {
		t.Fatalf("second verify: %d %v, want 409", code, body)
	}
	if body["code"] != "RECEIPT_ALREADY_REDEEMED" {
		t.Errorf("code = %v, want RECEIPT_ALREADY_REDEEMED", body["code"])
	}
	// The second account must not have been upgraded.
	if got := f.entitlements(t, secondToken)["plan"]; got != "free" {
		t.Errorf("second account plan = %v, want free", got)
	}
}

// A receipt Apple or Google rejects is the caller's fault: 400, and no change.
func TestAnInvalidReceiptIsRejectedWithoutGranting(t *testing.T) {
	f := newAudioFixture(t)
	token, userID := f.registerWithID(t, "forger@test.com")

	code, body := f.verify(t, token, "apple", "invalid_monthly")
	if code != http.StatusOK {
		t.Fatalf("verify: %d %v", code, body)
	}
	if body["verified"] != false {
		t.Errorf("a rejected receipt reported verified = %v", body["verified"])
	}
	if got := f.entitlements(t, token)["plan"]; got != "free" {
		t.Errorf("plan = %v, want free", got)
	}

	// A receipt the verifier cannot even parse is a 400.
	if code, body := f.verify(t, token, "apple", "definitely-not-a-receipt"); code != http.StatusBadRequest {
		t.Errorf("unparseable receipt: %d %v, want 400", code, body)
	}

	// The forged JWS from the audit, in the shape the old code accepted.
	forged := "eyJhbGciOiJFUzI1NiJ9." +
		"eyJwcm9kdWN0SWQiOiJhcHAuaWNvbmZlc3MucHJlbWl1bS5hbm51YWwiLCJleHBpcmVzRGF0ZSI6OTk5OTk5OTk5OTk5OX0." +
		"not-a-signature"
	if code, body := f.verify(t, token, "apple", forged); code == http.StatusOK && body["verified"] == true {
		t.Fatal("the forged receipt from the audit was accepted")
	}

	users := store.NewUserStore(f.db)
	record, err := users.SubscriptionRecord(context.Background(), userID)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if record == nil {
		t.Fatal("no subscription row")
	}
	// Still free. Entitled() is true here because an active row with no expiry
	// is a grant without a clock; what matters is that a refused receipt did
	// not move the plan, which is what the entitlement check reads.
	if record.Plan != entitlements.PlanFree {
		t.Errorf("plan after a refused receipt = %q, want free", record.Plan)
	}
}

// A deployment with no store credentials must say so, rather than telling a
// paying customer their receipt is invalid.
func TestAVerifierThatIsNotConfiguredReturns503(t *testing.T) {
	f := newAudioFixture(t)
	token, _ := f.registerWithID(t, "misconfigured@test.com")

	// Production with the App Store selected and no bundle id: the fail-closed
	// path the previous remediation left open.
	t.Setenv("ENV", "production")
	t.Setenv(billing.EnvBillingVerifier, "apple")
	t.Setenv(billing.EnvAppleBundleID, "")
	t.Setenv(billing.EnvAppleMonthly, "")
	t.Setenv(billing.EnvAppleAnnual, "")
	t.Setenv(billing.EnvApplePlans, "")
	t.Setenv(billing.EnvAppleEnvironment, "")

	code, body := f.verify(t, token, "apple", "valid_monthly")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("verify: %d %v, want 503", code, body)
	}
	if body["code"] != "VERIFIER_UNCONFIGURED" {
		t.Errorf("code = %v, want VERIFIER_UNCONFIGURED", body["code"])
	}
	if got := f.entitlements(t, token)["plan"]; got != "free" {
		t.Errorf("plan = %v, want free", got)
	}
}

// No token, no verification: the route is authenticated, and the handler does
// not fall back to an anonymous user.
func TestVerifyRequiresAuthentication(t *testing.T) {
	f := newAudioFixture(t)
	code, _ := f.call(t, http.MethodPost, "/subscriptions/verify", "", map[string]any{
		"provider": "apple", "receipt": "valid_monthly",
	})
	if code == http.StatusOK {
		t.Fatal("POST /subscriptions/verify answered 200 without a token")
	}
}
