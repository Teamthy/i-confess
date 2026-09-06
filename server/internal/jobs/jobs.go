package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/Teamthy/i-confess/internal/backoff"
)

// MemoryQueue is an in-process Queue.
//
// It exists for tests and for tooling that wants a queue without a database.
// It is NOT what the server runs: nothing in it survives a restart, so every
// job still queued when the process stops is gone. That is acceptable for a
// test and unacceptable for work a user is waiting on, which is why
// store.JobQueue exists.
type MemoryQueue struct {
	mu       sync.Mutex
	order    []string
	jobs     map[string]*Job
	handlers map[string]Handler
	byKey    map[string]string // idempotency key -> job id
	policy   backoff.Policy
	now      func() time.Time
}

// NewMemoryQueue returns a queue using the default retry policy.
func NewMemoryQueue() *MemoryQueue {
	return &MemoryQueue{
		jobs:     map[string]*Job{},
		handlers: map[string]Handler{},
		byKey:    map[string]string{},
		policy:   backoff.New(2*time.Second, 5*time.Minute),
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// WithPolicy replaces the retry schedule, which is what lets a test run
// retries without waiting.
func (q *MemoryQueue) WithPolicy(p backoff.Policy) *MemoryQueue {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.policy = p
	return q
}

// WithClock replaces the time source, so a test can advance past a backoff
// delay instead of sleeping through it.
func (q *MemoryQueue) WithClock(now func() time.Time) *MemoryQueue {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.now = now
	return q
}

func (q *MemoryQueue) Register(name string, fn Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers[name] = fn
}

func (q *MemoryQueue) HandlerFor(name string) (Handler, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	h, ok := q.handlers[name]
	return h, ok
}

func (q *MemoryQueue) Enqueue(_ context.Context, job Job) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if job.Type == "" {
		return "", fmt.Errorf("job type is required")
	}
	now := q.now()
	job.Prepare(now)
	job.UpdatedAt = now

	if job.IdempotencyKey != "" {
		if existing, ok := q.byKey[job.IdempotencyKey]; ok {
			// Returning the id alongside the error is deliberate: a caller that
			// retried its own request gets the job it already has, rather than
			// only a refusal it has to interpret.
			return existing, fmt.Errorf("%w: %s", ErrDuplicateJob, job.IdempotencyKey)
		}
		q.byKey[job.IdempotencyKey] = job.ID
	}
	q.jobs[job.ID] = &job
	q.order = append(q.order, job.ID)
	return job.ID, nil
}

func (q *MemoryQueue) Claim(_ context.Context, workerID string) (*Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := q.now()
	for _, id := range q.order {
		j := q.jobs[id]
		if j == nil || j.Status != StatusQueued {
			continue
		}
		if !j.AvailableAt.IsZero() && j.AvailableAt.After(now) {
			continue
		}
		j.Status = StatusRunning
		j.Attempts++
		j.WorkerID = workerID
		j.UpdatedAt = now
		out := *j
		return &out, nil
	}
	return nil, ErrEmptyQueue
}

func (q *MemoryQueue) Complete(_ context.Context, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	j := q.jobs[id]
	if j == nil {
		return ErrJobNotFound
	}
	j.Status = StatusCompleted
	j.LastError = ""
	j.UpdatedAt = q.now()
	return nil
}

func (q *MemoryQueue) Fail(_ context.Context, id string, cause error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	j := q.jobs[id]
	if j == nil {
		return ErrJobNotFound
	}
	now := q.now()
	j.LastError = errString(cause)
	j.UpdatedAt = now

	if IsPermanent(cause) {
		j.Status = StatusDeadLetter
		j.DeadLetterReason = "permanent: " + j.LastError
		return nil
	}
	if j.Attempts >= j.MaxAttempts {
		j.Status = StatusDeadLetter
		j.DeadLetterReason = fmt.Sprintf("exhausted %d attempt(s): %s", j.Attempts, j.LastError)
		return nil
	}
	j.Status = StatusQueued
	j.AvailableAt = now.Add(q.delayFor(j))
	j.WorkerID = ""
	return nil
}

func (q *MemoryQueue) FailPermanent(_ context.Context, id, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	j := q.jobs[id]
	if j == nil {
		return ErrJobNotFound
	}
	j.Status = StatusDeadLetter
	j.DeadLetterReason = reason
	j.UpdatedAt = q.now()
	return nil
}

func (q *MemoryQueue) RequeueDead(_ context.Context, limit int) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, id := range q.order {
		if limit > 0 && n >= limit {
			break
		}
		j := q.jobs[id]
		if j == nil || j.Status != StatusDeadLetter {
			continue
		}
		j.Status = StatusQueued
		j.Attempts = 0
		j.AvailableAt = time.Time{}
		j.DeadLetterReason = ""
		j.UpdatedAt = q.now()
		n++
	}
	return n, nil
}

func (q *MemoryQueue) Stats(_ context.Context) (Stats, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var s Stats
	for _, j := range q.jobs {
		s.Total++
		switch j.Status {
		case StatusQueued:
			s.Queued++
		case StatusRunning:
			s.Running++
		case StatusCompleted:
			s.Completed++
		case StatusFailed:
			s.Failed++
		case StatusDeadLetter:
			s.DeadLetter++
		}
	}
	return s, nil
}

// List returns a copy of every job, in enqueue order.
func (q *MemoryQueue) List() []Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Job, 0, len(q.order))
	for _, id := range q.order {
		if j := q.jobs[id]; j != nil {
			out = append(out, *j)
		}
	}
	return out
}

// ProcessNext claims one due job and runs it to completion or failure.
//
// The worker no longer uses this - it claims, runs and reports so that a
// long-running handler does not block the loop - but it is the simplest way to
// drive a queue from a test.
func (q *MemoryQueue) ProcessNext(ctx context.Context) error {
	job, err := q.Claim(ctx, "process-next")
	if err != nil {
		if err == ErrEmptyQueue {
			return nil
		}
		return err
	}
	h, ok := q.HandlerFor(job.Type)
	if !ok {
		reason := fmt.Sprintf("no handler registered for job type %q", job.Type)
		_ = q.FailPermanent(ctx, job.ID, reason)
		return fmt.Errorf("%s", reason)
	}
	if err := h(ctx, job.Payload); err != nil {
		if ferr := q.Fail(ctx, job.ID, err); ferr != nil {
			return ferr
		}
		return err
	}
	return q.Complete(ctx, job.ID)
}

// delayFor returns how long a job waits before its next attempt. An explicit
// RetryDelay on the job wins; otherwise the queue's policy decides, growing with
// the attempt count rather than retrying at a fixed interval forever.
func (q *MemoryQueue) delayFor(j *Job) time.Duration {
	if j.RetryDelay > 0 {
		return j.RetryDelay
	}
	return q.policy.Next(j.Attempts)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
