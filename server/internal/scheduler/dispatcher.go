package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/Teamthy/i-confess/internal/push"
)

// Dispatcher finds due schedules and sends their reminders (§19, §47).
//
// It owns no transport of its own: delivery goes through a push.Sender, and
// token lifecycle goes through the Store interface. That keeps the timing
// logic testable without a provider account or a database.

// Store is the persistence this package needs.
type Store interface {
	// DueSchedules returns enabled schedules to evaluate.
	DueSchedules(ctx context.Context) ([]Schedule, error)
	// ClaimDelivery reserves an occurrence, returning false if already sent.
	ClaimDelivery(ctx context.Context, scheduleID, userID, occurrenceKey string) (bool, error)
	// MarkDelivery records the outcome.
	MarkDelivery(ctx context.Context, scheduleID, occurrenceKey, status, detail string) error
	// PushTargetsFor returns a user's deliverable devices.
	PushTargetsFor(ctx context.Context, userID string) ([]Target, error)
	// ClearPushToken removes a token the provider rejected.
	ClearPushToken(ctx context.Context, userID, deviceID string) error
	// RecordPushFailure counts a transient failure.
	RecordPushFailure(ctx context.Context, userID, deviceID string) error
	// NotificationsEnabled reports whether the user wants schedule reminders.
	NotificationsEnabled(ctx context.Context, userID string) (bool, error)
}

// Target is a deliverable device.
type Target struct {
	DeviceID string
	Token    string
	Platform string
}

// Dispatcher runs the schedule sweep.
type Dispatcher struct {
	store  Store
	sender push.Sender
	Now    func() time.Time

	// lastSweep bounds the window each run examines.
	lastSweep time.Time
}

// New creates a dispatcher.
func New(store Store, sender push.Sender) *Dispatcher {
	return &Dispatcher{store: store, sender: sender, Now: time.Now}
}

func (d *Dispatcher) now() time.Time {
	if d.Now != nil {
		return d.Now().UTC()
	}
	return time.Now().UTC()
}

// Report summarises one sweep.
type Report struct {
	Evaluated int
	Sent      int
	Skipped   int
	Failed    int
}

// Sweep evaluates every schedule and dispatches what is due.
//
// The window is (lastSweep, now]. On the first run lastSweep is unset, so it
// looks back only a short way: a service starting after a week of downtime
// should not deliver a week of stale 6 AM reminders at once.
func (d *Dispatcher) Sweep(ctx context.Context) (Report, error) {
	now := d.now()
	after := d.lastSweep
	if after.IsZero() {
		after = now.Add(-15 * time.Minute)
	}
	// Cap the look-back for the same reason: a long outage must not produce a
	// burst of notifications for moments that have passed.
	if now.Sub(after) > 2*time.Hour {
		after = now.Add(-2 * time.Hour)
	}
	d.lastSweep = now

	var rep Report
	schedules, err := d.store.DueSchedules(ctx)
	if err != nil {
		return rep, err
	}

	for _, s := range schedules {
		rep.Evaluated++

		key, due := Due(s, after, now)
		if !due {
			continue
		}

		// Claim before sending. If the insert loses the race, another sweeper
		// already has this occurrence and we must not send a second time.
		claimed, err := d.store.ClaimDelivery(ctx, s.ID, s.UserID, key)
		if err != nil {
			log.Printf("scheduler: claim failed for %s@%s: %v", s.ID, key, err)
			rep.Failed++
			continue
		}
		if !claimed {
			rep.Skipped++
			continue
		}

		// Preference is checked after claiming so an opted-out user's
		// occurrence is still recorded as handled and not retried every tick.
		wants, err := d.store.NotificationsEnabled(ctx, s.UserID)
		if err == nil && !wants {
			_ = d.store.MarkDelivery(ctx, s.ID, key, "skipped", "user disabled schedule notifications")
			rep.Skipped++
			continue
		}

		sent, detail := d.deliver(ctx, s, key)
		if sent > 0 {
			rep.Sent++
			_ = d.store.MarkDelivery(ctx, s.ID, key, "sent", detail)
		} else {
			rep.Failed++
			_ = d.store.MarkDelivery(ctx, s.ID, key, "failed", detail)
		}
	}
	return rep, nil
}

// deliver sends to every device the user has, returning how many succeeded.
func (d *Dispatcher) deliver(ctx context.Context, s Schedule, key string) (sent int, detail string) {
	targets, err := d.store.PushTargetsFor(ctx, s.UserID)
	if err != nil {
		return 0, "failed to load devices: " + err.Error()
	}
	if len(targets) == 0 {
		return 0, "no registered devices"
	}

	label := s.Label
	if label == "" {
		label = "Your confession session"
	}
	minutes := s.DurationSeconds / 60
	if minutes < 1 {
		minutes = 1
	}

	lastErr := ""
	for _, t := range targets {
		n := push.Notification{
			Token:    t.Token,
			Platform: push.Platform(t.Platform),
			Title:    label,
			Body:     "Your " + itoa(minutes) + "-minute session is ready.",
			Data: map[string]string{
				// A deep link so tapping the notification opens the session
				// rather than the home screen.
				"deeplink":    "iconfess://schedules/" + s.ID + "/start",
				"schedule_id": s.ID,
			},
			// Collapse on the occurrence: if yesterday's reminder is still
			// undelivered, today's replaces it rather than stacking.
			CollapseKey: "sched-" + s.ID,
		}

		err := d.sender.Send(ctx, n)
		switch {
		case err == nil:
			sent++
		case push.IsInvalidToken(err):
			// The device is gone. Clearing the token is the correct response;
			// retrying would fail identically forever.
			log.Printf("scheduler: clearing dead token for user=%s device=%s", s.UserID, t.DeviceID)
			_ = d.store.ClearPushToken(ctx, s.UserID, t.DeviceID)
			lastErr = "device token no longer valid"
		default:
			_ = d.store.RecordPushFailure(ctx, s.UserID, t.DeviceID)
			lastErr = err.Error()
		}
	}

	if sent > 0 {
		return sent, "delivered to " + itoa(sent) + " device(s)"
	}
	return 0, lastErr
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ResetWindow rewinds the sweep window.
//
// Exists so a test can simulate the restart / second-replica case where two
// sweeps cover the same moment. Production never calls it: correctness there
// rests on the occurrence key, not on window bookkeeping.
func (d *Dispatcher) ResetWindow(to time.Time) { d.lastSweep = to }
