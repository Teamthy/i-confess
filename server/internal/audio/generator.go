// Package audio provides text-to-speech audio generation.
//
// This file implements the TTS generation service that creates audio
// from text using various TTS providers.
package audio

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/storage"
)

// ErrTTSFailed is returned when TTS generation fails.
var ErrTTSFailed = errors.New("audio: TTS generation failed")

// ErrNoTTSProvider is returned when no TTS provider is configured.
var ErrNoTTSProvider = errors.New("audio: no TTS provider configured")

// TTSProvider defines the interface for text-to-speech providers.
type TTSProvider interface {
	// Generate generates audio from text and returns the audio data.
	Generate(ctx context.Context, text, voiceID, language string) ([]byte, error)
	// GetVoiceInfo returns information about a specific voice.
	GetVoiceInfo(ctx context.Context, voiceID string) (*VoiceInfo, error)
	// ListVoices returns a list of available voices.
	ListVoices(ctx context.Context) ([]VoiceInfo, error)
	// Name returns the provider name.
	Name() string
}

// VoiceInfo contains information about a TTS voice.
type VoiceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Gender      string `json:"gender,omitempty"`
	Language    string `json:"language"`
	Provider    string `json:"provider"`
	Type        string `json:"type"` // professional, minister, generic
	Premium     bool   `json:"premium"`
	SampleRate  int    `json:"sample_rate,omitempty"`
	Bitrate     int    `json:"bitrate,omitempty"`
}

// Generator provides TTS generation capabilities.
type Generator struct {
	providers    map[string]TTSProvider
	defaultProvider string
	processor    *Processor
	storage      storage.ObjectStorage
	assetStore   AssetStorer
	jobStore     JobStorer
	mu           sync.RWMutex
}

// GeneratorConfig holds configuration for the generator.
type GeneratorConfig struct {
	DefaultProvider string
	Processor       *Processor
	Storage         storage.ObjectStorage
	AssetStore      AssetStorer
	JobStore        JobStorer
}

// NewGenerator creates a new audio generator.
func NewGenerator(cfg *GeneratorConfig) *Generator {
	return &Generator{
		providers:       make(map[string]TTSProvider),
		defaultProvider: cfg.DefaultProvider,
		processor:       cfg.Processor,
		storage:         cfg.Storage,
		assetStore:      cfg.AssetStore,
		jobStore:        cfg.JobStore,
	}
}

// RegisterProvider registers a TTS provider.
func (g *Generator) RegisterProvider(provider TTSProvider) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.providers[provider.Name()] = provider
	if g.defaultProvider == "" {
		g.defaultProvider = provider.Name()
	}
}

// GetProvider returns a specific TTS provider by name.
func (g *Generator) GetProvider(name string) (TTSProvider, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	provider, ok := g.providers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoTTSProvider, name)
	}
	return provider, nil
}

// Generate generates audio from text using the default or specified provider.
func (g *Generator) Generate(
	ctx context.Context,
	text string,
	voiceID string,
	confessionID string,
	contentVersionID string,
	variantID string,
	providerName string,
	qualityTier string,
) (*models.AudioAsset, error) {
	// Get the provider
	provider, err := g.getProvider(providerName)
	if err != nil {
		return nil, err
	}

	// Get voice info to determine language
	voiceInfo, err := provider.GetVoiceInfo(ctx, voiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get voice info: %w", err)
	}

	// Generate audio
	log.Printf("Generating audio for confession %s, voice %s, provider %s", confessionID, voiceID, provider.Name())
	audioData, err := provider.Generate(ctx, text, voiceID, voiceInfo.Language)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTTSFailed, err)
	}

	// Process the audio (normalize, transcode)
	processedData, err := g.processor.Process(ctx, audioData, "wav")
	if err != nil {
		return nil, fmt.Errorf("audio processing failed: %w", err)
	}

	// Create asset
	asset := &models.AudioAsset{
		ConfessionID:    confessionID,
		ContentVersionID: contentVersionID,
		VoiceID:         voiceID,
		VariantID:       variantID,
		AudioSource:     "generated",
		Status:          string(StatusProcessing),
	}

	// Store the asset
	if err := g.assetStore.UpsertAsset(ctx, asset); err != nil {
		return nil, fmt.Errorf("failed to store asset: %w", err)
	}

	// Upload to storage
	storageKey := g.generateStorageKey(asset)
	metadata := map[string]string{
		"confession_id":    confessionID,
		"content_version_id": contentVersionID,
		"voice_id":         voiceID,
		"variant_id":       variantID,
		"provider":         provider.Name(),
		"quality_tier":     qualityTier,
		"generated_at":      time.Now().UTC().Format(time.RFC3339),
	}

	if err := g.storage.Upload(ctx, storageKey, processedData, metadata); err != nil {
		return nil, fmt.Errorf("failed to upload to storage: %w", err)
	}

	// Update asset with storage info
	asset.URL = storageKey

	// Get duration from processed data
	// Inspect with empty format to auto-detect
	report, err := Inspect(processedData, "")
	if err == nil {
		asset.DurationSeconds = report.DurationSeconds
		asset.SizeBytes = int64(report.SizeBytes)
	}

	// Update the asset with final metadata
	if err := g.assetStore.UpsertAsset(ctx, asset); err != nil {
		return nil, fmt.Errorf("failed to update asset: %w", err)
	}

	return asset, nil
}

// GenerateAsync queues an audio generation job for async processing.
func (g *Generator) GenerateAsync(
	ctx context.Context,
	text string,
	voiceID string,
	confessionID string,
	contentVersionID string,
	variantID string,
	providerName string,
	qualityTier string,
	requestedBy string,
) (*models.AudioJob, error) {
	// Create the job
	job := &models.AudioJob{
		ID:               newID(),
		ContentVersionID: contentVersionID,
		ConfessionID:     confessionID,
		VariantID:        variantID,
		VoiceID:          voiceID,
		Provider:         providerName,
		QualityTier:      qualityTier,
		Status:          string(JobQueued),
		MaxAttempts:      3,
		RequestedBy:      requestedBy,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
		IdempotencyKey:   fmt.Sprintf("%s-%s-%s-%s", confessionID, contentVersionID, voiceID, variantID),
	}

	// Store the job
	if err := g.jobStore.CreateJob(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	return job, nil
}

// ProcessJob processes a queued audio generation job.
func (g *Generator) ProcessJob(ctx context.Context, job *models.AudioJob) error {
	// Update job status
	job.Status = string(JobProcessing)
	job.AttemptCount++
	job.StartedAt = time.Now().UTC().Format(time.RFC3339)
	if err := g.jobStore.UpdateJob(ctx, job); err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	// Get the text to generate - this would come from the content version
	// For now, we'll use a placeholder
	text := "This is the confession text for " + job.ConfessionID
	// In a real implementation, we would fetch the text from the content version

	// Generate the audio
	asset, err := g.Generate(
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
		job.Status = string(JobFailed)
		job.ErrorMessage = err.Error()
		job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		if err := g.jobStore.UpdateJob(ctx, job); err != nil {
			log.Printf("Failed to update failed job: %v", err)
		}
		return fmt.Errorf("generation failed: %w", err)
	}

	// Link the asset to the job
	job.AudioAssetID = asset.ID
	job.Status = string(JobSucceeded)
	job.CompletedAt = time.Now().UTC().Format(time.RFC3339)

	if err := g.jobStore.UpdateJob(ctx, job); err != nil {
		log.Printf("Failed to update successful job: %v", err)
		// Continue, as the asset was created successfully
	}

	return nil
}

// RetryJob retries a failed audio generation job.
func (g *Generator) RetryJob(ctx context.Context, jobID string) error {
	job, err := g.jobStore.JobByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	// Check if job can be retried
	if job.AttemptCount >= job.MaxAttempts {
		return fmt.Errorf("job %s has reached max attempts (%d)", jobID, job.MaxAttempts)
	}

	if job.Status != string(JobFailed) && job.Status != string(JobCancelled) {
		return fmt.Errorf("job %s is not in a retryable status: %s", jobID, job.Status)
	}

	// Reset job for retry
	job.Status = string(JobQueued)
	job.ErrorMessage = ""
	job.ErrorCode = ""
	job.ProviderJobID = ""
	job.StartedAt = ""
	job.CompletedAt = ""
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := g.jobStore.UpdateJob(ctx, job); err != nil {
		return fmt.Errorf("failed to update job for retry: %w", err)
	}

	return nil
}

// GetJobStatus returns the current status of a generation job.
func (g *Generator) GetJobStatus(ctx context.Context, jobID string) (*models.AudioJob, error) {
	return g.jobStore.JobByID(ctx, jobID)
}

// ListJobs returns a list of generation jobs with optional filters.
func (g *Generator) ListJobs(
	ctx context.Context,
	confessionID string,
	voiceID string,
	status string,
	limit int,
	offset int,
) ([]models.AudioJob, error) {
	// In a real implementation, this would query the job store
	// For now, return an empty list
	return []models.AudioJob{}, nil
}

// CancelJob cancels a pending generation job.
func (g *Generator) CancelJob(ctx context.Context, jobID string) error {
	job, err := g.jobStore.JobByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	// Check if job can be cancelled
	if job.Status != string(JobQueued) && job.Status != string(JobProcessing) {
		return fmt.Errorf("job %s is not in a cancellable status: %s", jobID, job.Status)
	}

	job.Status = string(JobCancelled)
	job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := g.jobStore.UpdateJob(ctx, job); err != nil {
		return fmt.Errorf("failed to update job for cancellation: %w", err)
	}

	return nil
}

// getProvider returns the appropriate provider (default or specified).
func (g *Generator) getProvider(name string) (TTSProvider, error) {
	if name != "" {
		return g.GetProvider(name)
	}
	return g.GetProvider(g.defaultProvider)
}

// generateStorageKey creates a deterministic storage key for generated audio.
func (g *Generator) generateStorageKey(asset *models.AudioAsset) string {
	var builder strings.Builder
	builder.WriteString("audio/generated/")
	builder.WriteString(asset.ConfessionID)
	
	if asset.ContentVersionID != "" {
		builder.WriteString("/version/")
		builder.WriteString(asset.ContentVersionID)
	}
	
	builder.WriteString("/voice/")
	builder.WriteString(asset.VoiceID)
	
	if asset.VariantID != "" {
		builder.WriteString("/variant/")
		builder.WriteString(asset.VariantID)
	}
	
	// Use default format (m4a) for generated audio
	builder.WriteString("/generated.m4a")
	
	return builder.String()
}

// generateCDNPath creates a CDN path for generated audio.
func (g *Generator) generateCDNPath(storageKey string) string {
	// In a real implementation, this would use the CDN domain
	return storageKey
}

// newID generates a new unique ID.
func newID() string {
	return fmt.Sprintf("id-%d", time.Now().UnixNano())
}

// ListVoices returns a list of available voices from all providers.
func (g *Generator) ListVoices(ctx context.Context) ([]VoiceInfo, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var voices []VoiceInfo
	for _, provider := range g.providers {
		providerVoices, err := provider.ListVoices(ctx)
		if err != nil {
			log.Printf("Failed to list voices from provider %s: %v", provider.Name(), err)
			continue
		}
		voices = append(voices, providerVoices...)
	}
	return voices, nil
}

// GetVoiceInfo returns information about a specific voice.
func (g *Generator) GetVoiceInfo(ctx context.Context, voiceID string) (*VoiceInfo, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, provider := range g.providers {
		voiceInfo, err := provider.GetVoiceInfo(ctx, voiceID)
		if err == nil {
			return voiceInfo, nil
		}
	}
	return nil, fmt.Errorf("voice %s not found", voiceID)
}

// BatchGenerate generates audio for multiple confessions/voices.
func (g *Generator) BatchGenerate(
	ctx context.Context,
	requests []GenerationRequest,
) ([]GenerationResult, error) {
	results := make([]GenerationResult, len(requests))

	for i, req := range requests {
		asset, err := g.Generate(
			ctx,
			req.Text,
			req.VoiceID,
			req.ConfessionID,
			req.ContentVersionID,
			req.VariantID,
			req.ProviderName,
			req.QualityTier,
		)
		
		results[i] = GenerationResult{
			Request:      req,
			Asset:        asset,
			Error:        err,
			Success:      err == nil,
		}
	}

	return results, nil
}

// GenerationRequest represents a request to generate audio.
type GenerationRequest struct {
	Text            string
	VoiceID         string
	ConfessionID    string
	ContentVersionID string
	VariantID       string
	ProviderName    string
	QualityTier     string
}

// GenerationResult represents the result of an audio generation.
type GenerationResult struct {
	Request GenerationRequest
	Asset   *models.AudioAsset
	Error   error
	Success bool
}

// GetStats returns statistics about audio generation.
func (g *Generator) GetStats(ctx context.Context) (GenerationStats, error) {
	// In a real implementation, this would query the job store for statistics
	return GenerationStats{
		TotalJobs:      0,
		CompletedJobs:  0,
		FailedJobs:     0,
		PendingJobs:    0,
		AverageDuration: 0,
	}, nil
}

// GenerationStats contains statistics about audio generation.
type GenerationStats struct {
	TotalJobs      int64
	CompletedJobs  int64
	FailedJobs     int64
	PendingJobs    int64
	AverageDuration float64 // in seconds
}
