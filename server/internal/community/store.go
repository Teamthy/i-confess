package community

import (
	"context"
	"time"

	"github.com/Teamthy/i-confess/internal/db"

	"github.com/google/uuid"
)

type Store struct{ db *db.DB }

func NewStore(db *db.DB) *Store { return &Store{db: db} }

func (s *Store) Create(ctx context.Context, authorID, body, visibility string) (*Post, error) {
	if visibility == "" {
		visibility = VisibilityPrivate
	}
	p := &Post{
		ID:         uuid.NewString(),
		AuthorID:   authorID,
		Body:       body,
		Visibility: visibility,
		Status:     StatusSubmitted,
		CreatedAt:  time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO community_posts (id, author_id, body, visibility, status, created_at)
		 VALUES (?,?,?,?,?,?)`,
		p.ID, p.AuthorID, p.Body, p.Visibility, p.Status, p.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Feed returns approved public/shared posts with AuthorID stripped for anonymity (IC-006).
func (s *Store) Feed(ctx context.Context, limit int) ([]FeedPost, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, body, visibility, status, created_at
		 FROM community_posts
		 WHERE visibility IN (?, ?) AND status IN (?, ?)
		 ORDER BY created_at DESC LIMIT ?`,
		VisibilityShared, VisibilityPublic, StatusApproved, StatusPublished, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]FeedPost, 0)
	for rows.Next() {
		var p FeedPost
		var ts string
		if err := rows.Scan(&p.ID, &p.Body, &p.Visibility, &p.Status, &ts); err != nil {
			continue
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) React(ctx context.Context, postID, userID string, r Reaction) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO community_reactions (id, post_id, user_id, reaction, created_at)
		 VALUES (?,?,?,?,?)
		 ON CONFLICT (post_id, user_id, reaction) DO NOTHING`,
		uuid.NewString(), postID, userID, string(r), time.Now().UTC().Format(time.RFC3339))
	return err
}
