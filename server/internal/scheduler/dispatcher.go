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

// Queue defers a delivery to something that outlives this process.
//
// The sweep is the wrong place to talk to APNs. A provider call takes as long
// as it takes, the sweep runs every minute on a ticker, and a delivery that was
// handed to the network when the process was restarted is gone with it. With a
// queue installed the sweep's only job is to decide *who* is due; retries,
// backoff and dead-lettering belong to the queue, which is what production runs.
//
// The interface is deliberately one method wide and names no job type: the
// adapter supplies that, so this package stays independent of how the queue
// names its work.
type Queue interface {
	// EnqueueNotification queues one delivery. The dedupe key is the
	// occurrence and the device, so a repeated enqueue of the same reminder
	// to the same phone is a no-op rather than a second notification.
	EnqueueNotification(ctx context.Context, payload map[string]any, dedupeKey string) error
}

// Dispatcher runs the schedule sweep.
type Dispatcher struct {
	store  Store
	sender push.Sender
	Now    func() time.Time

	// Queue, when set, takes ownership of delivery. Sender is then unused by
	// the sweep - it stays for the inline path, which is what development and
	// tests run on, where a queue would only hide the notification.
	Queue Queue

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

		// "queued" rather than "sent" when a queue owns delivery: the
		// occurrence is handled - the queue will not be asked for it again -
		// but nobody has reached a phone yet, and claiming otherwise would
		// make the delivery log useless for the one question it is opened to
		// answer: did this reminder arrive?
		status := "sent"
		if d.Queue != nil {
			status = "queued"
		}
		sent, detail := d.deliver(ctx, s, key)
		if sent > 0 {
			rep.Sent++
			_ = d.store.MarkDelivery(ctx, s.ID, key, status, detail)
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

	if d.Queue != nil {
		return d.enqueue(ctx, s, key, targets, label, minutes)
	}

	lastErr := ""
	for _, t := range targets {
		n := notificationFor(s, t, label, minutes)

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

// notificationFor builds the message a device receives.
//
// One place, so a queued reminder and an inline one are the same notification:
// the body, the deep link and the collapse key cannot drift apart depending on
// which path a deployment happens to run.
func notificationFor(s Schedule, t Target, label string, minutes int) push.Notification {
	return push.Notification{
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
}

// enqueue hands one delivery per device to the queue.
//
// The payload carries user_id and device_id as well as the message, because
// token lifecycle moved with the send: the component that learns a provider has
// rejected a token is now the queue handler, so it is the one that has to clear
// it. An enqueue that loses a dedupe race is not an error - the delivery it
// wanted is already queued, which is the outcome it asked for.
func (d *Dispatcher) enqueue(ctx context.Context, s Schedule, occurrence string, targets []Target,
	label string, minutes int) (queued int, detail string) {
	var firstErr string
	for _, t := range targets {
		n := notificationFor(s, t, label, minutes)
		payload := map[string]any{
			"token":     n.Token,
			"platform":  string(n.Platform),
			"title":     n.Title,
			"body":      n.Body,
			"data":      n.Data,
			"user_id":   s.UserID,
			"device_id": t.DeviceID,
		}
		// One reminder per occurrence per device. The occurrence key is in the
		// dedupe key because without it tomorrow's 6am would be swallowed as a
		// duplicate of today's.
		dedupe := "sched:" + s.ID + ":" + occurrence + ":" + t.DeviceID
		if err := d.Queue.EnqueueNotification(ctx, payload, dedupe); err != nil {
			log.Printf("scheduler: enqueue failed for %s@%s device=%s: %v", s.ID, occurrence, t.DeviceID, err)
			if firstErr == "" {
				firstErr = err.Error()
			}
			continue
		}
		queued++
	}
	if queued > 0 {
		return queued, "queued to " + itoa(queued) + " device(s)"
	}
	if firstErr == "" {
		firstErr = "nothing to queue"
	}
	return 0, firstErr
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
