package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/backoff"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/jobs"
)

// newJobQueue returns a durable queue on a fixed clock the test controls.
func newJobQueue(t *testing.T) (*JobQueue, *time.Time) {
	t.Helper()
	now := time.Unix(1700000000, 0).UTC()
	q := NewJobQueue(dbtest.New(t)).
		WithClock(func() time.Time { return now }).
		WithPolicy(backoff.New(time.Minute, time.Hour))
	return q, &now
}

func TestDurableQueueRoundTrip(t *testing.T) {
	q, _ := newJobQueue(t)
	ctx := context.Background()

	id, err := q.Enqueue(ctx, jobs.Job{
		Type:        "demo",
		Payload:     map[string]any{"hello": "world"},
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	got, err := q.Claim(ctx, "worker-a")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if got.ID != id {
		t.Errorf("claimed %q, want %q", got.ID, id)
	}
	if got.Status != jobs.StatusRunning {
		t.Errorf("claimed status = %q, want %q", got.Status, jobs.StatusRunning)
	}
	if got.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", got.Attempts)
	}
	if got.WorkerID != "worker-a" {
		t.Errorf("worker_id = %q, want worker-a", got.WorkerID)
	}
	if v, _ := got.Payload["hello"].(string); v != "world" {
		t.Errorf("payload did not survive the round trip: %v", got.Payload)
	}

	if err := q.Complete(ctx, id); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	after, err := q.ByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != jobs.StatusCompleted {
		t.Errorf("status after Complete = %q, want %q", after.Status, jobs.StatusCompleted)
	}
	// A completed job must not be handed out again.
	if _, err := q.Claim(ctx, "worker-a"); !errors.Is(err, jobs.ErrEmptyQueue) {
		t.Errorf("Claim after completion = %v, want ErrEmptyQueue", err)
	}
}

// TestIdempotencyKeyPreventsASecondJob is the durable half of the idempotency
// requirement. The in-memory queue kept seen keys in a map, so a restart forgot
// them and the same request produced the work twice.
func TestIdempotencyKeyPreventsASecondJob(t *testing.T) {
	q, _ := newJobQueue(t)
	ctx := context.Background()

	first, err := q.Enqueue(ctx, jobs.Job{Type: "demo", IdempotencyKey: "req-1"})
	if err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}
	second, err := q.Enqueue(ctx, jobs.Job{Type: "demo", IdempotencyKey: "req-1"})
	if !errors.Is(err, jobs.ErrDuplicateJob) {
		t.Fatalf("second Enqueue err = %v, want ErrDuplicateJob", err)
	}
	if second != first {
		t.Errorf("duplicate returned id %q, want the existing %q", second, first)
	}

	stats, err := q.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 1 {
		t.Errorf("two enqueues with one idempotency key produced %d jobs", stats.Total)
	}
}

// TestFailureWaitsOutItsBackoff is the exponential-backoff requirement. A job
// that failed against a provider must not come straight back.
func TestFailureWaitsOutItsBackoff(t *testing.T) {
	q, now := newJobQueue(t)
	ctx := context.Background()

	id, err := q.Enqueue(ctx, jobs.Job{Type: "demo", MaxAttempts: 5})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Claim(ctx, "w"); err != nil {
		t.Fatal(err)
	}
	if err := q.Fail(ctx, id, errors.New("provider said 429")); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	stored, err := q.ByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != jobs.StatusQueued {
		t.Fatalf("status after first failure = %q, want %q", stored.Status, jobs.StatusQueued)
	}
	if stored.AvailableAt.IsZero() {
		t.Fatal("the failed job has no available_at, so nothing enforces the backoff")
	}
	if wait := stored.AvailableAt.Sub(*now); wait != time.Minute {
		t.Errorf("backoff before attempt 2 = %v, want the policy's 1m base", wait)
	}

	if _, err := q.Claim(ctx, "w"); !errors.Is(err, jobs.ErrEmptyQueue) {
		t.Fatalf("the job was claimable before its backoff elapsed: %v", err)
	}

	// Step past it.
	*now = now.Add(2 * time.Minute)
	next, err := q.Claim(ctx, "w")
	if err != nil {
		t.Fatalf("Claim after backoff: %v", err)
	}
	if next.Attempts != 2 {
		t.Errorf("attempts = %d, want 2", next.Attempts)
	}
}

// TestBackoffGrowsWithEachFailure is what makes it exponential rather than a
// fixed retry interval: the second wait must be longer than the first.
func TestBackoffGrowsWithEachFailure(t *testing.T) {
	q, now := newJobQueue(t)
	ctx := context.Background()

	id, err := q.Enqueue(ctx, jobs.Job{Type: "demo", MaxAttempts: 5})
	if err != nil {
		t.Fatal(err)
	}
	var waits []time.Duration
	for i := 0; i < 3; i++ {
		if _, err := q.Claim(ctx, "w"); err != nil {
			t.Fatalf("claim %d: %v", i, err)
		}
		if err := q.Fail(ctx, id, errors.New("still down")); err != nil {
			t.Fatalf("fail %d: %v", i, err)
		}
		stored, err := q.ByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		waits = append(waits, stored.AvailableAt.Sub(*now))
		*now = stored.AvailableAt.Add(time.Second)
	}
	for i := 1; i < len(waits); i++ {
		if waits[i] <= waits[i-1] {
			t.Fatalf("waits did not grow: %v", waits)
		}
	}
	if waits[0] != time.Minute || waits[1] != 2*time.Minute || waits[2] != 4*time.Minute {
		t.Errorf("schedule = %v, want 1m 2m 4m", waits)
	}
}

func TestExhaustedAttemptsAreParkedWithAReason(t *testing.T) {
	q, now := newJobQueue(t)
	ctx := context.Background()

	id, err := q.Enqueue(ctx, jobs.Job{Type: "demo", MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := q.Claim(ctx, "w"); err != nil {
			t.Fatalf("claim %d: %v", i, err)
		}
		if err := q.Fail(ctx, id, errors.New("boom")); err != nil {
			t.Fatalf("fail %d: %v", i, err)
		}
		*now = now.Add(time.Hour)
	}

	stored, err := q.ByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != jobs.StatusDeadLetter {
		t.Fatalf("status = %q, want %q", stored.Status, jobs.StatusDeadLetter)
	}
	if stored.DeadLetterReason == "" {
		t.Error("a parked job records no reason, so nobody can tell why it stopped")
	}
	if _, err := q.Claim(ctx, "w"); !errors.Is(err, jobs.ErrEmptyQueue) {
		t.Error("a parked job is still being handed out")
	}
}

// TestPermanentFailureParksImmediately covers the other half of the dead-letter
// strategy: work that cannot succeed by being repeated must not consume its
// retries first.
func TestPermanentFailureParksImmediately(t *testing.T) {
	q, _ := newJobQueue(t)
	ctx := context.Background()

	id, err := q.Enqueue(ctx, jobs.Job{Type: "demo", MaxAttempts: 5})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Claim(ctx, "w"); err != nil {
		t.Fatal(err)
	}
	if err := q.Fail(ctx, id, jobs.Permanent(errors.New("confession does not exist"))); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	stored, err := q.ByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != jobs.StatusDeadLetter {
		t.Errorf("status = %q, want %q on the first permanent failure", stored.Status, jobs.StatusDeadLetter)
	}
	if stored.Attempts != 1 {
		t.Errorf("attempts = %d; a permanent failure should not have been retried", stored.Attempts)
	}
}

// TestRequeueDeadIsTheOperationalHalf parking a job is only useful if a human
// can put it back once the cause is fixed.
func TestRequeueDeadIsTheOperationalHalf(t *testing.T) {
	q, _ := newJobQueue(t)
	ctx := context.Background()

	id, err := q.Enqueue(ctx, jobs.Job{Type: "demo", MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Claim(ctx, "w"); err != nil {
		t.Fatal(err)
	}
	if err := q.Fail(ctx, id, errors.New("provider outage")); err != nil {
		t.Fatal(err)
	}

	n, err := q.RequeueDead(ctx, 10)
	if err != nil {
		t.Fatalf("RequeueDead: %v", err)
	}
	if n != 1 {
		t.Fatalf("requeued %d, want 1", n)
	}
	stored, err := q.ByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != jobs.StatusQueued {
		t.Errorf("status = %q, want %q", stored.Status, jobs.StatusQueued)
	}
	if stored.Attempts != 0 {
		t.Errorf("attempts = %d, want a fresh count", stored.Attempts)
	}
	if stored.DeadLetterReason != "" {
		t.Errorf("the old reason survived the requeue: %q", stored.DeadLetterReason)
	}
	if _, err := q.Claim(ctx, "w"); err != nil {
		t.Errorf("the requeued job is not claimable: %v", err)
	}
}

// TestTwoWorkersNeverClaimTheSameJob is the reason the claim is a single
// statement with FOR UPDATE SKIP LOCKED. Two workers polling one table must not
// both be handed the same work.
func TestTwoWorkersNeverClaimTheSameJob(t *testing.T) {
	q, _ := newJobQueue(t)
	ctx := context.Background()

	const n = 25
	for i := 0; i < n; i++ {
		if _, err := q.Enqueue(ctx, jobs.Job{Type: "demo"}); err != nil {
			t.Fatal(err)
		}
	}

	var (
		mu   sync.Mutex
		seen = map[string]int{}
		wg   sync.WaitGroup
	)
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(worker string) {
			defer wg.Done()
			for {
				job, err := q.Claim(ctx, worker)
				if errors.Is(err, jobs.ErrEmptyQueue) {
					return
				}
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				mu.Lock()
				seen[job.ID]++
				mu.Unlock()
				_ = q.Complete(ctx, job.ID)
			}
		}("w")
	}
	wg.Wait()

	if len(seen) != n {
		t.Errorf("%d distinct jobs claimed, want %d", len(seen), n)
	}
	for id, times := range seen {
		if times != 1 {
			t.Errorf("job %s was claimed %d times", id, times)
		}
	}
}

// TestStaleClaimIsReturnedToTheQueue covers a worker that dies mid-job. Without
// this the row sits in 'running' forever: not due, not failed, not finished.
func TestStaleClaimIsReturnedToTheQueue(t *testing.T) {
	q, now := newJobQueue(t)
	ctx := context.Background()

	id, err := q.Enqueue(ctx, jobs.Job{Type: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Claim(ctx, "worker-that-died"); err != nil {
		t.Fatal(err)
	}

	// Too recent to be considered abandoned.
	if n, err := q.ReclaimStale(ctx, time.Hour); err != nil || n != 0 {
		t.Fatalf("ReclaimStale = (%d, %v), want (0, nil) for a fresh claim", n, err)
	}

	*now = now.Add(2 * time.Hour)
	n, err := q.ReclaimStale(ctx, time.Hour)
	if err != nil {
		t.Fatalf("ReclaimStale: %v", err)
	}
	if n != 1 {
		t.Fatalf("reclaimed %d, want 1", n)
	}
	stored, err := q.ByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != jobs.StatusQueued {
		t.Errorf("status = %q, want %q", stored.Status, jobs.StatusQueued)
	}
	if stored.WorkerID != "" {
		t.Errorf("worker_id = %q, want it cleared", stored.WorkerID)
	}
}

func TestStatsCountsByStatus(t *testing.T) {
	q, now := newJobQueue(t)
	ctx := context.Background()

	// One that will complete, one that will be parked, one left waiting.
	// Enqueued a second apart: timestamps here carry second precision, so jobs
	// created in the same second tie and come back in id order rather than
	// enqueue order.
	done, err := q.Enqueue(ctx, jobs.Job{Type: "a"})
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Second)
	parked, err := q.Enqueue(ctx, jobs.Job{Type: "b", MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Second)
	if _, err := q.Enqueue(ctx, jobs.Job{Type: "c"}); err != nil {
		t.Fatal(err)
	}

	// Claims come out oldest-first, so this drains in a known order.
	first, err := q.Claim(ctx, "w")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != done {
		t.Fatalf("claimed %q, want the oldest job %q", first.ID, done)
	}
	if err := q.Complete(ctx, done); err != nil {
		t.Fatal(err)
	}
	second, err := q.Claim(ctx, "w")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != parked {
		t.Fatalf("claimed %q, want %q", second.ID, parked)
	}
	if err := q.Fail(ctx, parked, errors.New("boom")); err != nil {
		t.Fatal(err)
	}

	stats, err := q.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 3 {
		t.Errorf("total = %d, want 3", stats.Total)
	}
	if stats.Queued != 1 || stats.Completed != 1 || stats.DeadLetter != 1 {
		t.Errorf("stats = %+v, want 1 queued, 1 completed, 1 dead-lettered", stats)
	}
	if stats.Running != 0 {
		t.Errorf("running = %d after every claim was resolved", stats.Running)
	}
}

// TestUnregisteredTypeIsNotSilentlyDropped guards the specific failure this
// phase replaces: a job nobody can perform must park with a reason, not report
// success.
func TestUnregisteredTypeIsNotSilentlyDropped(t *testing.T) {
	q, _ := newJobQueue(t)
	ctx := context.Background()

	if _, err := q.Enqueue(ctx, jobs.Job{Type: "no.such.handler"}); err != nil {
		t.Fatal(err)
	}
	job, err := q.Claim(ctx, "w")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := q.HandlerFor(job.Type); ok {
		t.Fatal("a handler was registered for a type the test never registered")
	}
	if err := q.FailPermanent(ctx, job.ID, "no handler registered for job type \"no.such.handler\""); err != nil {
		t.Fatal(err)
	}
	stored, err := q.ByID(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != jobs.StatusDeadLetter {
		t.Errorf("status = %q, want %q", stored.Status, jobs.StatusDeadLetter)
	}
	if stored.DeadLetterReason == "" {
		t.Error("no reason recorded for a job nobody could perform")
	}
}
