package engine

import (
	"errors"
	"strconv"
	"strings"
)

// Strategy governs how the engine reconciles the duration the user asked for
// with the durations of the complete confession items actually available.
//
// The constraint that shapes all of this is that audio is never truncated. A
// confession is spoken; cutting it mid-sentence to hit a target would make the
// product feel broken at exactly the moment the user is praying along. So the
// only lever available is *which whole items to include*, and the strategies
// differ purely in how they resolve the resulting mismatch.
type Strategy string

// The five strategies from the product spec.
//
// These are deliberately untyped constants rather than typed Strategy values.
// Callers use them in two different type positions — assigned to plain string
// fields on request structs, and to Strategy-typed variables — and an untyped
// constant satisfies both without a conversion at every use site.
const (
	// StrategyExact requires the session to land precisely on the requested
	// duration. It fails rather than approximating, which makes it the right
	// choice for scheduled or ritual use where the length is the point, and
	// the wrong choice for casual browsing.
	StrategyExact = "EXACT"

	// StrategyClosest minimises the absolute distance from the requested
	// duration and may land on either side of it.
	StrategyClosest = "CLOSEST"

	// StrategyUnder never exceeds the requested duration. Correct when the
	// session has a hard stop — a commute, a lunch break, a child's bedtime.
	StrategyUnder = "UNDER"

	// StrategyOver guarantees the full requested duration is covered,
	// accepting an overshoot of up to one item. Correct when running short
	// is worse than running long.
	StrategyOver = "OVER"

	// StrategyBalanced is the default. It behaves like CLOSEST but refuses to
	// overshoot by more than the length of a single item, so the user gets
	// the tightest fit that is still honest about what was played.
	StrategyBalanced = "BALANCED"
)

// DefaultStrategy is used when a caller does not name one.
const DefaultStrategy = StrategyBalanced

var (
	// ErrNoExactFit is returned by StrategyExact when no whole-item prefix
	// lands exactly on the requested duration.
	ErrNoExactFit = errors.New("no combination of complete confessions matches the requested duration exactly")

	// ErrNoFit is returned when even the shortest candidate exceeds the
	// budget, so no session can be built without truncating.
	ErrNoFit = errors.New("no complete confession fits within the requested duration")

	// ErrUnreachable is returned by StrategyOver when the available content
	// cannot cover the requested duration at all.
	ErrUnreachable = errors.New("available content cannot cover the requested duration")
)

// IsValidStrategy reports whether s names a duration strategy. Matching is
// case-insensitive because the value arrives from HTTP bodies; use
// NormalizeStrategy to obtain the canonical form.
func IsValidStrategy(s string) bool {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case StrategyExact, StrategyClosest, StrategyUnder, StrategyOver, StrategyBalanced:
		return true
	}
	return false
}

// NormalizeStrategy folds any accepted spelling onto a canonical Strategy.
// An empty or unrecognised value yields DefaultStrategy rather than an error,
// because "no strategy specified" is a normal request, not a malformed one.
// Callers that must reject unknown values should check IsValidStrategy first.
func NormalizeStrategy(s string) Strategy {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case StrategyExact:
		return StrategyExact
	case StrategyClosest:
		return StrategyClosest
	case StrategyUnder:
		return StrategyUnder
	case StrategyOver:
		return StrategyOver
	case StrategyBalanced:
		return StrategyBalanced
	default:
		return DefaultStrategy
	}
}

// Strategies lists the canonical strategies, with the default first.
func Strategies() []Strategy {
	return []Strategy{StrategyBalanced, StrategyExact, StrategyClosest, StrategyUnder, StrategyOver}
}

// Plan decides how many leading candidates a session should contain.
//
// candidates holds item durations in the order the content selector emitted
// them. Plan never reorders or filters that sequence — category balance and
// diversity are the selector's responsibility, and a planner that quietly
// re-sorted items would undo it. Plan only chooses where to cut.
//
// Every strategy therefore selects a *prefix*, which is what keeps sessions
// deterministic and reproducible: the same inputs always yield the same queue.
//
// target must be positive and every candidate duration must be positive.
func Plan(target int, candidates []int, s Strategy) (n int, err error) {
	if target <= 0 {
		return 0, errors.New("target duration must be positive")
	}
	if len(candidates) == 0 {
		return 0, ErrNoFit
	}
	for i, d := range candidates {
		if d <= 0 {
			return 0, errors.New("candidate " + strconv.Itoa(i) + " has non-positive duration")
		}
	}

	// sums[k] is the total duration of the first k candidates, so sums[0] == 0.
	sums := make([]int, len(candidates)+1)
	for i, d := range candidates {
		sums[i+1] = sums[i] + d
	}

	switch strat := NormalizeStrategy(string(s)); strat {
	case StrategyUnder:
		// Longest prefix that still fits.
		best := 0
		for k := 1; k <= len(candidates); k++ {
			if sums[k] > target {
				break
			}
			best = k
		}
		if best == 0 {
			return 0, ErrNoFit
		}
		return best, nil

	case StrategyOver:
		// Shortest prefix that covers the target.
		for k := 1; k <= len(candidates); k++ {
			if sums[k] >= target {
				return k, nil
			}
		}
		return 0, ErrUnreachable

	case StrategyExact:
		for k := 1; k <= len(candidates); k++ {
			if sums[k] == target {
				return k, nil
			}
		}
		return 0, ErrNoExactFit

	case StrategyClosest:
		return closest(sums, target, 0), nil

	case StrategyBalanced:
		// CLOSEST, but never overshoot by more than one whole item. That cap
		// is the smallest honest bound available: the next item is atomic, so
		// anything tighter could only be met by truncating audio.
		tolerance := 0
		for _, d := range candidates {
			if d > tolerance {
				tolerance = d
			}
		}
		return closest(sums, target, target+tolerance), nil

	default:
		return 0, errors.New("unknown duration strategy: " + string(strat))
	}
}

// closest returns the prefix length whose total is nearest to target.
//
// k ranges from 1 upward: an empty session is never the right answer, even
// when zero items would technically be "closest" to a short target. When an
// under-fill and an over-fill are equidistant the shorter prefix wins, so the
// product errs on the side of finishing early rather than running long.
//
// A limit above zero restricts consideration to prefixes at or below it. If
// no prefix satisfies the limit the shortest one is returned, so the caller
// always gets a usable session rather than a failure.
func closest(sums []int, target, limit int) int {
	best, bestDiff := 0, -1
	for k := 1; k < len(sums); k++ {
		if limit > 0 && sums[k] > limit {
			continue
		}
		diff := abs(sums[k] - target)
		if bestDiff < 0 || diff < bestDiff {
			best, bestDiff = k, diff
		}
	}
	if best == 0 {
		// Nothing met the limit; the shortest prefix is the least-bad answer.
		return 1
	}
	return best
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Preset is one of the fixed session lengths offered in the session builder.
type Preset struct {
	// Name is the canonical identifier sent on the wire, e.g. "10m".
	Name string
	// Label is the human-readable form, e.g. "10 minutes".
	Label string
	// Seconds is the duration, or 0 for the custom preset where the caller
	// supplies an explicit length.
	Seconds int
	// Custom marks the open-ended preset.
	Custom bool
}

// presets is the ladder from the product spec, in ascending order.
//
// These are product surface, not implementation detail: they appear in the
// builder UI and in deep links, so the names are part of the API contract.
var presets = []Preset{
	{Name: "10m", Label: "10 minutes", Seconds: 10 * 60},
	{Name: "15m", Label: "15 minutes", Seconds: 15 * 60},
	{Name: "30m", Label: "30 minutes", Seconds: 30 * 60},
	{Name: "45m", Label: "45 minutes", Seconds: 45 * 60},
	{Name: "60m", Label: "60 minutes", Seconds: 60 * 60},
	{Name: "90m", Label: "90 minutes", Seconds: 90 * 60},
	{Name: "120m", Label: "2 hours", Seconds: 120 * 60},
	{Name: "180m", Label: "3 hours", Seconds: 180 * 60},
	{Name: "custom", Label: "Custom length", Seconds: 0, Custom: true},
}

// Presets returns the duration ladder in ascending order, custom last.
func Presets() []Preset {
	out := make([]Preset, len(presets))
	copy(out, presets)
	return out
}

// PresetFor resolves a preset name to its duration in seconds.
//
// Accepted spellings are case-insensitive and forgiving of the separators
// clients actually send: "30", "30m", "30min", "30_min" and "30 minutes" all
// resolve to the 30-minute preset.
//
// The custom preset is recognised but has no inherent duration, so it returns
// (0, false) here. Callers handling a custom preset must take the duration
// from the request instead; see IsCustomPreset.
func PresetFor(name string) (int, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" || key[0] == '-' || key[0] == '+' {
		// A signed value is not a preset name. Rejecting here matters because
		// "-" is otherwise treated as a separator, so "-30m" would otherwise
		// quietly resolve to the 30-minute preset.
		return 0, false
	}
	key = strings.TrimSuffix(key, "s") // "minutes" / "mins" -> "minute" / "min"
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	key = strings.ReplaceAll(key, " ", "")
	key = strings.TrimSuffix(key, "minute")
	key = strings.TrimSuffix(key, "min")
	key = strings.TrimSuffix(key, "m")
	if key == "" {
		return 0, false
	}
	minutes, err := strconv.Atoi(key)
	if err != nil {
		return 0, false
	}
	for _, p := range presets {
		if p.Custom {
			continue
		}
		if p.Seconds == minutes*60 {
			return p.Seconds, true
		}
	}
	return 0, false
}

// IsCustomPreset reports whether name refers to the open-ended preset.
func IsCustomPreset(name string) bool {
	return strings.ToLower(strings.TrimSpace(name)) == "custom"
}
