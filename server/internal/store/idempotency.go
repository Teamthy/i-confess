package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Teamthy/i-confess/internal/db"
	"time"
)

// IdempotencyRecord is a stored response for a previously executed mutation.
type IdempotencyRecord struct {
	Key     string
	UserID  string
	Method  string
	Path    string
	Status  int
	Body    string
	Expires time.Time
}

// IdempotencyStore persists the responses that make retried mutations safe
// (§47). It is deliberately backed by the same durable database as everything
// else: a retry that lands on a different API instance must still be caught,
// which rules out in-process memory.
type IdempotencyStore struct{ db *db.DB }

func NewIdempotencyStore(db *db.DB) *IdempotencyStore { return &IdempotencyStore{db: db} }

// Lookup returns a live record for the key, if one exists.
//
// The method and path must match: reusing one key across two different
// endpoints is a client bug, and replaying the first endpoint's response to
// the second would be silently wrong. A mismatch is reported as a miss so the
// caller can execute normally rather than serve an unrelated response.
func (s *IdempotencyStore) Lookup(ctx context.Context, key, method, path string) (IdempotencyRecord, bool) {
	var rec IdempotencyRecord
	var expires string
	err := s.db.QueryRowContext(ctx,
		`SELECT key, user_id, method, path, COALESCE(response_status,0), COALESCE(response_body,''), expires_at
		 FROM idempotency_keys WHERE key = ?`, key).
		Scan(&rec.Key, &rec.UserID, &rec.Method, &rec.Path, &rec.Status, &rec.Body, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return IdempotencyRecord{}, false
	}
	if err != nil {
		return IdempotencyRecord{}, false
	}
	if rec.Method != method || rec.Path != path {
		return IdempotencyRecord{}, false
	}
	if t, err := time.Parse(time.RFC3339, expires); err == nil && time.Now().UTC().After(t) {
		// Expired: treat as absent and let the row be replaced.
		return IdempotencyRecord{}, false
	}
	return rec, true
}

// Store records a response. An existing key is overwritten, so a client that
// reuses a key after expiry gets fresh behaviour rather than an error.
//
// Errors are swallowed by design: failing to remember a response must not fail
// the request that just succeeded. The cost is a possible duplicate side
// effect on the next retry, which is the same outcome as not having the
// mechanism at all.
func (s *IdempotencyStore) Store(ctx context.Context, key, userID, method, path string, status int, body string, expires time.Time) {
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO idempotency_keys (key,user_id,method,path,response_status,response_body,created_at,expires_at)
		 VALUES (?,?,?,?,?,?,?,?)
		 ON CONFLICT(key) DO UPDATE SET
		   response_status=excluded.response_status,
		   response_body=excluded.response_body,
		   expires_at=excluded.expires_at`,
		key, userID, method, path, status, body, now(), expires.UTC().Format(time.RFC3339))
}

// PurgeExpired removes lapsed keys. Called from the periodic cleanup job so the
// table does not grow without bound.
func (s *IdempotencyStore) PurgeExpired(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM idempotency_keys WHERE expires_at < ?`,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
