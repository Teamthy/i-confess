// Package oauth validates third-party identity assertions (§36, §37, §38, §64).
//
// The governing rule: the client never tells us who it is. It hands us a token
// from a provider, and this package verifies that token's signature against the
// provider's published keys before any identity claim is believed. A handler
// that trusted a client-supplied email or subject would let anyone sign in as
// anyone.
package oauth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Provider names.
const (
	ProviderGoogle = "google"
	ProviderApple  = "apple"
)

// Identity is a verified assertion about a person from a provider.
type Identity struct {
	Provider string
	// Subject is the provider's stable identifier. This, not the email, is the
	// join key: emails change hands and Apple's private relay addresses are
	// per-app, so keying on email would eventually merge two people.
	Subject string
	Email   string
	// EmailVerified reports whether the provider vouches for the address. An
	// unverified address must never be used to claim an existing account.
	EmailVerified bool
	Name          string
}

// Errors callers distinguish.
var (
	ErrInvalidToken   = errors.New("invalid identity token")
	ErrProviderUnavai = errors.New("identity provider unavailable")
)

// Verifier validates a provider's identity token.
type Verifier interface {
	Name() string
	// Verify checks the token's signature, issuer, audience and expiry, and
	// returns the asserted identity.
	Verify(ctx context.Context, idToken string) (*Identity, error)
}

// ---------------------------------------------------------------------------
// JWKS-based verification
// ---------------------------------------------------------------------------

// jwk is one key from a provider's JWKS document.
type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

// keyCache caches provider signing keys.
//
// Providers rotate keys, so the cache has a TTL and refetches on an unknown
// kid. Without caching, every social sign-in would make an extra network call
// to the provider on the critical path.
type keyCache struct {
	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
	ttl       time.Duration
}

func newKeyCache() *keyCache {
	return &keyCache{keys: map[string]*rsa.PublicKey{}, ttl: time.Hour}
}

func (c *keyCache) get(kid string) (*rsa.PublicKey, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if time.Since(c.fetchedAt) > c.ttl {
		return nil, false
	}
	k, ok := c.keys[kid]
	return k, ok
}

func (c *keyCache) put(keys map[string]*rsa.PublicKey) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys = keys
	c.fetchedAt = time.Now()
}

// OIDCVerifier verifies RS256 identity tokens against a JWKS endpoint.
//
// Google and Apple both issue standard OIDC tokens, so one implementation
// serves both with different issuer/audience configuration.
type OIDCVerifier struct {
	provider string
	// issuers are the acceptable `iss` values. Google uses two spellings.
	issuers []string
	// audience is our client id. Checking it is what stops a token minted for
	// a different app from being replayed against ours (§64).
	audience string
	jwksURL  string
	client   *http.Client
	cache    *keyCache
	// now is injectable for tests.
	now func() time.Time
}

// NewGoogle builds a verifier for Google Sign-In.
func NewGoogle(clientID string) *OIDCVerifier {
	return &OIDCVerifier{
		provider: ProviderGoogle,
		issuers:  []string{"https://accounts.google.com", "accounts.google.com"},
		audience: clientID,
		jwksURL:  "https://www.googleapis.com/oauth2/v3/certs",
		client:   &http.Client{Timeout: 10 * time.Second},
		cache:    newKeyCache(),
		now:      time.Now,
	}
}

// NewApple builds a verifier for Sign in with Apple.
func NewApple(clientID string) *OIDCVerifier {
	return &OIDCVerifier{
		provider: ProviderApple,
		issuers:  []string{"https://appleid.apple.com"},
		audience: clientID,
		jwksURL:  "https://appleid.apple.com/auth/keys",
		client:   &http.Client{Timeout: 10 * time.Second},
		cache:    newKeyCache(),
		now:      time.Now,
	}
}

// SetJWKSURL overrides the key endpoint. Test-only.
func (v *OIDCVerifier) SetJWKSURL(u string) { v.jwksURL = u }

// SetClock overrides the clock. Test-only.
func (v *OIDCVerifier) SetClock(f func() time.Time) { v.now = f }

// Name implements Verifier.
func (v *OIDCVerifier) Name() string { return v.provider }

// claims are the OIDC fields we consume.
type claims struct {
	Iss           string `json:"iss"`
	Sub           string `json:"sub"`
	Aud           any    `json:"aud"`
	Exp           int64  `json:"exp"`
	Iat           int64  `json:"iat"`
	Nonce         string `json:"nonce"`
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
}

// Verify implements Verifier.
func (v *OIDCVerifier) Verify(ctx context.Context, idToken string) (*Identity, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: not a JWT", ErrInvalidToken)
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: bad header encoding", ErrInvalidToken)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, fmt.Errorf("%w: bad header", ErrInvalidToken)
	}
	// Only RS256. Accepting the algorithm the token names would allow the
	// classic "alg: none" and HMAC-confusion attacks.
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("%w: unsupported algorithm %q", ErrInvalidToken, header.Alg)
	}

	key, err := v.keyFor(ctx, header.Kid)
	if err != nil {
		return nil, err
	}
	if err := verifyRS256(key, parts[0]+"."+parts[1], parts[2]); err != nil {
		return nil, fmt.Errorf("%w: signature check failed", ErrInvalidToken)
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: bad payload encoding", ErrInvalidToken)
	}
	var c claims
	if err := json.Unmarshal(payloadJSON, &c); err != nil {
		return nil, fmt.Errorf("%w: bad payload", ErrInvalidToken)
	}

	if !contains(v.issuers, c.Iss) {
		return nil, fmt.Errorf("%w: unexpected issuer %q", ErrInvalidToken, c.Iss)
	}
	// Audience binding: without it, a token issued to any other application
	// using the same provider could be replayed against this one.
	if !audienceMatches(c.Aud, v.audience) {
		return nil, fmt.Errorf("%w: token was not issued for this application", ErrInvalidToken)
	}
	now := v.now().Unix()
	if c.Exp == 0 || now >= c.Exp {
		return nil, fmt.Errorf("%w: token expired", ErrInvalidToken)
	}
	// Reject tokens claiming to be issued in the future beyond small clock
	// skew, which indicates a forged or replayed assertion.
	if c.Iat != 0 && c.Iat > now+300 {
		return nil, fmt.Errorf("%w: token issued in the future", ErrInvalidToken)
	}
	if c.Sub == "" {
		return nil, fmt.Errorf("%w: token has no subject", ErrInvalidToken)
	}

	name := c.Name
	if name == "" {
		name = c.GivenName
	}

	return &Identity{
		Provider:      v.provider,
		Subject:       c.Sub,
		Email:         strings.ToLower(strings.TrimSpace(c.Email)),
		EmailVerified: truthy(c.EmailVerified),
		Name:          name,
	}, nil
}

func (v *OIDCVerifier) keyFor(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if k, ok := v.cache.get(kid); ok {
		return k, nil
	}
	// Unknown kid or stale cache: refetch, because providers rotate keys and a
	// stale cache would reject every new token.
	keys, err := v.fetchKeys(ctx)
	if err != nil {
		return nil, err
	}
	v.cache.put(keys)
	k, ok := keys[kid]
	if !ok {
		return nil, fmt.Errorf("%w: unknown signing key", ErrInvalidToken)
	}
	return k, nil
}

func (v *OIDCVerifier) fetchKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderUnavai, err)
	}
	res, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderUnavai, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: jwks returned %d", ErrProviderUnavai, res.StatusCode)
	}

	var doc jwks
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: bad jwks document", ErrProviderUnavai)
	}
	out := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := rsaKeyFromJWK(k)
		if err != nil {
			continue
		}
		out[k.Kid] = pub
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: jwks contained no usable keys", ErrProviderUnavai)
	}
	return out, nil
}

func rsaKeyFromJWK(k jwk) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, err
	}
	// The exponent is big-endian and usually three bytes; left-pad to a uint32.
	if len(eb) > 4 {
		return nil, errors.New("exponent too large")
	}
	padded := make([]byte, 4)
	copy(padded[4-len(eb):], eb)
	e := binary.BigEndian.Uint32(padded)
	if e == 0 {
		return nil, errors.New("zero exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(e)}, nil
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// audienceMatches handles `aud` being either a string or an array.
func audienceMatches(aud any, want string) bool {
	if want == "" {
		// An unconfigured audience must fail closed: accepting any audience
		// would let tokens from other applications through.
		return false
	}
	switch a := aud.(type) {
	case string:
		return a == want
	case []any:
		for _, v := range a {
			if s, ok := v.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

// truthy handles providers sending email_verified as a bool or a string.
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true"
	}
	return false
}
