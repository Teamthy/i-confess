// Package moderation owns the ugc moderation lifecycle and the §75 audio QA
// gate: the vocabularies, the legal transitions, and the checklist logic.
//
// The schema has carried moderation tables since the baseline
// (moderation_cases, reports, content_moderation_history) and a full status
// column on user_confessions since PHASE 07, but until PHASE 31 no Go code
// read or wrote any of it: three admin routes answered 501, a user could not
// submit a confession for review, and nobody could file a report. This
// package is the single authority for that surface, the way
// internal/content/lifecycle.go is the authority for the editorial lifecycle.
// The test suite reads the live CHECK constraints back out of PostgreSQL and
// compares them against the lists here, so the code and the schema cannot
// drift apart silently.
package moderation

import "fmt"

// UGCStatus is one position in the user-confession moderation lifecycle.
type UGCStatus string

const (
	// UGCDraft is written but not offered for review. Only the author sees it.
	UGCDraft UGCStatus = "draft"
	// UGCSubmitted is in the moderation queue awaiting a decision.
	UGCSubmitted UGCStatus = "submitted"
	// UGCApproved cleared review and stays in the author's own surfaces
	// (the author did not ask for a public audience).
	UGCApproved UGCStatus = "approved"
	// UGCRejected failed review; Reason is always recorded with it.
	UGCRejected UGCStatus = "rejected"
	// UGCPublished cleared review with visibility=public and is cleared for
	// the public stream (§22).
	UGCPublished UGCStatus = "published"
	// UGCArchived is withdrawn by a moderator or the author.
	UGCArchived UGCStatus = "archived"
)

// ugcLifecycle mirrors user_confessions_status_check in
// migrations/0002_status_constraints.sql.
var ugcLifecycle = []UGCStatus{
	UGCDraft, UGCSubmitted, UGCApproved, UGCRejected, UGCPublished, UGCArchived,
}

// UGCStatuses returns every status the UGC lifecycle admits.
func UGCStatuses() []string {
	out := make([]string, len(ugcLifecycle))
	for i, s := range ugcLifecycle {
		out[i] = string(s)
	}
	return out
}

// ValidUGCStatus reports whether s is in the UGC lifecycle.
func ValidUGCStatus(s string) bool {
	for _, v := range ugcLifecycle {
		if string(v) == s {
			return true
		}
	}
	return false
}

// ugcTransitions is the enforced graph for the moves PHASE 31 implements.
// draft and rejected may be (re)submitted; submitted may be approved,
// rejected or withdrawn; approved may be archived. Everything else is
// refused: a review decision is final except through an explicit
// resubmission, which re-enters the queue.
var ugcTransitions = map[UGCStatus][]UGCStatus{
	UGCDraft:     {UGCSubmitted, UGCArchived},
	UGCSubmitted: {UGCApproved, UGCRejected, UGCPublished, UGCArchived},
	UGCRejected:  {UGCSubmitted, UGCArchived},
	UGCApproved:  {UGCArchived},
	UGCPublished: {UGCArchived},
	UGCArchived:  {},
}

// UGCAllowedFrom lists the statuses a user confession may move to from from.
func UGCAllowedFrom(from UGCStatus) []UGCStatus {
	return ugcTransitions[from]
}

// CanSubmit reports whether a confession in from may be submitted for review.
func CanSubmit(from UGCStatus) bool {
	for _, to := range ugcTransitions[from] {
		if to == UGCSubmitted {
			return true
		}
	}
	return false
}

// CanReview reports whether a confession in from may receive a review
// decision. Only a queued confession may: deciding on a draft would hide the
// author's intent to submit, and re-deciding a decided one rewrites history.
func CanReview(from UGCStatus) bool {
	return from == UGCSubmitted
}

// UGCTransitionError explains a refused UGC move in terms the API can return.
type UGCTransitionError struct {
	From, To UGCStatus
}

func (e *UGCTransitionError) Error() string {
	return fmt.Sprintf("cannot move a user confession from %q to %q", e.From, e.To)
}

// ValidateUGCTransition returns a *UGCTransitionError when the move is not in
// the enforced graph.
func ValidateUGCTransition(from, to UGCStatus) error {
	for _, allowed := range ugcTransitions[from] {
		if allowed == to {
			return nil
		}
	}
	return &UGCTransitionError{From: from, To: to}
}

// UGC visibility. §22 names PRIVATE/SHARED/PUBLIC for UGC; the collections
// surface uses private/unlisted/public (models.Visibility*), a different
// vocabulary for a different entity. The CHECK constraint added by
// migrations/0009_moderation.sql matches this list and the parity test reads
// it back.
const (
	VisibilityPrivate = "private"
	VisibilityShared  = "shared"
	VisibilityPublic  = "public"
)

var ugcVisibilities = []string{VisibilityPrivate, VisibilityShared, VisibilityPublic}

// UGCVisibilities returns every visibility a user confession may carry.
func UGCVisibilities() []string {
	out := make([]string, len(ugcVisibilities))
	copy(out, ugcVisibilities)
	return out
}

// ValidVisibility reports whether v is a UGC visibility.
func ValidVisibility(v string) bool {
	for _, x := range ugcVisibilities {
		if x == v {
			return true
		}
	}
	return false
}

// Report and case vocabularies, mirrored from migrations/0002_status_constraints.sql
// and compared against the live database by the parity test.
var (
	reportStatuses = []string{"open", "reviewed", "resolved", "dismissed"}
	caseStatuses   = []string{"open", "in_review", "resolved", "dismissed"}
)

// ReportStatuses returns every status a report may carry.
func ReportStatuses() []string {
	out := make([]string, len(reportStatuses))
	copy(out, reportStatuses)
	return out
}

// CaseStatuses returns every status a moderation case may carry.
func CaseStatuses() []string {
	out := make([]string, len(caseStatuses))
	copy(out, caseStatuses)
	return out
}

// Report decisions a moderator may record. "reviewed" exists in the
// vocabulary for a future triage step and is deliberately not reachable from
// the decision endpoint: closing a report must state an outcome, and
// "reviewed" is not one.
var reportDecisions = []string{"resolved", "dismissed"}

// ValidReportDecision reports whether d closes a report with an outcome.
func ValidReportDecision(d string) bool {
	for _, x := range reportDecisions {
		if x == d {
			return true
		}
	}
	return false
}

// ReportableEntityTypes are the entity types a user can file a report
// against. The list is exactly the set of entities a signed-in user has an
// honest read path to: published confessions and community posts. A user
// confession can only ever be seen by its author today, so accepting a report
// against one would mean validating ids nobody could have obtained; when UGC
// gains a public reader this list is where it is added.
var ReportableEntityTypes = []string{"confession", "community_post"}

// ValidReportEntityType reports whether t may be reported.
func ValidReportEntityType(t string) bool {
	for _, x := range ReportableEntityTypes {
		if x == t {
			return true
		}
	}
	return false
}

// Bounds for free-text report fields. Unbounded user text in a moderator's
// inbox is a denial-of-wallet against a human.
const (
	ReasonMinLen = 3
	ReasonMaxLen = 140
	DetailMaxLen = 2000
)
