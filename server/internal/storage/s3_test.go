package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// fakeAPI records what the provider asked S3 to do, so these tests assert on
// the requests actually sent rather than on the provider's own bookkeeping.
type fakeAPI struct {
	put       *s3.PutObjectInput
	got       *s3.GetObjectOutput
	getErr    error
	headErr   error
	headLen   *int64
	headMeta  map[string]string
	deleted   string
	listPages []*s3.ListObjectsV2Output
	putErr    error
}

func (f *fakeAPI) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.put = in
	if f.putErr != nil {
		return nil, f.putErr
	}
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeAPI) GetObject(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.got != nil {
		return f.got, nil
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("audio"))}, nil
}

func (f *fakeAPI) DeleteObject(_ context.Context, in *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	f.deleted = aws.ToString(in.Key)
	return &s3.DeleteObjectOutput{}, nil
}

func (f *fakeAPI) HeadObject(_ context.Context, _ *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if f.headErr != nil {
		return nil, f.headErr
	}
	return &s3.HeadObjectOutput{ContentLength: f.headLen, Metadata: f.headMeta}, nil
}

func (f *fakeAPI) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if len(f.listPages) == 0 {
		return &s3.ListObjectsV2Output{}, nil
	}
	out := f.listPages[0]
	f.listPages = f.listPages[1:]
	return out, nil
}

type fakePresigner struct {
	got     *s3.GetObjectInput
	expires time.Duration
}

func (f *fakePresigner) PresignGetObject(_ context.Context, in *s3.GetObjectInput,
	optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
	f.got = in
	opts := s3.PresignOptions{}
	for _, fn := range optFns {
		fn(&opts)
	}
	f.expires = opts.Expires
	return &v4.PresignedHTTPRequest{URL: "https://bucket.s3.amazonaws.com/key?sig=abc"}, nil
}

// httpError stands in for an AWS response fault, which the SDK surfaces with a
// status code rather than a sentinel error.
type httpError struct{ code int }

func (e *httpError) Error() string       { return "http error" }
func (e *httpError) HTTPStatusCode() int { return e.code }

const testKey = "audio/conf-1/var-1/voice-1/en/v1.m4a"

func TestUploadSetsImmutableCachingAndAudioContentType(t *testing.T) {
	api := &fakeAPI{}
	s := newS3WithClients(api, &fakePresigner{}, "bucket")

	meta := map[string]string{"confession_id": "conf-1", "duration_seconds": "30"}
	if err := s.Upload(context.Background(), testKey, []byte("data"), meta); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if api.put == nil {
		t.Fatal("PutObject was not called")
	}
	if got := aws.ToString(api.put.ContentType); got != "audio/mp4" {
		t.Errorf("ContentType = %q, want audio/mp4 (mime's table does not know .m4a)", got)
	}
	if cc := aws.ToString(api.put.CacheControl); !strings.Contains(cc, "immutable") {
		t.Errorf("CacheControl = %q, want an immutable policy: keys encode content, version and voice", cc)
	}
	if api.put.Metadata["duration_seconds"] != "30" {
		t.Errorf("metadata not forwarded: %v", api.put.Metadata)
	}
	if aws.ToString(api.put.Bucket) != "bucket" {
		t.Errorf("Bucket = %q, want bucket", aws.ToString(api.put.Bucket))
	}
}

func TestUploadRefusesAKeyOutsideTheAllowedScheme(t *testing.T) {
	api := &fakeAPI{}
	s := newS3WithClients(api, &fakePresigner{}, "bucket")

	if err := s.Upload(context.Background(), "../../etc/passwd", []byte("x"), nil); err == nil {
		t.Fatal("Upload accepted a traversal key")
	}
	if api.put != nil {
		t.Error("PutObject must not be called for an invalid key")
	}
}

func TestSignedURLCarriesTheRequestedExpiry(t *testing.T) {
	pre := &fakePresigner{}
	s := newS3WithClients(&fakeAPI{}, pre, "bucket")

	url, err := s.GenerateSignedURL(context.Background(), testKey, 4*time.Hour)
	if err != nil {
		t.Fatalf("GenerateSignedURL: %v", err)
	}
	if url == "" {
		t.Fatal("empty URL")
	}
	if pre.expires != 4*time.Hour {
		t.Errorf("expiry = %v, want 4h", pre.expires)
	}
	if aws.ToString(pre.got.Key) != testKey {
		t.Errorf("presigned key = %q, want %q", aws.ToString(pre.got.Key), testKey)
	}

	// A zero TTL must not produce an already-expired URL.
	if _, err := s.GenerateSignedURL(context.Background(), testKey, 0); err != nil {
		t.Fatalf("zero TTL: %v", err)
	}
	if pre.expires <= 0 {
		t.Errorf("zero TTL produced expiry %v; must fall back to a positive default", pre.expires)
	}
}

func TestExistsTreatsNotFoundAsAnAnswerNotAFailure(t *testing.T) {
	// A missing object is the normal answer to "is this already generated".
	// Treating it as an error would make every dedupe check fail.
	api := &fakeAPI{headErr: &s3types.NotFound{}}
	s := newS3WithClients(api, &fakePresigner{}, "bucket")

	ok, err := s.Exists(context.Background(), testKey)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if ok {
		t.Error("Exists = true for a NotFound response")
	}
}

func TestExistsPropagatesRealFailures(t *testing.T) {
	// The dangerous direction: reporting "absent" for a bucket that is merely
	// unreachable makes the caller regenerate audio that is already stored,
	// paying the provider twice.
	api := &fakeAPI{headErr: &httpError{code: 403}}
	s := newS3WithClients(api, &fakePresigner{}, "bucket")

	if _, err := s.Exists(context.Background(), testKey); err == nil {
		t.Fatal("Exists swallowed a 403")
	}
}

func TestRetryClassification(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"503 is transient", &httpError{code: 503}, true},
		{"429 is throttling", &httpError{code: 429}, true},
		{"403 will not change", &httpError{code: 403}, false},
		{"404 will not change", &httpError{code: 404}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{putErr: tc.err}
			s := newS3WithClients(api, &fakePresigner{}, "bucket")

			err := s.Upload(context.Background(), testKey, []byte("x"), nil)
			var se *StorageError
			if !errors.As(err, &se) {
				t.Fatalf("err = %v, want *StorageError", err)
			}
			if se.Retryable != tc.retryable {
				t.Errorf("Retryable = %v, want %v", se.Retryable, tc.retryable)
			}
			if se.Op != "upload" || se.Key != testKey {
				t.Errorf("error lost context: op=%q key=%q", se.Op, se.Key)
			}
		})
	}
}

func TestListFollowsPagination(t *testing.T) {
	tr := true
	api := &fakeAPI{listPages: []*s3.ListObjectsV2Output{
		{Contents: []s3types.Object{{Key: aws.String("a.m4a")}}, IsTruncated: &tr,
			NextContinuationToken: aws.String("next")},
		{Contents: []s3types.Object{{Key: aws.String("b.m4a")}}},
	}}
	s := newS3WithClients(api, &fakePresigner{}, "bucket")

	keys, err := s.List(context.Background(), "audio/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys, want 2 from both pages: %v", len(keys), keys)
	}
}

func TestDeleteIsIdempotent(t *testing.T) {
	api := &fakeAPI{}
	s := newS3WithClients(api, &fakePresigner{}, "bucket")

	// S3 DELETE succeeds for absent keys, so a cleanup job can retry without
	// first checking existence.
	for i := 0; i < 2; i++ {
		if err := s.Delete(context.Background(), testKey); err != nil {
			t.Fatalf("Delete attempt %d: %v", i+1, err)
		}
	}
	if api.deleted != testKey {
		t.Errorf("deleted key = %q, want %q", api.deleted, testKey)
	}
}

func TestGetMetadataNeverReturnsNil(t *testing.T) {
	api := &fakeAPI{headMeta: nil}
	s := newS3WithClients(api, &fakePresigner{}, "bucket")

	meta, err := s.GetMetadata(context.Background(), testKey)
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}
	if meta == nil {
		t.Fatal("nil map: callers ranging over this would be fine, but indexing a write would panic")
	}
}

func TestContentTypeMapping(t *testing.T) {
	cases := map[string]string{
		"a/v1.m4a": "audio/mp4", "a/v1.mp3": "audio/mpeg", "a/v1.wav": "audio/wav",
		"a/v1.flac": "audio/flac", "a/v1.ogg": "audio/ogg", "a/v1.xyz": "application/octet-stream",
	}
	for key, want := range cases {
		if got := contentTypeForKey(key); got != want {
			t.Errorf("contentTypeForKey(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestNewRefusesUnimplementedProviders(t *testing.T) {
	for _, provider := range []string{"gcs", "azure", "wasabi", ""} {
		_, err := New(&StorageConfig{Provider: provider, LocalRootPath: t.TempDir()})
		if !errors.Is(err, ErrProviderUnavailable) && !strings.Contains(err.Error(), "unsupported") {
			t.Errorf("New(%q) = %v, want a refusal naming the available providers", provider, err)
		}
	}
}

func TestValidateProviderRefusesLocalStorageInProduction(t *testing.T) {
	// The failure this prevents is silent: local storage boots healthy and
	// serves audio from local disk indefinitely, but the bytes die with the
	// container and are invisible to every other replica.
	err := ValidateProvider(&StorageConfig{Provider: "local", LocalRootPath: "/data"}, true)
	if err == nil {
		t.Fatal("production accepted STORAGE_PROVIDER=local")
	}
	if !strings.Contains(err.Error(), "§6") {
		t.Errorf("error should cite the rule it enforces, got: %v", err)
	}
}

func TestValidateProviderAcceptsTheSupportedCombinations(t *testing.T) {
	if err := ValidateProvider(&StorageConfig{Provider: "local", LocalRootPath: "/data"}, false); err != nil {
		t.Errorf("development + local = %v, want nil", err)
	}
	if err := ValidateProvider(&StorageConfig{
		Provider: "s3", S3Bucket: "b", S3Region: "r",
	}, true); err != nil {
		t.Errorf("production + s3 = %v, want nil", err)
	}
}

func TestValidateProviderRejectsHalfConfiguredS3(t *testing.T) {
	if err := ValidateProvider(&StorageConfig{Provider: "s3", S3Region: "r"}, true); err == nil {
		t.Error("s3 without S3_BUCKET was accepted")
	}
	if err := ValidateProvider(&StorageConfig{Provider: "s3", S3Bucket: "b"}, true); err == nil {
		t.Error("s3 without S3_REGION was accepted")
	}
}

func TestValidateProviderIsCaseInsensitive(t *testing.T) {
	// A safety check keyed on an exact string is a safety check a capital
	// letter defeats.
	if err := ValidateProvider(&StorageConfig{Provider: "LOCAL", LocalRootPath: "/d"}, true); err == nil {
		t.Error("STORAGE_PROVIDER=LOCAL bypassed the production guard")
	}
}
