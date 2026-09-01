package storage

import (
	"context"
	"fmt"
	"time"
)

// ObjectStorage defines the interface for all storage providers.
// This abstraction allows the system to support S3, GCS, Azure, local file storage,
// and future providers without changing the rest of the codebase.
//
// Key principle: Audio binaries NEVER go in the database. They live in object storage.
type ObjectStorage interface {
	// Upload stores a file with the given key and optional metadata.
	// Key format: audio/content/{id}/version/{v}/voice/{voice_id}/{asset_type}/{quality}/audio.{format}
	Upload(ctx context.Context, key string, data []byte, metadata map[string]string) error

	// Download retrieves a file by key.
	Download(ctx context.Context, key string) ([]byte, error)

	// Delete removes a file from storage (soft delete is metadata-only, hard delete removes binary).
	Delete(ctx context.Context, key string) error

	// GenerateSignedURL creates a time-limited URL for direct access without credentials.
	// Used for playback (4h expiry) and download (24h expiry).
	GenerateSignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)

	// List returns all keys with the given prefix (useful for cleanup, audits).
	List(ctx context.Context, prefix string) ([]string, error)

	// Exists checks if a key exists without downloading.
	Exists(ctx context.Context, key string) (bool, error)

	// GetSize returns file size in bytes without downloading the whole file.
	GetSize(ctx context.Context, key string) (int64, error)

	// GetMetadata retrieves custom metadata stored during upload.
	GetMetadata(ctx context.Context, key string) (map[string]string, error)
}

// StorageConfig holds configuration for the selected storage provider.
type StorageConfig struct {
	Provider string // "s3" | "gcs" | "azure" | "local"

	// S3-specific
	S3Bucket    string
	S3Region    string
	S3AccessKey string
	S3SecretKey string
	S3Endpoint  string // Optional for S3-compatible services

	// GCS-specific
	GCSProject      string
	GCSBucket       string
	GCSCredentialsJSON string // Path or inline JSON

	// Azure-specific
	AzureAccount    string
	AzureContainer  string
	AzureKey        string

	// Local-specific
	LocalRootPath string // Absolute path for development/testing

	// Common
	CDNDomain   string        // Optional CDN domain for signed URLs (e.g., audio.example.com)
	CDNProvider string        // "cloudflare" | "cloudfront" | "bunny" (may not apply to all storage)
	CacheTTL    time.Duration // Default cache TTL for CDN
}

// New creates a storage provider based on configuration.
func New(cfg *StorageConfig) (ObjectStorage, error) {
	switch cfg.Provider {
	case "s3":
		return NewS3Storage(cfg)
	case "gcs":
		return NewGCSStorage(cfg)
	case "azure":
		return NewAzureStorage(cfg)
	case "local":
		return NewLocalStorage(cfg)
	default:
		return nil, fmt.Errorf("unsupported storage provider: %s", cfg.Provider)
	}
}

// UploadOptions provides additional context for uploads (used by implementations as needed).
type UploadOptions struct {
	ContentType      string
	CacheControl     string
	ServerSideEncrypt bool
}

// StorageError wraps storage-related errors with retry policy.
type StorageError struct {
	Op       string // "upload" | "download" | "delete" | "sign_url"
	Key      string
	Err      error
	Retryable bool // Whether this error should trigger a retry
}

func (e *StorageError) Error() string {
	return fmt.Sprintf("storage %s failed for key %s: %v (retryable: %v)", e.Op, e.Key, e.Err, e.Retryable)
}

// Helper to determine if an error is retryable (used by job workers).
func IsRetryable(err error) bool {
	if storageErr, ok := err.(*StorageError); ok {
		return storageErr.Retryable
	}
	return false
}
