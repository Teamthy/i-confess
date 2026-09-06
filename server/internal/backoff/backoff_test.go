package backoff

import (
	"testing"
	"time"
)

func TestDoublesAndCaps(t *testing.T) {
	p := New(30*time.Second, 30*time.Minute)
	want := []time.Duration{
		30 * time.Second,
		60 * time.Second,
		2 * time.Minute,
		4 * time.Minute,
	}
	for i, w := range want {
		if got := p.Next(i + 1); got != w {
			t.Errorf("Next(%d) = %v, want %v", i+1, got, w)
		}
	}
	if got := p.Next(50); got != 30*time.Minute {
		t.Errorf("Next(50) = %v, want the 30m cap", got)
	}
}

func TestAttemptBelowOneIsTheFirstDelay(t *testing.T) {
	p := New(5*time.Second, 5*time.Minute)
	if got := p.Next(0); got != 5*time.Second {
		t.Errorf("Next(0) = %v, want the base delay rather than something negative", got)
	}
	if got := p.Next(-7); got != 5*time.Second {
		t.Errorf("Next(-7) = %v, want the base delay", got)
	}
}

// TestLargeAttemptCannotOverflow is the reason the shift is capped rather than
// the product: an overflowed duration is negative, and a negative delay reads
// as "retry immediately" - a retry storm against whatever just failed.
func TestLargeAttemptCannotOverflow(t *testing.T) {
	p := New(time.Second, 0)
	for _, a := range []int{63, 64, 100, 1 << 20} {
		if got := p.Next(a); got <= 0 {
			t.Errorf("Next(%d) = %v; must never be zero or negative", a, got)
		}
	}
}

func TestNoCapStillTerminates(t *testing.T) {
	p := New(time.Second, 0)
	if got := p.Next(40); got <= 0 {
		t.Errorf("uncapped Next(40) = %v", got)
	}
}

func TestZeroBaseMeansNoDelay(t *testing.T) {
	if got := (Policy{}).Next(3); got != 0 {
		t.Errorf("empty policy Next(3) = %v, want 0", got)
	}
}

func TestJitterStaysWithinBand(t *testing.T) {
	base := 10 * time.Second
	p := New(base, time.Hour).WithJitter(0.5, nil)
	lo, hi := base/2, base*3/2
	for i := 0; i < 200; i++ {
		got := p.Next(1)
		if got < lo || got > hi {
			t.Fatalf("jittered delay %v outside [%v, %v]", got, lo, hi)
		}
	}
}

// TestJitterActuallyVaries guards against a policy that claims to jitter and
// does not - the whole point is that a fleet does not return in lockstep.
func TestJitterActuallyVaries(t *testing.T) {
	p := New(10*time.Second, time.Hour).WithJitter(0.5, nil)
	seen := map[time.Duration]bool{}
	for i := 0; i < 50; i++ {
		seen[p.Next(1)] = true
	}
	if len(seen) < 2 {
		t.Errorf("50 jittered draws produced %d distinct value(s)", len(seen))
	}
}

func TestInjectedSourceIsUsed(t *testing.T) {
	// A source pinned at 0.0 pulls the delay to the bottom of the band.
	p := New(10*time.Second, time.Hour).WithJitter(0.5, func() float64 { return 0 })
	if got := p.Next(1); got != 5*time.Second {
		t.Errorf("with rand()=0, Next(1) = %v, want 5s", got)
	}
	// Pinned at 1.0 pushes it to the top.
	p = New(10*time.Second, time.Hour).WithJitter(0.5, func() float64 { return 1 })
	if got := p.Next(1); got != 15*time.Second {
		t.Errorf("with rand()=1, Next(1) = %v, want 15s", got)
	}
}

func TestJitterFractionIsClamped(t *testing.T) {
	p := New(10*time.Second, time.Hour).WithJitter(9, func() float64 { return 1 })
	if got := p.Next(1); got != 20*time.Second {
		t.Errorf("clamped jitter with rand()=1 gave %v, want 20s (100%% of base)", got)
	}
	if q := New(10*time.Second, time.Hour).WithJitter(-3, nil); q.Jitter != 0 {
		t.Errorf("negative jitter fraction = %v, want 0", q.Jitter)
	}
}

func TestJitterNeverExceedsMax(t *testing.T) {
	p := New(20*time.Minute, 30*time.Minute).WithJitter(1, func() float64 { return 1 })
	if got := p.Next(1); got != 30*time.Minute {
		t.Errorf("jitter pushed past the cap: %v", got)
	}
}

func TestSchedule(t *testing.T) {
	got := New(time.Second, 4*time.Second).Schedule(4)
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second}
	if len(got) != len(want) {
		t.Fatalf("Schedule(4) returned %d entries", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Schedule[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if n := len(New(time.Second, 0).Schedule(-2)); n != 0 {
		t.Errorf("Schedule(-2) returned %d entries, want 0", n)
	}
}
