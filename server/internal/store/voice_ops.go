package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/Teamthy/i-confess/internal/voiceeval"
)

// ---------------------------------------------------------------- expiry (§68)

// VoicesDueForExpiry returns voices whose grant is still in an authorizing or
// suspended state but whose expiry has passed. The rights check already
// refuses them (EffectiveStatus), but the stored status is flipped too so the
// admin UI, the audit log and the asset policy all agree.
func (s *VoicePlatformStore) VoicesDueForExpiry(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT voice_id FROM voice_rights_grants
		WHERE deleted_at IS NULL AND status IN ('APPROVED','RESTRICTED','SUSPENDED')
		AND expires_at IS NOT NULL AND expires_at <> '' AND expires_at <= ?`, tsNow())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// StoredGenerations lists completed renders for a voice that still have an
// object in storage.
func (s *VoicePlatformStore) StoredGenerations(ctx context.Context, voiceID string, limit int) ([]Generation, error) {
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+genCols+` FROM voice_generations
		WHERE voice_id = ? AND deleted_at IS NULL AND status = 'COMPLETED' AND storage_key IS NOT NULL AND storage_key <> ''
		ORDER BY created_at LIMIT ?`, voiceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Generation
	for rows.Next() {
		g, err := scanGen(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

// MarkGenerationPurged records that a render's object was deleted under a
// "delete" post-termination policy. The row stays (soft-deleted) as evidence.
func (s *VoicePlatformStore) MarkGenerationPurged(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE voice_generations SET storage_key = NULL, deleted_at = ?, updated_at = ?,
		row_version = row_version + 1 WHERE id = ?`, tsNow(), tsNow(), id)
	return err
}

// ---------------------------------------------------------------- sessions (§33)

// SessionItem is one section of an audio session.
type SessionItem struct {
	ID           string      `json:"id"`
	Position     int         `json:"position"`
	SectionType  string      `json:"type"`
	Style        string      `json:"style,omitempty"`
	GenerationID string      `json:"generationId,omitempty"`
	PauseMS      int         `json:"pauseMs,omitempty"`
	TextSHA256   string      `json:"textHash,omitempty"`
	Generation   *Generation `json:"generation,omitempty"`
}

// AudioSession is an ordered, multi-style listening session.
type AudioSession struct {
	ID          string        `json:"id"`
	OwnerUserID string        `json:"-"`
	VoiceID     string        `json:"voiceId"`
	Title       string        `json:"title,omitempty"`
	Purpose     string        `json:"purpose"`
	Visibility  string        `json:"visibility"`
	CreatedAt   string        `json:"createdAt"`
	Items       []SessionItem `json:"items"`
}

// CreateAudioSession stores a session and its items atomically.
func (s *VoicePlatformStore) CreateAudioSession(ctx context.Context, sess *AudioSession) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	sess.ID = "sess_" + uuid.NewString()
	now := tsNow()
	sess.CreatedAt = now
	if _, err := tx.ExecContext(ctx, `INSERT INTO audio_sessions (id, owner_user_id, voice_id, title, purpose, visibility, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, sess.ID, sess.OwnerUserID, sess.VoiceID, nullIfEmpty(sess.Title), sess.Purpose,
		sess.Visibility, now, now); err != nil {
		return err
	}
	for i := range sess.Items {
		it := &sess.Items[i]
		it.ID = uuid.NewString()
		it.Position = i
		if _, err := tx.ExecContext(ctx, `INSERT INTO audio_session_items (id, session_id, position, section_type, style,
			generation_id, pause_ms, text_sha256, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			it.ID, sess.ID, it.Position, it.SectionType, nullIfEmpty(it.Style), nullIfEmpty(it.GenerationID), it.PauseMS,
			nullIfEmpty(it.TextSHA256), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AudioSessionByID loads a session with its items and their generations.
func (s *VoicePlatformStore) AudioSessionByID(ctx context.Context, id string) (*AudioSession, error) {
	var sess AudioSession
	var title sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id, owner_user_id, voice_id, title, purpose, visibility, created_at
		FROM audio_sessions WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&sess.ID, &sess.OwnerUserID, &sess.VoiceID, &title, &sess.Purpose, &sess.Visibility, &sess.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sess.Title = title.String
	rows, err := s.db.QueryContext(ctx, `SELECT id, position, section_type, COALESCE(style,''), COALESCE(generation_id,''),
		pause_ms, COALESCE(text_sha256,'') FROM audio_session_items WHERE session_id = ? AND deleted_at IS NULL ORDER BY position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var it SessionItem
		if err := rows.Scan(&it.ID, &it.Position, &it.SectionType, &it.Style, &it.GenerationID, &it.PauseMS, &it.TextSHA256); err != nil {
			return nil, err
		}
		sess.Items = append(sess.Items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range sess.Items {
		if gid := sess.Items[i].GenerationID; gid != "" {
			g, err := s.GenerationByID(ctx, gid)
			if err != nil {
				return nil, err
			}
			sess.Items[i].Generation = g
		}
	}
	return &sess, nil
}

// AudioSpend is measured cost for a set of generations.
type AudioSpend struct {
	// InferenceSeconds is the sum of what workers reported spending on
	// inference. Rows from a worker that reported nothing contribute 0, which
	// is why Completed and Metered differ: a spend total computed over 3 of 400
	// renders has to be readable as such.
	InferenceSeconds float64 `json:"inferenceSeconds"`
	AudioSeconds     float64 `json:"audioSeconds"`
	Completed        int     `json:"completed"`
	// Metered counts rows with a reported inference_seconds; Priced counts rows
	// with a cost, which additionally requires the deployment to have set a
	// price. Both are needed to tell "no spend" from "spend nobody measured".
	Metered    int   `json:"metered"`
	Priced     int   `json:"priced"`
	CostMicros int64 `json:"costUsdMicros"`
}

// SecondsPerAudioSecond is the measured cost ratio - how many GPU-seconds one
// second of finished audio takes. ok is false when nothing has been measured
// yet, and the caller must then fall back to a stated assumption instead of
// dividing by zero or treating the engine as free.
func (s AudioSpend) SecondsPerAudioSecond() (float64, bool) {
	if s.Metered == 0 || s.AudioSeconds <= 0 {
		return 0, false
	}
	return s.InferenceSeconds / s.AudioSeconds, true
}

// VoiceSpend measures cost over completed generations, optionally for one
// voice and optionally bounded to those created at or after `since`
// (RFC3339 UTC; empty means unbounded). This is the only place spend is
// aggregated from, so the daily budget, the batch estimate and the metrics
// endpoint cannot drift to different definitions of "what a render cost".
func (s *VoicePlatformStore) VoiceSpend(ctx context.Context, voiceID, since string) (AudioSpend, error) {
	var out AudioSpend
	q := `SELECT COUNT(*),
	       COALESCE(SUM(inference_seconds), 0),
	       COALESCE(SUM(CASE WHEN inference_seconds IS NOT NULL THEN COALESCE(duration_ms, 0) / 1000.0 ELSE 0 END), 0),
	       COALESCE(SUM(cost_usd_micros), 0),
	       COUNT(inference_seconds),
	       COUNT(cost_usd_micros)
	   FROM voice_generations
	  WHERE status = 'COMPLETED' AND deleted_at IS NULL`
	args := make([]any, 0, 2)
	if voiceID != "" {
		q += ` AND voice_id = ?`
		args = append(args, voiceID)
	}
	if since != "" {
		q += ` AND created_at >= ?`
		args = append(args, since)
	}
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&out.Completed, &out.InferenceSeconds, &out.AudioSeconds,
		&out.CostMicros, &out.Metered, &out.Priced)
	return out, err
}

// BatchItem is one line of an admin batch.
type BatchItem struct {
	Position     int    `json:"position"`
	Label        string `json:"label,omitempty"`
	GenerationID string `json:"generationId,omitempty"`
	Error        string `json:"error,omitempty"`
	Status       string `json:"status,omitempty"`
}

// Batch is a bulk generation request (e.g. Psalm 1-150).
type Batch struct {
	ID        string         `json:"id"`
	VoiceID   string         `json:"voiceId"`
	Title     string         `json:"title"`
	Purpose   string         `json:"purpose"`
	Style     string         `json:"style"`
	Kind      string         `json:"kind"`
	Total     int            `json:"total"`
	Refused   int            `json:"refused"`
	CreatedBy string         `json:"createdBy,omitempty"`
	CreatedAt string         `json:"createdAt"`
	Progress  map[string]int `json:"progress,omitempty"`
	Items     []BatchItem    `json:"items,omitempty"`
	// The planned spend, recorded when the batch was queued (VE-017). Nil
	// means the batch predates the budget or was queued with no ceiling
	// configured; the distinction matters when reading an old batch back.
	EstInferenceSeconds *float64 `json:"estInferenceSeconds,omitempty"`
	EstCostMicros       *int64   `json:"estCostUsdMicros,omitempty"`
	EstimateBasis       string   `json:"estimateBasis,omitempty"`
	// Actual is measured spend for this batch's renders. Filled in by
	// BatchByID only: the list endpoint stays at one query per batch rather
	// than two.
	Actual *AudioSpend `json:"actualSpend,omitempty"`
}

// CreateBatch records a batch and its items.
func (s *VoicePlatformStore) CreateBatch(ctx context.Context, b *Batch) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	b.ID = "batch_" + uuid.NewString()[:13]
	b.CreatedAt = tsNow()
	b.Total = len(b.Items)
	b.Refused = 0
	for _, it := range b.Items {
		if it.GenerationID == "" {
			b.Refused++
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO voice_batches (id, voice_id, title, purpose, style, kind, total, refused,
		created_by, created_at, est_inference_seconds, est_cost_usd_micros, estimate_basis)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, b.ID, b.VoiceID, b.Title, b.Purpose, b.Style, b.Kind, b.Total, b.Refused,
		nullIfEmpty(b.CreatedBy), b.CreatedAt, b.EstInferenceSeconds, b.EstCostMicros, nullIfEmpty(b.EstimateBasis)); err != nil {
		return err
	}
	for _, it := range b.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO voice_batch_items (id, batch_id, position, label, generation_id, error, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, uuid.NewString(), b.ID, it.Position, nullIfEmpty(it.Label), nullIfEmpty(it.GenerationID),
			nullIfEmpty(it.Error), b.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Batches lists a voice's batches with per-status progress.
func (s *VoicePlatformStore) Batches(ctx context.Context, voiceID string) ([]Batch, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, voice_id, title, purpose, style, kind, total, refused, COALESCE(created_by,''), created_at,
		est_inference_seconds, est_cost_usd_micros, COALESCE(estimate_basis,'')
		FROM voice_batches WHERE voice_id = ? AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 100`, voiceID)
	if err != nil {
		return nil, err
	}
	var out []Batch
	for rows.Next() {
		var b Batch
		if err := rows.Scan(&b.ID, &b.VoiceID, &b.Title, &b.Purpose, &b.Style, &b.Kind, &b.Total, &b.Refused, &b.CreatedBy, &b.CreatedAt,
			&b.EstInferenceSeconds, &b.EstCostMicros, &b.EstimateBasis); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		p, err := s.batchProgress(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Progress = p
	}
	return out, nil
}

// BatchByID returns one batch with items and live generation status.
func (s *VoicePlatformStore) BatchByID(ctx context.Context, id string) (*Batch, error) {
	var b Batch
	err := s.db.QueryRowContext(ctx, `SELECT id, voice_id, title, purpose, style, kind, total, refused, COALESCE(created_by,''), created_at,
		est_inference_seconds, est_cost_usd_micros, COALESCE(estimate_basis,'')
		FROM voice_batches WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&b.ID, &b.VoiceID, &b.Title, &b.Purpose, &b.Style, &b.Kind, &b.Total, &b.Refused, &b.CreatedBy, &b.CreatedAt,
			&b.EstInferenceSeconds, &b.EstCostMicros, &b.EstimateBasis)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT i.position, COALESCE(i.label,''), COALESCE(i.generation_id,''), COALESCE(i.error,''),
		COALESCE(g.status,'') FROM voice_batch_items i LEFT JOIN voice_generations g ON g.id = i.generation_id
		WHERE i.batch_id = ? ORDER BY i.position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var it BatchItem
		if err := rows.Scan(&it.Position, &it.Label, &it.GenerationID, &it.Error, &it.Status); err != nil {
			return nil, err
		}
		b.Items = append(b.Items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	b.Progress, err = s.batchProgress(ctx, id)
	if err != nil {
		return nil, err
	}
	// What the batch actually cost, next to what it was budgeted for. Reading
	// only the estimate would leave the gate unfalsifiable: the number that
	// matters in hindsight is the one measured off the renders.
	spend, err := s.batchSpend(ctx, id)
	if err != nil {
		return nil, err
	}
	b.Actual = &spend
	return &b, nil
}

// batchSpend aggregates measured cost over the generations one batch queued.
func (s *VoicePlatformStore) batchSpend(ctx context.Context, id string) (AudioSpend, error) {
	var out AudioSpend
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(g.id),
	       COALESCE(SUM(g.inference_seconds), 0),
	       COALESCE(SUM(CASE WHEN g.inference_seconds IS NOT NULL THEN COALESCE(g.duration_ms, 0) / 1000.0 ELSE 0 END), 0),
	       COALESCE(SUM(g.cost_usd_micros), 0),
	       COUNT(g.inference_seconds), COUNT(g.cost_usd_micros)
	   FROM voice_batch_items i JOIN voice_generations g ON g.id = i.generation_id
	  WHERE i.batch_id = ? AND g.status = 'COMPLETED' AND g.deleted_at IS NULL`, id).
		Scan(&out.Completed, &out.InferenceSeconds, &out.AudioSeconds, &out.CostMicros, &out.Metered, &out.Priced)
	return out, err
}

func (s *VoicePlatformStore) batchProgress(ctx context.Context, id string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT COALESCE(g.status,'REFUSED'), COUNT(*) FROM voice_batch_items i
		LEFT JOIN voice_generations g ON g.id = i.generation_id WHERE i.batch_id = ? GROUP BY 1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- blind evaluation (§50)

// BlindTest is a blind listening test. Clip sources stay hidden while open.
type BlindTest struct {
	ID        string           `json:"id"`
	VoiceID   string           `json:"voiceId"`
	Title     string           `json:"title"`
	Status    string           `json:"status"`
	CreatedBy string           `json:"createdBy,omitempty"`
	CreatedAt string           `json:"createdAt"`
	ClosedAt  string           `json:"closedAt,omitempty"`
	Clips     []voiceeval.Clip `json:"clips"`
	// AudioKeys maps clip id -> storage key; never serialised.
	AudioKeys  map[string]string `json:"-"`
	Evaluators int               `json:"evaluators"`
	Ratings    int               `json:"ratings"`
}

// ErrBlindTestClosed is returned when rating a closed test.
var ErrBlindTestClosed = errors.New("blind test is closed")

// CreateBlindTest stores a test whose clips were already shuffled and
// labelled by voiceeval.Blind.
func (s *VoicePlatformStore) CreateBlindTest(ctx context.Context, t *BlindTest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	t.ID = "blind_" + uuid.NewString()[:13]
	t.Status, t.CreatedAt = "open", tsNow()
	if _, err := tx.ExecContext(ctx, `INSERT INTO voice_blind_tests (id, voice_id, title, status, created_by, created_at)
		VALUES (?, ?, ?, 'open', ?, ?)`, t.ID, t.VoiceID, t.Title, nullIfEmpty(t.CreatedBy), t.CreatedAt); err != nil {
		return err
	}
	for i := range t.Clips {
		c := &t.Clips[i]
		c.ID = uuid.NewString()
		if _, err := tx.ExecContext(ctx, `INSERT INTO voice_blind_clips (id, test_id, blind_label, source, audio_key, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`, c.ID, t.ID, c.BlindLabel, c.Source, t.AudioKeys[c.BlindLabel], t.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// BlindTestByID loads a test with clips (sources included; callers decide
// whether to reveal them) and rating counts.
func (s *VoicePlatformStore) BlindTestByID(ctx context.Context, id string) (*BlindTest, error) {
	var t BlindTest
	var createdBy, closedAt sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id, voice_id, title, status, created_by, created_at, closed_at
		FROM voice_blind_tests WHERE id = ? AND deleted_at IS NULL`, id).
		Scan(&t.ID, &t.VoiceID, &t.Title, &t.Status, &createdBy, &t.CreatedAt, &closedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.CreatedBy, t.ClosedAt = createdBy.String, closedAt.String
	rows, err := s.db.QueryContext(ctx, `SELECT id, blind_label, source, audio_key FROM voice_blind_clips
		WHERE test_id = ? AND deleted_at IS NULL ORDER BY blind_label`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	t.AudioKeys = map[string]string{}
	for rows.Next() {
		var c voiceeval.Clip
		var key string
		if err := rows.Scan(&c.ID, &c.BlindLabel, &c.Source, &key); err != nil {
			return nil, err
		}
		t.Clips = append(t.Clips, c)
		t.AudioKeys[c.ID] = key
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT evaluator_id), COUNT(*) FROM voice_blind_ratings
		WHERE test_id = ? AND deleted_at IS NULL`, id).Scan(&t.Evaluators, &t.Ratings)
	return &t, err
}

// BlindTests lists a voice's tests.
func (s *VoicePlatformStore) BlindTests(ctx context.Context, voiceID string) ([]BlindTest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.voice_id, t.title, t.status, t.created_at, COALESCE(t.closed_at,''),
		(SELECT COUNT(DISTINCT evaluator_id) FROM voice_blind_ratings r WHERE r.test_id = t.id AND r.deleted_at IS NULL),
		(SELECT COUNT(*) FROM voice_blind_ratings r WHERE r.test_id = t.id AND r.deleted_at IS NULL)
		FROM voice_blind_tests t WHERE t.voice_id = ? AND t.deleted_at IS NULL ORDER BY t.created_at DESC`, voiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BlindTest
	for rows.Next() {
		var t BlindTest
		if err := rows.Scan(&t.ID, &t.VoiceID, &t.Title, &t.Status, &t.CreatedAt, &t.ClosedAt, &t.Evaluators, &t.Ratings); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RateBlindClip upserts one evaluator's rating for one clip and dimension.
func (s *VoicePlatformStore) RateBlindClip(ctx context.Context, testID string, r voiceeval.Rating) error {
	var status string
	var clipTest string
	if err := s.db.QueryRowContext(ctx, `SELECT t.status, c.test_id FROM voice_blind_clips c JOIN voice_blind_tests t ON t.id = c.test_id
		WHERE c.id = ? AND c.deleted_at IS NULL`, r.ClipID).Scan(&status, &clipTest); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("unknown clip %q", r.ClipID)
		}
		return err
	}
	if clipTest != testID {
		return fmt.Errorf("clip %q is not in test %q", r.ClipID, testID)
	}
	if status != "open" {
		return ErrBlindTestClosed
	}
	now := tsNow()
	_, err := s.db.ExecContext(ctx, `INSERT INTO voice_blind_ratings (id, test_id, clip_id, evaluator_id, dimension, rating, comments, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (clip_id, evaluator_id, dimension) DO UPDATE SET rating = EXCLUDED.rating, comments = EXCLUDED.comments,
		created_at = EXCLUDED.created_at, row_version = voice_blind_ratings.row_version + 1`,
		uuid.NewString(), testID, r.ClipID, r.EvaluatorID, string(r.Dimension), r.Value, nullIfEmpty(r.Comments), now)
	return err
}

// BlindRatings returns every rating in a test.
func (s *VoicePlatformStore) BlindRatings(ctx context.Context, testID string) ([]voiceeval.Rating, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT evaluator_id, clip_id, dimension, rating, COALESCE(comments,'')
		FROM voice_blind_ratings WHERE test_id = ? AND deleted_at IS NULL`, testID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []voiceeval.Rating
	for rows.Next() {
		r := voiceeval.Rating{TestID: testID}
		var dim string
		if err := rows.Scan(&r.EvaluatorID, &r.ClipID, &dim, &r.Value, &r.Comments); err != nil {
			return nil, err
		}
		r.Dimension = voiceeval.Dimension(dim)
		out = append(out, r)
	}
	return out, rows.Err()
}

// CloseBlindTest closes a test so its sources can be revealed.
func (s *VoicePlatformStore) CloseBlindTest(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE voice_blind_tests SET status = 'closed', closed_at = ?, row_version = row_version + 1
		WHERE id = ? AND status = 'open'`, tsNow(), id)
	return err
}
