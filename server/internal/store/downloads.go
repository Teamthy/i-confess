package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Teamthy/i-confess/internal/db"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/google/uuid"
)

// DownloadStore manages offline download licences (PRD §28, §44).
//
// A download is a time-bounded licence to hold audio locally, not a permanent
// copy. Storing an expiry is what makes a cancelled subscription eventually
// stop working offline without relying on the app to police itself — a client
// that has the file could otherwise keep playing it indefinitely.
type DownloadStore struct{ db *db.DB }

func NewDownloadStore(db *db.DB) *DownloadStore { return &DownloadStore{db: db} }

// ErrDownloadLimit is returned when a user is at their plan's cap.
var ErrDownloadLimit = errors.New("download limit reached")

// ActiveCount returns how many live downloads a user holds.
func (s *DownloadStore) ActiveCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audio_downloads
		 WHERE user_id = ? AND removed_at IS NULL AND status != 'removed'`, userID).Scan(&n)
	return n, err
}

// Create records a download licence.
//
// Idempotent on (user, asset): re-requesting a download the user already holds
// refreshes its expiry rather than failing. A client retrying after a dropped
// connection must not be punished for it.
func (s *DownloadStore) Create(ctx context.Context, d *models.Download, ttl time.Duration) error {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	expires := time.Now().UTC().Add(ttl).Format(time.RFC3339)
	d.ExpiresAt = expires
	d.Status = "downloaded"
	d.CreatedAt, d.UpdatedAt = now(), now()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audio_downloads
		   (id,user_id,audio_asset_id,confession_id,voice_id,storage_key,
		    duration_seconds,file_size_bytes,checksum,status,downloaded_at,expires_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(user_id, audio_asset_id) DO UPDATE SET
		   status = 'downloaded', expires_at = excluded.expires_at,
		   removed_at = NULL, downloaded_at = excluded.downloaded_at,
		   updated_at = excluded.updated_at`,
		d.ID, d.UserID, d.AudioAssetID, nullIfEmpty(d.ConfessionID), nullIfEmpty(d.VoiceID),
		d.StorageKey, d.DurationSeconds, d.SizeBytes, nullIfEmpty(d.Checksum),
		d.Status, now(), expires, d.CreatedAt, d.UpdatedAt)
	return err
}

// ListActive returns a user's live downloads.
//
// Expired licences are filtered out rather than deleted: the client needs to
// see that something lapsed so it can purge the local file, and silently
// dropping the row would leave orphaned audio on the device forever.
func (s *DownloadStore) ListActive(ctx context.Context, userID string) ([]models.Download, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT d.id, d.user_id, d.audio_asset_id, COALESCE(d.confession_id,''), COALESCE(d.voice_id,''),
		        COALESCE(d.storage_key,''), COALESCE(d.duration_seconds,0), COALESCE(d.file_size_bytes,0),
		        COALESCE(d.checksum,''), d.status, COALESCE(d.expires_at,''), d.created_at,
		        COALESCE(c.title,'')
		 FROM audio_downloads d
		 LEFT JOIN confessions c ON c.id = d.confession_id
		 WHERE d.user_id = ? AND d.removed_at IS NULL AND d.status != 'removed'
		 ORDER BY d.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.Download{}
	nowT := time.Now().UTC()
	for rows.Next() {
		var d models.Download
		if err := rows.Scan(&d.ID, &d.UserID, &d.AudioAssetID, &d.ConfessionID, &d.VoiceID,
			&d.StorageKey, &d.DurationSeconds, &d.SizeBytes, &d.Checksum,
			&d.Status, &d.ExpiresAt, &d.CreatedAt, &d.Title); err != nil {
			return nil, err
		}
		if t, perr := time.Parse(time.RFC3339, d.ExpiresAt); perr == nil {
			d.Expired = nowT.After(t)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ByID loads one download, verifying ownership in SQL.
func (s *DownloadStore) ByID(ctx context.Context, userID, id string) (*models.Download, error) {
	var d models.Download
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, audio_asset_id, COALESCE(confession_id,''), COALESCE(voice_id,''),
		        COALESCE(storage_key,''), COALESCE(duration_seconds,0), COALESCE(file_size_bytes,0),
		        COALESCE(checksum,''), status, COALESCE(expires_at,''), created_at
		 FROM audio_downloads
		 WHERE id = ? AND user_id = ? AND removed_at IS NULL`, id, userID).
		Scan(&d.ID, &d.UserID, &d.AudioAssetID, &d.ConfessionID, &d.VoiceID,
			&d.StorageKey, &d.DurationSeconds, &d.SizeBytes, &d.Checksum,
			&d.Status, &d.ExpiresAt, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if t, perr := time.Parse(time.RFC3339, d.ExpiresAt); perr == nil {
		d.Expired = time.Now().UTC().After(t)
	}
	return &d, nil
}

// Remove marks a download deleted, freeing a slot against the plan limit.
func (s *DownloadStore) Remove(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE audio_downloads SET status = 'removed', removed_at = ?, updated_at = ?
		 WHERE id = ? AND user_id = ? AND removed_at IS NULL`,
		now(), now(), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeAll marks every download removed, used when entitlement lapses.
//
// The server cannot reach into a phone and delete files, so revocation works by
// refusing to renew: the licence expires, the client sees it gone on next sync,
// and playback of expired content is the client's contract to honour.
func (s *DownloadStore) RevokeAll(ctx context.Context, userID string) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE audio_downloads SET status = 'removed', removed_at = ?, updated_at = ?
		 WHERE user_id = ? AND removed_at IS NULL`, now(), now(), userID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
