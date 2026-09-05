// Package sessions owns the authoritative lifecycle of a confession session.
//
// A session is the central artifact of I CONFESS: a user asks for thirty minutes
// of healing declarations, the engine builds a queue, and the client plays it.
// Everything the product measures — completion rate, retention, streaks — is
// derived from the transitions recorded here.
//
// That is why this package exists. Before it, session status was a flat
// whitelist validated in the HTTP layer: any of created/playing/completed/
// abandoned could be written from any other, so a client could mark a session
// COMPLETED without ever playing a second of audio. Completion is a primary
// product metric, and a metric the client can forge is not a metric.
//
// The rule this package enforces is structural rather than incidental: there is
// no path to COMPLETED that does not pass through a state in which audio was
// actually running. See TestCompletionIsUnreachableWithoutPlayback, which proves
// that by walking the whole graph rather than asserting one input.
package sessions

import "strings"

// State is a point in the session lifecycle.
//
// Values are upper snake case and stable: they are persisted to the sessions
// table and returned on the wire, so changing one is a data migration, not a
// rename.
type State string

const (
	// Draft is a session under construction — the user is still choosing
	// categories, duration or voice. Nothing is playable yet.
	Draft State = "DRAFT"

	// Ready is a built session with a materialised queue, waiting to start.
	Ready State = "READY"

	// Scheduled is a ready session bound to a future time by the scheduler.
	Scheduled State = "SCHEDULED"

	// Starting is the brief window while the player is acquiring audio and
	// the OS audio session. Distinct from Active so a failure to acquire
	// audio is recorded as FAILED rather than silently vanishing.
	Starting State = "STARTING"

	// Active means audio is playing. This is the only state from which
	// listening time is credited.
	Active State = "ACTIVE"

	// Paused is a deliberate user pause. The session is still live.
	Paused State = "PAUSED"

	// Interrupted is an involuntary stop: a phone call, Bluetooth dropping,
	// the OS reclaiming the audio session, or the app being terminated.
	// It is deliberately not a failure — the session is expected to resume.
	Interrupted State = "INTERRUPTED"

	// Completed means the queue ran to its end. Terminal.
	Completed State = "COMPLETED"

	// Cancelled means the user abandoned the session. Terminal.
	Cancelled State = "CANCELLED"

	// Expired means the session lapsed before it could run — a scheduled
	// session whose window closed, for instance. Terminal.
	Expired State = "EXPIRED"

	// Failed means the session could not proceed: no audio resolved, the
	// download was revoked, playback errored out. Terminal for the attempt,
	// but a FAILED session may be reset to READY to try again.
	Failed State = "FAILED"
)

// all lists every state. Order is lifecycle order and is used for display.
var all = []State{
	Draft, Ready, Scheduled, Starting, Active, Paused,
	Interrupted, Completed, Cancelled, Expired, Failed,
}

// transitions is the single source of truth for the lifecycle. Every legal
// move appears here; anything absent is rejected by CanTransition.
//
// Keep this table honest. Adding an edge is a product decision with metric
// consequences, not a convenience for a caller.
//
// One deliberate loosening: any state that already holds a materialised,
// playable queue — READY, SCHEDULED, STARTING — may move straight to ACTIVE.
// STARTING is an *optional* state for clients that want to record the window
// spent acquiring audio; it is not a mandatory hop. Forcing it would cost every
// client a round trip and would turn a plain "press play" into a 409. The
// invariant the product actually depends on is that completion requires
// playback, and entering ACTIVE directly does not weaken that.
//
// DRAFT and FAILED may not reach ACTIVE: a draft has no queue, and a failed
// session must be reset to READY before it can be retried.
var transitions = map[State][]State{
	Draft:       {Ready, Cancelled, Failed, Expired},
	Ready:       {Scheduled, Starting, Active, Cancelled, Expired, Failed},
	Scheduled:   {Ready, Starting, Active, Cancelled, Expired},
	Starting:    {Active, Interrupted, Cancelled, Failed},
	Active:      {Paused, Interrupted, Completed, Cancelled, Failed},
	Paused:      {Active, Interrupted, Completed, Cancelled, Failed, Expired},
	Interrupted: {Active, Completed, Cancelled, Expired},
	Failed:      {Ready, Cancelled},
	// Completed, Cancelled and Expired are terminal: no outgoing edges.
	Completed: {},
	Cancelled: {},
	Expired:   {},
}

// terminal states admit no further transitions.
var terminal = map[State]bool{
	Completed: true,
	Cancelled: true,
	Expired:   true,
}

// legacy maps pre-state-machine wire values onto the canonical vocabulary.
//
// These four lowercase values are what the sessions table already contains and
// what shipped clients send. Accepting them on the way in keeps existing rows
// and older app builds working; nothing new writes them.
var legacy = map[string]State{
	"created":   Ready,
	"playing":   Active,
	"completed": Completed,
	"abandoned": Cancelled,
}

// playingStates are the states in which audio is genuinely running or was
// running moments ago. Only these may reach Completed.
var playingStates = map[State]bool{
	Active:      true,
	Paused:      true,
	Interrupted: true,
}

// Normalize resolves any accepted spelling of a session state to its canonical
// form, or "" if the value is not a session state at all.
//
// It is deliberately forgiving about case and surrounding whitespace, because
// the input arrives from HTTP bodies, stored rows and mobile clients. It is
// deliberately unforgiving about vocabulary: an unknown value returns "" rather
// than guessing, so a typo cannot silently become a state change.
func Normalize(s string) string {
	v := strings.ToUpper(strings.TrimSpace(s))
	if v == "" {
		return ""
	}
	if IsValid(State(v)) {
		return v
	}
	if st, ok := legacy[strings.ToLower(v)]; ok {
		return string(st)
	}
	return ""
}

// Parse resolves untrusted input to a canonical state, reporting whether the
// value was recognised. It is the normal entry point for HTTP handlers.
func Parse(s string) (State, bool) {
	v := Normalize(s)
	if v == "" {
		return "", false
	}
	return State(v), true
}

// IsValid reports whether s is a canonical session state. It does not accept
// legacy spellings — use Normalize for untrusted input.
func IsValid(s State) bool {
	for _, st := range all {
		if st == s {
			return true
		}
	}
	return false
}

// All returns the canonical states in lifecycle order.
func All() []State {
	out := make([]State, len(all))
	copy(out, all)
	return out
}

// IsTerminal reports whether a session in state s is finished and cannot move
// to any other state. Replaying the terminal state itself is still accepted as
// an idempotent no-op; see CanTransition.
func IsTerminal(s State) bool { return terminal[s] }

// IsPlaying reports whether audio is running, or was running and is expected to
// resume, in state s.
func IsPlaying(s State) bool { return playingStates[s] }

// CanTransition reports whether moving from one state to another is legal.
//
// A transition to the current state is allowed and treated as an idempotent
// no-op. Retried requests are normal on a mobile client over a poor connection,
// and replaying "PAUSED" against a session that is already paused should
// succeed rather than surface a spurious 409.
//
// Both arguments must already be canonical; pass untrusted input through
// Normalize first.
func CanTransition(from, to State) bool {
	if !IsValid(from) || !IsValid(to) {
		return false
	}
	if from == to {
		return true
	}
	for _, next := range transitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// AllowedFrom lists the states reachable from s, excluding s itself.
func AllowedFrom(s State) []State {
	src := transitions[s]
	out := make([]State, 0, len(src))
	out = append(out, src...)
	return out
}

// Reason explains why a transition was refused, in language safe to return to
// a client. It never mentions internals; it says what the user can do instead.
//
// Reason returns "" for any transition CanTransition accepts, so a caller can
// use it unconditionally when building an error body without first branching
// on legality.
func Reason(from, to State) string {
	switch {
	case !IsValid(from):
		return "unknown session state"
	case !IsValid(to):
		return "unknown target state"
	case CanTransition(from, to):
		return ""
	case terminal[from]:
		return "this session has already finished and cannot be changed"
	case to == Completed && !playingStates[from]:
		// The specific case that matters most: completion is earned by
		// listening, not declared by the client.
		return "a session can only be completed after playback has started"
	default:
		return "that change is not allowed from the session's current state"
	}
}
