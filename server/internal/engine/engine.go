// Package engine implements the Session Engine: the deterministic MVP logic that
// turns (categories, duration, voice, preferences) into an ordered confession session.
package engine

import (
	"context"
	"errors"
	"sort"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/sessions"
	"github.com/Teamthy/i-confess/internal/store"
)

var (
	ErrNoContent = errors.New("no published content available for the selected categories")
	ErrNoVoice   = errors.New("no available voice")
)

// Engine composes a session from published content with ready audio.
type Engine struct {
	content *store.ContentStore
	audio   *store.AudioStore
	users   *store.UserStore
}

func New(content *store.ContentStore, audio *store.AudioStore, users *store.UserStore) *Engine {
	return &Engine{content: content, audio: audio, users: users}
}

// Request is the session-engine input.
type Request struct {
	UserID          string
	CategoryIDs     []string
	DurationSeconds int
	VoiceID         string
	// Strategy controls how the requested duration is reconciled with the
	// complete confessions available. Empty means DefaultStrategy.
	Strategy Strategy
	// CategoryWeights biases how often each category appears in the queue,
	// keyed by category id. A category omitted from a non-empty map is not
	// selected; see weightedOrder.
	CategoryWeights map[string]float64
	// FavoriteIDs are confessions the listener has favourited. They are
	// selected ahead of other content in the same category.
	FavoriteIDs map[string]bool
	// RecentIDs are confessions the listener has lately played. They are held
	// back so the same words are not repeated day after day.
	RecentIDs map[string]bool

	// MaxDurationSeconds is the ceiling the caller's plan allows. Zero means
	// "no plan ceiling", in which case only the product bounds below apply.
	//
	// It lives here rather than only in the HTTP handlers because the handlers
	// are not the only callers. A schedule fired by the scheduler, or any
	// caller added later, has to hit the same limit; a control that lives at
	// one transport is a control four entry points can forget. PHASE 13 found
	// exactly that: the ad-hoc path returned 402, and the schedule path built
	// a free user a three-hour session.
	MaxDurationSeconds int
}

// Product bounds on a session length. The client offers presets of 10 to 180
// minutes plus a custom length; these are the rails behind them.
const (
	MinSessionSeconds = 60
	MaxSessionSeconds = 3 * 3600
)

var (
	// ErrDurationTooShort and ErrDurationTooLong reject lengths outside the
	// product bounds.
	ErrDurationTooShort = errors.New("duration is below the minimum session length")
	ErrDurationTooLong  = errors.New("duration is above the maximum session length")

	// ErrDurationExceedsPlan is returned when the requested length is inside
	// the product bounds but above what the caller's plan allows. Handlers map
	// it to 402.
	ErrDurationExceedsPlan = errors.New("duration exceeds the caller's plan limit")
)

// validateDuration is the single place session length is checked. Every Build
// goes through it, whatever the entry point.
func validateDuration(req Request) error {
	if req.DurationSeconds < MinSessionSeconds {
		return ErrDurationTooShort
	}
	if req.DurationSeconds > MaxSessionSeconds {
		return ErrDurationTooLong
	}
	if req.MaxDurationSeconds > 0 && req.DurationSeconds > req.MaxDurationSeconds {
		return ErrDurationExceedsPlan
	}
	return nil
}

// Build assembles an ordered, deterministic session.
//
// Rules:
//  1. Only published confessions with a ready audio asset for the chosen voice are used.
//  2. Categories are cycled round-robin so each requested category is represented evenly.
//  3. Within a confession the longest variant that still fits the remaining budget wins.
//  4. If a premium voice is requested by a free user, it falls back to an available free voice.
//  5. The duration strategy decides where the queue is cut. Audio is never
//     truncated to hit a target, so the built length usually differs slightly
//     from the requested one; both are recorded on the returned session.
//
// Build is deterministic: the same request against the same content yields the
// same queue, so a session stays reproducible after it has been created.
func (e *Engine) Build(ctx context.Context, req Request) (*models.Session, error) {
	if req.DurationSeconds <= 0 {
		req.DurationSeconds = 1800
	}
	if err := validateDuration(req); err != nil {
		return nil, err
	}
	req.Strategy = NormalizeStrategy(string(req.Strategy))

	voice, downgraded, err := e.resolveVoice(ctx, req)
	if err != nil {
		return nil, err
	}

	// Collect eligible confessions (with audio for this voice), grouped by category.
	byCat := map[string][]*models.Confession{}
	available, err := e.audio.ConfessionIDsWithVoice(ctx, voice.ID)
	if err != nil {
		return nil, err
	}

	for _, catID := range req.CategoryIDs {
		confs, err := e.content.ConfessionsByCategory(ctx, catID, true)
		if err != nil {
			return nil, err
		}
		for i := range confs {
			c := &confs[i]
			if !available[c.ID] {
				continue
			}
			c.Variants, _ = e.content.Variants(ctx, c.ID)
			if len(c.Variants) == 0 {
				continue
			}
			byCat[catID] = append(byCat[catID], c)
		}
	}
	if len(byCat) == 0 {
		return nil, ErrNoContent
	}
	applyPreferences(byCat, req.FavoriteIDs, req.RecentIDs)

	items, actual, err := e.pack(ctx, req, voice.ID, byCat)
	if err != nil {
		// "Nothing fits" and "content too thin to cover the request" both mean
		// the caller cannot get a session for these inputs. ErrNoExactFit is
		// left distinct: it says the content exists but cannot land precisely
		// on the requested length, which the API should report differently
		// from having nothing to offer.
		if errors.Is(err, ErrNoFit) || errors.Is(err, ErrUnreachable) {
			return nil, ErrNoContent
		}
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNoContent
	}

	sess := &models.Session{
		UserID:          req.UserID,
		Type:            classifyType(actual),
		DurationSeconds: actual,
		TargetDuration:  req.DurationSeconds,
		ActualDuration:  actual,
		Strategy:        string(req.Strategy),
		VoiceID:         voice.ID,
		Status:          string(sessions.Ready),
		Items:           items,
	}
	if downgraded {
		sess.VoiceDowngraded = true
		sess.VoiceDowngradeReason = "premium_voice_requires_subscription"
	}
	return sess, nil
}

// resolveVoice validates the requested voice and applies the premium fallback.
//
// It returns whether a substitution occurred so the caller can tell the
// listener. Silently swapping a voice the user deliberately chose makes a
// deliberate paywall look like a bug.
func (e *Engine) resolveVoice(ctx context.Context, req Request) (v *models.Voice, downgraded bool, err error) {
	if req.VoiceID != "" {
		requested, err := e.audio.VoiceByID(ctx, req.VoiceID)
		if err != nil {
			return nil, false, err
		}
		if !requested.Premium {
			return requested, false, nil
		}
		// Entitlement is resolved server-side; the client never asserts a plan.
		plan, _ := e.users.Subscription(ctx, req.UserID)
		if plan == "premium" {
			return requested, false, nil
		}
		// A premium voice requested on a free plan falls back, and the caller
		// is told so.
		downgraded = true
	}
	// Default: the first active free voice.
	voices, err := e.audio.ListVoices(ctx)
	if err != nil {
		return nil, false, err
	}
	for i := range voices {
		if voices[i].Status == "active" && !voices[i].Premium {
			return &voices[i], downgraded, nil
		}
	}
	return nil, false, ErrNoVoice
}

// pack fills the requested duration from category-balanced content and then
// lets the duration strategy decide where to cut.
//
// It runs in two passes, and the split is deliberate. The first walks the
// categories round-robin, taking the longest variant that still fits what
// remains of the budget, and stops when nothing fits — that is the deepest
// under-fill the content allows. The second generates exactly one further item
// at its natural length, the only candidate that can overshoot. Plan then
// chooses a prefix of that sequence according to the strategy.
//
// Keeping the walk and the arithmetic apart is what stops the length rule from
// undoing content selection: Plan only ever decides how many leading items to
// keep, so round-robin ordering and category diversity survive whatever
// duration strategy the caller chose.
//
// Confessions repeat as needed to reach the requested duration, and audio is
// never truncated — the built length is allowed to differ slightly from the
// requested one instead.
func (e *Engine) pack(ctx context.Context, req Request, voiceID string, byCat map[string][]*models.Confession) ([]models.SessionItem, int, error) {
	type variantOption struct {
		variant models.ConfessionVariant
		asset   models.AudioAsset
	}
	type confessionOption struct {
		conf     *models.Confession
		variants []variantOption // sorted descending by duration
	}

	// The walk order is the requested category order, expanded by weight.
	// Never range byCat: Go map iteration is randomised, and that would make
	// the queue — and therefore the session — differ between two identical
	// requests. Reproducibility is a hard requirement, since a created
	// session must stay stable as content changes.
	order := weightedOrder(req.CategoryIDs, req.CategoryWeights)

	catOptions := map[string][]*confessionOption{}
	built := map[string]bool{}
	for _, catID := range order {
		confs := byCat[catID]
		if len(confs) == 0 || built[catID] {
			continue
		}
		built[catID] = true
		for _, c := range confs {
			assets, _ := e.audio.AssetsFor(ctx, c.ID, voiceID)
			if len(assets) == 0 {
				continue
			}
			// Sort variants descending by duration for best-fit filling.
			sorted := append([]models.ConfessionVariant(nil), c.Variants...)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i].DurationSeconds > sorted[j].DurationSeconds })
			var opts []variantOption
			for _, v := range sorted {
				if asset, ok := matchAsset(assets, v.ID); ok {
					opts = append(opts, variantOption{variant: v, asset: asset})
				}
			}
			if len(opts) == 0 {
				continue
			}
			catOptions[catID] = append(catOptions[catID], &confessionOption{conf: c, variants: opts})
		}
	}

	// Keep the weighted expansion, minus categories that produced no playable
	// options, so a dominant category still gets its repeated slots.
	var catOrder []string
	for _, catID := range order {
		if len(catOptions[catID]) > 0 {
			catOrder = append(catOrder, catID)
		}
	}
	if len(catOrder) == 0 {
		return nil, 0, ErrNoFit
	}

	// emit produces the next item for a category, advancing its cursor.
	// A positive budget restricts the choice to variants that fit; a
	// non-positive budget takes the natural (longest) variant, which is how
	// the single overshoot candidate is produced.
	cursors := map[string]int{}
	emit := func(catID string, budget int) (models.SessionItem, bool) {
		opts := catOptions[catID]
		if len(opts) == 0 {
			return models.SessionItem{}, false
		}
		opt := opts[cursors[catID]%len(opts)]
		cursors[catID]++
		chosen := -1
		for i, vo := range opt.variants {
			if budget <= 0 || vo.variant.DurationSeconds <= budget {
				chosen = i
				break
			}
		}
		if chosen < 0 {
			return models.SessionItem{}, false
		}
		vo := opt.variants[chosen]
		return models.SessionItem{
			ConfessionID:    opt.conf.ID,
			VariantID:       vo.variant.ID,
			VoiceID:         voiceID,
			AudioAssetID:    vo.asset.ID,
			DurationSeconds: vo.variant.DurationSeconds,
			Status:          "queued",
			Title:           opt.conf.Title,
			Category:        catID,
			AudioURL:        vo.asset.URL,
			Text:            opt.conf.MediumText,
		}, true
	}

	// Pass one: the deepest under-fill.
	var items []models.SessionItem
	remaining := req.DurationSeconds
	for remaining > 0 {
		progressed := false
		for _, catID := range catOrder {
			it, ok := emit(catID, remaining)
			if !ok {
				continue // nothing in this confession fits what is left
			}
			it.Position = len(items)
			items = append(items, it)
			remaining -= it.DurationSeconds
			progressed = true
			if remaining <= 0 {
				break
			}
		}
		if !progressed {
			break
		}
	}

	// Pass two: one overshoot candidate, so OVER, CLOSEST and BALANCED have
	// something to weigh against the under-fill.
	for _, catID := range catOrder {
		if it, ok := emit(catID, 0); ok {
			it.Position = len(items)
			items = append(items, it)
			break
		}
	}
	if len(items) == 0 {
		return nil, 0, ErrNoFit
	}

	candidates := make([]int, len(items))
	for i := range items {
		candidates[i] = items[i].DurationSeconds
	}
	n, err := Plan(req.DurationSeconds, candidates, req.Strategy)
	if err != nil {
		return nil, 0, err
	}
	items = items[:n]

	// Positions must stay contiguous after the cut; a gap would leave the
	// player skipping a slot in the queue.
	total := 0
	for i := range items {
		items[i].Position = i
		total += items[i].DurationSeconds
	}
	return items, total, nil
}

func matchAsset(assets []models.AudioAsset, variantID string) (models.AudioAsset, bool) {
	for _, a := range assets {
		if a.VariantID == variantID {
			return a, true
		}
	}
	// If no variant-specific asset, fall back to any asset for the confession.
	if len(assets) > 0 {
		return assets[0], true
	}
	return models.AudioAsset{}, false
}

func classifyType(seconds int) string {
	switch {
	case seconds <= 15*60:
		return "quick"
	case seconds <= 60*60:
		return "standard"
	default:
		return "deep"
	}
}
