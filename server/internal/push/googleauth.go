package push

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

// Google service-account authentication for FCM (§47).
//
// FCM's HTTP v1 API needs a short-lived OAuth2 access token, obtained by
// signing a JWT with a service-account key and exchanging it at Google's token
// endpoint. That is roughly eighty lines against a stable, documented protocol,
// which is cheaper than taking google.golang.org/api — a large dependency tree
// pulled in for one grant type.
//
// The previous implementation accepted a pre-minted token from the environment.
// That works for a demo and fails in production within the hour, because these
// tokens expire in sixty minutes and nothing was refreshing them.

const (
	googleTokenURL = "https://oauth2.googleapis.com/token"
	fcmScope       = "https://www.googleapis.com/auth/firebase.messaging"
	// tokenLifetime is what we request. Google caps it at an hour.
	tokenLifetime = time.Hour
	// refreshMargin renews early so an in-flight send never races an expiry.
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

// GoogleTokenSource mints and caches FCM access tokens.
type GoogleTokenSource struct {
	account ServiceAccount
	key     *rsa.PrivateKey
	client  *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time

	// now is injectable for tests.
	now func() time.Time
}

// NewGoogleTokenSource parses a service-account JSON key.
func NewGoogleTokenSource(keyJSON []byte) (*GoogleTokenSource, error) {
	var sa ServiceAccount
	if err := json.Unmarshal(keyJSON, &sa); err != nil {
		return nil, fmt.Errorf("parse service account: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, fmt.Errorf("service account is missing client_email or private_key")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = googleTokenURL
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

	return &GoogleTokenSource{
		account: sa, key: key,
		client: &http.Client{Timeout: 15 * time.Second},
		now:    time.Now,
	}, nil
}

// ProjectID reports the project the key belongs to, so callers do not have to
// configure it separately and risk a mismatch.
func (g *GoogleTokenSource) ProjectID() string { return g.account.ProjectID }

// SetClock overrides the clock. Test-only.
func (g *GoogleTokenSource) SetClock(f func() time.Time) { g.now = f }

// SetTokenURL overrides the exchange endpoint. Test-only.
func (g *GoogleTokenSource) SetTokenURL(u string) { g.account.TokenURI = u }

// Token returns a valid access token, refreshing when needed.
//
// The mutex is held across the network call deliberately: on a cold start
// several sends arrive at once, and without it every one would mint its own
// token. Google rate-limits that, and the tokens are interchangeable anyway.
func (g *GoogleTokenSource) Token(ctx context.Context) (string, error) {
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
	defer res.Body.Close()

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
		ttl = tokenLifetime
	}
	g.token = out.AccessToken
	g.expiresAt = g.now().Add(ttl)
	return g.token, nil
}

// signedAssertion builds the RS256 JWT that is exchanged for an access token.
func (g *GoogleTokenSource) signedAssertion() (string, error) {
	now := g.now()
	header := `{"alg":"RS256","typ":"JWT"}`
	claims := fmt.Sprintf(
		`{"iss":%q,"scope":%q,"aud":%q,"iat":%d,"exp":%d}`,
		g.account.ClientEmail, fcmScope, g.account.TokenURI,
		now.Unix(), now.Add(tokenLifetime).Unix())

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

// NewFCMFromServiceAccount wires an FCM sender using a service-account key.
func NewFCMFromServiceAccount(keyJSON []byte) (*FCM, error) {
	src, err := NewGoogleTokenSource(keyJSON)
	if err != nil {
		return nil, err
	}
	if src.ProjectID() == "" {
		return nil, fmt.Errorf("service account has no project_id")
	}
	f := NewFCM(src.ProjectID(), src.Token)
	return f, nil
}
