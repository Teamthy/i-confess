package engine

import (
	"errors"
	"math/rand"
	"testing"
)

func sum(n int, c []int) int {
	t := 0
	for i := 0; i < n && i < len(c); i++ {
		t += c[i]
	}
	return t
}

func TestPlanUnderNeverExceedsTarget(t *testing.T) {
	cases := []struct {
		name       string
		target     int
		candidates []int
		wantN      int
	}{
		{"exact fill", 600, []int{300, 300, 300}, 2},
		{"stops before overflow", 600, []int{400, 400}, 1},
		{"takes everything available", 1000, []int{100, 100}, 2},
		{"single item fits", 600, []int{300}, 1},
		{"greedy prefix, not subset sum", 100, []int{60, 60, 10}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, err := Plan(c.target, c.candidates, StrategyUnder)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			if n != c.wantN {
				t.Fatalf("n = %d, want %d", n, c.wantN)
			}
			if got := sum(n, c.candidates); got > c.target {
				t.Errorf("UNDER produced %d seconds against a target of %d", got, c.target)
			}
		})
	}
}

func TestPlanUnderErrorsWhenNothingFits(t *testing.T) {
	// The shortest available confession is longer than the whole budget.
	// Truncating is not an option, so this must fail loudly.
	if _, err := Plan(30, []int{120, 300}, StrategyUnder); !errors.Is(err, ErrNoFit) {
		t.Errorf("err = %v, want ErrNoFit", err)
	}
	if _, err := Plan(30, nil, StrategyUnder); !errors.Is(err, ErrNoFit) {
		t.Errorf("empty candidates: err = %v, want ErrNoFit", err)
	}
}

func TestPlanOverAlwaysCoversTarget(t *testing.T) {
	n, err := Plan(600, []int{400, 400, 400}, StrategyOver)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if n != 2 {
		t.Fatalf("n = %d, want 2 (800s is the shortest prefix covering 600s)", n)
	}
	if got := sum(n, []int{400, 400, 400}); got < 600 {
		t.Errorf("OVER produced %d seconds, below the 600s target", got)
	}

	// Already covered by the first item: no padding.
	if n, _ := Plan(100, []int{400}, StrategyOver); n != 1 {
		t.Errorf("n = %d, want 1", n)
	}

	// Content too thin to ever reach the target.
	if _, err := Plan(5000, []int{60, 60}, StrategyOver); !errors.Is(err, ErrUnreachable) {
		t.Errorf("err = %v, want ErrUnreachable", err)
	}
}

func TestPlanExact(t *testing.T) {
	if n, err := Plan(600, []int{300, 300, 90}, StrategyExact); err != nil || n != 2 {
		t.Errorf("n = %d, err = %v; want n=2, no error", n, err)
	}
	if n, err := Plan(600, []int{300, 250, 90}, StrategyExact); !errors.Is(err, ErrNoExactFit) {
		t.Errorf("n = %d, err = %v; want ErrNoExactFit", n, err)
	}
	// A single item landing exactly.
	if n, err := Plan(300, []int{300}, StrategyExact); err != nil || n != 1 {
		t.Errorf("n = %d, err = %v; want n=1", n, err)
	}
}

func TestPlanClosestPicksMinimalDistance(t *testing.T) {
	cases := []struct {
		name       string
		target     int
		candidates []int
		wantN      int
		wantTotal  int
	}{
		{"prefers over-fill when nearer", 600, []int{400, 300}, 2, 700},
		{"prefers under-fill when nearer", 600, []int{550, 300}, 1, 550},
		{"takes three when that is nearest", 900, []int{300, 300, 300, 300}, 3, 900},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, err := Plan(c.target, c.candidates, StrategyClosest)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			if n != c.wantN {
				t.Fatalf("n = %d, want %d", n, c.wantN)
			}
			if got := sum(n, c.candidates); got != c.wantTotal {
				t.Errorf("total = %d, want %d", got, c.wantTotal)
			}
		})
	}
}

func TestPlanClosestNeverReturnsAnEmptySession(t *testing.T) {
	// Zero items is literally closest to a 10s target when every confession
	// is 5 minutes long, but an empty session is never what the user wanted.
	n, err := Plan(10, []int{300, 300}, StrategyClosest)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if n != 1 {
		t.Errorf("n = %d, want 1", n)
	}
}

func TestPlanTiesGoToTheShorterSession(t *testing.T) {
	// 500 under (100 short) vs 700 over (100 long): equidistant. Finishing
	// early is less disruptive than running long, so the shorter prefix wins.
	n, err := Plan(600, []int{500, 200}, StrategyClosest)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if n != 1 {
		t.Errorf("n = %d, want 1 (tie must favour the shorter session)", n)
	}
}

func TestPlanBalancedCapsOvershootAtOneItem(t *testing.T) {
	// Target 600. Under-fill 400 (200 short) vs over-fill 1000 (400 long).
	// The next item is 600s, so the cap is 600+600=1200 and 1000 is allowed;
	// but 400 is nearer, so BALANCED takes one item.
	n, err := Plan(600, []int{400, 600}, StrategyBalanced)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if n != 1 {
		t.Errorf("n = %d, want 1", n)
	}

	// Here the over-fill is strictly nearer and within one item of the
	// target, so BALANCED accepts the overshoot where UNDER would not.
	n, err = Plan(600, []int{500, 120}, StrategyBalanced)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if n != 2 {
		t.Errorf("n = %d, want 2 (620s is 20s over, closer than the 100s under-fill)", n)
	}
}

func TestPlanBalancedOvershootNeverExceedsLongestItem(t *testing.T) {
	// Property check: whatever the inputs, BALANCED must not overshoot the
	// target by more than the single longest candidate. Anything more would
	// mean the user was served padding they did not ask for.
	rng := rand.New(rand.NewSource(20260905))
	for i := 0; i < 2000; i++ {
		count := 1 + rng.Intn(8)
		cands := make([]int, count)
		longest := 0
		for j := range cands {
			cands[j] = 30 + rng.Intn(600)
			if cands[j] > longest {
				longest = cands[j]
			}
		}
		target := 60 + rng.Intn(3600)

		n, err := Plan(target, cands, StrategyBalanced)
		if err != nil {
			t.Fatalf("Plan(target=%d, cands=%v): %v", target, cands, err)
		}
		if n < 1 || n > len(cands) {
			t.Fatalf("n = %d out of range for %d candidates", n, len(cands))
		}
		total := sum(n, cands)
		if over := total - target; over > longest {
			t.Fatalf("overshoot %d exceeds longest item %d (target=%d total=%d cands=%v)",
				over, longest, target, total, cands)
		}
	}
}

func TestPlanStrategiesHoldTheirContracts(t *testing.T) {
	// The three strategies with hard guarantees are checked across many
	// random inputs, because a hand-picked table can miss an off-by-one.
	rng := rand.New(rand.NewSource(4242))
	for i := 0; i < 3000; i++ {
		count := 1 + rng.Intn(10)
		cands := make([]int, count)
		for j := range cands {
			cands[j] = 30 + rng.Intn(900)
		}
		target := 60 + rng.Intn(5400)

		if n, err := Plan(target, cands, StrategyUnder); err == nil {
			if got := sum(n, cands); got > target {
				t.Fatalf("UNDER exceeded target: total=%d target=%d cands=%v", got, target, cands)
			}
		} else if !errors.Is(err, ErrNoFit) {
			t.Fatalf("UNDER unexpected error: %v", err)
		}

		if n, err := Plan(target, cands, StrategyOver); err == nil {
			if got := sum(n, cands); got < target {
				t.Fatalf("OVER fell short: total=%d target=%d cands=%v", got, target, cands)
			}
		} else if !errors.Is(err, ErrUnreachable) {
			t.Fatalf("OVER unexpected error: %v", err)
		}

		if n, err := Plan(target, cands, StrategyExact); err == nil {
			if got := sum(n, cands); got != target {
				t.Fatalf("EXACT missed: total=%d target=%d", got, target)
			}
		} else if !errors.Is(err, ErrNoExactFit) {
			t.Fatalf("EXACT unexpected error: %v", err)
		}
	}
}

func TestPlanIsDeterministic(t *testing.T) {
	// Sessions must be reproducible: the same request builds the same queue.
	cands := []int{137, 211, 89, 340, 60, 60}
	for _, s := range Strategies() {
		first, err1 := Plan(600, cands, s)
		second, err2 := Plan(600, cands, s)
		if first != second || (err1 == nil) != (err2 == nil) {
			t.Errorf("%s: repeated calls disagreed (%d/%v vs %d/%v)", s, first, err1, second, err2)
		}
	}
}

func TestPlanRejectsBadInput(t *testing.T) {
	if _, err := Plan(0, []int{60}, StrategyBalanced); err == nil {
		t.Error("zero target accepted")
	}
	if _, err := Plan(-100, []int{60}, StrategyBalanced); err == nil {
		t.Error("negative target accepted")
	}
	if _, err := Plan(600, []int{60, 0}, StrategyBalanced); err == nil {
		t.Error("zero-duration candidate accepted")
	}
	if _, err := Plan(600, []int{60, -5}, StrategyBalanced); err == nil {
		t.Error("negative-duration candidate accepted")
	}
}

func TestPlanAcceptsUnspecifiedStrategyAsDefault(t *testing.T) {
	// An empty strategy is a normal request, not a malformed one.
	fromEmpty, err := Plan(600, []int{550, 100}, "")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	fromDefault, _ := Plan(600, []int{550, 100}, DefaultStrategy)
	if fromEmpty != fromDefault {
		t.Errorf("empty strategy gave %d, default gave %d; they must agree", fromEmpty, fromDefault)
	}
}

func TestStrategyValidationAndNormalisation(t *testing.T) {
	for _, s := range []string{"EXACT", "CLOSEST", "UNDER", "OVER", "BALANCED",
		"balanced", " Balanced ", "under"} {
		if !IsValidStrategy(s) {
			t.Errorf("IsValidStrategy(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "FAST", "exactish", "1", "closest-fit", "balanced2"} {
		if IsValidStrategy(s) {
			t.Errorf("IsValidStrategy(%q) = true, want false", s)
		}
	}
	if got := NormalizeStrategy("balanced"); got != StrategyBalanced {
		t.Errorf("NormalizeStrategy(\"balanced\") = %q, want %q", got, StrategyBalanced)
	}
	if got := NormalizeStrategy("nonsense"); got != DefaultStrategy {
		t.Errorf("NormalizeStrategy(\"nonsense\") = %q, want default %q", got, DefaultStrategy)
	}
	if got := NormalizeStrategy(""); got != DefaultStrategy {
		t.Errorf("NormalizeStrategy(\"\") = %q, want default %q", got, DefaultStrategy)
	}
	if DefaultStrategy != StrategyBalanced {
		t.Errorf("DefaultStrategy = %q, want BALANCED per the product spec", DefaultStrategy)
	}
}

func TestStrategiesAreTheFiveFromTheSpec(t *testing.T) {
	got := Strategies()
	if len(got) != 5 {
		t.Fatalf("Strategies() returned %d, want 5: %v", len(got), got)
	}
	want := map[Strategy]bool{
		StrategyExact: true, StrategyClosest: true, StrategyUnder: true,
		StrategyOver: true, StrategyBalanced: true,
	}
	for _, s := range got {
		if !want[s] {
			t.Errorf("unexpected strategy %q", s)
		}
		if !IsValidStrategy(string(s)) {
			t.Errorf("Strategies() returned %q but IsValidStrategy rejects it", s)
		}
	}
}

// TestStrategyConstantsAreUntyped guards a subtle compile-time contract: these
// constants are assigned both to plain string fields and to Strategy-typed
// variables across the codebase. Typing them would break one of the two.
func TestStrategyConstantsAreUntyped(t *testing.T) {
	var asString = StrategyBalanced
	var asStrategy Strategy = StrategyBalanced
	if asString != "BALANCED" || asStrategy != "BALANCED" {
		t.Errorf("constants did not satisfy both string and Strategy: %q / %q", asString, asStrategy)
	}
}

func TestPresetsCoverTheSpecLadder(t *testing.T) {
	want := []int{600, 900, 1800, 2700, 3600, 5400, 7200, 10800}
	got := Presets()
	if len(got) != len(want)+1 {
		t.Fatalf("Presets() returned %d entries, want %d (8 fixed + custom)", len(got), len(want)+1)
	}
	for i, secs := range want {
		if got[i].Seconds != secs {
			t.Errorf("Presets()[%d].Seconds = %d, want %d", i, got[i].Seconds, secs)
		}
		if got[i].Name == "" || got[i].Label == "" {
			t.Errorf("Presets()[%d] missing Name or Label", i)
		}
	}
	last := got[len(got)-1]
	if !last.Custom || last.Seconds != 0 {
		t.Errorf("last preset = %+v, want the custom preset with no fixed duration", last)
	}
}

func TestPresetForAcceptsClientSpellings(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"30m", 1800, true},
		{"30", 1800, true},
		{"30M", 1800, true},
		{"30min", 1800, true},
		{"30MIN", 1800, true},
		{"30_min", 1800, true},
		{"30 minutes", 1800, true},
		{"30 minutes ", 1800, true},
		{"10m", 600, true},
		{"180m", 10800, true},
		{"2h", 0, false}, // not a preset name; the ladder is expressed in minutes
		{"25m", 0, false},
		{"0", 0, false},
		{"", 0, false},
		{"custom", 0, false}, // recognised, but carries no fixed duration
		{"nonsense", 0, false},
		{"m", 0, false},
		{"-30m", 0, false},
	}
	for _, c := range cases {
		got, ok := PresetFor(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("PresetFor(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestIsCustomPreset(t *testing.T) {
	for _, in := range []string{"custom", "CUSTOM", " Custom "} {
		if !IsCustomPreset(in) {
			t.Errorf("IsCustomPreset(%q) = false, want true", in)
		}
	}
	for _, in := range []string{"", "30m", "customary"} {
		if IsCustomPreset(in) {
			t.Errorf("IsCustomPreset(%q) = true, want false", in)
		}
	}
}

func TestPresetsIsDefensiveCopy(t *testing.T) {
	p := Presets()
	p[0].Seconds = 1
	if Presets()[0].Seconds == 1 {
		t.Error("Presets() returned a reference to package state")
	}
}
