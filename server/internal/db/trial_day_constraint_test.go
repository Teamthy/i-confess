package db_test

import (
	"context"
	"regexp"
	"strconv"
	"testing"

	"github.com/Teamthy/i-confess/internal/billing"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// boundRe reads the two integers out of a BETWEEN-derived CHECK. PostgreSQL
// renders `day BETWEEN 1 AND 7` as `((day >= 1) AND (day <= 7))`, so the
// definition carries the bounds rather than a value list and the value-list
// parser used for the status vocabularies cannot read it.
var boundRe = regexp.MustCompile(`day\s*(>=|<=)\s*(\d+)`)

// TestTrialDayRangeMatchesConstraint is the PHASE 36/37 parity pattern applied
// to a range rather than a vocabulary: compare what the Go journey table
// declares with the CHECK installed in the live PostgreSQL database.
//
// The failure this catches is a lengthened journey. Adding an eighth day to
// billing.TrialJourney without a migration would let the store attempt an
// eighth completion and be rejected by the database — a 500 on a listener's
// last day of trial, at the exact moment the product is asking them to pay.
func TestTrialDayRangeMatchesConstraint(t *testing.T) {
	raw := dbtest.Raw(t)
	def, err := checkDefinition(context.Background(), raw, "trial_day_completions", "day")
	if err != nil {
		t.Fatalf("trial_day_completions.day: %v", err)
	}
	if def == "" {
		t.Fatal("trial_day_completions.day has no CHECK constraint in the live schema")
	}

	got := map[string]int{}
	for _, m := range boundRe.FindAllStringSubmatch(def, -1) {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatalf("constraint bound %q is not an integer: %v", m[2], err)
		}
		got[m[1]] = n
	}
	lower, hasLower := got[">="]
	upper, hasUpper := got["<="]
	if !hasLower || !hasUpper {
		t.Fatalf("could not read both bounds from %q", def)
	}

	wantDays := len(billing.TrialJourney)
	if lower != 1 {
		t.Errorf("CHECK lower bound = %d, want 1 (the first journey day)", lower)
	}
	if upper != wantDays {
		t.Errorf("CHECK upper bound = %d, but the journey has %d days; a mismatch means a completion can be rejected by the database", upper, wantDays)
	}
	if len(billing.JourneyIntents) != wantDays {
		t.Errorf("JourneyIntents has %d entries but the journey has %d days", len(billing.JourneyIntents), wantDays)
	}
}

// TestTrialDayCompletionsAreUniquePerUserAndDay proves the uniqueness that makes
// a completion rate a rate. Without it, finishing three sessions on one day
// would report three days of a seven-day journey.
func TestTrialDayCompletionsAreUniquePerUserAndDay(t *testing.T) {
	raw := dbtest.Raw(t)
	var n int
	if err := raw.QueryRowContext(context.Background(), `
		SELECT count(*)
		FROM pg_constraint
		WHERE conrelid = 'trial_day_completions'::regclass
		  AND contype = 'u'
		  AND pg_get_constraintdef(oid) ILIKE '%user_id%'
		  AND pg_get_constraintdef(oid) ILIKE '%day%'`).Scan(&n); err != nil {
		t.Fatalf("query constraints: %v", err)
	}
	if n != 1 {
		t.Errorf("found %d unique constraints over (user_id, day), want exactly 1", n)
	}
}
