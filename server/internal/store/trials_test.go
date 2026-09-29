package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/analytics"
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

	// Anchored to now rather than to a calendar date: a fixed start date
	// silently becomes an already-expired trial once the wall clock passes
	// it, and the entitlement projection reads the current time.
	at := time.Now().UTC().Add(-time.Hour)
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

func TestTrialExpirySweepDrainsDueRowsAndEmitsOnce(t *testing.T) {
	trials, events, firstID := trialEngagementFixture(t)
	ctx := context.Background()
	users := NewUserStore(trials.db)
	secondID := trialUser(t, users, "trial-sweep-second@example.com")
	futureID := trialUser(t, users, "trial-sweep-future@example.com")
	for _, userID := range []string{secondID, futureID} {
		if _, err := users.Subscription(ctx, userID); err != nil {
			t.Fatalf("ensure free subscription for %s: %v", userID, err)
		}
	}
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

	for i, userID := range []string{firstID, secondID} {
		start := at.Add(-trialdomain.TrialDuration - time.Duration(i+1)*time.Minute)
		if _, err := trials.Start(ctx, userID, start); err != nil {
			t.Fatalf("start expired trial %s: %v", userID, err)
		}
	}
	if _, err := trials.Start(ctx, futureID, at); err != nil {
		t.Fatalf("start future trial: %v", err)
	}

	// A limit of one proves the sweep can drain a backlog page by page.
	for i := 0; i < 2; i++ {
		if n, err := trials.SweepExpired(ctx, at, 1); err != nil || n != 1 {
			t.Fatalf("sweep page %d = %d, %v; want one expiry", i+1, n, err)
		}
	}
	if n, err := trials.SweepExpired(ctx, at, 1); err != nil || n != 0 {
		t.Fatalf("empty sweep = %d, %v; want no due trials", n, err)
	}

	for _, userID := range []string{firstID, secondID} {
		row, err := trials.Current(ctx, userID)
		if err != nil || row.State != trialdomain.Expired {
			t.Errorf("trial %s state = %v, %v; want EXPIRED", userID, row, err)
		}
		if plan, err := users.Subscription(ctx, userID); err != nil || plan != entitlements.PlanFree {
			t.Errorf("expired trial %s entitlement = %q, %v; want free", userID, plan, err)
		}
		if count, err := events.Count(ctx, userID, analytics.EventTrialExpired); err != nil || count != 1 {
			t.Errorf("trial_expired for %s = %d, %v; want exactly once", userID, count, err)
		}
	}
	if row, err := trials.Current(ctx, futureID); err != nil || row.State != trialdomain.Active {
		t.Errorf("future trial state = %v, %v; want ACTIVE", row, err)
	}
}

func TestConcurrentTrialExpirySweepsEmitOneTransitionEvent(t *testing.T) {
	trials, events, userID := trialEngagementFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	if _, err := trials.Start(ctx, userID, at.Add(-trialdomain.TrialDuration-time.Hour)); err != nil {
		t.Fatalf("start due trial: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			_, err := trials.SweepExpired(ctx, at, 500)
			errs <- err
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent sweep: %v", err)
		}
	}
	if count, err := events.Count(ctx, userID, analytics.EventTrialExpired); err != nil || count != 1 {
		t.Fatalf("trial_expired recorded %d times (err=%v), want exactly once", count, err)
	}
}

func TestTrialConversionDoesNotGrantPaidPremium(t *testing.T) {
	db := dbtest.New(t)
	defer db.Close()
	ctx := context.Background()
	users := NewUserStore(db)
	trials := NewTrialStore(db)
	userID := trialUser(t, users, "trial-convert@example.com")
	at := time.Now().UTC().Add(-time.Hour)
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
