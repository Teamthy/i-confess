package trial

import (
	"strings"
	"testing"
	"time"
)

// TestTrialTransitions is the named state-machine test. It enumerates the
// explicit forward edge table and proves that every reverse, skipped and
// self-transition is rejected.
func TestTrialTransitions(t *testing.T) {
	allowed := map[[2]State]bool{
		{Eligible, Started}:   true,
		{Started, Active}:     true,
		{Active, Expiring}:    true,
		{Active, Converted}:   true,
		{Expiring, Expired}:   true,
		{Expiring, Converted}: true,
	}
	for _, from := range All() {
		for _, to := range All() {
			want := allowed[[2]State{from, to}]
			got := CanTransition(from, to)
			if got != want {
				t.Errorf("CanTransition(%s,%s) = %v, want %v", from, to, got, want)
			}
			if err := Transition(from, to); (err == nil) != want {
				t.Errorf("Transition(%s,%s) error=%v, want allowed=%v", from, to, err, want)
			}
		}
	}
}

// TestTrialLifecycle proves the time-derived part of the graph does not skip
// the EXPIRING edge, and that terminal states stay terminal.
func TestTrialLifecycle(t *testing.T) {
	start := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	expires := start.Add(TrialDuration)

	cases := []struct {
		name  string
		state State
		now   time.Time
		want  State
	}{
		{"eligible never starts from a clock", Eligible, start.Add(48 * time.Hour), Eligible},
		{"started activates", Started, start, Active},
		{"active remains active", Active, start.Add(5 * 24 * time.Hour), Active},
		{"active enters expiring", Active, expires.Add(-24 * time.Hour), Expiring},
		{"active does not jump terminal", Active, expires.Add(time.Minute), Expired},
		{"expiring remains until expiry", Expiring, expires.Add(-time.Minute), Expiring},
		{"expiring expires", Expiring, expires, Expired},
		{"converted is terminal", Converted, expires.Add(24 * time.Hour), Converted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StateAt(tc.state, start, expires, tc.now); got != tc.want {
				t.Errorf("StateAt(%s) = %s, want %s", tc.state, got, tc.want)
			}
		})
	}

	if got := DayFor(start, expires, start); got != 1 {
		t.Errorf("day at start = %d, want 1", got)
	}
	if got := DayFor(start, expires, start.Add(6*24*time.Hour)); got != 7 {
		t.Errorf("day six days in = %d, want 7", got)
	}
	if got := DayFor(start, expires, expires); got != 0 {
		t.Errorf("day at expiry = %d, want 0", got)
	}

	if err := Transition(Active, Expired); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("skipped edge error = %v, want an explicit rejection", err)
	}
}
