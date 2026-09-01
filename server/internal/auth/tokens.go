package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// One-time tokens for email verification and password reset (§15, §16, §34).
//
// Two rules, both of which the previous implementation broke:
//
//  1. Tokens must be unpredictable. A token derived from the user id — such as
//     "reset-"+userID — is guessable by anyone who learns an id, which turns
//     password reset into an account-takeover primitive.
//  2. Only a hash of the token is stored. A leaked database backup or a SQL
//     injection must not yield working reset links, exactly as with passwords.

// tokenBytes is the entropy per token. 32 bytes is far beyond brute-force
// reach and keeps the URL-safe encoding a reasonable length.
const tokenBytes = 32

// NewOneTimeToken returns the plaintext token to send to the user and the hash
// to persist. The plaintext is never stored.
func NewOneTimeToken() (plaintext, hash string, err error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		// A failing CSPRNG must abort the operation, never fall back to a
		// weaker source.
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	plaintext = base64.RawURLEncoding.EncodeToString(buf)
	return plaintext, HashToken(plaintext), nil
}

// HashToken hashes a one-time token for storage and lookup.
//
// A plain SHA-256 is correct here, unlike for passwords: the input already has
// 256 bits of entropy, so there is nothing to brute-force and no need for a
// slow KDF on a hot verification path.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// TokensEqual compares token hashes in constant time, so a timing oracle
// cannot be used to recover a valid token byte by byte.
func TokensEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
