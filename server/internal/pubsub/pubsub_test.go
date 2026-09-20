package pubsub

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

// Tests for Pub/Sub push authentication (IC-003, PR B).
//
// The endpoint these protect is on the public internet and its payload is an
// instruction to go and read a subscription. If the token check can be
// satisfied by anyone, then anyone can make this server call Google about
// arbitrary purchase tokens and - worse - make it act on the answers. So the
// important cases here are the refusals: another audience, an expired token, a
// signature from another key, and an algorithm the verifier should not accept.

const (
	testAudience = "https://api.iconfess.app/webhooks/google"
	testKeyID    = "test-key-1"
)

// signToken mints an RS256 token the way Google does.
func signToken(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()

	header, err := json.Marshal(map[string]any{"alg": "RS256", "kid": kid, "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// jwksServer serves Google's key set for one key.
func jwksServer(t *testing.T, key *rsa.PublicKey, kid string) *httptest.Server {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"alg": "RS256",
			"use": "sig",
			"kid": kid,
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}},
	})
	if err != nil {
		t.Fatalf("marshal key set: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func validClaims() map[string]any {
	now := time.Now()
	return map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            testAudience,
		"exp":            now.Add(time.Hour).Unix(),
		"iat":            now.Unix(),
		"email":          "push@iconfess.iam.gserviceaccount.com",
		"email_verified": true,
		"sub":            "1234567890",
	}
}

func testVerifier(t *testing.T, kid string, mutate func(*OIDCVerifier)) (*OIDCVerifier, *rsa.PrivateKey) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	srv := jwksServer(t, &key.PublicKey, kid)

	verifier := &OIDCVerifier{
		Audience: testAudience,
		JWKSURL:  srv.URL,
	}
	if mutate != nil {
		mutate(verifier)
	}
	return verifier, key
}

func TestOIDCVerifierAcceptsAGoogleSignedToken(t *testing.T) {
	verifier, key := testVerifier(t, testKeyID, nil)
	token := signToken(t, key, testKeyID, validClaims())

	if err := verifier.Verify(context.Background(), token); err != nil {
		t.Fatalf("genuine token refused: %v", err)
	}
}

// TestOIDCVerifierRejectsAnotherAudience is the check that makes the endpoint
// safe to expose. A token minted for any other Google service is a perfectly
// valid Google token; only the audience distinguishes it from one issued for
// this push subscription.
func TestOIDCVerifierRejectsAnotherAudience(t *testing.T) {
	verifier, key := testVerifier(t, testKeyID, nil)
	claims := validClaims()
	claims["aud"] = "https://someone-elses-service.example"

	err := verifier.Verify(context.Background(), signToken(t, key, testKeyID, claims))
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
}

func TestOIDCVerifierRejectsAudienceListsWithoutThisAudience(t *testing.T) {
	verifier, key := testVerifier(t, testKeyID, nil)
	claims := validClaims()
	claims["aud"] = []string{"https://a.example", "https://b.example"}

	if err := verifier.Verify(context.Background(), signToken(t, key, testKeyID, claims)); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
}

func TestOIDCVerifierAcceptsAudienceListsContainingThisAudience(t *testing.T) {
	verifier, key := testVerifier(t, testKeyID, nil)
	claims := validClaims()
	claims["aud"] = []string{"https://a.example", testAudience}

	if err := verifier.Verify(context.Background(), signToken(t, key, testKeyID, claims)); err != nil {
		t.Fatalf("genuine token refused: %v", err)
	}
}

func TestOIDCVerifierRejectsExpiredAndFutureTokens(t *testing.T) {
	verifier, key := testVerifier(t, testKeyID, nil)

	expired := validClaims()
	expired["exp"] = time.Now().Add(-time.Minute).Unix()
	if err := verifier.Verify(context.Background(), signToken(t, key, testKeyID, expired)); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("expired token: err = %v, want ErrUnauthenticated", err)
	}

	future := validClaims()
	future["iat"] = time.Now().Add(time.Hour).Unix()
	if err := verifier.Verify(context.Background(), signToken(t, key, testKeyID, future)); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("future token: err = %v, want ErrUnauthenticated", err)
	}

	noExpiry := validClaims()
	delete(noExpiry, "exp")
	if err := verifier.Verify(context.Background(), signToken(t, key, testKeyID, noExpiry)); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("token without expiry: err = %v, want ErrUnauthenticated", err)
	}
}

func TestOIDCVerifierRejectsAnotherIssuer(t *testing.T) {
	verifier, key := testVerifier(t, testKeyID, nil)
	claims := validClaims()
	claims["iss"] = "https://accounts.evil.example"

	if err := verifier.Verify(context.Background(), signToken(t, key, testKeyID, claims)); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
}

// TestOIDCVerifierRejectsASignatureFromAnotherKey: the header is attacker
// controlled, so the key id can name the right key while the signature is made
// with a different one.
func TestOIDCVerifierRejectsASignatureFromAnotherKey(t *testing.T) {
	verifier, _ := testVerifier(t, testKeyID, nil)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}

	if err := verifier.Verify(context.Background(), signToken(t, other, testKeyID, validClaims())); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
}

// TestOIDCVerifierRejectsAlgorithmSubstitution: accepting the alg the token
// names is how an RS256 verifier is turned into an HS256 one with the public
// key as the HMAC secret.
func TestOIDCVerifierRejectsAlgorithmSubstitution(t *testing.T) {
	verifier, key := testVerifier(t, testKeyID, nil)
	claims := validClaims()

	header, err := json.Marshal(map[string]any{"alg": "none", "kid": testKeyID, "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	token := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload) + "."
	if err := verifier.Verify(context.Background(), token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("alg=none token: err = %v, want ErrUnauthenticated", err)
	}
	_ = key
}

func TestOIDCVerifierRejectsAnUnknownKeyID(t *testing.T) {
	verifier, key := testVerifier(t, testKeyID, nil)

	err := verifier.Verify(context.Background(), signToken(t, key, "rotated-away-key", validClaims()))
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
}

// TestOIDCVerifierChecksTheServiceAccount: with an expected sender configured,
// a token Google signed for a different project's push subscription is refused.
// Without it, any Google-signed token for this audience would be accepted - and
// an attacker can create a push subscription in their own project.
func TestOIDCVerifierChecksTheServiceAccount(t *testing.T) {
	const sender = "push@iconfess.iam.gserviceaccount.com"
	verifier, key := testVerifier(t, testKeyID, func(v *OIDCVerifier) {
		v.ServiceAccountEmail = sender
	})

	if err := verifier.Verify(context.Background(), signToken(t, key, testKeyID, validClaims())); err != nil {
		t.Fatalf("matching sender refused: %v", err)
	}

	claims := validClaims()
	claims["email"] = "push@attacker.iam.gserviceaccount.com"
	if err := verifier.Verify(context.Background(), signToken(t, key, testKeyID, claims)); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("foreign sender: err = %v, want ErrUnauthenticated", err)
	}

	unverified := validClaims()
	unverified["email_verified"] = false
	if err := verifier.Verify(context.Background(), signToken(t, key, testKeyID, unverified)); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("unverified email: err = %v, want ErrUnauthenticated", err)
	}
}

// TestOIDCVerifierWithoutAnAudienceRefusesEverything: there must be no
// configuration in which this endpoint authenticates nobody and therefore
// accepts everybody.
func TestOIDCVerifierWithoutAnAudienceRefusesEverything(t *testing.T) {
	verifier := &OIDCVerifier{}
	if err := verifier.Verify(context.Background(), "any.token.here"); !errors.Is(err, ErrUnconfigured) {
		t.Fatalf("err = %v, want ErrUnconfigured", err)
	}
}

func TestOIDCVerifierReportsAnUnreachableKeySet(t *testing.T) {
	verifier, key := testVerifier(t, testKeyID, func(v *OIDCVerifier) {
		v.JWKSURL = "http://127.0.0.1:1/certs"
	})
	err := verifier.Verify(context.Background(), signToken(t, key, testKeyID, validClaims()))
	// This is an outage, not an attack. An operator has to be able to tell the
	// two apart, so it must not be reported as an authentication failure.
	if errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("err = %v, want a fetch failure rather than ErrUnauthenticated", err)
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestOIDCVerifierVerifyRequestReadsTheBearerHeader(t *testing.T) {
	verifier, key := testVerifier(t, testKeyID, nil)
	token := signToken(t, key, testKeyID, validClaims())

	req := httptest.NewRequest(http.MethodPost, "/webhooks/google", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if err := verifier.VerifyRequest(context.Background(), req); err != nil {
		t.Fatalf("genuine request refused: %v", err)
	}

	missing := httptest.NewRequest(http.MethodPost, "/webhooks/google", nil)
	if err := verifier.VerifyRequest(context.Background(), missing); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("missing header: err = %v, want ErrUnauthenticated", err)
	}

	basic := httptest.NewRequest(http.MethodPost, "/webhooks/google", nil)
	basic.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	if err := verifier.VerifyRequest(context.Background(), basic); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("wrong scheme: err = %v, want ErrUnauthenticated", err)
	}
}

func TestOIDCFromEnvRequiresAnAudience(t *testing.T) {
	t.Setenv(EnvPushAudience, "")
	if _, err := OIDCFromEnv(); !errors.Is(err, ErrUnconfigured) {
		t.Fatalf("err = %v, want ErrUnconfigured", err)
	}

	t.Setenv(EnvPushAudience, testAudience)
	t.Setenv(EnvPushServiceAccount, "push@iconfess.iam.gserviceaccount.com")
	verifier, err := OIDCFromEnv()
	if err != nil {
		t.Fatalf("OIDCFromEnv: %v", err)
	}
	if verifier.Audience != testAudience || verifier.ServiceAccountEmail == "" {
		t.Errorf("verifier = %+v", verifier)
	}
}

// ---------------------------------------------------------------------------
// The envelope
// ---------------------------------------------------------------------------

func TestDecodeAcceptsADelivery(t *testing.T) {
	notification := []byte(`{"packageName":"app.iconfess","eventTimeMillis":"1700000000000"}`)
	body, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"data":        base64.StdEncoding.EncodeToString(notification),
			"messageId":   "1234567890",
			"publishTime": "2026-09-20T10:00:00Z",
		},
		"subscription": "projects/iconfess/subscriptions/rtdn-push",
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	env, err := Decode(body)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if env.Message.MessageID != "1234567890" {
		t.Errorf("message id = %q", env.Message.MessageID)
	}
	payload, err := env.Payload()
	if err != nil {
		t.Fatalf("Payload: %v", err)
	}
	if string(payload) != string(notification) {
		t.Errorf("payload = %s", payload)
	}
	if _, ok := env.PublishTime(); !ok {
		t.Error("publish time was not parsed")
	}
}

// A delivery without a message id cannot be deduplicated, and Pub/Sub retries
// until it is acknowledged - so the id is required, not merely recorded.
func TestDecodeRequiresAMessageIDAndPayload(t *testing.T) {
	for name, body := range map[string]string{
		"no id":      `{"message":{"data":"e30="}}`,
		"no data":    `{"message":{"messageId":"1"}}`,
		"empty":      `{"message":{}}`,
		"not json":   `nope`,
		"not base64": `{"message":{"messageId":"1","data":"!!not base64!!"}}`,
	} {
		env, err := Decode([]byte(body))
		if err != nil {
			continue
		}
		if _, err := env.Payload(); err == nil {
			t.Errorf("%s: delivery was accepted", name)
		}
	}
}
