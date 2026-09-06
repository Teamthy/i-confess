package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Teamthy/i-confess/internal/db"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/sessions"
)

// SessionStore manages sessions and session items.
type SessionStore struct{ db *db.DB }

// defaultSessionStrategy is the duration strategy applied when a caller does
// not name one. Keep in sync with engine.DefaultStrategy; the store cannot
// import engine, because engine imports the store.
const defaultSessionStrategy = "BALANCED"

func NewSessionStore(db *db.DB) *SessionStore { return &SessionStore{db: db} }

func (s *SessionStore) Create(ctx context.Context, sess *models.Session) error {
	if sess.ID == "" {
		sess.ID = newID()
	}
	if sess.Status == "" {
		sess.Status = string(sessions.Ready)
	}
	if sess.Strategy == "" {
		// Mirrors engine.DefaultStrategy. The store cannot import engine —
		// engine imports the store, so that would be a cycle. engine.Build
		// always sets this explicitly; the fallback only covers rows built
		// elsewhere.
		sess.Strategy = defaultSessionStrategy
	}
	sess.CreatedAt = now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO sessions (id,user_id,type,duration_seconds,strategy,target_duration,actual_duration,title,description,voice_id,status,created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		sess.ID, sess.UserID, sess.Type, sess.DurationSeconds, sess.Strategy,
		sess.TargetDuration, sess.ActualDuration, nullIfEmpty(sess.Title), nullIfEmpty(sess.Description),
		nullIfEmpty(sess.VoiceID), sess.Status, sess.CreatedAt)
	if err != nil {
		return err
	}
	for i := range sess.Items {
		it := &sess.Items[i]
		if it.ID == "" {
			it.ID = newID()
		}
		it.SessionID = sess.ID
		if it.Status == "" {
			it.Status = "queued"
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO session_items (id,session_id,confession_id,variant_id,voice_id,audio_asset_id,position,duration_seconds,status)
			 VALUES (?,?,?,?,?,?,?,?,?)`,
			it.ID, it.SessionID, it.ConfessionID, nullIfEmpty(it.VariantID), nullIfEmpty(it.VoiceID), nullIfEmpty(it.AudioAssetID), it.Position, it.DurationSeconds, it.Status)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SessionStore) ByID(ctx context.Context, id string) (*models.Session, error) {
	var sess models.Session
	var voiceID, startedAt, completedAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,type,duration_seconds,COALESCE(strategy,''),COALESCE(target_duration,0),COALESCE(actual_duration,0),
		        COALESCE(title,''),COALESCE(description,''),voice_id,status,created_at,started_at,completed_at
		 FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.UserID, &sess.Type, &sess.DurationSeconds, &sess.Strategy,
			&sess.TargetDuration, &sess.ActualDuration, &sess.Title, &sess.Description,
			&voiceID, &sess.Status, &sess.CreatedAt, &startedAt, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if voiceID.Valid {
		sess.VoiceID = voiceID.String
	}
	if startedAt.Valid {
		sess.StartedAt = startedAt.String
	}
	if completedAt.Valid {
		sess.CompletedAt = completedAt.String
	}
	items, err := s.Items(ctx, id)
	if err != nil {
		return nil, err
	}
	sess.Items = items
	return &sess, nil
}

func (s *SessionStore) Items(ctx context.Context, sessionID string) ([]models.SessionItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT si.id, si.confession_id, COALESCE(si.variant_id,''), COALESCE(si.voice_id,''), COALESCE(si.audio_asset_id,''), si.position, si.duration_seconds, si.status,
		        c.title, cat.name, COALESCE(a.cdn_path, a.storage_key, ''), COALESCE(c.medium_text, c.short_text, '')
		 FROM session_items si
		 JOIN confessions c ON c.id = si.confession_id
		 JOIN categories cat ON cat.id = c.category_id
		 LEFT JOIN audio_assets a ON a.id = si.audio_asset_id
		 WHERE si.session_id = ? ORDER BY si.position`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SessionItem
	for rows.Next() {
		var it models.SessionItem
		if err := rows.Scan(&it.ID, &it.ConfessionID, &it.VariantID, &it.VoiceID, &it.AudioAssetID, &it.Position, &it.DurationSeconds, &it.Status, &it.Title, &it.Category, &it.AudioURL, &it.Text); err != nil {
			return nil, err
		}
		it.SessionID = sessionID
		out = append(out, it)
	}
	return out, rows.Err()
}

// UpdateStatus moves a session to a canonical lifecycle state and stamps the
// timestamps that state implies.
//
// It deliberately does not validate the transition. Whether a move is legal is
// domain policy and lives in the sessions package, enforced by the API layer
// before this is reached. Keeping the policy in exactly one place means a
// second caller cannot quietly get a different answer.
//
// started_at and completed_at are written once and never overwritten. Resuming
// from PAUSED must not move started_at, or total listening time is understated;
// replaying a completion must not move completed_at, or the recorded finish
// time drifts on every retry.
func (s *SessionStore) UpdateStatus(ctx context.Context, id, status string) error {
	ts := now()
	switch sessions.State(status) {
	case sessions.Starting, sessions.Active:
		_, err := s.db.ExecContext(ctx,
			`UPDATE sessions SET status=?, started_at=COALESCE(NULLIF(started_at,''), ?) WHERE id=?`,
			status, ts, id)
		return err
	case sessions.Completed:
		_, err := s.db.ExecContext(ctx,
			`UPDATE sessions SET status=?, completed_at=COALESCE(NULLIF(completed_at,''), ?) WHERE id=?`,
			status, ts, id)
		return err
	default:
		_, err := s.db.ExecContext(ctx, `UPDATE sessions SET status=? WHERE id=?`, status, id)
		return err
	}
}

func (s *SessionStore) ListByUser(ctx context.Context, userID string, limit int) ([]models.Session, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,type,duration_seconds,COALESCE(strategy,''),COALESCE(target_duration,0),COALESCE(actual_duration,0),
		        COALESCE(title,''),COALESCE(description,''),COALESCE(voice_id,''),status,created_at,
		        COALESCE(started_at,''),COALESCE(completed_at,'')
		 FROM sessions WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Session
	for rows.Next() {
		var sess models.Session
		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.Type, &sess.DurationSeconds, &sess.Strategy,
			&sess.TargetDuration, &sess.ActualDuration, &sess.Title, &sess.Description,
			&sess.VoiceID, &sess.Status, &sess.CreatedAt, &sess.StartedAt, &sess.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// UpdateItemStatus sets one queue item's status.
//
// The status must already be canonical; the sessions package owns that
// vocabulary and the API layer normalises before reaching here.
func (s *SessionStore) UpdateItemStatus(ctx context.Context, itemID string, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE session_items SET status=? WHERE id=?`, status, itemID)
	return err
}

// SetPlayingItem marks one item as playing and demotes any other item in the
// session that still claimed to be. Without the demotion, an interrupted
// playback leaves two items marked PLAYING and the queue view lies about what
// is on screen.
func (s *SessionStore) SetPlayingItem(ctx context.Context, sessionID, itemID string, playing string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE session_items SET status=? WHERE session_id=? AND status=?`,
		string(sessions.ItemQueued), sessionID, playing); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE session_items SET status=? WHERE id=? AND session_id=?`,
		playing, itemID, sessionID); err != nil {
		return err
	}
	return tx.Commit()
}

// CountItems returns how many items are in each canonical status. Used to
// compute completion percentage and to decide when a session has run to its
// end.
func (s *SessionStore) CountItems(ctx context.Context, sessionID string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM session_items WHERE session_id=? GROUP BY status`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		// Fold legacy spellings so counts are correct for sessions created
		// before the canonical vocabulary existed.
		if c := sessions.NormalizeItemStatus(status); c != "" {
			status = c
		}
		out[status] += n
	}
	return out, rows.Err()
}

// SaveProgress records where the listener stopped, resolving multi-device
// conflicts by timestamp (§36).
//
// The client's own last_updated_at is stored verbatim and compared verbatim, so
// there is exactly one clock in play per device and the outcome does not depend
// on arrival order: the later timestamp wins. If the incoming update is older
// than what is stored it is discarded and the stored row is returned with
// applied=false, which lets the caller tell the client "your other device is
// further along" instead of silently rewinding it.
func (s *SessionStore) SaveProgress(ctx context.Context, p *models.SessionProgress) (stored models.SessionProgress, applied bool, err error) {
	existing, err := s.Progress(ctx, p.SessionID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return models.SessionProgress{}, false, err
	}
	if err == nil {
		if p.LastUpdatedAt != "" && existing.LastUpdatedAt != "" && p.LastUpdatedAt < existing.LastUpdatedAt {
			return *existing, false, nil
		}
	}

	if p.LastUpdatedAt == "" {
		p.LastUpdatedAt = now()
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO session_progress (session_id,user_id,queue_item_id,position_ms,completed_items,device_id,last_updated_at)
		 VALUES (?,?,?,?,?,?,?)
		 ON CONFLICT(session_id) DO UPDATE SET
		   queue_item_id=excluded.queue_item_id,
		   position_ms=excluded.position_ms,
		   completed_items=excluded.completed_items,
		   device_id=excluded.device_id,
		   last_updated_at=excluded.last_updated_at`,
		p.SessionID, p.UserID, nullIfEmpty(p.QueueItemID), p.PositionMS,
		p.CompletedItems, nullIfEmpty(p.DeviceID), p.LastUpdatedAt); err != nil {
		return models.SessionProgress{}, false, err
	}
	return *p, true, nil
}

// Progress reads the stored resume point, or ErrNotFound if the session has
// never reported one.
func (s *SessionStore) Progress(ctx context.Context, sessionID string) (*models.SessionProgress, error) {
	var p models.SessionProgress
	var itemID, deviceID sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT session_id,user_id,queue_item_id,position_ms,completed_items,device_id,last_updated_at
		 FROM session_progress WHERE session_id=?`, sessionID).
		Scan(&p.SessionID, &p.UserID, &itemID, &p.PositionMS, &p.CompletedItems, &deviceID, &p.LastUpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if itemID.Valid {
		p.QueueItemID = itemID.String
	}
	if deviceID.Valid {
		p.DeviceID = deviceID.String
	}
	return &p, nil
}
