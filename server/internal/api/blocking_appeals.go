package api

import (
	"errors"
	"log"
	"net/http"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/moderation"
	"github.com/Teamthy/i-confess/internal/store"
)

// Blocking and appeals (PHASE 42, master-plan 32).
//
// These two surfaces complete a moderation system that could report and decide
// but could not be answered and had no self-service boundary. They are
// deliberately asymmetric:
//
//   - blocking is a listener acting on their own view, with no moderator
//     involved and nothing shown to the blocked account;
//   - appealing is a listener asking a moderator to reconsider, which is work
//     that lands in the same queue as everything else a human has to read.

// createBlock records a boundary against another account.
//
// A self-block is a 400 rather than a no-op, because a listener who reaches for
// "block" on their own account is confused about what the button does and
// silently filtering their own content from their own feed would be the worst
// available answer.
func (h *Handler) createBlock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.UserID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "user_id required")
		return
	}
	blocker := h.userID(r)
	if blocker == req.UserID {
		httpx.WriteError(w, http.StatusBadRequest, "you cannot block yourself")
		return
	}
	rec, created, err := h.blocks.Block(r.Context(), blocker, req.UserID, req.Reason)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "no such account")
			return
		}
		log.Printf("moderation: block failed: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "failed to record the block")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.WriteJSON(w, status, rec)
}

// listBlocks returns the accounts this listener has blocked. The list is the
// listener's own and is never visible to anyone else.
func (h *Handler) listBlocks(w http.ResponseWriter, r *http.Request) {
	blocks, err := h.blocks.Blocks(r.Context(), h.userID(r))
	if err != nil {
		log.Printf("moderation: list blocks failed: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load blocks")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"blocks": blocks, "count": len(blocks)})
}

// deleteBlock removes a boundary. Idempotent: asking for a boundary to be gone
// when it is already gone is not an error.
func (h *Handler) deleteBlock(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("userId")
	if target == "" {
		httpx.WriteError(w, http.StatusBadRequest, "user id required")
		return
	}
	if target == h.userID(r) {
		httpx.WriteError(w, http.StatusBadRequest, "you cannot block or unblock yourself")
		return
	}
	if err := h.blocks.Unblock(r.Context(), h.userID(r), target); err != nil {
		log.Printf("moderation: unblock failed: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "failed to remove the block")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// createAppeal files an appeal against a decision made about this listener.
//
// The refusals are the point of the handler. Appealing requires the decision to
// exist, to belong to the caller, and to have actually been made: a report that
// is still open has not been dismissed, and a confession still in review has not
// been rejected. Accepting any of those would fill the queue with grievances
// against decisions nobody made.
func (h *Handler) createAppeal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DecisionType string `json:"decision_type"`
		DecisionID   string `json:"decision_id"`
		Statement    string `json:"statement"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.DecisionID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "decision_id required")
		return
	}
	rec, created, err := h.mod.Appeal(r.Context(), h.userID(r), req.DecisionType, req.DecisionID, req.Statement)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, "no such decision")
		case errors.Is(err, store.ErrAppealNotOwned):
			httpx.WriteError(w, http.StatusForbidden, "that decision was not made about your account")
		case errors.Is(err, store.ErrAppealableDecisionNotFound):
			httpx.WriteError(w, http.StatusConflict, err.Error())
		case isValidation(err):
			// Statement bounds are the caller's fault and must not be reported
			// as a server failure.
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
		default:
			log.Printf("moderation: appeal failed: %v", err)
			httpx.WriteError(w, http.StatusInternalServerError, "failed to file the appeal")
		}
		return
	}
	h.recordAudit(r, "appeal_created", req.DecisionType, req.DecisionID, "appeal filed", rec.Status)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.WriteJSON(w, status, rec)
}

// isValidation reports whether an error is a caller-fixable input problem. The
// domain validators return plain errors with a message meant for a person, so
// the classification is by message shape rather than a sentinel that the
// moderation package does not define.
func isValidation(err error) bool {
	if err == nil {
		return false
	}
	var terr *moderation.AppealTransitionError
	if errors.As(err, &terr) {
		return false
	}
	msg := err.Error()
	for _, prefix := range []string{
		"appeal statement must be",
		"a block needs",
		"an account cannot block itself",
		"block reason must be",
		"unknown appeal decision",
	} {
		if len(msg) >= len(prefix) && msg[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// listAppeals returns this listener's appeals with the moderator's reasoning
// once there is any.
func (h *Handler) listAppeals(w http.ResponseWriter, r *http.Request) {
	appeals, err := h.mod.AppealsFor(r.Context(), h.userID(r))
	if err != nil {
		log.Printf("moderation: list appeals failed: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load appeals")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"appeals": appeals, "count": len(appeals)})
}

// adminDecideAppeal closes an appeal. Overturning reopens the work the original
// decision closed; it does not publish or resolve anything, because an appeal
// succeeding means the first decision was wrong, not that the opposite one is
// right.
func (h *Handler) adminDecideAppeal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Decision == "" {
		httpx.WriteError(w, http.StatusBadRequest, "decision required")
		return
	}
	if !moderation.ValidAppealDecision(req.Decision) {
		httpx.WriteError(w, http.StatusBadRequest, "decision must be upheld or overturned")
		return
	}
	rec, err := h.mod.DecideAppeal(r.Context(), id, req.Decision, actor(r), req.Note)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, "no such appeal")
		case errors.Is(err, store.ErrAppealNotDecidable):
			httpx.WriteError(w, http.StatusConflict, "that appeal has already been decided")
		default:
			var terr *moderation.AppealTransitionError
			if errors.As(err, &terr) {
				httpx.WriteError(w, http.StatusConflict, err.Error())
				return
			}
			log.Printf("moderation: appeal decision failed: %v", err)
			httpx.WriteError(w, http.StatusInternalServerError, "failed to decide the appeal")
		}
		return
	}
	h.recordAudit(r, "appeal_"+req.Decision, rec.DecisionType, rec.DecisionID,
		"appeal closed as "+req.Decision, rec.Status)
	httpx.WriteJSON(w, http.StatusOK, rec)
}
