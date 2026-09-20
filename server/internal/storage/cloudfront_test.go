package storage

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestCloudFrontSignedURLUsesNativeCannedPolicy(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "key.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	signer, err := newCloudFrontSigner("https://media.example.com", "K123", path)
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	signed, err := signer.sign("audio/session/item.m4a", expires)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(signed)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "media.example.com" || u.Path != "/audio/session/item.m4a" {
		t.Fatalf("unexpected resource URL: %s", signed)
	}
	if q := u.Query(); q.Get("Key-Pair-Id") != "K123" || q.Get("Expires") != strconv.FormatInt(expires.Unix(), 10) || q.Get("Signature") == "" {
		t.Fatalf("missing CloudFront signature fields: %s", signed)
	}
	if u.Query().Get("X-Amz-Signature") != "" {
		t.Fatal("returned an S3 signature instead of a CloudFront signature")
	}
}

func TestCloudFrontConfigurationFailsClosed(t *testing.T) {
	if _, err := newCloudFrontSigner("https://media.example.com", "K123", ""); err == nil {
		t.Fatal("partial signing configuration was accepted")
	}
}
