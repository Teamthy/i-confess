package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Teamthy/i-confess/internal/billing"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/models"
)

// TrialsStore owns the §36 trial record: the one-per-account row, the CAS
// writes that move it along the graph, and the clock sweeps that make the
// passive states (started→active→expiring→expired) real without a cron.
//
// Every transition is a compare-and-set — `UPDATE ... WHERE status = from`.
// Two callers racing on the same trial cannot both win, and a mover that
// finds the row already elsewhere gets a typed error naming the state it
// found. That is the whole enforcement story: the graph lives in
// billing.TrialTransitions, this file refuses everything off it, and the
// CHECK constraint refuses everything outside the vocabulary.

type TrialsStore struct {
	db *db.DB
}

func NewTrialsStore(db *db.DB) *TrialsStore { return &TrialsStore{db: db} }

const trialColumns = `id, user_id, status, COALESCE(started_at,''), COALESCE(ends_at,''),
	COALESCE(converted_at,''), created_at, updated_at`

func scanTrial(row interface{ Scan(...any) error }) (models.Trial, error) {
	var t models.Trial
	err := row.Scan(&t.ID, &t.UserID, &t.Status, &t.StartedAt, &t.EndsAt,
		&t.ConvertedAt, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

// ErrNoTrial is the absence of a row — distinct from an error, because for a
// read path "no trial yet" is an answer, and the lifecycle GET turns it into
// a lazily created eligible row.
var ErrNoTrial = errors.New("no trial record")

// Current returns the caller's trial, creating the eligible row on first read.
// Reading is also where the clock is caught up (see advance): the lifecycle
// has no cron, so a state the time has already passed through is corrected on
// access and persisted before it is returned. A user who returns after three
// days sees EXPIRED, not a stale ACTIVE with two days left.
func (s *TrialsStore) Current(ctx context.Context, userID string, now time.Time) (models.Trial, error) {
	t, err := s.byUser(ctx, userID)
	if errors.Is(err, ErrNoTrial) {
		if _, ierr := s.db.ExecContext(ctx,
			`INSERT INTO trials (id, user_id, status, created_at, updated_at)
			 VALUES (?,?,?,?,?)
			 ON CONFLICT (user_id) DO NOTHING`,
			newID(), userID, string(billing.TrialEligible), nowUTC(now), nowUTC(now)); ierr != nil {
			return models.Trial{}, ierr
		}
		t, err = s.byUser(ctx, userID)
	}
	if err != nil {
		return models.Trial{}, err
	}
	return s.advance(ctx, t, now)
}

func (s *TrialsStore) byUser(ctx context.Context, userID string) (models.Trial, error) {
	t, err := scanTrial(s.db.QueryRowContext(ctx,
		`SELECT `+trialColumns+` FROM trials WHERE user_id = ?`, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Trial{}, ErrNoTrial
	}
	if err != nil {
		return models.Trial{}, err
	}
	// Day is derived, never stored, and every response carries it — a claim
	// whose own reply said "day 0" would send the client back to the shelf.
	t.Day = liveTrialDay(t, time.Now())
	return t, nil
}

// advance applies the clock-derived transitions, one CAS step at a time, and
// returns the settled state. A step that loses a race re-reads rather than
// trusts; a step the graph refuses is a bug in the graph, not a runtime
// condition, so it is returned as an error rather than swallowed.
func (s *TrialsStore) advance(ctx context.Context, t models.Trial, now time.Time) (models.Trial, error) {
	for {
		to, moving := clockStep(billing.TrialStatus(t.Status), parseTS(t.StartedAt), parseTS(t.EndsAt), now)
		if !moving {
			t.Day = liveTrialDay(t, now)
			return t, nil
		}
		if err := s.transition(ctx, t.ID, billing.TrialStatus(t.Status), to, now, false); err != nil {
			var te *billing.TrialTransitionError
			if errors.As(err, &te) {
				// Someone else moved it; settle against the new state.
				fresh, rerr := s.byUser(ctx, t.UserID)
				if rerr != nil {
					return models.Trial{}, rerr
				}
				t = fresh
				continue
			}
			return models.Trial{}, err
		}
		moved := t
		moved.Status = string(to)
		if to == billing.TrialActive && moved.StartedAt == "" {
			moved.StartedAt = nowUTC(now)
		}
		t = moved
	}
}

// clockStep is the pure half of the sweep: given a state and the clock, the
// next state time demands, if any. Kept out of SQL so the semantics are unit
// testable without a database.
func clockStep(status billing.TrialStatus, start, end, now time.Time) (billing.TrialStatus, bool) {
	switch status {
	case billing.TrialStarted:
		// Started implies a clock was set; if it already ran out before
		// anything read it, do not pretend a day of it was usable.
		if !end.IsZero() && !now.Before(end) {
			return billing.TrialExpired, true
		}
		return billing.TrialActive, true
	case billing.TrialActive:
		if !end.IsZero() && !now.Before(end) {
			return billing.TrialExpired, true
		}
		if !end.IsZero() && !now.Before(billing.TrialExpiringAt(end)) {
			return billing.TrialExpiring, true
		}
	case billing.TrialExpiring:
		if !end.IsZero() && !now.Before(end) {
			return billing.TrialExpired, true
		}
	}
	return "", false
}

// transition is the CAS write shared by every mover. Entering converted
// stamps converted_at, exactly the way entering published stamps published_at
// for UGC — the moment the state begins is part of the state.
func (s *TrialsStore) transition(ctx context.Context, id string, from, to billing.TrialStatus, now time.Time, force bool) error {
	if !force && !billing.ValidateTransition(from, to) {
		return &billing.TrialTransitionError{From: from, To: to}
	}
	q := `UPDATE trials SET status = ?, updated_at = ?`
	args := []any{string(to), nowUTC(now)}
	if to == billing.TrialConverted {
		q += `, converted_at = ?`
		args = append(args, nowUTC(now))
	}
	q += ` WHERE id = ? AND status = ?`
	args = append(args, id, string(from))
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return &billing.TrialTransitionError{From: from, To: to}
	}
	return nil
}

// Start claims the offer: eligible→started with the seven-day clock set in
// the same write. A claim on a used trial (started..converted) is the honest
// 409 the derived-trial design could never answer with: one trial per account,
// ever, including after expiry.
func (s *TrialsStore) Start(ctx context.Context, userID string, now time.Time) (models.Trial, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.Trial{}, err
	}
	defer tx.Rollback()

	t, err := scanTrial(tx.QueryRowContext(ctx,
		`SELECT `+trialColumns+` FROM trials WHERE user_id = ? FOR UPDATE`, userID))
	if errors.Is(err, sql.ErrNoRows) {
		// No row yet: the first read of the lifecycle creates eligible; a
		// start arriving first creates and claims it in this one transaction.
		id := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO trials (id, user_id, status, created_at, updated_at)
			 VALUES (?,?,?,?,?)`,
			id, userID, string(billing.TrialEligible), nowUTC(now), nowUTC(now)); err != nil {
			return models.Trial{}, err
		}
		t = models.Trial{ID: id, UserID: userID, Status: string(billing.TrialEligible)}
	} else if err != nil {
		return models.Trial{}, err
	}

	if err := transitionTx(ctx, tx, t.ID, billing.TrialStatus(t.Status), billing.TrialStarted, now); err != nil {
		return models.Trial{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE trials SET started_at = ?, ends_at = ? WHERE id = ?`,
		nowUTC(now), nowUTC(now.Add(billing.TrialDuration)), t.ID); err != nil {
		return models.Trial{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.Trial{}, err
	}
	return s.byUser(ctx, userID)
}

func transitionTx(ctx context.Context, tx *db.Tx, id string, from, to billing.TrialStatus, now time.Time) error {
	if !billing.ValidateTransition(from, to) {
		return &billing.TrialTransitionError{From: from, To: to}
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE trials SET status = ?, updated_at = ? WHERE id = ? AND status = ?`,
		string(to), nowUTC(now), id, string(from))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return &billing.TrialTransitionError{From: from, To: to}
	}
	return nil
}

// MarkExpiring moves an active trial into its last day. The API exposes it,
// but the read path and Sweep normally arrive here on their own; an explicit
// call lets an operator or the worker say "warn them now" without editing the
// clock.
func (s *TrialsStore) MarkExpiring(ctx context.Context, userID string, now time.Time) (models.Trial, error) {
	return s.step(ctx, userID, billing.TrialExpiring, now)
}

// Expire ends a running trial early. §36 does not enumerate a revocation, so
// this exists for the sweep and for ops, not for users: no client call can
// reach it with a target state the clock has not earned.
func (s *TrialsStore) Expire(ctx context.Context, userID string, now time.Time) (models.Trial, error) {
	return s.step(ctx, userID, billing.TrialExpired, now)
}

// Convert records the purchase the offer existed to produce. It runs from any
// running or ended state, never from eligible: an account that bought without
// starting a trial did not convert one, and marking it converted would
// silently retire an offer it never used.
func (s *TrialsStore) Convert(ctx context.Context, userID string, now time.Time) (models.Trial, error) {
	t, err := s.byUser(ctx, userID)
	if errors.Is(err, ErrNoTrial) {
		return t, err
	}
	if err != nil {
		return models.Trial{}, err
	}
	if billing.TrialStatus(t.Status) == billing.TrialConverted {
		return t, nil // idempotent: a re-verified receipt does not re-stamp
	}
	from := billing.TrialStatus(t.Status)
	if from == billing.TrialEligible {
		return t, &billing.TrialTransitionError{From: from, To: billing.TrialConverted}
	}
	if err := s.transition(ctx, t.ID, from, billing.TrialConverted, now, false); err != nil {
		return models.Trial{}, err
	}
	return s.byUser(ctx, userID)
}

func (s *TrialsStore) step(ctx context.Context, userID string, to billing.TrialStatus, now time.Time) (models.Trial, error) {
	t, err := s.Current(ctx, userID, now)
	if err != nil {
		return models.Trial{}, err
	}
	from := billing.TrialStatus(t.Status)
	if from == to {
		return t, nil
	}
	if err := s.transition(ctx, t.ID, from, to, now, false); err != nil {
		return models.Trial{}, err
	}
	return s.byUser(ctx, userID)
}

// Sweep applies the clock to every open trial in one pass: the query a worker
// runs (or that nothing runs, since Current advances on read). It reports the
// number of rows it moved so a caller can see silence is not the same as
// "nothing needed doing".
//
// The statements run expire-first so a started trial whose clock already ran
// out is not promoted to active just to be demoted again by the same sweep.
// Timestamps are the lexicographically ordered UTC RFC3339 strings this
// schema stores everywhere, compared against bind parameters, never pasted
// into SQL.
func (s *TrialsStore) Sweep(ctx context.Context, now time.Time) (int, error) {
	utc := nowUTC(now)
	expiringEdge := nowUTC(now.Add(billing.TrialExpiringWindow))
	statements := []struct {
		sql  string
		args []any
	}{
		{`UPDATE trials SET status='expired', updated_at = ?
		  WHERE status IN ('started','active','expiring')
		    AND ends_at IS NOT NULL AND ends_at <= ?`, []any{utc, utc}},
		{`UPDATE trials SET status='active', updated_at = ?
		  WHERE status='started'
		    AND started_at IS NOT NULL AND started_at <= ?`, []any{utc, utc}},
		{`UPDATE trials SET status='expiring', updated_at = ?
		  WHERE status='active'
		    AND ends_at IS NOT NULL AND ends_at > ? AND ends_at <= ?`,
			[]any{utc, utc, expiringEdge}},
	}
	moved := 0
	for _, st := range statements {
		res, err := s.db.ExecContext(ctx, st.sql, st.args...)
		if err != nil {
			return moved, err
		}
		n, _ := res.RowsAffected()
		moved += int(n)
	}
	return moved, nil
}

// SetStatus is the PATCH path: an explicit forward move, graph-validated,
// from an actor who says why. It is deliberately unable to jump the graph —
// the point of §36 is that no caller, including an admin, gets to un-expire a
// trial.
func (s *TrialsStore) SetStatus(ctx context.Context, userID string, to billing.TrialStatus, now time.Time) (models.Trial, error) {
	t, err := s.Current(ctx, userID, now)
	if err != nil {
		return models.Trial{}, err
	}
	from := billing.TrialStatus(t.Status)
	if from == to {
		return t, nil
	}
	if err := s.transition(ctx, t.ID, from, to, now, false); err != nil {
		return models.Trial{}, err
	}
	if to == billing.TrialStarted {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE trials SET started_at = ?, ends_at = ? WHERE id = ?`,
			nowUTC(now), nowUTC(now.Add(billing.TrialDuration)), t.ID); err != nil {
			return models.Trial{}, err
		}
	}
	return s.byUser(ctx, userID)
}

// liveTrialDay answers "which journey day" only while the clock is running.
// A converted trial's window may still contain "now" — the purchase can land
// on day two — but the journey is over the moment there is a subscription;
// reporting day 2 under CONVERTED would make the paywall advertise a trial
// that has been used.
func liveTrialDay(t models.Trial, now time.Time) int {
	switch billing.TrialStatus(t.Status) {
	case billing.TrialStarted, billing.TrialActive, billing.TrialExpiring:
		return billing.TrialDayWithin(parseTS(t.StartedAt), parseTS(t.EndsAt), now)
	}
	return 0
}

func nowUTC(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// parseTS lives in jobqueue.go: RFC3339 in, zero time on any failure. Same
// semantics this file needs, one helper shared.
