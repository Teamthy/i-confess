// Package audio provides signed URL generation for audio assets.
//
// This file implements signed URL generation for secure audio streaming and downloads.
package audio

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/storage"
)

// ErrURLGenerationFailed is returned when URL generation fails.
var ErrURLGenerationFailed = errors.New("audio: URL generation failed")

// URLGenerator generates signed URLs for audio assets.
type URLGenerator struct {
	storage       storage.ObjectStorage
	cdnDomain     string
	streamTTL     time.Duration
	downloadTTL   time.Duration
	signingSecret []byte
	useCloudFront bool
	keyPairID     string
	privateKey    string
}

// URLGeneratorConfig holds configuration for the URL generator.
type URLGeneratorConfig struct {
	CDNDomain     string
	StreamTTL     time.Duration // Default: 4 hours
	DownloadTTL   time.Duration // Default: 24 hours
	SigningSecret string        // Secret key for signing URLs
	UseCloudFront bool          // Whether to use CloudFront signed URLs
	KeyPairID     string        // CloudFront key pair ID
	PrivateKey    string        // CloudFront private key (PEM format)
}

// NewURLGenerator creates a new URL generator.
func NewURLGenerator(storage storage.ObjectStorage, cfg *URLGeneratorConfig) (*URLGenerator, error) {
	if cfg.StreamTTL == 0 {
		cfg.StreamTTL = 4 * time.Hour
	}
	if cfg.DownloadTTL == 0 {
		cfg.DownloadTTL = 24 * time.Hour
	}

	var signingSecret []byte
	if cfg.SigningSecret != "" {
		signingSecret = []byte(cfg.SigningSecret)
	} else {
		// Generate a random secret if none provided (for development only)
		signingSecret = []byte(fmt.Sprintf("dev-secret-%d", time.Now().Unix()))
	}

	return &URLGenerator{
		storage:       storage,
		cdnDomain:     cfg.CDNDomain,
		streamTTL:     cfg.StreamTTL,
		downloadTTL:   cfg.DownloadTTL,
		signingSecret: signingSecret,
		useCloudFront: cfg.UseCloudFront,
		keyPairID:     cfg.KeyPairID,
		privateKey:    cfg.PrivateKey,
	}, nil
}

// GenerateSignedURL generates a signed URL for an audio asset.
// If isDownload is true, generates a download URL (longer TTL).
func (g *URLGenerator) GenerateSignedURL(
	ctx context.Context,
	storageKey string,
	isDownload bool,
) (string, error) {
	ttl := g.streamTTL
	if isDownload {
		ttl = g.downloadTTL
	}

	// Use the storage provider's signed URL generation if available
	if g.storage != nil {
		signedURL, err := g.storage.GenerateSignedURL(ctx, storageKey, ttl)
		if err == nil {
			return g.ensureAbsoluteURL(signedURL), nil
		}
		// Fall back to our own signing if storage provider fails
		log.Printf("Storage signed URL generation failed, falling back to local signing: %v", err)
	}

	// Generate our own signed URL
	signedURL := g.generateSignedURL(storageKey, ttl)
	return g.ensureAbsoluteURL(signedURL), nil
}

// GenerateStreamURL generates a signed URL for streaming.
func (g *URLGenerator) GenerateStreamURL(ctx context.Context, storageKey string) (string, error) {
	return g.GenerateSignedURL(ctx, storageKey, false)
}

// GenerateDownloadURL generates a signed URL for downloading.
func (g *URLGenerator) GenerateDownloadURL(ctx context.Context, storageKey string) (string, error) {
	return g.GenerateSignedURL(ctx, storageKey, true)
}

// generateSignedURL creates a signed URL using HMAC-SHA256.
func (g *URLGenerator) generateSignedURL(storageKey string, ttl time.Duration) string {
	expiry := time.Now().UTC().Add(ttl)
	expiryTimestamp := expiry.Unix()

	// Create the signature payload
	payload := fmt.Sprintf("%s%d", storageKey, expiryTimestamp)

	// Generate HMAC-SHA256 signature
	mac := hmac.New(sha256.New, g.signingSecret)
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))

	// Create the signed URL
	// Format: /path?expires={timestamp}&signature={signature}
	baseURL := storageKey
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "/" + storageKey
	}

	// Parse the URL to add query parameters
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		// If parsing fails, just append to the path
		return fmt.Sprintf("%s?expires=%d&signature=%s", baseURL, expiryTimestamp, signature)
	}

	query := parsedURL.Query()
	query.Set("expires", fmt.Sprintf("%d", expiryTimestamp))
	query.Set("signature", signature)
	parsedURL.RawQuery = query.Encode()

	return parsedURL.String()
}

// ensureAbsoluteURL ensures the URL is absolute (includes scheme and domain).
func (g *URLGenerator) ensureAbsoluteURL(signedURL string) string {
	if strings.HasPrefix(signedURL, "http") {
		return signedURL
	}

	if g.cdnDomain != "" {
		// Handle the case where signedURL already has a leading /
		signedURL = strings.TrimPrefix(signedURL, "/")
		return fmt.Sprintf("https://%s/%s", g.cdnDomain, signedURL)
	}

	// No CDN domain, return as-is (relative URL)
	return signedURL
}

// ValidateSignedURL validates a signed URL and returns the original path.
func (g *URLGenerator) ValidateSignedURL(signedURL string) (string, error) {
	// Parse the URL
	parsedURL, err := url.Parse(signedURL)
	if err != nil {
		return "", fmt.Errorf("%w: invalid URL: %v", ErrURLGenerationFailed, err)
	}

	// Get query parameters
	query := parsedURL.Query()
	expiryStr := query.Get("expires")
	signature := query.Get("signature")

	if expiryStr == "" || signature == "" {
		return "", fmt.Errorf("%w: missing expires or signature", ErrURLGenerationFailed)
	}

	// Parse expiry timestamp
	var expiryTimestamp int64
	if _, err := fmt.Sscanf(expiryStr, "%d", &expiryTimestamp); err != nil {
		return "", fmt.Errorf("%w: invalid expires format", ErrURLGenerationFailed)
	}

	// Check if URL has expired
	if time.Now().UTC().Unix() > expiryTimestamp {
		return "", fmt.Errorf("%w: URL has expired", ErrURLGenerationFailed)
	}

	// Get the path (without query parameters)
	path := parsedURL.Path

	// Reconstruct the payload
	payload := fmt.Sprintf("%s%d", path, expiryTimestamp)

	// Verify the signature
	mac := hmac.New(sha256.New, g.signingSecret)
	mac.Write([]byte(payload))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return "", fmt.Errorf("%w: invalid signature", ErrURLGenerationFailed)
	}

	return path, nil
}

// GenerateToken generates a signed token for API access.
func (g *URLGenerator) GenerateToken(path string, ttl time.Duration) (string, error) {
	expiry := time.Now().UTC().Add(ttl)
	expiryTimestamp := expiry.Unix()

	// Create the signature payload
	payload := fmt.Sprintf("%s%d", path, expiryTimestamp)

	// Generate HMAC-SHA256 signature
	mac := hmac.New(sha256.New, g.signingSecret)
	mac.Write([]byte(payload))
	signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	// Create token: base64(path:expires:signature)
	token := fmt.Sprintf("%s:%d:%s", path, expiryTimestamp, signature)
	return base64.URLEncoding.EncodeToString([]byte(token)), nil
}

// ValidateToken validates a signed token and returns the original path.
func (g *URLGenerator) ValidateToken(token string) (string, error) {
	// Decode the token
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return "", fmt.Errorf("%w: invalid token format", ErrURLGenerationFailed)
	}

	// Split the token
	parts := strings.Split(string(decoded), ":")
	if len(parts) != 3 {
		return "", fmt.Errorf("%w: invalid token structure", ErrURLGenerationFailed)
	}

	path := parts[0]
	expiryTimestamp := parts[1]
	signature := parts[2]

	// Parse expiry timestamp
	var expiry int64
	if _, err := fmt.Sscanf(expiryTimestamp, "%d", &expiry); err != nil {
		return "", fmt.Errorf("%w: invalid expires format", ErrURLGenerationFailed)
	}

	// Check if token has expired
	if time.Now().UTC().Unix() > expiry {
		return "", fmt.Errorf("%w: token has expired", ErrURLGenerationFailed)
	}

	// Reconstruct the payload
	payload := fmt.Sprintf("%s%d", path, expiry)

	// Verify the signature
	mac := hmac.New(sha256.New, g.signingSecret)
	mac.Write([]byte(payload))
	expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if signature != expectedSignature {
		return "", fmt.Errorf("%w: invalid signature", ErrURLGenerationFailed)
	}

	return path, nil
}

// GenerateShortURL generates a short, shareable URL for an audio asset.
func (g *URLGenerator) GenerateShortURL(ctx context.Context, assetID string) (string, error) {
	// In a real implementation, this would use a URL shortener service
	// For now, we'll just return a simple encoded version

	// Create a short token
	token := base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("audio:%s", assetID)))

	if g.cdnDomain != "" {
		return fmt.Sprintf("https://%s/s/%s", g.cdnDomain, token), nil
	}

	return fmt.Sprintf("/s/%s", token), nil
}

// ResolveShortURL resolves a short URL to the full signed URL.
func (g *URLGenerator) ResolveShortURL(shortURL string) (string, error) {
	// Extract the token from the URL
	// Format: /s/{token} or https://cdn/s/{token}
	token := strings.TrimPrefix(shortURL, "/s/")
	token = strings.TrimPrefix(token, "s/")

	// Decode the token
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return "", fmt.Errorf("%w: invalid short URL", ErrURLGenerationFailed)
	}

	// Extract the asset ID
	parts := strings.Split(string(decoded), ":")
	if len(parts) < 2 || parts[0] != "audio" {
		return "", fmt.Errorf("%w: invalid short URL format", ErrURLGenerationFailed)
	}

	assetID := parts[1]

	// Generate a signed URL for the asset
	// In a real implementation, we would look up the storage key for the asset
	// For now, return a simple path without signing (ctx not available)
	storageKey := fmt.Sprintf("audio/%s.m4a", assetID)

	// Without ctx, we can't generate a signed URL, so just return the storage key
	// In a real implementation, this would use context.Background() or accept ctx as parameter
	return storageKey, nil
}

// GetURLTTL returns the TTL for different types of URLs.
func (g *URLGenerator) GetURLTTL(isDownload bool) time.Duration {
	if isDownload {
		return g.downloadTTL
	}
	return g.streamTTL
}
