package community

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Store struct{ db *sql.DB }
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Create(ctx context.Context, authorID, body, visibility string) (*Post, error) {
	p := &Post{ID: uuid.NewString(), AuthorID: authorID, Body: body, Visibility: visibility, Status: StatusSubmitted, CreatedAt: time.Now().UTC()}
	_, err := s.db.ExecContext(ctx, `INSERT INTO community_posts (id, author_id, body, visibility, status, created_at) VALUES (?,?,?,?,?,?)`,
		p.ID, p.AuthorID, p.Body, p.Visibility, p.Status, p.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Store) Feed(ctx context.Context, limit int) ([]Post, error) {
	if limit <= 0 || limit > 50 { limit = 20 }
	rows, err := s.db.QueryContext(ctx, `SELECT id, author_id, body, visibility, status, created_at FROM community_posts WHERE visibility=? AND status IN (?,?) ORDER BY created_at DESC LIMIT ?`, VisibilityShared, StatusApproved, StatusPublished, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []Post
	for rows.Next() {
		var p Post
		var ts string
		if err := rows.Scan(&p.ID, &p.AuthorID, &p.Body, &p.Visibility, &p.Status, &ts); err != nil { continue }
		p.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) React(ctx context.Context, postID, userID string, r Reaction) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO community_reactions (id, post_id, user_id, reaction, created_at) VALUES (?,?,?,?,?)`,
		uuid.NewString(), postID, userID, string(r), time.Now().UTC().Format(time.RFC3339))
	return err
}
