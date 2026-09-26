package api

import (
	"context"
	"errors"

	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/push"
	"github.com/Teamthy/i-confess/internal/scheduler"
	"github.com/Teamthy/i-confess/internal/workers"
)

// pushSender is the transport the dispatcher delivers through.
type pushSender = push.Sender

// schedulerStore adapts the concrete stores to the scheduler's Store interface.
//
// The adapter exists so the scheduler depends on a narrow interface it defines
// rather than on the whole persistence layer, which is what lets its timing
// logic be tested with a fake and no database.
type schedulerStore struct{ h *Handler }

// DueSchedules returns every enabled schedule for evaluation.
//
// It deliberately does not filter by time in SQL: "is this due?" depends on the
// user's timezone and DST, which the database cannot reason about. Filtering
// happens in Go where the IANA rules are available.
func (s schedulerStore) DueSchedules(ctx context.Context) ([]scheduler.Schedule, error) {
	rows, err := s.h.sched.AllEnabled(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.Schedule, 0, len(rows))
	for _, r := range rows {
		out = append(out, scheduler.Schedule{
			ID: r.ID, UserID: r.UserID, Label: r.Label,
			Time:            r.Time,
			DaysOfWeek:      r.DaysOfWeek,
			Timezone:        r.Timezone,
			DurationSeconds: r.DurationSeconds,
			Enabled:         r.Enabled,
		})
	}
	return out, nil
}

func (s schedulerStore) ClaimDelivery(ctx context.Context, scheduleID, userID, key string) (bool, error) {
	return s.h.library.ClaimDelivery(ctx, scheduleID, userID, key)
}

func (s schedulerStore) MarkDelivery(ctx context.Context, scheduleID, key, status, detail string) error {
	return s.h.library.MarkDelivery(ctx, scheduleID, key, status, detail)
}

func (s schedulerStore) PushTargetsFor(ctx context.Context, userID string) ([]scheduler.Target, error) {
	targets, err := s.h.library.PushTargetsFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.Target, 0, len(targets))
	for _, t := range targets {
		out = append(out, scheduler.Target{
			DeviceID: t.DeviceID, Token: t.Token, Platform: t.Platform,
		})
	}
	return out, nil
}

func (s schedulerStore) ClearPushToken(ctx context.Context, userID, deviceID string) error {
	return s.h.library.ClearPushToken(ctx, userID, deviceID)
}

func (s schedulerStore) RecordPushFailure(ctx context.Context, userID, deviceID string) error {
	return s.h.library.RecordPushFailure(ctx, userID, deviceID)
}

// NotificationsEnabled reports whether the user wants schedule reminders.
//
// Defaults to true on error: a database hiccup should not silently stop a
// user's morning routine, and an unwanted notification is more recoverable
// than a missed one they were relying on.
func (s schedulerStore) NotificationsEnabled(ctx context.Context, userID string) (bool, error) {
	prefs, err := s.h.library.NotificationPreferences(ctx, userID)
	if err != nil {
		return true, nil
	}
	return prefs.ScheduledSessions, nil
}

// SetPushSender installs the push transport and builds the dispatcher.
func (h *Handler) SetPushSender(sender pushSender) {
	if sender == nil {
		return
	}
	h.dispatcher = scheduler.New(schedulerStore{h: h}, sender)
}

// pushJobQueue defers reminder delivery to the durable job queue.
//
// It exists so the scheduler never has to know a job type name, and so the
// queue's duplicate-idempotency-key error can be swallowed in exactly one
// place: a key that is already queued means the delivery it wanted is on its
// way, which is the outcome, not a failure.
type pushJobQueue struct{ queue jobs.Queue }

func (q pushJobQueue) EnqueueNotification(ctx context.Context, payload map[string]any, dedupeKey string) error {
	_, err := q.queue.Enqueue(ctx, jobs.Job{
		Type:           workers.TypeNotificationSend,
		Payload:        payload,
		IdempotencyKey: dedupeKey,
	})
	if errors.Is(err, jobs.ErrDuplicateJob) {
		return nil
	}
	return err
}

// SetPushQueue routes scheduled reminders through the job queue instead of
// sending them inline during the sweep.
//
// Called after SetPushSender. Without it the sweep talks to APNs and FCM itself,
// which works until the first provider slowdown: the sweep then runs longer
// than its own tick, and a restart mid-send loses the delivery with nothing
// recorded but a row that says "sent".
func (h *Handler) SetPushQueue() bool {
	if h.dispatcher == nil || h.queue == nil {
		return false
	}
	h.dispatcher.Queue = pushJobQueue{queue: h.queue}
	return true
}

// ClearPushToken forgets a token a push provider rejected as dead.
//
// This is the worker's view of the same operation the sweep performs: the
// queue delivers reminders in production, so the queue handler is the
// component that learns a token is gone and has to act on it.
func (h *Handler) ClearPushToken(ctx context.Context, userID, deviceID string) error {
	return h.library.ClearPushToken(ctx, userID, deviceID)
}

// RecordPushFailure counts a delivery the provider rejected.
func (h *Handler) RecordPushFailure(ctx context.Context, userID, deviceID string) error {
	return h.library.RecordPushFailure(ctx, userID, deviceID)
}

// RunScheduleSweep dispatches any schedules that have become due.
//
// Exported so main can drive it on a ticker, and so a test can drive it
// deterministically without waiting on wall-clock time.
func (h *Handler) RunScheduleSweep(ctx context.Context) (scheduler.Report, error) {
	if h.dispatcher == nil {
		return scheduler.Report{}, nil
	}
	return h.dispatcher.Sweep(ctx)
}
