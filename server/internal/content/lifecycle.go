// Package content owns the editorial content lifecycle: the states a
// confession moves through between being written and being available to users.
//
// It exists because that lifecycle previously lived in three places that
// disagreed:
//
//  1. A schema comment documenting eight states.
//  2. A Go validator in the admin handler accepting those eight.
//  3. A CHECK constraint added in PHASE 07 allowing five, of which only three
//     were in the documented eight.
//
// The consequence was concrete and reachable: five of the eight governance
// states could not be persisted at all. An administrator moving a confession to
// `theological_review` passed validation in Go and was then refused by the
// database, surfacing as a 500 "failed to update confession". The governance
// workflow the directive describes in §22 — content review, theological
// review, audio QA — was unimplementable, and nothing in either test suite
// noticed because the validator was tested against itself and the schema tests
// counted constraints instead of reading them.
//
// This package is the single authority. The CHECK constraint in
// migrations/0003_content_lifecycle.sql is generated from the same list, and
// lifecycle_test.go reads the live constraint back out of PostgreSQL and
// compares it to All(). If the two drift, a test fails rather than a 500.
package content

// Status is one position in the editorial lifecycle.
type Status string

// The editorial lifecycle, in the order content normally moves through it.
//
// Order is informational — it documents intent and drives nothing. This phase
// deliberately does not enforce a transition graph: adding one without a
// workflow engine behind it would mean rejecting legitimate corrections
// (reopening a published confession to fix a typo) and the rules would be
// argued about in a code review rather than derived from how editors actually
// work. The vocabulary is enforced; the path through it is not yet.
const (
	// StatusDraft is written but not yet reviewed.
	StatusDraft Status = "draft"

	// StatusContentReview is with an editor: wording, length, tone.
	StatusContentReview Status = "content_review"

	// StatusTheologicalReview is with a theological reviewer. Directive §22
	// makes this mandatory before publication; it is the state that was
	// previously impossible to write.
	StatusTheologicalReview Status = "theological_review"

	// StatusAudioProduction has approved text and is being voiced.
	StatusAudioProduction Status = "audio_production"

	// StatusAudioQA is recorded and being checked against the text.
	StatusAudioQA Status = "audio_qa"

	// StatusApproved has cleared every review and may be published.
	StatusApproved Status = "approved"

	// StatusPublished is live and available to users.
	StatusPublished Status = "published"

	// StatusArchived is withdrawn. Not served to anyone, and not eligible for
	// new sessions. The content is still correct; it is simply no longer
	// offered.
	StatusArchived Status = "archived"

	// StatusDeprecated closes gap G-6, and is deliberately distinct from
	// StatusArchived.
	//
	// Deprecated content is still playable and still appears in sessions that
	// already contain it, because §9 requires that a created session's queue
	// never mutates when content changes. It is simply never offered to a new
	// session.
	//
	// That is the whole distinction, and it is load-bearing: archiving a
	// confession that a user has a scheduled session containing would either
	// silently rewrite their queue or break their next playback. Deprecating
	// it does neither.
	StatusDeprecated Status = "deprecated"
)

// lifecycle is the ordered list. All() and Valid() are derived from it so
// there is one place to change.
var lifecycle = []Status{
	StatusDraft,
	StatusContentReview,
	StatusTheologicalReview,
	StatusAudioProduction,
	StatusAudioQA,
	StatusApproved,
	StatusPublished,
	StatusArchived,
	StatusDeprecated,
}

// All returns every status the lifecycle contains, in order.
func All() []Status {
	out := make([]Status, len(lifecycle))
	copy(out, lifecycle)
	return out
}

// Valid reports whether s is a status in the lifecycle. It is the check every
// write path must make before touching the database; the CHECK constraint is
// the backstop, not the first line.
func Valid(s string) bool {
	for _, v := range lifecycle {
		if string(v) == s {
			return true
		}
	}
	return false
}

// IsServedToNewSessions reports whether a confession in this status may be
// offered when a user builds a session. Deprecated and archived content is
// excluded here while remaining playable in sessions that already hold it.
func IsServedToNewSessions(s string) bool {
	return s == string(StatusPublished)
}

// ServedStatuses returns every status IsServedToNewSessions admits. The store
// builds its WHERE clause from this rather than hardcoding 'published', so the
// query and the rule cannot disagree.
func ServedStatuses() []string {
	var out []string
	for _, st := range lifecycle {
		if IsServedToNewSessions(string(st)) {
			out = append(out, string(st))
		}
	}
	return out
}
