package store

import (
	"context"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
)

// PublicInboxStore holds what the website's public forms collect: newsletter
// sign-ups and contact messages. Neither needs an account.
type PublicInboxStore struct{ db *db.DB }

// NewPublicInboxStore returns a store over the given database.
func NewPublicInboxStore(d *db.DB) *PublicInboxStore { return &PublicInboxStore{db: d} }

// Subscribe records a newsletter address, idempotently, and stores the hash
// of a fresh unsubscribe token. Re-subscribing an address that had
// unsubscribed clears the unsubscribe and rotates the token.
func (s *PublicInboxStore) Subscribe(ctx context.Context, email, source, unsubscribeTokenHash string, at time.Time) error {
	now := at.UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO newsletter_subscriptions (id,email,source,unsubscribe_token_hash,created_at,updated_at)
		 VALUES (?,?,?,?,?,?)
		 ON CONFLICT (email) DO UPDATE SET unsubscribed_at=NULL, deleted_at=NULL,
		   unsubscribe_token_hash=excluded.unsubscribe_token_hash, updated_at=excluded.updated_at,
		   row_version=newsletter_subscriptions.row_version+1`,
		newID(), strings.ToLower(strings.TrimSpace(email)), source, unsubscribeTokenHash, now, now)
	return err
}

// Unsubscribe marks the subscription holding this token hash as unsubscribed.
// It reports false when no subscription holds the hash. Repeating it is
// harmless: the first unsubscribe time is kept.
func (s *PublicInboxStore) Unsubscribe(ctx context.Context, tokenHash string, at time.Time) (bool, error) {
	if tokenHash == "" {
		return false, nil
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE newsletter_subscriptions
		 SET unsubscribed_at=COALESCE(unsubscribed_at, ?), updated_at=?, row_version=row_version+1
		 WHERE unsubscribe_token_hash=? AND deleted_at IS NULL`,
		at.UTC().Format(time.RFC3339), at.UTC().Format(time.RFC3339), tokenHash)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// SaveContact stores a contact message and returns its id.
func (s *PublicInboxStore) SaveContact(ctx context.Context, name, email, message string, at time.Time) (string, error) {
	id := newID()
	now := at.UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO contact_messages (id,name,email,message,created_at,updated_at) VALUES (?,?,?,?,?,?)`,
		id, strings.TrimSpace(name), strings.TrimSpace(email), strings.TrimSpace(message), now, now)
	return id, err
}
