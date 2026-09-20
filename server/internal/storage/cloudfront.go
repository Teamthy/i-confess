package storage

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha1" // CloudFront's canned-policy protocol requires RSA-SHA1.
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// cloudFrontSigner creates native CloudFront canned-policy URLs. S3 remains
// private behind Origin Access Control; unlike replacing the host on an S3
// presigned URL, this signature is actually understood and enforced by the CDN.
type cloudFrontSigner struct {
	baseURL string
	keyID   string
	key     *rsa.PrivateKey
}

func newCloudFrontSigner(baseURL, keyID, keyPath string) (*cloudFrontSigner, error) {
	if keyID == "" && keyPath == "" {
		return nil, nil
	}
	if baseURL == "" || keyID == "" || keyPath == "" {
		return nil, fmt.Errorf("MEDIA_BASE_URL, CLOUDFRONT_KEY_PAIR_ID and CLOUDFRONT_PRIVATE_KEY_PATH must be set together")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("MEDIA_BASE_URL must be an absolute https URL")
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read CloudFront private key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("decode CloudFront private key: invalid PEM")
	}
	var key *rsa.PrivateKey
	if parsedKey, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = parsedKey
	} else if generic, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, _ = generic.(*rsa.PrivateKey)
	}
	if key == nil {
		return nil, fmt.Errorf("CloudFront private key must be RSA PKCS#1 or PKCS#8")
	}
	return &cloudFrontSigner{baseURL: strings.TrimRight(baseURL, "/"), keyID: keyID, key: key}, nil
}

func (s *cloudFrontSigner) sign(key string, expires time.Time) (string, error) {
	resource := s.baseURL + "/" + strings.TrimLeft(key, "/")
	policy := fmt.Sprintf(`{"Statement":[{"Resource":%q,"Condition":{"DateLessThan":{"AWS:EpochTime":%d}}}]}`, resource, expires.Unix())
	digest := sha1.Sum([]byte(policy))
	sig, err := rsa.SignPKCS1v15(nil, s.key, crypto.SHA1, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign CloudFront URL: %w", err)
	}
	// CloudFront uses a URL-safe variant of base64 with these substitutions.
	encoded := base64.StdEncoding.EncodeToString(sig)
	encoded = strings.NewReplacer("+", "-", "=", "_", "/", "~").Replace(encoded)
	q := url.Values{}
	q.Set("Expires", fmt.Sprint(expires.Unix()))
	q.Set("Signature", encoded)
	q.Set("Key-Pair-Id", s.keyID)
	return resource + "?" + q.Encode(), nil
}
