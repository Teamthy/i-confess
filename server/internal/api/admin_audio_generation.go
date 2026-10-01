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

// adminTriggerBatchGeneration triggers batch generation for multiple confessions.
// POST /admin/audio/generate/batch
func (h *Handler) adminTriggerBatchGeneration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConfessionIDs []string `json:"confession_ids"`
		VoiceID       string   `json:"voice_id"`
		Provider      string   `json:"provider,omitempty"`
		QualityTier   string   `json:"quality_tier,omitempty"`
	}

	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate required fields
	if len(req.ConfessionIDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "confession_ids is required")
		return
	}
	if req.VoiceID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "voice_id is required")
		return
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

	// Create jobs for each confession
	var createdJobs []string
	for _, confessionID := range req.ConfessionIDs {
		job, _, err := h.audio.CreateJob(r.Context(), &models.AudioJob{
			ConfessionID:   confessionID,
			VoiceID:        req.VoiceID,
			Provider:       req.Provider,
			QualityTier:    req.QualityTier,
			Status:         string(audio.JobQueued),
			MaxAttempts:    3,
			RequestedBy:    actor,
			IdempotencyKey: fmt.Sprintf("%s-%s-batch", confessionID, req.VoiceID),
		})
		if err != nil {
			log.Printf("Failed to create batch job for confession %s: %v", confessionID, err)
			// Continue with other confessions
			continue
		}
		createdJobs = append(createdJobs, job.ID)
	}

	// Record audit
	if err := h.audio.RecordAudit(r.Context(), actor, "audio_generation_batch",
		"audio_generation_job", "",
		fmt.Sprintf("confessions=%d, voice=%s", len(req.ConfessionIDs), req.VoiceID),
		"ok"); err != nil {
		log.Printf("Failed to record audit: %v", err)
	}

	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"message":         fmt.Sprintf("Created %d generation jobs", len(createdJobs)),
		"job_ids":         createdJobs,
		"total_requested": len(req.ConfessionIDs),
		"success_count":   len(createdJobs),
		"fail_count":      len(req.ConfessionIDs) - len(createdJobs),
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
