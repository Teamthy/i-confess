package email

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Teamthy/i-confess/internal/backoff"
)

// Queue delivers mail asynchronously with bounded retries (§57, §58).
//
// Authentication must not fail because a mail provider is slow. Enqueue is
// non-blocking and never returns an error the caller is expected to surface:
// registration succeeded, and a delayed verification email is a recoverable
// inconvenience the user can fix with "resend".
//
// This is an in-process queue backed by a buffered channel. That is honest
// about its limits — messages in flight are lost if the process dies, and it
// does not coordinate across instances. Durable delivery is a Redis/Postgres
// outbox; the Enqueue signature is the seam that makes that swap invisible.
type Queue struct {
	sender  Sender
	ch      chan *job
	wg      sync.WaitGroup
	stop    chan struct{}
	stopper sync.Once

	// MaxAttempts bounds retries per message.
	MaxAttempts int
	// Backoff returns the delay before an attempt (1-based).
	Backoff func(attempt int) time.Duration

	mu      sync.Mutex
	metrics Metrics
}

type job struct {
	msg      Message
	attempts int
}

// Metrics are counters for observability (§83).
type Metrics struct {
	Enqueued  int
	Sent      int
	Failed    int
	Dropped   int
	Retried   int
	Discarded int
}

// NewQueue creates a queue. It does not start workers; call Start.
func NewQueue(sender Sender, buffer int) *Queue {
	if buffer <= 0 {
		buffer = 256
	}
	return &Queue{
		sender:      sender,
		ch:          make(chan *job, buffer),
		stop:        make(chan struct{}),
		MaxAttempts: 4,
		Backoff:     defaultBackoff,
	}
}

// defaultBackoff grows quickly then caps: a provider outage should be retried
// patiently rather than hammered. The schedule lives in internal/backoff so the
// mailer, the job queue and the voice pipeline cannot drift apart.
var mailBackoff = backoff.New(5*time.Second, 5*time.Minute)

func defaultBackoff(attempt int) time.Duration { return mailBackoff.Next(attempt) }

// Start launches n delivery workers.
func (q *Queue) Start(ctx context.Context, n int) {
	if n <= 0 {
		n = 2
	}
	for i := 0; i < n; i++ {
		q.wg.Add(1)
		go q.run(ctx)
	}
}

// Stop drains in-flight work and waits for workers. Safe to call repeatedly.
func (q *Queue) Stop() {
	q.stopper.Do(func() { close(q.stop) })
	q.wg.Wait()
}

// Enqueue schedules a message. It never blocks.
//
// If the buffer is full the message is dropped and counted rather than blocking
// the caller: a full queue means the provider is already struggling, and
// stalling an HTTP handler behind it converts a mail problem into an outage.
func (q *Queue) Enqueue(msg Message) {
	q.count(func(m *Metrics) { m.Enqueued++ })
	select {
	case q.ch <- &job{msg: msg}:
	default:
		q.count(func(m *Metrics) { m.Dropped++ })
		log.Printf("email: queue full, dropped %s to %s", msg.Tag, MaskAddress(msg.To))
	}
}

func (q *Queue) run(ctx context.Context) {
	defer q.wg.Done()
	for {
		select {
		case <-q.stop:
			return
		case <-ctx.Done():
			return
		case j := <-q.ch:
			q.deliver(ctx, j)
		}
	}
}

func (q *Queue) deliver(ctx context.Context, j *job) {
	j.attempts++

	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err := q.sender.Send(sendCtx, j.msg)
	cancel()

	if err == nil {
		q.count(func(m *Metrics) { m.Sent++ })
		return
	}

	// A permanent failure is not retried: a malformed address or rejected
	// content will fail identically every time, and repeated hard bounces harm
	// sender reputation for every other user.
	if !IsRetryable(err) {
		q.count(func(m *Metrics) { m.Failed++ })
		log.Printf("email: permanent failure for %s to %s: %v", j.msg.Tag, MaskAddress(j.msg.To), err)
		return
	}

	if j.attempts >= q.MaxAttempts {
		q.count(func(m *Metrics) { m.Discarded++ })
		log.Printf("email: giving up on %s to %s after %d attempts: %v",
			j.msg.Tag, MaskAddress(j.msg.To), j.attempts, err)
		return
	}

	q.count(func(m *Metrics) { m.Retried++ })
	delay := q.Backoff(j.attempts)
	log.Printf("email: retrying %s to %s in %s (attempt %d): %v",
		j.msg.Tag, MaskAddress(j.msg.To), delay, j.attempts, err)

	// Re-queue after the backoff without occupying a worker while waiting.
	go func() {
		t := time.NewTimer(delay)
		defer t.Stop()
		select {
		case <-t.C:
			select {
			case q.ch <- j:
			default:
				q.count(func(m *Metrics) { m.Dropped++ })
			}
		case <-q.stop:
		case <-ctx.Done():
		}
	}()
}

// Metrics returns a snapshot.
func (q *Queue) Metrics() Metrics {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.metrics
}

func (q *Queue) count(f func(*Metrics)) {
	q.mu.Lock()
	f(&q.metrics)
	q.mu.Unlock()
}

// Drain waits until the queue is empty or the deadline passes. Test helper.
func (q *Queue) Drain(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(q.ch) == 0 {
			// Allow an in-flight send to complete.
			time.Sleep(10 * time.Millisecond)
			if len(q.ch) == 0 {
				return true
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}
