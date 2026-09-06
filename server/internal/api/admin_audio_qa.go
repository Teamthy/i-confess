package api

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/Teamthy/i-confess/internal/audio"
	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

// Audio QA and generation-job endpoints (§31, §75).
//
// Generated audio has always entered 'processing', but the codebase contained
// no UPDATE audio_assets statement at all, so there was no way out: a render sat
// in QA forever and could never be served. The only escape was re-posting the
// whole asset through POST /admin/audio with status "ready", which recorded
// neither reviewer nor reason - so "who approved this voice" was unanswerable.

// adminApproveAudio moves a render from QA to servable.
func (h *Handler) adminApproveAudio(w http.ResponseWriter, r *http.Request) {
	h.qaTransition(w, r, audio.StatusReady, false)
}

// adminRejectAudio records a human decision that a render must not ship.
//
// A note is required. A rejection with no reason cannot be acted on: the audio
// team needs to know whether the synthesis was wrong, the voice was wrong, or
// the text was wrong.
func (h *Handler) adminRejectAudio(w http.ResponseWriter, r *http.Request) {
	h.qaTransition(w, r, audio.StatusQARejected, true)
}

// adminPublishAudio surfaces an approved render in discovery.
func (h *Handler) adminPublishAudio(w http.ResponseWriter, r *http.Request) {
	h.qaTransition(w, r, audio.StatusPublished, false)
}

// adminArchiveAudio withdraws a render. Because session queues are snapshots,
// this is also how an asset is pulled from sessions that already reference it:
// the access check reads the asset's current status on every fetch.
func (h *Handler) adminArchiveAudio(w http.ResponseWriter, r *http.Request) {
	h.qaTransition(w, r, audio.StatusArchived, false)
}

func (h *Handler) qaTransition(w http.ResponseWriter, r *http.Request, to audio.AssetStatus, noteRequired bool) {
	id := r.PathValue("id")
	if id == "" {
		httpx.WriteError(w, http.StatusBadRequest, "asset id is required")
		return
	}

	var req struct {
		Note string `json:"note"`
	}
	// An empty body is valid for approve/publish/archive, so a decode failure
	// on an absent body is not an error.
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	req.Note = strings.TrimSpace(req.Note)
	if noteRequired && req.Note == "" {
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"a note is required: a rejection with no reason cannot be acted on")
		return
	}

	actor := ""
	if c := auth.FromContext(r); c != nil {
		actor = c.Sub
	}

	asset, err := h.audio.SetAssetStatus(r.Context(), id, to, actor, req.Note)
	var trans *audio.TransitionError
	if errors.As(err, &trans) {
		// 409: the asset exists and the request is well-formed, but this move is
		// not legal from where the asset currently is.
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":   trans.Error(),
			"status":  asset.Status,
			"allowed": audio.AllowedFrom(audio.AssetStatus(asset.Status)),
		})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "audio asset not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not update the audio asset")
		return
	}

	if err := h.audio.RecordAudit(r.Context(), actor, "audio_"+string(to), "audio_asset", id, req.Note, "ok"); err != nil {
		// The change is already committed; a lost audit line must not fail the
		// request, but it must not be silent either.
		log.Printf("audio: audit write failed for asset %s: %v", id, err)
	}

	httpx.WriteJSON(w, http.StatusOK, asset)
}

// adminListAudioJobs lists generation requests, newest first.
func (h *Handler) adminListAudioJobs(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "" && !audio.ValidJobStatus(status) {
		httpx.WriteError(w, http.StatusBadRequest,
			"status must be one of: queued, processing, succeeded, failed, cancelled")
		return
	}
	jobs, err := h.audio.Jobs(r.Context(), status, 100)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not list generation jobs")
		return
	}
	if jobs == nil {
		// An empty list, not null: a client iterating the response should not
		// have to special-case "no jobs yet".
		httpx.WriteJSON(w, http.StatusOK, []models.AudioJob{})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, jobs)
}

// adminGetAudioJob returns one generation request with its outcome.
func (h *Handler) adminGetAudioJob(w http.ResponseWriter, r *http.Request) {
	job, err := h.audio.JobByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "generation job not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not read the generation job")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, job)
}
