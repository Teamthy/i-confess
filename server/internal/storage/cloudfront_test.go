package storage

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func cloudFrontTestKey(t *testing.T) (*rsa.PublicKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "key.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return &key.PublicKey, path
}

func TestCloudFrontSignedURLUsesNativeCannedPolicy(t *testing.T) {
	public, path := cloudFrontTestKey(t)
	signer, err := newCloudFrontSigner("https://media.example.com", "K123", path)
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	const object = "audio/voices/v/reflection/First take/àṣẹ+1.m4a"
	signed, err := signer.sign(object, expires)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(signed)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "media.example.com" || u.Path != "/"+object {
		t.Fatalf("unexpected resource URL: %s", signed)
	}
	q := u.Query()
	if len(q) != 3 || q.Get("Key-Pair-Id") != "K123" || q.Get("Expires") != strconv.FormatInt(expires.Unix(), 10) || q.Get("Signature") == "" {
		t.Fatal("expected only the three native CloudFront canned-policy fields")
	}
	// Verify the signature with the public half of the fixture key and the
	// resource the viewer actually requests, not merely a nonempty query field.
	const wantResource = "https://media.example.com/audio/voices/v/reflection/First%20take/%C3%A0%E1%B9%A3%E1%BA%B9%2B1.m4a"
	resource, _, _ := strings.Cut(signed, "?")
	if resource != wantResource {
		t.Fatalf("resource = %q, want canonical playback path", resource)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.NewReplacer("-", "+", "_", "=", "~", "/").Replace(u.Query().Get("Signature")))
	if err != nil {
		t.Fatal(err)
	}
	verify := func(resource string, exp int64) error {
		policy := fmt.Sprintf(`{"Statement":[{"Resource":"%s","Condition":{"DateLessThan":{"AWS:EpochTime":%d}}}]}`, resource, exp)
		sum := sha1.Sum([]byte(policy))
		return rsa.VerifyPKCS1v15(public, crypto.SHA1, sum[:], sig)
	}
	if err := verify(wantResource, expires.Unix()); err != nil {
		t.Fatal("signature does not verify against the requested object and expiry")
	}
	if verify(wantResource+"x", expires.Unix()) == nil || verify(wantResource, expires.Unix()+1) == nil {
		t.Fatal("signature must bind both path and expiry")
	}
}

// Exercise production provider selection, not just the standalone signer. No
// S3 or CloudFront client requests are needed to mint native playback URLs.
func TestCloudFrontStorageSignsTheCanonicalViewerPath(t *testing.T) {
	_, pemPath := cloudFrontTestKey(t)
	s, err := New(&StorageConfig{
		Provider: "s3", S3Region: "us-east-1", S3Bucket: "fixture", S3AccessKey: "test", S3SecretKey: "test",
		CDNDomain: "https://Media.example.test:443/", CloudFrontKeyPairID: "KTEST", CloudFrontPrivateKeyPath: pemPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"audio/voices/fixture/master.wav", "voice-private/fixture/source.wav", "bible/audio/fixture/item.m4a", "audio/content/fixture/item.m4a",
	} {
		raw, err := s.GenerateSignedURL(t.Context(), key, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host != "media.example.test" || u.EscapedPath() != "/"+key || u.Query().Get("Key-Pair-Id") != "KTEST" {
			t.Errorf("did not issue a native URL at the root viewer path for %s", key)
		}
	}
	for _, key := range []string{"audio/content/../voices/fixture/master.wav", "audio/%76oices/fixture/master.wav"} {
		if raw, err := s.GenerateSignedURL(t.Context(), key, time.Minute); err == nil || raw != "" {
			t.Fatal("issued a URL for a noncanonical key")
		}
	}
}

func TestCloudFrontConfigurationFailsClosed(t *testing.T) {
	if _, err := newCloudFrontSigner("https://media.example.com", "K123", ""); err == nil {
		t.Fatal("partial signing configuration was accepted")
	}
}

func TestCloudFrontSignerRejectsNonOriginBase(t *testing.T) {
	_, path := cloudFrontTestKey(t)
	for _, base := range []string{
		"http://media.example.test", "https://media.example.test:8443", "https://media.example.test/cacheable/",
		"https://media.example.test/?v=1", "https://media.example.test/#ignored", "https://user@media.example.test",
		"https://media.example.test/%2f", "https://*.example.test",
	} {
		if _, err := newCloudFrontSigner(base, "K123", path); err == nil {
			t.Errorf("accepted a non-origin signing base: %s", base)
		}
	}
}
