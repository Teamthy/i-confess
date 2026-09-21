package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/trial"
	"github.com/google/uuid"
)

var (
	// ErrTrialNotEligible is returned when an account has already consumed or
	// converted its one trial, or already has a paid entitlement.
	ErrTrialNotEligible = errors.New("user is not eligible for a trial")
	// ErrTrialTransition is returned when a caller asks for a movement that is
	// not in the explicit lifecycle edge table.
	ErrTrialTransition = errors.New("invalid trial transition")
)

// Trial is the persisted account journey. State is deliberately not inferred
// from subscription.status: a trial can be converted while the old receipt row
// remains for audit, and an account can be eligible without a subscription
// clock having started.
type Trial struct {
	ID          string      `json:"id"`
	UserID      string      `json:"user_id"`
	State       trial.State `json:"state"`
	StartedAt   string      `json:"started_at,omitempty"`
	ExpiresAt   string      `json:"expires_at,omitempty"`
	ConvertedAt string      `json:"converted_at,omitempty"`
	CreatedAt   string      `json:"created_at"`
	UpdatedAt   string      `json:"updated_at"`
}

// TrialStore owns the trial row and the single projection writer that mirrors a
// running trial into subscriptions. No handler writes plan=premium/status=trial
// directly; this is the one place the trial can grant and revoke its projection.
type TrialStore struct{ db *db.DB }

func NewTrialStore(database *db.DB) *TrialStore { return &TrialStore{db: database} }

// Current returns the row, creating an ELIGIBLE row for an existing user. An
// absent row is not a reason to treat a user as active; it is the initial state.
func (s *TrialStore) Current(ctx context.Context, userID string) (*Trial, error) {
	if userID == "" {
		return nil, ErrNotFound
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO trials (id,user_id,state,created_at,updated_at)
		 VALUES (?,?,?,?,?) ON CONFLICT (user_id) DO NOTHING`,
		uuid.NewString(), userID, string(trial.Eligible), now(), now())
	if err != nil {
		return nil, err
	}
	return s.read(ctx, userID)
}

// Start atomically starts the one trial and writes the premium/trial
// subscription projection. The two lifecycle edges ELIGIBLE→STARTED→ACTIVE are
// applied in one transaction so a client never observes an entitled trial
// without an ACTIVE trial row, while the explicit state-machine edges remain
// testable independently.
func (s *TrialStore) Start(ctx context.Context, userID string, at time.Time) (*Trial, error) {
	if userID == "" {
		return nil, ErrNotFound
	}
	at = at.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO trials (id,user_id,state,created_at,updated_at)
		 VALUES (?,?,?,?,?) ON CONFLICT (user_id) DO NOTHING`,
		uuid.NewString(), userID, string(trial.Eligible), at.Format(time.RFC3339), at.Format(time.RFC3339)); err != nil {
		return nil, err
	}

	row, err := scanTrial(tx.QueryRowContext(ctx,
		`SELECT id,user_id,state,COALESCE(started_at,''),COALESCE(expires_at,''),
		        COALESCE(converted_at,''),created_at,updated_at
		 FROM trials WHERE user_id = ? FOR UPDATE`, userID))
	if err != nil {
		return nil, err
	}

	switch row.State {
	case trial.Active, trial.Expiring:
		// POST is idempotent: retrying after a lost response returns the
		// existing running trial and does not extend its expiry.
		return row, tx.Commit()
	case trial.Eligible:
		// Continue below.
	case trial.Started:
		// A crash cannot leave STARTED because the projection and the state
		// change share this transaction, but treating it as resumable makes
		// recovery safe for rows created by an older deploy.
	default:
		return nil, fmt.Errorf("%w: current state is %s", ErrTrialNotEligible, row.State)
	}

	expires := at.Add(trial.TrialDuration)
	started := at.Format(time.RFC3339)
	expiresText := expires.Format(time.RFC3339)
	if err := trial.Transition(row.State, trial.Started); err != nil && row.State != trial.Started {
		return nil, fmt.Errorf("%w: %v", ErrTrialTransition, err)
	}
	if row.State == trial.Eligible {
		if _, err := tx.ExecContext(ctx,
			`UPDATE trials SET state=?,started_at=?,expires_at=?,updated_at=? WHERE user_id=?`,
			string(trial.Started), started, expiresText, at.Format(time.RFC3339), userID); err != nil {
			return nil, err
		}
		row.State = trial.Started
	}
	if err := trial.Transition(row.State, trial.Active); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTrialTransition, err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE trials SET state=?,updated_at=? WHERE user_id=?`,
		string(trial.Active), at.Format(time.RFC3339), userID); err != nil {
		return nil, err
	}

	// This is the only trial entitlement projection writer. It refuses to
	// replace a paid/admin premium row; a trial is not a way around billing.
	res, err := tx.ExecContext(ctx,
		`UPDATE subscriptions SET plan='premium',status='trial',started_at=?,ends_at=?,updated_at=?
		 WHERE user_id=? AND (plan='free' OR status IN ('expired','cancelled','refunded','suspended'))`,
		started, expiresText, at.Format(time.RFC3339), userID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var plan, status string
		err = tx.QueryRowContext(ctx,
			`SELECT plan,status FROM subscriptions WHERE user_id=? FOR UPDATE`, userID).Scan(&plan, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if plan != "premium" || status != "trial" {
			return nil, fmt.Errorf("%w: subscription is %s/%s", ErrTrialNotEligible, plan, status)
		}
	}

	row.State = trial.Active
	row.StartedAt = started
	row.ExpiresAt = expiresText
	row.UpdatedAt = at.Format(time.RFC3339)
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return row, nil
}

// Refresh applies time-based edges and keeps the subscription projection safe.
// It walks ACTIVE→EXPIRING→EXPIRED rather than skipping the named edge when a
// request arrives after expiry. Conversion is never inferred from the clock.
func (s *TrialStore) Refresh(ctx context.Context, userID string, at time.Time) (*Trial, error) {
	at = at.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row, err := scanTrial(tx.QueryRowContext(ctx,
		`SELECT id,user_id,state,COALESCE(started_at,''),COALESCE(expires_at,''),
		        COALESCE(converted_at,''),created_at,updated_at
		 FROM trials WHERE user_id=? FOR UPDATE`, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	startedAt, _ := parseTime(row.StartedAt)
	expiresAt, _ := parseTime(row.ExpiresAt)
	desired := trial.StateAt(row.State, startedAt, expiresAt, at)
	if desired == row.State {
		return row, tx.Commit()
	}

	if row.State == trial.Active && desired == trial.Expired {
		if err := s.transitionTx(ctx, tx, row, trial.Expiring, at); err != nil {
			return nil, err
		}
		row.State = trial.Expiring
	}
	if err := trial.Transition(row.State, desired); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTrialTransition, err)
	}
	if err := s.transitionTx(ctx, tx, row, desired, at); err != nil {
		return nil, err
	}
	row.State = desired
	row.UpdatedAt = at.Format(time.RFC3339)
	if desired == trial.Expired || desired == trial.Converted {
		// Do not overwrite a paid subscription that arrived after the trial.
		if _, err := tx.ExecContext(ctx,
			`UPDATE subscriptions SET plan='free',status='expired',updated_at=?
			 WHERE user_id=? AND plan='premium' AND status='trial'`,
			at.Format(time.RFC3339), userID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return row, nil
}

// Convert records the terminal trial edge. It never grants paid premium: a
// store receipt must still pass through billing verification and its sole
// verified-subscription writer.
func (s *TrialStore) Convert(ctx context.Context, userID string, at time.Time) (*Trial, error) {
	at = at.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	row, err := scanTrial(tx.QueryRowContext(ctx,
		`SELECT id,user_id,state,COALESCE(started_at,''),COALESCE(expires_at,''),
		        COALESCE(converted_at,''),created_at,updated_at
		 FROM trials WHERE user_id=? FOR UPDATE`, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.State == trial.Converted {
		return row, tx.Commit()
	}
	if row.State != trial.Active && row.State != trial.Expiring {
		return nil, fmt.Errorf("%w: current state is %s", ErrTrialTransition, row.State)
	}
	if err := trial.Transition(row.State, trial.Converted); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTrialTransition, err)
	}
	if err := s.transitionTx(ctx, tx, row, trial.Converted, at); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE subscriptions SET plan='free',status='expired',updated_at=?
		 WHERE user_id=? AND plan='premium' AND status='trial'`,
		at.Format(time.RFC3339), userID); err != nil {
		return nil, err
	}
	row.State = trial.Converted
	row.ConvertedAt = at.Format(time.RFC3339)
	row.UpdatedAt = at.Format(time.RFC3339)
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *TrialStore) transitionTx(ctx context.Context, tx *db.Tx, row *Trial, to trial.State, at time.Time) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE trials SET state=?,converted_at=CASE WHEN ?='CONVERTED' THEN ? ELSE converted_at END,updated_at=? WHERE user_id=?`,
		string(to), string(to), at.Format(time.RFC3339), at.Format(time.RFC3339), row.UserID); err != nil {
		return err
	}
	return nil
}

func (s *TrialStore) read(ctx context.Context, userID string) (*Trial, error) {
	row, err := scanTrial(s.db.QueryRowContext(ctx,
		`SELECT id,user_id,state,COALESCE(started_at,''),COALESCE(expires_at,''),
		        COALESCE(converted_at,''),created_at,updated_at FROM trials WHERE user_id=?`, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return row, err
}

func scanTrial(row interface{ Scan(...any) error }) (*Trial, error) {
	var out Trial
	var state string
	err := row.Scan(&out.ID, &out.UserID, &state, &out.StartedAt, &out.ExpiresAt,
		&out.ConvertedAt, &out.CreatedAt, &out.UpdatedAt)
	out.State = trial.State(state)
	return &out, err
}

func parseTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}
