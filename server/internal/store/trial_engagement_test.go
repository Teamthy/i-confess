package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/analytics"
	"github.com/Teamthy/i-confess/internal/billing"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	trialdomain "github.com/Teamthy/i-confess/internal/trial"
)

// trialEngagementFixture wires the trial store to the analytics store it
// actually uses in production, so the funnel events are asserted against the
// table they are persisted to rather than against a test double. A double would
// prove the store called something; this proves the event can be queried later.
func trialEngagementFixture(t *testing.T) (*TrialStore, *AnalyticsStore, string) {
	t.Helper()
	db := dbtest.New(t)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	users := NewUserStore(db)
	analyticsStore := NewAnalyticsStore(db)
	trials := NewTrialStore(db).WithAnalytics(analyticsStore)
	userID := trialUser(t, users, "trial-engagement@example.com")
	if _, err := users.Subscription(ctx, userID); err != nil && !errors.Is(err, ErrNotFound) {
		t.Fatalf("subscription: %v", err)
	}
	return trials, analyticsStore, userID
}

// TestTrialDayCompletionRequiresARunningTrial proves a day cannot be claimed by
// an account that is not on a journey. Without this, a paid listener's ordinary
// listening would populate a trial funnel and the conversion rate would
// describe people who were never in the trial.
func TestTrialDayCompletionRequiresARunningTrial(t *testing.T) {
	trials, _, userID := trialEngagementFixture(t)
	ctx := context.Background()

	if _, err := trials.Current(ctx, userID); err != nil {
		t.Fatalf("current: %v", err)
	}
	at := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	if _, _, err := trials.CompleteDay(ctx, userID, "", at); !errors.Is(err, ErrTrialNotEligible) {
		t.Fatalf("CompleteDay on an ELIGIBLE trial = %v, want ErrTrialNotEligible", err)
	}
}

// TestTrialDayCompletionIsIdempotentPerDay is the uniqueness guarantee: two
// sessions finished on the same calendar day complete that day once. The second
// call is not an error — it returns the row that is actually stored, so a
// caller reporting "day 1 complete" never cites an id that matches nothing.
func TestTrialDayCompletionIsIdempotentPerDay(t *testing.T) {
	trials, _, userID := trialEngagementFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	if _, err := trials.Start(ctx, userID, at); err != nil {
		t.Fatalf("start: %v", err)
	}

	first, created, err := trials.CompleteDay(ctx, userID, "", at.Add(time.Hour))
	if err != nil || !created || first.Day != 1 {
		t.Fatalf("first completion = %+v created=%v err=%v, want day 1 created=true", first, created, err)
	}
	second, created, err := trials.CompleteDay(ctx, userID, "", at.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("second completion: %v", err)
	}
	if created {
		t.Error("second completion on the same day reported created=true, want false")
	}
	if second.ID != first.ID {
		t.Errorf("second completion returned id %q, want the stored row %q", second.ID, first.ID)
	}

	days, err := trials.CompletedDayNumbers(ctx, userID)
	if err != nil {
		t.Fatalf("completed days: %v", err)
	}
	if len(days) != 1 || days[0] != 1 {
		t.Fatalf("completed days = %v, want [1]", days)
	}
}

// TestTrialDayCompletionFollowsTheTrialClock proves the day number comes from
// the trial row rather than from the caller, so a listener cannot advance their
// own journey by asking for a later day.
func TestTrialDayCompletionFollowsTheTrialClock(t *testing.T) {
	trials, _, userID := trialEngagementFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	if _, err := trials.Start(ctx, userID, at); err != nil {
		t.Fatalf("start: %v", err)
	}

	for _, tc := range []struct {
		day    int
		offset time.Duration
	}{{1, 0}, {3, 48 * time.Hour}, {7, 6 * 24 * time.Hour}} {
		rec, created, err := trials.CompleteDay(ctx, userID, "", at.Add(tc.offset+time.Hour))
		if err != nil || !created {
			t.Fatalf("day %d: created=%v err=%v", tc.day, created, err)
		}
		if rec.Day != tc.day {
			t.Errorf("completion at +%v recorded day %d, want %d", tc.offset, rec.Day, tc.day)
		}
	}

	days, err := trials.CompletedDayNumbers(ctx, userID)
	if err != nil {
		t.Fatalf("completed days: %v", err)
	}
	if len(days) != 3 {
		t.Fatalf("completed days = %v, want 3 distinct days", days)
	}
}

// TestTrialDayCompletionStopsAfterExpiry closes the last hole in the funnel: an
// expired trial must not keep collecting day completions, or the completion
// rate would exceed the seven days the journey actually contains.
func TestTrialDayCompletionStopsAfterExpiry(t *testing.T) {
	trials, _, userID := trialEngagementFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	if _, err := trials.Start(ctx, userID, at); err != nil {
		t.Fatalf("start: %v", err)
	}
	after := at.Add(trialdomain.TrialDuration + time.Hour)
	if _, _, err := trials.CompleteDay(ctx, userID, "", after); !errors.Is(err, ErrTrialNotEligible) {
		t.Fatalf("CompleteDay after expiry = %v, want ErrTrialNotEligible", err)
	}
}

// TestTrialEngagementMeasuresTheJourney is the point of the phase: days
// completed is a count of real completions, not a clock reading. An account on
// day five that finished two sessions reports 2/7, not 5/7.
func TestTrialEngagementMeasuresTheJourney(t *testing.T) {
	trials, _, userID := trialEngagementFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	if _, err := trials.Start(ctx, userID, at); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, _, err := trials.CompleteDay(ctx, userID, "", at.Add(time.Hour)); err != nil {
		t.Fatalf("day 1: %v", err)
	}
	if _, _, err := trials.CompleteDay(ctx, userID, "", at.Add(2*24*time.Hour+time.Hour)); err != nil {
		t.Fatalf("day 3: %v", err)
	}

	// Day five of the clock, two days actually completed.
	eng, err := trials.Engagement(ctx, userID, at.Add(4*24*time.Hour+time.Hour))
	if err != nil {
		t.Fatalf("engagement: %v", err)
	}
	if eng.CurrentDay != 5 {
		t.Errorf("current day = %d, want 5", eng.CurrentDay)
	}
	if eng.DaysCompleted != 2 {
		t.Errorf("days completed = %d, want 2", eng.DaysCompleted)
	}
	if eng.DaysTotal != len(billing.TrialJourney) {
		t.Errorf("days total = %d, want %d", eng.DaysTotal, len(billing.TrialJourney))
	}
	wantRate := 2.0 / float64(len(billing.TrialJourney))
	if eng.CompletionRate != wantRate {
		t.Errorf("completion rate = %v, want %v", eng.CompletionRate, wantRate)
	}
	if eng.Funnel[analytics.EventTrialStarted] != 1 {
		t.Errorf("funnel trial_started = %d, want 1 (funnel=%v)", eng.Funnel[analytics.EventTrialStarted], eng.Funnel)
	}
	if eng.Funnel[analytics.EventTrialDayCompleted] != 0 {
		t.Errorf("funnel trial_day_completed = %d, want 0: completion events are recorded by the API, not the store", eng.Funnel[analytics.EventTrialDayCompleted])
	}
}

// TestTrialLifecycleEventsAreRecordedOnce proves the funnel's three lifecycle
// events are emitted by the store, exactly once each. Expiry is the important
// one: no handler reliably observes it, so if the transition did not record it
// the churn side of the funnel would be missing entirely.
func TestTrialLifecycleEventsAreRecordedOnce(t *testing.T) {
	trials, events, userID := trialEngagementFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)

	if _, err := trials.Start(ctx, userID, at); err != nil {
		t.Fatalf("start: %v", err)
	}
	// A repeated start is idempotent and must not count a second trial.
	if _, err := trials.Start(ctx, userID, at.Add(time.Hour)); err != nil {
		t.Fatalf("retry start: %v", err)
	}
	if got, err := events.Count(ctx, userID, analytics.EventTrialStarted); err != nil || got != 1 {
		t.Errorf("trial_started persisted %d times (err=%v), want 1", got, err)
	}

	if _, err := trials.Refresh(ctx, userID, at.Add(trialdomain.TrialDuration+time.Hour)); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	// Refreshing an already-expired trial is a no-op and must not re-record.
	if _, err := trials.Refresh(ctx, userID, at.Add(trialdomain.TrialDuration+2*time.Hour)); err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if got, err := events.Count(ctx, userID, analytics.EventTrialExpired); err != nil || got != 1 {
		t.Errorf("trial_expired persisted %d times (err=%v), want 1", got, err)
	}
}

// TestTrialConversionRecordsTheFunnel covers the conversion event and the day
// count it carries, which is what makes "converted on day 3" a reportable fact.
func TestTrialConversionRecordsTheFunnel(t *testing.T) {
	trials, analyticsStore, userID := trialEngagementFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	if _, err := trials.Start(ctx, userID, at); err != nil {
		t.Fatalf("start: %v", err)
	}
	for _, offset := range []time.Duration{time.Hour, 24*time.Hour + time.Hour} {
		if _, _, err := trials.CompleteDay(ctx, userID, "", at.Add(offset)); err != nil {
			t.Fatalf("completion at +%v: %v", offset, err)
		}
	}
	if _, err := trials.Convert(ctx, userID, at.Add(3*24*time.Hour)); err != nil {
		t.Fatalf("convert: %v", err)
	}

	n, err := analyticsStore.Count(ctx, userID, analytics.EventTrialConverted)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("trial_converted persisted %d times, want 1", n)
	}
	events, err := analyticsStore.Recent(ctx, userID, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	var found bool
	for _, ev := range events {
		if ev.Name != analytics.EventTrialConverted {
			continue
		}
		found = true
		if got := ev.Props["days_completed"]; got != float64(2) {
			t.Errorf("trial_converted days_completed = %v, want 2", got)
		}
	}
	if !found {
		t.Fatal("trial_converted is not in the persisted events")
	}
}

// TestAnalyticsStorePersistsWhatTheEndpointAcknowledges is the defect this
// phase removes from the other direction: POST /analytics/batch answered 202
// and handed the batch to a no-op sink, so the client was told its event was
// accepted and the server held no record of it.
func TestAnalyticsStorePersistsWhatTheEndpointAcknowledges(t *testing.T) {
	db := dbtest.New(t)
	defer db.Close()
	ctx := context.Background()
	users := NewUserStore(db)
	store := NewAnalyticsStore(db)
	userID := trialUser(t, users, "analytics@example.com")

	if err := store.Record(ctx, analytics.Event{
		Name:   analytics.EventSearchPerformed,
		UserID: userID,
		Props:  map[string]any{"kind": "confession"},
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	n, err := store.Count(ctx, userID, analytics.EventSearchPerformed)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("persisted %d events, want 1", n)
	}
	recent, err := store.Recent(ctx, userID, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(recent) != 1 || recent[0].Props["kind"] != "confession" {
		t.Fatalf("recent = %+v, want the recorded event with its props", recent)
	}
	// An event with no name is a programming error, not a row to store.
	if err := store.Record(ctx, analytics.Event{UserID: userID}); err == nil {
		t.Error("recording an event with no name succeeded, want an error")
	}
}

// TestTrialRefreshIgnoresABackwardClock is the regression behind the fix this
// test found. Once a trial has reached EXPIRING, a refresh carrying an earlier
// instant used to ask the state machine for EXPIRING->ACTIVE, which is not a
// lifecycle edge, and returned an error - so a plain status read became a 500.
// Clock readings arrive out of order in any multi-instance deployment; the row
// must keep the state it has already earned instead.
func TestTrialRefreshIgnoresABackwardClock(t *testing.T) {
	trials, _, userID := trialEngagementFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	if _, err := trials.Start(ctx, userID, at); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Inside the final 24 hours: the trial is EXPIRING.
	expiring, err := trials.Refresh(ctx, userID, at.Add(trialdomain.TrialDuration-time.Hour))
	if err != nil {
		t.Fatalf("refresh into expiring: %v", err)
	}
	if expiring.State != trialdomain.Expiring {
		t.Fatalf("state = %s, want EXPIRING", expiring.State)
	}

	// A clock that reads earlier must not move it back, and must not error.
	after, err := trials.Refresh(ctx, userID, at.Add(time.Hour))
	if err != nil {
		t.Fatalf("refresh with an earlier clock: %v", err)
	}
	if after.State != trialdomain.Expiring {
		t.Errorf("state after a backward clock = %s, want it to stay EXPIRING", after.State)
	}

	// And the forward edge still works afterwards: expiry is not lost.
	expired, err := trials.Refresh(ctx, userID, at.Add(trialdomain.TrialDuration+time.Hour))
	if err != nil {
		t.Fatalf("refresh into expired: %v", err)
	}
	if expired.State != trialdomain.Expired {
		t.Errorf("state = %s, want EXPIRED", expired.State)
	}
}
