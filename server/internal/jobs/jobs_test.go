package jobs

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/backoff"
)

func TestEnqueueAndProcessCompletesJob(t *testing.T) {
	q := NewMemoryQueue()
	var calls atomic.Int32
	q.Register("demo", func(ctx context.Context, payload map[string]any) error {
		calls.Add(1)
		return nil
	})

	id, err := q.Enqueue(context.Background(), Job{Type: "demo", Payload: map[string]any{"hello": "world"}, IdempotencyKey: "demo-1"})
	if err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if err := q.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext returned error: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d; want 1", calls.Load())
	}
	jobs := q.List()
	if len(jobs) != 1 {
		t.Fatalf("jobs length = %d; want 1", len(jobs))
	}
	if jobs[0].Status != StatusCompleted {
		t.Fatalf("job status = %q; want %q", jobs[0].Status, StatusCompleted)
	}
	if jobs[0].ID != id {
		t.Fatalf("job ID mismatch: got %q want %q", jobs[0].ID, id)
	}
}

// TestRetriesAndDeadLetter drives the retry path with a fake clock.
//
// A failed job is not immediately available again - it waits for its backoff -
// so the test steps time forward rather than sleeping, which also proves the
// backoff is actually honoured rather than merely recorded.
func TestRetriesAndDeadLetter(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	q := NewMemoryQueue().
		WithClock(func() time.Time { return now }).
		WithPolicy(backoff.New(time.Minute, time.Hour))
	q.Register("always-fail", func(ctx context.Context, payload map[string]any) error {
		return errors.New("boom")
	})

	_, err := q.Enqueue(context.Background(), Job{Type: "always-fail", MaxAttempts: 2})
	if err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}

	if err := q.ProcessNext(context.Background()); err == nil {
		t.Fatal("ProcessNext should fail on first retryable error")
	}
	got := q.List()
	if len(got) != 1 {
		t.Fatalf("jobs length = %d; want 1", len(got))
	}
	if got[0].Status != StatusQueued {
		t.Fatalf("status after first failure = %q; want %q", got[0].Status, StatusQueued)
	}
	if got[0].Attempts != 1 {
		t.Fatalf("attempts = %d; want 1", got[0].Attempts)
	}
	if got[0].AvailableAt.IsZero() {
		t.Fatal("a failed job was left immediately available; it should wait out its backoff")
	}

	// Before the backoff elapses the job must be left alone.
	if err := q.ProcessNext(context.Background()); err != nil {
		t.Fatalf("polling a queue with nothing due should be quiet, got %v", err)
	}
	if s := q.List()[0].Status; s != StatusQueued {
		t.Fatalf("job was retried before its backoff elapsed: status %q", s)
	}
	if n := q.List()[0].Attempts; n != 1 {
		t.Fatalf("attempts = %d after a poll that should not have claimed anything", n)
	}

	// Step past the one-minute base delay.
	now = now.Add(2 * time.Minute)
	if err := q.ProcessNext(context.Background()); err == nil {
		t.Fatal("ProcessNext should dead-letter after last retry")
	}
	final := q.List()[0]
	if final.Status != StatusDeadLetter {
		t.Fatalf("status after final failure = %q; want %q", final.Status, StatusDeadLetter)
	}
	if final.DeadLetterReason == "" {
		t.Error("a parked job records no reason, so nobody can tell why it stopped")
	}
}

func TestDuplicateIdempotencyKeyIsRejected(t *testing.T) {
	q := NewMemoryQueue()
	q.Register("demo", func(ctx context.Context, payload map[string]any) error { return nil })

	if _, err := q.Enqueue(context.Background(), Job{Type: "demo", IdempotencyKey: "dup"}); err != nil {
		t.Fatalf("first enqueue error: %v", err)
	}
	if _, err := q.Enqueue(context.Background(), Job{Type: "demo", IdempotencyKey: "dup"}); err == nil {
		t.Fatal("duplicate idempotency key should be rejected")
	}
}

func TestWorkerProcessesQueuedJobs(t *testing.T) {
	q := NewMemoryQueue()
	var calls atomic.Int32
	q.Register("demo", func(ctx context.Context, payload map[string]any) error {
		calls.Add(1)
		return nil
	})

	if _, err := q.Enqueue(context.Background(), Job{Type: "demo", Payload: map[string]any{"job": "background"}}); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}

	worker := NewWorker(q, 1)
	ctx := context.Background()
	worker.Start(ctx)
	defer worker.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if calls.Load() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("handler calls = %d; want 1", got)
	}
}

// Stop is called from both a signal handler and a deferred call during
// shutdown. Closing an already-closed channel panics, which turned a clean
// shutdown into a crash.
func TestWorkerStopIsIdempotent(t *testing.T) {
	q := NewMemoryQueue()
	w := NewWorker(q, 2)
	w.Start(context.Background())

	w.Stop()
	w.Stop() // must not panic
	w.Stop()
}
