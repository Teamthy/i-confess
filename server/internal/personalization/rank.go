package personalization

import (
	"fmt"
	"sort"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
)

// Weights are the deterministic rules. They are named so a reader can see the
// whole policy in one place and a test can prove each signal moves the
// result. The magnitudes encode a simple order of importance: what the
// listener asked for outranks what they did, what they did outranks when they
// did it, and a skip costs more than a listen earns so a category the listener
// keeps passing over sinks even if it is also one they sometimes finish.
const (
	WeightInterest       = 10.0 // per unit of explicit interest weight
	WeightCategoryListen = 3.0  // per completed item in the category
	WeightCategoryFav    = 5.0  // category is a favourite
	WeightConfessionFav  = 6.0  // confession is a favourite (confession ranking)
	WeightDaypartMatch   = 2.0  // per completed listen in the current daypart
	WeightCategorySkip   = 4.0  // per skipped item in the category
	WeightConfessionSkip = 5.0  // per skip of the confession itself
	WeightRepeat         = 2.0  // per repeat session of a confession (≥ threshold)

	// Category and confession fan-out, as in the v1 endpoint the clients
	// already render.
	TopCategories          = 6
	ConfessionsPerCategory = 2
	MaxConfessions         = 12
	MaxListenAgain         = 6
)

// Input is everything the ranking reads. Catalogue and evidence arrive
// separately so the same evidence can rank a different catalogue (a category
// page, a template) without re-deriving anything.
type Input struct {
	Categories []models.Category
	// Confessions is the published catalogue keyed by category id, already in
	// the catalogue's own sort order.
	Confessions map[string][]models.Confession
	// Interests are the listener's explicit and inferred weights by category.
	Interests map[string]float64
	Signals   Signals
	// Now is the request time; it is bucketed in the listener's timezone to
	// decide which daypart "now" is.
	Now time.Time
}

// Recommendation is the ranked output with the reasons attached.
type Recommendation struct {
	Categories  []RankedCategory
	Confessions []RankedConfession
	// ListenAgain is the repeat-listening rail: confessions the listener has
	// come back to, most returned-to first.
	ListenAgain []RankedConfession
	// SuggestedDurationSeconds is the session length to offer, derived from
	// signals 2 and 4 and snapped to the builder ladder. Zero when the listener
	// has no completed session yet, so the client keeps its default.
	SuggestedDurationSeconds int
	// Daypart is the bucket the request falls in, in the listener's timezone.
	Daypart Daypart
	// PreferredDaypart is when this listener usually listens, if known.
	PreferredDaypart Daypart
	// Present lists the signals that had evidence.
	Present []Signal
}

// RankedCategory is a category with its score and the reasons it earned it.
type RankedCategory struct {
	Category models.Category `json:"category"`
	Score    float64         `json:"score"`
	Reasons  []string        `json:"reasons,omitempty"`
}

// RankedConfession is a confession with its score and reasons.
type RankedConfession struct {
	Confession models.Confession `json:"confession"`
	Score      float64           `json:"score"`
	Reasons    []string          `json:"reasons,omitempty"`
}

// ladder is the builder's duration ladder in seconds. Mirrors
// engine.Presets(); the engine imports the store, which will import this
// package, so the numbers are repeated here and TestSuggestedDurationLadder
// keeps them from drifting.
var ladder = []int{10 * 60, 15 * 60, 30 * 60, 45 * 60, 60 * 60, 90 * 60, 120 * 60, 180 * 60}

// Ladder returns the duration ladder this package snaps to.
func Ladder() []int { return append([]int(nil), ladder...) }

// SuggestDuration combines signal 4 (typical duration) with signal 2
// (completion rate). The typical length is snapped to the nearest rung; a
// listener who finishes fewer than half of the items they start is offered
// one rung shorter, because a session that is not finished is not a ritual,
// and one who finishes nearly everything is offered one rung longer. The rate
// only counts once it rests on at least MinRateSample items.
func SuggestDuration(typical int, rate float64, sample int) int {
	if typical <= 0 {
		return 0
	}
	idx := 0
	for i, rung := range ladder {
		if abs(rung-typical) < abs(ladder[idx]-typical) {
			idx = i
		}
	}
	if sample >= MinRateSample {
		switch {
		case rate < LowCompletionRate && idx > 0:
			idx--
		case rate >= HighCompletionRate && idx < len(ladder)-1:
			idx++
		}
	}
	return ladder[idx]
}

const (
	// MinRateSample is how many completed-or-skipped items a completion rate
	// needs before it is allowed to move the suggested duration.
	MinRateSample = 3
	// LowCompletionRate and HighCompletionRate are the two thresholds the
	// duration rule reacts to.
	LowCompletionRate  = 0.5
	HighCompletionRate = 0.9
)

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Rank applies the rules. It is a pure function of its input.
func Rank(in Input) Recommendation {
	sig := in.Signals
	listens := sig.CategoriesListenedTo()
	dayparts := sig.DaypartsByCategory()
	catSkips := sig.SkipsByCategory()
	confSkips := sig.SkipsByConfession()
	repeats := sig.RepeatListens()
	rate, sample := sig.CompletionRate()
	nowPart := DaypartOf(in.Now.In(sig.loc()))
	preferred, _ := sig.PreferredDaypart()

	// Repeats roll up to the category so a category the listener keeps
	// returning to rises even without an explicit interest in it.
	repeatByCategory := map[string]int{}
	for catID, confs := range in.Confessions {
		for _, c := range confs {
			repeatByCategory[catID] += repeats[c.ID]
		}
	}

	cats := make([]RankedCategory, 0, len(in.Categories))
	for _, c := range in.Categories {
		rc := RankedCategory{Category: c}
		if w := in.Interests[c.ID]; w > 0 {
			rc.Score += WeightInterest * w
			rc.Reasons = append(rc.Reasons, "interest")
		}
		if n := listens[c.ID]; n > 0 {
			rc.Score += WeightCategoryListen * float64(n)
			rc.Reasons = append(rc.Reasons, fmt.Sprintf("listened %d", n))
		}
		if sig.Favourites.Categories[c.ID] {
			rc.Score += WeightCategoryFav
			rc.Reasons = append(rc.Reasons, "favourite")
		}
		if n := dayparts[c.ID][nowPart]; n > 0 {
			rc.Score += WeightDaypartMatch * float64(n)
			rc.Reasons = append(rc.Reasons, string(nowPart))
		}
		if n := catSkips[c.ID]; n > 0 {
			rc.Score -= WeightCategorySkip * float64(n)
			rc.Reasons = append(rc.Reasons, fmt.Sprintf("skipped %d", n))
		}
		if n := repeatByCategory[c.ID]; n > 0 {
			rc.Score += WeightRepeat * float64(n)
			rc.Reasons = append(rc.Reasons, fmt.Sprintf("repeated %d", n))
		}
		cats = append(cats, rc)
	}
	// Score desc, then the catalogue's own order, then name: the same input
	// must always produce the same list (v1 determinism requirement).
	sort.SliceStable(cats, func(i, j int) bool {
		if cats[i].Score != cats[j].Score {
			return cats[i].Score > cats[j].Score
		}
		if cats[i].Category.SortOrder != cats[j].Category.SortOrder {
			return cats[i].Category.SortOrder < cats[j].Category.SortOrder
		}
		return cats[i].Category.Name < cats[j].Category.Name
	})
	if len(cats) > TopCategories {
		cats = cats[:TopCategories]
	}

	var confs []RankedConfession
	for _, rc := range cats {
		ranked := rankConfessions(in.Confessions[rc.Category.ID], sig, confSkips, repeats)
		take := ConfessionsPerCategory
		if len(ranked) < take {
			take = len(ranked)
		}
		confs = append(confs, ranked[:take]...)
		if len(confs) >= MaxConfessions {
			confs = confs[:MaxConfessions]
			break
		}
	}

	// The repeat rail is the seventh signal made visible: it is ordered by
	// how often the listener came back, and a skipped repeat still counts,
	// because the listener chose it more often than they passed it.
	var again []RankedConfession
	for _, list := range in.Confessions {
		for _, c := range list {
			if n := repeats[c.ID]; n > 0 {
				again = append(again, RankedConfession{
					Confession: c, Score: float64(n),
					Reasons: []string{fmt.Sprintf("repeated %d", n)},
				})
			}
		}
	}
	sort.SliceStable(again, func(i, j int) bool {
		if again[i].Score != again[j].Score {
			return again[i].Score > again[j].Score
		}
		return again[i].Confession.Title < again[j].Confession.Title
	})
	if len(again) > MaxListenAgain {
		again = again[:MaxListenAgain]
	}

	return Recommendation{
		Categories:               cats,
		Confessions:              confs,
		ListenAgain:              again,
		SuggestedDurationSeconds: SuggestDuration(sig.TypicalDuration(), rate, sample),
		Daypart:                  nowPart,
		PreferredDaypart:         preferred,
		Present:                  sig.Present(),
	}
}

func rankConfessions(list []models.Confession, sig Signals, skips, repeats map[string]int) []RankedConfession {
	out := make([]RankedConfession, 0, len(list))
	for _, c := range list {
		rc := RankedConfession{Confession: c}
		if sig.Favourites.Confessions[c.ID] {
			rc.Score += WeightConfessionFav
			rc.Reasons = append(rc.Reasons, "favourite")
		}
		if n := repeats[c.ID]; n > 0 {
			rc.Score += WeightRepeat * float64(n)
			rc.Reasons = append(rc.Reasons, fmt.Sprintf("repeated %d", n))
		}
		if n := skips[c.ID]; n > 0 {
			rc.Score -= WeightConfessionSkip * float64(n)
			rc.Reasons = append(rc.Reasons, fmt.Sprintf("skipped %d", n))
		}
		out = append(out, rc)
	}
	// Stable: equal scores keep the catalogue's own order, which is the
	// deterministic tie-breaker the v1 endpoint promised.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}
