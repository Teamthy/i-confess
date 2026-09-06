package store

import (
	"context"
	"github.com/Teamthy/i-confess/internal/db"

	"github.com/Teamthy/i-confess/internal/models"
)

// EngagementStore manages favorites, playback history, and user confessions.
type EngagementStore struct{ db *db.DB }

func NewEngagementStore(db *db.DB) *EngagementStore { return &EngagementStore{db: db} }

// ---------- Favorites ----------

func (s *EngagementStore) AddFavorite(ctx context.Context, userID, entityType, entityID string) (*models.Favorite, error) {
	f := &models.Favorite{ID: newID(), UserID: userID, EntityType: entityType, EntityID: entityID, CreatedAt: now()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO favorites (id,user_id,entity_type,entity_id,created_at) VALUES (?,?,?,?,?)
		 ON CONFLICT(id) DO NOTHING`, f.ID, f.UserID, f.EntityType, f.EntityID, f.CreatedAt)
	return f, err
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
		`INSERT INTO user_confessions (id,user_id,title,text,category_id,is_private,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		uc.ID, uc.UserID, uc.Title, uc.Text, nullIfEmpty(uc.CategoryID), boolInt(uc.IsPrivate), uc.CreatedAt, uc.UpdatedAt)
	return err
}

func (s *EngagementStore) ListUserConfessions(ctx context.Context, userID string) ([]models.UserConfession, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,title,text,COALESCE(category_id,''),is_private,created_at,updated_at
		 FROM user_confessions WHERE user_id=? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.UserConfession
	for rows.Next() {
		var uc models.UserConfession
		if err := rows.Scan(&uc.ID, &uc.UserID, &uc.Title, &uc.Text, &uc.CategoryID, &uc.IsPrivate, &uc.CreatedAt, &uc.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, uc)
	}
	return out, rows.Err()
}
