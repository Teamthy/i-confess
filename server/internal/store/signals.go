package store

import (
	"context"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/personalization"
	"github.com/Teamthy/i-confess/internal/sessions"
)

// SignalStore reads the evidence personalization ranks on (master-plan 34).
//
// It reads and never writes. The signals are derived from tables other
// features already maintain — session_items and sessions are written by the
// session engine, favorites by the library — so there is no second bookkeeping
// that can drift from what the listener actually did. That is also why the
// derivation happens in the personalization package rather than in SQL: the
// rules are unit-tested against fixed evidence, and the store's only job is to
// fetch the evidence faithfully.
type SignalStore struct{ db *db.DB }

func NewSignalStore(db *db.DB) *SignalStore { return &SignalStore{db: db} }

// listenWindow bounds how much history one request reads. A listener with
// years of sessions is ranked on their most recent ~2000 items, which is
// months of daily use and keeps the query cheap and the result current.
const listenWindow = 2000

// Signals assembles the seven-signal profile for a listener. tz is the
// listener's profile timezone; an unknown name falls back to UTC rather than
// failing the request, because a wrong daypart is a weaker recommendation and
// a 500 is no recommendation.
func (s *SignalStore) Signals(ctx context.Context, userID, tz string) (personalization.Signals, error) {
	var out personalization.Signals
	if loc, err := time.LoadLocation(tz); err == nil && tz != "" {
		out.Location = loc
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT si.session_id, si.confession_id, COALESCE(c.category_id,''), si.status, COALESCE(s.started_at,'')
		   FROM session_items si
		   JOIN sessions s ON s.id = si.session_id
		   LEFT JOIN confessions c ON c.id = si.confession_id
		  WHERE s.user_id = ? AND s.deleted_at IS NULL
		  ORDER BY s.created_at DESC, si.position ASC
		  LIMIT ?`, userID, listenWindow)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var l personalization.Listen
		var status, startedAt string
		if err := rows.Scan(&l.SessionID, &l.ConfessionID, &l.CategoryID, &status, &startedAt); err != nil {
			return out, err
		}
		// Only what the listener finished or passed over is evidence. Queued
		// and playing items are undecided; failed items are the platform's
		// fault, not a preference.
		switch sessions.ItemStatus(sessions.NormalizeItemStatus(status)) {
		case sessions.ItemCompleted:
			l.Completed = true
		case sessions.ItemSkipped:
			l.Completed = false
		default:
			continue
		}
		if startedAt != "" {
			if t, err := time.Parse(time.RFC3339, startedAt); err == nil {
				l.StartedAt = t
			}
		}
		out.Listens = append(out.Listens, l)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	durRows, err := s.db.QueryContext(ctx,
		`SELECT CASE WHEN COALESCE(actual_duration,0) > 0 THEN actual_duration ELSE duration_seconds END
		   FROM sessions
		  WHERE user_id = ? AND status = ? AND deleted_at IS NULL
		  ORDER BY created_at DESC
		  LIMIT 200`, userID, string(sessions.Completed))
	if err != nil {
		return out, err
	}
	defer durRows.Close()
	for durRows.Next() {
		var d int
		if err := durRows.Scan(&d); err != nil {
			return out, err
		}
		if d > 0 {
			out.CompletedDurations = append(out.CompletedDurations, d)
		}
	}
	if err := durRows.Err(); err != nil {
		return out, err
	}

	favRows, err := s.db.QueryContext(ctx,
		`SELECT entity_type, entity_id FROM favorites WHERE user_id = ?`, userID)
	if err != nil {
		return out, err
	}
	defer favRows.Close()
	out.Favourites = personalization.Favourites{
		Confessions: map[string]bool{}, Categories: map[string]bool{}, Voices: map[string]bool{},
	}
	for favRows.Next() {
		var kind, id string
		if err := favRows.Scan(&kind, &id); err != nil {
			return out, err
		}
		switch kind {
		case "confession":
			out.Favourites.Confessions[id] = true
		case "category":
			out.Favourites.Categories[id] = true
		case "voice":
			out.Favourites.Voices[id] = true
		}
	}
	return out, favRows.Err()
}
