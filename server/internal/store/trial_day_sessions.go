package store

import (
	"context"
	"time"
)

// Trial day sessions (G-48): which sessions the journey built for a day.
//
// A slot is claimed before its session is built and attached afterwards. That
// ordering is what makes POST /subscriptions/trial/today idempotent: a retry
// finds the attached session and returns it, and a concurrent duplicate finds
// an unattached claim and backs off instead of building a second session.

// TrialDaySlot is one claimed slot. SessionID is empty while the session is
// still being built.
type TrialDaySlot struct {
	Day       int
	Slot      int
	SessionID string
}

// DaySlots returns the claimed slots for a user's journey day, by slot.
func (s *TrialStore) DaySlots(ctx context.Context, userID string, day int) (map[int]TrialDaySlot, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT slot, COALESCE(session_id,'') FROM trial_day_sessions
		 WHERE user_id=? AND day=? AND deleted_at IS NULL`, userID, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]TrialDaySlot{}
	for rows.Next() {
		var sl TrialDaySlot
		if err := rows.Scan(&sl.Slot, &sl.SessionID); err != nil {
			return nil, err
		}
		sl.Day = day
		out[sl.Slot] = sl
	}
	return out, rows.Err()
}

// ClaimDaySlot claims one slot before its session is built. It reports false
// when the slot was already claimed (by an earlier or a concurrent request).
func (s *TrialStore) ClaimDaySlot(ctx context.Context, trialID, userID string, day, slot int, at time.Time) (bool, error) {
	now := at.UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO trial_day_sessions (id,trial_id,user_id,day,slot,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?)
		 ON CONFLICT (user_id,day,slot) DO NOTHING`,
		newID(), trialID, userID, day, slot, now, now)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// AttachDaySession records the session built for a claimed slot.
func (s *TrialStore) AttachDaySession(ctx context.Context, userID string, day, slot int, sessionID string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE trial_day_sessions SET session_id=?, updated_at=?, row_version=row_version+1
		 WHERE user_id=? AND day=? AND slot=? AND deleted_at IS NULL`,
		sessionID, at.UTC().Format(time.RFC3339), userID, day, slot)
	return err
}

// ReleaseDaySlot drops a claim whose build failed, so the next request can try
// again. An attached slot is never released.
func (s *TrialStore) ReleaseDaySlot(ctx context.Context, userID string, day, slot int) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM trial_day_sessions
		 WHERE user_id=? AND day=? AND slot=? AND session_id IS NULL`,
		userID, day, slot)
	return err
}
