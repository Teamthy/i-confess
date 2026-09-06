package deletion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Teamthy/i-confess/internal/db"
	"strings"
	"time"

	"github.com/google/uuid"
)

// GracePeriod is how long a deletion request can be cancelled.
//
// It exists because deletion is irreversible and is a favoured move in account
// takeover: an attacker holding a session briefly must not be able to destroy
// an account before its owner can react. Seven days is long enough for someone
// to notice the confirmation email and short enough not to feel like a refusal.
const GracePeriod = 7 * 24 * time.Hour

// Errors callers distinguish.
var (
	ErrNotFound         = errors.New("account not found")
	ErrAlreadyRequested = errors.New("deletion is already scheduled")
	ErrNotRequested     = errors.New("no deletion is scheduled")
	ErrAlreadyDeleted   = errors.New("account is already deleted")
)

// Service performs account deletion.
type Service struct {
	db  *db.DB
	Now func() time.Time
}

// NewService wires a deletion service.
func NewService(db *db.DB) *Service {
	return &Service{db: db, Now: time.Now}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Status describes where an account sits in the deletion lifecycle.
type Status struct {
	Requested   bool   `json:"deletion_requested"`
	RequestedAt string `json:"requested_at,omitempty"`
	// ErasesAt is when the grace period ends.
	ErasesAt string `json:"erases_at,omitempty"`
	Deleted  bool   `json:"deleted"`
}

// Request schedules deletion and immediately revokes every session.
//
// Revoking at request time rather than at erasure is what limits the damage of
// a request the owner did not make: an attacker holding a stolen session is cut
// off at once. The owner, who has the password, can still sign in during the
// grace period to cancel — so the account is locked, not destroyed.
func (s *Service) Request(ctx context.Context, userID, reason string) (*Status, error) {
	var requestedAt, deletedAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT deletion_requested_at, deleted_at FROM users WHERE id = ?`, userID).
		Scan(&requestedAt, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if deletedAt.Valid && deletedAt.String != "" {
		return nil, ErrAlreadyDeleted
	}
	if requestedAt.Valid && requestedAt.String != "" {
		return nil, ErrAlreadyRequested
	}

	now := s.now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET deletion_requested_at = ?, status = 'pending_deletion', updated_at = ?
		 WHERE id = ?`,
		now.Format(time.RFC3339), now.Format(time.RFC3339), userID); err != nil {
		return nil, err
	}
	// Revoke every session now, so the account stops working the moment
	// deletion is requested.
	if _, err := tx.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`,
		now.Format(time.RFC3339), userID); err != nil {
		return nil, err
	}
	// Record the request immediately. If erasure later fails or is delayed,
	// there is still evidence the user asked.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO deletion_records (id,user_ref,requested_at,reason,created_at)
		 VALUES (?,?,?,?,?)`,
		uuid.New().String(), userID, now.Format(time.RFC3339),
		truncate(reason, 500), now.Format(time.RFC3339)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &Status{
		Requested:   true,
		RequestedAt: now.Format(time.RFC3339),
		ErasesAt:    now.Add(GracePeriod).Format(time.RFC3339),
	}, nil
}

// Cancel stops a scheduled deletion and restores the account.
func (s *Service) Cancel(ctx context.Context, userID string) error {
	var requestedAt, deletedAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT deletion_requested_at, deleted_at FROM users WHERE id = ?`, userID).
		Scan(&requestedAt, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if deletedAt.Valid && deletedAt.String != "" {
		// Past erasure there is nothing left to restore, and saying so plainly
		// is better than implying recovery is possible.
		return ErrAlreadyDeleted
	}
	if !requestedAt.Valid || requestedAt.String == "" {
		return ErrNotRequested
	}

	now := s.now().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET deletion_requested_at = NULL, status = 'active', updated_at = ?
		 WHERE id = ?`, now, userID)
	if err != nil {
		return err
	}
	// The deletion record is left in place: the request genuinely happened, and
	// erasing the evidence of a cancelled request would make the log a lie.
	_, err = s.db.ExecContext(ctx,
		`UPDATE deletion_records SET reason = COALESCE(reason,'') || ' [cancelled ' || ? || ']'
		 WHERE user_ref = ? AND erased_at IS NULL`, now, userID)
	return err
}

// StatusFor reports an account's deletion state.
func (s *Service) StatusFor(ctx context.Context, userID string) (*Status, error) {
	var requestedAt, deletedAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT deletion_requested_at, deleted_at FROM users WHERE id = ?`, userID).
		Scan(&requestedAt, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	st := &Status{}
	if deletedAt.Valid && deletedAt.String != "" {
		st.Deleted = true
		return st, nil
	}
	if requestedAt.Valid && requestedAt.String != "" {
		st.Requested = true
		st.RequestedAt = requestedAt.String
		if t, perr := time.Parse(time.RFC3339, requestedAt.String); perr == nil {
			st.ErasesAt = t.Add(GracePeriod).Format(time.RFC3339)
		}
	}
	return st, nil
}

// ErasureReport summarises what an erasure did.
type ErasureReport struct {
	UserRef     string         `json:"user_ref"`
	ErasedAt    string         `json:"erased_at"`
	RowsDeleted int            `json:"rows_deleted"`
	PerTable    map[string]int `json:"per_table"`
	Retained    []string       `json:"retained"`
}

// Erase performs the irreversible deletion.
//
// Everything runs in one transaction: a partial erasure that deletes a profile
// but leaves listening history is worse than a failure, because it looks
// complete while the sensitive data survives.
func (s *Service) Erase(ctx context.Context, userID string) (*ErasureReport, error) {
	var deletedAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT deleted_at FROM users WHERE id = ?`, userID).Scan(&deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if deletedAt.Valid && deletedAt.String != "" {
		return nil, ErrAlreadyDeleted
	}

	now := s.now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	report := &ErasureReport{
		UserRef: userID, ErasedAt: now.Format(time.RFC3339),
		PerTable: map[string]int{}, Retained: RetainedCategories(),
	}

	for _, p := range Policies {
		n, err := applyPolicy(ctx, tx, p, userID)
		if err != nil {
			// A missing table is tolerated so the policy can name tables added
			// in later migrations; anything else aborts the whole erasure.
			if isMissingTable(err) {
				continue
			}
			return nil, fmt.Errorf("apply policy for %s: %w", p.Table, err)
		}
		if n > 0 {
			report.PerTable[p.Table] = n
			report.RowsDeleted += n
		}
	}

	// Tombstone the identity. The row survives so retained records
	// (subscriptions, consents) keep a valid foreign key, but every
	// identifying field is destroyed.
	//
	// The email is replaced with a unique non-address rather than blanked,
	// because the column is UNIQUE and NOT NULL: two deleted accounts must not
	// collide, and the value must not resemble a real address someone could
	// later register.
	tombstone := fmt.Sprintf("deleted-%s@invalid", shortRef(userID))
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET
		   email = ?, password_hash = '', display_name = NULL,
		   timezone = 'UTC', status = 'deleted', email_verified = 0, mfa_enabled = 0,
		   last_login_at = NULL, deleted_at = ?, anonymised_at = ?, updated_at = ?
		 WHERE id = ?`,
		tombstone, now.Format(time.RFC3339), now.Format(time.RFC3339),
		now.Format(time.RFC3339), userID); err != nil {
		return nil, err
	}

	tables := make([]string, 0, len(report.PerTable))
	for t := range report.PerTable {
		tables = append(tables, t)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE deletion_records SET erased_at = ?, rows_deleted = ?, tables_affected = ?,
		        retained_categories = ?
		 WHERE user_ref = ? AND erased_at IS NULL`,
		now.Format(time.RFC3339), report.RowsDeleted, strings.Join(tables, ","),
		strings.Join(report.Retained, " | "), userID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return report, nil
}

// applyPolicy executes one table's rule.
func applyPolicy(ctx context.Context, tx *db.Tx, p TablePolicy, userID string) (int, error) {
	switch p.Action {
	case Retain:
		return 0, nil

	case Anonymise:
		// Table and column names come from the compile-time policy, never from
		// input; the user id is always a bound parameter.
		res, err := tx.ExecContext(ctx,
			fmt.Sprintf(`UPDATE %s SET %s = NULL WHERE %s = ?`, p.Table, p.column(), p.column()),
			userID)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return int(n), nil

	default:
		var q string
		if p.column() == "user_id" {
			q = fmt.Sprintf(`DELETE FROM %s WHERE user_id = ?`, p.Table)
		} else {
			// Child tables reference their parent, so scope through it.
			parent := parentTableFor(p.Table)
			q = fmt.Sprintf(`DELETE FROM %s WHERE %s IN (SELECT id FROM %s WHERE user_id = ?)`,
				p.Table, p.column(), parent)
		}
		res, err := tx.ExecContext(ctx, q, userID)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return int(n), nil
	}
}

// parentTableFor maps a child table to the table its foreign key points at.
func parentTableFor(child string) string {
	switch child {
	case "user_collection_items":
		return "user_collections"
	case "session_items":
		return "sessions"
	}
	return child
}

func isMissingTable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "does not exist")
}

// DueForErasure lists accounts whose grace period has elapsed.
func (s *Service) DueForErasure(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}
	cutoff := s.now().Add(-GracePeriod).Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM users
		 WHERE deletion_requested_at IS NOT NULL AND deletion_requested_at <= ?
		   AND deleted_at IS NULL
		 ORDER BY deletion_requested_at LIMIT ?`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// shortRef derives a stable, non-identifying suffix for the tombstone address.
func shortRef(id string) string {
	clean := strings.ReplaceAll(id, "-", "")
	if len(clean) > 16 {
		clean = clean[:16]
	}
	if clean == "" {
		return uuid.New().String()[:16]
	}
	return clean
}
