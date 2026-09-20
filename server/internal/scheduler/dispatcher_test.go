package scheduler

import (
	"context"
	"errors"
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

// status reports what was recorded for a schedule occurrence.
func (f *fakeStore) status(scheduleID, key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.marks[scheduleID+"@"+key]
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

// ---------------------------------------------------------------------------
// Queued delivery (IC-012)
// ---------------------------------------------------------------------------
//
// The sweep decides who is due; the queue performs the send. These tests pin
// the two properties that make handing delivery off safe: the sweep stops
// talking to the provider, and every device gets exactly one queued job carrying
// the same message the inline path would have sent.

// fakeQueue records what the sweep asked to be delivered.
type fakeQueue struct {
	mu      sync.Mutex
	payload []map[string]any
	keys    []string
	seen    map[string]bool
	err     error
}

func newFakeQueue() *fakeQueue { return &fakeQueue{seen: map[string]bool{}} }

func (q *fakeQueue) EnqueueNotification(_ context.Context, payload map[string]any, dedupeKey string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	// Mirror the queue's own contract: a dedupe key already present is not a
	// second delivery. The real queue reports Jobs.ErrDuplicateJob and the
	// adapter turns that into success.
	if q.seen[dedupeKey] {
		return nil
	}
	q.seen[dedupeKey] = true
	q.payload = append(q.payload, payload)
	q.keys = append(q.keys, dedupeKey)
	return nil
}

func (q *fakeQueue) count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.payload)
}

func TestQueuedSweepDoesNotContactTheProvider(t *testing.T) {
	d, _, sender, _ := setup(t)
	q := newFakeQueue()
	d.Queue = q

	rep, err := d.Sweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Sent != 1 {
		t.Fatalf("queued delivery not counted: %+v", rep)
	}
	if n := len(sender.Sent()); n != 0 {
		t.Fatalf("the sweep sent %d notification(s) directly; the queue owns delivery now", n)
	}
	if q.count() != 1 {
		t.Fatalf("queued %d deliveries, want 1", q.count())
	}
}

// The recorded status must not claim a delivery that has not happened.
func TestQueuedOccurrenceIsRecordedAsQueued(t *testing.T) {
	d, store, _, fire := setup(t)
	d.Queue = newFakeQueue()

	if _, err := d.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}

	key, ok := Due(store.schedules[0], fire.Add(-time.Minute).UTC(), fire.UTC())
	if !ok {
		t.Fatal("the fixture schedule is not due, so this test asserts nothing")
	}
	if got := store.status(store.schedules[0].ID, key); got != "queued" {
		t.Fatalf("delivery status = %q, want \"queued\"", got)
	}
}

// The payload is the contract with the worker: it carries everything the
// provider call needs, and the two ids that let the handler clear a dead token.
func TestQueuedPayloadCarriesTheSameMessageAsInlineDelivery(t *testing.T) {
	d, store, _, _ := setup(t)
	q := newFakeQueue()
	d.Queue = q
	store.targets["u1"] = []Target{
		{DeviceID: "phone", Token: "t-ios", Platform: "ios"},
		{DeviceID: "tablet", Token: "t-android", Platform: "android"},
	}

	if _, err := d.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if q.count() != 2 {
		t.Fatalf("queued %d deliveries, want one per device", q.count())
	}

	for i, payload := range q.payload {
		if payload["token"] == "" || payload["platform"] == "" {
			t.Fatalf("payload %d has no destination: %+v", i, payload)
		}
		if payload["title"] == "" || payload["body"] == "" {
			t.Fatalf("payload %d has no message: %+v", i, payload)
		}
		data, _ := payload["data"].(map[string]string)
		if !strings.Contains(data["deeplink"], "/start") {
			t.Fatalf("payload %d lost the deep link: %+v", i, data)
		}
		if payload["user_id"] != "u1" || payload["device_id"] == "" {
			t.Fatalf("payload %d cannot be attributed to a device: %+v", i, payload)
		}
	}
}

// Tomorrow's reminder is not a duplicate of today's.
func TestQueuedDedupeKeyIsPerOccurrence(t *testing.T) {
	d, store, _, fire := setup(t)
	q := newFakeQueue()
	d.Queue = q

	store.schedules[0].Time = "06:00"
	store.schedules[0].DaysOfWeek = []int{0, 1, 2, 3, 4, 5, 6}

	if _, err := d.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}

	// The same schedule, the next morning.
	next := fire.Add(24 * time.Hour)
	d.Now = func() time.Time { return next.UTC() }
	d.lastSweep = fire.Add(time.Minute).UTC()
	if _, err := d.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}

	if q.count() != 2 {
		t.Fatalf("queued %d deliveries across two mornings, want 2", q.count())
	}
	if q.keys[0] == q.keys[1] {
		t.Fatalf("both occurrences share the dedupe key %q, so the second reminder would be swallowed", q.keys[0])
	}
}

// A queue that is down is a failed delivery, not a silent one.
func TestQueueFailureIsReportedAsFailed(t *testing.T) {
	d, store, _, fire := setup(t)
	q := newFakeQueue()
	q.err = errors.New("queue unavailable")
	d.Queue = q

	rep, err := d.Sweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Failed != 1 {
		t.Fatalf("a queue failure was not reported: %+v", rep)
	}

	key, _ := Due(store.schedules[0], fire.Add(-time.Minute).UTC(), fire.UTC())
	if got := store.status(store.schedules[0].ID, key); got != "failed" {
		t.Fatalf("delivery status = %q, want \"failed\"", got)
	}
}
