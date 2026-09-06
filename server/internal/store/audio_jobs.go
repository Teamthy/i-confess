package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Teamthy/i-confess/internal/audio"
	"github.com/Teamthy/i-confess/internal/models"
)

// Generation jobs.
//
// audio_generation_jobs has existed since the baseline schema with status,
// attempt counts, error fields and a unique idempotency key, and nothing in the
// codebase had ever written to it. Generation was a synchronous provider call
// inside an HTTP request: no record of what was asked for, no status to poll,
// nothing to retry, and a client disconnect mid-render discarded work the
// provider had already billed for.

const jobColumns = `id, COALESCE(confession_id,''), COALESCE(variant_id,''), voice_id,
	COALESCE(content_version_id,''), COALESCE(provider,''), COALESCE(provider_job_id,''),
	COALESCE(quality_tier,''), COALESCE(format,''), status, attempt_count, max_attempts,
	COALESCE(requested_by,''), COALESCE(started_at,''), COALESCE(completed_at,''),
	COALESCE(error_code,''), COALESCE(error_message,''), COALESCE(audio_asset_id,''),
	COALESCE(idempotency_key,''), created_at, updated_at`

func scanJob(row interface{ Scan(...any) error }) (models.AudioJob, error) {
	var j models.AudioJob
	err := row.Scan(&j.ID, &j.ConfessionID, &j.VariantID, &j.VoiceID, &j.ContentVersionID,
		&j.Provider, &j.ProviderJobID, &j.QualityTier, &j.Format, &j.Status, &j.AttemptCount,
		&j.MaxAttempts, &j.RequestedBy, &j.StartedAt, &j.CompletedAt, &j.ErrorCode,
		&j.ErrorMessage, &j.AudioAssetID, &j.IdempotencyKey, &j.CreatedAt, &j.UpdatedAt)
	return j, err
}

// CreateJob records a generation request.
//
// The second return reports whether this call created the job. When an
// idempotency key is supplied and a job already carries it, the existing job is
// returned instead and nothing new is written - so a double-submitted form or a
// client retry cannot bill the provider twice for the same render.
func (s *AudioStore) CreateJob(ctx context.Context, j *models.AudioJob) (models.AudioJob, bool, error) {
	if j.VoiceID == "" {
		return models.AudioJob{}, false, errors.New("voice_id is required")
	}
	if j.ContentVersionID == "" {
		return models.AudioJob{}, false, errors.New("content_version_id is required")
	}
	if j.ID == "" {
		j.ID = newID()
	}
	if j.Status == "" {
		j.Status = string(audio.JobQueued)
	}
	if !audio.ValidJobStatus(j.Status) {
		return models.AudioJob{}, false, fmt.Errorf("%q is not a valid job status", j.Status)
	}
	if j.MaxAttempts <= 0 {
		j.MaxAttempts = 3
	}
	j.CreatedAt, j.UpdatedAt = now(), now()

	if j.QualityTier == "" {
		j.QualityTier = "standard"
	}
	if j.Format == "" {
		j.Format = "m4a"
	}
	// provider is NOT NULL in the schema, and it is legitimately unknown when a
	// job is recorded before an adapter is chosen. Recording "unknown" keeps the
	// row honest; sending NULL would fail the insert and lose the request.
	if j.Provider == "" {
		j.Provider = "unknown"
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO audio_generation_jobs (id, confession_id, variant_id, voice_id, content_version_id,
		    provider, quality_tier, format, status, attempt_count, max_attempts, requested_by,
		    idempotency_key, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (idempotency_key) DO NOTHING`,
		j.ID, nullIfEmpty(j.ConfessionID), nullIfEmpty(j.VariantID), j.VoiceID, j.ContentVersionID,
		j.Provider, j.QualityTier, j.Format, j.Status, j.AttemptCount, j.MaxAttempts,
		nullIfEmpty(j.RequestedBy), nullIfEmpty(j.IdempotencyKey), j.CreatedAt, j.UpdatedAt)
	if err != nil {
		return models.AudioJob{}, false, fmt.Errorf("insert generation job: %w", err)
	}

	if n, _ := res.RowsAffected(); n == 0 && j.IdempotencyKey != "" {
		existing, err := s.JobByIdempotencyKey(ctx, j.IdempotencyKey)
		if err != nil {
			return models.AudioJob{}, false, err
		}
		return existing, false, nil
	}
	return *j, true, nil
}

// JobByIdempotencyKey returns the job recorded for a key, if any.
func (s *AudioStore) JobByIdempotencyKey(ctx context.Context, key string) (models.AudioJob, error) {
	j, err := scanJob(s.db.QueryRowContext(ctx,
		`SELECT `+jobColumns+` FROM audio_generation_jobs WHERE idempotency_key = ?`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return models.AudioJob{}, ErrNotFound
	}
	return j, err
}

// JobByID returns one generation job.
func (s *AudioStore) JobByID(ctx context.Context, id string) (models.AudioJob, error) {
	j, err := scanJob(s.db.QueryRowContext(ctx,
		`SELECT `+jobColumns+` FROM audio_generation_jobs WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.AudioJob{}, ErrNotFound
	}
	return j, err
}

// Jobs lists generation requests, newest first, optionally filtered by status.
func (s *AudioStore) Jobs(ctx context.Context, status string, limit int) ([]models.AudioJob, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT ` + jobColumns + ` FROM audio_generation_jobs`
	args := []any{}
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC, id LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.AudioJob
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// StartJob marks a job as in flight and counts the attempt.
func (s *AudioStore) StartJob(ctx context.Context, id string) (models.AudioJob, error) {
	return s.transitionJob(ctx, id, audio.JobProcessing, "", "", "", func(existing models.AudioJob) string {
		return ""
	})
}

// RequeueJob returns a failed job to the queue so it can be attempted again.
//
// This is a distinct step from StartJob on purpose: the lifecycle does not
// allow failed -> processing directly, because a retry is a decision that
// should be visible in the job's history rather than something the runner does
// on the side.
func (s *AudioStore) RequeueJob(ctx context.Context, id string) (models.AudioJob, error) {
	return s.transitionJob(ctx, id, audio.JobQueued, "", "", "", nil)
}

// CompleteJob records success and links the resulting asset.
func (s *AudioStore) CompleteJob(ctx context.Context, id, assetID string) (models.AudioJob, error) {
	return s.transitionJob(ctx, id, audio.JobSucceeded, assetID, "", "", nil)
}

// FailJob records failure with a code and message worth showing an operator.
func (s *AudioStore) FailJob(ctx context.Context, id, code, message string) (models.AudioJob, error) {
	return s.transitionJob(ctx, id, audio.JobFailed, "", code, message, nil)
}

// transitionJob is the single place a job's status changes, so the lifecycle
// rules cannot be bypassed by a caller writing the column directly.
func (s *AudioStore) transitionJob(ctx context.Context, id string, to audio.JobStatus,
	assetID, code, message string, _ func(models.AudioJob) string) (models.AudioJob, error) {

	current, err := s.JobByID(ctx, id)
	if err != nil {
		return models.AudioJob{}, err
	}
	if err := audio.ValidateJobTransition(audio.JobStatus(current.Status), to); err != nil {
		return models.AudioJob{}, err
	}

	// started_at is stamped on the first attempt only, so a retried job keeps
	// its original start time and the queue age stays meaningful.
	started := current.StartedAt
	if to == audio.JobProcessing && started == "" {
		started = now()
	}
	attempt := current.AttemptCount
	if to == audio.JobProcessing {
		attempt++
	}
	completed := current.CompletedAt
	if to == audio.JobSucceeded || to == audio.JobFailed || to == audio.JobCancelled {
		completed = now()
	}
	if assetID == "" {
		assetID = current.AudioAssetID
	}
	if code == "" {
		code = current.ErrorCode
	}
	if message == "" {
		message = current.ErrorMessage
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE audio_generation_jobs
		 SET status=?, attempt_count=?, started_at=?, completed_at=?, error_code=?, error_message=?,
		     audio_asset_id=?, updated_at=?
		 WHERE id=?`,
		string(to), attempt, nullIfEmpty(started), nullIfEmpty(completed), nullIfEmpty(code),
		nullIfEmpty(message), nullIfEmpty(assetID), now(), id); err != nil {
		return models.AudioJob{}, fmt.Errorf("update generation job: %w", err)
	}
	return s.JobByID(ctx, id)
}

// JobsForConfession lists a confession's generation history.
func (s *AudioStore) JobsForConfession(ctx context.Context, confessionID string) ([]models.AudioJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+jobColumns+` FROM audio_generation_jobs WHERE confession_id = ?
		 ORDER BY created_at DESC, id`, confessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.AudioJob
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// SetAssetStatus moves an audio asset through its QA lifecycle, recording who
// decided and why.
//
// The codebase previously contained no UPDATE audio_assets statement at all, so
// status was only ever set at INSERT. Generated audio entered 'processing' and
// could never leave: there was no way to approve or reject a render short of
// re-posting the whole asset, which recorded neither reviewer nor reason.
func (s *AudioStore) SetAssetStatus(ctx context.Context, assetID string, to audio.AssetStatus, actor, note string) (models.AudioAsset, error) {
	asset, err := s.AssetByID(ctx, assetID)
	if err != nil {
		return models.AudioAsset{}, err
	}
	if err := audio.ValidateTransition(audio.AssetStatus(asset.Status), to); err != nil {
		return models.AudioAsset{}, err
	}

	reviewed := asset.QAReviewedBy
	reviewedAt := asset.QAReviewedAt
	// A human decision is recorded; a mechanical move to failed or archived is
	// not a review and must not overwrite the last reviewer.
	if to == audio.StatusReady || to == audio.StatusPublished || to == audio.StatusQARejected {
		reviewed = actor
		reviewedAt = now()
	}
	if note == "" {
		note = asset.QANote
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE audio_assets SET status=?, qa_reviewed_by=?, qa_reviewed_at=?, qa_note=?, updated_at=? WHERE id=?`,
		string(to), nullIfEmpty(reviewed), nullIfEmpty(reviewedAt), nullIfEmpty(strings.TrimSpace(note)),
		now(), assetID); err != nil {
		return models.AudioAsset{}, fmt.Errorf("update audio asset status: %w", err)
	}
	return s.AssetByID(ctx, assetID)
}
