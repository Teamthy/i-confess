package push

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Push transport and credential handling (§47).

// ---------------------------------------------------------------------------
// Error classification
// ---------------------------------------------------------------------------

// The distinction that matters: a dead device is purged, a transient fault is
// retried. Conflating them means either hammering dead tokens forever — which
// damages standing with Apple and Google — or discarding good ones on a blip.
func TestErrorClassesAreDistinct(t *testing.T) {
	dead := InvalidToken("unregistered")
	transient := Retryable("503")
	permanent := Permanent("bad topic")

	if !IsInvalidToken(dead) {
		t.Fatal("invalid-token error not recognised")
	}
	if IsRetryable(dead) {
		t.Fatal("a dead token must not be retried")
	}
	if !IsRetryable(transient) {
		t.Fatal("transient error not retryable")
	}
	if IsInvalidToken(transient) || IsInvalidToken(permanent) {
		t.Fatal("a transient or permanent error was mistaken for a dead token")
	}
}

// ---------------------------------------------------------------------------
// Router
// ---------------------------------------------------------------------------

func TestRouterDispatchesByPlatform(t *testing.T) {
	apns, fcm := NewMemorySender(), NewMemorySender()
	r := &Router{APNs: apns, FCM: fcm}

	r.Send(context.Background(), Notification{Token: "a", Platform: PlatformIOS})
	r.Send(context.Background(), Notification{Token: "b", Platform: PlatformAndroid})
	// Web push also rides FCM, which is what the Firebase web SDK registers.
	r.Send(context.Background(), Notification{Token: "c", Platform: PlatformWeb})

	if len(apns.Sent()) != 1 {
		t.Fatalf("apns got %d, want 1", len(apns.Sent()))
	}
	if len(fcm.Sent()) != 2 {
		t.Fatalf("fcm got %d, want 2", len(fcm.Sent()))
	}
}

func TestRouterRefusesUnconfiguredPlatform(t *testing.T) {
	r := &Router{FCM: NewMemorySender()}

	err := r.Send(context.Background(), Notification{Token: "a", Platform: PlatformIOS})
	if err == nil {
		t.Fatal("sent to iOS with no APNs configured")
	}
	if IsRetryable(err) {
		t.Fatal("missing configuration is not a transient fault")
	}
	if err := r.Send(context.Background(), Notification{Token: "a", Platform: "toaster"}); err == nil {
		t.Fatal("unknown platform accepted")
	}
}

func TestRouterConfigured(t *testing.T) {
	if (&Router{}).Configured() {
		t.Fatal("empty router reported as configured")
	}
	if !(&Router{FCM: NewMemorySender()}).Configured() {
		t.Fatal("router with one provider reported as unconfigured")
	}
}

// ---------------------------------------------------------------------------
// APNs
// ---------------------------------------------------------------------------

func testAPNs(t *testing.T, handler http.HandlerFunc) (*APNs, *httptest.Server) {
	t.Helper()
	// A throwaway EC key; APNs uses ES256.
	pemKey := ecKeyPEM(t)
	srv := httptest.NewServer(handler)
	a, err := NewAPNs(pemKey, "TEAM123", "KEY123", "app.iconfess", false)
	if err != nil {
		t.Fatal(err)
	}
	a.BaseURL = srv.URL
	a.Client = srv.Client()
	return a, srv
}

func TestAPNsSendsExpectedHeaders(t *testing.T) {
	var gotTopic, gotAuth, gotType, gotExp, gotPath string
	a, srv := testAPNs(t, func(w http.ResponseWriter, r *http.Request) {
		gotTopic, gotAuth = r.Header.Get("apns-topic"), r.Header.Get("authorization")
		gotType, gotExp = r.Header.Get("apns-push-type"), r.Header.Get("apns-expiration")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	err := a.Send(context.Background(), Notification{
		Token: "devtoken", Platform: PlatformIOS,
		Title: "Morning", Body: "Ready", CollapseKey: "sched-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotTopic != "app.iconfess" {
		t.Fatalf("apns-topic = %q", gotTopic)
	}
	if !strings.HasPrefix(gotAuth, "bearer ") {
		t.Fatalf("authorization = %q, want a bearer JWT", gotAuth)
	}
	if gotType != "alert" {
		t.Fatalf("apns-push-type = %q", gotType)
	}
	// A reminder for a 6 AM session is worthless at 9 AM, so it must expire.
	if gotExp == "" || gotExp == "0" {
		t.Fatalf("apns-expiration = %q; the reminder would be stored indefinitely", gotExp)
	}
	if gotPath != "/3/device/devtoken" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestAPNsClassifiesResponses(t *testing.T) {
	cases := []struct {
		status int
		body   string
		dead   bool
		retry  bool
	}{
		{http.StatusGone, `{"reason":"Unregistered"}`, true, false},
		{http.StatusBadRequest, `{"reason":"BadDeviceToken"}`, true, false},
		{http.StatusTooManyRequests, `{"reason":"TooManyRequests"}`, false, true},
		{http.StatusInternalServerError, `{"reason":"InternalServerError"}`, false, true},
		{http.StatusForbidden, `{"reason":"InvalidProviderToken"}`, false, false},
		{http.StatusBadRequest, `{"reason":"BadTopic"}`, false, false},
	}
	for _, tc := range cases {
		a, srv := testAPNs(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte(tc.body))
		})
		err := a.Send(context.Background(), Notification{Token: "t", Platform: PlatformIOS})
		srv.Close()

		if err == nil {
			t.Fatalf("%d %s: expected an error", tc.status, tc.body)
		}
		if IsInvalidToken(err) != tc.dead {
			t.Fatalf("%s: dead = %v, want %v (%v)", tc.body, IsInvalidToken(err), tc.dead, err)
		}
		if IsRetryable(err) != tc.retry {
			t.Fatalf("%s: retryable = %v, want %v (%v)", tc.body, IsRetryable(err), tc.retry, err)
		}
	}
}

// The provider JWT is cached: Apple rejects refreshes more often than once per
// twenty minutes.
func TestAPNsCachesAuthToken(t *testing.T) {
	seen := map[string]bool{}
	a, srv := testAPNs(t, func(w http.ResponseWriter, r *http.Request) {
		seen[r.Header.Get("authorization")] = true
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	for i := 0; i < 5; i++ {
		a.Send(context.Background(), Notification{Token: "t", Platform: PlatformIOS})
	}
	if len(seen) != 1 {
		t.Fatalf("minted %d provider tokens for 5 sends, want 1", len(seen))
	}
}

func TestAPNsRejectsEmptyToken(t *testing.T) {
	a, srv := testAPNs(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("provider contacted with an empty token")
	})
	defer srv.Close()

	if err := a.Send(context.Background(), Notification{Platform: PlatformIOS}); err == nil {
		t.Fatal("empty token accepted")
	}
}

// ---------------------------------------------------------------------------
// FCM
// ---------------------------------------------------------------------------

func TestFCMClassifiesResponses(t *testing.T) {
	cases := []struct {
		status int
		body   string
		dead   bool
		retry  bool
	}{
		{http.StatusNotFound, `{"error":{"status":"NOT_FOUND"}}`, true, false},
		{http.StatusBadRequest, `{"error":{"status":"UNREGISTERED"}}`, true, false},
		{http.StatusTooManyRequests, `{}`, false, true},
		{http.StatusServiceUnavailable, `{}`, false, true},
		{http.StatusUnauthorized, `{}`, false, false},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte(tc.body))
		}))
		f := NewFCM("proj", func(context.Context) (string, error) { return "tok", nil })
		f.BaseURL = srv.URL
		err := f.Send(context.Background(), Notification{Token: "t", Platform: PlatformAndroid})
		srv.Close()

		if err == nil {
			t.Fatalf("%d: expected an error", tc.status)
		}
		if IsInvalidToken(err) != tc.dead {
			t.Fatalf("%d %s: dead = %v, want %v", tc.status, tc.body, IsInvalidToken(err), tc.dead)
		}
		if IsRetryable(err) != tc.retry {
			t.Fatalf("%d: retryable = %v, want %v", tc.status, IsRetryable(err), tc.retry)
		}
	}
}

func TestFCMSendsHighPriorityWithTTL(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, r.ContentLength)
		r.Body.Read(b)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	f := NewFCM("proj", func(context.Context) (string, error) { return "tok", nil })
	f.BaseURL = srv.URL

	if err := f.Send(context.Background(), Notification{
		Token: "t", Platform: PlatformAndroid, Title: "T", Body: "B",
	}); err != nil {
		t.Fatal(err)
	}
	// A devotional reminder delivered hours late is worse than not delivered.
	if !strings.Contains(body, `"priority":"high"`) {
		t.Fatalf("not sent at high priority: %s", body)
	}
	if !strings.Contains(body, `"ttl"`) {
		t.Fatalf("no TTL set, so a stale reminder could arrive whenever the phone reconnects: %s", body)
	}
}

func TestFCMWithoutCredentialsFailsPermanently(t *testing.T) {
	f := NewFCM("proj", nil)
	err := f.Send(context.Background(), Notification{Token: "t", Platform: PlatformAndroid})
	if err == nil {
		t.Fatal("sent with no credential source")
	}
	if IsRetryable(err) {
		t.Fatal("missing credentials is not a transient fault")
	}
}

// ---------------------------------------------------------------------------
// Google service-account tokens
// ---------------------------------------------------------------------------

func serviceAccountJSON(t *testing.T, tokenURL string) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	sa := map[string]string{
		"type": "service_account", "project_id": "iconfess-prod",
		"private_key": string(keyPEM), "client_email": "fcm@iconfess.iam.gserviceaccount.com",
		"token_uri": tokenURL,
	}
	b, _ := json.Marshal(sa)
	return b
}

// The previous implementation took a pre-minted token from the environment,
// which expires within the hour and was never refreshed.
func TestGoogleTokenSourceMintsAndCaches(t *testing.T) {
	var exchanges int
	var gotGrant, gotAssertion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchanges++
		r.ParseForm()
		gotGrant = r.Form.Get("grant_type")
		gotAssertion = r.Form.Get("assertion")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": fmt.Sprintf("ya29.token-%d", exchanges), "expires_in": 3600,
		})
	}))
	defer srv.Close()

	src, err := NewGoogleTokenSource(serviceAccountJSON(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	src.SetClock(func() time.Time { return now })

	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "ya29.token-1" {
		t.Fatalf("token = %q", tok)
	}
	if gotGrant != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
		t.Fatalf("grant_type = %q", gotGrant)
	}
	// The assertion must be a three-part signed JWT, not a bare claim set.
	if parts := strings.Split(gotAssertion, "."); len(parts) != 3 || parts[2] == "" {
		t.Fatalf("assertion is not a signed JWT: %q", gotAssertion)
	}

	// Repeated calls reuse the cached token: on a cold start many sends arrive
	// at once and Google rate-limits minting.
	for i := 0; i < 10; i++ {
		src.Token(context.Background())
	}
	if exchanges != 1 {
		t.Fatalf("%d token exchanges for 11 calls, want 1", exchanges)
	}

	// It refreshes before expiry rather than at it, so an in-flight send never
	// races the boundary.
	now = now.Add(58 * time.Minute)
	if _, err := src.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if exchanges != 2 {
		t.Fatalf("token was not refreshed near expiry (%d exchanges)", exchanges)
	}
}

func TestGoogleTokenSourceRejectsBadKeys(t *testing.T) {
	for _, bad := range [][]byte{
		[]byte(`not json`),
		[]byte(`{"type":"service_account"}`),
		[]byte(`{"client_email":"a@b.com","private_key":"-----BEGIN PRIVATE KEY-----\nnope\n-----END PRIVATE KEY-----\n"}`),
	} {
		if _, err := NewGoogleTokenSource(bad); err == nil {
			t.Fatalf("accepted an invalid service account: %s", bad)
		}
	}
}

// The project id comes from the key, so it cannot be misconfigured separately.
func TestFCMFromServiceAccountUsesKeyProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 3600})
	}))
	defer srv.Close()

	f, err := NewFCMFromServiceAccount(serviceAccountJSON(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if f.ProjectID != "iconfess-prod" {
		t.Fatalf("project = %q, want the value from the key", f.ProjectID)
	}
}

// A token-endpoint failure must not leak the assertion, which is a credential.
func TestTokenExchangeFailureDoesNotLeakAssertion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		http.Error(w, "invalid_grant: "+r.Form.Get("assertion"), http.StatusBadRequest)
	}))
	defer srv.Close()

	src, _ := NewGoogleTokenSource(serviceAccountJSON(t, srv.URL))
	_, err := src.Token(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "eyJ") {
		t.Fatalf("error message contains the signed assertion: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Logging hygiene
// ---------------------------------------------------------------------------

func TestMaskToken(t *testing.T) {
	if got := maskToken("abcdefghijklmnop"); strings.Contains(got, "efghijkl") {
		t.Fatalf("token not masked: %q", got)
	}
	if maskToken("short") != "***" {
		t.Fatal("short token not fully masked")
	}
}

// ecKeyPEM generates a throwaway EC P-256 key in PKCS#8 PEM form.
func ecKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}
