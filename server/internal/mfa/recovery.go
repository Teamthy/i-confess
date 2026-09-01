package mfa

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

// Recovery codes (§41, §81).
//
// Recovery is the part of MFA people actually need: phones are lost far more
// often than accounts are attacked. It must work without the second factor and
// without weakening it — which rules out "email us and we'll turn it off",
// because that silently reduces MFA to the security of an inbox.

// RecoveryCodeCount is how many codes are issued at enrolment.
const RecoveryCodeCount = 10

// recoveryAlphabet excludes characters people misread when copying by hand:
// 0/O, 1/I/l. Recovery codes get transcribed from paper under stress.
const recoveryAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// GenerateRecoveryCodes returns plaintext codes to show the user once, and
// their hashes to store.
//
// Only hashes are persisted, exactly as with passwords: a leaked database must
// not yield working second factors.
func GenerateRecoveryCodes() (plaintext []string, hashes []string, err error) {
	for i := 0; i < RecoveryCodeCount; i++ {
		code, err := randomRecoveryCode()
		if err != nil {
			return nil, nil, err
		}
		plaintext = append(plaintext, code)
		hashes = append(hashes, HashRecoveryCode(code))
	}
	return plaintext, hashes, nil
}

// randomRecoveryCode produces a grouped code like "ABCD-EFGH-JKMN".
func randomRecoveryCode() (string, error) {
	const groups, size = 3, 4
	buf := make([]byte, groups*size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate recovery code: %w", err)
	}

	var b strings.Builder
	for i, v := range buf {
		if i > 0 && i%size == 0 {
			b.WriteByte('-')
		}
		// Modulo bias is negligible here: 256 % 31 skews the distribution by
		// under 3%, against a 31^12 space. Rejection sampling would be more
		// correct but adds a retry loop for no practical gain.
		b.WriteByte(recoveryAlphabet[int(v)%len(recoveryAlphabet)])
	}
	return b.String(), nil
}

// NormaliseRecoveryCode makes comparison forgiving of how a code was typed.
//
// Users retype these from paper, so case and separators must not matter; the
// entropy is in the characters, not the formatting.
func NormaliseRecoveryCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(code) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// HashRecoveryCode hashes a code for storage.
//
// Plain SHA-256 rather than bcrypt: the input already carries ~59 bits of
// entropy from a random alphabet, so there is nothing to brute-force and no
// reason to make verification slow.
func HashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(NormaliseRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}

// MatchRecoveryCode finds a supplied code among stored hashes in constant time.
//
// Every candidate is compared even after a match, so the time taken does not
// reveal the position of the matching code.
func MatchRecoveryCode(code string, hashes []string) (index int, ok bool) {
	want := HashRecoveryCode(code)
	index, ok = -1, false
	for i, h := range hashes {
		if subtle.ConstantTimeCompare([]byte(h), []byte(want)) == 1 {
			index, ok = i, true
		}
	}
	return index, ok
}
