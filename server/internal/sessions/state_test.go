package sessions

import (
	"strings"
	"testing"
)

func TestNormalizeAcceptsCanonicalAndLegacy(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// canonical, any case
		{"READY", "READY"},
		{"ready", "READY"},
		{"  Active  ", "ACTIVE"},
		{"interrupted", "INTERRUPTED"},
		// legacy wire values, still present in stored rows
		{"created", "READY"},
		{"CREATED", "READY"},
		{"playing", "ACTIVE"},
		{"completed", "COMPLETED"},
		{"abandoned", "CANCELLED"},
		// rejected
		{"", ""},
		{"   ", ""},
		{"done", ""},
		{"finished", ""},
		{"ACTIVEE", ""},
		{"COMPLETED!", ""},
		{"null", ""},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsValidRejectsLegacySpellings(t *testing.T) {
	// IsValid is the strict predicate. Legacy values must go through
	// Normalize, otherwise a caller can bypass the vocabulary check.
	for _, bad := range []State{"created", "playing", "completed", "abandoned", "", "NOPE"} {
		if IsValid(bad) {
			t.Errorf("IsValid(%q) = true, want false", bad)
		}
	}
	for _, good := range All() {
		if !IsValid(good) {
			t.Errorf("IsValid(%q) = false, want true", good)
		}
	}
}

func TestAllCoversSpecifiedVocabulary(t *testing.T) {
	// The lifecycle is fixed by the product spec. If someone adds a state
	// here without updating this list, that is a deliberate act and the
	// test should make them say so.
	want := []State{
		Draft, Ready, Scheduled, Starting, Active, Paused,
		Interrupted, Completed, Cancelled, Expired, Failed,
	}
	got := All()
	if len(got) != len(want) {
		t.Fatalf("All() returned %d states, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("All()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEveryStateIsInTheTransitionTable(t *testing.T) {
	// A state missing from the table would be silently unreachable and
	// un-exitable. Terminal states must still be present, with no edges.
	for _, s := range All() {
		edges, ok := transitions[s]
		if !ok {
			t.Fatalf("state %q is absent from the transition table", s)
		}
		if IsTerminal(s) && len(edges) != 0 {
			t.Errorf("terminal state %q has outgoing edges %v", s, edges)
		}
		if !IsTerminal(s) && len(edges) == 0 {
			t.Errorf("non-terminal state %q is a dead end", s)
		}
	}
}

func TestTransitionEdgesAreValidStates(t *testing.T) {
	for from, tos := range transitions {
		if !IsValid(from) {
			t.Errorf("transition table key %q is not a valid state", from)
		}
		for _, to := range tos {
			if !IsValid(to) {
				t.Errorf("%q -> %q: target is not a valid state", from, to)
			}
		}
	}
}

func TestCanTransition(t *testing.T) {
	allowed := [][2]State{
		{Draft, Ready},
		{Ready, Scheduled},
		{Ready, Starting},
		{Ready, Active}, // plain "press play", no STARTING hop required
		{Scheduled, Starting},
		{Scheduled, Active}, // scheduler fires and playback begins at once
		{Scheduled, Ready},
		{Starting, Active},
		{Active, Paused},
		{Paused, Active},
		{Active, Completed},
		{Paused, Completed},
		{Interrupted, Completed},
		{Active, Interrupted},
		{Interrupted, Active},
		{Failed, Ready},
		{Ready, Cancelled},
		{Scheduled, Expired},
	}
	for _, e := range allowed {
		if !CanTransition(e[0], e[1]) {
			t.Errorf("CanTransition(%q, %q) = false, want true", e[0], e[1])
		}
	}

	denied := [][2]State{
		{Ready, Completed},     // forged completion — the bug this package closes
		{Draft, Completed},     //
		{Scheduled, Completed}, //
		{Starting, Completed},  //
		{Completed, Active},    // terminal
		{Cancelled, Ready},     // terminal
		{Expired, Starting},    // terminal
		{Draft, Active},        // a draft has no queue to play
		{Failed, Active},       // must be reset to READY first
		{Paused, Scheduled},    // cannot re-schedule a live session
		{Interrupted, Paused},  // must resume before pausing
		{"created", Active},    // legacy spelling: not canonical, must be normalized
		{Active, "playing"},    //
		{"", Ready},            //
		{Ready, ""},            //
	}
	for _, e := range denied {
		if CanTransition(e[0], e[1]) {
			t.Errorf("CanTransition(%q, %q) = true, want false", e[0], e[1])
		}
	}
}

func TestSelfTransitionIsAlwaysIdempotent(t *testing.T) {
	// A retried request replaying the current state must succeed, so a
	// mobile client on a bad connection does not get a spurious 409. This
	// holds for terminal states too: replaying COMPLETED against a session
	// that already completed is a benign no-op, not a conflict.
	for _, s := range All() {
		if !CanTransition(s, s) {
			t.Errorf("CanTransition(%q, %q) = false, want true (idempotent no-op)", s, s)
		}
		if r := Reason(s, s); r != "" {
			t.Errorf("Reason(%q, %q) = %q, want empty for a no-op", s, s, r)
		}
	}
}

func TestTerminalStatesHaveNoExit(t *testing.T) {
	// Terminal is about leaving, not about no-op replay.
	for _, s := range All() {
		if !IsTerminal(s) {
			continue
		}
		for _, to := range All() {
			if to == s {
				continue
			}
			if CanTransition(s, to) {
				t.Errorf("terminal state %q can move to %q", s, to)
			}
		}
	}
}

// TestCompletionRequiresPlaybackOnEveryPath is the test that matters.
//
// It proves COMPLETED cannot be reached without first passing through a state
// in which audio actually ran. A naive reachability walk is not enough — DRAFT
// does reach COMPLETED, legitimately, via ACTIVE. What must not exist is a
// path that skips playback entirely, because that is the path a client would
// use to forge a completion.
//
// A path that never touches a playback state lies entirely within the
// non-playback subgraph, so the property is equivalent to: in the subgraph
// induced by non-playback states, COMPLETED is unreachable from every state.
// That formulation is cycle-safe, unlike a path-enumerating walk — the
// lifecycle contains a deliberate ACTIVE <-> PAUSED cycle.
func TestCompletionRequiresPlaybackOnEveryPath(t *testing.T) {
	// Restrict every edge to non-playback targets.
	restricted := map[State][]State{}
	for _, s := range All() {
		if playingStates[s] {
			continue
		}
		for _, next := range transitions[s] {
			if playingStates[next] {
				continue
			}
			restricted[s] = append(restricted[s], next)
		}
	}

	for _, start := range All() {
		if playingStates[start] {
			continue
		}
		seen := map[State]bool{start: true}
		stack := []State{start}
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, next := range restricted[cur] {
				if next == Completed {
					t.Errorf("COMPLETED is reachable from %q using only non-playback states; "+
						"a client could forge completion. Remove that edge.", start)
					continue
				}
				if !seen[next] {
					seen[next] = true
					stack = append(stack, next)
				}
			}
		}
	}

	// The contrapositive, stated positively: COMPLETED has no incoming edge
	// from a non-playback state.
	for _, s := range All() {
		if playingStates[s] {
			continue
		}
		for _, next := range transitions[s] {
			if next == Completed {
				t.Errorf("edge %q -> COMPLETED bypasses playback", s)
			}
		}
	}
}

// TestPlaybackStatesAreTheOnlyOnesThatMayComplete states the invariant from the
// other direction: the set of states allowed to complete is exactly the set of
// states in which audio ran.
func TestPlaybackStatesAreTheOnlyOnesThatMayComplete(t *testing.T) {
	for _, s := range All() {
		canComplete := false
		for _, next := range transitions[s] {
			if next == Completed {
				canComplete = true
			}
		}
		if canComplete != playingStates[s] {
			t.Errorf("state %q: may complete = %v, is a playback state = %v; these must agree",
				s, canComplete, playingStates[s])
		}
	}
}

func TestReasonIsClientSafeAndActionable(t *testing.T) {
	// The forged-completion case must say something a user understands.
	r := Reason(Ready, Completed)
	if r == "" {
		t.Fatal("Reason(READY, COMPLETED) returned no explanation")
	}
	if !strings.Contains(strings.ToLower(r), "playback") {
		t.Errorf("Reason(READY, COMPLETED) = %q; expected it to mention playback", r)
	}

	if got := Reason(Completed, Active); got == "" {
		t.Error("Reason(COMPLETED, ACTIVE) returned no explanation for a terminal state")
	}
	if got := Reason(Active, Paused); got != "" {
		t.Errorf("Reason on a legal transition = %q, want empty", got)
	}
	if got := Reason("BOGUS", Ready); got == "" {
		t.Error("Reason on an invalid state returned no explanation")
	}
	// No internal identifiers should leak to a client.
	for _, s := range All() {
		for _, to := range All() {
			msg := Reason(s, to)
			if strings.Contains(msg, "transitions[") || strings.Contains(msg, "map[") {
				t.Errorf("Reason(%q, %q) leaks internals: %q", s, to, msg)
			}
		}
	}
}

func TestAllowedFromExcludesSelf(t *testing.T) {
	for _, s := range All() {
		for _, next := range AllowedFrom(s) {
			if next == s {
				t.Errorf("AllowedFrom(%q) includes itself", s)
			}
		}
	}
	if got := AllowedFrom(Ready); len(got) != len(transitions[Ready]) {
		t.Errorf("AllowedFrom(READY) returned %d edges, want %d", len(got), len(transitions[Ready]))
	}
}

func TestAllIsDefensiveCopy(t *testing.T) {
	a := All()
	a[0] = "MUTATED"
	if All()[0] == "MUTATED" {
		t.Error("All() returned a reference to package state; callers can corrupt the vocabulary")
	}
}
