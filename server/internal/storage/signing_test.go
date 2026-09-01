package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const secret = "test-signing-secret"

func TestSignVerifyRoundTrip(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	exp := now.Add(10 * time.Minute).Unix()
	sig := Sign(secret, "audio/a/b/c/en/v1.mp3", exp)

	if err := Verify(secret, "audio/a/b/c/en/v1.mp3", sig, exp, now); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	exp := now.Add(10 * time.Minute).Unix()
	key := "audio/a/b/c/en/v1.mp3"
	sig := Sign(secret, key, exp)

	cases := []struct {
		name          string
		key, sig      string
		exp           int64
		signingSecret string
	}{
		{"swapped key", "audio/other/b/c/en/v1.mp3", sig, exp, secret},
		{"extended expiry", key, sig, exp + 3600, secret},
		{"garbage signature", key, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", exp, secret},
		{"wrong secret", key, Sign("other-secret", key, exp), exp, secret},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Verify(tc.signingSecret, tc.key, tc.sig, tc.exp, now); err == nil {
				t.Fatal("tampered link accepted")
			}
		})
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	exp := now.Add(-time.Second).Unix()
	sig := Sign(secret, "audio/a/b/c/en/v1.mp3", exp)

	err := Verify(secret, "audio/a/b/c/en/v1.mp3", sig, exp, now)
	if err == nil {
		t.Fatal("expired link accepted")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("error = %v, want an expiry error", err)
	}
}

// The signature must not be reusable for a different key even when an attacker
// controls where the key/expiry boundary falls.
func TestSignatureIsNotAmbiguousAcrossKeyBoundary(t *testing.T) {
	a := Sign(secret, "audio/x", 12)
	b := Sign(secret, "audio/x\n1", 2)
	if a == b {
		t.Fatal("key/expiry concatenation is ambiguous")
	}
}

func TestAudioKeyIsDeterministic(t *testing.T) {
	k1 := AudioKeyFor("CONF-1", "VAR-1", "VOICE-1", "en", 1)
	k2 := AudioKeyFor("conf-1", "var-1", "voice-1", "en", 1)
	if k1 != k2 {
		t.Fatalf("keys differ by case: %q vs %q", k1, k2)
	}
	if !strings.HasPrefix(k1, "audio/") || !strings.HasSuffix(k1, "v1.mp3") {
		t.Fatalf("unexpected key shape: %q", k1)
	}
	// Different identity or version must produce a different key, otherwise
	// regeneration would silently overwrite a live asset.
	if AudioKeyFor("conf-1", "var-1", "voice-1", "en", 2) == k1 {
		t.Fatal("version not reflected in key")
	}
	if AudioKeyFor("conf-1", "var-1", "voice-2", "en", 1) == k1 {
		t.Fatal("voice not reflected in key")
	}
	if AudioKeyFor("conf-1", "var-1", "voice-1", "de", 1) == k1 {
		t.Fatal("language not reflected in key")
	}
}

func TestAudioKeySanitizesTraversal(t *testing.T) {
	k := AudioKeyFor("../../etc", "passwd", "v", "en", 1)
	if strings.Contains(k, "..") {
		t.Fatalf("traversal survived sanitisation: %q", k)
	}
	if !ValidKey(k) {
		t.Fatalf("sanitised key should be valid: %q", k)
	}
}

func TestValidKey(t *testing.T) {
	good := []string{"audio/a/b/c/en/v1.mp3"}
	bad := []string{
		"", "etc/passwd", "audio/../../etc/passwd", "/audio/a.mp3",
		"audio//a.mp3", "audio/a?b.mp3", "audio/a\x00.mp3", "audio/a\\b.mp3",
		strings.Repeat("audio/x", 200),
	}
	for _, k := range good {
		if !ValidKey(k) {
			t.Fatalf("valid key rejected: %q", k)
		}
	}
	for _, k := range bad {
		if ValidKey(k) {
			t.Fatalf("invalid key accepted: %q", k)
		}
	}
}

func newLocal(t *testing.T, now time.Time) *LocalStorage {
	t.Helper()
	s, err := NewLocalStorage(&StorageConfig{
		LocalRootPath: t.TempDir(), CDNDomain: "/media", SigningSecret: secret,
	})
	if err != nil {
		t.Fatal(err)
	}
	l := s.(*LocalStorage)
	l.SetClock(func() time.Time { return now })
	return l
}

func TestLocalPutExistsAndChecksum(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := newLocal(t, now)
	key := AudioKeyFor("c1", "v1", "voice", "en", 1)

	if _, ok := mustExists(t, l, key); ok {
		t.Fatal("key exists before write")
	}
	obj, err := putObj(t, l, key, []byte("hello audio"))
	if err != nil {
		t.Fatal(err)
	}
	if obj.SizeBytes != 11 {
		t.Fatalf("size = %d, want 11", obj.SizeBytes)
	}
	if len(obj.Checksum) != 64 {
		t.Fatalf("checksum is not a hex sha256: %q", obj.Checksum)
	}
	if _, ok := mustExists(t, l, key); !ok {
		t.Fatal("key missing after write")
	}
	// Same bytes must yield the same checksum, which is what dedupe relies on.
	obj2, _ := putObj(t, l, key, []byte("hello audio"))
	if obj2.Checksum != obj.Checksum {
		t.Fatal("checksum is not stable for identical content")
	}
}

func TestLocalPutRejectsEscapingKey(t *testing.T) {
	l := newLocal(t, time.Now())
	if err := l.Upload(context.Background(), "audio/../../escape.mp3", []byte("x"), nil); err == nil {
		t.Fatal("escaping key accepted")
	}
	if err := l.Upload(context.Background(), "notaudio/x.mp3", []byte("x"), nil); err == nil {
		t.Fatal("key outside the audio namespace accepted")
	}
}

// The dev origin must enforce the same rules as the CDN: unsigned, tampered and
// expired requests are all refused.
func TestLocalHandlerEnforcesSignature(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := newLocal(t, now)
	key := AudioKeyFor("c1", "v1", "voice", "en", 1)
	if _, err := putObj(t, l, key, []byte("audio-bytes")); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.StripPrefix("", l.Handler("/media")))
	defer srv.Close()

	get := func(path string) int {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		return res.StatusCode
	}

	// Unsigned.
	if code := get("/media/" + key); code != http.StatusForbidden {
		t.Fatalf("unsigned request: got %d, want 403", code)
	}

	// Properly signed.
	signed, err := mustSign(t, l, key, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if code := get(signed); code != http.StatusOK {
		t.Fatalf("signed request: got %d, want 200", code)
	}

	// Tampered key, valid-looking signature.
	u, _ := url.Parse(signed)
	q := u.Query()
	if code := get("/media/" + AudioKeyFor("other", "v1", "voice", "en", 1) + "?" + q.Encode()); code != http.StatusForbidden {
		t.Fatalf("tampered key: got %d, want 403", code)
	}

	// Expired: the clock moves past the signature's life.
	l.SetClock(func() time.Time { return now.Add(11 * time.Minute) })
	if code := get(signed); code != http.StatusGone {
		t.Fatalf("expired link: got %d, want 410", code)
	}
}

func TestSignedURLDefaultsTTL(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := newLocal(t, now)
	u, err := mustSign(t, l, AudioKeyFor("c", "v", "voice", "en", 1), 0)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(u)
	exp, _, err := ParseSignedQuery(parsed.Query())
	if err != nil {
		t.Fatal(err)
	}
	if exp <= now.Unix() {
		t.Fatal("default TTL produced an already-expired link")
	}
}

// ---- helpers bridging the tests to the ObjectStorage interface ----

func mustSign(t *testing.T, l *LocalStorage, key string, ttl time.Duration) (string, error) {
	t.Helper()
	return l.GenerateSignedURL(context.Background(), key, ttl)
}

type putResult struct {
	SizeBytes int64
	Checksum  string
}

func putObj(t *testing.T, l *LocalStorage, key string, data []byte) (putResult, error) {
	t.Helper()
	if err := l.Upload(context.Background(), key, data, nil); err != nil {
		return putResult{}, err
	}
	sum := sha256.Sum256(data)
	return putResult{SizeBytes: int64(len(data)), Checksum: hex.EncodeToString(sum[:])}, nil
}

func mustExists(t *testing.T, l *LocalStorage, key string) (struct{}, bool) {
	t.Helper()
	ok, _ := l.Exists(context.Background(), key)
	return struct{}{}, ok
}
