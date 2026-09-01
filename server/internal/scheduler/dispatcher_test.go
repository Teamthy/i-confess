package scheduler

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/push"
)

// Dispatch behaviour (§19, §47).

// fakeStore is an in-memory Store, so timing and delivery logic are tested
// without a database.
type fakeStore struct {
	mu        sync.Mutex
	schedules []Schedule
	claimed   map[string]bool
	marks     map[string]string
	targets   map[string][]Target
	optedOut  map[string]bool
	cleared   []string
	failures  []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		claimed: map[string]bool{}, marks: map[string]string{},
		targets: map[string][]Target{}, optedOut: map[string]bool{},
	}
}

func (f *fakeStore) DueSchedules(context.Context) ([]Schedule, error) { return f.schedules, nil }

func (f *fakeStore) ClaimDelivery(_ context.Context, scheduleID, _, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := scheduleID + "@" + key
	if f.claimed[k] {
		return false, nil
	}
	f.claimed[k] = true
	return true, nil
}

func (f *fakeStore) MarkDelivery(_ context.Context, scheduleID, key, status, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.marks[scheduleID+"@"+key] = status
	return nil
}

func (f *fakeStore) PushTargetsFor(_ context.Context, userID string) ([]Target, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.targets[userID], nil
}

func (f *fakeStore) ClearPushToken(_ context.Context, userID, deviceID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleared = append(f.cleared, userID+"/"+deviceID)
	return nil
}

func (f *fakeStore) RecordPushFailure(_ context.Context, userID, deviceID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures = append(f.failures, userID+"/"+deviceID)
	return nil
}

func (f *fakeStore) NotificationsEnabled(_ context.Context, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.optedOut[userID], nil
}

func setup(t *testing.T) (*Dispatcher, *fakeStore, *push.MemorySender, time.Time) {
	t.Helper()
	loc := lagos(t)
	store := newFakeStore()
	sender := push.NewMemorySender()

	store.schedules = []Schedule{weekdaySchedule("Africa/Lagos")}
	store.schedules[0].UserID = "u1"
	store.targets["u1"] = []Target{{DeviceID: "d1", Token: "tok-1", Platform: "ios"}}

	fire := time.Date(2026, 9, 2, 6, 0, 0, 0, loc) // Wednesday 06:00 Lagos
	d := New(store, sender)
	d.Now = func() time.Time { return fire.UTC() }
	d.lastSweep = fire.Add(-time.Minute).UTC()

	return d, store, sender, fire
}

func TestSweepDeliversDueSchedule(t *testing.T) {
	d, _, sender, _ := setup(t)

	rep, err := d.Sweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Sent != 1 {
		t.Fatalf("sent = %d, want 1 (report: %+v)", rep.Sent, rep)
	}

	msgs := sender.Sent()
	if len(msgs) != 1 {
		t.Fatalf("delivered %d notifications, want 1", len(msgs))
	}
	m := msgs[0]
	if m.Token != "tok-1" || m.Platform != push.PlatformIOS {
		t.Fatalf("wrong target: %+v", m)
	}
	// A tap must open the session, not the home screen.
	if !strings.Contains(m.Data["deeplink"], "/start") {
		t.Fatalf("no actionable deep link: %v", m.Data)
	}
	if m.Title == "" || m.Body == "" {
		t.Fatal("notification has no title or body")
	}
}

// The property that stops someone being woken twice.
func TestOccurrenceIsDeliveredOnlyOnce(t *testing.T) {
	d, store, sender, fire := setup(t)

	if _, err := d.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}

	// A second sweep covering the same moment, as happens on restart or with
	// a second replica.
	d.lastSweep = fire.Add(-time.Minute).UTC()
	rep, err := d.Sweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(sender.Sent()) != 1 {
		t.Fatalf("occurrence delivered %d times, want 1", len(sender.Sent()))
	}
	if rep.Skipped != 1 {
		t.Fatalf("duplicate was not reported as skipped: %+v", rep)
	}
	_ = store
}

// A user who turned reminders off must not be notified.
func TestRespectsNotificationPreference(t *testing.T) {
	d, store, sender, _ := setup(t)
	store.optedOut["u1"] = true

	rep, _ := d.Sweep(context.Background())
	if len(sender.Sent()) != 0 {
		t.Fatal("notified a user who disabled schedule reminders")
	}
	if rep.Skipped != 1 {
		t.Fatalf("opt-out not counted as skipped: %+v", rep)
	}

	// The occurrence is still recorded, so it is not retried every tick.
	if store.marks["s1@2026-09-02T06:00"] != "skipped" {
		t.Fatalf("occurrence not marked: %v", store.marks)
	}
}

// A dead token must be cleared, not retried forever.
func TestInvalidTokenIsCleared(t *testing.T) {
	d, store, _, _ := setup(t)
	dead := &erroringSender{err: push.InvalidToken("unregistered")}
	d.sender = dead

	if _, err := d.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.cleared) != 1 || store.cleared[0] != "u1/d1" {
		t.Fatalf("dead token not cleared: %v", store.cleared)
	}
	// A dead token is not a transient failure, so it must not inflate the
	// failure counter that eventually writes a device off.
	if len(store.failures) != 0 {
		t.Fatalf("invalid token counted as a transient failure: %v", store.failures)
	}
}

// A transient failure counts toward writing the device off, but does not clear
// a token that may still be good.
func TestTransientFailureIsCountedNotCleared(t *testing.T) {
	d, store, _, _ := setup(t)
	d.sender = &erroringSender{err: push.Retryable("apns 503")}

	rep, _ := d.Sweep(context.Background())
	if rep.Failed != 1 {
		t.Fatalf("failure not reported: %+v", rep)
	}
	if len(store.failures) != 1 {
		t.Fatalf("transient failure not counted: %v", store.failures)
	}
	if len(store.cleared) != 0 {
		t.Fatalf("a possibly-good token was cleared on a transient error: %v", store.cleared)
	}
}

// Every device the user owns should get the reminder.
func TestDeliversToAllDevices(t *testing.T) {
	d, store, sender, _ := setup(t)
	store.targets["u1"] = []Target{
		{DeviceID: "phone", Token: "t-ios", Platform: "ios"},
		{DeviceID: "tablet", Token: "t-android", Platform: "android"},
	}

	d.Sweep(context.Background())
	if len(sender.Sent()) != 2 {
		t.Fatalf("delivered to %d devices, want 2", len(sender.Sent()))
	}
}

// A partial failure still counts as delivered: the user got the reminder.
func TestPartialDeliveryCountsAsSent(t *testing.T) {
	d, store, _, _ := setup(t)
	store.targets["u1"] = []Target{
		{DeviceID: "good", Token: "ok", Platform: "ios"},
		{DeviceID: "bad", Token: "dead", Platform: "android"},
	}
	d.sender = &selectiveSender{failToken: "dead"}

	rep, _ := d.Sweep(context.Background())
	if rep.Sent != 1 {
		t.Fatalf("partial delivery not counted as sent: %+v", rep)
	}
}

// A user with no registered device must not be reported as a failure needing
// investigation — it is an expected state, not an error.
func TestNoDevicesIsNotAnError(t *testing.T) {
	d, store, _, _ := setup(t)
	store.targets["u1"] = nil

	rep, err := d.Sweep(context.Background())
	if err != nil {
		t.Fatalf("sweep errored on a user with no devices: %v", err)
	}
	if rep.Sent != 0 {
		t.Fatal("reported a delivery with no devices")
	}
}

// A long outage must not release a burst of stale reminders.
func TestLongOutageDoesNotFloodTheUser(t *testing.T) {
	loc := lagos(t)
	store := newFakeStore()
	sender := push.NewMemorySender()
	store.schedules = []Schedule{weekdaySchedule("Africa/Lagos")}
	store.schedules[0].UserID = "u1"
	store.targets["u1"] = []Target{{DeviceID: "d1", Token: "t", Platform: "ios"}}

	now := time.Date(2026, 9, 9, 7, 0, 0, 0, loc)
	d := New(store, sender)
	d.Now = func() time.Time { return now.UTC() }
	// Simulate a week of downtime.
	d.lastSweep = now.AddDate(0, 0, -7).UTC()

	d.Sweep(context.Background())
	if n := len(sender.Sent()); n > 1 {
		t.Fatalf("a week of downtime released %d notifications at once", n)
	}
}

// The first sweep after a cold start must not replay the recent past.
func TestFirstSweepLooksBackOnlyBriefly(t *testing.T) {
	loc := lagos(t)
	store := newFakeStore()
	sender := push.NewMemorySender()
	store.schedules = []Schedule{weekdaySchedule("Africa/Lagos")}
	store.schedules[0].UserID = "u1"
	store.targets["u1"] = []Target{{DeviceID: "d1", Token: "t", Platform: "ios"}}

	// Start up at 10:00, four hours after the 06:00 firing.
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, loc)
	d := New(store, sender)
	d.Now = func() time.Time { return now.UTC() }

	d.Sweep(context.Background())
	if len(sender.Sent()) != 0 {
		t.Fatal("a cold start replayed a firing from four hours earlier")
	}
}

type erroringSender struct{ err error }

func (e *erroringSender) Name() string { return "erroring" }
func (e *erroringSender) Send(context.Context, push.Notification) error {
	return e.err
}

type selectiveSender struct{ failToken string }

func (s *selectiveSender) Name() string { return "selective" }
func (s *selectiveSender) Send(_ context.Context, n push.Notification) error {
	if n.Token == s.failToken {
		return push.InvalidToken("gone")
	}
	return nil
}
