package api

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/billing"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

// Store notification webhook tests (IC-003, PR B).
//
// These are the audit's acceptance tests, run through the real router and a
// real database:
//
//   - a replayed webhook has a single effect;
//   - reordered webhooks leave the correct final state;
//   - a webhook that is not signed by the store changes nothing;
//   - a refund revokes entitlement while the paid period is still running.
//
// They are end-to-end on purpose. Each of those properties spans the signature
// check, the idempotency index, the ordering watermark and the entitlement
// resolver, and a unit test of any one of them would pass while the others
// failed.

const (
	webhookBundleID = "app.iconfess"
	webhookSKU      = "app.iconfess.premium.monthly"
	webhookTxnID    = "2000000987654321"
)

// webhookPKI is a self-signed stand-in for Apple's PKI, pinned as the trust
// anchor exactly the way production pins Apple's root.
type webhookPKI struct {
	root         *x509.Certificate
	rootKey      *ecdsa.PrivateKey
	intermediate *x509.Certificate
	intKey       *ecdsa.PrivateKey
	leaf         *x509.Certificate
	leafKey      *ecdsa.PrivateKey
}

func newWebhookPKI(t *testing.T) *webhookPKI {
	t.Helper()

	issue := func(cn string, curve elliptic.Curve, isCA bool, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
		key, err := ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(time.Now().UnixNano()),
			Subject:      pkix.Name{CommonName: cn, Organization: []string{"iCONFESS Test"}},
			// Wide validity window on purpose: a notification's chain is verified at
			// the time Apple signed it, and the ordering tests sign notifications
			// dated in the past.
			NotBefore:             time.Now().Add(-30 * 24 * time.Hour),
			NotAfter:              time.Now().Add(365 * 24 * time.Hour),
			KeyUsage:              x509.KeyUsageDigitalSignature,
			BasicConstraintsValid: true,
			IsCA:                  isCA,
		}
		if isCA {
			tmpl.KeyUsage |= x509.KeyUsageCertSign
		}
		signer, signerKey := tmpl, key
		if parent != nil {
			signer, signerKey = parent, parentKey
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, &key.PublicKey, signerKey)
		if err != nil {
			t.Fatalf("create certificate: %v", err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatalf("parse certificate: %v", err)
		}
		return cert, key
	}

	root, rootKey := issue("iCONFESS Webhook Root", elliptic.P384(), true, nil, nil)
	intermediate, intKey := issue("iCONFESS Webhook WWDR", elliptic.P384(), true, root, rootKey)
	leaf, leafKey := issue("iCONFESS Webhook Leaf", elliptic.P256(), false, intermediate, intKey)
	return &webhookPKI{root: root, rootKey: rootKey, intermediate: intermediate, intKey: intKey, leaf: leaf, leafKey: leafKey}
}

func (p *webhookPKI) pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(p.root)
	return pool
}

func (p *webhookPKI) chain() []*x509.Certificate {
	return []*x509.Certificate{p.leaf, p.intermediate, p.root}
}

// signJWS builds an ES256 JWS with the certificate chain in the header, as
// Apple does.
func (p *webhookPKI) signJWS(t *testing.T, claims map[string]any) string {
	t.Helper()

	x5c := make([]string, 0, 3)
	for _, c := range p.chain() {
		x5c = append(x5c, base64.StdEncoding.EncodeToString(c.Raw))
	}
	header, err := json.Marshal(map[string]any{"alg": "ES256", "x5c": x5c})
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
	r, s, err := ecdsa.Sign(rand.Reader, p.leafKey, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// notification builds an App Store notification body.
func (p *webhookPKI) notification(t *testing.T, kind, subtype, uuid string, at time.Time, tx map[string]any) []byte {
	t.Helper()

	data := map[string]any{
		"appAppleId":  1234567890,
		"bundleId":    webhookBundleID,
		"environment": "Sandbox",
	}
	if tx != nil {
		data["signedTransactionInfo"] = p.signJWS(t, tx)
	}
	payload := map[string]any{
		"notificationType": kind,
		"subtype":          subtype,
		"notificationUUID": uuid,
		"version":          "2.0",
		"signedDate":       at.UnixMilli(),
		"data":             data,
	}
	body, err := json.Marshal(map[string]any{"signedPayload": p.signJWS(t, payload)})
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	return body
}

func webhookTxClaims(expires time.Time) map[string]any {
	return map[string]any{
		"transactionId":         webhookTxnID,
		"originalTransactionId": webhookTxnID,
		"bundleId":              webhookBundleID,
		"productId":             webhookSKU,
		"type":                  "Auto-Renewable Subscription",
		"inAppOwnershipType":    "PURCHASED",
		"environment":           "Sandbox",
		"signedDate":            time.Now().UnixMilli(),
		"expiresDate":           expires.UnixMilli(),
	}
}

// webhookHarness is a handler whose Apple trust anchor is a generated CA, so
// notifications can be genuinely signed in a test.
type webhookHarness struct {
	*authHarness
	pki *webhookPKI
}

func newWebhookHarness(t *testing.T) *webhookHarness {
	t.Helper()

	a := newAuthHarness(t)
	pki := newWebhookPKI(t)
	verifier, err := billing.NewAppleNotificationVerifier(billing.AppleNotificationConfig{
		BundleID:     webhookBundleID,
		Environment:  "Sandbox",
		ProductPlans: map[string]string{webhookSKU: "monthly"},
		Roots:        pki.pool(),
		Now:          func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		t.Fatalf("build notification verifier: %v", err)
	}
	a.h.SetAppleNotificationVerifier(verifier)
	return &webhookHarness{authHarness: a, pki: pki}
}

// post delivers a notification body to the Apple endpoint.
func (w *webhookHarness) post(t *testing.T, path string, body []byte) (int, map[string]any) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	w.router.ServeHTTP(rec, req)

	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// seedSubscription records a verified purchase, as the verify endpoint would.
func seedSubscription(t *testing.T, h *Handler, userID string, plan, status, expires string) {
	t.Helper()
	if err := store.NewUserStore(h.db).SaveVerifiedSubscription(t.Context(), userID, store.VerifiedSubscription{
		Plan:                  plan,
		Status:                status,
		ExpiresAt:             expires,
		Provider:              "apple",
		ProviderTransactionID: webhookTxnID,
		OriginalTransactionID: webhookTxnID,
		ProductID:             webhookSKU,
		StoreEnvironment:      "Sandbox",
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
}

// entitlementOf reads the plan the server would grant right now.
func entitlementOf(t *testing.T, h *Handler, userID string) string {
	t.Helper()
	plan, err := store.NewUserStore(h.db).Subscription(t.Context(), userID)
	if err != nil {
		t.Fatalf("read entitlement: %v", err)
	}
	return plan
}

// ---------------------------------------------------------------------------
// Acceptance: replay, ordering, forgery, revocation
// ---------------------------------------------------------------------------

// TestAppleWebhookReplayHasASingleEffect is the audit's "replayed webhook ->
// single effect".
//
// The retry carries an expiry a year out. If the duplicate were applied, the
// subscription would jump a year; the test reads the row back to prove it did
// not.
func TestAppleWebhookReplayHasASingleEffect(t *testing.T) {
	w := newWebhookHarness(t)
	_, userID := w.register(t, "webhook-replay@test.com")

	now := time.Now().UTC()
	seedSubscription(t, w.h, userID, "premium", models.SubscriptionActive, now.Add(30*24*time.Hour).Format(time.RFC3339))

	body := w.pki.notification(t, "DID_RENEW", "", "replay-uuid-1", now,
		webhookTxClaims(now.Add(31*24*time.Hour)))

	code, out := w.post(t, "/subscriptions/webhooks/apple", body)
	if code != http.StatusOK {
		t.Fatalf("first delivery: %d %v", code, out)
	}
	if out["status"] != store.NotificationApplied {
		t.Fatalf("first delivery status = %v, want applied", out["status"])
	}

	// The retry: same notification id, a much later expiry.
	retry := w.pki.notification(t, "DID_RENEW", "", "replay-uuid-1", now,
		webhookTxClaims(now.Add(365*24*time.Hour)))
	code, out = w.post(t, "/subscriptions/webhooks/apple", retry)
	if code != http.StatusOK {
		t.Fatalf("replay: %d %v", code, out)
	}
	if out["status"] != store.NotificationDuplicate {
		t.Fatalf("replay status = %v, want duplicate", out["status"])
	}

	record, err := store.NewUserStore(w.h.db).SubscriptionRecord(t.Context(), userID)
	if err != nil || record == nil {
		t.Fatalf("read subscription: %v", err)
	}
	want := now.Add(31 * 24 * time.Hour).Format(time.RFC3339)
	if record.EndsAt != want {
		t.Fatalf("ends_at = %s, want %s - the replay applied a second effect", record.EndsAt, want)
	}
}

// TestAppleWebhookOutOfOrderLeavesTheNewestState is the audit's "reordered
// webhooks -> correct final state".
//
// The expiry is delivered first, then a renewal that predates it. Applying them
// in arrival order leaves a subscription that ended entitling premium.
func TestAppleWebhookOutOfOrderLeavesTheNewestState(t *testing.T) {
	w := newWebhookHarness(t)
	_, userID := w.register(t, "webhook-order@test.com")

	now := time.Now().UTC()
	seedSubscription(t, w.h, userID, "premium", models.SubscriptionActive, now.Add(24*time.Hour).Format(time.RFC3339))

	// Newer event first: the subscription ended an hour ago.
	expiry := w.pki.notification(t, "EXPIRED", "", "order-uuid-expired", now,
		webhookTxClaims(now.Add(-time.Hour)))
	if code, out := w.post(t, "/subscriptions/webhooks/apple", expiry); code != http.StatusOK {
		t.Fatalf("expiry: %d %v", code, out)
	}
	if got := entitlementOf(t, w.h, userID); got != "free" {
		t.Fatalf("entitlement after expiry = %q, want free", got)
	}

	// The delayed renewal arrives afterwards, dated a day earlier.
	late := w.pki.notification(t, "DID_RENEW", "", "order-uuid-renew", now.Add(-24*time.Hour),
		webhookTxClaims(now.Add(20*24*time.Hour)))
	code, out := w.post(t, "/subscriptions/webhooks/apple", late)
	if code != http.StatusOK {
		t.Fatalf("late renewal: %d %v", code, out)
	}
	if out["status"] != store.NotificationStale {
		t.Fatalf("late renewal status = %v, want stale", out["status"])
	}
	if got := entitlementOf(t, w.h, userID); got != "free" {
		t.Fatalf("entitlement = %q - a late renewal re-granted premium after expiry", got)
	}
}

// TestAppleWebhookRejectsAnUnsignedNotification: the endpoint is public, so the
// signature is the only thing standing between a stranger and the entitlement
// table.
func TestAppleWebhookRejectsAnUnsignedNotification(t *testing.T) {
	w := newWebhookHarness(t)
	_, userID := w.register(t, "webhook-forgery@test.com")

	now := time.Now().UTC()
	seedSubscription(t, w.h, userID, "premium", models.SubscriptionRefunded, now.Add(30*24*time.Hour).Format(time.RFC3339))

	// A body that asserts an active subscription with no Apple signature.
	forged := []byte(fmt.Sprintf(`{
		"notificationType":"DID_RENEW",
		"notificationUUID":"forged-1",
		"version":"2.0",
		"signedDate":%d,
		"data":{"bundleId":%q,"environment":"Sandbox","signedTransactionInfo":"eyJhbGciOiJFUzI1NiJ9.%s.x"}
	}`, now.UnixMilli(), webhookBundleID, base64.RawURLEncoding.EncodeToString([]byte(`{"productId":"`+webhookSKU+`","expiresDate":99999999999999}`))))

	code, out := w.post(t, "/subscriptions/webhooks/apple", forged)
	if code != http.StatusBadRequest {
		t.Fatalf("forged notification: %d %v, want 400", code, out)
	}
	if got := entitlementOf(t, w.h, userID); got != "free" {
		t.Fatalf("a forged notification granted %q", got)
	}
}

// TestAppleWebhookRefundRevokesEntitlement: a refunded subscription keeps its
// expiry date, so watching only the clock leaves it premium for the rest of the
// paid year.
func TestAppleWebhookRefundRevokesEntitlement(t *testing.T) {
	w := newWebhookHarness(t)
	_, userID := w.register(t, "webhook-refund@test.com")

	now := time.Now().UTC()
	seedSubscription(t, w.h, userID, "premium", models.SubscriptionActive, now.Add(300*24*time.Hour).Format(time.RFC3339))
	if got := entitlementOf(t, w.h, userID); got != "premium" {
		t.Fatalf("entitlement = %q before the refund, want premium", got)
	}

	body := w.pki.notification(t, "REFUND", "UNREPORTED", "refund-uuid-1", now,
		webhookTxClaims(now.Add(300*24*time.Hour)))
	code, out := w.post(t, "/subscriptions/webhooks/apple", body)
	if code != http.StatusOK {
		t.Fatalf("refund: %d %v", code, out)
	}
	if got := entitlementOf(t, w.h, userID); got != "free" {
		t.Fatalf("entitlement = %q after a refund, want free", got)
	}

	record, err := store.NewUserStore(w.h.db).SubscriptionRecord(t.Context(), userID)
	if err != nil || record == nil {
		t.Fatalf("read subscription: %v", err)
	}
	if record.Status != models.SubscriptionRefunded {
		t.Errorf("status = %q, want refunded", record.Status)
	}
}

// TestAppleWebhookIsServedUnderV1 keeps the versioned alias honest: a client
// that only knows /v1 must be able to reach the endpoint a store is configured
// against.
func TestAppleWebhookIsServedUnderV1(t *testing.T) {
	w := newWebhookHarness(t)
	_, userID := w.register(t, "webhook-v1@test.com")

	now := time.Now().UTC()
	seedSubscription(t, w.h, userID, "premium", models.SubscriptionActive, now.Add(24*time.Hour).Format(time.RFC3339))

	body := w.pki.notification(t, "DID_RENEW", "", "v1-uuid-1", now, webhookTxClaims(now.Add(30*24*time.Hour)))
	if code, out := w.post(t, "/v1/subscriptions/webhooks/apple", body); code != http.StatusOK {
		t.Fatalf("/v1 delivery: %d %v", code, out)
	}
}

// TestAppleWebhookWithoutConfigurationRefuses: a deployment that cannot verify
// must not silently accept. 503 rather than 200 so the store retries - this is
// the one refusal a configuration change resolves.
func TestAppleWebhookWithoutConfigurationRefuses(t *testing.T) {
	a := newAuthHarness(t)
	// No verifier injected and no Apple configuration in the environment, so
	// the endpoint has nothing to verify with.
	t.Setenv("ENV", "production")
	t.Setenv("APPLE_BUNDLE_ID", "")
	t.Setenv("APPLE_PRODUCT_MONTHLY", "")

	req := httptest.NewRequest(http.MethodPost, "/subscriptions/webhooks/apple",
		bytes.NewReader([]byte(`{"signedPayload":"x.y.z"}`)))
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// The Play endpoint
// ---------------------------------------------------------------------------

// TestGoogleWebhookRequiresPushAuthentication: the endpoint is open to the
// internet and Pub/Sub is the only legitimate caller, so an unauthenticated
// request is refused before its body is looked at.
func TestGoogleWebhookRequiresPushAuthentication(t *testing.T) {
	a := newAuthHarness(t)
	t.Setenv("PUBSUB_PUSH_AUDIENCE", "https://api.iconfess.app/webhooks/google")

	req := httptest.NewRequest(http.MethodPost, "/subscriptions/webhooks/google",
		bytes.NewReader([]byte(`{"message":{"messageId":"1","data":"e30="}}`)))
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	// And with no audience configured at all, it refuses to run rather than
	// accepting unauthenticated pushes.
	t.Setenv("PUBSUB_PUSH_AUDIENCE", "")
	rec = httptest.NewRecorder()
	a.router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/subscriptions/webhooks/google", bytes.NewReader([]byte(`{}`))))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d with no audience, want 503", rec.Code)
	}
}
