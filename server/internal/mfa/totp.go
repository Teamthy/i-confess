// Package mfa implements time-based one-time passwords and recovery codes
// (§41, §42, §81).
//
// TOTP (RFC 6238) is implemented directly rather than pulled from a dependency:
// it is HMAC-SHA1 over a time counter, about forty lines, and the algorithm has
// not changed since 2011. The tests below verify it against the RFC's published
// vectors, which is stronger assurance than trusting an unaudited module.
package mfa

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// Period is the TOTP step. 30 seconds is what every authenticator app
	// assumes; changing it would silently break them all.
	Period = 30 * time.Second
	// Digits in a generated code.
	Digits = 6
	// Skew is how many steps either side of now are accepted.
	//
	// One step (±30s) tolerates ordinary clock drift between a phone and the
	// server. Widening it multiplies the guess space an attacker gets per
	// window, so it stays at one.
	Skew = 1
)

var (
	// ErrInvalidCode is returned for a wrong or reused code.
	ErrInvalidCode = errors.New("invalid verification code")
	// ErrInvalidSecret is returned when a stored secret cannot be decoded.
	ErrInvalidSecret = errors.New("invalid secret")
)

// b32 is unpadded base32, the encoding authenticator apps expect.
var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret creates a new TOTP secret.
//
// 20 bytes is the RFC 4226 recommendation and matches SHA-1's block usage;
// longer secrets gain nothing against this construction.
func GenerateSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return b32.EncodeToString(buf), nil
}

// Code computes the TOTP for a secret at a point in time.
func Code(secret string, t time.Time) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", ErrInvalidSecret
	}
	return hotp(key, uint64(t.Unix())/uint64(Period.Seconds())), nil
}

// Verify checks a code against a secret, allowing for clock skew.
//
// It returns the matched counter so the caller can reject replays: a code stays
// valid for its whole window, and without remembering the last accepted counter
// an attacker who observes one code can reuse it within the same 30 seconds.
func Verify(secret, code string, at time.Time) (counter uint64, ok bool) {
	code = strings.TrimSpace(code)
	if len(code) != Digits {
		return 0, false
	}
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return 0, false
	}

	current := uint64(at.Unix()) / uint64(Period.Seconds())
	for delta := -Skew; delta <= Skew; delta++ {
		c := current + uint64(delta)
		// Constant-time compare: a byte-by-byte early exit would leak how much
		// of a guessed code was right.
		if subtle.ConstantTimeCompare([]byte(hotp(key, c)), []byte(code)) == 1 {
			return c, true
		}
	}
	return 0, false
}

// hotp implements RFC 4226 truncation.
func hotp(key []byte, counter uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	// Dynamic truncation: the low nibble of the last byte selects the offset.
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])

	mod := uint32(1)
	for i := 0; i < Digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", Digits, value%mod)
}

// ProvisioningURI builds the otpauth:// URI an authenticator app scans.
//
// The issuer appears twice — as a label prefix and as a parameter — because
// older apps read only one or the other, and getting it wrong means the entry
// shows up unlabelled in a list of dozens.
func ProvisioningURI(secret, accountName, issuer string) string {
	label := url.PathEscape(issuer + ":" + accountName)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", Digits))
	q.Set("period", fmt.Sprintf("%d", int(Period.Seconds())))
	return "otpauth://totp/" + label + "?" + q.Encode()
}
