package storage

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha1" // CloudFront's canned-policy protocol requires RSA-SHA1.
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aws/smithy-go/encoding/httpbinding"
)

// cloudFrontSigner creates native CloudFront canned-policy URLs. S3 remains
// private behind Origin Access Control; unlike replacing the host on an S3
// presigned URL, this signature is actually understood and enforced by the CDN.
type cloudFrontSigner struct {
	origin url.URL
	keyID  string
	key    *rsa.PrivateKey
}

const dnsLabel = `[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?`

var distributionHost = regexp.MustCompile(`^(?:` + dnsLabel + `\.)+` + dnsLabel + `$`)

// CloudFrontOrigin parses the bare HTTPS distribution or alternate DNS name.
// The viewer path must start with the object key; a URL prefix could silently
// route governed audio through a different cache behavior. Never echo a bad
// URL (it may contain credentials) in configuration errors.
func CloudFrontOrigin(raw string) (url.URL, error) {
	host, ok := strings.CutPrefix(strings.TrimSuffix(raw, "/"), "https://")
	host = strings.TrimSuffix(host, ":443") // the same HTTPS origin
	if !ok || len(host) > 253 || !distributionHost.MatchString(host) {
		return url.URL{}, errors.New("CloudFront MEDIA_BASE_URL must be a bare HTTPS distribution origin (no nonstandard port, path, credentials, query or fragment)")
	}
	return url.URL{Scheme: "https", Host: strings.ToLower(host)}, nil
}

func newCloudFrontSigner(baseURL, keyID, keyPath string) (*cloudFrontSigner, error) {
	if keyID == "" && keyPath == "" {
		return nil, nil
	}
	if baseURL == "" || keyID == "" || keyPath == "" {
		return nil, fmt.Errorf("MEDIA_BASE_URL, CLOUDFRONT_KEY_PAIR_ID and CLOUDFRONT_PRIVATE_KEY_PATH must be set together")
	}
	origin, err := CloudFrontOrigin(baseURL)
	if err != nil {
		return nil, err
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
	return &cloudFrontSigner{origin: origin, keyID: keyID, key: key}, nil
}

func (s *cloudFrontSigner) sign(key string, expires time.Time) (string, error) {
	if !ValidKey(key) {
		return "", errors.New("invalid media key")
	}
	// Use the S3 SDK's path encoding. Keep separators, but distinguish a
	// literal plus (%2B) from a space (%20) and escape non-ASCII key bytes.
	// Sign exactly this resource; do not escape it again when adding the query.
	u := s.origin
	u.Path = "/" + key
	u.RawPath = httpbinding.EscapePath(u.Path, false)
	resource := u.String()
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
	u.RawQuery = q.Encode()
	return u.String(), nil
}
