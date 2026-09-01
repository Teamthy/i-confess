// Package scheduler decides when a user's routine is due and dispatches the
// reminder (§19, §33, §34, §47).
//
// All the difficulty here is timezones. A user's "6:00 AM" means 6:00 AM where
// they are, which is a different UTC instant in summer than in winter, and a
// different instant again if they travel. Getting this wrong is not a rounding
// error — it wakes someone at 4 AM.
package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Weekday numbering. The schedule table stores 1=Monday .. 7=Sunday (ISO-8601),
// which differs from Go's time.Weekday (0=Sunday). Conflating the two shifts
// every schedule by a day, so conversion happens in exactly one place.
const (
	Monday = 1
	Sunday = 7
)

// isoWeekday converts a Go weekday to the ISO numbering the schema uses.
func isoWeekday(d time.Weekday) int {
	if d == time.Sunday {
		return Sunday
	}
	return int(d)
}

// Schedule is the subset of a stored schedule this package needs.
type Schedule struct {
	ID     string
	UserID string
	Label  string
	// Time is "HH:MM" in the user's local wall clock.
	Time string
	// DaysOfWeek uses ISO numbering, 1=Monday .. 7=Sunday.
	DaysOfWeek []int
	// Timezone is an IANA name. A fixed offset would break across DST.
	Timezone        string
	DurationSeconds int
	Enabled         bool
}

// ParseDays reads the stored comma-separated day list.
func ParseDays(csv string) []int {
	var out []int
	for _, part := range strings.Split(csv, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < Monday || n > Sunday {
			continue
		}
		out = append(out, n)
	}
	return out
}

// ParseTime reads "HH:MM" into hour and minute.
func ParseTime(hhmm string) (hour, minute int, err error) {
	parts := strings.Split(strings.TrimSpace(hhmm), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("time %q is not HH:MM", hhmm)
	}
	hour, err = strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("invalid hour in %q", hhmm)
	}
	minute, err = strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("invalid minute in %q", hhmm)
	}
	return hour, minute, nil
}

// OccurrenceKey identifies one intended firing, e.g. "2026-09-02T06:00".
//
// Keyed on the user's local wall clock rather than a UTC instant: that is what
// the user asked for, and it keeps the key stable if they travel. It is the
// idempotency key for dispatch, so a sweeper that runs twice cannot send twice.
func OccurrenceKey(localTime time.Time) string {
	return localTime.Format("2006-01-02T15:04")
}

// Due reports whether a schedule should fire in the window (after, now].
//
// A window rather than an instant because the sweeper runs periodically and
// would otherwise miss firings between ticks. Half-open at the start so two
// consecutive windows cannot both claim the same moment.
func Due(s Schedule, after, now time.Time) (key string, due bool) {
	if !s.Enabled || len(s.DaysOfWeek) == 0 {
		return "", false
	}
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		// An unparseable timezone must not silently become UTC: that would
		// fire at the wrong hour for most of the world. Skip and let the
		// caller log it.
		return "", false
	}
	hour, minute, err := ParseTime(s.Time)
	if err != nil {
		return "", false
	}

	localAfter := after.In(loc)
	localNow := now.In(loc)

	// Walk each local calendar day the window touches. A window is normally
	// minutes long, but after downtime it can span days, and every missed
	// occurrence still needs its own key so nothing is double-sent later.
	day := time.Date(localAfter.Year(), localAfter.Month(), localAfter.Day(), 0, 0, 0, 0, loc)
	end := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)

	for !day.After(end) {
		if containsDay(s.DaysOfWeek, isoWeekday(day.Weekday())) {
			// Constructing the local time this way lets the zone resolve DST.
			fire := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc)
			if fire.After(localAfter) && !fire.After(localNow) {
				return OccurrenceKey(fire), true
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return "", false
}

// NextOccurrence returns when a schedule will next fire, for display.
//
// Returns the zero time if the schedule can never fire, which a caller should
// surface rather than showing a misleading "next: never".
func NextOccurrence(s Schedule, from time.Time) time.Time {
	if !s.Enabled || len(s.DaysOfWeek) == 0 {
		return time.Time{}
	}
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return time.Time{}
	}
	hour, minute, err := ParseTime(s.Time)
	if err != nil {
		return time.Time{}
	}

	local := from.In(loc)
	// Eight days, not seven: a schedule at today's already-passed time must
	// roll to the same weekday next week.
	for i := 0; i < 8; i++ {
		day := local.AddDate(0, 0, i)
		if !containsDay(s.DaysOfWeek, isoWeekday(day.Weekday())) {
			continue
		}
		fire := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc)
		if fire.After(local) {
			return fire
		}
	}
	return time.Time{}
}

func containsDay(days []int, want int) bool {
	for _, d := range days {
		if d == want {
			return true
		}
	}
	return false
}
