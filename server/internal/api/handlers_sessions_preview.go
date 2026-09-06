package api

import (
	"errors"
	"net/http"

	"github.com/Teamthy/i-confess/internal/engine"
	"github.com/Teamthy/i-confess/internal/httpx"
)

// previewSession dry-runs engine.Build without persisting (§5.3).
// Builder preview `30 MIN • 12 • Voices` must match the actual session
// that POST /v1/sessions creates — otherwise the user is shown a fiction.
//
// The client sends the same shape as createSession; the server returns
// the would-be snapshot (target/actual, item count, voice downgrade) and
// a truncated queue preview (first 3 items) so the builder can render
// without committing an outbox event.
func (h *Handler) previewSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CategoryIDs     []string           `json:"category_ids"`
		CategoryWeights map[string]float64 `json:"category_weights"`
		DurationSeconds int                `json:"duration_seconds"`
		DurationPreset  string             `json:"duration_preset"`
		Strategy        string             `json:"strategy"`
		VoiceID         string             `json:"voice_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.CategoryIDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "at least one category is required")
		return
	}
	if req.DurationSeconds == 0 && req.DurationPreset != "" {
		if secs, ok := engine.PresetFor(req.DurationPreset); ok {
			req.DurationSeconds = secs
		} else {
			httpx.WriteError(w, http.StatusBadRequest, "unknown duration_preset")
			return
		}
	}
	if req.DurationSeconds < 60 || req.DurationSeconds > 3*3600 {
		httpx.WriteError(w, http.StatusBadRequest, "duration must be 1 minute to 3 hours")
		return
	}
	if req.Strategy != "" && !engine.IsValidStrategy(req.Strategy) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid strategy")
		return
	}
	start := engine.Strategy(req.Strategy)
	if start == "" {
		start = engine.StrategyBalanced
	}

	ent := h.entitlementsFor(r.Context(), h.userID(r))
	if max := ent.MaxSessionSeconds(); req.DurationSeconds > max {
		writePlanLimit(w, ent)
		return
	}

	favs, _ := h.eng.ListFavorites(r.Context(), h.userID(r), "confession")
	favMap := map[string]bool{}
	for _, f := range favs {
		favMap[f.EntityID] = true
	}
	history, _ := h.eng.History(r.Context(), h.userID(r), 20)
	recentMap := map[string]bool{}
	for i, rec := range history {
		if i >= 10 {
			break
		}
		if rec.ConfessionID != "" {
			recentMap[rec.ConfessionID] = true
		}
	}

	sess, err := h.engn.Build(r.Context(), engine.Request{
		UserID:          h.userID(r),
		CategoryIDs:     req.CategoryIDs,
		CategoryWeights: req.CategoryWeights,
		DurationSeconds: req.DurationSeconds,
		Strategy:        start,
		VoiceID:         req.VoiceID,
		FavoriteIDs:     favMap,
		RecentIDs:       recentMap,

		MaxDurationSeconds: ent.MaxSessionSeconds(),
	})
	if errors.Is(err, engine.ErrDurationExceedsPlan) {
		writePlanLimit(w, ent)
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusUnprocessableEntity, "no content for the selected categories and voice")
		return
	}

	// Sign the full session, then truncate for preview response
	h.signSessionAudio(r.Context(), sess, ent)
	totalItems := len(sess.Items)
	previewItems := sess.Items
	if len(previewItems) > 3 {
		previewItems = previewItems[:3]
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"target_duration":  sess.TargetDuration,
		"actual_duration":  sess.ActualDuration,
		"target_seconds":   req.DurationSeconds,
		"actual_seconds":   sess.ActualDuration,
		"total_items":      totalItems,
		"items_preview":    previewItems,
		"voice_downgraded": sess.VoiceDowngraded,
		"voice_id":         sess.VoiceID,
		"strategy":         string(start),
		"display":          previewDisplay(req.DurationSeconds, totalItems, sess.VoiceID, sess.VoiceDowngraded),
	})
}

func previewDisplay(seconds, count int, voiceID string, downgraded bool) string {
	min := seconds / 60
	s := itoaPreview(min) + " MIN • " + itoaPreview(count)
	if voiceID != "" {
		s += " • " + voiceID
	} else {
		s += " • Voices"
	}
	if downgraded {
		s += " ↓"
	}
	return s
}

func itoaPreview(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
