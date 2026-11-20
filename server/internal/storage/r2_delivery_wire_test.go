package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestR2StagingRequiresDedicatedConfig(t *testing.T) {
	vars := map[string]string{
		"ICF_R2_CHECK_CONFIRM_BUCKET": r2CheckBucket, "ICF_R2_CHECK_ACCOUNT_ID": strings.Repeat("a", 32),
		"ICF_R2_CHECK_ACCESS_KEY_ID": "fixture-access", "ICF_R2_CHECK_SECRET_ACCESS_KEY": "fixture-secret",
		"S3_ENDPOINT": "http://wrong.example", "S3_BUCKET": "production", "S3_SECRET_KEY": "ambient-secret",
	}
	get := func(k string) string { return vars[k] }
	cfg, err := r2StagingConfig(get)
	if err != nil || cfg.S3Bucket != r2CheckBucket || cfg.S3Endpoint != "https://"+strings.Repeat("a", 32)+".r2.cloudflarestorage.com" ||
		cfg.S3Region != "auto" || cfg.S3SecretKey != "fixture-secret" {
		t.Fatal("incorrect private staging config")
	}
	for _, change := range []struct{ key, value string }{
		{"ICF_R2_CHECK_CONFIRM_BUCKET", "production"}, {"ICF_R2_CHECK_ACCOUNT_ID", "a.example"},
		{"ICF_R2_CHECK_ACCOUNT_ID", "https://private-fixture-secret@example.test"},
		{"ICF_R2_CHECK_ACCESS_KEY_ID", ""}, {"ICF_R2_CHECK_SECRET_ACCESS_KEY", " "},
	} {
		orig := vars[change.key]
		vars[change.key] = change.value
		cfg, err := r2StagingConfig(get)
		if cfg != nil || err == nil || strings.Contains(err.Error(), "fixture-secret") {
			t.Errorf("%s did not fail closed and redact its value", change.key)
		}
		vars[change.key] = orig
	}
}

type r2ObservedSigner struct {
	S3Presigner
	record func(string)
}

func (p r2ObservedSigner) PresignGetObject(ctx context.Context, in *s3.GetObjectInput, opts ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
	r, err := p.S3Presigner.PresignGetObject(ctx, in, opts...)
	if err == nil {
		p.record(r.URL)
	}
	return r, err
}

func TestR2CheckPinsTheViewerDestination(t *testing.T) {
	const origin, path = "https://fixture.example.test", "/icf-media-check-staging/audio/voices/_delivery-check/id.wav"
	q := url.Values{"X-Amz-Date": {"20260103T140500Z"}, "X-Amz-Expires": {"20"}, "X-Amz-Algorithm": {"AWS4-HMAC-SHA256"},
		"X-Amz-Signature": {strings.Repeat("a", 64)}, "response-cache-control": {r2PrivateCC}, "X-Amz-Credential": {"sensitive-fixture-value"}}
	raw := origin + path + "?" + q.Encode()
	if _, _, err := checkedR2URL(raw, origin, path, r2ProbeTTL); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(raw, "https:", "http:", 1), strings.Replace(raw, origin, origin+".other.test", 1),
		strings.Replace(raw, origin, "https://user@fixture.example.test", 1), strings.Replace(raw, path, path+"2", 1),
		strings.Replace(raw, "Expires=20", "Expires=3600", 1), strings.Replace(raw, "response-cache-control", "absent", 1), raw + "#fragment"} {
		if u, _, err := checkedR2URL(bad, origin, path, r2ProbeTTL); u != nil || err == nil || strings.Contains(err.Error(), "sensitive-fixture-value") {
			t.Error("did not safely reject a misrouted or incorrectly scoped presign")
		}
	}
}

// The TLS fixture recognizes exact URLs minted by the real SDK. This is not
// an R2 signature verifier. Advancing its clock exercises the operator check's
// decisions, not a cloud service's expiry or authorization implementation.
type r2WireFixture struct {
	t                             *testing.T
	fault                         string
	mu                            sync.Mutex
	minted                        map[string]time.Time
	now                           time.Time
	path, tag, cache, contentType string
	data                          []byte
	steps                         []string
}

func (f *r2WireFixture) remember(raw string) {
	u, err := url.Parse(raw)
	if err != nil {
		f.t.Fatal("bad fixture URL")
	}
	date, err := time.Parse("20060102T150405Z", u.Query().Get("X-Amz-Date"))
	if err != nil {
		f.t.Fatal("bad fixture date")
	}
	ttl, err := strconv.Atoi(u.Query().Get("X-Amz-Expires"))
	if err != nil {
		f.t.Fatal("bad fixture TTL")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.minted[u.RequestURI()] = date.Add(time.Duration(ttl) * time.Second)
}

func (f *r2WireFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.path == "" {
		f.path = r.URL.Path
	}
	if r.URL.Path != f.path || !strings.HasPrefix(f.path, "/"+r2CheckBucket+"/"+r2CheckPrefix) {
		f.t.Error("operation outside the single generated fixture key")
		w.WriteHeader(400)
		return
	}
	if r.Method != http.MethodGet && !strings.Contains(r.Header.Get("Authorization"), "/auto/s3/aws4_request") {
		f.t.Error("storage operation missing region-scoped SigV4 authorization")
	}
	switch r.Method {
	case http.MethodGet:
		f.get(w, r)
	case http.MethodPut:
		f.put(w, r)
	case http.MethodHead:
		if f.data == nil {
			f.steps = append(f.steps, "absent")
			w.WriteHeader(404)
			return
		}
		f.steps = append(f.steps, "head")
		w.Header().Set("Content-Length", strconv.Itoa(len(f.data)))
		w.Header().Set("Content-Type", f.contentType)
		w.Header().Set("Cache-Control", f.cache)
		w.Header().Set("X-Amz-Meta-"+r2RunTag, f.tag)
		if f.fault == "changed-owner" {
			w.Header().Set("X-Amz-Meta-"+r2RunTag, "other-run")
		}
	case http.MethodDelete:
		f.steps = append(f.steps, "delete")
		if f.fault == "delete-fails" {
			w.WriteHeader(403)
			return
		}
		if f.fault != "delete-ignored" {
			f.data = nil
		}
		w.WriteHeader(204)
	default:
		f.t.Error("unexpected fixture operation")
		w.WriteHeader(405)
	}
}

func (f *r2WireFixture) put(w http.ResponseWriter, r *http.Request) {
	creating := f.data == nil
	if creating {
		f.steps = append(f.steps, "create")
		if r.Header.Get("If-None-Match") != "*" {
			f.t.Error("unconditional initial PUT")
		}
		if f.fault == "conflict" {
			f.data = []byte("pre-existing")
			w.WriteHeader(412)
			return
		}
	} else {
		f.steps = append(f.steps, "upload")
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Error("bad fixture upload")
		w.WriteHeader(400)
		return
	}
	if creating || f.fault != "stale-upload" {
		f.data = data
	}
	f.tag, f.cache, f.contentType = r.Header.Get("X-Amz-Meta-"+r2RunTag), r.Header.Get("Cache-Control"), r.Header.Get("Content-Type")
	if f.path != "/"+r2CheckBucket+"/"+r2CheckPrefix+f.tag+".wav" || len(f.tag) < 16 {
		f.t.Error("missing run identity")
	}
	if !creating && f.fault == "new-cacheable" {
		f.cache = r2LegacyCC
	}
	if creating && f.fault == "lost-put-ack" {
		w.WriteHeader(500)
	}
}

func (f *r2WireFixture) get(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
		f.t.Error("viewer request sent credentials")
	}
	exp, known := f.minted[r.URL.RequestURI()]
	partial := r.Header.Get("Range") != ""
	step, code := "signed", 200
	switch {
	case r.URL.RawQuery == "":
		step, code = "unsigned", 403
	case !known:
		step, code = "altered", 403
	case !f.now.Before(exp):
		step, code = "expired", 403
		if partial {
			step = "expired-range"
		}
	case f.cache == r2PrivateCC:
		step = "updated"
	case len(f.minted) > 1:
		step = "fresh"
	case partial:
		step = "range"
	}
	f.steps = append(f.steps, step)
	switch {
	case step == "unsigned" && f.fault == "unsigned-redirect":
		code = 307
		w.Header().Set("Location", "/must-not-follow")
	case step == "unsigned" && f.fault == "unsigned-missing":
		code = 404
	case step == "unsigned" && f.fault == "unsigned-ok", step == "altered" && f.fault == "invalid-ok", step == "expired-range" && f.fault == "expired-range-ok":
		code = 200
	case step == "fresh" && f.fault == "outage-after-expiry":
		code = 403
	}
	if code != 200 {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(code)
		if f.fault == "denial-with-bytes" {
			_, _ = w.Write(f.data)
		} else {
			// A real denial can echo the key, not the private range marker.
			_, _ = fmt.Fprintf(w, "<Error><Code>AccessDenied</Code><Resource>%s</Resource></Error>", f.path)
		}
		return
	}
	w.Header().Set("Cache-Control", r.URL.Query().Get("response-cache-control"))
	w.Header().Set("Content-Type", f.contentType)
	if f.fault == "legacy-header" {
		w.Header().Set("Cache-Control", f.cache)
	}
	body := f.data
	if partial && len(body) >= 60 {
		if r.Header.Get("Range") != r2Range {
			f.t.Error("unexpected range")
		}
		code, body = 206, body[44:60]
		if f.fault != "range-header" {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 44-59/%d", len(f.data)))
		}
		if f.fault == "range-bytes" {
			body = []byte("wrong range data")
		}
	}
	w.WriteHeader(code)
	_, _ = w.Write(body)
}

func TestR2DeliveryCheckOverLocalTLS(t *testing.T) {
	for _, tc := range []struct {
		fault, wantError string
		remains          bool
	}{
		{"all-checks", "", false},
		{"conflict", "create conflicted", true}, {"lost-put-ack", "create synthetic legacy fixture", false},
		{"changed-owner", "cleanup refused", true}, {"legacy-header", "playback Cache-Control", false},
		{"range-header", "incorrect Content-Range", false}, {"range-bytes", "byte mismatch", false},
		{"unsigned-ok", "unsigned fixture GET", false}, {"invalid-ok", "altered signature fixture GET", false},
		{"unsigned-redirect", "HTTP 307", false}, {"unsigned-missing", "HTTP 404", false},
		{"denial-with-bytes", "denial carried audio", false}, {"expired-range-ok", "expired fixture GET (range=true)", false},
		{"outage-after-expiry", "fresh signed GET after expiry", false}, {"new-cacheable", "cache metadata", false},
		{"stale-upload", "read back application upload", false}, {"cancel", "cancelled before expiry", false},
		{"delete-fails", "cleanup DELETE", true}, {"delete-ignored", "confirm the current fixture key is absent", true},
	} {
		t.Run(tc.fault, func(t *testing.T) {
			f := &r2WireFixture{t: t, fault: tc.fault, minted: map[string]time.Time{}, now: time.Now()}
			origin := httptest.NewTLSServer(f)
			defer origin.Close()
			client := r2CheckClient(origin.Client().Transport)
			defer client.CloseIdleConnections()
			s := r2CheckStore(&StorageConfig{S3Endpoint: origin.URL, S3Bucket: r2CheckBucket, S3Region: "auto",
				S3AccessKey: "fixture-key", S3SecretKey: "fixture-secret"}, client)
			s.presigner = r2ObservedSigner{s.presigner, f.remember}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			wait := func(ctx context.Context, until time.Time) error {
				if tc.fault == "cancel" {
					cancel()
					return ctx.Err()
				}
				f.mu.Lock()
				defer f.mu.Unlock()
				f.now = until
				return nil
			}
			var output bytes.Buffer
			err := checkPrivateR2Delivery(ctx, s, origin.URL, client, wait, func(format string, args ...any) { fmt.Fprintf(&output, format, args...) })
			f.mu.Lock()
			defer f.mu.Unlock()
			if tc.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
				want := []string{"create", "head", "signed", "range", "unsigned", "altered", "signed", "expired", "expired-range", "fresh", "head", "upload", "head", "updated", "head", "delete", "absent"}
				if !slices.Equal(f.steps, want) {
					t.Fatalf("incomplete synthetic check: %v", f.steps)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("got %v, want %s", err, tc.wantError)
			}
			if (f.data != nil) != tc.remains {
				t.Error("unexpected fixture cleanup result")
			}
			if (tc.fault == "conflict" || tc.fault == "changed-owner") && slices.Contains(f.steps, "delete") {
				t.Error("deleted an unowned object")
			}
			for _, secret := range []string{"https://", "X-Amz-Signature", "fixture-key", "fixture-secret"} {
				if strings.Contains(output.String()+fmt.Sprint(err), secret) {
					t.Error("leaked request details")
				}
			}
		})
	}
}
