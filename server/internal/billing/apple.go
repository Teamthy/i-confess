package billing

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
)

// Apple App Store receipt verification (IC-003).
//
// What this file replaces, and why it is written the way it is: the previous
// production verifier split the receipt on ".", base64-decoded the middle
// segment and trusted the claims inside it. It never looked at the header, the
// signature, or the certificate chain. Anyone who could base64-encode JSON
// could grant themselves premium in production by sending
//
//	eyJhbGciOiJFUzI1NiJ9.<base64 of {"productId":"...","expiresDate":<future>}>.x
//
// A receipt is an assertion by the client. Nothing in it may be believed until
// Apple's signature over it has been checked, and that check is only meaningful
// against a trust anchor the client cannot supply — here, the pinned Apple Root
// CA - G3 in appleroot.go.
//
// The verification order below is deliberate: chain, then signature, then
// claims. Reading claims before the signature is verified is the bug this file
// exists to fix, even when the claims are only used for logging.

// AppleEnvironments are the values Apple puts in the JWS "environment" claim.
// A sandbox receipt must never grant production entitlement: sandbox
// subscriptions are free, so accepting one is the same class of hole as
// accepting an unsigned payload.
const (
	AppleEnvironmentSandbox    = "Sandbox"
	AppleEnvironmentProduction = "Production"
)

// AppleConfig configures the App Store verifier.
type AppleConfig struct {
	// BundleID is the app the transaction must belong to. Required: without it
	// a receipt from any other app that posts to this endpoint would be
	// accepted, and the endpoint is reachable by anyone.
	BundleID string
	// Environment restricts which store environment is accepted. Empty accepts
	// either Sandbox or Production, which is only appropriate in development.
	Environment string
	// ProductPlans maps an App Store product identifier to a plan in this
	// server's catalogue. A product that is not in the map does not grant
	// anything: an unmapped product means the catalogue and the store
	// configuration disagree, and guessing a plan from a substring would turn
	// that mistake into free premium.
	ProductPlans map[string]string
	// Roots overrides the trust anchors. Nil means the pinned Apple root.
	// Tests use this to stand up their own certificate authority.
	Roots *x509.CertPool
	// Now is injectable for tests.
	Now func() time.Time
}

func (c AppleConfig) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

// AppleVerifier verifies StoreKit 2 / App Store Server API signed transactions.
type AppleVerifier struct {
	cfg AppleConfig
}

// NewAppleVerifier builds a verifier, validating that it can decide anything at
// all. A verifier constructed without a bundle id or a product map would reject
// every genuine receipt at runtime; failing at construction means the process
// does not start rather than silently refusing every purchase.
func NewAppleVerifier(cfg AppleConfig) (*AppleVerifier, error) {
	if strings.TrimSpace(cfg.BundleID) == "" {
		return nil, fmt.Errorf("%w: APPLE_BUNDLE_ID is required", ErrUnconfigured)
	}
	if len(cfg.ProductPlans) == 0 {
		return nil, fmt.Errorf("%w: no App Store product ids are mapped to plans", ErrUnconfigured)
	}
	switch cfg.Environment {
	case "", AppleEnvironmentSandbox, AppleEnvironmentProduction:
	default:
		return nil, fmt.Errorf("%w: unknown Apple environment %q", ErrUnconfigured, cfg.Environment)
	}
	if cfg.Roots == nil {
		roots, err := AppleRoots()
		if err != nil {
			return nil, err
		}
		cfg.Roots = roots
	}
	return &AppleVerifier{cfg: cfg}, nil
}

// appleJWSHeader is the subset of the JOSE header that matters. alg is checked
// explicitly: accepting the algorithm named in the token is the classic
// algorithm-confusion bug, and "none" is a valid JWS algorithm.
type appleJWSHeader struct {
	Alg string   `json:"alg"`
	X5c []string `json:"x5c"`
}

// appleTransaction is the decoded JWSTransactionDecodedPayload.
//
// Pointer fields distinguish "absent" from "zero": an absent expiresDate and an
// expiresDate of 0 are different facts, and a subscription without an expiry
// must not be treated as one that never expires.
type appleTransaction struct {
	TransactionID         string `json:"transactionId"`
	OriginalTransactionID string `json:"originalTransactionId"`
	BundleID              string `json:"bundleId"`
	ProductID             string `json:"productId"`
	SubscriptionGroupID   string `json:"subscriptionGroupIdentifier"`
	Type                  string `json:"type"`
	InAppOwnershipType    string `json:"inAppOwnershipType"`
	Environment           string `json:"environment"`
	ExpiresDate           *int64 `json:"expiresDate"`
	RevocationDate        *int64 `json:"revocationDate"`
	RevocationReason      *int64 `json:"revocationReason"`
	// SignedDate is when Apple signed this payload, in milliseconds. It is not
	// an entitlement: it says which clock the certificate chain is valid
	// against.
	SignedDate *int64 `json:"signedDate"`
}

// Verify implements Verifier.
func (v *AppleVerifier) Verify(_ context.Context, provider, receipt string) (Verification, error) {
	if p := strings.ToLower(strings.TrimSpace(provider)); p != "apple" {
		return Verification{}, fmt.Errorf("%w: provider %q is not handled by the App Store verifier",
			ErrInvalidReceipt, provider)
	}

	parts := strings.Split(strings.TrimSpace(receipt), ".")
	if len(parts) != 3 {
		return Verification{}, fmt.Errorf("%w: apple receipt is not a three-part JWS", ErrInvalidReceipt)
	}

	header, err := decodeAppleHeader(parts[0])
	if err != nil {
		return Verification{}, err
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Verification{}, fmt.Errorf("%w: apple payload is not base64url", ErrInvalidReceipt)
	}

	// The signature is checked against the leaf certificate, and the leaf is
	// only trustworthy if it chains to Apple. Both steps happen before a single
	// claim is believed.
	//
	// The chain is validated at the time Apple signed the payload rather than
	// at the time this request arrives, because Apple's signing certificates
	// rotate: a receipt signed while a certificate was valid stays valid after
	// it expires, and Apple's own libraries do the same. Reading signedDate
	// here does not trust the payload - it only chooses which clock to check
	// certificates against. The signature below still has to be correct before
	// anything in the payload is used, and a forged signedDate inside a payload
	// with an invalid signature never gets that far.
	leaf, err := v.verifyChain(header.X5c, signingTime(payload, v.cfg.now()))
	if err != nil {
		return Verification{}, err
	}
	if err := verifyJWSES256(leaf, parts[0], parts[1], parts[2]); err != nil {
		return Verification{}, err
	}

	var tx appleTransaction
	if err := json.Unmarshal(payload, &tx); err != nil {
		return Verification{}, fmt.Errorf("%w: apple payload is not a transaction", ErrInvalidReceipt)
	}

	return v.evaluate(tx)
}

// signingTime returns the time Apple signed a payload, for use as the clock the
// certificate chain is validated against.
//
// A future timestamp is ignored in favour of the current clock: the chain must
// not be validated against a time the client invented, and Apple's certificates
// are all valid now, so a payload signed in the future is either a clock skew on
// Apple's side or a forgery that will fail signature verification anyway.
// Anything unparsable falls back to the current time, which is the stricter
// choice.
func signingTime(payload []byte, now time.Time) time.Time {
	var claims struct {
		SignedDate *int64 `json:"signedDate"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.SignedDate == nil {
		return now
	}
	signed := time.UnixMilli(*claims.SignedDate).UTC()
	if signed.After(now) {
		return now
	}
	return signed
}

// decodeAppleHeader parses and validates the JOSE header.
func decodeAppleHeader(encoded string) (appleJWSHeader, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return appleJWSHeader{}, fmt.Errorf("%w: apple JWS header is not base64url", ErrInvalidReceipt)
	}
	var header appleJWSHeader
	if err := json.Unmarshal(raw, &header); err != nil {
		return appleJWSHeader{}, fmt.Errorf("%w: apple JWS header is not JSON", ErrInvalidReceipt)
	}
	// Exact match, not a prefix test: "ES256K" and "ES256" are different
	// algorithms, and a case-insensitive comparison would let "NO NE" through
	// as a variant of a real one.
	if header.Alg != "ES256" {
		return appleJWSHeader{}, fmt.Errorf("%w: apple JWS alg is %q, want ES256", ErrInvalidReceipt, header.Alg)
	}
	if len(header.X5c) < 2 {
		return appleJWSHeader{}, fmt.Errorf(
			"%w: apple JWS carries %d certificates, want at least a leaf and an intermediate",
			ErrInvalidReceipt, len(header.X5c))
	}
	return header, nil
}

// verifyChain parses x5c and verifies the leaf against the pinned roots.
func (v *AppleVerifier) verifyChain(x5c []string, at time.Time) (*x509.Certificate, error) {
	return verifyAppleChain(x5c, v.cfg.Roots, at)
}

// verifyAppleChain verifies a certificate chain against a trust pool.
//
// Package-level and parameterised by the roots because App Store Server
// Notifications are signed the same way receipts are, by the same chain, and
// must be checked against the same pinned root. Sharing the function is what
// guarantees the notification path cannot be configured with a weaker trust
// anchor than the receipt path.
func verifyAppleChain(x5c []string, roots *x509.CertPool, at time.Time) (*x509.Certificate, error) {
	certs := make([]*x509.Certificate, 0, len(x5c))
	for i, encoded := range x5c {
		der, err := decodeX5cEntry(encoded)
		if err != nil {
			return nil, fmt.Errorf("%w: apple x5c[%d] is not a certificate: %v", ErrInvalidReceipt, i, err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("%w: apple x5c[%d] is not a certificate", ErrInvalidReceipt, i)
		}
		certs = append(certs, cert)
	}

	// Apple's x5c is leaf, WWDR intermediate, root. Putting the whole tail in
	// the intermediates pool is correct whether or not the root is included:
	// Go considers the roots and the intermediates independently, and a
	// certificate present in both terminates the chain as a trust anchor.
	intermediates := x509.NewCertPool()
	for _, c := range certs[1:] {
		intermediates.AddCert(c)
	}

	// KeyUsages is ExtKeyUsageAny because Apple's signing certificates do not
	// carry an extended key usage for TLS, and Go's default (ServerAuth) would
	// reject every genuine receipt. What is asserted here is "this certificate
	// chains to Apple", which the signatures along the chain establish; what the
	// certificate is used for is asserted by the JWS signature that follows.
	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		CurrentTime:   at,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}
	if _, err := certs[0].Verify(opts); err != nil {
		return nil, fmt.Errorf("%w: apple certificate chain does not verify against the pinned root: %v",
			ErrInvalidReceipt, err)
	}
	return certs[0], nil
}

// decodeX5cEntry accepts both padded and unpadded base64. RFC 7515 requires
// standard base64 with padding for x5c; real-world encoders differ, so both are
// accepted, but only as encodings of the same bytes.
func decodeX5cEntry(encoded string) ([]byte, error) {
	if der, err := base64.StdEncoding.DecodeString(encoded); err == nil {
		return der, nil
	}
	return base64.RawStdEncoding.DecodeString(strings.TrimRight(encoded, "="))
}

// verifyJWSES256 checks the JWS signature over "header.payload".
func verifyJWSES256(leaf *x509.Certificate, header, payload, signature string) error {
	pub, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("%w: apple signing certificate does not hold an ECDSA key", ErrInvalidReceipt)
	}
	// ES256 is defined over P-256. A P-384 key in a chain that verifies is
	// still not a token signed the way this code believes it is.
	if pub.Curve != elliptic.P256() {
		return fmt.Errorf("%w: apple signing key is not P-256", ErrInvalidReceipt)
	}

	sig, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("%w: apple JWS signature is not base64url", ErrInvalidReceipt)
	}
	// JWS ECDSA signatures are the fixed-width concatenation r||s, not DER.
	// A short slice would be silently left-padded by big.Int.SetBytes, turning
	// a truncated signature into a different valid-looking one.
	if len(sig) != 64 {
		return fmt.Errorf("%w: apple JWS signature is %d bytes, want 64", ErrInvalidReceipt, len(sig))
	}

	digest := sha256.Sum256([]byte(header + "." + payload))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return fmt.Errorf("%w: apple JWS signature does not match the payload", ErrInvalidReceipt)
	}
	return nil
}

// evaluate applies the transaction's claims to this server's catalogue.
//
// Reaching this point means the payload is genuinely Apple's. What remains is
// whether it entitles this user to anything: the right app, the right store
// environment, a product this server knows, no refund, and time left on the
// clock.
func (v *AppleVerifier) evaluate(tx appleTransaction) (Verification, error) {
	if tx.BundleID != v.cfg.BundleID {
		return Verification{}, fmt.Errorf("%w: apple transaction is for bundle %q, not %q",
			ErrInvalidReceipt, tx.BundleID, v.cfg.BundleID)
	}

	switch tx.Environment {
	case AppleEnvironmentSandbox, AppleEnvironmentProduction:
	default:
		return Verification{}, fmt.Errorf("%w: apple transaction has unknown environment %q",
			ErrInvalidReceipt, tx.Environment)
	}
	if v.cfg.Environment != "" && tx.Environment != v.cfg.Environment {
		return Verification{}, fmt.Errorf("%w: apple transaction is from %s but this deployment accepts %s",
			ErrInvalidReceipt, tx.Environment, v.cfg.Environment)
	}

	if tx.TransactionID == "" || tx.OriginalTransactionID == "" {
		return Verification{}, fmt.Errorf("%w: apple transaction is missing an identifier", ErrInvalidReceipt)
	}

	plan, mapped := v.cfg.ProductPlans[tx.ProductID]
	if !mapped {
		return Verification{}, fmt.Errorf("%w: App Store product %q is not mapped to a plan on this server",
			ErrInvalidReceipt, tx.ProductID)
	}

	// Apple omits inAppOwnershipType on some older payloads; when present it
	// must be a direct purchase or a family share. Anything else (a legacy
	// educational or volume-purchase type, say) is not a subscription this
	// server sold.
	switch tx.InAppOwnershipType {
	case "", "PURCHASED", "FAMILY_SHARED":
	default:
		return Verification{}, fmt.Errorf("%w: apple transaction has ownership type %q",
			ErrInvalidReceipt, tx.InAppOwnershipType)
	}

	base := Verification{
		Provider:              "apple",
		TransactionID:         tx.TransactionID,
		OriginalTransactionID: tx.OriginalTransactionID,
		ProductID:             tx.ProductID,
		Environment:           tx.Environment,
	}

	// A refund or a revocation is a decision Apple has already made. It matters
	// even when the expiry date is still in the future: a refunded annual
	// subscription keeps its expiry date, and watching only the clock would
	// leave the entitlement active for the rest of the year.
	if tx.RevocationDate != nil && *tx.RevocationDate > 0 {
		base.Valid = false
		base.State = models.SubscriptionRefunded
		base.Detail = "apple: transaction was revoked or refunded"
		if tx.RevocationReason != nil {
			base.Detail = fmt.Sprintf("apple: transaction was revoked (reason %d)", *tx.RevocationReason)
		}
		return base, nil
	}

	if tx.ExpiresDate == nil || *tx.ExpiresDate == 0 {
		return Verification{}, fmt.Errorf("%w: apple transaction for %q carries no expiry",
			ErrInvalidReceipt, tx.ProductID)
	}
	expires := time.UnixMilli(*tx.ExpiresDate).UTC()
	if !expires.After(v.cfg.now()) {
		base.Valid = false
		base.State = models.SubscriptionExpired
		base.ExpiresAt = expires.Format(time.RFC3339)
		base.Detail = "apple: subscription has expired"
		return base, nil
	}

	base.Valid = true
	base.State = models.SubscriptionActive
	base.PlanID = plan
	base.ExpiresAt = expires.Format(time.RFC3339)
	base.Detail = "apple: verified StoreKit 2 transaction " + tx.TransactionID
	return base, nil
}
