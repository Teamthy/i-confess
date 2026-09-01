package oauth

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
)

// verifyRS256 checks an RS256 JWT signature over the signing input.
//
// Split out so the cryptographic step is one small, auditable function: the
// signing input is exactly "header.payload" as it appeared in the token, never
// a re-encoding, because re-serialising JSON would change the bytes that were
// actually signed.
func verifyRS256(pub *rsa.PublicKey, signingInput, signatureB64 string) error {
	sig, err := base64.RawURLEncoding.DecodeString(signatureB64)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(signingInput))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig)
}
