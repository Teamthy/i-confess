package scheduler

import (
	"testing"
	"time"
)

// Scheduling correctness (§19, §33, §34).
//
// These tests are mostly about timezones, because that is where the bugs are.
// "6 AM" is not a UTC instant, and getting it wrong wakes someone at 4 AM.

func lagos(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Africa/Lagos")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	return loc
}

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("tzdata for %s unavailable", name)
	}
	return loc
}

func weekdaySchedule(tz string) Schedule {
	return Schedule{
		ID: "s1", UserID: "u1", Label: "Morning",
		Time: "06:00", DaysOfWeek: []int{1, 2, 3, 4, 5},
		Timezone: tz, DurationSeconds: 1800, Enabled: true,
	}
}

// The ISO/Go weekday mismatch shifts every schedule by a day if conflated.
func TestISOWeekdayMapping(t *testing.T) {
	cases := map[time.Weekday]int{
		time.Monday: 1, time.Tuesday: 2, time.Wednesday: 3,
		time.Thursday: 4, time.Friday: 5, time.Saturday: 6, time.Sunday: 7,
	}
	for in, want := range cases {
		if got := isoWeekday(in); got != want {
			t.Fatalf("isoWeekday(%v) = %d, want %d", in, got, want)
		}
	}
}

// A 6 AM schedule must fire at 6 AM local, which is 05:00 UTC in Lagos.
func TestFiresAtLocalTimeNotUTC(t *testing.T) {
	loc := lagos(t)
	s := weekdaySchedule("Africa/Lagos")

	// Wednesday 2026-09-02, 06:00 Lagos == 05:00 UTC.
	fire := time.Date(2026, 9, 2, 6, 0, 0, 0, loc)
	after := fire.Add(-1 * time.Minute)
	now := fire.Add(1 * time.Second)

	key, due := Due(s, after, now)
	if !due {
		t.Fatal("schedule did not fire at its local time")
	}
	if key != "2026-09-02T06:00" {
		t.Fatalf("key = %q, want the local wall clock", key)
	}

	// The same UTC instant must NOT fire for a user in a genuinely different
	// offset. (London is skipped deliberately: in September it is BST, the
	// same UTC+1 as Lagos, so it *should* coincide.)
	ny := s
	ny.Timezone = "America/New_York" // UTC-4 in September: 01:00 local
	if _, due := Due(ny, after, now); due {
		t.Fatal("a Lagos 6 AM instant fired a New York schedule")
	}

	// And the New York user fires at their own 6 AM, four hours later.
	nyLoc := mustLoad(t, "America/New_York")
	nyFire := time.Date(2026, 9, 2, 6, 0, 0, 0, nyLoc)
	if key, due := Due(ny, nyFire.Add(-time.Minute), nyFire); !due || key != "2026-09-02T06:00" {
		t.Fatalf("New York schedule: due=%v key=%q", due, key)
	}
}

// The window is half-open so two consecutive sweeps cannot both claim one
// moment — the property that stops duplicate notifications.
func TestWindowIsHalfOpen(t *testing.T) {
	loc := lagos(t)
	s := weekdaySchedule("Africa/Lagos")
	fire := time.Date(2026, 9, 2, 6, 0, 0, 0, loc)

	// First sweep covers (05:59, 06:00] and claims it.
	if _, due := Due(s, fire.Add(-time.Minute), fire); !due {
		t.Fatal("first window missed the firing")
	}
	// Second sweep starts exactly where the first ended and must not re-claim.
	if _, due := Due(s, fire, fire.Add(time.Minute)); due {
		t.Fatal("consecutive windows both claimed the same firing")
	}
}

// Only configured days fire.
func TestOnlyConfiguredDaysFire(t *testing.T) {
	loc := lagos(t)
	s := weekdaySchedule("Africa/Lagos") // Mon-Fri

	// 2026-09-05 is a Saturday.
	sat := time.Date(2026, 9, 5, 6, 0, 0, 0, loc)
	if _, due := Due(s, sat.Add(-time.Minute), sat); due {
		t.Fatal("a weekday-only schedule fired on Saturday")
	}
	// 2026-09-07 is a Monday.
	mon := time.Date(2026, 9, 7, 6, 0, 0, 0, loc)
	if _, due := Due(s, mon.Add(-time.Minute), mon); !due {
		t.Fatal("a weekday schedule did not fire on Monday")
	}
}

// The central DST case: across a spring-forward, 6 AM local must still mean
// 6 AM local, even though the UTC offset changed overnight.
func TestSurvivesSpringForward(t *testing.T) {
	loc := mustLoad(t, "Europe/London")
	s := weekdaySchedule("Europe/London")

	// UK clocks go forward on 2026-03-29 (a Sunday). Check the Monday after.
	before := time.Date(2026, 3, 27, 6, 0, 0, 0, loc) // Friday, GMT
	after := time.Date(2026, 3, 30, 6, 0, 0, 0, loc)  // Monday, BST

	// The UTC offsets genuinely differ, which is the point.
	_, offBefore := before.Zone()
	_, offAfter := after.Zone()
	if offBefore == offAfter {
		t.Skip("tzdata has no DST transition here")
	}

	for _, fire := range []time.Time{before, after} {
		key, due := Due(s, fire.Add(-time.Minute), fire)
		if !due {
			t.Fatalf("schedule did not fire at %s", fire)
		}
		if got := key[11:]; got != "06:00" {
			t.Fatalf("fired at local %s, want 06:00 — DST shifted the alarm", got)
		}
	}
}

// A schedule at a time that does not exist on a spring-forward day must not
// crash or fire at a wild hour. Go normalises 02:30 on such a day forward.
func TestNonexistentLocalTimeIsHandled(t *testing.T) {
	loc := mustLoad(t, "Europe/London")
	s := Schedule{
		ID: "s1", Time: "01:30", DaysOfWeek: []int{1, 2, 3, 4, 5, 6, 7},
		Timezone: "Europe/London", Enabled: true,
	}

	// 2026-03-29 01:00-02:00 local does not exist in London.
	day := time.Date(2026, 3, 29, 0, 0, 0, 0, loc)
	// Must not panic, and any key produced must be well-formed.
	if key, due := Due(s, day, day.Add(6*time.Hour)); due && len(key) != 16 {
		t.Fatalf("malformed occurrence key across a DST gap: %q", key)
	}
}

// After downtime the sweeper's window can span days. Each missed occurrence
// needs its own key so a later run cannot re-send an already-sent one.
func TestLongWindowAfterDowntime(t *testing.T) {
	loc := lagos(t)
	s := weekdaySchedule("Africa/Lagos")

	// Server down from Monday 05:00 to Wednesday 07:00.
	after := time.Date(2026, 9, 7, 5, 0, 0, 0, loc)
	now := time.Date(2026, 9, 9, 7, 0, 0, 0, loc)

	key, due := Due(s, after, now)
	if !due {
		t.Fatal("no firing found across a multi-day window")
	}
	// The earliest missed occurrence is returned, so a catch-up proceeds in
	// order rather than skipping to the most recent.
	if key != "2026-09-07T06:00" {
		t.Fatalf("key = %q, want the earliest missed occurrence", key)
	}
}

func TestDisabledAndInvalidSchedulesNeverFire(t *testing.T) {
	loc := lagos(t)
	fire := time.Date(2026, 9, 2, 6, 0, 0, 0, loc)
	win := func(s Schedule) bool {
		_, due := Due(s, fire.Add(-time.Minute), fire)
		return due
	}

	disabled := weekdaySchedule("Africa/Lagos")
	disabled.Enabled = false
	if win(disabled) {
		t.Fatal("a disabled schedule fired")
	}

	noDays := weekdaySchedule("Africa/Lagos")
	noDays.DaysOfWeek = nil
	if win(noDays) {
		t.Fatal("a schedule with no days fired")
	}

	// An unparseable timezone must be skipped, never silently treated as UTC:
	// that would fire at the wrong hour for most of the world.
	badTZ := weekdaySchedule("Mars/Olympus")
	if win(badTZ) {
		t.Fatal("a schedule with an invalid timezone fired")
	}

	badTime := weekdaySchedule("Africa/Lagos")
	badTime.Time = "25:99"
	if win(badTime) {
		t.Fatal("a schedule with an invalid time fired")
	}
}

func TestParseTime(t *testing.T) {
	h, m, err := ParseTime("06:05")
	if err != nil || h != 6 || m != 5 {
		t.Fatalf("ParseTime(06:05) = %d,%d,%v", h, m, err)
	}
	for _, bad := range []string{"", "6", "6:00:00", "25:00", "06:60", "aa:bb", "-1:00"} {
		if _, _, err := ParseTime(bad); err == nil {
			t.Fatalf("ParseTime(%q) accepted an invalid time", bad)
		}
	}
}

func TestParseDaysIgnoresGarbage(t *testing.T) {
	got := ParseDays("1,2, 3 ,,9,0,abc,7")
	want := []int{1, 2, 3, 7}
	if len(got) != len(want) {
		t.Fatalf("ParseDays = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseDays = %v, want %v", got, want)
		}
	}
}

// The occurrence key is the idempotency key, so identical intent must always
// produce an identical string.
func TestOccurrenceKeyIsStable(t *testing.T) {
	loc := lagos(t)
	a := time.Date(2026, 9, 2, 6, 0, 0, 0, loc)
	b := time.Date(2026, 9, 2, 6, 0, 30, 500, loc) // same minute
	if OccurrenceKey(a) != OccurrenceKey(b) {
		t.Fatal("keys differ within the same minute")
	}
	if OccurrenceKey(a) == OccurrenceKey(a.Add(time.Hour)) {
		t.Fatal("different hours produced the same key")
	}
}

func TestNextOccurrence(t *testing.T) {
	loc := lagos(t)
	s := weekdaySchedule("Africa/Lagos")

	// Friday 08:00: the next weekday firing is Monday 06:00.
	fri := time.Date(2026, 9, 4, 8, 0, 0, 0, loc)
	next := NextOccurrence(s, fri)
	if next.IsZero() {
		t.Fatal("no next occurrence found")
	}
	if next.Weekday() != time.Monday || next.Hour() != 6 {
		t.Fatalf("next = %s, want Monday 06:00", next)
	}

	// Today's time already passed must roll forward, not return the past.
	wed := time.Date(2026, 9, 2, 7, 0, 0, 0, loc)
	if n := NextOccurrence(s, wed); !n.After(wed) {
		t.Fatalf("next occurrence %s is not after %s", n, wed)
	}

	// A schedule that can never fire reports zero rather than a wrong answer.
	dead := s
	dead.Enabled = false
	if !NextOccurrence(dead, fri).IsZero() {
		t.Fatal("a disabled schedule reported a next occurrence")
	}
}

// A user in a half-hour offset zone must be served correctly.
func TestHalfHourOffsetZone(t *testing.T) {
	loc := mustLoad(t, "Asia/Kolkata") // UTC+05:30
	s := weekdaySchedule("Asia/Kolkata")

	fire := time.Date(2026, 9, 2, 6, 0, 0, 0, loc)
	key, due := Due(s, fire.Add(-time.Minute), fire)
	if !due {
		t.Fatal("schedule in a half-hour offset zone did not fire")
	}
	if key != "2026-09-02T06:00" {
		t.Fatalf("key = %q", key)
	}
}
