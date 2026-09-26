// Package googleauth mints OAuth2 access tokens from a Google service-account
// key.
//
// Two callers need this and neither needs the rest of google.golang.org/api:
// FCM's HTTP v1 API (scope firebase.messaging) and the Play Developer API
// (scope androidpublisher). The protocol is a signed JWT exchanged at Google's
// token endpoint, which is roughly eighty lines against a stable, documented
// specification — cheaper than taking a large dependency tree for one grant
// type.
//
// The package exists because billing verification needs exactly the same flow
// as push delivery. It was extracted from internal/push rather than copied, so
// there is one implementation of the service-account handshake to get right:
// a second copy would be a second place for the token cache, the refresh margin
// and the scope to drift.
//
// The previous FCM implementation accepted a pre-minted token from the
// environment. That works for a demo and fails in production within the hour,
// because these tokens expire in sixty minutes and nothing was refreshing them.
package googleauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TokenURL is Google's OAuth2 token endpoint. A service-account key normally
// names it in its token_uri field; this is the fallback.
const TokenURL = "https://oauth2.googleapis.com/token"

// Scopes this server requests. Google returns a token carrying only the scopes
// asked for, so a leaked FCM token cannot read Play purchases and vice versa.
const (
	ScopeFirebaseMessaging = "https://www.googleapis.com/auth/firebase.messaging"
	ScopeAndroidPublisher  = "https://www.googleapis.com/auth/androidpublisher"
)

const (
	// TokenLifetime is what we request. Google caps it at an hour.
	TokenLifetime = time.Hour
	// refreshMargin renews early so an in-flight call never races an expiry.
	refreshMargin = 5 * time.Minute
)

// ServiceAccount is the subset of a Google service-account JSON key we use.
type ServiceAccount struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	PrivateKey  string `json:"private_key"`
	ClientEmail string `json:"client_email"`
	TokenURI    string `json:"token_uri"`
}

// TokenSource mints and caches access tokens for one service account.
type TokenSource struct {
	account ServiceAccount
	key     *rsa.PrivateKey
	scopes  []string
	client  *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time

	// now is injectable for tests.
	now func() time.Time
}

// New parses a service-account JSON key and returns a token source for the
// given scopes. At least one scope is required: a token with no scope is
// accepted by the token endpoint and then rejected by every API, which turns a
// configuration mistake into a confusing 403 much later.
func New(keyJSON []byte, scopes ...string) (*TokenSource, error) {
	var sa ServiceAccount
	if err := json.Unmarshal(keyJSON, &sa); err != nil {
		return nil, fmt.Errorf("parse service account: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, fmt.Errorf("service account is missing client_email or private_key")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = TokenURL
	}

	var wanted []string
	for _, s := range scopes {
		if s = strings.TrimSpace(s); s != "" {
			wanted = append(wanted, s)
		}
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("service account token source needs at least one scope")
	}

	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return nil, fmt.Errorf("service account private_key is not valid PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Older keys are PKCS#1.
		if k, err1 := x509.ParsePKCS1PrivateKey(block.Bytes); err1 == nil {
			parsed = k
		} else {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("service account key is not RSA")
	}

	return &TokenSource{
		account: sa, key: key, scopes: wanted,
		client: &http.Client{Timeout: 15 * time.Second},
		now:    time.Now,
	}, nil
}

// ProjectID reports the project the key belongs to, so callers do not have to
// configure it separately and risk a mismatch.
func (g *TokenSource) ProjectID() string { return g.account.ProjectID }

// Scopes returns the scopes this source requests.
func (g *TokenSource) Scopes() []string { return append([]string(nil), g.scopes...) }

// SetClock overrides the clock. Test-only.
func (g *TokenSource) SetClock(f func() time.Time) { g.now = f }

// SetTokenURL overrides the exchange endpoint. Test-only.
func (g *TokenSource) SetTokenURL(u string) { g.account.TokenURI = u }

// Token returns a valid access token, refreshing when needed.
//
// The mutex is held across the network call deliberately: on a cold start
// several sends arrive at once, and without it every one would mint its own
// token. Google rate-limits that, and the tokens are interchangeable anyway.
func (g *TokenSource) Token(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.token != "" && g.now().Before(g.expiresAt.Add(-refreshMargin)) {
		return g.token, nil
	}

	assertion, err := g.signedAssertion()
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		g.account.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusOK {
		// The body can contain the assertion; never log it verbatim.
		return "", fmt.Errorf("token endpoint returned %d", res.StatusCode)
	}

	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("token endpoint returned no access token")
	}

	ttl := time.Duration(out.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = TokenLifetime
	}
	g.token = out.AccessToken
	g.expiresAt = g.now().Add(ttl)
	return g.token, nil
}

// signedAssertion builds the RS256 JWT that is exchanged for an access token.
func (g *TokenSource) signedAssertion() (string, error) {
	now := g.now()
	header := `{"alg":"RS256","typ":"JWT"}`
	claims := fmt.Sprintf(
		`{"iss":%q,"scope":%q,"aud":%q,"iat":%d,"exp":%d}`,
		g.account.ClientEmail, strings.Join(g.scopes, " "), g.account.TokenURI,
		now.Unix(), now.Add(TokenLifetime).Unix())

	input := b64(header) + "." + b64(claims)
	digest := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, g.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign assertion: %w", err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func b64(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}
