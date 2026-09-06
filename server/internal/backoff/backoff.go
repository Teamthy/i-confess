// Package backoff computes how long to wait before retrying an operation.
//
// It exists as its own leaf package because three separate implementations had
// grown: internal/email had a 5s-based doubling, internal/voice had an
// identical 30s-based one that nothing called, and internal/jobs had a fixed
// delay that was not exponential at all. A retry schedule is a policy, and
// three policies for one question is how a queue ends up hammering a provider
// that is already down.
package backoff

import (
	"math"
	"math/rand"
	"time"
)

// Policy is an exponential retry schedule.
//
// Next(attempt) returns Base * 2^(attempt-1), capped at Max. Jitter, when set,
// spreads the delay by up to that fraction of its own value, so a fleet of
// workers that all failed at the same moment does not all come back at the same
// moment.
type Policy struct {
	// Base is the delay before the first retry. Must be > 0.
	Base time.Duration
	// Max caps the delay. Zero means no cap, which is almost never what you
	// want for a provider call.
	Max time.Duration
	// Jitter is the fraction of the delay that may be randomised, 0 to 1.
	// Zero gives a fully deterministic schedule, which is what tests want.
	Jitter float64

	rnd func() float64
}

// New returns a policy doubling from base up to max, with no jitter.
func New(base, max time.Duration) Policy {
	return Policy{Base: base, Max: max}
}

// WithJitter returns a copy of the policy that randomises up to frac of each
// delay, drawing from the supplied source. A nil source uses math/rand.
func (p Policy) WithJitter(frac float64, src func() float64) Policy {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	p.Jitter = frac
	p.rnd = src
	return p
}

// Next returns the delay before the given attempt, which is 1-based: Next(1) is
// the wait before the first retry, after the initial failure.
//
// Attempts below 1 are treated as 1 rather than producing a negative or
// fractional delay, and the doubling is bounded so a large attempt count cannot
// overflow into a negative duration - which would mean "retry immediately",
// forever.
func (p Policy) Next(attempt int) time.Duration {
	if p.Base <= 0 {
		return 0
	}
	if attempt < 1 {
		attempt = 1
	}
	// Cap the shift, then check for overflow *before* multiplying: once the
	// product has wrapped, dividing it back out cannot be trusted to reveal
	// that it did. An overflowed duration is negative, which reads as
	// "retry immediately" - a retry storm against whatever just failed.
	const maxShift = 62
	shift := attempt - 1
	if shift > maxShift {
		shift = maxShift
	}
	mult := int64(1) << uint(shift)
	var d time.Duration
	if mult > int64(math.MaxInt64)/int64(p.Base) {
		d = time.Duration(math.MaxInt64)
	} else {
		d = time.Duration(mult) * p.Base
	}
	if p.Max > 0 && d > p.Max {
		d = p.Max
	}
	if p.Jitter > 0 && d > 0 {
		r := p.rnd
		if r == nil {
			r = rand.Float64
		}
		// Centre the jitter on the computed delay: it may come out up to
		// Jitter*d earlier or later, never more than Max.
		spread := float64(d) * p.Jitter
		d = time.Duration(float64(d) + (r()*2-1)*spread)
		if p.Max > 0 && d > p.Max {
			d = p.Max
		}
		if d < 0 {
			d = 0
		}
	}
	return d
}

// Schedule returns the delays for the first n retries, which is what a
// configuration surface or a log line wants to show.
func (p Policy) Schedule(n int) []time.Duration {
	if n < 0 {
		n = 0
	}
	out := make([]time.Duration, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, p.Next(i))
	}
	return out
}
