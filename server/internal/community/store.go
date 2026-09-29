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

// Feed is the anonymous public reader. Shared posts are deliberately excluded:
// a public endpoint cannot enforce an author's circle membership.
func (s *Store) Feed(ctx context.Context, limit int) ([]FeedPost, error) {
	return s.FeedFor(ctx, limit, "")
}

// FeedFor applies a listener's private block boundaries without ever including
// author identifiers in the public feed projection. Anonymous readers pass an
// empty viewer ID and receive the unfiltered public feed.
func (s *Store) FeedFor(ctx context.Context, limit int, viewerID string) ([]FeedPost, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, body, visibility, status, created_at
		 FROM community_posts p
		 WHERE visibility = ? AND status IN (?, ?)
		   AND NOT EXISTS (
		     SELECT 1 FROM user_blocks b
		     WHERE b.blocker_id = ? AND b.blocked_id = p.author_id
		   )
		 ORDER BY created_at DESC LIMIT ?`,
		VisibilityPublic, StatusApproved, StatusPublished, viewerID, limit)
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

// PostAuthor resolves a feed-visible post to its owner for server-side actions.
// The author ID stays inside the service boundary: public feed responses remain
// anonymous, while moderation/block workflows can act on the correct account.
func (s *Store) PostAuthor(ctx context.Context, postID string) (string, error) {
	var authorID string
	err := s.db.QueryRowContext(ctx,
		`SELECT author_id FROM community_posts
		 WHERE id = ? AND visibility = ? AND status IN (?, ?)`,
		postID, VisibilityPublic, StatusApproved, StatusPublished).Scan(&authorID)
	return authorID, err
}

func (s *Store) React(ctx context.Context, postID, userID string, r Reaction) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO community_reactions (id, post_id, user_id, reaction, created_at)
		 VALUES (?,?,?,?,?)
		 ON CONFLICT (post_id, user_id, reaction) DO NOTHING`,
		uuid.NewString(), postID, userID, string(r), time.Now().UTC().Format(time.RFC3339))
	return err
}
