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

// Subscribe records a newsletter address, idempotently. Re-subscribing an
// address that had unsubscribed clears the unsubscribe.
func (s *PublicInboxStore) Subscribe(ctx context.Context, email, source string, at time.Time) error {
	now := at.UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO newsletter_subscriptions (id,email,source,created_at,updated_at)
		 VALUES (?,?,?,?,?)
		 ON CONFLICT (email) DO UPDATE SET unsubscribed_at=NULL, deleted_at=NULL, updated_at=excluded.updated_at,
		   row_version=newsletter_subscriptions.row_version+1`,
		newID(), strings.ToLower(strings.TrimSpace(email)), source, now, now)
	return err
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
