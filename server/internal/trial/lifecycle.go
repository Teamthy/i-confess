// Package trial owns the account trial lifecycle.
//
// A trial is not a second subscription vocabulary. The trial records the
// customer journey, while the subscriptions row remains the entitlement
// projection consumed by the rest of the service. A running trial writes
// premium/trial once, and models.Subscription.Entitled is the only place that
// answers whether that projection is live at a particular instant.
package trial

import (
	"fmt"
	"time"
)

// State is one state in the six-state trial lifecycle from directive §36.
type State string

const (
	Eligible  State = "ELIGIBLE"
	Started   State = "STARTED"
	Active    State = "ACTIVE"
	Expiring  State = "EXPIRING"
	Expired   State = "EXPIRED"
	Converted State = "CONVERTED"
)

// All returns the complete vocabulary in lifecycle order.
func All() []State {
	return []State{Eligible, Started, Active, Expiring, Expired, Converted}
}

// forwardEdges is intentionally explicit. The trial state machine is not an
// ordinal comparison: conversion is terminal, expiry is terminal, and a
// converted trial must never be moved back to an entitled state.
var forwardEdges = map[State][]State{
	Eligible:  {Started},
	Started:   {Active},
	Active:    {Expiring, Converted},
	Expiring:  {Expired, Converted},
	Expired:   {},
	Converted: {},
}

// CanTransition reports whether the exact directed edge is part of the
// lifecycle. Keeping this separate from Transition lets callers validate a
// proposed edge without changing state.
func CanTransition(from, to State) bool {
	for _, candidate := range forwardEdges[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

// Transition applies one forward edge and rejects every other movement,
// including self-transitions. Idempotent HTTP operations handle a repeated
// request before calling this function; the state machine itself stays strict.
func Transition(from, to State) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("trial transition %s -> %s is not allowed", from, to)
	}
	return nil
}

// StateAt returns the time-derived state for a started trial. It does not
// invent a start: callers with no started_at remain ELIGIBLE. The one-day
// expiring window is deliberately explicit so API and worker callers use the
// same boundary.
func StateAt(current State, startedAt, expiresAt, now time.Time) State {
	switch current {
	case Eligible:
		return Eligible
	case Started:
		return Active
	case Active, Expiring:
		if !expiresAt.IsZero() && !now.Before(expiresAt) {
			return Expired
		}
		if !expiresAt.IsZero() && !now.Before(expiresAt.Add(-24*time.Hour)) {
			return Expiring
		}
		return Active
	case Expired, Converted:
		return current
	default:
		return current
	}
}

// TrialDuration is the catalogue default. A single constant keeps the API,
// persistence and tests from quietly disagreeing about what "7-day trial"
// means.
const TrialDuration = 7 * 24 * time.Hour

// DayFor maps a trial start to the deterministic journey day. Day seven lasts
// until expiry; after expiry the status endpoint reports zero because there is
// no current journey day.
func DayFor(start, expires, now time.Time) int {
	if start.IsZero() || expires.IsZero() || now.Before(start) || !now.Before(expires) {
		return 0
	}
	day := int(now.Sub(start)/(24*time.Hour)) + 1
	if day < 1 || day > 7 {
		return 0
	}
	return day
}
