package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Signed-URL scheme shared by every provider that does not delegate signing to
// the cloud vendor (§11).
//
// A signed URL carries an expiry and an HMAC over (key, expiry). The verifying
// edge — the CDN in production, our own handler in development — recomputes the
// MAC and refuses anything expired or tampered with. The signing secret never
// leaves the server, and audio bytes never pass through the Go API.

const (
	// ParamExpires is the unix-seconds expiry query parameter.
	ParamExpires = "exp"
	// ParamSignature is the base64url HMAC query parameter.
	ParamSignature = "sig"
)

// Sign computes the signature for a key/expiry pair.
//
// The newline separator is load-bearing: without it, ("audio/x", 12) and
// ("audio/x1", 2) would produce the same MAC input, letting an attacker who
// controls part of a key shift the boundary and reuse a signature.
func Sign(secret, key string, expires int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s\n%d", key, expires)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify checks a signature and expiry in constant time.
func Verify(secret, key, sig string, expires int64, now time.Time) error {
	if expires <= 0 {
		return errors.New("missing expiry")
	}
	if now.Unix() >= expires {
		return errors.New("link expired")
	}
	want := Sign(secret, key, expires)
	// hmac.Equal is constant time. A timing oracle here would let an attacker
	// forge a signature byte by byte.
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return errors.New("invalid signature")
	}
	return nil
}

// ParseSignedQuery extracts the expiry and signature from a query string.
func ParseSignedQuery(q url.Values) (expires int64, sig string, err error) {
	sig = q.Get(ParamSignature)
	if sig == "" {
		return 0, "", errors.New("missing signature")
	}
	expires, err = strconv.ParseInt(q.Get(ParamExpires), 10, 64)
	if err != nil {
		return 0, "", errors.New("invalid expiry")
	}
	return expires, sig, nil
}

// allowedPrefixes are the namespaces objects may live under. An allowlist
// rather than a denylist: a new caller must opt in deliberately instead of
// being able to write anywhere the path checks happen not to forbid.
var allowedPrefixes = []string{"audio/", "avatars/"}

// ValidKey rejects keys that could escape the permitted namespaces.
//
// Keys are interpolated into filesystem paths and URLs, so traversal sequences,
// absolute paths, NUL bytes and query separators are all refused before they
// reach a provider.
func ValidKey(key string) bool {
	if key == "" || len(key) > 512 {
		return false
	}
	if strings.Contains(key, "..") || strings.HasPrefix(key, "/") || strings.Contains(key, "//") {
		return false
	}
	if strings.ContainsAny(key, "\\?#\x00") {
		return false
	}
	for _, p := range allowedPrefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}
