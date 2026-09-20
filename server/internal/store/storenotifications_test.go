package store

import (
	"context"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
)

// Tests for the store notification ledger (IC-003, PR B).
//
// The two properties that make a push stream safe to write entitlement from are
// tested here, because they are properties of the write rather than of the
// parsing:
//
//   - a replayed notification has one effect, not two;
//   - an out-of-order notification cannot move a subscription backwards.
//
// Both are checked against a real database, because the guarantees are enforced
// by a unique index and a transaction. A test with a fake store would assert
// that the test's own map behaves, which is the failure mode the audit called
// out: a suite that is green because the database was never involved.

func notificationFixture(t *testing.T) (*UserStore, string, StoreNotification) {
	t.Helper()
	users, userID := newSubscriptionFixture(t)
	ctx := context.Background()

	now := time.Now().UTC()
	// An active subscription as the verify endpoint would have written it:
	// Apple identity, a period running, and no store event applied yet.
	if err := users.SaveVerifiedSubscription(ctx, userID, VerifiedSubscription{
		Plan:                  "premium",
		Status:                models.SubscriptionActive,
		ExpiresAt:             now.Add(30 * 24 * time.Hour).Format(time.RFC3339),
		Provider:              "apple",
		ProviderTransactionID: "2000000123456789",
		OriginalTransactionID: "2000000123456789",
		ProductID:             "app.iconfess.premium.monthly",
		StoreEnvironment:      "Sandbox",
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	base := StoreNotification{
		Provider:              "apple",
		NotificationID:        "notification-1",
		NotificationType:      "DID_RENEW",
		OriginalTransactionID: "2000000123456789",
		ProductID:             "app.iconfess.premium.monthly",
		EventTime:             now.Format(time.RFC3339),
		Applies:               true,
		Plan:                  "premium",
		State:                 models.SubscriptionActive,
		ExpiresAt:             now.Add(31 * 24 * time.Hour).Format(time.RFC3339),
		Environment:           "Sandbox",
	}
	return users, userID, base
}

// TestReplayedNotificationHasOneEffect is the idempotency property. Both stores
// deliver at least once and retry until acknowledged, so a duplicate is not an
// edge case - it is the normal case for any endpoint that is at all slow.
func TestReplayedNotificationHasOneEffect(t *testing.T) {
	users, userID, note := notificationFixture(t)
	ctx := context.Background()

	first, err := users.ApplyStoreNotification(ctx, note)
	if err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if !first.Applied() {
		t.Fatalf("first delivery: %+v, want applied", first)
	}

	// The retry carries a renewal a month further out. If it were applied, the
	// duplicate would extend the subscription - so the test can tell a second
	// effect from no second effect.
	replay := note
	replay.ExpiresAt = time.Now().UTC().Add(365 * 24 * time.Hour).Format(time.RFC3339)
	second, err := users.ApplyStoreNotification(ctx, replay)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if second.Status != NotificationDuplicate {
		t.Fatalf("replay status = %q, want %q", second.Status, NotificationDuplicate)
	}
	if second.Applied() {
		t.Fatal("the replay reported itself as applied")
	}

	record, err := users.SubscriptionRecord(ctx, userID)
	if err != nil || record == nil {
		t.Fatalf("read subscription: %v", err)
	}
	if record.EndsAt == replay.ExpiresAt {
		t.Fatalf("the replay applied a second effect: ends_at = %s", record.EndsAt)
	}

	// The ledger holds exactly one row for the notification, which is what
	// makes a third delivery cheap rather than expensive.
	entries, err := users.StoreNotificationsFor(ctx, "apple", "2000000123456789")
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ledger has %d entries for one notification id, want 1", len(entries))
	}
}

// TestOutOfOrderNotificationDoesNotUndoNewerState is the ordering property.
//
// A retry that was delayed, or two deliveries racing, can deliver yesterday's
// DID_RENEW after today's EXPIRED. Applying them in arrival order leaves the
// subscription entitled after it has ended.
func TestOutOfOrderNotificationDoesNotUndoNewerState(t *testing.T) {
	users, userID, note := notificationFixture(t)
	ctx := context.Background()

	now := time.Now().UTC()
	// The newer event: the subscription ended an hour ago.
	expired := note
	expired.NotificationID = "notification-expired"
	expired.NotificationType = "EXPIRED"
	expired.EventTime = now.Format(time.RFC3339)
	expired.State = models.SubscriptionExpired
	expired.ExpiresAt = now.Add(-time.Hour).Format(time.RFC3339)

	if outcome, err := users.ApplyStoreNotification(ctx, expired); err != nil {
		t.Fatalf("apply expiry: %v", err)
	} else if !outcome.Applied() {
		t.Fatalf("expiry was not applied: %+v", outcome)
	}

	// The delayed one: a renewal from yesterday, arriving afterwards. Its
	// expiry is in the future and its state is active, so applying it would
	// resurrect a subscription that has ended.
	stale := note
	stale.NotificationID = "notification-renew-old"
	stale.EventTime = now.Add(-24 * time.Hour).Format(time.RFC3339)
	stale.ExpiresAt = now.Add(20 * 24 * time.Hour).Format(time.RFC3339)

	outcome, err := users.ApplyStoreNotification(ctx, stale)
	if err != nil {
		t.Fatalf("apply late renewal: %v", err)
	}
	if outcome.Status != NotificationStale {
		t.Fatalf("late renewal status = %q, want %q", outcome.Status, NotificationStale)
	}

	record, err := users.SubscriptionRecord(ctx, userID)
	if err != nil || record == nil {
		t.Fatalf("read subscription: %v", err)
	}
	if record.Status != models.SubscriptionExpired {
		t.Fatalf("status = %q, want expired: a late renewal undid a newer expiry", record.Status)
	}
	if plan, err := users.Subscription(ctx, userID); err != nil || plan != "free" {
		t.Fatalf("entitlement = %q (err %v), want free", plan, err)
	}
}

// TestNotificationArrivingBeforeTheReceiptIsRecordedNotApplied: the store can
// notify before any client has redeemed the purchase. There is no account to
// write, and attaching one by product id would be how a notification lands on
// the wrong subscriber.
func TestNotificationArrivingBeforeTheReceiptIsRecordedNotApplied(t *testing.T) {
	users, _ := newSubscriptionFixture(t)
	ctx := context.Background()

	outcome, err := users.ApplyStoreNotification(ctx, StoreNotification{
		Provider:              "apple",
		NotificationID:        "early-notification",
		NotificationType:      "SUBSCRIBED",
		OriginalTransactionID: "9999999999999999",
		EventTime:             time.Now().UTC().Format(time.RFC3339),
		Applies:               true,
		Plan:                  "premium",
		State:                 models.SubscriptionActive,
		ExpiresAt:             time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if outcome.Status != NotificationUnmatched {
		t.Fatalf("status = %q, want %q", outcome.Status, NotificationUnmatched)
	}
	// It is still recorded: the ledger is the audit trail, and Apple will not
	// send it again.
	entries, err := users.StoreNotificationsFor(ctx, "apple", "9999999999999999")
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ledger has %d entries, want the unmatched notification recorded", len(entries))
	}
}

// A notification that cannot change entitlement must not change it, and must
// still be recorded so a replay is recognised.
func TestInformationalNotificationChangesNothing(t *testing.T) {
	users, userID, note := notificationFixture(t)
	ctx := context.Background()

	before, err := users.SubscriptionRecord(ctx, userID)
	if err != nil || before == nil {
		t.Fatalf("read subscription: %v", err)
	}

	info := note
	info.NotificationID = "test-notification"
	info.NotificationType = "TEST"
	info.Applies = false
	info.State = ""
	info.ExpiresAt = ""

	outcome, err := users.ApplyStoreNotification(ctx, info)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if outcome.Status != NotificationIgnored {
		t.Fatalf("status = %q, want %q", outcome.Status, NotificationIgnored)
	}

	after, err := users.SubscriptionRecord(ctx, userID)
	if err != nil || after == nil {
		t.Fatalf("read subscription: %v", err)
	}
	if after.Status != before.Status || after.EndsAt != before.EndsAt {
		t.Fatalf("an informational notification changed the subscription: %+v -> %+v", before, after)
	}
}

// A refund revokes access even though the paid period is still running.
func TestRefundNotificationRevokesWhileThePeriodRuns(t *testing.T) {
	users, userID, note := notificationFixture(t)
	ctx := context.Background()

	refund := note
	refund.NotificationID = "refund-notification"
	refund.NotificationType = "REFUND"
	refund.State = models.SubscriptionRefunded
	refund.ExpiresAt = ""
	refund.EventTime = time.Now().UTC().Add(time.Minute).Format(time.RFC3339)

	outcome, err := users.ApplyStoreNotification(ctx, refund)
	if err != nil {
		t.Fatalf("apply refund: %v", err)
	}
	if !outcome.Applied() {
		t.Fatalf("refund was not applied: %+v", outcome)
	}

	record, err := users.SubscriptionRecord(ctx, userID)
	if err != nil || record == nil {
		t.Fatalf("read subscription: %v", err)
	}
	if record.Status != models.SubscriptionRefunded {
		t.Errorf("status = %q, want refunded", record.Status)
	}
	// The purchase itself is still recorded. "What was refunded?" is the first
	// question a support agent asks.
	if record.Plan != "premium" {
		t.Errorf("plan = %q, want the plan that was bought to be preserved", record.Plan)
	}
	if plan, err := users.Subscription(ctx, userID); err != nil || plan != "free" {
		t.Fatalf("entitlement = %q (err %v), want free", plan, err)
	}
}

// Play notifications name the purchase by its token, so that is what the row
// must be found by.
func TestGoogleNotificationMatchesOnThePurchaseToken(t *testing.T) {
	users, userID := newSubscriptionFixture(t)
	ctx := context.Background()

	if err := users.SaveVerifiedSubscription(ctx, userID, VerifiedSubscription{
		Plan:                  "premium",
		Status:                models.SubscriptionActive,
		ExpiresAt:             time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339),
		Provider:              "google",
		ProductID:             "premium_monthly",
		PurchaseToken:         "play-token-abc123",
		ProviderTransactionID: "GPA.3333",
		StoreEnvironment:      "Production",
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	outcome, err := users.ApplyStoreNotification(ctx, StoreNotification{
		Provider:         "google",
		NotificationID:   "pubsub-1",
		NotificationType: "12",
		Subtype:          "SUBSCRIPTION_REVOKED",
		PurchaseToken:    "play-token-abc123",
		EventTime:        time.Now().UTC().Format(time.RFC3339),
		Applies:          true,
		State:            models.SubscriptionRefunded,
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if outcome.UserID != userID {
		t.Fatalf("outcome = %+v, want it matched to the subscribing account", outcome)
	}

	if plan, err := users.Subscription(ctx, userID); err != nil || plan != "free" {
		t.Fatalf("entitlement = %q (err %v), want free after a revocation", plan, err)
	}
}

// A notification missing the fields the guarantees rest on is refused rather
// than stored with a fabricated value: an invented timestamp would silently win
// or lose against real events.
func TestNotificationValidation(t *testing.T) {
	users, _, note := notificationFixture(t)
	ctx := context.Background()

	cases := map[string]func(*StoreNotification){
		"no id":     func(n *StoreNotification) { n.NotificationID = "" },
		"no time":   func(n *StoreNotification) { n.EventTime = "" },
		"bad time":  func(n *StoreNotification) { n.EventTime = "yesterday" },
		"no state":  func(n *StoreNotification) { n.State = "" },
		"bad state": func(n *StoreNotification) { n.State = "sort of active" },
	}
	for name, mutate := range cases {
		bad := note
		bad.NotificationID = "validation-" + name
		mutate(&bad)
		if _, err := users.ApplyStoreNotification(ctx, bad); err == nil {
			t.Errorf("%s: notification was accepted", name)
		}
	}
}

// The unique index is the second line of defence: two different notification
// ids that name the same store event cannot both be applied by accident, and a
// purchase token cannot be moved between accounts.
func TestPurchaseTokenBindsOneAccount(t *testing.T) {
	users, first := newSubscriptionFixture(t)
	ctx := context.Background()

	second, err := users.Create(ctx, "second@test.com", "hash", "Second", "UTC")
	if err != nil {
		t.Fatalf("create second user: %v", err)
	}

	in := VerifiedSubscription{
		Plan:          "premium",
		Status:        models.SubscriptionActive,
		ExpiresAt:     time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339),
		Provider:      "google",
		ProductID:     "premium_monthly",
		PurchaseToken: "shared-token",
	}
	if err := users.SaveVerifiedSubscription(ctx, first, in); err != nil {
		t.Fatalf("first redemption: %v", err)
	}
	if err := users.SaveVerifiedSubscription(ctx, second.ID, in); err == nil {
		t.Fatal("the same Play purchase token was redeemed by a second account")
	}
}
