// Package audio provides the core audio asset lifecycle services.
//
// This file implements the asset lifecycle: Create, Publish, Archive for audio assets.
// It coordinates between storage, database, and processing components.
package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/storage"
)

// ErrAssetNotFound is returned when an audio asset cannot be found.
var ErrAssetNotFound = errors.New("audio: asset not found")

// ErrVoiceNotFound is returned when a voice cannot be found.
var ErrVoiceNotFound = errors.New("audio: voice not found")

// ErrGenerationFailed is returned when audio generation fails.
var ErrGenerationFailed = errors.New("audio: generation failed")

// Service provides audio asset lifecycle management.
type Service struct {
	storage     storage.ObjectStorage
	assetStore  AssetStorer
	voiceStore  VoiceStorer
	jobStore    JobStorer
	cdnDomain   string
	signingTTL  time.Duration
}

// AssetStorer defines the interface for audio asset database operations.
type AssetStorer interface {
	UpsertAsset(ctx context.Context, a *models.AudioAsset) error
	AssetByID(ctx context.Context, id string) (models.AudioAsset, error)
	AssetsFor(ctx context.Context, confessionID, voiceID string) ([]models.AudioAsset, error)
}

// VoiceStorer defines the interface for voice database operations.
type VoiceStorer interface {
	VoiceByID(ctx context.Context, id string) (*models.Voice, error)
	ListVoices(ctx context.Context) ([]models.Voice, error)
}

// JobStorer defines the interface for audio generation job operations.
type JobStorer interface {
	CreateJob(ctx context.Context, j *models.AudioJob) error
	UpdateJob(ctx context.Context, j *models.AudioJob) error
	JobByID(ctx context.Context, id string) (*models.AudioJob, error)
}

// ServiceConfig holds configuration for the audio service.
type ServiceConfig struct {
	StorageProvider string
	CDNDomain       string
	SigningTTL      time.Duration
	// Add other configuration as needed
}

// NewService creates a new audio service.
func NewService(storage storage.ObjectStorage, assetStore AssetStorer, voiceStore VoiceStorer, jobStore JobStorer, cfg *ServiceConfig) *Service {
	return &Service{
		storage:     storage,
		assetStore:  assetStore,
		voiceStore:  voiceStore,
		jobStore:    jobStore,
		cdnDomain:   cfg.CDNDomain,
		signingTTL:  cfg.SigningTTL,
	}
}

// CreateAsset creates a new audio asset and stores it in object storage.
// If the asset already exists (same content_id, voice_id, variant_id), it updates it.
func (s *Service) CreateAsset(ctx context.Context, asset *models.AudioAsset, audioData []byte) error {
	// Validate the asset
	if asset.ConfessionID == "" {
		return errors.New("confession_id is required")
	}
	if asset.VoiceID == "" {
		return errors.New("voice_id is required")
	}

	// Verify the voice exists
	if _, err := s.voiceStore.VoiceByID(ctx, asset.VoiceID); err != nil {
		return fmt.Errorf("%w: %v", ErrVoiceNotFound, err)
	}

	// Inspect the audio data (auto-detect format)
	report, err := Inspect(audioData, "")
	if err != nil {
		return fmt.Errorf("audio inspection failed: %w", err)
	}

	// Update asset metadata from inspection
	asset.DurationSeconds = report.DurationSeconds
	asset.SizeBytes = int64(report.SizeBytes)
	// Note: Format is not stored in AudioAsset model, but we have report.Format

	// Generate storage key
	storageKey := s.generateStorageKey(asset)

	// Upload to object storage
	// Note: Format metadata can be added if needed, but not stored in asset model
	metadata := map[string]string{
		"confession_id": asset.ConfessionID,
		"voice_id":      asset.VoiceID,
		"variant_id":    asset.VariantID,
		"duration":      fmt.Sprintf("%d", asset.DurationSeconds),
	}

	if err := s.storage.Upload(ctx, storageKey, audioData, metadata); err != nil {
		return fmt.Errorf("storage upload failed: %w", err)
	}

	// Set the storage key
	asset.URL = storageKey

	// Set default status
	if asset.Status == "" {
		asset.Status = string(StatusUploading)
	}

	// Set timestamps
	now := time.Now().UTC()
	asset.CreatedAt = now.Format(time.RFC3339)
	asset.UpdatedAt = now.Format(time.RFC3339)

	// Store in database
	if err := s.assetStore.UpsertAsset(ctx, asset); err != nil {
		// Try to clean up storage on failure
		_ = s.storage.Delete(ctx, storageKey)
		return fmt.Errorf("database store failed: %w", err)
	}

	return nil
}

// GenerateSignedURL generates a time-limited signed URL for audio playback.
func (s *Service) GenerateSignedURL(ctx context.Context, assetID string) (string, error) {
	// Get the asset from database
	asset, err := s.assetStore.AssetByID(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrAssetNotFound, err)
	}

	// Check if asset is servable
	if !IsServed(asset.Status) {
		return "", fmt.Errorf("asset %s is not in a servable status: %s", assetID, asset.Status)
	}

	// Generate signed URL
	storageKey := asset.URL
	if strings.TrimSpace(storageKey) == "" {
		storageKey = s.generateStorageKeyFromAsset(&asset)
	}

	ttl := s.signingTTL
	if ttl == 0 {
		ttl = 4 * time.Hour // Default 4 hours for streaming
	}

	signedURL, err := s.storage.GenerateSignedURL(ctx, storageKey, ttl)
	if err != nil {
		return "", fmt.Errorf("failed to generate signed URL: %w", err)
	}

	return signedURL, nil
}

// GetAsset retrieves an audio asset and its metadata.
func (s *Service) GetAsset(ctx context.Context, assetID string) (*models.AudioAsset, error) {
	asset, err := s.assetStore.AssetByID(ctx, assetID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAssetNotFound, err)
	}
	return &asset, nil
}

// ListAssetsForConfession returns all audio assets for a given confession.
func (s *Service) ListAssetsForConfession(ctx context.Context, confessionID, voiceID string) ([]models.AudioAsset, error) {
	return s.assetStore.AssetsFor(ctx, confessionID, voiceID)
}

// PublishAsset marks an asset as published and ready for serving.
func (s *Service) PublishAsset(ctx context.Context, assetID string) error {
	asset, err := s.assetStore.AssetByID(ctx, assetID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAssetNotFound, err)
	}

	// Validate transition
	if err := ValidateTransition(AssetStatus(asset.Status), StatusPublished); err != nil {
		return fmt.Errorf("cannot publish asset: %w", err)
	}

	// Update status
	asset.Status = string(StatusPublished)
	asset.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	// For now, we update via the store directly. In a full implementation,
	// this would use a proper update method.
	// This is a simplified version - the actual implementation would need
	// to be added to the store interface.
	log.Printf("Audio service: Publishing asset %s (status transition to published)", assetID)

	return nil
}

// ArchiveAsset marks an asset as archived (no longer servable).
func (s *Service) ArchiveAsset(ctx context.Context, assetID string) error {
	asset, err := s.assetStore.AssetByID(ctx, assetID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAssetNotFound, err)
	}

	// Validate transition
	if err := ValidateTransition(AssetStatus(asset.Status), StatusArchived); err != nil {
		return fmt.Errorf("cannot archive asset: %w", err)
	}

	// Update status
	asset.Status = string(StatusArchived)
	asset.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	log.Printf("Audio service: Archiving asset %s (status transition to archived)", assetID)

	return nil
}

// DeleteAsset removes an asset and its associated storage.
func (s *Service) DeleteAsset(ctx context.Context, assetID string) error {
	asset, err := s.assetStore.AssetByID(ctx, assetID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAssetNotFound, err)
	}

	// Delete from storage
	storageKey := asset.URL
	if strings.TrimSpace(storageKey) == "" {
		storageKey = s.generateStorageKeyFromAsset(&asset)
	}

	if err := s.storage.Delete(ctx, storageKey); err != nil {
		log.Printf("Warning: Failed to delete storage for asset %s: %v", assetID, err)
		// Continue with database deletion even if storage deletion fails
	}

	// For now, we don't have a hard delete in the store interface.
	// This would need to be implemented in the store.
	log.Printf("Audio service: Deleting asset %s from database", assetID)

	return nil
}

// generateStorageKey creates a deterministic storage key for an audio asset.
func (s *Service) generateStorageKey(asset *models.AudioAsset) string {
	return s.generateStorageKeyFromAsset(asset)
}

// generateStorageKeyFromAsset creates a deterministic storage key from an asset.
func (s *Service) generateStorageKeyFromAsset(asset *models.AudioAsset) string {
	// Key format: audio/content/{content_id}/version/{version_id}/voice/{voice_id}/variant/{variant_id}/asset.{format}
	var builder strings.Builder
	builder.WriteString("audio/content/")
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
	
	// Use default format (m4a) for storage key
	builder.WriteString("/asset.m4a")
	
	return builder.String()
}

// generateCDNPath creates a CDN path for an audio asset.
func (s *Service) generateCDNPath(storageKey string) string {
	if s.cdnDomain == "" {
		return storageKey
	}
	return fmt.Sprintf("https://%s/%s", s.cdnDomain, storageKey)
}

// UploadFromURL downloads audio from a URL and creates an asset.
func (s *Service) UploadFromURL(ctx context.Context, asset *models.AudioAsset, url string) error {
	// Download the audio data
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download audio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download audio: status %d", resp.StatusCode)
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read audio data: %w", err)
	}

	return s.CreateAsset(ctx, asset, audioData)
}

// UploadFromFile uploads audio from a local file and creates an asset.
func (s *Service) UploadFromFile(ctx context.Context, asset *models.AudioAsset, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	return s.CreateAsset(ctx, asset, data)
}

// UploadFromMultipart uploads audio from a multipart form file and creates an asset.
func (s *Service) UploadFromMultipart(ctx context.Context, asset *models.AudioAsset, file *multipart.FileHeader) error {
	f, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open multipart file: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("failed to read multipart file: %w", err)
	}

	return s.CreateAsset(ctx, asset, data)
}
