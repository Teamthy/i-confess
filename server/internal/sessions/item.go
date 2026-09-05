package sessions

import "strings"

// ItemStatus is the lifecycle of one queue item inside a session.
//
// It is deliberately separate from the session State above. A session is the
// ritual; an item is one confession inside it. Conflating the two is what makes
// playback code unreadable — "is this paused?" means something different for a
// session than for the track currently playing.
type ItemStatus string

const (
	// ItemQueued is waiting its turn.
	ItemQueued ItemStatus = "QUEUED"
	// ItemPlaying is the item audio is currently running on. At most one item
	// per session is ever in this state.
	ItemPlaying ItemStatus = "PLAYING"
	// ItemCompleted ran to the end. This is what counts towards session
	// completion and listening statistics.
	ItemCompleted ItemStatus = "COMPLETED"
	// ItemSkipped was deliberately passed over by the listener. Recorded
	// separately from completion because it feeds recommendations (§18):
	// skipped content should be offered less, not more.
	ItemSkipped ItemStatus = "SKIPPED"
	// ItemFailed could not be played: audio missing, download revoked, decode
	// error. Distinct from skipped so an infrastructure failure is never
	// mistaken for a listener's taste.
	ItemFailed ItemStatus = "FAILED"
)

// itemLegacy folds the spellings the session_items table already contains onto
// the canonical vocabulary.
var itemLegacy = map[string]ItemStatus{
	"queued":  ItemQueued,
	"played":  ItemCompleted,
	"skipped": ItemSkipped,
}

var itemAll = []ItemStatus{ItemQueued, ItemPlaying, ItemCompleted, ItemSkipped, ItemFailed}

// NormalizeItemStatus resolves any accepted spelling to its canonical form, or
// "" if the value is not an item status.
func NormalizeItemStatus(s string) string {
	v := strings.ToUpper(strings.TrimSpace(s))
	if v == "" {
		return ""
	}
	for _, st := range itemAll {
		if string(st) == v {
			return v
		}
	}
	if st, ok := itemLegacy[strings.ToLower(v)]; ok {
		return string(st)
	}
	return ""
}

// IsValidItemStatus reports whether s is a canonical item status.
func IsValidItemStatus(s ItemStatus) bool {
	for _, st := range itemAll {
		if st == s {
			return true
		}
	}
	return false
}

// IsFinalItemStatus reports whether an item has finished and will not be
// revisited in this session.
func IsFinalItemStatus(s ItemStatus) bool {
	switch s {
	case ItemCompleted, ItemSkipped, ItemFailed:
		return true
	}
	return false
}

// CountsTowardsCompletion reports whether an item in this state should be
// counted as listened-to when the session's completion percentage is computed.
//
// Skipped items do not count. A session where the listener skipped everything
// must not report as fully completed — that number feeds both the product
// metrics and the completion screen the listener sees.
func CountsTowardsCompletion(s ItemStatus) bool { return s == ItemCompleted }
