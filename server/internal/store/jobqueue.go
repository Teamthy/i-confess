package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Teamthy/i-confess/internal/backoff"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/jobs"
)

// JobQueue is the PostgreSQL implementation of jobs.Queue.
//
// This is the queue the server runs. It differs from jobs.MemoryQueue in the one
// way that matters operationally: the work is in the database, so a deploy, a
// crash or an OOM kill does not discard everything that was waiting.
//
// Claiming uses FOR UPDATE SKIP LOCKED, which is what lets several workers poll
// one table without handing the same job to two of them and without a lock
// manager of our own.
type JobQueue struct {
	db     *db.DB
	policy backoff.Policy
	now    func() time.Time

	mu       sync.RWMutex
	handlers map[string]jobs.Handler
}

// NewJobQueue returns a durable queue using the default retry policy: 30 seconds
// doubling to 30 minutes, which suits work that depends on an external provider.
func NewJobQueue(d *db.DB) *JobQueue {
	return &JobQueue{
		db:       d,
		policy:   backoff.New(30*time.Second, 30*time.Minute),
		now:      func() time.Time { return time.Now().UTC() },
		handlers: map[string]jobs.Handler{},
	}
}

// WithPolicy replaces the retry schedule.
func (q *JobQueue) WithPolicy(p backoff.Policy) *JobQueue {
	q.policy = p
	return q
}

// WithClock replaces the time source, so a test can step past a backoff delay
// instead of sleeping through it.
func (q *JobQueue) WithClock(now func() time.Time) *JobQueue {
	q.now = now
	return q
}

func (q *JobQueue) Register(name string, h jobs.Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers[name] = h
}

func (q *JobQueue) HandlerFor(name string) (jobs.Handler, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	h, ok := q.handlers[name]
	return h, ok
}

const jobSelectColumns = `id, type, COALESCE(payload,'{}'), status, attempts, max_attempts,
	COALESCE(last_error,''), COALESCE(idempotency_key,''), COALESCE(retry_delay_ms,0),
	COALESCE(available_at,''), COALESCE(worker_id,''), COALESCE(dead_letter_reason,''),
	created_at, updated_at`

// Enqueue inserts a job.
//
// An idempotency key that is already in the table does not produce a second job:
// the insert resolves to the existing row and its id comes back with
// jobs.ErrDuplicateJob. That is the durable version of "the same request twice
// does the work once" - and unlike an in-memory set of seen keys, it still holds
// after a restart.
func (q *JobQueue) Enqueue(ctx context.Context, j jobs.Job) (string, error) {
	if j.Type == "" {
		return "", fmt.Errorf("job type is required")
	}
	now := q.now()
	j.Prepare(now)
	j.UpdatedAt = now

	payload, err := json.Marshal(j.Payload)
	if err != nil {
		return "", fmt.Errorf("encode payload: %w", err)
	}

	var id string
	var inserted bool
	// DO UPDATE with a self-assignment is a no-op write whose only purpose is to
	// make RETURNING fire on the conflict path. xmax is zero for a row this
	// statement inserted and non-zero for one it found already there.
	err = q.db.QueryRowContext(ctx, `
		INSERT INTO jobs (id, type, payload, status, attempts, max_attempts, last_error,
		                  idempotency_key, retry_delay_ms, available_at, worker_id,
		                  dead_letter_reason, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT (idempotency_key) DO UPDATE SET updated_at = jobs.updated_at
		RETURNING id, (xmax = 0)`,
		j.ID, j.Type, string(payload), j.Status, j.Attempts, j.MaxAttempts, j.LastError,
		nullIfEmpty(j.IdempotencyKey), j.RetryDelay.Milliseconds(), tsText(j.AvailableAt),
		nullIfEmpty(j.WorkerID), nullIfEmpty(j.DeadLetterReason),
		tsText(j.CreatedAt), tsText(j.UpdatedAt)).Scan(&id, &inserted)
	if err != nil {
		return "", fmt.Errorf("enqueue job: %w", err)
	}
	if !inserted {
		return id, fmt.Errorf("%w: %s", jobs.ErrDuplicateJob, j.IdempotencyKey)
	}
	return id, nil
}

// Claim atomically reserves the oldest job that is due and marks it running.
//
// Ordering is by created_at with id as a tiebreaker. The tiebreaker is not
// cosmetic: timestamps in this schema carry second precision, so every job
// enqueued in the same second ties, and without a second sort key PostgreSQL is
// free to return them in any order at all - which makes a claim unreproducible
// and a test that depends on order flaky. Within one second the order is by id,
// which is stable but arbitrary; strict FIFO across sub-second bursts would need
// a sequence column, and a work queue does not need it.
func (q *JobQueue) Claim(ctx context.Context, workerID string) (*jobs.Job, error) {
	now := q.now()
	nowText := tsText(now)

	var j jobs.Job
	var payload, idem, available, worker, reason string
	var retryDelayMs int64
	var createdAt, updatedAt string

	err := q.db.QueryRowContext(ctx, `
		UPDATE jobs
		SET status = ?, attempts = attempts + 1, worker_id = ?, claimed_at = ?, updated_at = ?
		WHERE id = (
			SELECT id FROM jobs
			WHERE status = ? AND (available_at = '' OR available_at <= ?)
			ORDER BY created_at ASC, id ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+jobSelectColumns,
		jobs.StatusRunning, workerID, nowText, nowText, jobs.StatusQueued, nowText).
		Scan(&j.ID, &j.Type, &payload, &j.Status, &j.Attempts, &j.MaxAttempts, &j.LastError,
			&idem, &retryDelayMs, &available, &worker, &reason, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, jobs.ErrEmptyQueue
	}
	if err != nil {
		return nil, fmt.Errorf("claim job: %w", err)
	}

	if payload != "" {
		_ = json.Unmarshal([]byte(payload), &j.Payload)
	}
	if j.Payload == nil {
		j.Payload = map[string]any{}
	}
	j.IdempotencyKey = idem
	j.WorkerID = worker
	j.DeadLetterReason = reason
	j.RetryDelay = time.Duration(retryDelayMs) * time.Millisecond
	j.AvailableAt = parseTS(available)
	j.CreatedAt = parseTS(createdAt)
	j.UpdatedAt = parseTS(updatedAt)
	return &j, nil
}

// Complete marks a claimed job finished.
func (q *JobQueue) Complete(ctx context.Context, id string) error {
	res, err := q.db.ExecContext(ctx,
		`UPDATE jobs SET status=?, last_error='', worker_id='', updated_at=? WHERE id=? AND status=?`,
		jobs.StatusCompleted, tsText(q.now()), id, jobs.StatusRunning)
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	return checkTouched(res, id)
}

// Fail records a failure and decides whether the job deserves another attempt.
//
// The decision is made inside a transaction that holds the row, so two workers
// reporting on the same job cannot both conclude there is an attempt left.
func (q *JobQueue) Fail(ctx context.Context, id string, cause error) error {
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	permanent := jobs.IsPermanent(cause)
	now := q.now()

	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var attempts, maxAttempts int
	var retryDelayMs int64
	err = tx.QueryRowContext(ctx,
		`SELECT attempts, max_attempts, COALESCE(retry_delay_ms,0) FROM jobs WHERE id=? FOR UPDATE`, id).
		Scan(&attempts, &maxAttempts, &retryDelayMs)
	if err == sql.ErrNoRows {
		return jobs.ErrJobNotFound
	}
	if err != nil {
		return fmt.Errorf("load job for retry: %w", err)
	}
	if maxAttempts <= 0 {
		maxAttempts = jobs.DefaultMaxAttempts
	}

	var (
		status = jobs.StatusQueued
		avail  string
		reason string
	)
	switch {
	case permanent:
		status = jobs.StatusDeadLetter
		reason = "permanent: " + msg
	case attempts >= maxAttempts:
		status = jobs.StatusDeadLetter
		reason = fmt.Sprintf("exhausted %d of %d attempts: %s", attempts, maxAttempts, msg)
	default:
		// An explicit delay on the job wins; otherwise the shared policy grows
		// the wait with the attempt count.
		delay := time.Duration(retryDelayMs) * time.Millisecond
		if delay <= 0 {
			delay = q.policy.Next(attempts)
		}
		avail = tsText(now.Add(delay))
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE jobs SET status=?, last_error=?, available_at=?, dead_letter_reason=?, worker_id='', updated_at=?
		 WHERE id=?`, status, msg, avail, reason, tsText(now), id); err != nil {
		return fmt.Errorf("reschedule job: %w", err)
	}
	return tx.Commit()
}

// FailPermanent parks a job without further attempts.
func (q *JobQueue) FailPermanent(ctx context.Context, id, reason string) error {
	res, err := q.db.ExecContext(ctx,
		`UPDATE jobs SET status=?, dead_letter_reason=?, worker_id='', updated_at=? WHERE id=?`,
		jobs.StatusDeadLetter, reason, tsText(q.now()), id)
	if err != nil {
		return fmt.Errorf("park job: %w", err)
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return jobs.ErrJobNotFound
	}
	return nil
}

// RequeueDead puts parked jobs back in the queue with a fresh attempt count.
func (q *JobQueue) RequeueDead(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	res, err := q.db.ExecContext(ctx, `
		UPDATE jobs SET status=?, attempts=0, available_at='', dead_letter_reason='',
		                last_error='', worker_id='', updated_at=?
		WHERE id IN (
			SELECT id FROM jobs WHERE status=? ORDER BY created_at ASC LIMIT ?
		)`, jobs.StatusQueued, tsText(q.now()), jobs.StatusDeadLetter, limit)
	if err != nil {
		return 0, fmt.Errorf("requeue dead letters: %w", err)
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// ReclaimStale returns jobs that a worker claimed and never finished - because
// the worker died mid-job - to the queue.
//
// Without this a crashed worker strands its work in 'running' forever: the row
// is not due, not failed and not finished, and nothing looks at it again.
func (q *JobQueue) ReclaimStale(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := tsText(q.now().Add(-olderThan))
	res, err := q.db.ExecContext(ctx, `
		UPDATE jobs SET status=?, worker_id='', claimed_at=NULL, updated_at=?
		WHERE status=? AND claimed_at IS NOT NULL AND claimed_at <> '' AND claimed_at <= ?`,
		jobs.StatusQueued, tsText(q.now()), jobs.StatusRunning, cutoff)
	if err != nil {
		return 0, fmt.Errorf("reclaim stale jobs: %w", err)
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// Stats counts jobs by status.
func (q *JobQueue) Stats(ctx context.Context) (jobs.Stats, error) {
	var s jobs.Stats
	rows, err := q.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM jobs GROUP BY status`)
	if err != nil {
		return s, fmt.Errorf("job stats: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return s, err
		}
		s.Total += n
		switch status {
		case jobs.StatusQueued:
			s.Queued = n
		case jobs.StatusRunning:
			s.Running = n
		case jobs.StatusCompleted:
			s.Completed = n
		case jobs.StatusFailed:
			s.Failed = n
		case jobs.StatusDeadLetter:
			s.DeadLetter = n
		}
	}
	return s, rows.Err()
}

// ByID reads one job, for the admin queue view.
func (q *JobQueue) ByID(ctx context.Context, id string) (*jobs.Job, error) {
	var j jobs.Job
	var payload, idem, available, worker, reason, createdAt, updatedAt string
	var retryDelayMs int64
	err := q.db.QueryRowContext(ctx, `SELECT `+jobSelectColumns+` FROM jobs WHERE id=?`, id).
		Scan(&j.ID, &j.Type, &payload, &j.Status, &j.Attempts, &j.MaxAttempts, &j.LastError,
			&idem, &retryDelayMs, &available, &worker, &reason, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, jobs.ErrJobNotFound
	}
	if err != nil {
		return nil, err
	}
	if payload != "" {
		_ = json.Unmarshal([]byte(payload), &j.Payload)
	}
	if j.Payload == nil {
		j.Payload = map[string]any{}
	}
	j.IdempotencyKey = idem
	j.WorkerID = worker
	j.DeadLetterReason = reason
	j.RetryDelay = time.Duration(retryDelayMs) * time.Millisecond
	j.AvailableAt = parseTS(available)
	j.CreatedAt = parseTS(createdAt)
	j.UpdatedAt = parseTS(updatedAt)
	return &j, nil
}

// tsText formats a timestamp the way the rest of this schema stores them.
// The zero time becomes the empty string, which sorts before every timestamp and
// so means "available immediately".
func tsText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func checkTouched(res sql.Result, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %s (or it is no longer running)", jobs.ErrJobNotFound, id)
	}
	return nil
}
