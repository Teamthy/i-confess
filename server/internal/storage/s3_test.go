package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
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
		"a/v1.flac": "audio/flac", "a/v1.ogg": "audio/ogg", "a/v1.unknowncustombin": "application/octet-stream",
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

// ---------------------------------------------------------------------------
// R2-shaped endpoint, over real HTTP (VE-007)
// ---------------------------------------------------------------------------

// The tests above inject a stub S3API, so they assert what the provider asked a
// client to do - never what goes on the wire. Cloudflare R2, MinIO and Ceph are
// reached through the same branch: an explicit endpoint, path-style addressing
// and SigV4. This test builds the real client against a local origin that speaks
// S3 and checks the requests, which is the part that no stub can cover.
//
// What this does NOT establish: that Cloudflare R2 itself behaves the same. That
// needs an account, a bucket and credentials, and until somebody runs it the
// claim "R2 integration is tested" would be false. What is established is that
// the code path R2 uses is exercised end to end rather than assumed.
//
// Nor does it pin UsePathStyle: the SDK falls back to path-style for an endpoint
// it cannot virtual-host, so this request would look identical either way.
// TestExplicitEndpointRequestsPathStyle covers the option itself.
// The option that the wire test cannot see, asserted directly.
func TestExplicitEndpointRequestsPathStyle(t *testing.T) {
	opts := s3ClientOptions(&StorageConfig{S3Endpoint: "https://acct.r2.cloudflarestorage.com"})
	if len(opts) == 0 {
		t.Fatal("an explicit endpoint produced no client options")
	}
	co := &s3.Options{}
	for _, o := range opts {
		o(co)
	}
	if !co.UsePathStyle {
		t.Error("R2/MinIO style endpoint did not request path-style addressing")
	}
	if got := aws.ToString(co.BaseEndpoint); got != "https://acct.r2.cloudflarestorage.com" {
		t.Errorf("BaseEndpoint = %q", got)
	}
	// No endpoint (real AWS) must stay untouched: forcing path style there
	// changes the URL shape of every upload in production.
	if opts := s3ClientOptions(&StorageConfig{S3Bucket: "b", S3Region: "eu-west-1"}); len(opts) != 0 {
		t.Errorf("AWS endpoint override invented options: %d", len(opts))
	}
}

func TestR2StyleEndpointIsExercisedOverHTTP(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []*http.Request
		bodies   = map[string]string{}
	)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) > 0 {
			bodies[r.URL.Path] = string(body)
		}
		mu.Lock()
		requests = append(requests, r.Clone(context.Background()))
		mu.Unlock()

		switch {
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "missing"):
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>not found</Message></Error>`))
		case strings.Contains(r.URL.Path, "broken"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		default:
			w.Header().Set("Content-Type", "audio/mp4")
			_, _ = w.Write([]byte("audio-bytes"))
		}
	}))
	defer origin.Close()

	store, err := NewS3Storage(&StorageConfig{
		Provider: "s3", S3Bucket: "iconfess-media", S3Region: "auto",
		S3AccessKey: "r2-access-key-id", S3SecretKey: "r2-secret",
		// Exactly how an R2 deployment is configured: an account endpoint, no
		// CloudFront, region "auto".
		S3Endpoint: origin.URL, CDNDomain: "", SigningSecret: "",
	})
	if err != nil {
		t.Fatalf("build R2-shaped storage: %v", err)
	}
	if _, ok := store.(*S3Storage); !ok {
		t.Fatalf("expected the S3 path for an explicit endpoint, got %T", store)
	}
	ctx := context.Background()
	key := "audio/conf-1/var-1/voice-1/en/v1.m4a"

	if err := store.Upload(ctx, key, []byte("payload-bytes"), nil); err != nil {
		t.Fatalf("upload: %v", err)
	}
	got, err := store.Download(ctx, key)
	if err != nil || string(got) != "audio-bytes" {
		t.Fatalf("download: %q %v", got, err)
	}

	mu.Lock()
	seen := append([]*http.Request(nil), requests...)
	mu.Unlock()

	var put *http.Request
	for _, r := range seen {
		if r.Method == http.MethodPut {
			put = r
		}
	}
	if put == nil {
		t.Fatal("no PUT reached the origin")
	}
	// Path-style addressing is what an S3-compatible endpoint requires: the
	// bucket is the first path segment, not a subdomain.
	if want := "/iconfess-media/" + key; put.URL.Path != want {
		t.Errorf("PUT path = %q, want %q (path-style addressing)", put.URL.Path, want)
	}
	// SigV4 against the injected credentials, scoped to the R2 region.
	auth := put.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=r2-access-key-id/") {
		t.Errorf("PUT is not SigV4-signed with the configured key: %q", auth)
	}
	if !strings.Contains(auth, "/auto/s3/aws4_request") {
		t.Errorf("PUT credential is not scoped to the configured region: %q", auth)
	}
	if put.Header.Get("X-Amz-Content-Sha256") == "" {
		t.Error("PUT carries no payload hash")
	}
	if ct := put.Header.Get("Content-Type"); ct != "audio/mp4" {
		t.Errorf("PUT Content-Type = %q, want audio/mp4", ct)
	}
	if cc := put.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("PUT Cache-Control = %q", cc)
	}
	if bodies[put.URL.Path] != "payload-bytes" {
		t.Errorf("origin received %q", bodies[put.URL.Path])
	}

	// A presigned URL is the playback path when there is no CloudFront. It has
	// to point at the same origin, carry an expiry, and leak nothing else.
	signed, err := store.GenerateSignedURL(ctx, key, 15*time.Minute)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}
	u, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("presigned URL is not a URL: %v", err)
	}
	if u.Host != strings.TrimPrefix(origin.URL, "http://") {
		t.Errorf("presigned URL host = %q, want the configured endpoint", u.Host)
	}
	if u.Query().Get("X-Amz-Signature") == "" || u.Query().Get("X-Amz-Expires") == "" {
		t.Errorf("presigned URL is missing its signature or expiry: %s", signed)
	}
	if strings.Contains(signed, "r2-secret") {
		t.Error("presigned URL leaked the secret key")
	}
	// Fetching it must work against the origin, with the signature on the query.
	resp, err := http.Get(signed)
	if err != nil {
		t.Fatalf("fetch presigned URL: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("presigned fetch status = %d", resp.StatusCode)
	}

	// Error classification over a real socket: a missing key is an answer, a
	// 500 is a fault to retry.
	if _, err := store.Download(ctx, "audio/missing/v1.m4a"); err == nil {
		t.Error("missing object did not error")
	} else if IsRetryable(err) {
		t.Errorf("404 classified as retryable: %v", err)
	}
	if _, err := store.Download(ctx, "audio/broken/v1.m4a"); err == nil {
		t.Error("500 did not error")
	} else if !IsRetryable(err) {
		t.Errorf("500 not classified as retryable: %v", err)
	}
}
