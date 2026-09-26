package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Teamthy/i-confess/internal/analytics"
	"github.com/Teamthy/i-confess/internal/billing"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/trial"
	"github.com/google/uuid"
)

// TrialDayCompletion records that a real session was completed on one journey
// day. SessionID is the evidence: a day is completed by playback, not by a
// client saying so.
type TrialDayCompletion struct {
	ID          string `json:"id"`
	TrialID     string `json:"trial_id"`
	UserID      string `json:"user_id"`
	Day         int    `json:"day"`
	SessionID   string `json:"session_id,omitempty"`
	CompletedAt string `json:"completed_at"`
}

// TrialEngagement is the measured journey: which days were actually completed,
// plus the funnel counts that make a conversion rate a real number rather than
// an assertion.
type TrialEngagement struct {
	TrialID        string               `json:"trial_id"`
	State          trial.State          `json:"state"`
	CurrentDay     int                  `json:"current_day"`
	CompletedDays  []int                `json:"completed_days"`
	DaysCompleted  int                  `json:"days_completed"`
	DaysTotal      int                  `json:"days_total"`
	CompletionRate float64              `json:"completion_rate"`
	Completions    []TrialDayCompletion `json:"completions"`
	Funnel         map[string]int       `json:"funnel"`
	Events         []analytics.Event    `json:"events"`
}

// CompleteDay records one journey day as completed, idempotently.
//
// It is called from the session completion path, never from a client request,
// so the row always names a session that genuinely reached COMPLETED. Three
// refusals matter and are errors rather than silent no-ops:
//
//   - no running trial: a paid listener or an account that never started is not
//     on a journey, and inventing day rows for them would inflate the funnel;
//   - a day outside 1..7: the CHECK constraint would reject it, and the caller
//     should find out here instead;
//   - a second completion on the same day: not an error, but reported as
//     already recorded so the caller does not count it twice.
//
// The day is derived from the trial clock, not supplied by the caller. A
// listener who finishes two sessions on Day 3 completes Day 3 once.
func (s *TrialStore) CompleteDay(ctx context.Context, userID, sessionID string, at time.Time) (*TrialDayCompletion, bool, error) {
	if userID == "" {
		return nil, false, ErrNotFound
	}
	at = at.UTC()

	row, err := s.Refresh(ctx, userID, at)
	if err != nil {
		return nil, false, err
	}
	if row.State != trial.Active && row.State != trial.Expiring {
		return nil, false, fmt.Errorf("%w: trial state is %s", ErrTrialNotEligible, row.State)
	}
	started, _ := parseTime(row.StartedAt)
	expires, _ := parseTime(row.ExpiresAt)
	day := trial.DayFor(started, expires, at)
	if day < 1 || day > len(billing.TrialJourney) {
		return nil, false, fmt.Errorf("%w: no current journey day", ErrTrialNotEligible)
	}

	rec := &TrialDayCompletion{
		ID:          newID(),
		TrialID:     row.ID,
		UserID:      userID,
		Day:         day,
		SessionID:   sessionID,
		CompletedAt: at.Format(time.RFC3339),
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO trial_day_completions
		 (id,trial_id,user_id,day,session_id,completed_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?)
		 ON CONFLICT (user_id,day) DO NOTHING`,
		rec.ID, rec.TrialID, rec.UserID, rec.Day, nullIfEmpty(rec.SessionID),
		rec.CompletedAt, rec.CompletedAt, rec.CompletedAt)
	if err != nil {
		return nil, false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		existing, err := s.dayCompletion(ctx, userID, day)
		if err != nil {
			return nil, false, err
		}
		// Return the row that is actually stored so a caller reporting "day 3
		// complete" never cites an id that matches nothing.
		return existing, false, nil
	}
	return rec, true, nil
}

func (s *TrialStore) dayCompletion(ctx context.Context, userID string, day int) (*TrialDayCompletion, error) {
	var rec TrialDayCompletion
	var sessionID sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id,trial_id,user_id,day,session_id,completed_at
		 FROM trial_day_completions WHERE user_id=? AND day=? AND deleted_at IS NULL`,
		userID, day).Scan(&rec.ID, &rec.TrialID, &rec.UserID, &rec.Day, &sessionID, &rec.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rec.SessionID = sessionID.String
	return &rec, nil
}

// Engagement returns the measured journey for one account. Days completed is
// counted from persisted rows, so the denominator of a conversion funnel is
// observable rather than inferred from a clock.
func (s *TrialStore) Engagement(ctx context.Context, userID string, at time.Time) (*TrialEngagement, error) {
	row, err := s.Refresh(ctx, userID, at)
	if err != nil {
		return nil, err
	}
	started, _ := parseTime(row.StartedAt)
	expires, _ := parseTime(row.ExpiresAt)

	out := &TrialEngagement{
		TrialID:       row.ID,
		State:         row.State,
		CurrentDay:    trial.DayFor(started, expires, at.UTC()),
		CompletedDays: []int{},
		Completions:   []TrialDayCompletion{},
		DaysTotal:     len(billing.TrialJourney),
		Funnel:        map[string]int{},
		Events:        []analytics.Event{},
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id,trial_id,user_id,day,session_id,completed_at
		 FROM trial_day_completions WHERE user_id=? AND deleted_at IS NULL ORDER BY day`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rec TrialDayCompletion
		var sessionID sql.NullString
		if err := rows.Scan(&rec.ID, &rec.TrialID, &rec.UserID, &rec.Day, &sessionID, &rec.CompletedAt); err != nil {
			return nil, err
		}
		rec.SessionID = sessionID.String
		out.Completions = append(out.Completions, rec)
		out.CompletedDays = append(out.CompletedDays, rec.Day)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out.DaysCompleted = len(out.CompletedDays)
	if out.DaysTotal > 0 {
		out.CompletionRate = float64(out.DaysCompleted) / float64(out.DaysTotal)
	}

	// Funnel counts are read from the same persisted events the analytics
	// surface exposes, so an admin dashboard and this endpoint cannot disagree.
	funnel, events, err := s.trialFunnel(ctx, userID)
	if err != nil {
		return nil, err
	}
	out.Funnel = funnel
	out.Events = events
	return out, nil
}

func (s *TrialStore) trialFunnel(ctx context.Context, userID string) (map[string]int, []analytics.Event, error) {
	funnel := map[string]int{}
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, props, occurred_at FROM analytics_events
		 WHERE user_id=? AND deleted_at IS NULL AND name IN (?,?,?,?,?)
		 ORDER BY occurred_at`,
		userID, analytics.EventTrialStarted, analytics.EventTrialDayCompleted,
		analytics.EventTrialConverted, analytics.EventTrialExpired, analytics.EventSubscriptionCancelled)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var events []analytics.Event
	for rows.Next() {
		var name, occurred string
		var props sql.NullString
		if err := rows.Scan(&name, &props, &occurred); err != nil {
			return nil, nil, err
		}
		funnel[name]++
		ev := analytics.Event{Name: name, UserID: userID, Timestamp: occurred}
		if props.Valid && props.String != "" {
			decoded := map[string]any{}
			if err := json.Unmarshal([]byte(props.String), &decoded); err == nil {
				ev.Props = decoded
			}
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if events == nil {
		events = []analytics.Event{}
	}
	return funnel, events, nil
}

// CompletedDayNumbers returns the sorted set of completed journey days. Used by
// the journey projection so the UI can mark a day done without recomputing.
func (s *TrialStore) CompletedDayNumbers(ctx context.Context, userID string) ([]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT day FROM trial_day_completions WHERE user_id=? AND deleted_at IS NULL ORDER BY day`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int{}
	for rows.Next() {
		var d int
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Persisted analytics
// ---------------------------------------------------------------------------

// AnalyticsStore persists the events that were previously acknowledged and
// dropped. Only actor and entity identifiers are stored: the ingestion path
// strips body, email and scripture text before anything reaches here, so the
// table cannot become a place where confession content accumulates.
type AnalyticsStore struct{ db *db.DB }

func NewAnalyticsStore(database *db.DB) *AnalyticsStore { return &AnalyticsStore{db: database} }

// Record writes one event. A missing timestamp is stamped now so an event
// cannot be inserted with an empty occurred_at and vanish from a range query.
func (s *AnalyticsStore) Record(ctx context.Context, ev analytics.Event) error {
	if ev.Name == "" {
		return errors.New("analytics: event name is required")
	}
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	var props string
	if len(ev.Props) > 0 {
		encoded, err := json.Marshal(ev.Props)
		if err != nil {
			return err
		}
		props = string(encoded)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO analytics_events (id,user_id,name,props,occurred_at,created_at)
		 VALUES (?,?,?,?,?,?)`,
		uuid.NewString(), nullIfEmpty(ev.UserID), ev.Name, nullIfEmpty(props), ev.Timestamp, now())
	return err
}

// Track makes AnalyticsStore satisfy analytics.Sink, so the ingestion endpoint
// and the server-side trial transitions can share one writer.
func (s *AnalyticsStore) Track(ev analytics.Event) error { return s.Record(context.Background(), ev) }

// Count returns how many times an event was recorded for a user. Zero is a
// meaningful answer: it is how a test proves an event was never emitted.
func (s *AnalyticsStore) Count(ctx context.Context, userID, name string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM analytics_events WHERE user_id=? AND name=? AND deleted_at IS NULL`,
		userID, name).Scan(&n)
	return n, err
}

// Recent returns the newest events for a user, newest first, for the
// subscription-management surface and admin analytics module.
func (s *AnalyticsStore) Recent(ctx context.Context, userID string, limit int) ([]analytics.Event, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, props, occurred_at FROM analytics_events
		 WHERE user_id=? AND deleted_at IS NULL ORDER BY occurred_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []analytics.Event{}
	for rows.Next() {
		var name, occurred string
		var props sql.NullString
		if err := rows.Scan(&name, &props, &occurred); err != nil {
			return nil, err
		}
		ev := analytics.Event{Name: name, UserID: userID, Timestamp: occurred}
		if props.Valid && props.String != "" {
			decoded := map[string]any{}
			if err := json.Unmarshal([]byte(props.String), &decoded); err == nil {
				ev.Props = decoded
			}
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}
