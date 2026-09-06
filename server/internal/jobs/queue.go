package jobs

import (
	"context"
	"errors"
	"time"
)

// Queue is the background work queue.
//
// Two implementations exist. MemoryQueue is in-process and is what the tests
// use; store.JobQueue is the PostgreSQL one the server runs. They are
// interchangeable behind this interface, which is the point: the worker, the
// handlers and the callers that enqueue do not care where the work is held.
//
// The durability difference is the whole reason the second one exists. A queue
// that lives in a process's memory loses every pending job when the process
// restarts, and a restart is not an unusual event - it is a deploy.
type Queue interface {
	// Register associates a job type with the function that performs it.
	Register(name string, h Handler)

	// HandlerFor resolves the function registered for a job type.
	HandlerFor(name string) (Handler, bool)

	// Enqueue adds a job. When the job carries an idempotency key that has
	// already been used, it returns the id of the job already queued along
	// with ErrDuplicateJob, so a caller that retries a request neither
	// duplicates the work nor has to treat its own retry as a failure.
	Enqueue(ctx context.Context, j Job) (string, error)

	// Claim atomically reserves the next job whose retry delay has elapsed and
	// marks it running. It returns ErrEmptyQueue when there is nothing due.
	//
	// Atomic matters: two workers must never be handed the same job. The
	// PostgreSQL implementation does this with FOR UPDATE SKIP LOCKED; the
	// in-memory one with a mutex.
	Claim(ctx context.Context, workerID string) (*Job, error)

	// Complete marks a claimed job finished.
	Complete(ctx context.Context, id string) error

	// Fail records a retryable failure. The job returns to the queue with a
	// backoff delay, or moves to dead_letter once its attempts are exhausted.
	Fail(ctx context.Context, id string, cause error) error

	// FailPermanent parks a job without further attempts. For work that cannot
	// succeed by being repeated: an invalid payload, an unregistered type, a
	// reference to something that no longer exists.
	FailPermanent(ctx context.Context, id, reason string) error

	// RequeueDead puts parked jobs back in the queue, resetting their attempt
	// count. This is the operational half of a dead-letter strategy: parking a
	// job is only useful if a human can put it back once the cause is fixed.
	// It returns how many were requeued.
	RequeueDead(ctx context.Context, limit int) (int, error)

	// Stats counts jobs by status, for the health endpoint and the admin
	// queue view.
	Stats(ctx context.Context) (Stats, error)
}

var (
	// ErrEmptyQueue means nothing was due to run. It is not an error condition
	// for the caller; the worker treats it as "poll again later".
	ErrEmptyQueue = errors.New("no jobs available")

	// ErrDuplicateJob means a job with this idempotency key is already queued.
	// The returned id is the existing job's.
	ErrDuplicateJob = errors.New("a job with this idempotency key is already queued")

	// ErrJobNotFound means the id does not refer to a job in this queue.
	ErrJobNotFound = errors.New("job not found")
)

// Stats counts jobs by status.
type Stats struct {
	Queued     int `json:"queued"`
	Running    int `json:"running"`
	Completed  int `json:"completed"`
	Failed     int `json:"failed"`
	DeadLetter int `json:"dead_letter"`
	Total      int `json:"total"`
}

// DefaultMaxAttempts is how many times a job is attempted before it is parked.
const DefaultMaxAttempts = 3

// Job statuses. These are the values jobs_status_check accepts.
const (
	StatusQueued     = "queued"
	StatusRunning    = "running"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusDeadLetter = "dead_letter"
)

// AllStatuses is every status the schema permits, in lifecycle order.
func AllStatuses() []string {
	return []string{StatusQueued, StatusRunning, StatusCompleted, StatusFailed, StatusDeadLetter}
}

// ValidStatus reports whether s is a status the schema accepts. Checked before
// a write so a typo is caught here rather than by a constraint deep inside a
// request.
func ValidStatus(s string) bool {
	for _, v := range AllStatuses() {
		if s == v {
			return true
		}
	}
	return false
}

// IsTerminal reports whether a job in this status will not be picked up again
// without intervention.
func IsTerminal(status string) bool {
	return status == StatusCompleted || status == StatusDeadLetter
}

// Handler performs a job. Returning an error marks the attempt failed and
// schedules a retry; returning ErrPermanent parks the job immediately.
type Handler func(ctx context.Context, payload map[string]any) error

// ErrPermanent wraps a cause that retrying cannot fix.
type ErrPermanent struct{ Cause error }

func (e ErrPermanent) Error() string { return "permanent failure: " + e.Cause.Error() }
func (e ErrPermanent) Unwrap() error { return e.Cause }

// Permanent marks err as not worth retrying.
func Permanent(err error) error { return ErrPermanent{Cause: err} }

// IsPermanent reports whether err, or anything it wraps, was marked permanent.
func IsPermanent(err error) bool {
	var p ErrPermanent
	return errors.As(err, &p)
}

// Job is one unit of queued work.
type Job struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Payload     map[string]any `json:"payload,omitempty"`
	Status      string         `json:"status"`
	Attempts    int            `json:"attempts"`
	MaxAttempts int            `json:"max_attempts"`

	// RetryDelay, when set, overrides the queue's backoff policy for this job.
	// Left zero, the policy decides.
	RetryDelay time.Duration `json:"retry_delay,omitempty"`

	IdempotencyKey string `json:"idempotency_key,omitempty"`

	// AvailableAt is when the job next becomes claimable. Zero means
	// immediately. Non-zero after a failed attempt, so a job that failed
	// against a provider that is rate-limiting does not come straight back.
	AvailableAt time.Time `json:"available_at,omitempty"`

	// WorkerID is who has it claimed, for diagnosing a stuck job.
	WorkerID string `json:"worker_id,omitempty"`

	// DeadLetterReason records why the job was parked. Without it a parked job
	// is just a row that stopped moving.
	DeadLetterReason string `json:"dead_letter_reason,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LastError string    `json:"last_error,omitempty"`
}

// Prepare fills in the defaults a caller is not expected to think about: an id,
// a status, a creation time, an attempt ceiling, a non-nil payload.
//
// It is exported so a queue implementation in another package applies exactly
// the defaults the in-memory one does, rather than reinventing them slightly
// differently - which is how a durable queue ends up rejecting jobs the tests
// happily enqueue.
func (j *Job) Prepare(now time.Time) {
	if j.MaxAttempts <= 0 {
		j.MaxAttempts = DefaultMaxAttempts
	}
	if j.Status == "" {
		j.Status = StatusQueued
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = now
	}
	if j.ID == "" {
		j.ID = randomID()
	}
	if j.Payload == nil {
		j.Payload = map[string]any{}
	}
}
