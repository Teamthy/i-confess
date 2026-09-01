// Package engine implements the Session Engine: the deterministic MVP logic that
// turns (categories, duration, voice, preferences) into an ordered confession session.
package engine

import (
	"context"
	"errors"
	"sort"

	"github.com/Teamthy/i-confess/internal/models"
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
}

// Build assembles an ordered, deterministic session.
//
// MVP rules:
//  1. Only published confessions with a ready audio asset for the chosen voice are used.
//  2. Categories are cycled round-robin so each requested category is represented evenly.
//  3. The shortest variant that still fits the remaining budget is preferred (packing).
//  4. If a premium voice is requested by a free user, it falls back to an available free voice.
func (e *Engine) Build(ctx context.Context, req Request) (*models.Session, error) {
	if req.DurationSeconds <= 0 {
		req.DurationSeconds = 1800
	}

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

	items, total := e.pack(ctx, req, voice.ID, byCat)
	if len(items) == 0 {
		return nil, ErrNoContent
	}

	sess := &models.Session{
		UserID:          req.UserID,
		Type:            classifyType(total),
		DurationSeconds: total,
		VoiceID:         voice.ID,
		Status:          "created",
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

// pack greedily fills the budget by cycling categories round-robin and, within
// each category, cycling its confessions. Confessions repeat as needed to reach
// the requested duration (matching PRD Â§19), and the longest variant that still
// fits the remaining budget is preferred so sessions fill efficiently.
func (e *Engine) pack(ctx context.Context, req Request, voiceID string, byCat map[string][]*models.Confession) ([]models.SessionItem, int) {
	type variantOption struct {
		variant models.ConfessionVariant
		asset   models.AudioAsset
	}
	type confessionOption struct {
		conf     *models.Confession
		variants []variantOption // sorted descending by duration
	}

	catOptions := map[string][]*confessionOption{}
	var catOrder []string
	for catID, confs := range byCat {
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
		if len(catOptions[catID]) > 0 {
			catOrder = append(catOrder, catID)
		}
	}
	if len(catOrder) == 0 {
		return nil, 0
	}

	cursors := map[string]int{}
	remaining := req.DurationSeconds
	var items []models.SessionItem
	idx := 0

	for remaining > 0 {
		progressed := false
		for _, catID := range catOrder {
			opts := catOptions[catID]
			if len(opts) == 0 {
				continue
			}
			opt := opts[cursors[catID]%len(opts)]
			// Longest variant that fits the remaining budget.
			chosen := -1
			for i, vo := range opt.variants {
				if vo.variant.DurationSeconds <= remaining {
					chosen = i
					break
				}
			}
			cursors[catID]++
			if chosen < 0 {
				continue // nothing in this confession fits the remaining time
			}
			vo := opt.variants[chosen]
			remaining -= vo.variant.DurationSeconds
			items = append(items, models.SessionItem{
				ConfessionID:    opt.conf.ID,
				VariantID:       vo.variant.ID,
				VoiceID:         voiceID,
				AudioAssetID:    vo.asset.ID,
				Position:        idx,
				DurationSeconds: vo.variant.DurationSeconds,
				Status:          "queued",
				Title:           opt.conf.Title,
				Category:        catID,
				AudioURL:        vo.asset.URL,
				Text:            opt.conf.MediumText,
			})
			idx++
			progressed = true
			if remaining <= 0 {
				break
			}
		}
		if !progressed {
			break // no variant fits the remaining budget
		}
	}
	total := req.DurationSeconds - remaining
	return items, total
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
