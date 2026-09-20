// Package pubsub decodes Pub/Sub push deliveries and verifies the identity of
// whoever sent them.
//
// Google Play does not call this server. It publishes to a Pub/Sub topic, and a
// push subscription delivers the message as an HTTP POST. That means the
// endpoint is reachable by anyone on the internet, and the body is an envelope
// this server did not write - so before a single byte of it is believed, the
// request has to be shown to have come from the push subscription.
//
// Google's mechanism is an OpenID Connect token in the Authorization header,
// signed by Google and addressed to the audience configured on the
// subscription. That is what this package verifies:
//
//   - the signature, against Google's published keys, fetched from the JWKS
//     endpoint and cached;
//   - the issuer, which must be Google's accounts service;
//   - the audience, which stops a token minted for any other Google service
//     (or any other project's push subscription) from being replayed here;
//   - the expiry, so a captured token is not a permanent credential;
//   - the service-account email, when one is configured, which is what
//     distinguishes a push subscription in this project from one an attacker
//     can create in theirs.
//
// The package is deliberately small and dependency-free: the alternative is
// google.golang.org/api/idtoken and its transitive tree, for one JWT check
// against a documented spec.
package pubsub

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Configuration.
const (
	// EnvPushAudience is the audience configured on the push subscription. The
	// OIDC token must be addressed to it. Without it nothing can be verified,
	// and the endpoint fails closed rather than accepting unverified pushes.
	EnvPushAudience = "PUBSUB_PUSH_AUDIENCE"
	// EnvPushServiceAccount is the email of the service account the push
	// subscription runs as. Optional, and recommended: it narrows acceptance
	// from "any Google-signed token for this audience" to "this project".
	EnvPushServiceAccount = "PUBSUB_PUSH_SERVICE_ACCOUNT"
	// EnvJWKSURL overrides Google's key set. Tests use it; there is no
	// production reason to change it.
	EnvJWKSURL = "PUBSUB_JWKS_URL"
)

// DefaultJWKSURL is Google's public key set for OIDC tokens.
const DefaultJWKSURL = "https://www.googleapis.com/oauth2/v3/certs"

// ErrUnconfigured means push authentication cannot be performed here.
var ErrUnconfigured = errors.New("pubsub: push authentication is not configured")

// ErrUnauthenticated means the request did not carry a token this server
// accepts. The causes are the usual ones and are listed in the error text: a
// missing header, a token from another audience, an expired token, an unknown
// key.
var ErrUnauthenticated = errors.New("pubsub: push request is not authenticated")

// PushEnvelope is the JSON body a Pub/Sub push subscription delivers.
type PushEnvelope struct {
	Message struct {
		Data        string            `json:"data"`
		MessageID   string            `json:"messageId"`
		PublishTime string            `json:"publishTime"`
		Attributes  map[string]string `json:"attributes"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// Decode parses a delivery body.
//
// messageId is required. It is the idempotency key for the notification: Pub/Sub
// redelivers until the endpoint acknowledges, so the same message arrives more
// than once as a matter of course, and without the id there is no way to tell a
// redelivery from a new event.
func Decode(body []byte) (PushEnvelope, error) {
	var env PushEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return PushEnvelope{}, fmt.Errorf("%w: delivery body is not JSON: %v", ErrUnauthenticated, err)
	}
	if strings.TrimSpace(env.Message.MessageID) == "" {
		return PushEnvelope{}, fmt.Errorf("%w: delivery has no messageId", ErrUnauthenticated)
	}
	if strings.TrimSpace(env.Message.Data) == "" {
		return PushEnvelope{}, fmt.Errorf("%w: delivery %s carries no data", ErrUnauthenticated, env.Message.MessageID)
	}
	return env, nil
}

// Payload decodes the base64 notification inside the envelope.
func (e PushEnvelope) Payload() ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(e.Message.Data)
	if err != nil {
		return nil, fmt.Errorf("pubsub: message %s payload is not base64: %w", e.Message.MessageID, err)
	}
	return raw, nil
}

// PublishTime parses the envelope's publish time, reporting whether it was
// usable. It is not used for ordering - the notification's own event time is -
// but it dates the audit record.
func (e PushEnvelope) PublishTime() (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(e.Message.PublishTime))
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// OIDCVerifier verifies the OpenID Connect token on a push request.
type OIDCVerifier struct {
	// Audience is the audience the token must be addressed to. Required.
	Audience string
	// ServiceAccountEmail, when set, is the only sender accepted.
	ServiceAccountEmail string
	// JWKSURL overrides Google's key set. Empty uses DefaultJWKSURL.
	JWKSURL string
	// HTTPClient is optional; a bounded default is used otherwise.
	HTTPClient *http.Client
	// Now is injectable for tests.
	Now func() time.Time

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
	maxAge  time.Duration
}

func (v *OIDCVerifier) now() time.Time {
	if v.Now != nil {
		return v.Now().UTC()
	}
	return time.Now().UTC()
}

func (v *OIDCVerifier) jwksURL() string {
	if strings.TrimSpace(v.JWKSURL) != "" {
		return v.JWKSURL
	}
	return DefaultJWKSURL
}

func (v *OIDCVerifier) httpClient() *http.Client {
	if v.HTTPClient != nil {
		return v.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// jwtHeader is the JOSE header.
type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

// jwtClaims is the subset of OIDC claims that matter here.
type jwtClaims struct {
	Iss           string          `json:"iss"`
	Aud           json.RawMessage `json:"aud"`
	Exp           *int64          `json:"exp"`
	Iat           *int64          `json:"iat"`
	Email         string          `json:"email"`
	EmailVerified *bool           `json:"email_verified"`
	Sub           string          `json:"sub"`
}

// audiences decodes aud, which the spec allows to be a string or a list.
func (c jwtClaims) audiences() []string {
	raw := strings.TrimSpace(string(c.Aud))
	if raw == "" || raw == "null" {
		return nil
	}
	var single string
	if err := json.Unmarshal(c.Aud, &single); err == nil {
		return []string{single}
	}
	var many []string
	if err := json.Unmarshal(c.Aud, &many); err == nil {
		return many
	}
	return nil
}

// Verify checks a bearer token and reports why it was refused.
func (v *OIDCVerifier) Verify(ctx context.Context, token string) error {
	if strings.TrimSpace(v.Audience) == "" {
		// Never "accept everything". An endpoint that authenticates nobody is
		// indistinguishable from one that authenticates everybody.
		return fmt.Errorf("%w: %s is not set", ErrUnconfigured, EnvPushAudience)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("%w: no bearer token", ErrUnauthenticated)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("%w: token is not a three-part JWT", ErrUnauthenticated)
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("%w: token header is not base64url", ErrUnauthenticated)
	}
	var header jwtHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return fmt.Errorf("%w: token header is not JSON", ErrUnauthenticated)
	}
	// Algorithm is compared exactly. Trusting the header's alg is how an RS256
	// verifier is turned into an HS256 one with the public key as the secret.
	if header.Alg != "RS256" {
		return fmt.Errorf("%w: token alg is %q, want RS256", ErrUnauthenticated, header.Alg)
	}

	key, err := v.key(ctx, header.Kid)
	if err != nil {
		return err
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("%w: token signature is not base64url", ErrUnauthenticated)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return fmt.Errorf("%w: token signature does not verify", ErrUnauthenticated)
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("%w: token claims are not base64url", ErrUnauthenticated)
	}
	var claims jwtClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return fmt.Errorf("%w: token claims are not JSON", ErrUnauthenticated)
	}

	// Issuer: Google's accounts service, spelled either way. Both spellings
	// appear in the wild and Google documents both as valid.
	switch claims.Iss {
	case "https://accounts.google.com", "accounts.google.com":
	default:
		return fmt.Errorf("%w: token issuer is %q", ErrUnauthenticated, claims.Iss)
	}

	audiences := claims.audiences()
	matched := false
	for _, aud := range audiences {
		if aud == v.Audience {
			matched = true
			break
		}
	}
	if !matched {
		// This is the check that makes the endpoint safe to expose: a token
		// minted for any other Google service is a valid Google token, and only
		// the audience distinguishes it from one issued for this subscription.
		return fmt.Errorf("%w: token audience %v does not include %q", ErrUnauthenticated, audiences, v.Audience)
	}

	if claims.Exp == nil {
		return fmt.Errorf("%w: token has no expiry", ErrUnauthenticated)
	}
	now := v.now()
	if !now.Before(time.Unix(*claims.Exp, 0).UTC()) {
		return fmt.Errorf("%w: token expired at %s", ErrUnauthenticated,
			time.Unix(*claims.Exp, 0).UTC().Format(time.RFC3339))
	}
	if claims.Iat != nil && time.Unix(*claims.Iat, 0).UTC().After(now.Add(clockSkew)) {
		return fmt.Errorf("%w: token was issued in the future", ErrUnauthenticated)
	}

	if want := strings.TrimSpace(v.ServiceAccountEmail); want != "" {
		if !strings.EqualFold(strings.TrimSpace(claims.Email), want) {
			return fmt.Errorf("%w: token is for %q, want %q", ErrUnauthenticated, claims.Email, want)
		}
		if claims.EmailVerified != nil && !*claims.EmailVerified {
			return fmt.Errorf("%w: token email is not verified", ErrUnauthenticated)
		}
	}
	return nil
}

// VerifyRequest pulls the bearer token out of a request and verifies it.
func (v *OIDCVerifier) VerifyRequest(ctx context.Context, r *http.Request) error {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return fmt.Errorf("%w: no Authorization header", ErrUnauthenticated)
	}
	const prefix = "bearer "
	if !strings.HasPrefix(strings.ToLower(header), prefix) {
		return fmt.Errorf("%w: Authorization scheme is not Bearer", ErrUnauthenticated)
	}
	return v.Verify(ctx, strings.TrimSpace(header[len(prefix):]))
}

// clockSkew is how far ahead of this server a token's timestamps may be. The
// two clocks are not the same clock.
const clockSkew = 2 * time.Minute

// keySet is Google's published key set.
type keySet struct {
	Keys []struct {
		Kty string `json:"kty"`
		Alg string `json:"alg"`
		Use string `json:"use"`
		Kid string `json:"kid"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

// key returns the public key with the given id, fetching the key set when it is
// not cached.
//
// An unknown kid forces a refresh before it is refused: Google rotates these
// keys, and a cache that only expired on a timer would refuse every push for
// however long it took to roll over.
func (v *OIDCVerifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if kid == "" {
		return nil, fmt.Errorf("%w: token names no key id", ErrUnauthenticated)
	}
	v.mu.Lock()
	key, ok := v.keys[kid]
	fresh := time.Since(v.fetched) < v.maxAge && v.maxAge > 0
	v.mu.Unlock()
	if ok && fresh {
		return key, nil
	}

	if err := v.refresh(ctx); err != nil {
		if ok {
			// The cached key still verifies; serving it beats failing a
			// legitimate push because Google's key endpoint blipped.
			return key, nil
		}
		return nil, err
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	key, ok = v.keys[kid]
	if !ok {
		return nil, fmt.Errorf("%w: token key id %q is not in Google's key set", ErrUnauthenticated, kid)
	}
	return key, nil
}

// refresh fetches and caches Google's key set.
func (v *OIDCVerifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL(), nil)
	if err != nil {
		return fmt.Errorf("%w: build key request: %v", ErrUnavailable, err)
	}
	res, err := v.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("%w: fetch Google's key set: %v", ErrUnavailable, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: Google's key set returned %d", ErrUnavailable, res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%w: read Google's key set: %v", ErrUnavailable, err)
	}
	var set keySet
	if err := json.Unmarshal(body, &set); err != nil {
		return fmt.Errorf("%w: Google's key set is not JSON: %v", ErrUnavailable, err)
	}

	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		parsed, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = parsed
	}
	if len(keys) == 0 {
		return fmt.Errorf("%w: Google's key set contains no usable keys", ErrUnavailable)
	}

	v.mu.Lock()
	v.keys = keys
	v.fetched = time.Now()
	v.maxAge = cacheAge(res.Header.Get("Cache-Control"))
	v.mu.Unlock()
	return nil
}

// ErrUnavailable means the key set could not be fetched. It is kept apart from
// ErrUnauthenticated so an operator can tell "we could not check" from "this
// request is not from Google" - the first is an outage, the second is an
// attack, and they are not the same alert.
var ErrUnavailable = errors.New("pubsub: cannot reach Google's key set")

// parseRSAPublicKey builds an RSA public key from the JWK's base64url n and e.
func parseRSAPublicKey(n, e string) (*rsa.PublicKey, error) {
	modulus, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(n, "="))
	if err != nil {
		return nil, fmt.Errorf("modulus is not base64url: %w", err)
	}
	exponentBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(e, "="))
	if err != nil {
		return nil, fmt.Errorf("exponent is not base64url: %w", err)
	}
	exponent := 0
	for _, b := range exponentBytes {
		exponent = exponent<<8 | int(b)
	}
	if exponent == 0 {
		return nil, errors.New("exponent is zero")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}, nil
}

// cacheAge reads max-age out of a Cache-Control header, defaulting to an hour.
func cacheAge(header string) time.Duration {
	const fallback = time.Hour
	for _, directive := range strings.Split(header, ",") {
		directive = strings.TrimSpace(directive)
		if !strings.HasPrefix(directive, "max-age=") {
			continue
		}
		seconds, err := strconv.Atoi(strings.TrimPrefix(directive, "max-age="))
		if err != nil || seconds <= 0 {
			return fallback
		}
		return time.Duration(seconds) * time.Second
	}
	return fallback
}

// OIDCFromEnv builds a verifier from the environment.
//
// The audience is required. A deployment that has not set it cannot verify
// anything, and is told so at the point of use rather than being handed a
// verifier that accepts every request.
func OIDCFromEnv() (*OIDCVerifier, error) {
	audience := strings.TrimSpace(os.Getenv(EnvPushAudience))
	if audience == "" {
		return nil, fmt.Errorf("%w: %s is required to authenticate Pub/Sub pushes",
			ErrUnconfigured, EnvPushAudience)
	}
	return &OIDCVerifier{
		Audience:            audience,
		ServiceAccountEmail: strings.TrimSpace(os.Getenv(EnvPushServiceAccount)),
		JWKSURL:             strings.TrimSpace(os.Getenv(EnvJWKSURL)),
	}, nil
}
