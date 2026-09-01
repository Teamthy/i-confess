package oauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Social identity verification (§36, §37, §64).
//
// These tests exist because the failure mode is total: a verifier that accepts
// a token it should not lets anyone sign in as anyone.

type testIssuer struct {
	key *rsa.PrivateKey
	kid string
	srv *httptest.Server
}

func newTestIssuer(t *testing.T) *testIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ti := &testIssuer{key: key, kid: "test-key-1"}

	ti.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
		json.NewEncoder(w).Encode(jwks{Keys: []jwk{
			{Kid: ti.kid, Kty: "RSA", Alg: "RS256", N: n, E: e},
		}})
	}))
	t.Cleanup(ti.srv.Close)
	return ti
}

// sign builds a JWT with the given claims, signed by the test issuer.
func (ti *testIssuer) sign(t *testing.T, header map[string]any, payload map[string]any) string {
	t.Helper()
	if header == nil {
		header = map[string]any{"alg": "RS256", "kid": ti.kid, "typ": "JWT"}
	}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(payload)
	input := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb)

	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, ti.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func newVerifier(t *testing.T, ti *testIssuer, audience string) *OIDCVerifier {
	t.Helper()
	v := NewGoogle(audience)
	v.SetJWKSURL(ti.srv.URL)
	v.SetClock(func() time.Time { return time.Unix(1_700_000_000, 0) })
	return v
}

func validClaims() map[string]any {
	return map[string]any{
		"iss": "https://accounts.google.com",
		"sub": "google-subject-123",
		"aud": "our-client-id",
		"exp": 1_700_003_600,
		"iat": 1_700_000_000,
		// Deliberately a string: Google has historically sent it both ways.
		"email":          "grace@example.com",
		"email_verified": "true",
		"name":           "Grace",
	}
}

func TestVerifyAcceptsWellFormedToken(t *testing.T) {
	ti := newTestIssuer(t)
	v := newVerifier(t, ti, "our-client-id")

	id, err := v.Verify(context.Background(), ti.sign(t, nil, validClaims()))
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if id.Subject != "google-subject-123" {
		t.Fatalf("subject = %q", id.Subject)
	}
	if id.Email != "grace@example.com" || !id.EmailVerified {
		t.Fatalf("email claims not parsed: %+v", id)
	}
	if id.Provider != ProviderGoogle {
		t.Fatalf("provider = %q", id.Provider)
	}
}

// A token minted for another application must not be accepted here, or any
// developer using the same provider could sign into our users' accounts (§64).
func TestVerifyRejectsWrongAudience(t *testing.T) {
	ti := newTestIssuer(t)
	v := newVerifier(t, ti, "our-client-id")

	c := validClaims()
	c["aud"] = "someone-elses-client-id"

	if _, err := v.Verify(context.Background(), ti.sign(t, nil, c)); err == nil {
		t.Fatal("token for a different audience was accepted")
	}
}

// An unconfigured audience must fail closed rather than accept everything.
func TestVerifyWithNoConfiguredAudienceRejects(t *testing.T) {
	ti := newTestIssuer(t)
	v := newVerifier(t, ti, "")

	if _, err := v.Verify(context.Background(), ti.sign(t, nil, validClaims())); err == nil {
		t.Fatal("verifier with no audience configured accepted a token")
	}
}

func TestVerifyRejectsWrongIssuer(t *testing.T) {
	ti := newTestIssuer(t)
	v := newVerifier(t, ti, "our-client-id")

	c := validClaims()
	c["iss"] = "https://evil.example.com"

	if _, err := v.Verify(context.Background(), ti.sign(t, nil, c)); err == nil {
		t.Fatal("token from an unexpected issuer was accepted")
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	ti := newTestIssuer(t)
	v := newVerifier(t, ti, "our-client-id")

	c := validClaims()
	c["exp"] = 1_699_999_999 // one second before the frozen clock

	if _, err := v.Verify(context.Background(), ti.sign(t, nil, c)); err == nil {
		t.Fatal("expired token accepted")
	}
}

// The classic JWT attack: claim no signature is needed.
func TestVerifyRejectsAlgNone(t *testing.T) {
	ti := newTestIssuer(t)
	v := newVerifier(t, ti, "our-client-id")

	hb, _ := json.Marshal(map[string]any{"alg": "none", "kid": ti.kid})
	pb, _ := json.Marshal(validClaims())
	token := base64.RawURLEncoding.EncodeToString(hb) + "." +
		base64.RawURLEncoding.EncodeToString(pb) + "."

	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("alg:none token accepted")
	}
}

// A token signed by a key we do not trust must be refused, even if every claim
// is otherwise perfect.
func TestVerifyRejectsForeignSigningKey(t *testing.T) {
	realIssuer := newTestIssuer(t)
	attacker := newTestIssuer(t)
	// The attacker uses the same kid so the lookup succeeds and the signature
	// check is what has to catch it.
	attacker.kid = realIssuer.kid

	v := newVerifier(t, realIssuer, "our-client-id")

	if _, err := v.Verify(context.Background(), attacker.sign(t, nil, validClaims())); err == nil {
		t.Fatal("token signed by an untrusted key was accepted")
	}
}

// Tampering with the payload must invalidate the signature.
func TestVerifyRejectsTamperedPayload(t *testing.T) {
	ti := newTestIssuer(t)
	v := newVerifier(t, ti, "our-client-id")

	token := ti.sign(t, nil, validClaims())

	// Swap the payload for one claiming a different subject.
	forged, _ := json.Marshal(map[string]any{
		"iss": "https://accounts.google.com", "sub": "victim-subject",
		"aud": "our-client-id", "exp": 1_700_003_600,
	})
	parts := splitJWT(token)
	tampered := parts[0] + "." + base64.RawURLEncoding.EncodeToString(forged) + "." + parts[2]

	if _, err := v.Verify(context.Background(), tampered); err == nil {
		t.Fatal("tampered payload accepted")
	}
}

func TestVerifyRejectsMalformedTokens(t *testing.T) {
	ti := newTestIssuer(t)
	v := newVerifier(t, ti, "our-client-id")

	for _, tok := range []string{"", "abc", "a.b", "a.b.c.d", "!!!.???.###"} {
		if _, err := v.Verify(context.Background(), tok); err == nil {
			t.Fatalf("malformed token accepted: %q", tok)
		}
	}
}

func TestVerifyRejectsTokenWithoutSubject(t *testing.T) {
	ti := newTestIssuer(t)
	v := newVerifier(t, ti, "our-client-id")

	c := validClaims()
	delete(c, "sub")

	if _, err := v.Verify(context.Background(), ti.sign(t, nil, c)); err == nil {
		t.Fatal("token with no subject accepted")
	}
}

// email_verified arrives as a bool from Apple and a string from Google.
func TestEmailVerifiedAcceptsBothEncodings(t *testing.T) {
	ti := newTestIssuer(t)
	v := newVerifier(t, ti, "our-client-id")

	c := validClaims()
	c["email_verified"] = true
	id, err := v.Verify(context.Background(), ti.sign(t, nil, c))
	if err != nil || !id.EmailVerified {
		t.Fatalf("bool email_verified not honoured: %+v %v", id, err)
	}

	c["email_verified"] = false
	id, err = v.Verify(context.Background(), ti.sign(t, nil, c))
	if err != nil || id.EmailVerified {
		t.Fatalf("false email_verified not honoured: %+v %v", id, err)
	}
}

// Keys are cached so a sign-in does not always cost an extra network call.
func TestJWKSIsCached(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	fetches := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches++
		n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
		json.NewEncoder(w).Encode(jwks{Keys: []jwk{{Kid: "k1", Kty: "RSA", Alg: "RS256", N: n, E: e}}})
	}))
	defer srv.Close()

	ti := &testIssuer{key: key, kid: "k1", srv: srv}
	v := newVerifier(t, ti, "our-client-id")

	for i := 0; i < 5; i++ {
		if _, err := v.Verify(context.Background(), ti.sign(t, nil, validClaims())); err != nil {
			t.Fatal(err)
		}
	}
	if fetches != 1 {
		t.Fatalf("fetched JWKS %d times for 5 verifications, want 1", fetches)
	}
}

// If the provider is unreachable the error must be distinguishable from an
// invalid token: one is our problem, the other is the caller's.
func TestProviderOutageIsDistinctFromInvalidToken(t *testing.T) {
	ti := newTestIssuer(t)
	token := ti.sign(t, nil, validClaims())

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer dead.Close()

	v := NewGoogle("our-client-id")
	v.SetJWKSURL(dead.URL)
	v.SetClock(func() time.Time { return time.Unix(1_700_000_000, 0) })

	_, err := v.Verify(context.Background(), token)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, ErrProviderUnavai) {
		t.Fatalf("error = %v, want a provider-unavailable error", err)
	}
}

func TestAppleVerifierUsesAppleIssuer(t *testing.T) {
	v := NewApple("com.iconfess.app")
	if v.Name() != ProviderApple {
		t.Fatalf("provider = %q", v.Name())
	}
	if !contains(v.issuers, "https://appleid.apple.com") {
		t.Fatalf("apple issuer not configured: %v", v.issuers)
	}
}

func splitJWT(t string) [3]string {
	var out [3]string
	i, start := 0, 0
	for j := 0; j < len(t) && i < 3; j++ {
		if t[j] == '.' {
			out[i] = t[start:j]
			i++
			start = j + 1
		}
	}
	if i < 3 {
		out[i] = t[start:]
	}
	return out
}
