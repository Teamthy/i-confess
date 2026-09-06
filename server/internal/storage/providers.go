package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3API is the subset of the AWS S3 client this provider calls. It exists so
// the provider can be tested against a fake instead of a live bucket: there is
// no other way to prove upload, signing and not-found handling actually work.
type S3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// S3Presigner is the subset of s3.PresignClient this provider calls.
type S3Presigner interface {
	PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

// S3Storage implements ObjectStorage for Amazon S3 using the AWS SDK v2.
type S3Storage struct {
	client    S3API
	presigner S3Presigner
	bucket    string
	region    string
}

// NewS3Storage creates a working S3 storage provider.
//
// Credentials resolve in this order: explicit S3AccessKey/S3SecretKey when
// both are set, otherwise the SDK default chain (env, shared config, then
// instance/task role). The default chain is what production should use, since
// an instance role needs no secret on disk.
func NewS3Storage(cfg *StorageConfig) (ObjectStorage, error) {
	if cfg.S3Bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET not configured")
	}
	if cfg.S3Region == "" {
		return nil, fmt.Errorf("S3_REGION not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.S3Region),
	}
	if cfg.S3AccessKey != "" && cfg.S3SecretKey != "" {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			awscreds.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, "")))
	}

	awscfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	var clientOpts []func(*s3.Options)
	if cfg.S3Endpoint != "" {
		// An explicit endpoint means an S3-compatible service (MinIO, R2,
		// Ceph), which generally requires path-style addressing.
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.S3Endpoint)
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awscfg, clientOpts...)
	return &S3Storage{
		client:    client,
		presigner: s3.NewPresignClient(client),
		bucket:    cfg.S3Bucket,
		region:    cfg.S3Region,
	}, nil
}

// newS3WithClients builds a provider around injected clients. Tests use this.
func newS3WithClients(client S3API, presigner S3Presigner, bucket string) *S3Storage {
	return &S3Storage{client: client, presigner: presigner, bucket: bucket, region: "test"}
}

func (s *S3Storage) Upload(ctx context.Context, key string, data []byte, metadata map[string]string) error {
	if !ValidKey(key) {
		return &StorageError{Op: "upload", Key: key, Err: fmt.Errorf("invalid storage key")}
	}
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentTypeForKey(key)),
		// Audio objects are immutable: the key encodes content, version and
		// voice, so a key never changes meaning once written. That is what
		// makes aggressive caching safe.
		CacheControl: aws.String("public, max-age=31536000, immutable"),
	}
	if len(metadata) > 0 {
		input.Metadata = metadata
	}
	if _, err := s.client.PutObject(ctx, input); err != nil {
		return s3Error("upload", key, err)
	}
	return nil
}

func (s *S3Storage) Download(ctx context.Context, key string) (data []byte, err error) {
	out, gerr := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if gerr != nil {
		return nil, s3Error("download", key, gerr)
	}
	// A close failure on a streaming body can mean the transfer was cut short.
	// Reporting success here would hand back a partial file, which the audio
	// inspector could still parse - as shorter audio than what was stored.
	defer func() {
		if cerr := out.Body.Close(); cerr != nil && err == nil {
			err = &StorageError{Op: "download", Key: key, Err: cerr, Retryable: true}
		}
	}()

	data, rerr := io.ReadAll(out.Body)
	if rerr != nil {
		return nil, &StorageError{Op: "download", Key: key, Err: rerr, Retryable: true}
	}
	return data, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	// S3 DELETE is idempotent: deleting an absent key succeeds. Callers may
	// therefore retry a cleanup job without first checking existence.
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return s3Error("delete", key, err)
	}
	return nil
}

func (s *S3Storage) GenerateSignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if !ValidKey(key) {
		return "", &StorageError{Op: "sign_url", Key: key, Err: fmt.Errorf("invalid storage key")}
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	res, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(o *s3.PresignOptions) {
		o.Expires = ttl
	})
	if err != nil {
		return "", s3Error("sign_url", key, err)
	}
	return res.URL, nil
}

func (s *S3Storage) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	var token *string

	for {
		out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, s3Error("list", prefix, err)
		}
		for _, o := range out.Contents {
			if o.Key != nil {
				keys = append(keys, *o.Key)
			}
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			return keys, nil
		}
		token = out.NextContinuationToken
	}
}

func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// A missing object is a legitimate answer to "does this exist", not a
		// failure. Any other error must surface or callers will conclude the
		// object is absent and regenerate audio that is already stored.
		if isNotFoundErr(err) {
			return false, nil
		}
		return false, s3Error("exists", key, err)
	}
	return true, nil
}

func (s *S3Storage) GetSize(ctx context.Context, key string) (int64, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, s3Error("get_size", key, err)
	}
	if out.ContentLength == nil {
		return 0, nil
	}
	return *out.ContentLength, nil
}

func (s *S3Storage) GetMetadata(ctx context.Context, key string) (map[string]string, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, s3Error("get_metadata", key, err)
	}
	if out.Metadata == nil {
		return map[string]string{}, nil
	}
	return out.Metadata, nil
}

// s3Error wraps an AWS failure with a retry decision.
func s3Error(op, key string, err error) error {
	return &StorageError{Op: op, Key: key, Err: err, Retryable: retryableAWSError(err)}
}

// retryableAWSError is true for faults that may succeed on a later attempt:
// throttling, server faults, and connection problems. A 403 or 400 will fail
// identically every time, and retrying it only burns money and latency.
func retryableAWSError(err error) bool {
	var respErr interface{ HTTPStatusCode() int }
	if errors.As(err, &respErr) {
		code := respErr.HTTPStatusCode()
		return code == http.StatusTooManyRequests || code >= 500
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func isNotFoundErr(err error) bool {
	var nf *s3types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	var nsk *s3types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var respErr interface{ HTTPStatusCode() int }
	return errors.As(err, &respErr) && respErr.HTTPStatusCode() == http.StatusNotFound
}

// contentTypeForKey maps the audio extensions this system stores. mime's table
// does not know m4a, and an empty Content-Type would make browsers download
// rather than play.
func contentTypeForKey(key string) string {
	switch strings.ToLower(filepath.Ext(key)) {
	case ".m4a":
		return "audio/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".flac":
		return "audio/flac"
	case ".ogg":
		return "audio/ogg"
	case ".weba", ".webm":
		return "audio/webm"
	}
	if ct := mime.TypeByExtension(filepath.Ext(key)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// ===== Providers that are not implemented =====
//
// GCS and Azure are named in StorageConfig and in audio_assets.storage_provider,
// but neither has an implementation. The previous versions of these types had
// a full set of methods that each returned "not yet implemented". That is worse
// than having no code at all: a stub satisfies the interface, so the provider
// constructs successfully and the failure surfaces at the first upload in
// production instead of at boot.
//
// These constructors fail immediately and say what to use instead. New() also
// refuses them, so a misconfiguration is caught before the process serves
// traffic.

// ErrProviderUnavailable is returned by providers named in configuration but
// not implemented. It is a configuration error, not a runtime fault.
var ErrProviderUnavailable = errors.New("storage provider is not implemented")

// NewGCSStorage always fails. Google Cloud Storage is not implemented.
func NewGCSStorage(cfg *StorageConfig) (ObjectStorage, error) {
	return nil, fmt.Errorf("%w: \"gcs\" — use \"s3\" or \"local\"", ErrProviderUnavailable)
}

// NewAzureStorage always fails. Azure Blob Storage is not implemented.
func NewAzureStorage(cfg *StorageConfig) (ObjectStorage, error) {
	return nil, fmt.Errorf("%w: \"azure\" — use \"s3\" or \"local\"", ErrProviderUnavailable)
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
