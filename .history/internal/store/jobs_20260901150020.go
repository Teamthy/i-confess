package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/google/uuid"
)

// JobStore handles persistence of background jobs.
type JobStore struct {
	db *sql.DB
}

func NewJobStore(db *sql.DB) *JobStore {
	return &JobStore{db: db}
}

// Save persists a job record to the database.
func (s *JobStore) Save(ctx context.Context, j *jobs.Job) error {
	if j.ID == "" {
		j.ID = uuid.New().String()
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now().UTC()
	}
	j.UpdatedAt = time.Now().UTC()

	payload, _ := json.Marshal(j.Payload)
	retryDelayMs := int64(j.RetryDelay.Milliseconds())

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO jobs (id, type, payload, status, attempts, max_attempts, last_error, idempotency_key, retry_delay_ms, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET 
		   status=excluded.status, attempts=excluded.attempts, last_error=excluded.last_error, updated_at=excluded.updated_at`,
		j.ID, j.Type, string(payload), j.Status, j.Attempts, j.MaxAttempts, j.LastError,
		nullIfEmpty(j.IdempotencyKey), retryDelayMs, j.CreatedAt.Format(time.RFC3339), j.UpdatedAt.Format(time.RFC3339))
	return err
}

// ByID retrieves a job by its ID.
func (s *JobStore) ByID(ctx context.Context, id string) (*jobs.Job, error) {
	var j jobs.Job
	var payload sql.NullString
	var retryDelayMs int64

	err := s.db.QueryRowContext(ctx,
		`SELECT id, type, COALESCE(payload, '{}'), status, attempts, max_attempts, COALESCE(last_error, ''), idempotency_key, COALESCE(retry_delay_ms, 0), created_at, updated_at
		 FROM jobs WHERE id = ?`, id).
		Scan(&j.ID, &j.Type, &payload, &j.Status, &j.Attempts, &j.MaxAttempts, &j.LastError,
			&j.IdempotencyKey, &retryDelayMs, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if payload.Valid {
		_ = json.Unmarshal([]byte(payload.String), &j.Payload)
	}
	if j.Payload == nil {
		j.Payload = map[string]any{}
	}
	j.RetryDelay = time.Duration(retryDelayMs) * time.Millisecond
	return &j, nil
}

// ListByStatus retrieves jobs by status (for polling workers).
func (s *JobStore) ListByStatus(ctx context.Context, status string, limit int) ([]jobs.Job, error) {
	q := `SELECT id, type, COALESCE(payload, '{}'), status, attempts, max_attempts, COALESCE(last_error, ''), idempotency_key, COALESCE(retry_delay_ms, 0), created_at, updated_at
	      FROM jobs WHERE status = ? ORDER BY created_at ASC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []jobs.Job
	for rows.Next() {
		var j jobs.Job
		var payload sql.NullString
		var retryDelayMs int64
		var createdAt, updatedAt string

		if err := rows.Scan(&j.ID, &j.Type, &payload, &j.Status, &j.Attempts, &j.MaxAttempts, &j.LastError,
			&j.IdempotencyKey, &retryDelayMs, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if payload.Valid {
			_ = json.Unmarshal([]byte(payload.String), &j.Payload)
		}
		if j.Payload == nil {
			j.Payload = map[string]any{}
		}
		j.RetryDelay = time.Duration(retryDelayMs) * time.Millisecond
		j.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		j.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		out = append(out, j)
	}
	return out, rows.Err()
}

// Delete removes a job from the database.
func (s *JobStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE id = ?`, id)
	return err
}
