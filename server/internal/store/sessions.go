package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
	// Resolve the content snapshot once per distinct confession rather than
	// once per item. The engine already sets Title and Text on the item, but the
	// category it sets is an id where the queue returns a name, and a caller is
	// free to leave the fields empty. Resolving here makes the snapshot correct
	// regardless of who built the queue.
	snapshots, err := s.contentSnapshots(ctx, tx, sess.Items)
	if err != nil {
		return err
	}

	for i := range sess.Items {
		it := &sess.Items[i]
		if it.ID == "" {
			it.ID = newID()
		}
		it.SessionID = sess.ID
		// Callers hand in a mix of spellings, including the lowercase ones this
		// column was originally constrained to. Folding them here means a
		// caller cannot fail the CHECK on a status the state machine already
		// understands - and one it does not understand is rejected rather than
		// quietly stored as something else.
		if canonical := sessions.NormalizeItemStatus(it.Status); canonical != "" {
			it.Status = canonical
		} else if it.Status == "" {
			it.Status = string(sessions.ItemQueued)
		} else {
			return fmt.Errorf("session item %s has unrecognised status %q", it.ID, it.Status)
		}
		snap := snapshots[it.ConfessionID]
		_, err = tx.ExecContext(ctx,
			`INSERT INTO session_items (id,session_id,confession_id,variant_id,voice_id,audio_asset_id,position,duration_seconds,status,title,category_name,text)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			it.ID, it.SessionID, it.ConfessionID, nullIfEmpty(it.VariantID), nullIfEmpty(it.VoiceID), nullIfEmpty(it.AudioAssetID), it.Position, it.DurationSeconds, it.Status,
			nullIfEmpty(snap.title), nullIfEmpty(snap.categoryName), nullIfEmpty(snap.text))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SessionStore) ByID(ctx context.Context, id string) (*models.Session, error) {
	var sess models.Session
	var voiceID, startedAt, completedAt, deletedAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,type,duration_seconds,COALESCE(strategy,''),COALESCE(target_duration,0),COALESCE(actual_duration,0),
		        COALESCE(title,''),COALESCE(description,''),voice_id,status,created_at,started_at,completed_at,deleted_at
		 FROM sessions WHERE id = ? AND (deleted_at IS NULL OR deleted_at = '')`, id).
		Scan(&sess.ID, &sess.UserID, &sess.Type, &sess.DurationSeconds, &sess.Strategy,
			&sess.TargetDuration, &sess.ActualDuration, &sess.Title, &sess.Description,
			&voiceID, &sess.Status, &sess.CreatedAt, &startedAt, &completedAt, &deletedAt)
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
	if deletedAt.Valid {
		sess.DeletedAt = deletedAt.String
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
		        COALESCE(si.title, c.title), COALESCE(si.category_name, cat.name), COALESCE(a.cdn_path, a.storage_key, ''),
		        COALESCE(si.text, c.medium_text, c.short_text, ''), COALESCE(a.status, '')
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
		if err := rows.Scan(&it.ID, &it.ConfessionID, &it.VariantID, &it.VoiceID, &it.AudioAssetID, &it.Position, &it.DurationSeconds, &it.Status, &it.Title, &it.Category, &it.AudioURL, &it.Text, &it.AssetStatus); err != nil {
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
		 FROM sessions
		 WHERE user_id = ? AND (deleted_at IS NULL OR deleted_at = '')
		 ORDER BY created_at DESC LIMIT ?`, userID, limit)
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
	// Same folding as CreateSession: the column is constrained to the canonical
	// vocabulary, and a caller should not have to know which spelling of a state
	// the state machine settled on to write it.
	canonical := sessions.NormalizeItemStatus(status)
	if canonical == "" {
		return fmt.Errorf("unrecognised session item status %q", status)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE session_items SET status=? WHERE id=?`, canonical, itemID)
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

// contentSnapshot is the frozen view of a confession taken when a session is
// built.
type contentSnapshot struct {
	title        string
	categoryName string
	text         string
}

// contentSnapshots reads title, category name and body text for the distinct
// confessions in a queue.
//
// It runs inside the caller's transaction so the snapshot and the queue are
// written atomically: a session can never be persisted with items whose
// snapshot was taken against a different state of the content.
func (s *SessionStore) contentSnapshots(ctx context.Context, tx *db.Tx, items []models.SessionItem) (map[string]contentSnapshot, error) {
	out := make(map[string]contentSnapshot, len(items))
	for _, it := range items {
		if _, seen := out[it.ConfessionID]; seen {
			continue
		}
		var snap contentSnapshot
		err := tx.QueryRowContext(ctx,
			`SELECT c.title, k.name, COALESCE(c.medium_text, c.short_text, '')
			   FROM confessions c
			   JOIN categories k ON k.id = c.category_id
			  WHERE c.id = ?`, it.ConfessionID).
			Scan(&snap.title, &snap.categoryName, &snap.text)
		if err != nil {
			// A queue referencing a confession that does not exist is a caller
			// bug. Failing here is better than snapshotting an empty title that
			// renders as a blank card at playback time.
			return nil, fmt.Errorf("snapshot content %s: %w", it.ConfessionID, err)
		}
		out[it.ConfessionID] = snap
	}
	return out, nil
}

// SoftDelete removes a session from the listener's history without destroying
// the record of what they listened to. Streaks and completion metrics are
// derived from these rows, so a deletion a listener can perform must not edit
// the numbers.
//
// It reports false when the session was already gone, so the caller can answer
// 404 rather than confirming a deletion that did not happen.
func (s *SessionStore) SoftDelete(ctx context.Context, id, at string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET deleted_at = ?
		 WHERE id = ? AND (deleted_at IS NULL OR deleted_at = '')`, at, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ListByUserPage returns one page of a listener's history.
//
// This is keyset pagination on (created_at, id) rather than OFFSET. An offset
// scan gets slower the deeper the listener pages, and a session created while
// they were reading shifts every later row, so page 2 can repeat a session
// already seen on page 1. The id tiebreaker is what makes the order total:
// created_at carries second precision and ties constantly.
//
// An empty cursorCreatedAt returns the first page.
func (s *SessionStore) ListByUserPage(ctx context.Context, userID string, limit int,
	cursorCreatedAt, cursorID string) ([]models.Session, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := `SELECT id,user_id,type,duration_seconds,COALESCE(strategy,''),COALESCE(target_duration,0),COALESCE(actual_duration,0),
	        COALESCE(title,''),COALESCE(description,''),COALESCE(voice_id,''),status,created_at,
	        COALESCE(started_at,''),COALESCE(completed_at,'')
	     FROM sessions
	     WHERE user_id = ? AND (deleted_at IS NULL OR deleted_at = '')`
	args := []any{userID}
	if cursorCreatedAt != "" {
		// Strictly older than the cursor, with id breaking ties in the same
		// second. Both halves matter: comparing on created_at alone would skip
		// or repeat every session created in the cursor's second.
		q += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, cursorCreatedAt, cursorCreatedAt, cursorID)
	}
	q += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
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
