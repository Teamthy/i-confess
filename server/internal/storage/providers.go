package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// S3Storage implements ObjectStorage for Amazon S3.
type S3Storage struct {
	bucket string
	region string
	// client *s3.Client (to be initialized with AWS SDK)
}

// NewS3Storage creates a new S3 storage provider.
func NewS3Storage(cfg *StorageConfig) (ObjectStorage, error) {
	if cfg.S3Bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET not configured")
	}
	if cfg.S3Region == "" {
		return nil, fmt.Errorf("S3_REGION not configured")
	}

	// TODO: Initialize AWS S3 client with credentials from cfg.S3AccessKey, cfg.S3SecretKey
	// For now, stub implementation

	return &S3Storage{
		bucket: cfg.S3Bucket,
		region: cfg.S3Region,
	}, nil
}

func (s *S3Storage) Upload(ctx context.Context, key string, data []byte, metadata map[string]string) error {
	// TODO: Implement S3 PutObject with metadata
	return fmt.Errorf("S3Storage.Upload not yet implemented")
}

func (s *S3Storage) Download(ctx context.Context, key string) ([]byte, error) {
	// TODO: Implement S3 GetObject
	return nil, fmt.Errorf("S3Storage.Download not yet implemented")
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	// TODO: Implement S3 DeleteObject
	return fmt.Errorf("S3Storage.Delete not yet implemented")
}

func (s *S3Storage) GenerateSignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	// TODO: Use AWS Signature Version 4 presigner for time-limited GET access
	return "", fmt.Errorf("S3Storage.GenerateSignedURL not yet implemented")
}

func (s *S3Storage) List(ctx context.Context, prefix string) ([]string, error) {
	// TODO: Implement S3 ListObjectsV2
	return nil, fmt.Errorf("S3Storage.List not yet implemented")
}

func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	// TODO: Use HeadObject to check existence without downloading
	return false, fmt.Errorf("S3Storage.Exists not yet implemented")
}

func (s *S3Storage) GetSize(ctx context.Context, key string) (int64, error) {
	// TODO: Use HeadObject to get ContentLength
	return 0, fmt.Errorf("S3Storage.GetSize not yet implemented")
}

func (s *S3Storage) GetMetadata(ctx context.Context, key string) (map[string]string, error) {
	// TODO: Use HeadObject to retrieve Metadata
	return nil, fmt.Errorf("S3Storage.GetMetadata not yet implemented")
}

// ===== GCS Storage =====

// GCSStorage implements ObjectStorage for Google Cloud Storage.
type GCSStorage struct {
	project string
	bucket  string
	// client *storage.Client (to be initialized with GCS SDK)
}

// NewGCSStorage creates a new GCS storage provider.
func NewGCSStorage(cfg *StorageConfig) (ObjectStorage, error) {
	if cfg.GCSProject == "" {
		return nil, fmt.Errorf("GCS_PROJECT not configured")
	}
	if cfg.GCSBucket == "" {
		return nil, fmt.Errorf("GCS_BUCKET not configured")
	}

	// TODO: Initialize GCS client from cfg.GCSCredentialsJSON

	return &GCSStorage{
		project: cfg.GCSProject,
		bucket:  cfg.GCSBucket,
	}, nil
}

func (g *GCSStorage) Upload(ctx context.Context, key string, data []byte, metadata map[string]string) error {
	// TODO: Implement GCS object write with metadata
	return fmt.Errorf("GCSStorage.Upload not yet implemented")
}

func (g *GCSStorage) Download(ctx context.Context, key string) ([]byte, error) {
	// TODO: Implement GCS object read
	return nil, fmt.Errorf("GCSStorage.Download not yet implemented")
}

func (g *GCSStorage) Delete(ctx context.Context, key string) error {
	// TODO: Implement GCS object delete
	return fmt.Errorf("GCSStorage.Delete not yet implemented")
}

func (g *GCSStorage) GenerateSignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	// TODO: Use storage.SignedURL with service account key for time-limited GET
	return "", fmt.Errorf("GCSStorage.GenerateSignedURL not yet implemented")
}

func (g *GCSStorage) List(ctx context.Context, prefix string) ([]string, error) {
	// TODO: Implement GCS object iteration with prefix
	return nil, fmt.Errorf("GCSStorage.List not yet implemented")
}

func (g *GCSStorage) Exists(ctx context.Context, key string) (bool, error) {
	// TODO: Use Attrs to check existence
	return false, fmt.Errorf("GCSStorage.Exists not yet implemented")
}

func (g *GCSStorage) GetSize(ctx context.Context, key string) (int64, error) {
	// TODO: Use Attrs to get Size
	return 0, fmt.Errorf("GCSStorage.GetSize not yet implemented")
}

func (g *GCSStorage) GetMetadata(ctx context.Context, key string) (map[string]string, error) {
	// TODO: Use Attrs to retrieve Metadata
	return nil, fmt.Errorf("GCSStorage.GetMetadata not yet implemented")
}

// ===== Azure Storage =====

// AzureStorage implements ObjectStorage for Azure Blob Storage.
type AzureStorage struct {
	account   string
	container string
	// client *azblob.Client (to be initialized with Azure SDK)
}

// NewAzureStorage creates a new Azure storage provider.
func NewAzureStorage(cfg *StorageConfig) (ObjectStorage, error) {
	if cfg.AzureAccount == "" {
		return nil, fmt.Errorf("AZURE_ACCOUNT not configured")
	}
	if cfg.AzureContainer == "" {
		return nil, fmt.Errorf("AZURE_CONTAINER not configured")
	}

	// TODO: Initialize Azure blob client from cfg.AzureKey

	return &AzureStorage{
		account:   cfg.AzureAccount,
		container: cfg.AzureContainer,
	}, nil
}

func (a *AzureStorage) Upload(ctx context.Context, key string, data []byte, metadata map[string]string) error {
	// TODO: Implement Azure Upload with metadata
	return fmt.Errorf("AzureStorage.Upload not yet implemented")
}

func (a *AzureStorage) Download(ctx context.Context, key string) ([]byte, error) {
	// TODO: Implement Azure Download
	return nil, fmt.Errorf("AzureStorage.Download not yet implemented")
}

func (a *AzureStorage) Delete(ctx context.Context, key string) error {
	// TODO: Implement Azure Delete
	return fmt.Errorf("AzureStorage.Delete not yet implemented")
}

func (a *AzureStorage) GenerateSignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	// TODO: Use SAS token for time-limited access
	return "", fmt.Errorf("AzureStorage.GenerateSignedURL not yet implemented")
}

func (a *AzureStorage) List(ctx context.Context, prefix string) ([]string, error) {
	// TODO: Implement Azure List blobs with prefix
	return nil, fmt.Errorf("AzureStorage.List not yet implemented")
}

func (a *AzureStorage) Exists(ctx context.Context, key string) (bool, error) {
	// TODO: Use GetProperties to check existence
	return false, fmt.Errorf("AzureStorage.Exists not yet implemented")
}

func (a *AzureStorage) GetSize(ctx context.Context, key string) (int64, error) {
	// TODO: Use GetProperties to get ContentLength
	return 0, fmt.Errorf("AzureStorage.GetSize not yet implemented")
}

func (a *AzureStorage) GetMetadata(ctx context.Context, key string) (map[string]string, error) {
	// TODO: Use GetProperties to retrieve metadata
	return nil, fmt.Errorf("AzureStorage.GetMetadata not yet implemented")
}

// ===== Local File Storage =====

// LocalStorage implements ObjectStorage using the local file system (development/testing only).
type LocalStorage struct {
	rootPath string
	// baseURL is the URL prefix signed links point at, e.g. "/media".
	baseURL string
	// secret signs URLs. Distinct from the JWT secret so either can be rotated
	// independently.
	secret string
	// now is injectable so expiry behaviour is testable without sleeping.
	now func() time.Time
}

// NewLocalStorage creates a file-system provider for development and tests.
//
// It implements the same signed-URL contract as the production CDN so expiry,
// tampering and range-request behaviour are exercised in every environment
// rather than only discovered in production.
func NewLocalStorage(cfg *StorageConfig) (ObjectStorage, error) {
	if cfg.LocalRootPath == "" {
		return nil, fmt.Errorf("LOCAL_ROOT_PATH not configured")
	}
	if err := os.MkdirAll(cfg.LocalRootPath, 0o755); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	base := cfg.CDNDomain
	if base == "" {
		base = "/media"
	}
	return &LocalStorage{
		rootPath: cfg.LocalRootPath,
		baseURL:  strings.TrimRight(base, "/"),
		secret:   cfg.SigningSecret,
		now:      time.Now,
	}, nil
}

// SetClock overrides the clock. Test-only.
func (l *LocalStorage) SetClock(f func() time.Time) { l.now = f }

func (l *LocalStorage) clock() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

// path resolves a key to a filesystem path, refusing anything that escapes the
// storage root.
func (l *LocalStorage) path(key string) (string, error) {
	if !ValidKey(key) {
		return "", fmt.Errorf("invalid storage key %q", key)
	}
	root, err := filepath.Abs(l.rootPath)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(key)))
	if err != nil {
		return "", err
	}
	// Defence in depth: confirm containment even after ValidKey and cleaning.
	if !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("storage key escapes root")
	}
	return abs, nil
}

func (l *LocalStorage) Upload(ctx context.Context, key string, data []byte, metadata map[string]string) error {
	p, err := l.path(key)
	if err != nil {
		return &StorageError{Op: "upload", Key: key, Err: err}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return &StorageError{Op: "upload", Key: key, Err: err, Retryable: true}
	}
	// Write to a temp file then rename. A crash mid-write must not leave a
	// truncated object that would later be served as valid audio.
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return &StorageError{Op: "upload", Key: key, Err: err, Retryable: true}
	}
	if err := os.Rename(tmp, p); err != nil {
		return &StorageError{Op: "upload", Key: key, Err: err, Retryable: true}
	}
	if len(metadata) > 0 {
		if b, err := json.Marshal(metadata); err == nil {
			_ = os.WriteFile(p+".meta.json", b, 0o644)
		}
	}
	return nil
}

func (l *LocalStorage) Download(ctx context.Context, key string) ([]byte, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, &StorageError{Op: "download", Key: key, Err: err}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, &StorageError{Op: "download", Key: key, Err: err}
	}
	return data, nil
}

func (l *LocalStorage) Delete(ctx context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return &StorageError{Op: "delete", Key: key, Err: err}
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return &StorageError{Op: "delete", Key: key, Err: err}
	}
	_ = os.Remove(p + ".meta.json")
	return nil
}

// GenerateSignedURL mints a time-limited link. The returned URL is the only
// way to read an object: the origin refuses unsigned requests.
func (l *LocalStorage) GenerateSignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if !ValidKey(key) {
		return "", &StorageError{Op: "sign_url", Key: key, Err: fmt.Errorf("invalid storage key")}
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	exp := l.clock().Add(ttl).Unix()
	sig := Sign(l.secret, key, exp)
	return fmt.Sprintf("%s/%s?%s=%d&%s=%s", l.baseURL, key, ParamExpires, exp, ParamSignature, sig), nil
}

func (l *LocalStorage) List(ctx context.Context, prefix string) ([]string, error) {
	root, err := filepath.Abs(l.rootPath)
	if err != nil {
		return nil, err
	}
	var out []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasSuffix(p, ".meta.json") || strings.HasSuffix(p, ".tmp") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(rel)
		if strings.HasPrefix(key, prefix) {
			out = append(out, key)
		}
		return nil
	})
	if err != nil {
		return nil, &StorageError{Op: "list", Key: prefix, Err: err}
	}
	return out, nil
}

func (l *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	p, err := l.path(key)
	if err != nil {
		return false, nil
	}
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false, nil
	}
	return true, nil
}

func (l *LocalStorage) GetSize(ctx context.Context, key string) (int64, error) {
	p, err := l.path(key)
	if err != nil {
		return 0, &StorageError{Op: "stat", Key: key, Err: err}
	}
	fi, err := os.Stat(p)
	if err != nil {
		return 0, &StorageError{Op: "stat", Key: key, Err: err}
	}
	return fi.Size(), nil
}

func (l *LocalStorage) GetMetadata(ctx context.Context, key string) (map[string]string, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, &StorageError{Op: "stat", Key: key, Err: err}
	}
	b, err := os.ReadFile(p + ".meta.json")
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, &StorageError{Op: "stat", Key: key, Err: err}
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, &StorageError{Op: "stat", Key: key, Err: err}
	}
	return m, nil
}

// Handler serves signed objects. In production this role belongs to the CDN;
// serving it here means the dev environment enforces identical signature and
// expiry rules rather than handing out audio unguarded.
func (l *LocalStorage) Handler(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, prefix), "/")

		exp, sig, err := ParseSignedQuery(r.URL.Query())
		if err != nil {
			http.Error(w, "forbidden: "+err.Error(), http.StatusForbidden)
			return
		}
		if err := Verify(l.secret, key, sig, exp, l.clock()); err != nil {
			// An expired link is a normal, recoverable client condition: the
			// app should refetch the session rather than treat it as fatal.
			status := http.StatusForbidden
			if strings.Contains(err.Error(), "expired") {
				status = http.StatusGone
			}
			http.Error(w, "forbidden: "+err.Error(), status)
			return
		}
		p, err := l.path(key)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		f, err := os.Open(p)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil || fi.IsDir() {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if ct := mime.TypeByExtension(filepath.Ext(key)); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		// Private only: a signed URL is a bearer credential for one listener,
		// so shared caches must never retain it.
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// ServeContent gives range requests, which the audio element needs for
		// seeking and for resuming mid-track.
		http.ServeContent(w, r, filepath.Base(p), fi.ModTime(), f)
	})
}
