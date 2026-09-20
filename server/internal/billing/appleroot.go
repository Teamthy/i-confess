package billing

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
)

// Apple Root CA - G3, the trust anchor for every JWS Apple signs for the App
// Store: StoreKit 2 transactions, App Store Server API responses and App Store
// Server Notifications V2. Downloaded from
// https://www.apple.com/certificateauthority/AppleRootCA-G3.cer
//
// The chain in a JWS header's x5c is leaf -> WWDR intermediate -> this root.
// Verifying a receipt means verifying that chain, and that is only meaningful
// against a pinned root: a signature check against a certificate the attacker
// also supplied proves nothing. This is the single most important line of
// defence in the whole billing path, which is why the bytes are compiled in
// rather than fetched at boot. A verifier that fetched its trust anchor over
// the network would fail open the first time DNS lied to it.
const appleRootCAG3PEM = `-----BEGIN CERTIFICATE-----
MIICQzCCAcmgAwIBAgIILcX8iNLFS5UwCgYIKoZIzj0EAwMwZzEbMBkGA1UEAwwS
QXBwbGUgUm9vdCBDQSAtIEczMSYwJAYDVQQLDB1BcHBsZSBDZXJ0aWZpY2F0aW9u
IEF1dGhvcml0eTETMBEGA1UECgwKQXBwbGUgSW5jLjELMAkGA1UEBhMCVVMwHhcN
MTQwNDMwMTgxOTA2WhcNMzkwNDMwMTgxOTA2WjBnMRswGQYDVQQDDBJBcHBsZSBS
b290IENBIC0gRzMxJjAkBgNVBAsMHUFwcGxlIENlcnRpZmljYXRpb24gQXV0aG9y
aXR5MRMwEQYDVQQKDApBcHBsZSBJbmMuMQswCQYDVQQGEwJVUzB2MBAGByqGSM49
AgEGBSuBBAAiA2IABJjpLz1AcqTtkyJygRMc3RCV8cWjTnHcFBbZDuWmBSp3ZHtf
TjjTuxxEtX/1H7YyYl3J6YRbTzBPEVoA/VhYDKX1DyxNB0cTddqXl5dvMVztK517
IDvYuVTZXpmkOlEKMaNCMEAwHQYDVR0OBBYEFLuw3qFYM4iapIqZ3r6966/ayySr
MA8GA1UdEwEB/wQFMAMBAf8wDgYDVR0PAQH/BAQDAgEGMAoGCCqGSM49BAMDA2gA
MGUCMQCD6cHEFl4aXTQY2e3v9GwOAEZLuN+yRhHFD/3meoyhpmvOwgPUnPWTxnS4
at+qIxUCMG1mihDK1A3UT82NQz60imOlM27jbdoXt2QfyFMm+YhidDkLF1vLUagM
6BgD56KyKA==
-----END CERTIFICATE-----
`

// AppleRootCAG3SHA256Hex is the SHA-256 fingerprint of the certificate above,
// lowercase hex. It is asserted at load time.
//
// The pin makes the embedded bytes self-checking: if this file is ever
// regenerated, re-encoded or truncated, verification fails closed and loudly
// instead of trusting whatever bytes happen to be there. Apple publishes this
// value (63:34:3A:BF:...:91:79) and it is what the reference implementations in
// other languages compare against.
const AppleRootCAG3SHA256Hex = "63343abfb89a6a03ebb57e9b3f5fa7be7c4f5c756f3017b3a8c488c3653e9179"

// Expected subject fields, checked alongside the fingerprint. Two independent
// assertions of the same identity: a substituted certificate would have to
// match both the hash and the name. Compared field by field rather than through
// pkix.Name.String(), whose rendering is a formatting detail this package
// should not depend on.
const (
	AppleRootCAG3CommonName         = "Apple Root CA - G3"
	AppleRootCAG3Organization       = "Apple Inc."
	AppleRootCAG3OrganizationalUnit = "Apple Certification Authority"
	AppleRootCAG3Country            = "US"
)

// AppleRoots returns the pinned Apple trust anchors.
//
// Callers get a fresh pool, so a caller that adds certificates cannot affect
// another caller's verification.
func AppleRoots() (*x509.CertPool, error) {
	return AppleRootsWith(nil)
}

// AppleRootsWith builds the trust pool from the pinned Apple root plus any
// extra PEM blobs.
//
// The extras exist for key rotation and for tests that need to stand up their
// own certificate authority. They are additional anchors, never replacements:
// nothing can remove the pinned root, because a configuration mistake that
// dropped Apple from the trust store would turn every genuine purchase into a
// rejection — a revenue outage rather than a security one, but one that a
// single env var should not be able to cause.
func AppleRootsWith(extraPEM [][]byte) (*x509.CertPool, error) {
	block, _ := pem.Decode([]byte(appleRootCAG3PEM))
	if block == nil {
		return nil, fmt.Errorf("billing: embedded Apple root CA is not valid PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("billing: parse embedded Apple root CA: %w", err)
	}
	sum := sha256.Sum256(cert.Raw)
	if got := hex.EncodeToString(sum[:]); got != AppleRootCAG3SHA256Hex {
		return nil, fmt.Errorf(
			"billing: embedded Apple root CA has SHA-256 %s, expected %s - refusing to trust it",
			got, AppleRootCAG3SHA256Hex)
	}
	if !cert.IsCA {
		return nil, fmt.Errorf("billing: embedded Apple root CA is not a CA certificate")
	}
	if err := assertAppleIdentity(cert); err != nil {
		return nil, err
	}

	pool := x509.NewCertPool()
	pool.AddCert(cert)

	for _, extra := range extraPEM {
		if len(extra) == 0 {
			continue
		}
		if !pool.AppendCertsFromPEM(extra) {
			return nil, fmt.Errorf("billing: extra root bundle contains no usable certificate")
		}
	}
	return pool, nil
}

// assertAppleIdentity checks the subject, issuer and self-signature of the
// pinned certificate.
//
// The fingerprint above is the decisive check; these are the readable ones. A
// certificate that hashes to the pin is Apple's by definition, so the value
// here is in what a failure says: "the embedded root is not Apple's" is a
// sentence an operator can act on, where a hex mismatch is not.
func assertAppleIdentity(cert *x509.Certificate) error {
	first := func(v []string) string {
		if len(v) == 0 {
			return ""
		}
		return v[0]
	}
	if cert.Subject.CommonName != AppleRootCAG3CommonName ||
		first(cert.Subject.Organization) != AppleRootCAG3Organization {
		return fmt.Errorf("billing: embedded Apple root CA subject is not %q (%s)",
			AppleRootCAG3CommonName, cert.Subject.String())
	}
	if first(cert.Subject.OrganizationalUnit) != AppleRootCAG3OrganizationalUnit {
		return fmt.Errorf("billing: embedded Apple root CA unit is not %q (%s)",
			AppleRootCAG3OrganizationalUnit, cert.Subject.String())
	}
	if first(cert.Subject.Country) != AppleRootCAG3Country {
		return fmt.Errorf("billing: embedded Apple root CA country is not %q (%s)",
			AppleRootCAG3Country, cert.Subject.String())
	}
	// A trust anchor that does not verify against itself is not a trust anchor.
	if err := cert.CheckSignatureFrom(cert); err != nil {
		return fmt.Errorf("billing: embedded Apple root CA is not self-signed: %w", err)
	}
	return nil
}
