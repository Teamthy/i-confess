// Package retention provides the uniform row-retention contract introduced by
// PHASE 38. Every application table has a nullable deleted_at tombstone and a
// row_version counter; callers must use this package for reversible deletion
// rather than physically removing a durable row.
package retention

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
)

var (
	// ErrNotFound means the row is absent or has already been deleted.
	ErrNotFound = errors.New("retention row not found")
	identifier  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// Store applies soft-delete and restore operations. The table name is checked
// as an identifier before it is interpolated; values remain query parameters.
type Store struct {
	db *db.DB
}

func NewStore(database *db.DB) *Store { return &Store{db: database} }

func validTable(table string) bool { return identifier.MatchString(table) }

// SoftDelete marks a row deleted and advances its optimistic row version. A
// second delete is a no-op with ErrNotFound, which makes retries safe and keeps
// a tombstone's first deletion timestamp authoritative.
func (s *Store) SoftDelete(ctx context.Context, table, id string) error {
	if !validTable(table) {
		return fmt.Errorf("invalid retention table %q", table)
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE `+table+` SET deleted_at = ?, row_version = row_version + 1
		 WHERE id = ? AND deleted_at IS NULL`,
		time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("soft delete %s: %w", table, err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("soft delete %s rows affected: %w", table, err)
	} else if n != 1 {
		return ErrNotFound
	}
	return nil
}

// Restore reverses a soft delete and advances the row version again. Restore
// is intentionally explicit; normal reads never expose a tombstoned row.
func (s *Store) Restore(ctx context.Context, table, id string) error {
	if !validTable(table) {
		return fmt.Errorf("invalid retention table %q", table)
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE `+table+` SET deleted_at = NULL, row_version = row_version + 1
		 WHERE id = ? AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return fmt.Errorf("restore %s: %w", table, err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("restore %s rows affected: %w", table, err)
	} else if n != 1 {
		return ErrNotFound
	}
	return nil
}
