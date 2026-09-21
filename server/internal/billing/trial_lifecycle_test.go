package billing

import (
	"sort"
	"testing"
	"time"
)

// TestTrialTransitions holds the §36 graph (G-3). The vocabulary alone was
// never the spec's point: what a lifecycle promises is that some moves are
// impossible. "You cannot un-expire a trial" and "a purchase that arrives
// after the clock ran out still converts" must be properties of the table,
// not of every caller remembering them.

func TestTrialTransitions(t *testing.T) {
	cases := []struct {
		from, to TrialStatus
		ok       bool
		why      string
	}{
		{TrialEligible, TrialStarted, true, "the claim"},
		{TrialEligible, TrialActive, false, "no skipping the claim: the clock starts when the user asks"},
		{TrialEligible, TrialConverted, false, "a purchase without a started trial did not convert one"},
		{TrialEligible, TrialExpired, false, "an unstarted trial cannot run out"},
		{TrialStarted, TrialActive, true, "the clock running"},
		{TrialStarted, TrialExpired, true, "the clock ran out before anything read it"},
		{TrialStarted, TrialConverted, true, "the purchase that lands seconds after the claim"},
		{TrialStarted, TrialEligible, false, "a claimed offer is not returned to the shelf"},
		{TrialActive, TrialExpiring, true, "the last day"},
		{TrialActive, TrialExpired, true, "the last moment"},
		{TrialActive, TrialConverted, true, "the conversion the offer existed for"},
		{TrialActive, TrialStarted, false, "no rewinding the clock"},
		{TrialExpiring, TrialExpired, true, "the warning becomes the end"},
		{TrialExpiring, TrialConverted, true, "the upsell lands"},
		{TrialExpiring, TrialActive, false, "extending a trial from the API is a billing change, not a state move"},
		{TrialExpired, TrialConverted, true, "a late purchase is still a conversion; the trial does not un-happen"},
		{TrialExpired, TrialActive, false, "expired is the re-arm the product refuses (trial abuse)"},
		{TrialConverted, TrialActive, false, "terminal"},
		{TrialConverted, TrialExpired, false, "conversion does not un-convert when the subscription lapses; the subscription's own states carry that"},
		{TrialStatus("bogus"), TrialStarted, false, "unknown states are never valid on either side"},
		{TrialStarted, TrialStatus("bogus"), false, "a typo cannot invent an edge"},
		{TrialStatus(""), TrialStatus(""), false, "empty is not a state"},
	}
	for _, c := range cases {
		if got := ValidateTransition(c.from, c.to); got != c.ok {
			t.Errorf("ValidateTransition(%q→%q) = %v, want %v (%s)", c.from, c.to, got, c.ok, c.why)
		}
	}
}

func TestTrialTransitionTableIsClosed(t *testing.T) {
	states := TrialStatuses()
	if len(states) != 6 {
		t.Fatalf("§36 names six states, table carries %d", len(states))
	}
	for _, want := range []TrialStatus{TrialEligible, TrialStarted, TrialActive, TrialExpiring, TrialExpired, TrialConverted} {
		if _, ok := TrialTransitions[want]; !ok {
			t.Errorf("%s has no entry in the transition table", want)
		}
	}
	for from, tos := range TrialTransitions {
		for _, to := range tos {
			if !ValidTrialStatus(to) {
				t.Errorf("%s→%s names a state outside the vocabulary", from, to)
			}
			if to == from {
				t.Errorf("%s→%s: a self-transition is never a move", from, to)
			}
			// Every edge must point forward in §36's declared order — the
			// single property that makes "no rewinding" a fact of the table
			// rather than of every caller's memory.
			rank := func(s TrialStatus) int {
				for i, v := range states {
					if v == s {
						return i
					}
				}
				return -1
			}
			if rank(to) <= rank(from) {
				t.Errorf("%s→%s moves backwards or sideways in the declared order", from, to)
			}
		}
	}
	if len(TrialTransitions[TrialConverted]) != 0 {
		t.Error("converted must be terminal")
	}
	if !IsTerminal(TrialConverted) {
		t.Error("IsTerminal(converted) must be true")
	}
	if IsTerminal(TrialExpired) {
		t.Error("expired is not terminal: a late receipt still converts")
	}
}

func TestTrialAllowedFrom(t *testing.T) {
	allowed := TrialAllowedFrom(TrialEligible)
	if len(allowed) != 1 || allowed[0] != TrialStarted {
		t.Errorf("eligible→ only started, got %v", allowed)
	}
	active := TrialAllowedFrom(TrialActive)
	if len(active) != 3 {
		t.Errorf("active allows expiring/expired/converted, got %v", active)
	}
	sorted := append([]TrialStatus(nil), active...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	for i := range active {
		if active[i] != sorted[i] {
			t.Fatalf("TrialAllowedFrom must return a sorted list for stable 409 bodies: %v", active)
		}
	}
}

func TestTrialClockMath(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(TrialDuration)
	if TrialDuration != 7*24*time.Hour {
		t.Errorf("§36's trial is seven days, got %v", TrialDuration)
	}
	if want := end.Add(-24 * time.Hour); !TrialExpiringAt(end).Equal(want) {
		t.Errorf("expiring at %v, want %v", TrialExpiringAt(end), want)
	}
	day := func(at time.Time) int { return TrialDayWithin(start, end, at) }
	cases := []struct {
		at   time.Time
		want int
	}{
		{start.Add(-time.Second), 0}, // before the clock: no day
		{start, 1},
		{start.Add(24*time.Hour - time.Second), 1},
		{start.Add(24 * time.Hour), 2},
		{start.Add(6 * 24 * time.Hour), 7},
		{end.Add(-time.Second), 7},
		{end, 0}, // past the wire: the journey is over, not frozen at 7
		{end.Add(time.Hour), 0},
	}
	for _, c := range cases {
		if got := day(c.at); got != c.want {
			t.Errorf("day at %v = %d, want %d", c.at, got, c.want)
		}
	}
	// A record without a set clock has no day, whatever the wall clock says.
	if TrialDayWithin(time.Time{}, end, start) != 0 || TrialDayWithin(start, time.Time{}, start) != 0 {
		t.Error("unset bounds must answer 0")
	}
}
