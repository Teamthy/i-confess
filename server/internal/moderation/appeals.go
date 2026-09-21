package moderation

import "fmt"

// Appeals (master-plan 32).
//
// Every decision PHASE 31 made is terminal: a dismissed report and a rejected
// confession both end the conversation, and the person the decision was made
// about has no way to answer. An appeal is that answer, and it has a lifecycle
// of its own because it is work a moderator does, not a flag on the decision.
//
// The rule the lifecycle encodes: an appeal is heard once. A moderator may
// uphold the original decision or overturn it, and either way the appeal is
// closed. There is no re-filing, because an appeal that could be filed until a
// moderator relented is not an appeal — it is a queue.

// AppealStatus is one state in the appeal lifecycle.
type AppealStatus string

const (
	// AppealSubmitted is a filed appeal nobody has picked up yet.
	AppealSubmitted AppealStatus = "submitted"
	// AppealUnderReview is a moderator holding it.
	AppealUnderReview AppealStatus = "under_review"
	// AppealUpheld means the original decision stands. Terminal.
	AppealUpheld AppealStatus = "upheld"
	// AppealOverturned means the original decision is reversed and the work it
	// closed is reopened. Terminal.
	AppealOverturned AppealStatus = "overturned"
)

// appealStatuses is the complete vocabulary in lifecycle order. The CHECK
// installed by migrations/0019_blocking_appeals.sql must match it, and the
// parity test reads the constraint back from the live database.
var appealStatuses = []AppealStatus{
	AppealSubmitted, AppealUnderReview, AppealUpheld, AppealOverturned,
}

// AppealStatuses returns every status an appeal may carry.
func AppealStatuses() []string {
	out := make([]string, len(appealStatuses))
	for i, s := range appealStatuses {
		out[i] = string(s)
	}
	return out
}

// ValidAppealStatus reports whether s is a known appeal status.
func ValidAppealStatus(s string) bool {
	for _, x := range appealStatuses {
		if string(x) == s {
			return true
		}
	}
	return false
}

// appealEdges is intentionally explicit and forward-only, in the same shape as
// internal/trial and internal/content. An appeal is not an ordinal: both
// decisions are terminal, and a decided appeal must never be reopened, because
// reopening it would make the moderator's recorded outcome provisional and the
// appellant's position unknowable.
var appealEdges = map[AppealStatus][]AppealStatus{
	AppealSubmitted:   {AppealUnderReview, AppealUpheld, AppealOverturned},
	AppealUnderReview: {AppealUpheld, AppealOverturned},
	AppealUpheld:      {},
	AppealOverturned:  {},
}

// AppealAllowedFrom lists the statuses an appeal may move to from from.
func AppealAllowedFrom(from AppealStatus) []AppealStatus {
	return appealEdges[from]
}

// CanAppealTransition reports whether the exact directed edge is allowed.
func CanAppealTransition(from, to AppealStatus) bool {
	for _, candidate := range appealEdges[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

// AppealTransitionError explains a refused appeal move in terms the API returns.
type AppealTransitionError struct{ From, To AppealStatus }

func (e *AppealTransitionError) Error() string {
	return fmt.Sprintf("cannot move an appeal from %q to %q", e.From, e.To)
}

// ValidateAppealTransition returns an *AppealTransitionError when the move is
// not in the graph. Self-transitions are refused: a moderator recording a
// second decision on a closed appeal is a bug, not an idempotent retry.
func ValidateAppealTransition(from, to AppealStatus) error {
	if CanAppealTransition(from, to) {
		return nil
	}
	return &AppealTransitionError{From: from, To: to}
}

// IsAppealTerminal reports whether an appeal in this status can never move
// again. The queue and the counts both need the question, and answering it from
// the edge table rather than a second list is what keeps them from disagreeing.
func IsAppealTerminal(s AppealStatus) bool {
	return len(appealEdges[s]) == 0
}

// AppealDecisions are the two outcomes a moderator may record. Both close the
// appeal; they differ in what happens to the decision being appealed.
var AppealDecisions = []string{string(AppealUpheld), string(AppealOverturned)}

// ValidAppealDecision reports whether d closes an appeal with an outcome.
func ValidAppealDecision(d string) bool {
	for _, x := range AppealDecisions {
		if x == d {
			return true
		}
	}
	return false
}

// AppealableDecisionTypes are the decisions that can be appealed. Both are
// decisions that end a conversation the appellant started: a report they filed
// that was dismissed, and a confession they wrote that was rejected.
//
// Nothing that grants something is appealable — there is no appeal against
// approval, because nobody is harmed by it.
var AppealableDecisionTypes = []string{"report", "confession"}

// ValidAppealDecisionType reports whether t names an appealable decision.
func ValidAppealDecisionType(t string) bool {
	for _, x := range AppealableDecisionTypes {
		if x == t {
			return true
		}
	}
	return false
}

// Bounds for the appellant's own words. A statement is read by a human, so it
// is bounded for the same reason a report detail is.
const (
	AppealStatementMinLen = 10
	AppealStatementMaxLen = 4000
)

// ValidateAppealStatement checks the appellant's statement against the bounds.
func ValidateAppealStatement(s string) error {
	n := len([]rune(s))
	if n < AppealStatementMinLen {
		return fmt.Errorf("appeal statement must be at least %d characters", AppealStatementMinLen)
	}
	if n > AppealStatementMaxLen {
		return fmt.Errorf("appeal statement must be at most %d characters", AppealStatementMaxLen)
	}
	return nil
}
