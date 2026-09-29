// Package audio provides audio playback resolution and authorization.
//
// This file implements the playback resolver that checks entitlements and
// generates signed URLs for audio streaming.
package audio

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/storage"
)

// ErrNotEntitled is returned when a user is not entitled to play the requested audio.
var ErrNotEntitled = errors.New("audio: not entitled to play this content")

// ErrAssetNotReady is returned when an audio asset is not in a playable state.
var ErrAssetNotReady = errors.New("audio: asset not ready for playback")

// PlaybackResolver resolves audio playback requests with authorization.
type PlaybackResolver struct {
	storage     storage.ObjectStorage
	assetStore  AssetStorer
	voiceStore  VoiceStorer
	cdnDomain   string
	streamTTL   time.Duration
	downloadTTL time.Duration
	// allowAllPremiumVoices is a configuration flag
	allowAllPremiumVoices bool
}

// PlaybackResolverConfig holds configuration for the playback resolver.
type PlaybackResolverConfig struct {
	CDNDomain       string
	StreamTTL       time.Duration // Default: 4 hours
	DownloadTTL     time.Duration // Default: 24 hours
	AllowAllPremium bool          // If true, allow all premium voices (dev mode)
}

// NewPlaybackResolver creates a new playback resolver.
func NewPlaybackResolver(
	storage storage.ObjectStorage,
	assetStore AssetStorer,
	voiceStore VoiceStorer,
	cfg *PlaybackResolverConfig,
) *PlaybackResolver {
	streamTTL := cfg.StreamTTL
	if streamTTL == 0 {
		streamTTL = 4 * time.Hour
	}
	downloadTTL := cfg.DownloadTTL
	if downloadTTL == 0 {
		downloadTTL = 24 * time.Hour
	}

	return &PlaybackResolver{
		storage:               storage,
		assetStore:            assetStore,
		voiceStore:            voiceStore,
		cdnDomain:             cfg.CDNDomain,
		streamTTL:             streamTTL,
		downloadTTL:           downloadTTL,
		allowAllPremiumVoices: cfg.AllowAllPremium,
	}
}

// PlaybackRequest represents a request to play audio.
type PlaybackRequest struct {
	UserID       string
	AssetID      string
	ConfessionID string
	VoiceID      string
	IsDownload   bool // If true, generate a download URL (longer TTL)
}

// PlaybackResponse contains the resolved playback information.
type PlaybackResponse struct {
	AssetID         string `json:"asset_id"`
	ConfessionID    string `json:"confession_id"`
	VoiceID         string `json:"voice_id"`
	Format          string `json:"format"`
	DurationSeconds int    `json:"duration_seconds"`
	SizeBytes       int64  `json:"size_bytes"`
	StreamURL       string `json:"stream_url"`
	ExpiresAt       string `json:"expires_at"`
	// VoiceDowngraded indicates a premium voice was substituted with a free one
	VoiceDowngraded bool   `json:"voice_downgraded,omitempty"`
	DowngradeReason string `json:"downgrade_reason,omitempty"`
}

// ResolvePlayback resolves a playback request and returns a signed URL.
func (r *PlaybackResolver) ResolvePlayback(ctx context.Context, req *PlaybackRequest) (*PlaybackResponse, error) {
	// Get the asset
	asset, err := r.getAsset(ctx, req.AssetID, req.ConfessionID, req.VoiceID)
	if err != nil {
		return nil, err
	}

	// Check if asset is servable
	if !IsServed(asset.Status) {
		return nil, fmt.Errorf("%w: asset %s has status %s", ErrAssetNotReady, asset.ID, asset.Status)
	}

	// Check if voice is premium and user access
	voice, err := r.voiceStore.VoiceByID(ctx, asset.VoiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get voice: %w", err)
	}

	// If voice is premium and user is specified, check access
	if req.UserID != "" && voice.Premium && !r.allowAllPremiumVoices {
		// Try to downgrade to a free voice
		downgradedAsset, downgradeReason, err := r.tryDowngrade(ctx, req.ConfessionID, asset.VoiceID)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrNotEntitled, err)
		}
		if downgradedAsset.ID != "" {
			v, _ := r.voiceStore.VoiceByID(ctx, downgradedAsset.VoiceID)
			return r.buildResponse(ctx, downgradedAsset, *v, req.IsDownload, true, downgradeReason)
		}
		return nil, fmt.Errorf("%w: user %s does not have access to premium voice %s", ErrNotEntitled, req.UserID, asset.VoiceID)
	}
	// If allowAllPremiumVoices is true (dev mode), grant access to all voices

	return r.buildResponse(ctx, *asset, *voice, req.IsDownload, false, "")
}

// GetPlaybackURL returns a signed URL for streaming an audio asset.
func (r *PlaybackResolver) GetPlaybackURL(ctx context.Context, userID, assetID string) (string, error) {
	req := &PlaybackRequest{
		UserID:     userID,
		AssetID:    assetID,
		IsDownload: false,
	}
	resp, err := r.ResolvePlayback(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.StreamURL, nil
}

// GetDownloadURL returns a signed URL for downloading an audio asset.
func (r *PlaybackResolver) GetDownloadURL(ctx context.Context, userID, assetID string) (string, error) {
	req := &PlaybackRequest{
		UserID:     userID,
		AssetID:    assetID,
		IsDownload: true,
	}
	resp, err := r.ResolvePlayback(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.StreamURL, nil
}

// getAsset retrieves an audio asset by ID or by confession/voice.
func (r *PlaybackResolver) getAsset(ctx context.Context, assetID, confessionID, voiceID string) (*models.AudioAsset, error) {
	if assetID != "" {
		asset, err := r.assetStore.AssetByID(ctx, assetID)
		if err != nil {
			return nil, fmt.Errorf("asset not found: %w", err)
		}
		return &asset, nil
	}

	// If no asset ID, try to find by confession and voice
	assets, err := r.assetStore.AssetsFor(ctx, confessionID, voiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list assets: %w", err)
	}

	if len(assets) == 0 {
		return nil, fmt.Errorf("%w: no assets found for confession %s, voice %s", ErrAssetNotFound, confessionID, voiceID)
	}

	// Return the first servable asset
	for i := range assets {
		if IsServed(assets[i].Status) {
			return &assets[i], nil
		}
	}

	return nil, fmt.Errorf("%w: no servable assets found for confession %s, voice %s", ErrAssetNotReady, confessionID, voiceID)
}

// tryDowngrade attempts to find a free voice alternative for the requested confession.
func (r *PlaybackResolver) tryDowngrade(ctx context.Context, confessionID, requestedVoiceID string) (models.AudioAsset, string, error) {
	// Get all assets for this confession
	assets, err := r.assetStore.AssetsFor(ctx, confessionID, "")
	if err != nil {
		return models.AudioAsset{}, "", fmt.Errorf("failed to list assets for downgrade: %w", err)
	}

	// Find a servable asset with a non-premium voice
	for i := range assets {
		if !IsServed(assets[i].Status) {
			continue
		}

		// Check if this voice is free (not premium)
		voice, err := r.voiceStore.VoiceByID(ctx, assets[i].VoiceID)
		if err != nil {
			continue // Skip if we can't get voice info
		}

		if !voice.Premium {
			return assets[i], fmt.Sprintf("voice %s not available, using free voice %s", requestedVoiceID, assets[i].VoiceID), nil
		}
	}

	return models.AudioAsset{}, "", fmt.Errorf("no free voice available for confession %s", confessionID)
}

// buildResponse constructs a PlaybackResponse from an asset and voice.
func (r *PlaybackResolver) buildResponse(
	ctx context.Context,
	asset models.AudioAsset,
	voice models.Voice,
	isDownload bool,
	downgraded bool,
	downgradeReason string,
) (*PlaybackResponse, error) {
	// Generate signed URL
	storageKey := asset.URL
	if strings.TrimSpace(storageKey) == "" {
		// Fallback key generation with default format
		storageKey = fmt.Sprintf("audio/content/%s/voice/%s/asset.m4a", asset.ConfessionID, asset.VoiceID)
	}

	ttl := r.streamTTL
	if isDownload {
		ttl = r.downloadTTL
	}

	signedURL, err := r.storage.GenerateSignedURL(ctx, storageKey, ttl)
	if err != nil {
		return nil, fmt.Errorf("failed to generate signed URL: %w", err)
	}

	// Ensure the URL is absolute
	if !strings.HasPrefix(signedURL, "http") && r.cdnDomain != "" {
		signedURL = fmt.Sprintf("https://%s/%s", r.cdnDomain, signedURL)
	}

	// Parse and validate the URL
	parsedURL, err := url.Parse(signedURL)
	if err != nil {
		return nil, fmt.Errorf("invalid signed URL: %w", err)
	}

	// Calculate expiration
	expiresAt := time.Now().UTC().Add(ttl)

	resp := &PlaybackResponse{
		AssetID:         asset.ID,
		ConfessionID:    asset.ConfessionID,
		VoiceID:         asset.VoiceID,
		Format:          "m4a", // Default format, can be determined from URL
		DurationSeconds: asset.DurationSeconds,
		SizeBytes:       asset.SizeBytes,
		StreamURL:       parsedURL.String(),
		ExpiresAt:       expiresAt.Format(time.RFC3339),
		VoiceDowngraded: downgraded,
		DowngradeReason: downgradeReason,
	}

	return resp, nil
}

// ValidatePlayback checks if an asset can be played without generating a URL.
func (r *PlaybackResolver) ValidatePlayback(ctx context.Context, userID, assetID string) error {
	asset, err := r.assetStore.AssetByID(ctx, assetID)
	if err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}

	if !IsServed(asset.Status) {
		return fmt.Errorf("%w: asset %s has status %s", ErrAssetNotReady, assetID, asset.Status)
	}

	// Check voice entitlement
	voice, err := r.voiceStore.VoiceByID(ctx, asset.VoiceID)
	if err != nil {
		return fmt.Errorf("failed to get voice: %w", err)
	}

	if userID != "" && voice.Premium && !r.allowAllPremiumVoices {
		// Check if downgrade is possible
		_, _, err := r.tryDowngrade(ctx, asset.ConfessionID, asset.VoiceID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrNotEntitled, err)
		}
		// Downgrade is possible, but we just validate, don't generate URL
	}

	return nil
}

// GetAssetStatus returns the current status of an audio asset.
func (r *PlaybackResolver) GetAssetStatus(ctx context.Context, assetID string) (string, error) {
	asset, err := r.assetStore.AssetByID(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("asset not found: %w", err)
	}
	return asset.Status, nil
}

// IsAssetReady checks if an asset is ready for playback.
func (r *PlaybackResolver) IsAssetReady(ctx context.Context, assetID string) (bool, error) {
	status, err := r.GetAssetStatus(ctx, assetID)
	if err != nil {
		return false, err
	}
	return IsServed(status), nil
}
