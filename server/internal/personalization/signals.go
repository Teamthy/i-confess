// Package personalization turns what a listener has actually done into a
// deterministic ranking of what to offer next.
//
// Master-plan item 34 names seven behavioural signals and one rule about them:
// start with deterministic rules, do not introduce ML. This package is that
// rule made concrete. Every input is a count or a duration the store can
// produce with a query, every weight is a named constant, and the same
// Signals always rank the same catalogue the same way. There is no model, no
// randomness and no learned parameter anywhere in the package.
//
// The seven signals are a product contract, not an implementation detail:
// TestSevenListenerSignalsAreTheProductContract asserts that each one is
// represented as a real quantity (never collapsed to a yes/no) and that each
// one, on its own, changes the ranking.
package personalization

import (
	"sort"
	"time"
)

// Signal names the seven behavioural inputs of master-plan item 34, in the
// order the plan lists them. The names are stable: they are reported by the
// API so a client can say why something was recommended.
type Signal string

const (
	SignalCategories     Signal = "categories_listened_to"
	SignalCompletionRate Signal = "completion_rate"
	SignalTimeOfDay      Signal = "time_of_day"
	SignalDuration       Signal = "session_duration"
	SignalFavourites     Signal = "favourites"
	SignalSkips          Signal = "skips"
	SignalRepeats        Signal = "repeat_listening"
)

// All returns the seven signals in master-plan order.
func All() []Signal {
	return []Signal{
		SignalCategories, SignalCompletionRate, SignalTimeOfDay, SignalDuration,
		SignalFavourites, SignalSkips, SignalRepeats,
	}
}

// Daypart is a coarse local time-of-day bucket. Four buckets are enough to
// tell "starts the day with this" from "falls asleep to this", which is the
// distinction the ritual is built on (§18), and coarse enough that a listener
// with a handful of sessions still lands somewhere.
type Daypart string

const (
	Morning   Daypart = "morning"   // 05:00–11:59
	Afternoon Daypart = "afternoon" // 12:00–16:59
	Evening   Daypart = "evening"   // 17:00–20:59
	Night     Daypart = "night"     // 21:00–04:59
)

// DaypartOf buckets a local wall-clock time.
func DaypartOf(t time.Time) Daypart {
	switch h := t.Hour(); {
	case h >= 5 && h < 12:
		return Morning
	case h >= 12 && h < 17:
		return Afternoon
	case h >= 17 && h < 21:
		return Evening
	default:
		return Night
	}
}

// Listen is one confession heard, or passed over, inside one session. The
// store produces these from session_items joined to sessions; the item status
// is already normalised to the canonical vocabulary.
type Listen struct {
	SessionID    string
	ConfessionID string
	CategoryID   string
	// Completed is true when the item ran to the end, false when it was
	// skipped. Failed items never reach this package: an infrastructure
	// failure is not a matter of taste (see sessions.ItemFailed).
	Completed bool
	// StartedAt is the session start in UTC. Zero when the session never
	// started, in which case the item contributes no time-of-day evidence.
	StartedAt time.Time
}

// Favourites is what the listener explicitly marked. Each set is keyed by
// entity id.
type Favourites struct {
	Confessions map[string]bool
	Categories  map[string]bool
	Voices      map[string]bool
}

// Signals is the seven-signal profile of one listener. Each field is the raw
// evidence; the methods derive the quantities the ranking uses.
//
// Nothing here is a boolean summary of a count. Repeat listening in particular
// is kept per confession as "how many separate sessions completed this", not
// "has repeated: yes" — a confession heard nine times and one heard twice are
// not the same recommendation.
type Signals struct {
	// Listens is every completed or skipped item, newest session first.
	Listens []Listen
	// CompletedDurations are the actual lengths, in seconds, of the sessions
	// the listener finished.
	CompletedDurations []int
	// Favourites is the explicit set.
	Favourites Favourites
	// Location is the listener's profile timezone, used to bucket UTC session
	// starts into local dayparts. nil means UTC.
	Location *time.Location
}

func (s Signals) loc() *time.Location {
	if s.Location == nil {
		return time.UTC
	}
	return s.Location
}

// CategoriesListenedTo is signal 1: completed items per category.
func (s Signals) CategoriesListenedTo() map[string]int {
	out := map[string]int{}
	for _, l := range s.Listens {
		if l.Completed && l.CategoryID != "" {
			out[l.CategoryID]++
		}
	}
	return out
}

// CompletionRate is signal 2: completed items over completed plus skipped.
// The second value reports how many items the rate rests on, so a caller can
// tell 1/1 from 40/40 before acting on it.
func (s Signals) CompletionRate() (rate float64, sample int) {
	completed, skipped := 0, 0
	for _, l := range s.Listens {
		if l.Completed {
			completed++
		} else {
			skipped++
		}
	}
	sample = completed + skipped
	if sample == 0 {
		return 0, 0
	}
	return float64(completed) / float64(sample), sample
}

// DaypartsByCategory is signal 3: for each category, how many completed
// listens started in each local daypart.
func (s Signals) DaypartsByCategory() map[string]map[Daypart]int {
	out := map[string]map[Daypart]int{}
	for _, l := range s.Listens {
		if !l.Completed || l.StartedAt.IsZero() || l.CategoryID == "" {
			continue
		}
		dp := DaypartOf(l.StartedAt.In(s.loc()))
		if out[l.CategoryID] == nil {
			out[l.CategoryID] = map[Daypart]int{}
		}
		out[l.CategoryID][dp]++
	}
	return out
}

// PreferredDaypart is the daypart with the most completed listens overall,
// and whether there was any time-of-day evidence at all. Ties resolve in the
// order of the day so the answer is deterministic.
func (s Signals) PreferredDaypart() (Daypart, bool) {
	counts := map[Daypart]int{}
	for _, l := range s.Listens {
		if l.Completed && !l.StartedAt.IsZero() {
			counts[DaypartOf(l.StartedAt.In(s.loc()))]++
		}
	}
	best, bestN := Daypart(""), 0
	for _, dp := range []Daypart{Morning, Afternoon, Evening, Night} {
		if counts[dp] > bestN {
			best, bestN = dp, counts[dp]
		}
	}
	return best, bestN > 0
}

// TypicalDuration is signal 4: the median actual length of the sessions the
// listener finished, in seconds. Median rather than mean so one abandoned
// three-hour experiment does not double the suggestion. Zero when nothing
// has been finished yet.
func (s Signals) TypicalDuration() int {
	if len(s.CompletedDurations) == 0 {
		return 0
	}
	d := append([]int(nil), s.CompletedDurations...)
	sort.Ints(d)
	return d[len(d)/2]
}

// SkipsByCategory is signal 6 aggregated by category.
func (s Signals) SkipsByCategory() map[string]int {
	out := map[string]int{}
	for _, l := range s.Listens {
		if !l.Completed && l.CategoryID != "" {
			out[l.CategoryID]++
		}
	}
	return out
}

// SkipsByConfession is signal 6 at the level the listener acted on.
func (s Signals) SkipsByConfession() map[string]int {
	out := map[string]int{}
	for _, l := range s.Listens {
		if !l.Completed && l.ConfessionID != "" {
			out[l.ConfessionID]++
		}
	}
	return out
}

// RepeatListens is signal 7: for each confession, the number of distinct
// sessions in which it was completed. A confession appears in the result only
// once it has been heard in at least RepeatThreshold sessions, because one
// listen is discovery and the second is the listener coming back.
func (s Signals) RepeatListens() map[string]int {
	sessionsFor := map[string]map[string]bool{}
	for _, l := range s.Listens {
		if !l.Completed || l.ConfessionID == "" || l.SessionID == "" {
			continue
		}
		if sessionsFor[l.ConfessionID] == nil {
			sessionsFor[l.ConfessionID] = map[string]bool{}
		}
		sessionsFor[l.ConfessionID][l.SessionID] = true
	}
	out := map[string]int{}
	for id, sessions := range sessionsFor {
		if n := len(sessions); n >= RepeatThreshold {
			out[id] = n
		}
	}
	return out
}

// RepeatThreshold is the number of separate sessions that turn a listen into
// repeat listening.
const RepeatThreshold = 2

// Present reports which of the seven signals carry any evidence for this
// listener. It is what "personalized: true" means in the API: at least one
// behavioural signal, not merely an interest the listener typed in.
func (s Signals) Present() []Signal {
	var out []Signal
	if len(s.CategoriesListenedTo()) > 0 {
		out = append(out, SignalCategories)
	}
	if _, n := s.CompletionRate(); n > 0 {
		out = append(out, SignalCompletionRate)
	}
	if _, ok := s.PreferredDaypart(); ok {
		out = append(out, SignalTimeOfDay)
	}
	if s.TypicalDuration() > 0 {
		out = append(out, SignalDuration)
	}
	if len(s.Favourites.Confessions)+len(s.Favourites.Categories)+len(s.Favourites.Voices) > 0 {
		out = append(out, SignalFavourites)
	}
	if len(s.SkipsByConfession()) > 0 {
		out = append(out, SignalSkips)
	}
	if len(s.RepeatListens()) > 0 {
		out = append(out, SignalRepeats)
	}
	return out
}
