package store

import (
	"context"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/models"
)

// Tests for subscription storage (IC-003).
//
// The finding these close: entitlement was resolved from the status column
// alone, ends_at was never written or read, and SetSubscription was an
// unguarded UPDATE that touched every row for a user and inserted another when
// none existed. A subscription whose paid period had ended therefore stayed
// premium until something else rewrote the row, and nothing did.

func newSubscriptionFixture(t *testing.T) (*UserStore, string) {
	t.Helper()
	conn := dbtest.New(t)
	users := NewUserStore(conn)
	user, err := users.Create(context.Background(), "subscriber@test.com", "hash", "Subscriber", "Africa/Lagos")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return users, user.ID
}

func boolPtr(v bool) *bool { return &v }

// The clock is what made the old behaviour wrong: a row can say active and
// still grant nothing, because the store does not call back when a period ends.
func TestSubscriptionEntitlementNeedsStatusAndClock(t *testing.T) {
	users, userID := newSubscriptionFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()

	cases := []struct {
		name    string
		in      VerifiedSubscription
		want    string
		because string
	}{
		{
			name: "active with time left",
			in: VerifiedSubscription{Plan: "premium", Status: models.SubscriptionActive,
				ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339)},
			want: "premium",
		},
		{
			name: "active but the period ended",
			in: VerifiedSubscription{Plan: "premium", Status: models.SubscriptionActive,
				ExpiresAt: now.Add(-24 * time.Hour).Format(time.RFC3339)},
			want:    "free",
			because: "the row still reads active; only the clock says otherwise",
		},
		{
			name: "grace period with time left",
			in: VerifiedSubscription{Plan: "premium", Status: models.SubscriptionGrace,
				ExpiresAt: now.Add(5 * 24 * time.Hour).Format(time.RFC3339)},
			want: "premium",
		},
		{
			name: "cancelled but paid through the period",
			in: VerifiedSubscription{Plan: "premium", Status: models.SubscriptionCancelled,
				ExpiresAt: now.Add(10 * 24 * time.Hour).Format(time.RFC3339)},
			want:    "premium",
			because: "cancel means do not renew, not revoke what was paid for",
		},
		{
			name: "cancelled and the period ended",
			in: VerifiedSubscription{Plan: "premium", Status: models.SubscriptionCancelled,
				ExpiresAt: now.Add(-time.Hour).Format(time.RFC3339)},
			want: "free",
		},
		{
			name: "refunded with time left on the clock",
			in: VerifiedSubscription{Plan: "premium", Status: models.SubscriptionRefunded,
				ExpiresAt: now.Add(300 * 24 * time.Hour).Format(time.RFC3339)},
			want:    "free",
			because: "a refunded annual subscription keeps its expiry date",
		},
		{
			name: "suspended",
			in: VerifiedSubscription{Plan: "premium", Status: models.SubscriptionSuspended,
				ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339)},
			want: "free",
		},
		{
			name: "expired",
			in: VerifiedSubscription{Plan: "premium", Status: models.SubscriptionExpired,
				ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339)},
			want: "free",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := users.SaveVerifiedSubscription(ctx, userID, tc.in); err != nil {
				t.Fatalf("save: %v", err)
			}
			got, err := users.Subscription(ctx, userID)
			if err != nil {
				t.Fatalf("subscription: %v", err)
			}
			if got != tc.want {
				t.Errorf("plan = %q, want %q", got, tc.want)
				if tc.because != "" {
					t.Logf("why: %s", tc.because)
				}
			}
		})
	}
}

// A row with no expiry is an administrative grant without a clock, and must
// keep working: the entitlement check has to distinguish "no end date" from
// "end date in the past".
func TestSubscriptionWithoutAnExpiryKeepsEntitling(t *testing.T) {
	users, userID := newSubscriptionFixture(t)
	ctx := context.Background()

	if err := users.SetSubscription(ctx, userID, "premium", "active"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := users.Subscription(ctx, userID)
	if err != nil {
		t.Fatalf("subscription: %v", err)
	}
	if got != "premium" {
		t.Fatalf("plan = %q, want premium for a grant with no expiry", got)
	}
}

// The administrative override clears the store's expiry. Leaving it in place
// would silently re-expire the plan the operator just granted.
func TestAdministrativeGrantOverridesAStoreExpiry(t *testing.T) {
	users, userID := newSubscriptionFixture(t)
	ctx := context.Background()

	if err := users.SaveVerifiedSubscription(ctx, userID, VerifiedSubscription{
		Plan: "premium", Status: models.SubscriptionActive,
		ExpiresAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		Provider:  "apple", OriginalTransactionID: "tx-1",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if plan, _ := users.Subscription(ctx, userID); plan != "free" {
		t.Fatalf("plan = %q, want free before the grant", plan)
	}

	if err := users.SetSubscription(ctx, userID, "premium", "active"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if plan, _ := users.Subscription(ctx, userID); plan != "premium" {
		t.Fatal("the administrative grant did not take effect")
	}
}

// One row per user, and repeated verification must not create a second one: the
// renewal path runs this on every launch and every webhook.
func TestSaveVerifiedSubscriptionUpsertsOneRow(t *testing.T) {
	users, userID := newSubscriptionFixture(t)
	ctx := context.Background()

	first := VerifiedSubscription{
		Plan: "premium", Status: models.SubscriptionActive,
		ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339),
		Provider:  "google", ProviderTransactionID: "order-1",
		OriginalTransactionID: "orig-1", ProductID: "premium_monthly",
		StoreEnvironment: "Production", AutoRenew: boolPtr(true),
	}
	if err := users.SaveVerifiedSubscription(ctx, userID, first); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// A renewal: same purchase, later expiry.
	renewed := first
	renewed.ExpiresAt = time.Now().UTC().Add(60 * 24 * time.Hour).Format(time.RFC3339)
	renewed.ProviderTransactionID = "order-2"
	if err := users.SaveVerifiedSubscription(ctx, userID, renewed); err != nil {
		t.Fatalf("renewal save: %v", err)
	}

	record, err := users.SubscriptionRecord(ctx, userID)
	if err != nil || record == nil {
		t.Fatalf("record: %v", err)
	}
	if record.ProviderTransactionID != "order-2" {
		t.Errorf("transaction id = %q, want the renewal's", record.ProviderTransactionID)
	}
	if record.OriginalTransactionID != "orig-1" {
		t.Errorf("original transaction id = %q, want it preserved across renewal", record.OriginalTransactionID)
	}
	if record.AutoRenew == nil || !*record.AutoRenew {
		t.Errorf("auto-renew = %v, want true", record.AutoRenew)
	}

	var rows int
	if err := users.db.QueryRowContext(ctx,
		`SELECT count(*) FROM subscriptions WHERE user_id = ?`, userID).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Fatalf("%d subscription rows for one user, want 1", rows)
	}
}

// A receipt is a bearer token. Binding the store's original transaction id to
// an account is what stops the same purchase being redeemed by every account
// that obtains a copy of the string.
func TestSubscriptionOwnerBindsAPurchaseToOneAccount(t *testing.T) {
	users, userID := newSubscriptionFixture(t)
	ctx := context.Background()

	if err := users.SaveVerifiedSubscription(ctx, userID, VerifiedSubscription{
		Plan: "premium", Status: models.SubscriptionActive,
		ExpiresAt:             time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		Provider:              "apple",
		OriginalTransactionID: "2000000123456789",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	owner, found, err := users.SubscriptionOwner(ctx, "apple", "2000000123456789")
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	if !found || owner != userID {
		t.Fatalf("owner = (%q, %v), want (%q, true)", owner, found, userID)
	}

	// The same purchase must not appear unclaimed for another provider or id.
	for _, probe := range []struct{ provider, id string }{
		{"google", "2000000123456789"},
		{"apple", "9999999999999999"},
		{"", ""},
	} {
		owner, found, err := users.SubscriptionOwner(ctx, probe.provider, probe.id)
		if err != nil {
			t.Fatalf("owner(%q,%q): %v", probe.provider, probe.id, err)
		}
		if found {
			t.Errorf("(%q,%q) reported as owned by %q", probe.provider, probe.id, owner)
		}
	}
}

// The unique index from migration 0010 is the backstop for the same rule: a
// second account cannot store the purchase even if the check is bypassed.
func TestTheDatabaseRefusesASecondAccountForOnePurchase(t *testing.T) {
	conn := dbtest.New(t)
	users := NewUserStore(conn)
	ctx := context.Background()

	first, err := users.Create(ctx, "first@test.com", "hash", "First", "Africa/Lagos")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := users.Create(ctx, "second@test.com", "hash", "Second", "Africa/Lagos")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	sub := VerifiedSubscription{
		Plan: "premium", Status: models.SubscriptionActive,
		ExpiresAt:             time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		Provider:              "apple",
		OriginalTransactionID: "2000000987654321",
	}
	if err := users.SaveVerifiedSubscription(ctx, first.ID, sub); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := users.SaveVerifiedSubscription(ctx, second.ID, sub); err == nil {
		t.Fatal("the same purchase was stored against a second account")
	}
}

// An expired row must not be reported to a client as active: the client shows
// "Premium" over it, and the user reports a bug in a product that is behaving
// correctly.
func TestSubscriptionStateReportsTheClockNotJustTheColumn(t *testing.T) {
	users, userID := newSubscriptionFixture(t)
	ctx := context.Background()

	if err := users.SaveVerifiedSubscription(ctx, userID, VerifiedSubscription{
		Plan: "premium", Status: models.SubscriptionActive,
		ExpiresAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		Provider:  "apple", OriginalTransactionID: "tx-old",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	plan, status, err := users.SubscriptionState(ctx, userID)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if status != models.SubscriptionExpired {
		t.Errorf("status = %q, want expired once the period has ended", status)
	}
	if plan != "premium" {
		t.Errorf("plan = %q, want the stored plan so a client can say what lapsed", plan)
	}
}

// An unparsable expiry must not be read as "no expiry": that turns a corrupt
// row into a permanent subscription.
func TestSubscriptionTreatsACorruptExpiryAsExpired(t *testing.T) {
	users, userID := newSubscriptionFixture(t)
	ctx := context.Background()

	if err := users.SaveVerifiedSubscription(ctx, userID, VerifiedSubscription{
		Plan: "premium", Status: models.SubscriptionActive,
		ExpiresAt: "not a timestamp",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if plan, err := users.Subscription(ctx, userID); err != nil {
		t.Fatalf("subscription: %v", err)
	} else if plan != "free" {
		t.Errorf("plan = %q, want free for a row with an unparsable expiry", plan)
	}
}

// Every user is created with a free subscription row, so the ordinary state for
// someone who has never paid is a row that says free - not the absence of a
// row. Both must resolve to free rather than to an error.
func TestSubscriptionForANewUserIsFree(t *testing.T) {
	users, userID := newSubscriptionFixture(t)
	ctx := context.Background()

	record, err := users.SubscriptionRecord(ctx, userID)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if record == nil {
		t.Fatal("a new user has no subscription row - Create is supposed to add a free one")
	}
	if record.Plan != "free" || record.Status != models.SubscriptionActive {
		t.Errorf("new user row = (%q, %q), want (free, active)", record.Plan, record.Status)
	}
	if plan, err := users.Subscription(ctx, userID); err != nil || plan != "free" {
		t.Fatalf("Subscription = (%q, %v), want (free, nil)", plan, err)
	}
	if plan, status, err := users.SubscriptionState(ctx, userID); err != nil || plan != "free" || status != "active" {
		t.Fatalf("SubscriptionState = (%q, %q, %v), want (free, active, nil)", plan, status, err)
	}

	// A user whose row is genuinely missing is still free, not an error: a row
	// can be absent after a partial restore, and pricing someone out because of
	// it would be worse than the data problem.
	if _, err := users.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE user_id = ?`, userID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if plan, err := users.Subscription(ctx, userID); err != nil || plan != "free" {
		t.Fatalf("Subscription without a row = (%q, %v), want (free, nil)", plan, err)
	}
	if plan, status, err := users.SubscriptionState(ctx, userID); err != nil || plan != "free" || status != "none" {
		t.Fatalf("SubscriptionState without a row = (%q, %q, %v), want (free, none, nil)", plan, status, err)
	}
	if record, err := users.SubscriptionRecord(ctx, userID); err != nil || record != nil {
		t.Fatalf("SubscriptionRecord without a row = (%+v, %v), want (nil, nil)", record, err)
	}
}
