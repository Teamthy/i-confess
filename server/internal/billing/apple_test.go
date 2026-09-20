package billing

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"
)

// Tests for App Store verification (IC-003).
//
// The finding these tests close: production "verification" split the receipt on
// ".", base64-decoded the payload and believed the claims in it. No signature
// was checked, no certificate chain was checked, and the crypto/x509 import in
// that file was unused. The tests below therefore do not merely check that a
// good receipt is accepted — they check that a *forged* one is refused, because
// that is the property that was absent.

const (
	testBundleID      = "app.iconfess"
	testMonthlySKU    = "app.iconfess.premium.monthly"
	testAnnualSKU     = "app.iconfess.premium.annual"
	testTransactionID = "2000000123456789"
)

// ---------------------------------------------------------------------------
// Test certificate authority
// ---------------------------------------------------------------------------

// appleTestPKI stands in for Apple's PKI. Real receipts cannot be used in a
// test: they expire, they leak a real purchase, and they would make the suite
// depend on Apple being reachable. What matters is that the verifier's trust
// decision is exercised, so the test generates a CA and pins it exactly the way
// production pins Apple's root.
type appleTestPKI struct {
	root         *x509.Certificate
	rootKey      *ecdsa.PrivateKey
	intermediate *x509.Certificate
	intKey       *ecdsa.PrivateKey
	leaf         *x509.Certificate
	leafKey      *ecdsa.PrivateKey
}

func newAppleTestPKI(t *testing.T) *appleTestPKI {
	t.Helper()

	root, rootKey := issueCert(t, certSpec{
		commonName: "iCONFESS Test Root CA", curve: elliptic.P384(), isCA: true,
	})
	intermediate, intKey := issueCert(t, certSpec{
		commonName: "iCONFESS Test WWDR", curve: elliptic.P384(), isCA: true,
		parent: root, parentKey: rootKey,
	})
	leaf, leafKey := issueCert(t, certSpec{
		commonName: "iCONFESS Test Signing Leaf", curve: elliptic.P256(),
		parent: intermediate, parentKey: intKey,
	})
	return &appleTestPKI{
		root: root, rootKey: rootKey,
		intermediate: intermediate, intKey: intKey,
		leaf: leaf, leafKey: leafKey,
	}
}

// pool is the pinned trust anchor for a verifier under test.
func (p *appleTestPKI) pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(p.root)
	return pool
}

// chain is what a real x5c header carries: leaf, intermediate, root.
func (p *appleTestPKI) chain() []*x509.Certificate {
	return []*x509.Certificate{p.leaf, p.intermediate, p.root}
}

type certSpec struct {
	commonName string
	curve      elliptic.Curve
	isCA       bool
	parent     *x509.Certificate
	parentKey  *ecdsa.PrivateKey
	// notBefore and notAfter override the validity window, for tests that need
	// a certificate which has already expired.
	notBefore time.Time
	notAfter  time.Time
}

func issueCert(t *testing.T, spec certSpec) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(spec.curve, rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	notBefore := spec.notBefore
	if notBefore.IsZero() {
		notBefore = time.Now().Add(-time.Hour)
	}
	notAfter := spec.notAfter
	if notAfter.IsZero() {
		notAfter = time.Now().Add(365 * 24 * time.Hour)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: spec.commonName, Organization: []string{"iCONFESS Test"}},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	if spec.isCA {
		tmpl.IsCA = true
		tmpl.KeyUsage |= x509.KeyUsageCertSign
	}

	parent, parentKey := tmpl, key
	if spec.parent != nil {
		parent, parentKey = spec.parent, spec.parentKey
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert, key
}

// ---------------------------------------------------------------------------
// JWS construction
// ---------------------------------------------------------------------------

// signAppleJWS builds a receipt the way Apple would: ES256 over
// "header.payload", with the certificate chain in the header.
func signAppleJWS(t *testing.T, key *ecdsa.PrivateKey, chain []*x509.Certificate, claims map[string]any) string {
	t.Helper()

	x5c := make([]string, 0, len(chain))
	for _, c := range chain {
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
	return signJWSWithHeader(t, key, header, payload)
}

func signJWSWithHeader(t *testing.T, key *ecdsa.PrivateKey, header, payload []byte) string {
	t.Helper()

	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	// JWS ECDSA signatures are the fixed-width concatenation r||s.
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func appleClaims() map[string]any {
	now := time.Now()
	return map[string]any{
		"transactionId":               testTransactionID,
		"originalTransactionId":       testTransactionID,
		"bundleId":                    testBundleID,
		"productId":                   testMonthlySKU,
		"subscriptionGroupIdentifier": "20000001",
		"type":                        "Auto-Renewable Subscription",
		"inAppOwnershipType":          "PURCHASED",
		"environment":                 AppleEnvironmentSandbox,
		"signedDate":                  now.UnixMilli(),
		"purchaseDate":                now.Add(-24 * time.Hour).UnixMilli(),
		"expiresDate":                 now.Add(30 * 24 * time.Hour).UnixMilli(),
	}
}

func newAppleVerifier(t *testing.T, pki *appleTestPKI, mutate func(*AppleConfig)) *AppleVerifier {
	t.Helper()

	cfg := AppleConfig{
		BundleID: testBundleID,
		ProductPlans: map[string]string{
			testMonthlySKU: "monthly",
			testAnnualSKU:  "annual",
		},
		Roots: pki.pool(),
	}
	if mutate != nil {
		mutate(&cfg)
	}
	verifier, err := NewAppleVerifier(cfg)
	if err != nil {
		t.Fatalf("build verifier: %v", err)
	}
	return verifier
}

// ---------------------------------------------------------------------------
// The exploit
// ---------------------------------------------------------------------------

// TestAppleVerifierRejectsAnUnsignedForgedJWS is the regression test for IC-003.
//
// This is the exact payload shape the previous production verifier accepted:
// three dot-separated segments, a base64 payload claiming a product and a
// future expiry, and a signature segment that was never examined. Anyone who
// could run base64 could have obtained premium.
func TestAppleVerifierRejectsAnUnsignedForgedJWS(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, nil)

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"ES256"}`))
	payload, _ := json.Marshal(appleClaims())
	forged := header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".not-a-signature"

	got, err := verifier.Verify(context.Background(), "apple", forged)
	if err == nil && got.Valid {
		t.Fatal("a forged, unsigned JWS was accepted as a valid purchase - premium is free again")
	}
	if err == nil {
		t.Fatalf("forged JWS returned no error (result %+v)", got)
	}
	if !errors.Is(err, ErrInvalidReceipt) {
		t.Fatalf("error = %v, want ErrInvalidReceipt", err)
	}
	if !strings.Contains(err.Error(), "chain") && !strings.Contains(err.Error(), "certificate") {
		t.Logf("note: rejection reason was %q", err)
	}
}

// A signature that verifies for a different payload must not be reusable: the
// attack is to take a receipt the attacker legitimately bought and rewrite the
// expiry or the product.
func TestAppleVerifierRejectsATamperedPayload(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, nil)

	genuine := signAppleJWS(t, pki.leafKey, pki.chain(), appleClaims())
	parts := strings.Split(genuine, ".")

	claims := appleClaims()
	claims["expiresDate"] = time.Now().Add(3650 * 24 * time.Hour).UnixMilli()
	tampered, _ := json.Marshal(claims)

	forged := parts[0] + "." + base64.RawURLEncoding.EncodeToString(tampered) + "." + parts[2]
	if got, err := verifier.Verify(context.Background(), "apple", forged); err == nil && got.Valid {
		t.Fatal("a tampered payload verified - the signature is not bound to the payload")
	}
}

// A real signature that chains to a different CA must be refused. Without this,
// an attacker who can obtain any certificate would be able to mint receipts.
func TestAppleVerifierRejectsAChainFromAnotherCA(t *testing.T) {
	pinned := newAppleTestPKI(t)
	rogue := newAppleTestPKI(t)
	rogue.root.Subject.CommonName = "Rogue Root CA"

	verifier := newAppleVerifier(t, pinned, nil)
	forged := signAppleJWS(t, rogue.leafKey, rogue.chain(), appleClaims())

	got, err := verifier.Verify(context.Background(), "apple", forged)
	if err == nil && got.Valid {
		t.Fatal("a receipt signed by an unrelated CA was accepted - the trust anchor is not being enforced")
	}
	if err == nil {
		t.Fatal("expected an error for a chain that does not reach the pinned root")
	}
}

// Algorithm confusion: the header names the algorithm, so the header must never
// be trusted. "none" is a valid JWS algorithm and must be refused, and an
// HS256 token signed with the leaf's public key (the class of bug that broke
// several JWT libraries) must be refused because only ES256 is accepted.
func TestAppleVerifierRejectsAlgorithmSubstitution(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, nil)

	x5c := []string{base64.StdEncoding.EncodeToString(pki.leaf.Raw)}
	payload, _ := json.Marshal(appleClaims())

	for _, alg := range []string{"none", "HS256", "ES256K", "es256", "RS256", ""} {
		header, _ := json.Marshal(map[string]any{"alg": alg, "x5c": x5c})
		token := base64.RawURLEncoding.EncodeToString(header) + "." +
			base64.RawURLEncoding.EncodeToString(payload) + ".AAAA"
		if got, err := verifier.Verify(context.Background(), "apple", token); err == nil && got.Valid {
			t.Errorf("alg=%q was accepted", alg)
		}
	}
}

// A header with no certificates cannot be verified, and must not fall back to
// trusting the payload.
func TestAppleVerifierRejectsAHeaderWithoutCertificates(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, nil)

	payload, _ := json.Marshal(appleClaims())
	for _, x5c := range [][]string{nil, {}, {"not-base64!!"}, {"bm90YWNlcnQ="}} {
		header, _ := json.Marshal(map[string]any{"alg": "ES256", "x5c": x5c})
		token := base64.RawURLEncoding.EncodeToString(header) + "." +
			base64.RawURLEncoding.EncodeToString(payload) + ".AAAA"
		if got, err := verifier.Verify(context.Background(), "apple", token); err == nil && got.Valid {
			t.Errorf("x5c=%v was accepted", x5c)
		}
	}
}

// A truncated signature must be rejected rather than left-padded into a
// different value by big.Int.SetBytes.
func TestAppleVerifierRejectsATruncatedSignature(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, nil)

	genuine := signAppleJWS(t, pki.leafKey, pki.chain(), appleClaims())
	parts := strings.Split(genuine, ".")
	raw, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	short := base64.RawURLEncoding.EncodeToString(raw[:40])
	token := parts[0] + "." + parts[1] + "." + short

	if got, err := verifier.Verify(context.Background(), "apple", token); err == nil && got.Valid {
		t.Fatal("a 40-byte signature was accepted as valid")
	}
}

// ---------------------------------------------------------------------------
// Claim-level rules
// ---------------------------------------------------------------------------

func TestAppleVerifierAcceptsAGenuineTransaction(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, nil)

	token := signAppleJWS(t, pki.leafKey, pki.chain(), appleClaims())
	got, err := verifier.Verify(context.Background(), "apple", token)
	if err != nil {
		t.Fatalf("genuine transaction was rejected: %v", err)
	}
	if !got.Valid {
		t.Fatalf("genuine transaction was not valid: %+v", got)
	}
	if got.PlanID != "monthly" {
		t.Errorf("plan = %q, want monthly", got.PlanID)
	}
	if got.TransactionID != testTransactionID || got.OriginalTransactionID != testTransactionID {
		t.Errorf("identifiers not captured: %+v", got)
	}
	if got.Environment != AppleEnvironmentSandbox {
		t.Errorf("environment = %q, want Sandbox", got.Environment)
	}
	if got.ExpiresAt == "" {
		t.Error("expiry not captured - the entitlement would never lapse")
	}
}

// An annual product must map to the annual plan rather than defaulting to the
// cheaper one, and the mapping must come from configuration rather than from a
// substring of the product id.
func TestAppleVerifierMapsProductsFromConfiguration(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, nil)

	claims := appleClaims()
	claims["productId"] = testAnnualSKU
	token := signAppleJWS(t, pki.leafKey, pki.chain(), claims)

	got, err := verifier.Verify(context.Background(), "apple", token)
	if err != nil || !got.Valid {
		t.Fatalf("annual transaction rejected: %v %+v", err, got)
	}
	if got.PlanID != "annual" {
		t.Errorf("plan = %q, want annual", got.PlanID)
	}
}

// An unmapped product is a configuration disagreement, not a purchase. It must
// not be guessed from the product id string.
func TestAppleVerifierRejectsAnUnmappedProduct(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, nil)

	claims := appleClaims()
	claims["productId"] = "app.iconfess.something.else"
	token := signAppleJWS(t, pki.leafKey, pki.chain(), claims)

	if got, err := verifier.Verify(context.Background(), "apple", token); err == nil && got.Valid {
		t.Fatal("an unmapped product granted a plan")
	}
}

// A receipt for another app must not grant entitlement here. The endpoint is
// public, so without this check any other app's customers are customers.
func TestAppleVerifierRejectsAnotherBundleID(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, nil)

	claims := appleClaims()
	claims["bundleId"] = "com.example.other"
	token := signAppleJWS(t, pki.leafKey, pki.chain(), claims)

	if got, err := verifier.Verify(context.Background(), "apple", token); err == nil && got.Valid {
		t.Fatal("a transaction for another bundle id was accepted")
	}
}

// Sandbox subscriptions are free, so accepting one in production is a way to
// obtain premium without paying.
func TestAppleVerifierRejectsSandboxInAProductionDeployment(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, func(c *AppleConfig) {
		c.Environment = AppleEnvironmentProduction
	})

	token := signAppleJWS(t, pki.leafKey, pki.chain(), appleClaims()) // environment: Sandbox
	if got, err := verifier.Verify(context.Background(), "apple", token); err == nil && got.Valid {
		t.Fatal("a Sandbox receipt was accepted by a Production deployment")
	}

	production := appleClaims()
	production["environment"] = AppleEnvironmentProduction
	fresh := signAppleJWS(t, pki.leafKey, pki.chain(), production)
	got, err := verifier.Verify(context.Background(), "apple", fresh)
	if err != nil || !got.Valid {
		t.Fatalf("Production receipt rejected by a Production deployment: %v %+v", err, got)
	}
}

func TestAppleVerifierRejectsUnknownEnvironment(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, nil)

	for _, env := range []string{"", "sandbox", "PRODUCTION", "Staging", "Xcode"} {
		claims := appleClaims()
		claims["environment"] = env
		token := signAppleJWS(t, pki.leafKey, pki.chain(), claims)
		if got, err := verifier.Verify(context.Background(), "apple", token); err == nil && got.Valid {
			t.Errorf("environment %q was accepted", env)
		}
	}
}

// An expired subscription is genuine but does not entitle anything. The
// distinction matters: the caller returns a different, more useful answer than
// "invalid receipt", and the entitlement must not be granted.
func TestAppleVerifierReportsAnExpiredSubscription(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, nil)

	claims := appleClaims()
	claims["expiresDate"] = time.Now().Add(-time.Hour).UnixMilli()
	token := signAppleJWS(t, pki.leafKey, pki.chain(), claims)

	got, err := verifier.Verify(context.Background(), "apple", token)
	if err != nil {
		t.Fatalf("expired but genuine transaction returned an error: %v", err)
	}
	if got.Valid {
		t.Fatal("an expired subscription was reported valid - entitlements never lapse")
	}
	if got.Detail == "" {
		t.Error("refusal carries no detail for the caller to report")
	}
}

// A refund must revoke access even while the expiry date is still in the
// future: a refunded annual subscription keeps its expiry.
func TestAppleVerifierRevokesARefundedTransaction(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, nil)

	claims := appleClaims()
	claims["revocationDate"] = time.Now().Add(-time.Hour).UnixMilli()
	claims["revocationReason"] = 0
	token := signAppleJWS(t, pki.leafKey, pki.chain(), claims)

	got, err := verifier.Verify(context.Background(), "apple", token)
	if err != nil {
		t.Fatalf("refunded transaction returned an error: %v", err)
	}
	if got.Valid {
		t.Fatal("a refunded transaction still granted premium")
	}
}

// A subscription with no expiry cannot be allowed: treating "absent" as
// "never expires" is a permanent free subscription.
func TestAppleVerifierRejectsAMissingExpiry(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, nil)

	for _, expires := range []any{nil, 0, float64(0)} {
		claims := appleClaims()
		if expires == nil {
			delete(claims, "expiresDate")
		} else {
			claims["expiresDate"] = expires
		}
		token := signAppleJWS(t, pki.leafKey, pki.chain(), claims)
		if got, err := verifier.Verify(context.Background(), "apple", token); err == nil && got.Valid {
			t.Errorf("expiresDate=%v was accepted", expires)
		}
	}
}

func TestAppleVerifierRejectsAnotherProvider(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleVerifier(t, pki, nil)
	token := signAppleJWS(t, pki.leafKey, pki.chain(), appleClaims())

	for _, provider := range []string{"google", "", "stripe"} {
		if got, err := verifier.Verify(context.Background(), provider, token); err == nil && got.Valid {
			t.Errorf("provider %q was accepted by the App Store verifier", provider)
		}
	}
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// A verifier that cannot decide must not be constructible: the alternative is a
// process that starts and then refuses every genuine purchase.
func TestNewAppleVerifierRefusesIncompleteConfiguration(t *testing.T) {
	pki := newAppleTestPKI(t)

	cases := map[string]AppleConfig{
		"no bundle id": {ProductPlans: map[string]string{testMonthlySKU: "monthly"}, Roots: pki.pool()},
		"no products":  {BundleID: testBundleID, Roots: pki.pool()},
		"bad environment": {BundleID: testBundleID, Environment: "prod",
			ProductPlans: map[string]string{testMonthlySKU: "monthly"}, Roots: pki.pool()},
	}
	for name, cfg := range cases {
		if _, err := NewAppleVerifier(cfg); err == nil {
			t.Errorf("%s: verifier was constructed", name)
		} else if !errors.Is(err, ErrUnconfigured) {
			t.Errorf("%s: error = %v, want ErrUnconfigured", name, err)
		}
	}
}

// The pinned root is the trust anchor for every Apple receipt. If the embedded
// bytes are wrong, verification must fail closed rather than trust whatever is
// there, so the bytes are asserted against a published fingerprint at load.
func TestPinnedAppleRootIsValidAndMatchesItsFingerprint(t *testing.T) {
	roots, err := AppleRoots()
	if err != nil {
		t.Fatalf("pinned Apple root failed to load: %v", err)
	}
	if roots == nil {
		t.Fatal("no roots returned")
	}

	// The constant must be a real SHA-256, not a placeholder.
	raw, err := hex.DecodeString(AppleRootCAG3SHA256Hex)
	if err != nil {
		t.Fatalf("pinned fingerprint is not hex: %v", err)
	}
	if len(raw) != sha256.Size {
		t.Fatalf("pinned fingerprint is %d bytes, want %d", len(raw), sha256.Size)
	}
}

// Extra trust anchors are additive, and a malformed bundle is refused rather
// than silently ignored.
func TestAppleRootsWithRejectsUnusableExtras(t *testing.T) {
	if _, err := AppleRootsWith([][]byte{[]byte("-----BEGIN CERTIFICATE-----\nnope\n-----END CERTIFICATE-----\n")}); err == nil {
		t.Fatal("a malformed extra root bundle was accepted")
	}
	if _, err := AppleRootsWith([][]byte{nil, {}}); err != nil {
		t.Fatalf("empty extras should be ignored, got %v", err)
	}
}

// The verifier must never widen its trust because a caller supplied a
// certificate: the pool is fixed at construction.
func TestAppleVerifierUsesOnlyItsConfiguredRoots(t *testing.T) {
	pinned := newAppleTestPKI(t)
	other := newAppleTestPKI(t)

	verifier := newAppleVerifier(t, pinned, nil)
	// A chain that includes the *other* CA's root in x5c must not be trusted
	// just because it is present in the header.
	token := signAppleJWS(t, other.leafKey, other.chain(), appleClaims())

	if got, err := verifier.Verify(context.Background(), "apple", token); err == nil && got.Valid {
		t.Fatal("a chain whose root was supplied in the header was trusted")
	}
}

// The chain is validated at the time Apple signed the payload, not at the time
// the request arrives. Apple's signing certificates rotate, and a receipt signed
// while one was valid stays valid afterwards - that is why the payload carries
// signedDate at all.
func TestAppleVerifierAcceptsAReceiptSignedWhileTheCertificateWasValid(t *testing.T) {
	// A signing leaf that expired an hour ago.
	// Every certificate in the chain is checked at the signing time, so they all
	// have to have been valid then: a real rotation expires the signing
	// certificate while the CA above it lives on.
	root, rootKey := issueCert(t, certSpec{
		commonName: "iCONFESS Test Root CA", curve: elliptic.P384(), isCA: true,
		notBefore: time.Now().Add(-6 * time.Hour),
	})
	intermediate, intKey := issueCert(t, certSpec{
		commonName: "iCONFESS Test WWDR", curve: elliptic.P384(), isCA: true,
		parent: root, parentKey: rootKey,
		notBefore: time.Now().Add(-6 * time.Hour),
	})
	leaf, leafKey := issueCert(t, certSpec{
		commonName: "iCONFESS Rotated Signing Leaf", curve: elliptic.P256(),
		parent: intermediate, parentKey: intKey,
		notBefore: time.Now().Add(-3 * time.Hour),
		notAfter:  time.Now().Add(-time.Hour),
	})
	stale := &appleTestPKI{root: root, rootKey: rootKey, intermediate: intermediate, intKey: intKey, leaf: leaf, leafKey: leafKey}

	verifier := newAppleVerifier(t, stale, nil)
	claims := appleClaims()
	claims["signedDate"] = time.Now().Add(-2 * time.Hour).UnixMilli() // inside the leaf's window
	claims["expiresDate"] = time.Now().Add(30 * 24 * time.Hour).UnixMilli()

	got, err := verifier.Verify(context.Background(), "apple", signAppleJWS(t, leafKey, stale.chain(), claims))
	if err != nil {
		t.Fatalf("a receipt signed while the certificate was valid was refused: %v", err)
	}
	if !got.Valid {
		t.Fatal("a receipt signed while the certificate was valid did not grant")
	}

	// The same expired leaf with a signedDate after it expired must be refused:
	// the window is what makes the rule safe, not a blanket exemption for
	// expired certificates.
	claims["signedDate"] = time.Now().UnixMilli()
	if got, err := verifier.Verify(context.Background(), "apple", signAppleJWS(t, leafKey, stale.chain(), claims)); err == nil && got.Valid {
		t.Fatal("a receipt claimed to be signed after its certificate expired")
	}
}

// signedDate only chooses the clock for the certificate check. It must not be
// able to move that clock forward: a payload claiming to be signed in the future
// is validated against the current time.
func TestSigningTimeClampsFutureTimestamps(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	future, _ := json.Marshal(map[string]any{"signedDate": now.Add(72 * time.Hour).UnixMilli()})
	if got := signingTime(future, now); !got.Equal(now) {
		t.Errorf("signingTime(future) = %s, want the current clock %s", got, now)
	}

	past, _ := json.Marshal(map[string]any{"signedDate": now.Add(-48 * time.Hour).UnixMilli()})
	if got := signingTime(past, now); !got.Equal(now.Add(-48 * time.Hour)) {
		t.Errorf("signingTime(past) = %s, want %s", got, now.Add(-48*time.Hour))
	}

	for _, payload := range [][]byte{
		[]byte(`{}`),
		[]byte(`not json`),
		[]byte(`{"signedDate":null}`),
	} {
		if got := signingTime(payload, now); !got.Equal(now) {
			t.Errorf("signingTime(%s) = %s, want the current clock %s", payload, got, now)
		}
	}
}
