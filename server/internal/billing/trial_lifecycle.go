package billing

import (
	"fmt"
	"sort"
	"time"
)

// The trial lifecycle (§36, G-3).
//
// Before this file the trial was a derived illusion: GET /subscriptions/trial
// divided the caller's age by 24h and everyone looked like they were on a
// trial, forever. §36 describes a state machine, and a state machine is a
// thing that can refuse — "you have already used your trial" was not
// expressible when the state was a subtraction.
//
// The six states and their one honest meaning each:
//
//	eligible   — no clock has run for this account; the offer stands
//	started    — the offer was claimed; the clock is set (7 days)
//	active     — the clock is running; premium is on
//	expiring   — inside the last 24h; the "your trial ends" moment
//	expired    — the clock ran out and no purchase arrived; premium off
//	converted  — a verified receipt arrived while or right after the clock ran;
//	             the offer became a subscription. Terminal.
//
// Backward moves do not exist. A store that refunds into an expired trial does
// not reopen the trial; the subscription's own state machine (§34) handles
// that, and reviving free trials on refund is how promotional abuse is
// designed by accident.

type TrialStatus string

const (
	TrialEligible  TrialStatus = "eligible"
	TrialStarted   TrialStatus = "started"
	TrialActive    TrialStatus = "active"
	TrialExpiring  TrialStatus = "expiring"
	TrialExpired   TrialStatus = "expired"
	TrialConverted TrialStatus = "converted"
)

// TrialDuration is the whole offer: seven days, matching the seven-day journey
// the trial screen lists. One clock, not two — DayFor and this constant are
// the same promise.
const TrialDuration = 7 * 24 * time.Hour

// TrialExpiringWindow is how long before the end the trial says "expiring".
// A day, because the point of the state is to precede a decision, and a
// warning twelve hours out arrives after most people's last evening.
const TrialExpiringWindow = 24 * time.Hour

// TrialTransitions is the graph. A state may only move where this table says;
// the database CHECK constrains the vocabulary, this constrains the movement,
// and the store's CAS write is where both are actually enforced.
var TrialTransitions = map[TrialStatus][]TrialStatus{
	TrialEligible: {TrialStarted},
	// A claimed trial whose clock already ran out before anything read it
	// goes straight to expired; "started→active" is the normal opening.
	// started→converted covers the purchase that lands in the seconds after
	// the claim, before any read has promoted the row to active — refusing it
	// would turn a faster payment into an error.
	TrialStarted: {TrialActive, TrialExpired, TrialConverted},
	// Conversion can arrive at any point the clock is running.
	TrialActive:    {TrialExpiring, TrialExpired, TrialConverted},
	TrialExpiring:  {TrialExpired, TrialConverted},
	TrialExpired:   {TrialConverted},
	TrialConverted: {},
}

// TrialStatuses in declaration order — the order the lifecycle walks.
func TrialStatuses() []TrialStatus {
	return []TrialStatus{TrialEligible, TrialStarted, TrialActive, TrialExpiring, TrialExpired, TrialConverted}
}

func ValidTrialStatus(s TrialStatus) bool {
	_, ok := TrialTransitions[s]
	return ok
}

// ValidateTransition reports whether from→to is an edge of the graph. An
// unknown state on either side is never valid, which keeps a vocabulary
// change from silently widening the graph.
func ValidateTransition(from, to TrialStatus) bool {
	if !ValidTrialStatus(from) || !ValidTrialStatus(to) {
		return false
	}
	for _, ok := range TrialTransitions[from] {
		if ok == to {
			return true
		}
	}
	return false
}

// IsTerminal: converted is the only state with no way out — by graph and by
// design; expired is not terminal because a late purchase still converts.
func IsTerminal(s TrialStatus) bool { return s == TrialConverted }

// TrialAllowedFrom lists where a state may move, sorted for stable
// assertions and 409 bodies.
func TrialAllowedFrom(from TrialStatus) []TrialStatus {
	out := append([]TrialStatus(nil), TrialTransitions[from]...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// TrialTransitionError names a refused move for the API: the caller learns
// where the trial stands and where it is allowed to go.
type TrialTransitionError struct {
	From TrialStatus
	To   TrialStatus
}

func (e *TrialTransitionError) Error() string {
	return fmt.Sprintf("a trial cannot move from %s to %s", e.From, e.To)
}

// TrialDayWithin maps the running clock to the journey day (1..7). Outside a
// running window it answers 0 — no day, not "day one". DayFor (age-derived)
// stays for the legacy journey endpoint; this is the record-derived answer.
func TrialDayWithin(start, end, now time.Time) int {
	if start.IsZero() || end.IsZero() || now.Before(start) || !now.Before(end) {
		return 0
	}
	day := int(now.Sub(start).Hours()/24) + 1
	if day > len(TrialJourney) {
		day = len(TrialJourney)
	}
	return day
}

// TrialExpiringAt is the moment ACTIVE becomes EXPIRING.
func TrialExpiringAt(end time.Time) time.Time {
	return end.Add(-TrialExpiringWindow)
}
