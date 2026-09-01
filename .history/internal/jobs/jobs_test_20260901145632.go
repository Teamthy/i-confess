package jobs

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestEnqueueAndProcessCompletesJob(t *testing.T) {
	q := NewMemoryQueue()
	var calls atomic.Int32
	q.Register("demo", func(ctx context.Context, payload map[string]any) error {
		calls.Add(1)
		return nil
	})

	id, err := q.Enqueue(Job{Type: "demo", Payload: map[string]any{"hello": "world"}, IdempotencyKey: "demo-1"})
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

func TestRetriesAndDeadLetter(t *testing.T) {
	q := NewMemoryQueue()
	q.Register("always-fail", func(ctx context.Context, payload map[string]any) error {
		return errors.New("boom")
	})

	_, err := q.Enqueue(Job{Type: "always-fail", MaxAttempts: 2, RetryDelay: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}

	if err := q.ProcessNext(context.Background()); err == nil {
		t.Fatal("ProcessNext should fail on first retryable error")
	}
	jobs := q.List()
	if len(jobs) != 1 {
		t.Fatalf("jobs length = %d; want 1", len(jobs))
	}
	if jobs[0].Status != StatusQueued {
		t.Fatalf("status after first failure = %q; want %q", jobs[0].Status, StatusQueued)
	}
	if jobs[0].Attempts != 1 {
		t.Fatalf("attempts = %d; want 1", jobs[0].Attempts)
	}

	if err := q.ProcessNext(context.Background()); err == nil {
		t.Fatal("ProcessNext should dead-letter after last retry")
	}
	jobs = q.List()
	if jobs[0].Status != StatusDeadLetter {
		t.Fatalf("status after final failure = %q; want %q", jobs[0].Status, StatusDeadLetter)
	}
}

func TestDuplicateIdempotencyKeyIsRejected(t *testing.T) {
	q := NewMemoryQueue()
	q.Register("demo", func(ctx context.Context, payload map[string]any) error { return nil })

	if _, err := q.Enqueue(Job{Type: "demo", IdempotencyKey: "dup"}); err != nil {
		t.Fatalf("first enqueue error: %v", err)
	}
	if _, err := q.Enqueue(Job{Type: "demo", IdempotencyKey: "dup"}); err == nil {
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

	if _, err := q.Enqueue(Job{Type: "demo", Payload: map[string]any{"job": "background"}}); err != nil {
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
