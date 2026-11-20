package storage

// Shared by the offline wire tests and the separately tagged staging test.
// Nothing in this check is linked into the API or accepts a playback key/URL
// from an operator. All errors returned to the operator must be safe to log:
// SDK and http.Client errors can include a complete bearer URL.

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/media"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/logging"
)

const (
	r2CheckBucket = "icf-media-check-staging"
	r2CheckPrefix = "audio/voices/_delivery-check/" // fixed operator-approved namespace
	r2RunTag      = "icf-smoke-run"
	r2LegacyCC    = "public, max-age=3600, immutable"
	r2PrivateCC   = "private, no-store"
	r2ProbeTTL    = 20 * time.Second
	r2Range       = "bytes=44-59" // first 16 PCM samples, unique to this WAV
)

var r2AccountID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func r2StagingConfig(getenv func(string) string) (*StorageConfig, error) {
	if getenv("ICF_R2_CHECK_CONFIRM_BUCKET") != r2CheckBucket {
		return nil, errors.New("explicit approval for the dedicated icf-media-check-staging bucket is required")
	}
	account := getenv("ICF_R2_CHECK_ACCOUNT_ID")
	if !r2AccountID.MatchString(account) {
		return nil, errors.New("ICF_R2_CHECK_ACCOUNT_ID must be the staging account ID, not an endpoint")
	}
	key, secret := getenv("ICF_R2_CHECK_ACCESS_KEY_ID"), getenv("ICF_R2_CHECK_SECRET_ACCESS_KEY")
	if strings.TrimSpace(key) == "" || strings.TrimSpace(secret) == "" {
		return nil, errors.New("both dedicated ICF_R2_CHECK credential variables are required")
	}
	return &StorageConfig{Provider: "s3", S3Bucket: r2CheckBucket, S3Region: "auto",
		S3Endpoint: "https://" + account + ".r2.cloudflarestorage.com", S3AccessKey: key, S3SecretKey: secret}, nil
}

// Use the same S3Storage implementation, SDK presigner and path-style options
// as playback. Pin credentials and transport; do not consult AWS profiles,
// instance metadata, alternate endpoints or a browser cookie jar.
func r2CheckStore(cfg *StorageConfig, httpClient *http.Client) *S3Storage {
	client := s3.NewFromConfig(aws.Config{Region: cfg.S3Region, HTTPClient: httpClient, RetryMaxAttempts: 1, Logger: logging.Nop{},
		Credentials: credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, ""),
	}, s3ClientOptions(cfg)...)
	return newS3WithClients(client, s3.NewPresignClient(client), cfg.S3Bucket)
}

func r2CheckClient(transport http.RoundTripper) *http.Client {
	if transport == nil {
		transport = &http.Transport{DisableCompression: true} // direct TLS; no proxy from the environment
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func r2HTTPStatus(err error) int {
	var response interface{ HTTPStatusCode() int }
	if errors.As(err, &response) {
		return response.HTTPStatusCode()
	}
	return 0
}

// Only the fixed step name and a numeric status are safe to report.
func r2StepError(step string, err error) error {
	if code := r2HTTPStatus(err); code != 0 {
		return fmt.Errorf("%s: HTTP %d", step, code)
	}
	return fmt.Errorf("%s: request did not complete", step)
}

func r2FixtureURL(ctx context.Context, s *S3Storage, origin, key string, ttl time.Duration) (*url.URL, time.Time, error) {
	raw, err := s.GenerateSignedURL(ctx, key, ttl)
	if err != nil {
		return nil, time.Time{}, r2StepError("sign fixture GET", err)
	}
	return checkedR2URL(raw, origin, "/"+s.bucket+"/"+key, ttl)
}

func checkedR2URL(raw, origin, path string, ttl time.Duration) (*url.URL, time.Time, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Scheme != "https" || u.Scheme+"://"+u.Host != origin ||
		u.EscapedPath() != path || u.Fragment != "" {
		return nil, time.Time{}, errors.New("presigner did not return the exact fixture HTTPS origin and path; not fetching it")
	}
	q := u.Query()
	signedAt, err := time.Parse("20060102T150405Z", q.Get("X-Amz-Date"))
	if err != nil || q.Get("X-Amz-Expires") != strconv.Itoa(int(ttl/time.Second)) ||
		q.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" || len(q.Get("X-Amz-Signature")) != 64 ||
		q.Get("response-cache-control") != r2PrivateCC {
		return nil, time.Time{}, errors.New("unexpected private SigV4 playback parameters")
	}
	return u, signedAt.Add(ttl), nil
}

func privateNoStore(h http.Header) bool {
	directives := map[string]bool{}
	for _, v := range strings.Split(strings.Join(h.Values("Cache-Control"), ","), ",") {
		directives[strings.ToLower(strings.TrimSpace(v))] = true
	}
	return len(directives) == 2 && directives["private"] && directives["no-store"]
}

// No redirects, cookies, AWS Authorization header, cache busters, unbounded
// reads, response dumps, or status-only positive controls.
func r2ViewerGET(ctx context.Context, client *http.Client, u url.URL, wav []byte, partial, denied bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return errors.New("cannot form fixture request")
	}
	if partial {
		req.Header.Set("Range", r2Range)
	}
	req.Header.Set("Accept-Encoding", "identity")
	res, err := client.Do(req)
	if err != nil {
		return errors.New("viewer request did not complete")
	}
	defer res.Body.Close()
	const maxBody = 32 << 10 // the fixture is 8044 bytes; also bound any error page
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return errors.New("incomplete or oversized fixture response")
	}
	slice := wav[44:60]
	if denied {
		if res.StatusCode != http.StatusForbidden && res.StatusCode != http.StatusUnauthorized {
			return fmt.Errorf("expected authorization denial, got HTTP %d", res.StatusCode)
		}
		if bytes.Contains(body, slice) || strings.HasPrefix(strings.ToLower(res.Header.Get("Content-Type")), "audio/") {
			return errors.New("denial carried audio")
		}
		return nil
	}
	wantCode, wantBytes := http.StatusOK, wav
	if partial {
		wantCode, wantBytes = http.StatusPartialContent, slice
	}
	if res.StatusCode != wantCode {
		return fmt.Errorf("expected HTTP %d, got HTTP %d", wantCode, res.StatusCode)
	}
	if partial && res.Header.Get("Content-Range") != fmt.Sprintf("bytes 44-59/%d", len(wav)) {
		return errors.New("incorrect Content-Range")
	}
	if !bytes.Equal(body, wantBytes) {
		return errors.New("fixture byte mismatch")
	}
	ctype, _, _ := strings.Cut(res.Header.Get("Content-Type"), ";")
	if !strings.EqualFold(strings.TrimSpace(ctype), "audio/wav") {
		return errors.New("fixture did not have its audio/wav content type")
	}
	if !privateNoStore(res.Header) {
		return errors.New("playback Cache-Control is not private, no-store")
	}
	return nil
}

func checkR2Head(ctx context.Context, s *S3Storage, key, id, cache string, size int) error {
	head, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return r2StepError("inspect fixture metadata", err)
	}
	if head.Metadata[r2RunTag] != id || aws.ToString(head.CacheControl) != cache || aws.ToInt64(head.ContentLength) != int64(size) ||
		aws.ToString(head.ContentType) != "audio/wav" {
		return errors.New("unexpected fixture owner, size, type or cache metadata")
	}
	return nil
}

func cleanupR2Fixture(s *S3Storage, key, id string) error {
	// The upload may have succeeded even if its acknowledgement was lost.
	// Reconcile only this run's marker/key; a cancelled probe still cleans up.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	head, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if isNotFoundErr(err) {
		return nil
	}
	if err != nil {
		return r2StepError("cleanup HEAD", err)
	}
	if head.Metadata[r2RunTag] != id {
		return errors.New("cleanup refused: fixture ownership changed; inspect the recorded fixture ID")
	}
	if err := s.Delete(ctx, key); err != nil {
		return r2StepError("cleanup DELETE", err)
	}
	if exists, err := s.Exists(ctx, key); err != nil || exists {
		return errors.New("cleanup could not confirm the current fixture key is absent")
	}
	return nil
}

// One generated key, no listing. Start with a legacy-header object so a signed
// response cannot pass just by echoing a new upload's private metadata. The
// final normal Upload tests the application's private write path on that key.
func checkPrivateR2Delivery(ctx context.Context, s *S3Storage, origin string, client *http.Client,
	waitUntil func(context.Context, time.Time) error, logf func(string, ...any)) (result error) {
	id := rand.Text()
	key := r2CheckPrefix + id + ".wav"
	wav := media.ToneBytes(1)
	// Independent of the key: an R2 denial may legitimately echo the key in XML.
	if _, err := rand.Read(wav[44:60]); err != nil {
		return errors.New("could not generate the synthetic PCM marker")
	}
	meta := map[string]string{r2RunTag: id}
	logf("synthetic fixture ID: %s", id)
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: &s.bucket, Key: &key, Body: bytes.NewReader(wav),
		IfNoneMatch: aws.String("*"), ContentType: aws.String("audio/wav"), CacheControl: aws.String(r2LegacyCC), Metadata: meta})
	if code := r2HTTPStatus(err); code == http.StatusPreconditionFailed || code == http.StatusConflict {
		return errors.New("fixture create conflicted; no overwrite or deletion attempted")
	}
	defer func() { result = errors.Join(result, cleanupR2Fixture(s, key, id)) }()
	if err != nil {
		return r2StepError("create synthetic legacy fixture", err)
	}
	if err := checkR2Head(ctx, s, key, id, r2LegacyCC, len(wav)); err != nil {
		return err
	}
	u, expires, err := r2FixtureURL(ctx, s, origin, key, r2ProbeTTL)
	if err != nil {
		return err
	}
	for _, partial := range []bool{false, true} {
		if err := r2ViewerGET(ctx, client, *u, wav, partial, false); err != nil {
			return fmt.Errorf("valid legacy fixture GET (range=%t): %w", partial, err)
		}
	}
	logf("signed GET 200 and range 206 matched the fixture and overrode its legacy metadata")
	unsigned, altered := *u, *u
	unsigned.RawQuery = ""
	sig := u.Query().Get("X-Amz-Signature")
	replacement := "0"
	if sig[0] == '0' {
		replacement = "1"
	}
	// Change only the signature, not the encoding of any signed parameter.
	altered.RawQuery = strings.Replace(altered.RawQuery, "X-Amz-Signature="+sig, "X-Amz-Signature="+replacement+sig[1:], 1)
	for _, check := range []struct {
		name string
		url  url.URL
	}{{"unsigned", unsigned}, {"altered signature", altered}} {
		if err := r2ViewerGET(ctx, client, check.url, wav, false, true); err != nil {
			return fmt.Errorf("%s fixture GET: %w", check.name, err)
		}
	}
	// Bracket the denials with a still-working, unchanged link to this object.
	if err := r2ViewerGET(ctx, client, *u, wav, false, false); err != nil {
		return fmt.Errorf("original signed GET after denial checks: %w", err)
	}
	// A refusal after the URL was already stale cannot establish the signature
	// checks above. Also never sleep an unbounded time on a mis-set clock.
	if remaining := time.Until(expires); remaining <= 0 || remaining > r2ProbeTTL+time.Second {
		return errors.New("fixture expiry outside the expected window; check clocks and rerun")
	}
	if err := waitUntil(ctx, expires.Add(2*time.Second)); err != nil {
		return errors.New("cancelled before expiry check")
	}
	for _, partial := range []bool{false, true} {
		if err := r2ViewerGET(ctx, client, *u, wav, partial, true); err != nil {
			return fmt.Errorf("expired fixture GET (range=%t): %w", partial, err)
		}
	}
	// Distinguish expiry from an object that disappeared or service that now
	// refuses every request, before changing the object in any way.
	fresh, _, err := r2FixtureURL(ctx, s, origin, key, time.Minute)
	if err != nil {
		return err
	}
	if err := r2ViewerGET(ctx, client, *fresh, wav, false, false); err != nil {
		return fmt.Errorf("fresh signed GET after expiry: %w", err)
	}
	logf("unsigned, altered and expired URLs denied; fresh link read the same bytes")
	if err := checkR2Head(ctx, s, key, id, r2LegacyCC, len(wav)); err != nil {
		return err
	}
	wav[44] ^= 0xff // a stale read of the legacy fixture must not pass
	if err := s.Upload(ctx, key, wav, meta); err != nil {
		return r2StepError("upload through application storage", err)
	}
	if err := checkR2Head(ctx, s, key, id, r2PrivateCC, len(wav)); err != nil {
		return err
	}
	if err := r2ViewerGET(ctx, client, *fresh, wav, false, false); err != nil {
		return fmt.Errorf("read back application upload: %w", err)
	}
	return nil
}
