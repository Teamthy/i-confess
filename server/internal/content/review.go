package content

// ReviewStatus is the theological review vocabulary (directive §22; PHASE 40
// columns, PHASE 44 writer).
//
// It is the review *outcome* recorded on a confession, distinct from the
// lifecycle status theological_review, which says whose desk the text is on.
// A confession can be in audio production and still carry "reviewed" — the
// outcome outlives the step. It is closed and database-enforced
// (confessions_theological_review_status_check); the parity test in
// internal/db compares this list to the live CHECK in both directions.
type ReviewStatus string

const (
	// ReviewUnreviewed is the default: nobody has recorded a review.
	ReviewUnreviewed ReviewStatus = "unreviewed"
	// ReviewReviewed means a theological reviewer read the text and found
	// its scriptural basis sound.
	ReviewReviewed ReviewStatus = "reviewed"
	// ReviewNeedsRevision means the reviewer read it and sent it back. The
	// notes say why; the text must change before it is reviewed again.
	ReviewNeedsRevision ReviewStatus = "needs_revision"
)

var reviewStatuses = []ReviewStatus{ReviewUnreviewed, ReviewReviewed, ReviewNeedsRevision}

// ReviewStatuses returns the closed vocabulary, in order.
func ReviewStatuses() []ReviewStatus {
	out := make([]ReviewStatus, len(reviewStatuses))
	copy(out, reviewStatuses)
	return out
}

// ValidReview reports whether s is a review outcome.
func ValidReview(s string) bool {
	for _, r := range reviewStatuses {
		if string(r) == s {
			return true
		}
	}
	return false
}

// ReviewOutcomes are the outcomes a reviewer may record. "unreviewed" is the
// absence of a review and cannot be written back: a review that happened is
// a fact, and a wrong one is corrected by a new review, not by pretending it
// did not occur.
func ReviewOutcomes() []ReviewStatus {
	return []ReviewStatus{ReviewReviewed, ReviewNeedsRevision}
}

// ValidReviewOutcome reports whether s may be recorded by a reviewer.
func ValidReviewOutcome(s string) bool {
	for _, r := range ReviewOutcomes() {
		if string(r) == s {
			return true
		}
	}
	return false
}
