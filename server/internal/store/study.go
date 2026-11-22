package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
)

// StudyStore holds a listener's private study data: journal entries and the
// Scripture verses they are memorising. Every read and write is scoped to the
// owner; another user's id is indistinguishable from one that does not exist.
type StudyStore struct{ db *db.DB }

// NewStudyStore returns a store over the given database.
func NewStudyStore(d *db.DB) *StudyStore { return &StudyStore{db: d} }

// JournalEntry is one private journal entry.
type JournalEntry struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Mood       string `json:"mood"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	RowVersion int    `json:"row_version"`
}

// ListJournal returns the owner's entries, newest first.
func (s *StudyStore) ListJournal(ctx context.Context, userID string, limit int) ([]JournalEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,title,body,mood,created_at,updated_at,row_version FROM journal_entries
		 WHERE user_id=? AND deleted_at IS NULL ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]JournalEntry, 0)
	for rows.Next() {
		var e JournalEntry
		if err := rows.Scan(&e.ID, &e.Title, &e.Body, &e.Mood, &e.CreatedAt, &e.UpdatedAt, &e.RowVersion); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CreateJournal stores a new entry.
func (s *StudyStore) CreateJournal(ctx context.Context, userID string, e *JournalEntry) error {
	e.ID, e.CreatedAt, e.UpdatedAt, e.RowVersion = newID(), now(), now(), 1
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO journal_entries (id,user_id,title,body,mood,created_at,updated_at) VALUES (?,?,?,?,?,?,?)`,
		e.ID, userID, e.Title, e.Body, e.Mood, e.CreatedAt, e.UpdatedAt)
	return err
}

// UpdateJournal replaces an entry's text. It returns ErrNotFound for an entry
// that is not the owner's, and ErrConflict when rowVersion is stale, so an
// edit made on another device is never silently overwritten.
func (s *StudyStore) UpdateJournal(ctx context.Context, userID string, e *JournalEntry, rowVersion int) error {
	e.UpdatedAt = now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE journal_entries SET title=?, body=?, mood=?, updated_at=?, row_version=row_version+1
		 WHERE id=? AND user_id=? AND deleted_at IS NULL AND row_version=?`,
		e.Title, e.Body, e.Mood, e.UpdatedAt, e.ID, userID, rowVersion)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		e.RowVersion = rowVersion + 1
		return nil
	}
	var exists int
	err = s.db.QueryRowContext(ctx, `SELECT 1 FROM journal_entries WHERE id=? AND user_id=? AND deleted_at IS NULL`, e.ID, userID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return ErrStudyConflict
}

// DeleteJournal removes an entry. Deleting is a soft delete so the retention
// sweep, not a request, decides when bytes leave the database.
func (s *StudyStore) DeleteJournal(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE journal_entries SET deleted_at=?, row_version=row_version+1 WHERE id=? AND user_id=? AND deleted_at IS NULL`,
		now(), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ErrStudyConflict is a stale row_version on a journal edit.
var ErrStudyConflict = errors.New("entry changed since it was read")

// MemoryVerse is one verse in spaced practice.
type MemoryVerse struct {
	ID        string `json:"id"`
	Reference string `json:"reference"`
	Text      string `json:"text"`
	Title     string `json:"title"`
	Box       int    `json:"box"`
	DueAt     string `json:"due_at"`
	CreatedAt string `json:"created_at"`
}

// MemoryIntervals are the days until the next return for each box (Leitner).
var MemoryIntervals = [6]int{1, 1, 3, 7, 14, 30}

// ListMemory returns the owner's verses, soonest due first.
func (s *StudyStore) ListMemory(ctx context.Context, userID string) ([]MemoryVerse, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,reference,text,title,box,due_at,created_at FROM memory_verses
		 WHERE user_id=? AND deleted_at IS NULL ORDER BY due_at ASC LIMIT 500`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MemoryVerse, 0)
	for rows.Next() {
		var m MemoryVerse
		if err := rows.Scan(&m.ID, &m.Reference, &m.Text, &m.Title, &m.Box, &m.DueAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddMemory puts a verse into practice, due now. Adding a reference that is
// already in practice reports created=false and leaves its progress alone; one
// that was removed comes back at box 0.
func (s *StudyStore) AddMemory(ctx context.Context, userID string, m *MemoryVerse, at time.Time) (created bool, err error) {
	ts := at.UTC().Format(time.RFC3339)
	m.ID, m.Box, m.DueAt, m.CreatedAt = newID(), 0, ts, ts
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO memory_verses (id,user_id,reference,text,title,box,due_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,0,?,?,?)
		 ON CONFLICT (user_id, reference) DO UPDATE SET
		   text=excluded.text, title=excluded.title, box=0, due_at=excluded.due_at,
		   deleted_at=NULL, updated_at=excluded.updated_at, row_version=memory_verses.row_version+1
		 WHERE memory_verses.deleted_at IS NOT NULL`,
		m.ID, userID, m.Reference, m.Text, m.Title, ts, ts, ts)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if err := s.db.QueryRowContext(ctx,
		`SELECT id,box,due_at,created_at FROM memory_verses WHERE user_id=? AND reference=?`, userID, m.Reference).
		Scan(&m.ID, &m.Box, &m.DueAt, &m.CreatedAt); err != nil {
		return false, err
	}
	return n == 1, nil
}

// GradeMemory records one recall: remembered moves the verse up a box,
// missed sends it back to the first. The next due time is computed here, not
// accepted from a client.
func (s *StudyStore) GradeMemory(ctx context.Context, userID, id string, remembered bool, at time.Time) (*MemoryVerse, error) {
	m := &MemoryVerse{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id,reference,text,title,box,due_at,created_at FROM memory_verses WHERE id=? AND user_id=? AND deleted_at IS NULL`,
		id, userID).Scan(&m.ID, &m.Reference, &m.Text, &m.Title, &m.Box, &m.DueAt, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if remembered {
		m.Box = min(m.Box+1, 5)
	} else {
		m.Box = 0
	}
	m.DueAt = at.UTC().Add(time.Duration(MemoryIntervals[m.Box]) * 24 * time.Hour).Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`UPDATE memory_verses SET box=?, due_at=?, updated_at=?, row_version=row_version+1 WHERE id=? AND user_id=?`,
		m.Box, m.DueAt, at.UTC().Format(time.RFC3339), id, userID)
	return m, err
}

// DeleteMemory takes a verse out of practice.
func (s *StudyStore) DeleteMemory(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE memory_verses SET deleted_at=?, row_version=row_version+1 WHERE id=? AND user_id=? AND deleted_at IS NULL`,
		now(), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
