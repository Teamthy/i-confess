// Package jobs provides background job processing for audio generation.
//
// This file implements the audio generation job handler that processes
// queued audio generation jobs.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Teamthy/i-confess/internal/audio"
	"github.com/Teamthy/i-confess/internal/models"
)

// ErrHandlerNotStarted is returned when trying to use a handler that hasn't been started.
var ErrHandlerNotStarted = errors.New("jobs: audio handler not started")

// AudioHandler processes audio generation jobs from the queue.
type AudioHandler struct {
	generator    *audio.Generator
	jobStore     JobStorer
	assetStore   audio.AssetStorer
	concurrency  int
	stopChan     chan struct{}
	wg           sync.WaitGroup
	started     bool
	mu           sync.Mutex
}

// JobStorer defines the interface for job database operations.
type JobStorer interface {
	GetNextJob(ctx context.Context, queue string) (*models.AudioJob, error)
	UpdateJob(ctx context.Context, job *models.AudioJob) error
	JobByID(ctx context.Context, id string) (*models.AudioJob, error)
}

// AudioHandlerConfig holds configuration for the audio handler.
type AudioHandlerConfig struct {
	Generator    *audio.Generator
	JobStore     JobStorer
	AssetStore   audio.AssetStorer
	Concurrency  int // Number of concurrent workers
}

// NewAudioHandler creates a new audio generation job handler.
func NewAudioHandler(cfg *AudioHandlerConfig) *AudioHandler {
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 4 // default
	}

	return &AudioHandler{
		generator:   cfg.Generator,
		jobStore:    cfg.JobStore,
		assetStore:  cfg.AssetStore,
		concurrency: concurrency,
		stopChan:    make(chan struct{}),
	}
}

// Start starts the audio handler workers.
func (h *AudioHandler) Start() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.started {
		return errors.New("audio handler already started")
	}

	h.started = true

	// Start worker goroutines
	for i := 0; i < h.concurrency; i++ {
		h.wg.Add(1)
		go h.worker(i)
	}

	log.Printf("Audio handler started with %d workers", h.concurrency)
	return nil
}

// Stop stops the audio handler workers.
func (h *AudioHandler) Stop() error {
	h.mu.Lock()
	if !h.started {
		h.mu.Unlock()
		return ErrHandlerNotStarted
	}
	h.mu.Unlock()

	close(h.stopChan)
	h.wg.Wait()
	h.mu.Lock()
	h.started = false
	h.mu.Unlock()

	log.Printf("Audio handler stopped")
	return nil
}

// worker is a single worker goroutine that processes jobs.
func (h *AudioHandler) worker(id int) {
	defer h.wg.Done()

	log.Printf("Audio worker %d started", id)

	for {
		select {
		case <-h.stopChan:
			log.Printf("Audio worker %d stopping", id)
			return
		default:
			// Get next job from queue
			job, err := h.getNextJob()
			if err != nil {
				log.Printf("Audio worker %d: failed to get next job: %v", id, err)
				time.Sleep(1 * time.Second) // Backoff on error
				continue
			}

			if job == nil {
				// No jobs available, wait and retry
				time.Sleep(500 * time.Millisecond)
				continue
			}

			// Process the job
			if err := h.processJob(job); err != nil {
				log.Printf("Audio worker %d: failed to process job %s: %v", id, job.ID, err)
				// Job error is already recorded in processJob
			}
		}
	}
}

// getNextJob retrieves the next job from the queue.
func (h *AudioHandler) getNextJob() (*models.AudioJob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to get a queued job
	job, err := h.jobStore.GetNextJob(ctx, "audio_generation")
	if err != nil {
		return nil, fmt.Errorf("failed to get next job: %w", err)
	}

	return job, nil
}

// processJob processes a single audio generation job.
func (h *AudioHandler) processJob(job *models.AudioJob) error {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second) // 5 minute timeout
	defer cancel()

	log.Printf("Processing audio generation job %s for confession %s, voice %s",
		job.ID, job.ConfessionID, job.VoiceID)

	// Update job status to processing
	job.Status = string(audio.JobProcessing)
	job.AttemptCount++
	job.StartedAt = time.Now().UTC().Format(time.RFC3339)
	if err := h.jobStore.UpdateJob(ctx, job); err != nil {
		log.Printf("Failed to update job %s status: %v", job.ID, err)
		// Continue processing even if we can't update the job
	}

	// In a real implementation, we would fetch the text from the content version
	// For now, use a placeholder
	text := "This is the confession text"

	// Generate the audio
	asset, err := h.generator.Generate(
		ctx,
		text,
		job.VoiceID,
		job.ConfessionID,
		job.ContentVersionID,
		job.VariantID,
		job.Provider,
		job.QualityTier,
	)
	if err != nil {
		// Update job with error
		job.Status = string(audio.JobFailed)
		job.ErrorMessage = err.Error()
		job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		if updateErr := h.jobStore.UpdateJob(ctx, job); updateErr != nil {
			log.Printf("Failed to update failed job %s: %v", job.ID, updateErr)
		}
		return fmt.Errorf("generation failed: %w", err)
	}

	// Link the asset to the job
	job.AudioAssetID = asset.ID
	job.Status = string(audio.JobSucceeded)
	job.CompletedAt = time.Now().UTC().Format(time.RFC3339)

	if err := h.jobStore.UpdateJob(ctx, job); err != nil {
		log.Printf("Failed to update successful job %s: %v", job.ID, err)
		// Continue, as the asset was created successfully
	}

	log.Printf("Successfully processed job %s, created asset %s", job.ID, asset.ID)
	return nil
}

// ProcessJobNow processes a job immediately (synchronous).
func (h *AudioHandler) ProcessJobNow(ctx context.Context, job *models.AudioJob) error {
	return h.processJob(job)
}

// QueueJob queues a new audio generation job.
func (h *AudioHandler) QueueJob(
	ctx context.Context,
	confessionID string,
	contentVersionID string,
	voiceID string,
	variantID string,
	provider string,
	qualityTier string,
	requestedBy string,
) (*models.AudioJob, error) {
	// Create the job
	job := &models.AudioJob{
		ID:               newJobID(),
		ContentVersionID: contentVersionID,
		ConfessionID:     confessionID,
		VariantID:        variantID,
		VoiceID:          voiceID,
		Provider:         provider,
		QualityTier:      qualityTier,
		Status:          string(audio.JobQueued),
		MaxAttempts:      3,
		RequestedBy:      requestedBy,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
		IdempotencyKey:   fmt.Sprintf("%s-%s-%s-%s", confessionID, contentVersionID, voiceID, variantID),
	}

	// Store the job
	if err := h.jobStore.UpdateJob(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	return job, nil
}

// GetJobStatus returns the current status of a job.
func (h *AudioHandler) GetJobStatus(ctx context.Context, jobID string) (*models.AudioJob, error) {
	return h.jobStore.JobByID(ctx, jobID)
}

// RetryJob retries a failed job.
func (h *AudioHandler) RetryJob(ctx context.Context, jobID string) error {
	job, err := h.jobStore.JobByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	// Check if job can be retried
	if job.AttemptCount >= job.MaxAttempts {
		return fmt.Errorf("job %s has reached max attempts (%d)", jobID, job.MaxAttempts)
	}

	if job.Status != string(audio.JobFailed) && job.Status != string(audio.JobCancelled) {
		return fmt.Errorf("job %s is not in a retryable status: %s", jobID, job.Status)
	}

	// Reset job for retry
	job.Status = string(audio.JobQueued)
	job.ErrorMessage = ""
	job.ErrorCode = ""
	job.ProviderJobID = ""
	job.StartedAt = ""
	job.CompletedAt = ""
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := h.jobStore.UpdateJob(ctx, job); err != nil {
		return fmt.Errorf("failed to update job for retry: %w", err)
	}

	return nil
}

// CancelJob cancels a pending job.
func (h *AudioHandler) CancelJob(ctx context.Context, jobID string) error {
	job, err := h.jobStore.JobByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	// Check if job can be cancelled
	if job.Status != string(audio.JobQueued) && job.Status != string(audio.JobProcessing) {
		return fmt.Errorf("job %s is not in a cancellable status: %s", jobID, job.Status)
	}

	job.Status = string(audio.JobCancelled)
	job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := h.jobStore.UpdateJob(ctx, job); err != nil {
		return fmt.Errorf("failed to update job for cancellation: %w", err)
	}

	return nil
}

// GetStats returns statistics about job processing.
func (h *AudioHandler) GetStats() HandlerStats {
	// In a real implementation, this would track statistics
	return HandlerStats{
		JobsProcessed: 0,
		JobsFailed:    0,
		WorkersActive: 0,
		QueueDepth:    0,
	}
}

// HandlerStats contains statistics about the audio handler.
type HandlerStats struct {
	JobsProcessed int64
	JobsFailed    int64
	WorkersActive int
	QueueDepth    int
}

// IsStarted returns whether the handler has been started.
func (h *AudioHandler) IsStarted() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.started
}

// newJobID generates a new job ID.
func newJobID() string {
	return fmt.Sprintf("job-%d", time.Now().UnixNano())
}
