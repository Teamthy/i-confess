package models

import (
	"testing"
	"time"
)

// Entitlement is the commercial boundary of the product: it decides whether a
// paying customer hears premium audio, and it is the thing IC-003 got wrong for
// the opposite reason — the old code granted entitlement from a mutable status
// string that nothing rewrote when a period ended, so a lapsed subscription
// stayed premium forever.
//
// These tests are table-driven because the rule is a truth table over (status,
// clock), and the interesting cases are the disagreements between the two: a
// status column that says active over an expiry in the past, a cancellation that
// should still grant until the period ends, and a corrupt row that must not.
//
// It is deliberately not tested through the API. The handler, the store and the
// audio gate all call this function, so a test of the handler would only pin one
// of its three callers.

func TestSubscriptionEntitled(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	ago := now.Add(-time.Hour).Format(time.RFC3339)
	soon := now.Add(time.Hour).Format(time.RFC3339)

	cases := []struct {
		name   string
		sub    Subscription
		entitl bool
	}{
		// The paid period is live.
		{"active with time left", Subscription{Plan: "premium", Status: SubscriptionActive, EndsAt: soon}, true},
		{"trial with time left", Subscription{Plan: "premium", Status: SubscriptionTrial, EndsAt: soon}, true},
		{"grace period is still entitled", Subscription{Plan: "premium", Status: SubscriptionGrace, EndsAt: soon}, true},

		// The clock has passed the expiry. This is the case the status column
		// cannot express, because nothing rewrites it when a period lapses.
		{"active but the period ended", Subscription{Plan: "premium", Status: SubscriptionActive, EndsAt: ago}, false},
		{"trial that ended", Subscription{Plan: "premium", Status: SubscriptionTrial, EndsAt: ago}, false},
		{"grace period that ended", Subscription{Plan: "premium", Status: SubscriptionGrace, EndsAt: ago}, false},

		// Cancel means "do not renew", not "revoke what was paid for".
		{"cancelled with time left", Subscription{Plan: "premium", Status: SubscriptionCancelled, EndsAt: soon}, true},
		{"cancelled after the period", Subscription{Plan: "premium", Status: SubscriptionCancelled, EndsAt: ago}, false},
		{"cancelled with no end date", Subscription{Plan: "premium", Status: SubscriptionCancelled}, false},

		// No clock at all: an operator grant (SetSubscription), or a row written
		// before verification existed. It stays entitled.
		{"active with no expiry", Subscription{Plan: "premium", Status: SubscriptionActive}, true},
		{"trial with no expiry", Subscription{Plan: "premium", Status: SubscriptionTrial}, true},
		{"grace with no expiry", Subscription{Plan: "premium", Status: SubscriptionGrace}, true},

		// States that never entitle, whatever the date says.
		{"expired", Subscription{Plan: "premium", Status: SubscriptionExpired, EndsAt: soon}, false},
		{"refunded", Subscription{Plan: "premium", Status: SubscriptionRefunded, EndsAt: soon}, false},
		{"suspended", Subscription{Plan: "premium", Status: SubscriptionSuspended, EndsAt: soon}, false},
		{"empty status", Subscription{Plan: "premium", EndsAt: soon}, false},
		{"unknown status", Subscription{Plan: "premium", Status: "woteva", EndsAt: soon}, false},
		{"no row at all", Subscription{}, false},

		// Free is free even with a live date on it: the plan is not premium, and
		// Entitled answers the entitlement question, not the row-validity one.
		{"free with time left", Subscription{Plan: "free", Status: SubscriptionActive, EndsAt: soon}, true},

		// A corrupt expiry is a data error, not a grant without a clock. Treating
		// the two alike is how a typo in a date column becomes a permanent
		// subscription.
		{"unparsable expiry", Subscription{Plan: "premium", Status: SubscriptionActive, EndsAt: "next tuesday"}, false},
		{"whitespace expiry", Subscription{Plan: "premium", Status: SubscriptionActive, EndsAt: "   "}, true},
		{"expiry with the wrong timezone form", Subscription{Plan: "premium", Status: SubscriptionActive, EndsAt: "2027-01-01 00:00:00"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.sub.Entitled(now); got != tc.entitl {
				t.Errorf("Entitled(%s) = %v, want %v (row: status=%q ends_at=%q)",
					now.Format(time.RFC3339), got, tc.entitl, tc.sub.Status, tc.sub.EndsAt)
			}
		})
	}
}

// The boundary itself: the instant a period ends, entitlement is gone. An
// off-by-one here is a day of free premium per subscriber, or a customer cut off
// early.
func TestSubscriptionEntitlementAtTheBoundary(t *testing.T) {
	expiry := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	sub := Subscription{Plan: "premium", Status: SubscriptionActive, EndsAt: expiry.Format(time.RFC3339)}

	if sub.Entitled(expiry) {
		t.Error("entitled at the exact instant the period ends: the clock must be exclusive")
	}
	if !sub.Entitled(expiry.Add(-time.Nanosecond)) {
		t.Error("not entitled a nanosecond before the period ends")
	}
	// Expiry is parsed as an instant, so the equivalence class is the instant
	// rather than the spelling: the same moment written in another offset is
	// still the same moment.
	other := Subscription{
		Plan: "premium", Status: SubscriptionActive,
		EndsAt: expiry.Add(-time.Hour).In(time.FixedZone("WAT", 3600)).Format(time.RFC3339),
	}
	if other.Entitled(expiry) {
		t.Error("an expiry written in another timezone was not compared as an instant")
	}
}

// What a client is shown. A row that still reads active months after its period
// ended is the shape of the bug, so the client must be told expired rather than
// active - otherwise the app shows "Premium" over a free account and the support
// request is about a product that is behaving correctly.
func TestSubscriptionEffectiveStatus(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	ago := now.Add(-time.Hour).Format(time.RFC3339)
	soon := now.Add(time.Hour).Format(time.RFC3339)

	cases := []struct {
		name string
		sub  Subscription
		want string
	}{
		{"live", Subscription{Status: SubscriptionActive, EndsAt: soon}, SubscriptionActive},
		{"lapsed reads expired despite the column", Subscription{Status: SubscriptionActive, EndsAt: ago}, SubscriptionExpired},
		{"lapsed trial", Subscription{Status: SubscriptionTrial, EndsAt: ago}, SubscriptionExpired},
		{"lapsed cancellation", Subscription{Status: SubscriptionCancelled, EndsAt: ago}, SubscriptionExpired},
		{"cancelled but still paid for", Subscription{Status: SubscriptionCancelled, EndsAt: soon}, SubscriptionCancelled},
		{"refunded is reported as refunded", Subscription{Status: SubscriptionRefunded, EndsAt: soon}, SubscriptionRefunded},
		{"suspended is reported as suspended", Subscription{Status: SubscriptionSuspended, EndsAt: soon}, SubscriptionSuspended},
		{"no end date keeps its status", Subscription{Status: SubscriptionActive}, SubscriptionActive},
		{"corrupt expiry is not silently expired", Subscription{Status: SubscriptionActive, EndsAt: "soon"}, SubscriptionActive},
		{"no status is none", Subscription{EndsAt: soon}, "none"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.sub.EffectiveStatus(now); got != tc.want {
				t.Errorf("EffectiveStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

// ExpiresAt reports whether the row carries a usable expiry, which is what
// separates "no clock" from "broken clock".
func TestSubscriptionExpiresAt(t *testing.T) {
	if _, ok := (Subscription{}).ExpiresAt(); ok {
		t.Error("an empty ends_at reported a usable expiry")
	}
	if _, ok := (Subscription{EndsAt: "whenever"}).ExpiresAt(); ok {
		t.Error("an unparsable ends_at reported a usable expiry")
	}
	expiry, ok := (Subscription{EndsAt: "2026-09-20T12:00:00Z"}).ExpiresAt()
	if !ok || !expiry.Equal(time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("ExpiresAt = (%s, %v), want the parsed instant", expiry, ok)
	}
	// UTC, so a caller cannot compare a local time against a UTC one.
	if loc, _ := (Subscription{EndsAt: "2026-09-20T12:00:00+02:00"}).ExpiresAt(); loc.Location() != time.UTC {
		t.Errorf("ExpiresAt returned %s, want UTC", loc.Location())
	}
}
