package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Teamthy/i-confess/internal/models"
)

// AudioStore manages voices and audio assets.
type AudioStore struct{ db *sql.DB }

func NewAudioStore(db *sql.DB) *AudioStore { return &AudioStore{db: db} }

func (s *AudioStore) CreateVoice(ctx context.Context, v *models.Voice) error {
	if v.ID == "" {
		v.ID = newID()
	}
	v.CreatedAt, v.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO voices (id,name,description,type,provider,gender,language,premium,status,sample_url,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		v.ID, v.Name, v.Description, v.Type, v.Provider, v.Gender, v.Language, boolInt(v.Premium), v.Status, v.SampleURL, v.CreatedAt, v.UpdatedAt)
	return err
}

func (s *AudioStore) ListVoices(ctx context.Context) ([]models.Voice, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,name,COALESCE(description,''),type,COALESCE(provider,''),COALESCE(gender,''),language,premium,status,COALESCE(sample_url,''),created_at,updated_at
		 FROM voices ORDER BY premium, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Voice
	for rows.Next() {
		var v models.Voice
		if err := rows.Scan(&v.ID, &v.Name, &v.Description, &v.Type, &v.Provider, &v.Gender, &v.Language, &v.Premium, &v.Status, &v.SampleURL, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *AudioStore) VoiceByID(ctx context.Context, id string) (*models.Voice, error) {
	var v models.Voice
	err := s.db.QueryRowContext(ctx,
		`SELECT id,name,COALESCE(description,''),type,COALESCE(provider,''),COALESCE(gender,''),language,premium,status,COALESCE(sample_url,''),created_at,updated_at
		 FROM voices WHERE id = ?`, id).
		Scan(&v.ID, &v.Name, &v.Description, &v.Type, &v.Provider, &v.Gender, &v.Language, &v.Premium, &v.Status, &v.SampleURL, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &v, err
}

func (s *AudioStore) UpsertAsset(ctx context.Context, a *models.AudioAsset) error {
	if a.ID == "" {
		a.ID = newID()
	}
	if a.Status == "" {
		a.Status = "ready"
	}
	a.CreatedAt, a.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audio_assets (id,confession_id,variant_id,voice_id,url,duration_seconds,size_bytes,status,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET url=excluded.url, duration_seconds=excluded.duration_seconds, size_bytes=excluded.size_bytes, status=excluded.status, updated_at=excluded.updated_at`,
		a.ID, a.ConfessionID, nullIfEmpty(a.VariantID), a.VoiceID, a.URL, a.DurationSeconds, a.SizeBytes, a.Status, a.CreatedAt, a.UpdatedAt)
	return err
}

// AssetsFor returns the ready audio assets for a confession, filtered by voice if given.
func (s *AudioStore) AssetsFor(ctx context.Context, confessionID, voiceID string) ([]models.AudioAsset, error) {
	q := `SELECT id,confession_id,COALESCE(variant_id,''),voice_id,url,COALESCE(duration_seconds,0),COALESCE(size_bytes,0),status,created_at,updated_at
	      FROM audio_assets WHERE confession_id = ? AND status = 'ready'`
	args := []any{confessionID}
	if voiceID != "" {
		q += ` AND voice_id = ?`
		args = append(args, voiceID)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.AudioAsset
	for rows.Next() {
		var a models.AudioAsset
		if err := rows.Scan(&a.ID, &a.ConfessionID, &a.VariantID, &a.VoiceID, &a.URL, &a.DurationSeconds, &a.SizeBytes, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ConfessionIDsWithVoice returns confession ids that have a ready asset for the given voice.
func (s *AudioStore) ConfessionIDsWithVoice(ctx context.Context, voiceID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT confession_id FROM audio_assets WHERE voice_id = ? AND status = 'ready'`, voiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
