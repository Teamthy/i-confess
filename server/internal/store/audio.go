package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/Teamthy/i-confess/internal/audio"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/models"
)

// AudioStore manages voices and audio assets.
type AudioStore struct{ db *db.DB }

func NewAudioStore(db *db.DB) *AudioStore { return &AudioStore{db: db} }

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
	if a.ConfessionID == "" {
		return errors.New("confession_id is required")
	}
	if a.VoiceID == "" {
		return errors.New("voice_id is required")
	}
	if a.URL == "" {
		a.URL = "https://cdn.local/audio/" + a.ID
	}
	if a.DurationSeconds <= 0 {
		a.DurationSeconds = 0
	}
	if a.SizeBytes <= 0 {
		a.SizeBytes = 0
	}

	a.CreatedAt, a.UpdatedAt = now(), now()
	storageKey := a.URL
	if strings.TrimSpace(storageKey) == "" {
		storageKey = "audio/" + a.ID + ".m4a"
	}
	if strings.TrimSpace(a.VariantID) == "" {
		a.VariantID = "default"
	}
	// The conflict target is the table's real uniqueness rule. It used to be
	// ON CONFLICT(id), and because id is a fresh UUID on every call that branch
	// could never fire: a second render for the same confession, version and
	// voice hit the (content_id, content_version_id, voice_id, asset_type,
	// quality_tier) constraint and failed with a 500 instead of replacing the
	// asset. Regenerating audio was therefore impossible.
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO audio_assets (
			id, content_id, content_version_id, voice_id, variant_id,
			asset_type, quality_tier, storage_provider, storage_key, cdn_path,
			format, codec, container,
			duration_seconds, file_size_bytes, status,
			created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(content_id, content_version_id, voice_id, variant_id, asset_type, quality_tier) DO UPDATE SET
			content_id = excluded.content_id,
			voice_id = excluded.voice_id,
			asset_type = excluded.asset_type,
			quality_tier = excluded.quality_tier,
			storage_key = excluded.storage_key,
			cdn_path = excluded.cdn_path,
			format = excluded.format,
			codec = excluded.codec,
			container = excluded.container,
			duration_seconds = excluded.duration_seconds,
			file_size_bytes = excluded.file_size_bytes,
			status = excluded.status,
			-- A new render invalidates the previous review: the bytes a
			-- reviewer approved are no longer the bytes being served.
			qa_reviewed_by = NULL,
			qa_reviewed_at = NULL,
			qa_note = NULL,
			updated_at = excluded.updated_at
		RETURNING id`,
		a.ID,
		a.ConfessionID,
		nullIfEmpty(a.ContentVersionID),
		a.VoiceID,
		// Never NULL: Postgres treats NULLs as distinct in a unique index, so a
		// null variant would let the same render be inserted twice.
		a.VariantID,
		"stream",
		"standard",
		"local",
		storageKey,
		a.URL,
		"m4a",
		"aac",
		"mp4",
		a.DurationSeconds,
		a.SizeBytes,
		a.Status,
		a.CreatedAt,
		a.UpdatedAt,
	).Scan(&a.ID)
	// The id is read back rather than assumed: on conflict the surviving row
	// keeps its own id, and a caller that linked a job to the discarded one
	// would be pointing at an asset that does not exist.
	return err
}

const assetColumns = `id, content_id, COALESCE(variant_id,''), voice_id,
	COALESCE(cdn_path, storage_key), COALESCE(duration_seconds,0), COALESCE(file_size_bytes,0),
	status, COALESCE(content_version_id,''), COALESCE(qa_reviewed_by,''), COALESCE(qa_reviewed_at,''),
	COALESCE(qa_note,''), created_at, updated_at`

func scanAsset(row interface{ Scan(...any) error }) (models.AudioAsset, error) {
	var a models.AudioAsset
	err := row.Scan(&a.ID, &a.ConfessionID, &a.VariantID, &a.VoiceID, &a.URL, &a.DurationSeconds,
		&a.SizeBytes, &a.Status, &a.ContentVersionID, &a.QAReviewedBy, &a.QAReviewedAt, &a.QANote,
		&a.CreatedAt, &a.UpdatedAt)
	return a, err
}

// AssetByID returns one audio asset with its QA record.
func (s *AudioStore) AssetByID(ctx context.Context, id string) (models.AudioAsset, error) {
	a, err := scanAsset(s.db.QueryRowContext(ctx,
		`SELECT `+assetColumns+` FROM audio_assets WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.AudioAsset{}, ErrNotFound
	}
	return a, err
}

// AssetsFor returns the ready audio assets for a confession, filtered by voice if given.
func (s *AudioStore) AssetsFor(ctx context.Context, confessionID, voiceID string) ([]models.AudioAsset, error) {
	// The servable statuses come from internal/audio, the same authority the
	// entitlement gate reads. This was a literal status = 'ready', so the query
	// and the schema's own CHECK vocabulary could drift apart unnoticed.
	served := audio.ServedStatuses()
	q := `SELECT a.id, a.content_id, COALESCE(a.variant_id,''), a.voice_id, COALESCE(a.cdn_path, a.storage_key),
	             COALESCE(a.duration_seconds,0), COALESCE(a.file_size_bytes,0), a.status, a.created_at, a.updated_at
	      FROM audio_assets a
	      LEFT JOIN content_versions cv ON cv.id = a.content_version_id
	      WHERE a.content_id = ? AND a.status IN (` + placeholders(len(served)) + `)`
	args := []any{confessionID}
	for _, st := range served {
		args = append(args, string(st))
	}
	if voiceID != "" {
		q += ` AND a.voice_id = ?`
		args = append(args, voiceID)
	}
	// Newest content version first, then newest render, then id for stability.
	//
	// This was ORDER BY created_at, id - ascending - so matchAsset's "first
	// asset for this confession" fallback picked the OLDEST render. Once
	// versioning exists that means editing a confession and re-voicing it
	// leaves every new session playing audio rendered from the superseded text,
	// with no way to correct it short of deleting the old asset. Verified
	// before fixing: adding a v2 render left the engine selecting v1.
	//
	// Assets with no content version are legacy and sort last rather than
	// first, so a versioned render always wins over an unattributed one.
	// This must come last: the voice filter above appends to the WHERE clause.
	q += ` ORDER BY cv.version_number DESC NULLS LAST, a.created_at DESC, a.id`
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

// ConfessionIDsWithVoice returns confession ids that have a servable asset for
// the given voice.
func (s *AudioStore) ConfessionIDsWithVoice(ctx context.Context, voiceID string) (map[string]bool, error) {
	served := audio.ServedStatuses()
	args := make([]any, 0, 1+len(served))
	args = append(args, voiceID)
	for _, st := range served {
		args = append(args, string(st))
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT content_id FROM audio_assets WHERE voice_id = ? AND status IN (`+placeholders(len(served))+`)`,
		args...)
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

// RecordAudit writes a privileged-action record (PRD S51, S80).
//
// Audit writes must never fail the request that succeeded: losing a log line is
// bad, but rolling back a completed rights change because the log table was
// busy is worse. Callers log the error and continue.
func (s *AudioStore) RecordAudit(ctx context.Context, actor, action, entity, entityID, detail, result string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_logs (id, admin_user_id, actor, action, entity, entity_id, detail, result, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		newID(), nullIfEmpty(actor), actor, action, entity, nullIfEmpty(entityID),
		truncateAudit(detail), result, now())
	return err
}

// AuditEntry is one recorded privileged action.
type AuditEntry struct {
	ID        string `json:"id"`
	Actor     string `json:"actor,omitempty"`
	Action    string `json:"action"`
	Entity    string `json:"entity"`
	EntityID  string `json:"entity_id,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Result    string `json:"result,omitempty"`
	CreatedAt string `json:"created_at"`
}

// AuditTrail returns recent privileged actions, newest first.
func (s *AudioStore) AuditTrail(ctx context.Context, entity, entityID string, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, COALESCE(actor,''), action, entity, COALESCE(entity_id,''),
	             COALESCE(detail,''), COALESCE(result,''), created_at
	      FROM audit_logs`
	args := []any{}
	if entity != "" {
		q += ` WHERE entity = ?`
		args = append(args, entity)
		if entityID != "" {
			q += ` AND entity_id = ?`
			args = append(args, entityID)
		}
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.Entity, &e.EntityID,
			&e.Detail, &e.Result, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func truncateAudit(s string) string {
	if len(s) > 1000 {
		return s[:1000]
	}
	return s
}
