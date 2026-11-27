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

// Subscribe records a new newsletter address with the hash of its
// unsubscribe token, and reports whether the address was new.
//
// An address already on file is left exactly as it is. The form is public
// and anonymous, so it cannot prove it is the address's owner asking: it must
// not undo an unsubscribe (that would put someone back on the list against
// their wishes) and must not rotate the token (that would break the
// unsubscribe link in every mail already sent). Coming back after leaving
// needs a confirmed opt-in, which is not built yet.
func (s *PublicInboxStore) Subscribe(ctx context.Context, email, source, unsubscribeTokenHash string, at time.Time) (created bool, err error) {
	now := at.UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO newsletter_subscriptions (id,email,source,unsubscribe_token_hash,created_at,updated_at)
		 VALUES (?,?,?,?,?,?)
		 ON CONFLICT (email) DO NOTHING`,
		newID(), strings.ToLower(strings.TrimSpace(email)), source, unsubscribeTokenHash, now, now)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
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
