package audio

import "fmt"

// AssetStatus is an audio asset's place in its production lifecycle.
//
// This is the single authority for the vocabulary in
// audio_assets_status_check and for every query that decides whether an asset
// may reach a listener. Those two used to be maintained separately: the CHECK
// allowed six values, the selection query asked for exactly 'ready', and the
// entitlement gate asked for exactly 'ready'. So an asset the schema was happy
// to store as 'published' could never be selected and could never be served -
// and nothing compared the two lists, so the disagreement was invisible.
type AssetStatus string

const (
	// StatusUploading is bytes in transit to object storage.
	StatusUploading AssetStatus = "uploading"
	// StatusProcessing is generated audio awaiting human QA. Nothing reaches a
	// listener from here (§31): a synthesised voice saying the wrong thing is
	// not a defect a listener should discover.
	StatusProcessing AssetStatus = "processing"
	// StatusReady has passed QA and may be served.
	StatusReady AssetStatus = "ready"
	// StatusPublished is ready and surfaced in discovery.
	StatusPublished AssetStatus = "published"
	// StatusFailed is a provider or processing fault.
	StatusFailed AssetStatus = "failed"
	// StatusQARejected is a human decision that this render must not ship. It
	// is distinct from StatusFailed: a failure is retried, a rejection is a
	// judgement about the content or the voice.
	StatusQARejected AssetStatus = "qa_rejected"
	// StatusArchived is withdrawn. Terminal.
	StatusArchived AssetStatus = "archived"
)

// All is every status the database constraint accepts, in lifecycle order.
func All() []AssetStatus {
	return []AssetStatus{
		StatusUploading, StatusProcessing, StatusReady, StatusPublished,
		StatusFailed, StatusQARejected, StatusArchived,
	}
}

// Valid reports whether the status is one this system defines.
func (s AssetStatus) Valid() bool {
	for _, v := range All() {
		if s == v {
			return true
		}
	}
	return false
}

// ServedStatuses are the statuses a listener may be given a playable link for.
//
// Selection and entitlement both read this, so they cannot disagree about what
// is servable.
func ServedStatuses() []AssetStatus {
	return []AssetStatus{StatusReady, StatusPublished}
}

// IsServed reports whether an asset in this status may reach a listener.
//
// An empty status is not served. That is the case for a session item with no
// audio asset at all, and treating the absence of evidence as evidence of
// approval is how audio that was never QA'd gets played.
func IsServed(status string) bool {
	for _, v := range ServedStatuses() {
		if AssetStatus(status) == v {
			return true
		}
	}
	return false
}

// assetTransitions is the legal QA lifecycle. It is deliberately narrow: the
// point of a QA workflow is that reaching a listener requires a human step,
// and a permissive graph quietly removes it.
var assetTransitions = map[AssetStatus][]AssetStatus{
	StatusUploading:  {StatusProcessing, StatusFailed},
	StatusProcessing: {StatusReady, StatusQARejected, StatusFailed},
	StatusReady:      {StatusPublished, StatusArchived, StatusFailed},
	StatusPublished:  {StatusArchived},
	// A failed render is regenerated; a rejected one may be regenerated after
	// the underlying problem is fixed.
	StatusFailed:     {StatusProcessing, StatusArchived},
	StatusQARejected: {StatusProcessing, StatusArchived},
	StatusArchived:   {}, // terminal
}

// CanTransition reports whether an asset may move between two statuses.
func CanTransition(from, to AssetStatus) bool {
	if from == to {
		return true
	}
	for _, allowed := range assetTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// AllowedFrom lists the statuses an asset may move to.
func AllowedFrom(from AssetStatus) []AssetStatus {
	return assetTransitions[from]
}

// TransitionError explains a refusal in terms the API can return.
type TransitionError struct {
	From, To AssetStatus
}

func (e *TransitionError) Error() string {
	allowed := AllowedFrom(e.From)
	if len(allowed) == 0 {
		return fmt.Sprintf("an audio asset in %q cannot change status", e.From)
	}
	return fmt.Sprintf("an audio asset cannot move from %q to %q; allowed: %v", e.From, e.To, allowed)
}

// ValidateTransition returns a *TransitionError when the move is illegal.
func ValidateTransition(from, to AssetStatus) error {
	if !to.Valid() {
		return fmt.Errorf("%q is not a recognised audio asset status", to)
	}
	if !CanTransition(from, to) {
		return &TransitionError{From: from, To: to}
	}
	return nil
}
