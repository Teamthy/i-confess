package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/entitlements"
	"github.com/Teamthy/i-confess/internal/models"
	trialdomain "github.com/Teamthy/i-confess/internal/trial"
)

func trialUser(t *testing.T, users *UserStore, email string) string {
	t.Helper()
	u, err := users.Create(context.Background(), email, "hash", "Trial", "UTC")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u.ID
}

// TestTrialLifecycle exercises the persistent graph and the entitlement
// projection together: an account starts eligible, receives exactly one
// seven-day premium trial, loses it at expiry, and cannot start again.
func TestTrialLifecycle(t *testing.T) {
	db := dbtest.New(t)
	defer db.Close()
	ctx := context.Background()
	users := NewUserStore(db)
	trials := NewTrialStore(db)
	userID := trialUser(t, users, "trial-lifecycle@example.com")

	at := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	eligible, err := trials.Current(ctx, userID)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if eligible.State != trialdomain.Eligible {
		t.Fatalf("initial state = %s, want ELIGIBLE", eligible.State)
	}
	if plan, err := users.Subscription(ctx, userID); err != nil || plan != entitlements.PlanFree {
		t.Fatalf("initial plan = %q, %v, want free", plan, err)
	}

	running, err := trials.Start(ctx, userID, at)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if running.State != trialdomain.Active {
		t.Fatalf("started state = %s, want ACTIVE", running.State)
	}
	if plan, err := users.Subscription(ctx, userID); err != nil || plan != entitlements.PlanPremium {
		t.Fatalf("running trial plan = %q, %v, want premium", plan, err)
	}
	sub, err := users.SubscriptionRecord(ctx, userID)
	if err != nil || sub == nil {
		t.Fatalf("subscription: %v %+v", err, sub)
	}
	if sub.Plan != entitlements.PlanPremium || sub.Status != models.SubscriptionTrial {
		t.Fatalf("trial projection = %s/%s, want premium/trial", sub.Plan, sub.Status)
	}

	// Repeating start is idempotent and does not extend the expiry.
	retried, err := trials.Start(ctx, userID, at.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("retry start: %v", err)
	}
	if retried.ExpiresAt != running.ExpiresAt {
		t.Fatalf("retry changed expiry from %q to %q", running.ExpiresAt, retried.ExpiresAt)
	}

	expiredAt := at.Add(trialdomain.TrialDuration).Add(time.Minute)
	expired, err := trials.Refresh(ctx, userID, expiredAt)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if expired.State != trialdomain.Expired {
		t.Fatalf("expired state = %s, want EXPIRED", expired.State)
	}
	if plan, err := users.Subscription(ctx, userID); err != nil || plan != entitlements.PlanFree {
		t.Fatalf("expired trial plan = %q, %v, want free", plan, err)
	}
	if _, err := trials.Start(ctx, userID, expiredAt.Add(time.Hour)); !errors.Is(err, ErrTrialNotEligible) {
		t.Fatalf("restart error = %v, want ErrTrialNotEligible", err)
	}
}

func TestTrialConversionDoesNotGrantPaidPremium(t *testing.T) {
	db := dbtest.New(t)
	defer db.Close()
	ctx := context.Background()
	users := NewUserStore(db)
	trials := NewTrialStore(db)
	userID := trialUser(t, users, "trial-convert@example.com")
	at := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	if _, err := trials.Start(ctx, userID, at); err != nil {
		t.Fatalf("start: %v", err)
	}
	converted, err := trials.Convert(ctx, userID, at.Add(time.Hour))
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if converted.State != trialdomain.Converted {
		t.Fatalf("state = %s, want CONVERTED", converted.State)
	}
	if plan, err := users.Subscription(ctx, userID); err != nil || plan != entitlements.PlanFree {
		t.Fatalf("converted plan = %q, %v, want free until a verified purchase", plan, err)
	}
}
