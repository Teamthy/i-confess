package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/Teamthy/i-confess/internal/audio"
	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/workers"
)

// Audio generation admin endpoints.
//
// These endpoints provide administrative control over audio generation jobs
// and the audio generation pipeline.

// adminCreateAudioGeneration queues a new audio generation job.
// POST /admin/audio/generate
func (h *Handler) adminCreateAudioGeneration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConfessionID     string `json:"confession_id"`
		ContentVersionID string `json:"content_version_id"`
		VariantID        string `json:"variant_id,omitempty"`
		VoiceID          string `json:"voice_id"`
		Provider         string `json:"provider,omitempty"`
		QualityTier      string `json:"quality_tier,omitempty"`
		Language         string `json:"language,omitempty"`
		Text             string `json:"text,omitempty"`
	}

	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate required fields
	if req.ConfessionID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "confession_id is required")
		return
	}
	if req.VoiceID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "voice_id is required")
		return
	}
	if req.Language == "" {
		req.Language = "en"
	}

	// Get the authenticated user
	actor := ""
	if c := auth.FromContext(r); c != nil {
		actor = c.Sub
	}

	// Set defaults
	if req.Provider == "" {
		req.Provider = "default"
	}
	if req.QualityTier == "" {
		req.QualityTier = "standard"
	}

	// Refuse before recording anything if this server cannot render.
	//
	// This endpoint used to accept the request, write a job row and answer
	// "Job queued for processing" with no worker behind it. The row sat in
	// 'queued' forever and the operator had no way to tell a slow render from
	// one that would never start.
	if h.pipeline == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable,
			"voice synthesis is not configured on this server")
		return
	}
	if h.queue == nil || !h.queueDurable {
		httpx.WriteError(w, http.StatusServiceUnavailable,
			"the durable background queue is not configured on this server; use POST /admin/audio/generate to render synchronously")
		return
	}

	// The same gates the synchronous endpoint applies. A queued render is not
	// a cheaper version of a render: unreviewed text must not reach a voice,
	// and a job for a missing confession would fail on every retry.
	conf, err := h.cont.ConfessionByID(r.Context(), req.ConfessionID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "confession not found")
		return
	}
	switch conf.Status {
	case "approved", "published", "ready":
	default:
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"confession must be approved by an editor before audio can be generated")
		return
	}
	if strings.TrimSpace(textForVariant(conf, req.VariantID)) == "" {
		httpx.WriteError(w, http.StatusUnprocessableEntity, "confession has no text for this variant")
		return
	}

	// Snapshot the text now, not when a worker picks the job up. Otherwise an
	// edit between queueing and rendering changes what the audio says while
	// the job record still points at the version that was approved.
	version, err := h.cont.EnsureVersion(r.Context(), conf.ID, conf.Title,
		conf.ShortText, conf.MediumText, conf.LongText, conf.Language, actor)
	if err != nil {
		log.Printf("Failed to snapshot confession text: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not snapshot the confession text")
		return
	}

	// Create the job
	job, created, err := h.audio.CreateJob(r.Context(), &models.AudioJob{
		ConfessionID:     req.ConfessionID,
		ContentVersionID: version.ID,
		VariantID:        req.VariantID,
		VoiceID:          req.VoiceID,
		Provider:         providerName(h.pipeline),
		QualityTier:      req.QualityTier,
		Status:           string(audio.JobQueued),
		MaxAttempts:      jobs.DefaultMaxAttempts,
		RequestedBy:      actor,
		// Keyed on the render, including the text version, so re-submitting
		// the form does not queue a second render of the same words. The old
		// key omitted the version, so every resubmit was a new job.
		IdempotencyKey: generationKey(req.ConfessionID, req.VariantID, req.VoiceID, req.Language, version.ID, false),
	})
	if err != nil {
		log.Printf("Failed to create audio generation job: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create generation job")
		return
	}
	if !created {
		// Already requested. Report the existing job rather than queueing
		// another render of the same thing.
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"job_id": job.ID,
			"status": job.Status,
			"reused": true,
			"note":   "this render was already requested; see the job for its status",
		})
		return
	}

	// Hand the render to the durable queue. This is the line the endpoint was
	// missing: without it the job row was a record of work nobody had.
	queueID, err := h.queue.Enqueue(r.Context(), jobs.Job{
		Type: workers.TypeAudioGenerate,
		Payload: map[string]any{
			"confession_id": req.ConfessionID,
			"variant_id":    req.VariantID,
			"voice_id":      req.VoiceID,
			"language":      req.Language,
			"actor":         actor,
		},
		// Keyed on the job row so a duplicate enqueue cannot double-render.
		IdempotencyKey: "job:" + job.ID,
		MaxAttempts:    jobs.DefaultMaxAttempts,
	})
	if err != nil && !errors.Is(err, jobs.ErrDuplicateJob) {
		log.Printf("Failed to enqueue generation job %s: %v", job.ID, err)
		// The row exists but nothing will run it. Say so instead of leaving
		// the operator watching a job that is queued in name only.
		httpx.WriteError(w, http.StatusInternalServerError,
			"the job was recorded but could not be queued; retry or use POST /admin/audio/generate")
		return
	}

	// Record audit
	if err := h.audio.RecordAudit(r.Context(), actor, "audio_generation_create",
		"audio_generation_job", job.ID,
		fmt.Sprintf("confession=%s, voice=%s, provider=%s", req.ConfessionID, req.VoiceID, req.Provider),
		"ok"); err != nil {
		log.Printf("Failed to record audit: %v", err)
	}

	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"job_id":   job.ID,
		"queue_id": queueID,
		"status":   job.Status,
		"message":  "Job queued for processing",
	})
}

// adminGetAudioGeneration returns the status of a generation job.
// GET /admin/audio/generate/{id}
func (h *Handler) adminGetAudioGeneration(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "job id is required")
		return
	}

	job, err := h.audio.JobByID(r.Context(), jobID)
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "generation job not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to get generation job")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, job)
}

// adminListAudioGenerations lists audio generation jobs.
// GET /admin/audio/generate
func (h *Handler) adminListAudioGenerations(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if _, err := fmt.Sscanf(l, "%d", &limit); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid limit")
			return
		}
	}

	jobs, err := h.audio.Jobs(r.Context(), status, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to list generation jobs")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, jobs)
}

// adminRetryAudioGeneration retries a failed generation job.
// POST /admin/audio/generate/{id}/retry
func (h *Handler) adminRetryAudioGeneration(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "job id is required")
		return
	}

	// Get the authenticated user
	actor := ""
	if c := auth.FromContext(r); c != nil {
		actor = c.Sub
	}

	// Get the job
	job, err := h.audio.JobByID(r.Context(), jobID)
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "generation job not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to get generation job")
		return
	}

	// Check if job can be retried
	if job.AttemptCount >= job.MaxAttempts {
		httpx.WriteError(w, http.StatusConflict,
			fmt.Sprintf("job has reached max attempts (%d)", job.MaxAttempts))
		return
	}

	if job.Status != string(audio.JobFailed) && job.Status != string(audio.JobCancelled) {
		httpx.WriteError(w, http.StatusConflict,
			fmt.Sprintf("job is not in a retryable status: %s", job.Status))
		return
	}

	// Requeue the job
	requeuedJob, err := h.audio.RequeueJob(r.Context(), jobID)
	if err != nil {
		log.Printf("Failed to requeue job %s: %v", jobID, err)
		httpx.WriteError(w, http.StatusInternalServerError, "failed to requeue job")
		return
	}

	// Record audit
	if err := h.audio.RecordAudit(r.Context(), actor, "audio_generation_retry",
		"audio_generation_job", jobID,
		fmt.Sprintf("attempt=%d", requeuedJob.AttemptCount+1),
		"ok"); err != nil {
		log.Printf("Failed to record audit: %v", err)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"job_id":       requeuedJob.ID,
		"status":       requeuedJob.Status,
		"attempt":      requeuedJob.AttemptCount,
		"max_attempts": requeuedJob.MaxAttempts,
		"message":      "Job requeued for processing",
	})
}

// adminCancelAudioGeneration cancels a pending generation job.
// POST /admin/audio/generate/{id}/cancel
func (h *Handler) adminCancelAudioGeneration(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "job id is required")
		return
	}

	// Get the authenticated user
	actor := ""
	if c := auth.FromContext(r); c != nil {
		actor = c.Sub
	}

	// Get the job
	job, err := h.audio.JobByID(r.Context(), jobID)
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "generation job not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to get generation job")
		return
	}

	// Check if job can be cancelled
	if job.Status != string(audio.JobQueued) && job.Status != string(audio.JobProcessing) {
		httpx.WriteError(w, http.StatusConflict,
			fmt.Sprintf("job is not in a cancellable status: %s", job.Status))
		return
	}

	// Cancel the job - we'll use FailJob with a cancellation code
	failedJob, err := h.audio.FailJob(r.Context(), jobID, "cancelled", "Job cancelled by admin")
	if err != nil {
		log.Printf("Failed to cancel job %s: %v", jobID, err)
		httpx.WriteError(w, http.StatusInternalServerError, "failed to cancel job")
		return
	}

	// Record audit
	if err := h.audio.RecordAudit(r.Context(), actor, "audio_generation_cancel",
		"audio_generation_job", jobID, "", "ok"); err != nil {
		log.Printf("Failed to record audit: %v", err)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"job_id":  failedJob.ID,
		"status":  failedJob.Status,
		"message": "Job cancelled",
	})
}

// adminGetAudioGenerationStats returns statistics about audio generation.
// GET /admin/audio/generate/stats
func (h *Handler) adminGetAudioGenerationStats(w http.ResponseWriter, r *http.Request) {
	// Get job counts by status
	allJobs, err := h.audio.Jobs(r.Context(), "", 1000)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to get generation jobs")
		return
	}

	// Initialize stats
	stats := map[string]any{
		"total":           len(allJobs),
		"by_status":       map[string]int{},
		"by_provider":     map[string]int{},
		"by_voice":        map[string]int{},
		"recent_failures": []map[string]any{},
	}

	// Count by status, provider, voice
	for _, job := range allJobs {
		stats["by_status"].(map[string]int)[job.Status]++
		stats["by_provider"].(map[string]int)[job.Provider]++
		stats["by_voice"].(map[string]int)[job.VoiceID]++
	}

	// Get recent failures
	failedJobs, err := h.audio.Jobs(r.Context(), string(audio.JobFailed), 10)
	if err != nil {
		log.Printf("Failed to get failed jobs: %v", err)
	} else {
		// Build failures list
		failures := make([]map[string]any, 0, len(failedJobs))
		for _, job := range failedJobs {
			failures = append(failures, map[string]any{
				"job_id":     job.ID,
				"confession": job.ConfessionID,
				"voice":      job.VoiceID,
				"error_code": job.ErrorCode,
				"error":      job.ErrorMessage,
				"attempts":   job.AttemptCount,
			})
		}
		stats["recent_failures"] = failures
	}

	httpx.WriteJSON(w, http.StatusOK, stats)
}

// adminTriggerBatchGeneration queues one render per confession given.
// POST /admin/audio/generate/batch
//
// This is the older bulk path: it speaks in confession ids, and its renders are
// produced by the cloud-TTS pipeline rather than the GPU worker. Until now it
// was also a no-op wearing a receipt. It called CreateJob without a
// content_version_id - which the store refuses, because a job must point at
// the exact text a render was made from - the loop logged every failure and
// continued, and the response still read "Created 0 generation jobs" with a
// 202 in front of it. Any list of ids therefore "succeeded" having queued
// nothing, and the only signal was a zero in a body phrased as a success.
//
// It now runs the same sequence the single-item endpoint above runs - resolve,
// editorial gate, snapshot the text, record, enqueue - per item, bounded by the
// same GPU budget, and reports each item's outcome so "0 queued, 12 refused" is
// a result an operator can act on instead of a mystery.
func (h *Handler) adminTriggerBatchGeneration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConfessionIDs []string `json:"confession_ids"`
		VoiceID       string   `json:"voice_id"`
		Provider      string   `json:"provider,omitempty"`
		QualityTier   string   `json:"quality_tier,omitempty"`
		Language      string   `json:"language,omitempty"`
	}

	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.ConfessionIDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "confession_ids is required")
		return
	}
	if len(req.ConfessionIDs) > maxBatchItems {
		httpx.WriteError(w, http.StatusBadRequest, "a batch is limited to 10000 confessions")
		return
	}
	if req.VoiceID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "voice_id is required")
		return
	}
	if req.Language == "" {
		req.Language = "en"
	}
	if req.QualityTier == "" {
		req.QualityTier = "standard"
	}
	actor := ""
	if c := auth.FromContext(r); c != nil {
		actor = c.Sub
	}

	// Same guards as the single endpoint: a batch is not a licence to queue work
	// this server cannot run, and it must not answer 202 when nothing can.
	if h.pipeline == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "voice synthesis is not configured on this server")
		return
	}
	if h.queue == nil || !h.queueDurable {
		httpx.WriteError(w, http.StatusServiceUnavailable,
			"the durable background queue is not configured on this server; use POST /admin/audio/generate to render synchronously")
		return
	}

	type itemResult struct {
		ConfessionID string `json:"confession_id"`
		JobID        string `json:"job_id,omitempty"`
		Status       string `json:"status,omitempty"`
		Outcome      string `json:"outcome"` // queued | reused | refused
		Error        string `json:"error,omitempty"`
		Note         string `json:"note,omitempty"`
	}

	// Resolve and gate everything first, read-only. The budget needs the texts,
	// and a batch that cannot fit the budget must not have touched a single row.
	type pending struct {
		conf    *models.Confession
		id      string
		variant string
		text    string
	}
	items := make([]pending, 0, len(req.ConfessionIDs))
	results := make([]itemResult, 0, len(req.ConfessionIDs))
	for _, id := range req.ConfessionIDs {
		conf, err := h.cont.ConfessionByID(r.Context(), id)
		if err != nil {
			results = append(results, itemResult{ConfessionID: id, Outcome: "refused", Error: "confession not found"})
			continue
		}
		switch conf.Status {
		case "approved", "published", "ready":
		default:
			results = append(results, itemResult{ConfessionID: id, Outcome: "refused",
				Error: "confession is " + conf.Status + "; an editor must approve it before audio can be generated"})
			continue
		}
		text := textForVariant(conf, "")
		if strings.TrimSpace(text) == "" {
			results = append(results, itemResult{ConfessionID: id, Outcome: "refused", Error: "confession has no text for this variant"})
			continue
		}
		items = append(items, pending{conf: conf, id: id, text: text})
	}

	texts := make([]string, len(items))
	for i, it := range items {
		texts[i] = it.text
	}
	if _, err := h.checkBatchBudget(r.Context(), req.VoiceID, texts); err != nil {
		var be *budgetExceeded
		if errors.As(err, &be) {
			h.writeBudgetError(w, r, req.VoiceID, actor, be)
			return
		}
		log.Printf("batch budget check failed for voice %s: %v", req.VoiceID, err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not evaluate the GPU budget")
		return
	}

	queued, reused, refused := 0, 0, len(results)
	for _, it := range items {
		// Snapshot the text now, exactly as the single endpoint does: an edit
		// between queueing and rendering must not change what the audio says
		// while the job record still points at the approved version.
		version, err := h.cont.EnsureVersion(r.Context(), it.conf.ID, it.conf.Title,
			it.conf.ShortText, it.conf.MediumText, it.conf.LongText, it.conf.Language, actor)
		if err != nil {
			refused++
			results = append(results, itemResult{ConfessionID: it.id, Outcome: "refused", Error: "could not snapshot the confession text"})
			continue
		}
		job, created, err := h.audio.CreateJob(r.Context(), &models.AudioJob{
			ConfessionID: it.id, ContentVersionID: version.ID, VariantID: it.variant, VoiceID: req.VoiceID,
			Provider: providerName(h.pipeline), QualityTier: req.QualityTier, Status: string(audio.JobQueued),
			MaxAttempts: jobs.DefaultMaxAttempts, RequestedBy: actor,
			IdempotencyKey: generationKey(it.id, it.variant, req.VoiceID, req.Language, version.ID, false),
		})
		if err != nil {
			refused++
			results = append(results, itemResult{ConfessionID: it.id, Outcome: "refused", Error: err.Error()})
			continue
		}
		if !created {
			reused++
			results = append(results, itemResult{ConfessionID: it.id, JobID: job.ID, Status: job.Status, Outcome: "reused",
				Note: "this render was already requested; see the job for its status"})
			continue
		}
		if _, err := h.queue.Enqueue(r.Context(), jobs.Job{
			Type: workers.TypeAudioGenerate,
			Payload: map[string]any{"confession_id": it.id, "variant_id": it.variant, "voice_id": req.VoiceID,
				"language": req.Language, "actor": actor},
			IdempotencyKey: "job:" + job.ID,
			MaxAttempts:    jobs.DefaultMaxAttempts,
		}); err != nil && !errors.Is(err, jobs.ErrDuplicateJob) {
			log.Printf("Failed to enqueue generation job %s: %v", job.ID, err)
			// The row exists and nothing will run it. The job lifecycle has no
			// queued -> failed edge, so it cannot be closed from here without
			// widening that; recording the item as unqueued is the honest
			// answer until it does.
			refused++
			results = append(results, itemResult{ConfessionID: it.id, JobID: job.ID, Status: job.Status, Outcome: "refused",
				Error: "the job was recorded but could not be queued; retry or use POST /admin/audio/generate"})
			continue
		}
		queued++
		results = append(results, itemResult{ConfessionID: it.id, JobID: job.ID, Status: job.Status, Outcome: "queued"})
	}

	if err := h.audio.RecordAudit(r.Context(), actor, "audio_generation_batch", "audio_generation_job", "",
		fmt.Sprintf("confessions=%d, queued=%d, reused=%d, refused=%d, voice=%s, provider=%s",
			len(req.ConfessionIDs), queued, reused, refused, req.VoiceID, providerName(h.pipeline)),
		"ok"); err != nil {
		log.Printf("Failed to record audit: %v", err)
	}

	// A batch that queued nothing *and* found nothing already queued is not a
	// request being processed, and must not be reported as one. Reuse alone is
	// success: the render is queued, just not by this call.
	status := http.StatusAccepted
	if queued == 0 && reused == 0 {
		status = http.StatusUnprocessableEntity
	}
	httpx.WriteJSON(w, status, map[string]any{
		"message": fmt.Sprintf("%d queued, %d reused, %d refused of %d requested",
			queued, reused, refused, len(req.ConfessionIDs)),
		"queued": queued, "reused": reused, "refused": refused,
		"total_requested": len(req.ConfessionIDs),
		"items":           results,
	})
}

// adminGetAudioGenerationProviders returns information about available TTS providers.
// GET /admin/audio/providers
func (h *Handler) adminGetAudioGenerationProviders(w http.ResponseWriter, r *http.Request) {
	// In a real implementation, this would query the generator for available providers
	// For now, return a static list
	providers := []map[string]any{
		{
			"name":         "google",
			"display_name": "Google Cloud Text-to-Speech",
			"status":       "available",
			"capabilities": []string{"neural", "waveNet", "standard"},
		},
		{
			"name":         "amazon",
			"display_name": "Amazon Polly",
			"status":       "available",
			"capabilities": []string{"neural", "standard"},
		},
		{
			"name":         "microsoft",
			"display_name": "Microsoft Azure Cognitive Services",
			"status":       "available",
			"capabilities": []string{"neural"},
		},
		{
			"name":         "elevenlabs",
			"display_name": "ElevenLabs",
			"status":       "available",
			"capabilities": []string{"neural", "emotional"},
		},
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"providers": providers,
		"default":   "google",
	})
}
