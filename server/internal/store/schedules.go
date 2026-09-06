package store

import (
	"context"
	"database/sql"
	"github.com/Teamthy/i-confess/internal/db"
	"strconv"
	"strings"

	"github.com/Teamthy/i-confess/internal/models"
)

// ScheduleStore manages user schedules.
type ScheduleStore struct{ db *db.DB }

func NewScheduleStore(db *db.DB) *ScheduleStore { return &ScheduleStore{db: db} }

func (s *ScheduleStore) Create(ctx context.Context, sc *models.Schedule) error {
	if sc.ID == "" {
		sc.ID = newID()
	}
	if sc.Timezone == "" {
		sc.Timezone = "UTC"
	}
	sc.CreatedAt, sc.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO schedules (id,user_id,label,time,days_of_week,timezone,duration_seconds,voice_id,category_ids,enabled,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		sc.ID, sc.UserID, sc.Label, sc.Time, daysToStr(sc.DaysOfWeek), sc.Timezone, sc.DurationSeconds,
		nullIfEmpty(sc.VoiceID), nullIfEmpty(strings.Join(sc.CategoryIDs, ",")), boolInt(sc.Enabled), sc.CreatedAt, sc.UpdatedAt)
	return err
}

func (s *ScheduleStore) ByID(ctx context.Context, id string) (*models.Schedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,label,time,days_of_week,timezone,duration_seconds,COALESCE(voice_id,''),COALESCE(category_ids,''),enabled,created_at,updated_at
		 FROM schedules WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	return scanSchedule(rows)
}

func (s *ScheduleStore) ListByUser(ctx context.Context, userID string) ([]models.Schedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,label,time,days_of_week,timezone,duration_seconds,COALESCE(voice_id,''),COALESCE(category_ids,''),enabled,created_at,updated_at
		 FROM schedules WHERE user_id = ? ORDER BY time`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Schedule
	for rows.Next() {
		sc, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sc)
	}
	return out, rows.Err()
}

func (s *ScheduleStore) Update(ctx context.Context, sc *models.Schedule) error {
	sc.UpdatedAt = now()
	_, err := s.db.ExecContext(ctx,
		`UPDATE schedules SET label=?, time=?, days_of_week=?, timezone=?, duration_seconds=?, voice_id=?, category_ids=?, enabled=?, updated_at=?
		 WHERE id=? AND user_id=?`,
		sc.Label, sc.Time, daysToStr(sc.DaysOfWeek), sc.Timezone, sc.DurationSeconds, nullIfEmpty(sc.VoiceID),
		nullIfEmpty(strings.Join(sc.CategoryIDs, ",")), boolInt(sc.Enabled), sc.UpdatedAt, sc.ID, sc.UserID)
	return err
}

func (s *ScheduleStore) Delete(ctx context.Context, id, userID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM schedules WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanSchedule(rows *sql.Rows) (*models.Schedule, error) {
	var sc models.Schedule
	var days, cats sql.NullString
	if err := rows.Scan(&sc.ID, &sc.UserID, &sc.Label, &sc.Time, &days, &sc.Timezone, &sc.DurationSeconds, &sc.VoiceID, &cats, &sc.Enabled, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
		return nil, err
	}
	sc.DaysOfWeek = strToDays(days.String)
	if cats.Valid && cats.String != "" {
		sc.CategoryIDs = strings.Split(cats.String, ",")
	}
	return &sc, nil
}

func daysToStr(days []int) string {
	if len(days) == 0 {
		return "1,2,3,4,5,6,7"
	}
	parts := make([]string, len(days))
	for i, d := range days {
		parts[i] = strconv.Itoa(d)
	}
	return strings.Join(parts, ",")
}

func strToDays(s string) []int {
	if s == "" {
		return []int{1, 2, 3, 4, 5, 6, 7}
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n >= 1 && n <= 7 {
			out = append(out, n)
		}
	}
	return out
}

// AllEnabled returns every enabled schedule, for the dispatcher sweep.
//
// Not filtered by time in SQL: whether a schedule is due depends on the user's
// IANA timezone and DST rules, which the database cannot evaluate. The row
// count is bounded by active users with routines, which is small enough to
// scan each tick.
func (s *ScheduleStore) AllEnabled(ctx context.Context) ([]models.Schedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,label,time,days_of_week,timezone,duration_seconds,COALESCE(voice_id,''),COALESCE(category_ids,''),enabled,created_at,updated_at
		 FROM schedules WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.Schedule{}
	for rows.Next() {
		sc, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sc)
	}
	return out, rows.Err()
}
