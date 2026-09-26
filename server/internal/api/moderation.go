package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/moderation"
	"github.com/Teamthy/i-confess/internal/ratelimit"
	"github.com/Teamthy/i-confess/internal/store"
)

// Moderation endpoints (§10, §22, §75).
//
// Three of these routes - the queue, user-confession review and confession QA -
// answered 501 until PHASE 31, and the schema had carried their tables since
// the baseline. A 501 answers a client honestly, but it also means the
// workflow does not exist: no user could file a report, no author could offer
// a confession, no reviewer could decide one, and the §75 audio QA gate had
// columns (qa_passed_at, qa_report) that nothing wrote.

// actor returns the authenticated subject for audit columns, or "" when the
// request somehow arrives without claims (the admin wrapper guarantees them;
// "" is persisted as NULL rather than a fabricated identity).
func actor(r *http.Request) string {
	if c := auth.FromContext(r); c != nil {
		return c.Sub
	}
	return ""
}

// createReport lets a signed-in user flag published content or a community
// post. It is the intake the queue and the case machinery are empty without.
func (h *Handler) createReport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		Reason     string `json:"reason"`
		Detail     string `json:"detail"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeCode(w, http.StatusBadRequest, "REPORT_INVALID", "invalid request body")
		return
	}
	req.EntityType = strings.TrimSpace(req.EntityType)
	req.EntityID = strings.TrimSpace(req.EntityID)
	req.Reason = strings.TrimSpace(req.Reason)
	req.Detail = strings.TrimSpace(req.Detail)

	if !moderation.ValidReportEntityType(req.EntityType) || req.EntityID == "" {
		writeCode(w, http.StatusBadRequest, "REPORT_INVALID",
			"entity_type must be one of: "+strings.Join(moderation.ReportableEntityTypes, ", "))
		return
	}
	if len(req.Reason) < moderation.ReasonMinLen || len(req.Reason) > moderation.ReasonMaxLen {
		writeCode(w, http.StatusUnprocessableEntity, "REPORT_INVALID",
			"a reason between 3 and 140 characters is required")
		return
	}
	if len(req.Detail) > moderation.DetailMaxLen {
		writeCode(w, http.StatusUnprocessableEntity, "REPORT_INVALID", "detail must be 2000 characters or fewer")
		return
	}

	userID := h.userID(r)
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// A report is work for a human. Throttling per account keeps one angry or
	// scripted user from turning the queue into a wall of duplicates the
	// moderator has to drain by hand; the unique index removes same-entity
	// duplicates, this bound covers the rest.
	if ok, retry := h.limiter.Allow("report:acct:"+userID, ratelimit.ReportSubmission); !ok {
		ratelimit.TooManyRequests(w, retry)
		return
	}

	exists, err := h.mod.ReportableEntityExists(r.Context(), req.EntityType, req.EntityID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not file the report")
		return
	}
	if !exists {
		// Deliberately vague (S80): the reporter learns nothing about whether
		// the id names unpublished, deleted or never-existing content.
		writeCode(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "content not found")
		return
	}

	rep, already, err := h.mod.CreateReport(r.Context(), userID, req.EntityType, req.EntityID, req.Reason, req.Detail)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not file the report")
		return
	}
	status := http.StatusCreated
	if already {
		status = http.StatusOK
	}
	if !already {
		// G-41: the admin-wide sink must see the moderation intake, not only
		// the queue. A duplicate filing is deliberately not a second audit
		// row: the queue holds one case and the trail says so too.
		h.recordAudit(r, "report_created", "report", rep.ID,
			req.Reason+" on "+req.EntityType+" "+req.EntityID, string(rep.Status))
	}
	// Same shape on both paths: a retry-safe client can treat a duplicate
	// submission identically to a first filing.
	httpx.WriteJSON(w, status, map[string]any{"report": rep, "already_reported": already})
}

// submitUserConfession offers the caller's own draft for review (§22). The
// POST /me/confessions route creates drafts; this is the act that puts one in
// front of a moderator.
func (h *Handler) submitUserConfession(w http.ResponseWriter, r *http.Request) {
	uc, err := h.mod.SubmitUserConfession(r.Context(), h.userID(r), r.PathValue("id"))
	var trans *moderation.UGCTransitionError
	switch {
	// A foreign id is reported as 404, not 403: confirming that the id exists
	// but belongs to someone else leaks exactly what §71 says not to.
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrForbidden):
		writeCode(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "confession not found")
	case errors.As(err, &trans):
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":   trans.Error(),
			"status":  string(trans.From),
			"allowed": moderation.UGCAllowedFrom(trans.From),
		})
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "could not submit the confession")
	default:
		h.recordAudit(r, "user_confession_submitted", "user_confession", uc.ID,
			"visibility="+uc.Visibility, uc.Status)
		httpx.WriteJSON(w, http.StatusOK, uc)
	}
}

// adminListModerationQueue is the moderator's work list: UGC awaiting review,
// open reports, editorial content parked in a human review state, oldest
// first in every section.
func (h *Handler) adminListModerationQueue(w http.ResponseWriter, r *http.Request) {
	q, err := h.mod.ModerationQueue(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not load the moderation queue")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, q)
}

// adminReviewUserConfession records the decision on a queued confession.
// Approving a confession whose author asked for a public audience publishes
// it; approving any other clears it for the author's own surfaces.
func (h *Handler) adminReviewUserConfession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Decision        string `json:"decision"`
		Note            string `json:"note"`
		RejectionReason string `json:"rejection_reason"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Decision = strings.TrimSpace(req.Decision)
	req.RejectionReason = strings.TrimSpace(req.RejectionReason)
	if req.Decision != string(moderation.UGCApproved) && req.Decision != string(moderation.UGCRejected) {
		httpx.WriteError(w, http.StatusBadRequest, "decision must be approved or rejected")
		return
	}
	if req.Decision == string(moderation.UGCRejected) && req.RejectionReason == "" {
		// Same rule the audio QA rejection keeps: a rejection with no reason
		// cannot be acted on by the author.
		httpx.WriteError(w, http.StatusUnprocessableEntity, "a rejection reason is required")
		return
	}

	uc, err := h.mod.ReviewUserConfession(r.Context(), r.PathValue("id"),
		req.Decision, actor(r), strings.TrimSpace(req.Note), req.RejectionReason)
	var trans *moderation.UGCTransitionError
	switch {
	case errors.Is(err, store.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "user confession not found")
	case errors.As(err, &trans):
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":   trans.Error(),
			"status":  string(trans.From),
			"allowed": moderation.UGCAllowedFrom(trans.From),
		})
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "could not review the user confession")
	default:
		// The action names the decision, the result names the state that
		// resulted: an approved public-intent confession lands published, and
		// the trail must be able to tell that apart from a private approval.
		action, detail := "user_confession_approved", req.Note
		if req.Decision == string(moderation.UGCRejected) {
			action, detail = "user_confession_rejected", req.RejectionReason
		}
		h.recordAudit(r, action, "user_confession", uc.ID, detail, uc.Status)
		httpx.WriteJSON(w, http.StatusOK, uc)
	}
}

// adminDecideReport closes an open report. Resolving the last open report on
// an entity also closes its moderation case, which is what removes the entity
// from the queue.
func (h *Handler) adminDecideReport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Decision = strings.TrimSpace(req.Decision)
	if !moderation.ValidReportDecision(req.Decision) {
		httpx.WriteError(w, http.StatusBadRequest, "decision must be resolved or dismissed")
		return
	}

	rep, err := h.mod.DecideReport(r.Context(), r.PathValue("id"), req.Decision, actor(r), strings.TrimSpace(req.Note))
	switch {
	case errors.Is(err, store.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "report not found")
	case errors.Is(err, store.ErrReportNotOpen):
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":  "the report is already closed",
			"status": rep.Status,
		})
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "could not decide the report")
	default:
		// The audit action is the outcome, not the verb: "report_resolved"
		// and "report_dismissed" are what an investigator searches for.
		h.recordAudit(r, "report_"+req.Decision, "report", rep.ID, req.Note, rep.Status)
		httpx.WriteJSON(w, http.StatusOK, rep)
	}
}

// adminQAConfession runs the §75 audio QA checklist against a confession in
// audio_qa. On a pass it moves to approved and qa_passed_at is set; on a fail
// nothing transitions, but the report is still persisted and returned, so a
// failed gate is distinguishable from a gate nobody ran.
func (h *Handler) adminQAConfession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Note string `json:"note"`
	}
	// An empty body is valid; the note is optional context for the report.
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	report, err := h.mod.RunConfessionQA(r.Context(), r.PathValue("id"), actor(r), req.Note)
	var gateState *store.ErrQAGateState
	switch {
	case errors.Is(err, store.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "confession not found")
	case errors.As(err, &gateState):
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":   gateState.Error(),
			"status":  gateState.Status,
			"allowed": []string{"audio_qa"},
		})
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "could not run the QA checklist")
	case !report.Passed:
		// The failed gate is audited with the names of the checks that kept
		// it shut: the 422 body is transient, the trail is not.
		h.recordAudit(r, "confession_qa_failed", "confession", r.PathValue("id"),
			failedQAChecks(report), "audio_qa")
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, report)
	default:
		h.recordAudit(r, "confession_qa_passed", "confession", r.PathValue("id"),
			req.Note, "approved")
		httpx.WriteJSON(w, http.StatusOK, report)
	}
}

// failedQAChecks names the checklist lines that did not pass, for the audit
// detail column.
func failedQAChecks(report models.QAReport) string {
	names := make([]string, 0, len(report.Checks))
	for _, c := range report.Checks {
		if !c.Passed {
			names = append(names, c.Name)
		}
	}
	if len(names) == 0 {
		return report.Note
	}
	return strings.Join(names, ", ")
}
