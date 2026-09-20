package store

import (
	"context"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/lib/pq"
)

// EngagementStore manages favorites, playback history, and user confessions.
type EngagementStore struct{ db *db.DB }

func NewEngagementStore(db *db.DB) *EngagementStore { return &EngagementStore{db: db} }

// ---------- Favorites ----------

// AddFavorite records a favourite, or returns the existing one unchanged.
//
// Favouriting is idempotent because the gesture is: a listener who taps the
// heart on a confession they have already favourited means "this is a
// favourite", not "make a second one". The conflict target is the natural key
// (user, entity_type, entity_id) rather than the primary key — the previous
// `ON CONFLICT(id)` could never fire, because the id is freshly generated on
// every call, so three taps wrote three rows (see migration 0013).
//
// On conflict the stored row is returned rather than the one just built, so
// the caller sees the real id and the original created_at. Reporting a
// fabricated id for a row that was never inserted would give clients a handle
// that matches nothing in the table.
func (s *EngagementStore) AddFavorite(ctx context.Context, userID, entityType, entityID string) (*models.Favorite, error) {
	f := &models.Favorite{ID: newID(), UserID: userID, EntityType: entityType, EntityID: entityID, CreatedAt: now()}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO favorites (id,user_id,entity_type,entity_id,created_at) VALUES (?,?,?,?,?)
		 ON CONFLICT (user_id,entity_type,entity_id) DO UPDATE SET entity_id = favorites.entity_id
		 RETURNING id, created_at`,
		f.ID, f.UserID, f.EntityType, f.EntityID, f.CreatedAt).Scan(&f.ID, &f.CreatedAt)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *EngagementStore) RemoveFavorite(ctx context.Context, userID, entityType, entityID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM favorites WHERE user_id=? AND entity_type=? AND entity_id=?`, userID, entityType, entityID)
	return err
}

func (s *EngagementStore) ListFavorites(ctx context.Context, userID, entityType string) ([]models.Favorite, error) {
	q := `SELECT id,user_id,entity_type,entity_id,created_at FROM favorites WHERE user_id=?`
	args := []any{userID}
	if entityType != "" {
		q += ` AND entity_type=?`
		args = append(args, entityType)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Favorite
	for rows.Next() {
		var f models.Favorite
		if err := rows.Scan(&f.ID, &f.UserID, &f.EntityType, &f.EntityID, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListFavoritesDetailed returns favourites with each entity's display name
// resolved, newest first.
//
// The favourites table is polymorphic and stores only (entity_type,
// entity_id), which is the right shape for writing but useless for rendering:
// a library screen built on the bare rows can only print opaque ids. Resolving
// here rather than in the handler keeps it a single round trip per entity kind
// instead of one per favourite, which is what a naive client-side hydration
// would produce.
//
// A favourite whose target no longer exists is returned with Missing set
// rather than dropped. The row is real, the user can see it in their data
// export, and they need a way to clear it.
func (s *EngagementStore) ListFavoritesDetailed(ctx context.Context, userID, entityType string) ([]models.Favorite, error) {
	favs, err := s.ListFavorites(ctx, userID, entityType)
	if err != nil {
		return nil, err
	}
	if len(favs) == 0 {
		return favs, nil
	}

	// Group the ids by kind so each table is queried once.
	byType := map[string][]string{}
	for _, f := range favs {
		byType[f.EntityType] = append(byType[f.EntityType], f.EntityID)
	}

	type label struct{ title, subtitle string }
	titles := map[string]label{}

	// Confessions carry the category they belong to as a subtitle: "Peace"
	// under a confession title is what makes a list of favourites readable.
	for kind, ids := range byType {
		var query string
		switch kind {
		case "confession":
			query = `SELECT f.id, f.title, COALESCE(c.name,'')
			         FROM confessions f LEFT JOIN categories c ON c.id = f.category_id
			         WHERE f.id = ANY(?)`
		case "category":
			query = `SELECT id, name, COALESCE(description,'') FROM categories WHERE id = ANY(?)`
		case "voice":
			query = `SELECT id, name, COALESCE(description,'') FROM voices WHERE id = ANY(?)`
		case "session":
			query = `SELECT id, type, status FROM sessions WHERE id = ANY(?)`
		default:
			continue
		}

		rows, err := s.db.QueryContext(ctx, query, pq.Array(ids))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, title, subtitle string
			if err := rows.Scan(&id, &title, &subtitle); err != nil {
				rows.Close()
				return nil, err
			}
			titles[kind+"\x00"+id] = label{title, subtitle}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	for i := range favs {
		l, ok := titles[favs[i].EntityType+"\x00"+favs[i].EntityID]
		if !ok {
			favs[i].Missing = true
			continue
		}
		favs[i].Title, favs[i].Subtitle = l.title, l.subtitle
	}
	return favs, nil
}

// ---------- Playback history ----------

func (s *EngagementStore) RecordPlayback(ctx context.Context, rec *models.PlaybackRecord) error {
	if rec.ID == "" {
		rec.ID = newID()
	}
	if rec.ListenedAt == "" {
		rec.ListenedAt = now()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO playback_history (id,user_id,session_id,confession_id,duration_seconds,completed,skipped,listened_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		rec.ID, rec.UserID, nullIfEmpty(rec.SessionID), nullIfEmpty(rec.ConfessionID), rec.DurationSeconds, boolInt(rec.Completed), boolInt(rec.Skipped), rec.ListenedAt)
	return err
}

func (s *EngagementStore) History(ctx context.Context, userID string, limit int) ([]models.PlaybackRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,COALESCE(session_id,''),COALESCE(confession_id,''),COALESCE(duration_seconds,0),completed,skipped,listened_at
		 FROM playback_history WHERE user_id=? ORDER BY listened_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.PlaybackRecord
	for rows.Next() {
		var rec models.PlaybackRecord
		if err := rows.Scan(&rec.ID, &rec.UserID, &rec.SessionID, &rec.ConfessionID, &rec.DurationSeconds, &rec.Completed, &rec.Skipped, &rec.ListenedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ---------- User confessions ----------

func (s *EngagementStore) CreateUserConfession(ctx context.Context, uc *models.UserConfession) error {
	if uc.ID == "" {
		uc.ID = newID()
	}
	uc.CreatedAt, uc.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_confessions (id,user_id,title,text,category_id,is_private,status,visibility,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		uc.ID, uc.UserID, uc.Title, uc.Text, nullIfEmpty(uc.CategoryID), boolInt(uc.IsPrivate),
		uc.Status, uc.Visibility, uc.CreatedAt, uc.UpdatedAt)
	return err
}

func (s *EngagementStore) ListUserConfessions(ctx context.Context, userID string) ([]models.UserConfession, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+ucColumns+` FROM user_confessions WHERE user_id=? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.UserConfession
	for rows.Next() {
		uc, err := scanUserConfession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, uc)
	}
	return out, rows.Err()
}
