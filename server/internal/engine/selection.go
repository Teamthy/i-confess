package engine

import (
	"sort"

	"github.com/Teamthy/i-confess/internal/models"
)

// weightRoundSize is the granularity at which category weights are apportioned.
//
// Ten is chosen because the weights in the product spec are percentages
// (Healing 40%, Faith 20%, Peace 20%, Purpose 20%), and a round of ten turns
// those into exactly 4/2/2/2 slots. A coarser round — say four, one per
// category — would round 40% down to a single slot and silently discard the
// weighting the listener asked for.
const weightRoundSize = 10

// weightedOrder expands the requested categories into the round-robin order the
// packer walks, honouring CategoryWeights (§18).
//
// With no usable weights it returns the requested order unchanged, one slot per
// category, which is plain round-robin.
//
// A category that is absent from the weights map, or carries a non-positive
// weight, is not selected. That is deliberate: sending weights is an explicit
// statement of intent, and quietly giving unweighted categories an equal share
// would make "Healing 100%" impossible to express. Callers who want every
// category represented must weight every category.
//
// The result is deterministic — ties break on category id, never on map
// iteration order — because sessions must be reproducible.
func weightedOrder(catIDs []string, weights map[string]float64) []string {
	if len(catIDs) == 0 {
		return nil
	}

	// Keep the requested categories that carry a usable weight.
	var picked []string
	total := 0.0
	for _, id := range catIDs {
		if w := weights[id]; w > 0 {
			picked = append(picked, id)
			total += w
		}
	}
	if len(picked) == 0 || total <= 0 {
		return append([]string(nil), catIDs...)
	}

	// Sort by weight descending, then id, so that every tie below resolves
	// the same way on every call.
	sort.SliceStable(picked, func(i, j int) bool {
		wi, wj := weights[picked[i]], weights[picked[j]]
		if wi != wj {
			return wi > wj
		}
		return picked[i] < picked[j]
	})

	// Largest-remainder apportionment: whole slots first, then hand the
	// leftovers to the largest fractional parts in the order above.
	quota := make([]float64, len(picked))
	slots := make([]int, len(picked))
	assigned := 0
	for i, id := range picked {
		quota[i] = weights[id] / total * weightRoundSize
		slots[i] = int(quota[i])
		assigned += slots[i]
	}
	fracs := make([]float64, len(picked))
	for i := range picked {
		fracs[i] = quota[i] - float64(int(quota[i]))
	}
	order := make([]int, len(picked))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return fracs[order[a]] > fracs[order[b]] })
	for k := 0; assigned < weightRoundSize && k < len(order); k++ {
		slots[order[k]]++
		assigned++
	}

	// Interleave: one slot per pass, heaviest first. Spreading the slots
	// rather than emitting them in blocks keeps a dominant category from
	// monopolising the start of the session.
	out := make([]string, 0, weightRoundSize)
	for pass := 1; ; pass++ {
		emitted := false
		for _, i := range order {
			if slots[i] >= pass {
				out = append(out, picked[i])
				emitted = true
			}
		}
		if !emitted {
			break
		}
	}
	return out
}

// applyPreferences narrows and reorders the candidate pool using the
// listener's favourites and recent history (§18).
//
// Recency is applied first and wins over favourites: a confession heard
// yesterday is held back even if the listener favourited it. Repeating the same
// words two days running is the more irritating failure, and the favourite is
// still in the library for the next session.
//
// Both steps are conservative. A category whose entire pool was played
// recently keeps its full pool rather than being dropped, because a thin
// library should degrade to repetition, not to an empty session.
func applyPreferences(byCat map[string][]*models.Confession, favorites, recent map[string]bool) {
	if len(recent) > 0 {
		for catID, confs := range byCat {
			fresh := make([]*models.Confession, 0, len(confs))
			for _, c := range confs {
				if !recent[c.ID] {
					fresh = append(fresh, c)
				}
			}
			if len(fresh) > 0 {
				byCat[catID] = fresh
			}
		}
	}
	if len(favorites) > 0 {
		for catID, confs := range byCat {
			// Stable, so non-favourites keep the ordering the content store
			// gave them and favourites sort ahead of them.
			sort.SliceStable(confs, func(i, j int) bool {
				return favorites[confs[i].ID] && !favorites[confs[j].ID]
			})
			byCat[catID] = confs
		}
	}
}
