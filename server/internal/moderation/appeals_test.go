package moderation

import (
	"errors"
	"strings"
	"testing"
)

// TestAppealTransitions is the unit edge table for the appeal lifecycle, in the
// same shape as TestTrialTransitions and TestContentTransitions: every pair in
// the vocabulary is checked, so an edge that should not exist is caught here
// rather than by whichever handler happens to try it.
func TestAppealTransitions(t *testing.T) {
	allowed := map[AppealStatus]map[AppealStatus]bool{
		AppealSubmitted:   {AppealUnderReview: true, AppealUpheld: true, AppealOverturned: true},
		AppealUnderReview: {AppealUpheld: true, AppealOverturned: true},
		AppealUpheld:      {},
		AppealOverturned:  {},
	}

	for _, from := range appealStatuses {
		for _, to := range appealStatuses {
			want := allowed[from][to]
			if got := CanAppealTransition(from, to); got != want {
				t.Errorf("CanAppealTransition(%s, %s) = %v, want %v", from, to, got, want)
			}
			err := ValidateAppealTransition(from, to)
			if want && err != nil {
				t.Errorf("ValidateAppealTransition(%s, %s) = %v, want nil", from, to, err)
			}
			if !want && err == nil {
				t.Errorf("ValidateAppealTransition(%s, %s) = nil, want an error", from, to)
			}
			if !want && err != nil {
				var terr *AppealTransitionError
				if !errors.As(err, &terr) {
					t.Errorf("ValidateAppealTransition(%s, %s) returned %T, want *AppealTransitionError", from, to, err)
				}
			}
		}
	}

	// A self-transition is a second decision on the same appeal, which the
	// lifecycle forbids: a moderator re-deciding would make the recorded
	// outcome provisional and the appellant's position unknowable.
	for _, s := range appealStatuses {
		if CanAppealTransition(s, s) {
			t.Errorf("%s may transition to itself; a re-decision must be refused", s)
		}
	}
}

// TestAppealTerminalStatesAreExactlyTheDecisions proves the two questions the
// queue and the API both ask - "is this closed?" and "is this a decision?" -
// are answered by the same edge table rather than by two lists that can
// disagree.
func TestAppealTerminalStatesAreExactlyTheDecisions(t *testing.T) {
	for _, s := range appealStatuses {
		isDecision := s == AppealUpheld || s == AppealOverturned
		if IsAppealTerminal(s) != isDecision {
			t.Errorf("IsAppealTerminal(%s) = %v, want %v", s, IsAppealTerminal(s), isDecision)
		}
	}
	if IsAppealTerminal(AppealSubmitted) || IsAppealTerminal(AppealUnderReview) {
		t.Error("an undecided appeal must not be terminal, or it could never be decided")
	}
}

// TestAppealStatusVocabularyIsClosed checks the exported vocabulary against the
// constants, and that the validity predicate agrees with the list. A status
// added to one and not the other is how a CHECK constraint and a Go switch end
// up describing different systems.
func TestAppealStatusVocabularyIsClosed(t *testing.T) {
	got := AppealStatuses()
	want := []string{"submitted", "under_review", "upheld", "overturned"}
	if len(got) != len(want) {
		t.Fatalf("AppealStatuses() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AppealStatuses()[%d] = %q, want %q", i, got[i], want[i])
		}
		if !ValidAppealStatus(got[i]) {
			t.Errorf("ValidAppealStatus(%q) = false, want true", got[i])
		}
	}
	for _, bad := range []string{"", "approved", "rejected", "SUBMITTED", "under review"} {
		if ValidAppealStatus(bad) {
			t.Errorf("ValidAppealStatus(%q) = true, want false", bad)
		}
	}
}

// TestAppealDecisionsAreTheTwoOutcomes keeps the decision endpoint's accepted
// inputs tied to the terminal states. A third decision would be a state the
// lifecycle has no edge for.
func TestAppealDecisionsAreTheTwoOutcomes(t *testing.T) {
	if len(AppealDecisions) != 2 {
		t.Fatalf("AppealDecisions = %v, want upheld and overturned", AppealDecisions)
	}
	for _, d := range AppealDecisions {
		if !ValidAppealDecision(d) {
			t.Errorf("ValidAppealDecision(%q) = false, want true", d)
		}
		if !ValidAppealStatus(d) {
			t.Errorf("decision %q is not an appeal status; a decision must land the appeal in a real state", d)
		}
		if !IsAppealTerminal(AppealStatus(d)) {
			t.Errorf("decision %q does not close the appeal", d)
		}
	}
	if ValidAppealDecision("submitted") || ValidAppealDecision("under_review") {
		t.Error("an in-progress status is not a decision")
	}
}

// TestOnlyDecisionsThatEndAConversationAreAppealable documents the boundary:
// nothing that grants something is appealable, because nobody is harmed by an
// approval.
func TestOnlyDecisionsThatEndAConversationAreAppealable(t *testing.T) {
	for _, want := range []string{"report", "confession"} {
		if !ValidAppealDecisionType(want) {
			t.Errorf("ValidAppealDecisionType(%q) = false, want true", want)
		}
	}
	for _, bad := range []string{"", "approval", "user", "session", "REPORT"} {
		if ValidAppealDecisionType(bad) {
			t.Errorf("ValidAppealDecisionType(%q) = true, want false", bad)
		}
	}
}

// TestAppealStatementBoundsAreEnforced checks the bound that protects the human
// who reads the appeal.
func TestAppealStatementBoundsAreEnforced(t *testing.T) {
	if err := ValidateAppealStatement("too short"); err == nil {
		t.Error("a statement below the minimum was accepted")
	}
	if err := ValidateAppealStatement(strings.Repeat("a", AppealStatementMinLen)); err != nil {
		t.Errorf("a statement at the minimum was refused: %v", err)
	}
	if err := ValidateAppealStatement(strings.Repeat("a", AppealStatementMaxLen)); err != nil {
		t.Errorf("a statement at the maximum was refused: %v", err)
	}
	if err := ValidateAppealStatement(strings.Repeat("a", AppealStatementMaxLen+1)); err == nil {
		t.Error("a statement above the maximum was accepted")
	}
}

// TestBlockingRules covers the two rules that keep a block a boundary rather
// than a weapon: no self-blocks, and both directions stop contact.
func TestBlockingRules(t *testing.T) {
	if err := ValidateBlock("a", "a"); err == nil {
		t.Error("a self-block was accepted")
	}
	if err := ValidateBlock("", "b"); err == nil {
		t.Error("a block with no blocker was accepted")
	}
	if err := ValidateBlock("a", ""); err == nil {
		t.Error("a block with no target was accepted")
	}
	if err := ValidateBlock("a", "b"); err != nil {
		t.Errorf("a normal block was refused: %v", err)
	}

	// A block is a boundary in both directions. Honouring only one would let a
	// listener keep contacting someone who asked not to hear from them.
	if !BlocksReaction(true, false) {
		t.Error("a reaction was allowed from an account the author blocked")
	}
	if !BlocksReaction(false, true) {
		t.Error("a reaction was allowed toward an account the reactor blocked")
	}
	if !BlocksReaction(true, true) {
		t.Error("mutual blocks should still block")
	}
	if BlocksReaction(false, false) {
		t.Error("a reaction between two accounts with no block was refused")
	}

	if err := ValidateBlockReason(strings.Repeat("r", BlockReasonMaxLen)); err != nil {
		t.Errorf("a reason at the maximum was refused: %v", err)
	}
	if err := ValidateBlockReason(strings.Repeat("r", BlockReasonMaxLen+1)); err == nil {
		t.Error("a reason above the maximum was accepted")
	}
	if err := ValidateBlockReason(""); err != nil {
		t.Errorf("an empty reason was refused, but most blocks are not explained: %v", err)
	}
}
