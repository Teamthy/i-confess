package api

import (
	"context"
	"errors"
	"net/http"
	"sort"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/sessions"
	"github.com/Teamthy/i-confess/internal/store"
)

// The session playback lifecycle (§20, §46).
//
// These endpoints are the only writers of session state apart from the
// scheduler. Each one goes through the state machine in internal/sessions
// rather than assigning a status, which is what makes "a client cannot claim a
// completion it did not earn" a structural property instead of a convention.

// ownSession loads a session and asserts the caller owns it, writing the
// appropriate error response and returning false if anything is wrong.
func (h *Handler) ownSession(w http.ResponseWriter, r *http.Request) (*models.Session, bool) {
	sess, err := h.sess.ByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "session not found")
		return nil, false
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load session")
		return nil, false
	}
	if sess.UserID != h.userID(r) {
		httpx.WriteError(w, http.StatusForbidden, "not your session")
		return nil, false
	}
	return sess, true
}

// itemInSession reports whether an item belongs to a session. Every item write
// is keyed on the item alone, so a handler accepting an item id from a client
// has to establish this itself before writing anything.
func (h *Handler) itemInSession(ctx context.Context, sessionID, itemID string) bool {
	items, err := h.sess.Items(ctx, sessionID)
	if err != nil {
		return false
	}
	for i := range items {
		if items[i].ID == itemID {
			return true
		}
	}
	return false
}

// sessionState resolves a persisted status to a canonical state. A value that
// does not resolve means the row predates or postdates a vocabulary we
// understand, and guessing would be worse than refusing.
func sessionState(sess *models.Session) (sessions.State, bool) {
	return sessions.Parse(sess.Status)
}

// transition moves a session to the given state, or writes a 409 explaining
// why that is not allowed from where the session currently is.
func (h *Handler) transition(w http.ResponseWriter, r *http.Request, sess *models.Session, to sessions.State) bool {
	from, ok := sessionState(sess)
	if !ok {
		httpx.WriteError(w, http.StatusConflict, "session is in an unrecognised state")
		return false
	}
	if !sessions.CanTransition(from, to) {
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error": sessions.Reason(from, to),
			"from":  string(from),
			"to":    string(to),
			"code":  "INVALID_TRANSITION",
		})
		return false
	}
	if err := h.sess.UpdateStatus(r.Context(), sess.ID, string(to)); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update session")
		return false
	}
	sess.Status = string(to)
	return true
}

// firstQueued returns the earliest item still waiting, or nil.
func firstQueued(items []models.SessionItem) *models.SessionItem {
	sorted := append([]models.SessionItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })
	for i := range sorted {
		if sessions.NormalizeItemStatus(sorted[i].Status) == string(sessions.ItemQueued) {
			return &sorted[i]
		}
	}
	return nil
}

// startSession begins playback.
//
// Wrapped in the idempotency middleware: a phone on a poor connection will
// retry this, and two starts must not both stamp started_at or both emit a
// session-started event.
func (h *Handler) startSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.ownSession(w, r)
	if !ok {
		return
	}
	if !h.transition(w, r, sess, sessions.Active) {
		return
	}
	// Put the first waiting item on screen so the queue view and the player
	// agree about what is playing.
	items, err := h.sess.Items(r.Context(), sess.ID)
	if err == nil {
		if next := firstQueued(items); next != nil {
			_ = h.sess.SetPlayingItem(r.Context(), sess.ID, next.ID, string(sessions.ItemPlaying))
		}
	}
	sess.Items = items
	httpx.WriteJSON(w, http.StatusOK, sess)
}

func (h *Handler) pauseSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.ownSession(w, r)
	if !ok {
		return
	}
	if !h.transition(w, r, sess, sessions.Paused) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sess)
}

func (h *Handler) resumeSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.ownSession(w, r)
	if !ok {
		return
	}
	// ACTIVE is reachable from both PAUSED (a deliberate pause) and
	// INTERRUPTED (a phone call, a Bluetooth drop, the OS reclaiming audio);
	// the state machine distinguishes them and both may resume.
	if !h.transition(w, r, sess, sessions.Active) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sess)
}

// completeSession closes a session out.
//
// The state machine is what stops this being a way to inflate completion
// stats: COMPLETED is only reachable from ACTIVE, PAUSED or INTERRUPTED, so a
// session that never played cannot be completed no matter what the client
// sends.
func (h *Handler) completeSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.ownSession(w, r)
	if !ok {
		return
	}
	if !h.transition(w, r, sess, sessions.Completed) {
		return
	}

	items, _ := h.sess.Items(r.Context(), sess.ID)
	completed, total := 0, len(items)
	for i := range items {
		status := sessions.NormalizeItemStatus(items[i].Status)
		// Whatever was playing when the listener finished counts as heard.
		// Items still queued were not played and are not counted, so the
		// completion screen reports something true.
		if status == string(sessions.ItemPlaying) {
			_ = h.sess.UpdateItemStatus(r.Context(), items[i].ID, string(sessions.ItemCompleted))
			status = string(sessions.ItemCompleted)
		}
		if sessions.CountsTowardsCompletion(sessions.ItemStatus(status)) {
			completed++
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":          sess.Status,
		"session":         sess,
		"items_total":     total,
		"items_completed": completed,
	})
}

// getSessionQueue returns the queue with each item's status and the stored
// resume point, which is what the player needs to render the up-next list and
// pick up where the listener left off.
func (h *Handler) getSessionQueue(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.ownSession(w, r)
	if !ok {
		return
	}
	items, err := h.sess.Items(r.Context(), sess.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load queue")
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Position < items[j].Position })

	counts := map[string]int{}
	for i := range items {
		status := sessions.NormalizeItemStatus(items[i].Status)
		items[i].Status = status
		counts[status]++
	}

	out := map[string]any{
		"session_id":      sess.ID,
		"status":          sess.Status,
		"items":           items,
		"counts":          counts,
		"items_total":     len(items),
		"items_completed": counts[string(sessions.ItemCompleted)],
	}
	if prog, err := h.sess.Progress(r.Context(), sess.ID); err == nil {
		out["progress"] = prog
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// syncProgress records where the listener is (§36).
//
// Local-first clients push this opportunistically, so it arrives out of order
// and from several devices. The store resolves that by timestamp: a stale
// update is discarded and the newer stored state is returned with applied
// false, so a second device never rewinds the first.
func (h *Handler) syncProgress(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.ownSession(w, r)
	if !ok {
		return
	}
	var req struct {
		QueueItemID   string `json:"queue_item_id"`
		PositionMS    int64  `json:"position_ms"`
		DeviceID      string `json:"device_id"`
		LastUpdatedAt string `json:"last_updated_at"`
		ItemStatus    string `json:"item_status"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PositionMS < 0 {
		httpx.WriteError(w, http.StatusBadRequest, "position_ms must not be negative")
		return
	}

	// The item id is client-supplied and every item write below is keyed on the
	// item alone, so the session boundary has to be established here. Without
	// it any queue item in the system could be rewritten - or claimed as this
	// user's resume point - through a session its owner never authorised.
	if req.QueueItemID != "" && !h.itemInSession(r.Context(), sess.ID, req.QueueItemID) {
		httpx.WriteError(w, http.StatusNotFound, "item is not in this session")
		return
	}

	// An item status change rides along with the progress push so a client
	// does not need a second round trip per track.
	if req.ItemStatus != "" {
		canonical := sessions.NormalizeItemStatus(req.ItemStatus)
		if canonical == "" {
			httpx.WriteError(w, http.StatusBadRequest, "unknown item_status")
			return
		}
		if req.QueueItemID == "" {
			httpx.WriteError(w, http.StatusBadRequest, "queue_item_id is required with item_status")
			return
		}
		if err := h.sess.UpdateItemStatus(r.Context(), req.QueueItemID, canonical); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "failed to update item")
			return
		}
	}

	counts, _ := h.sess.CountItems(r.Context(), sess.ID)
	stored, applied, err := h.sess.SaveProgress(r.Context(), &models.SessionProgress{
		SessionID:      sess.ID,
		UserID:         sess.UserID,
		QueueItemID:    req.QueueItemID,
		PositionMS:     req.PositionMS,
		CompletedItems: counts[string(sessions.ItemCompleted)],
		DeviceID:       req.DeviceID,
		LastUpdatedAt:  req.LastUpdatedAt,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save progress")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"applied":  applied,
		"progress": stored,
	})
}

// skipSessionItem marks an item skipped and advances the queue.
//
// Skipping is recorded rather than merely navigated past: it is the signal
// recommendations use to offer less of what a listener does not want (§18),
// and it is kept distinct from failure so an infrastructure problem is never
// read as a matter of taste.
func (h *Handler) skipSessionItem(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.ownSession(w, r)
	if !ok {
		return
	}
	var req struct {
		ItemID string `json:"item_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.ItemID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "item_id is required")
		return
	}

	items, err := h.sess.Items(r.Context(), sess.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load queue")
		return
	}
	// The item must belong to this session. Without this check an id from
	// another session would be rewritten through an endpoint the owner of
	// that session never authorised.
	var target *models.SessionItem
	for i := range items {
		if items[i].ID == req.ItemID {
			target = &items[i]
			break
		}
	}
	if target == nil {
		httpx.WriteError(w, http.StatusNotFound, "item is not in this session")
		return
	}
	if err := h.sess.UpdateItemStatus(r.Context(), req.ItemID, string(sessions.ItemSkipped)); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to skip item")
		return
	}

	var next *models.SessionItem
	for i := range items {
		if items[i].ID == req.ItemID {
			continue
		}
		if sessions.NormalizeItemStatus(items[i].Status) == string(sessions.ItemQueued) {
			candidate := &items[i]
			if next == nil || candidate.Position < next.Position {
				next = candidate
			}
		}
	}
	if next != nil {
		_ = h.sess.SetPlayingItem(r.Context(), sess.ID, next.ID, string(sessions.ItemPlaying))
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"skipped":  req.ItemID,
		"next":     next,
		"session":  sess,
		"advanced": next != nil,
	})
}
