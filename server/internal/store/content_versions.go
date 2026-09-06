package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Teamthy/i-confess/internal/models"
)

// Content versions (§9, §62).
//
// Generation snapshots the confession's current text before it synthesizes, so
// a render can always be traced to the exact words that were spoken. Without
// this, editing a confession silently changes the meaning of audio that was
// already approved and is already playing into people's sessions.
//
// The content_versions table has existed since the baseline schema and nothing
// had ever written to it. audio_generation_jobs.content_version_id is a NOT NULL
// foreign key onto it, so recording a generation request was impossible until
// versioning existed.

// EnsureVersion returns the content version for a confession's current text,
// creating one only if the text actually changed.
//
// Idempotence matters: regenerating the same audio for unchanged text must not
// inflate the version history, or "which version is current" stops meaning
// anything.
func (s *ContentStore) EnsureVersion(ctx context.Context, confessionID, title, short, medium, long, language, actor string) (models.ContentVersion, error) {
	if confessionID == "" {
		return models.ContentVersion{}, errors.New("confession_id is required")
	}
	if language == "" {
		language = "en"
	}

	// Reuse the newest version whose text matches exactly. All three lengths
	// are compared because any of them can be the one a variant speaks.
	var existing models.ContentVersion
	err := s.db.QueryRowContext(ctx,
		`SELECT id, confession_id, version_number, title, COALESCE(short_text,''), COALESCE(medium_text,''),
		        COALESCE(long_text,''), language, status, COALESCE(approved_by,''), COALESCE(approved_at,''),
		        COALESCE(published_at,''), created_at, updated_at
		 FROM content_versions
		 WHERE confession_id = ? AND title = ? AND COALESCE(short_text,'') = ?
		   AND COALESCE(medium_text,'') = ? AND COALESCE(long_text,'') = ?
		 ORDER BY version_number DESC LIMIT 1`,
		confessionID, title, short, medium, long,
	).Scan(&existing.ID, &existing.ConfessionID, &existing.VersionNumber, &existing.Title,
		&existing.ShortText, &existing.MediumText, &existing.LongText, &existing.Language,
		&existing.Status, &existing.ApprovedBy, &existing.ApprovedAt, &existing.PublishedAt,
		&existing.CreatedAt, &existing.UpdatedAt)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return models.ContentVersion{}, fmt.Errorf("look up content version: %w", err)
	}

	var next int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version_number), 0) + 1 FROM content_versions WHERE confession_id = ?`,
		confessionID).Scan(&next); err != nil {
		return models.ContentVersion{}, fmt.Errorf("next version number: %w", err)
	}

	v := models.ContentVersion{
		ID: newID(), ConfessionID: confessionID, VersionNumber: next,
		Title: title, ShortText: short, MediumText: medium, LongText: long,
		Language: language,
		// A version starts approved: the editorial gate (§62) is enforced on the
		// confession before generation is allowed at all, so re-approving the
		// snapshot would be a second, weaker gate on the same decision.
		Status:     "approved",
		ApprovedBy: actor,
	}
	if actor != "" {
		v.ApprovedAt = now()
	}
	v.CreatedAt, v.UpdatedAt = now(), now()

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO content_versions (id, confession_id, version_number, title, short_text, medium_text,
		                               long_text, language, status, approved_by, approved_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.ConfessionID, v.VersionNumber, v.Title, v.ShortText, v.MediumText, v.LongText,
		v.Language, v.Status, nullIfEmpty(v.ApprovedBy), nullIfEmpty(v.ApprovedAt), v.CreatedAt, v.UpdatedAt,
	); err != nil {
		return models.ContentVersion{}, fmt.Errorf("insert content version: %w", err)
	}
	return v, nil
}

// VersionByID returns one content version.
func (s *ContentStore) VersionByID(ctx context.Context, id string) (models.ContentVersion, error) {
	var v models.ContentVersion
	err := s.db.QueryRowContext(ctx,
		`SELECT id, confession_id, version_number, title, COALESCE(short_text,''), COALESCE(medium_text,''),
		        COALESCE(long_text,''), language, status, COALESCE(approved_by,''), COALESCE(approved_at,''),
		        COALESCE(published_at,''), created_at, updated_at
		 FROM content_versions WHERE id = ?`, id,
	).Scan(&v.ID, &v.ConfessionID, &v.VersionNumber, &v.Title, &v.ShortText, &v.MediumText,
		&v.LongText, &v.Language, &v.Status, &v.ApprovedBy, &v.ApprovedAt, &v.PublishedAt,
		&v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ContentVersion{}, ErrNotFound
	}
	if err != nil {
		return models.ContentVersion{}, err
	}
	return v, nil
}

// VersionsForConfession lists a confession's versions, newest first.
func (s *ContentStore) VersionsForConfession(ctx context.Context, confessionID string) ([]models.ContentVersion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, confession_id, version_number, title, COALESCE(short_text,''), COALESCE(medium_text,''),
		        COALESCE(long_text,''), language, status, COALESCE(approved_by,''), COALESCE(approved_at,''),
		        COALESCE(published_at,''), created_at, updated_at
		 FROM content_versions WHERE confession_id = ? ORDER BY version_number DESC`, confessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ContentVersion
	for rows.Next() {
		var v models.ContentVersion
		if err := rows.Scan(&v.ID, &v.ConfessionID, &v.VersionNumber, &v.Title, &v.ShortText,
			&v.MediumText, &v.LongText, &v.Language, &v.Status, &v.ApprovedBy, &v.ApprovedAt,
			&v.PublishedAt, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
