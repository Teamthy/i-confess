package auth

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Password policy, per NIST SP 800-63B.
//
// The guidance is deliberately counter-intuitive and worth stating, because the
// obvious approach makes security worse:
//
//   - Enforce a minimum length. Length is what actually resists guessing.
//   - Do NOT enforce composition rules. Requiring a symbol does not make a
//     password harder to guess; it makes people append "1!" to the same word
//     they would have used anyway, and every attacker's dictionary knows that.
//   - Do NOT force periodic rotation. It produces the same password with an
//     incremented digit, and teaches people to write them down.
//   - DO check against known-compromised values. This is the control that
//     removes the most real-world risk, because the passwords that get breached
//     are overwhelmingly the ones everyone already guessed.
//
// Before this file the only check in the codebase was "len(password) < 8", in
// two places, which accepted "password" and "12345678".

const (
	// MinPasswordLength is the floor. Eight is NIST's minimum; longer is
	// always accepted and encouraged by the client.
	MinPasswordLength = 8
	// MaxPasswordLength bounds the bcrypt input. bcrypt truncates at 72 bytes
	// silently, so anything above that gives a false sense of length. 256 is
	// generous and stops a caller from sending a megabyte to be hashed.
	MaxPasswordLength = 256
)

// commonPasswords is a small, embedded blocklist of the values that dominate
// every published breach corpus. It is not a substitute for checking against a
// full corpus such as Have I Been Pwned's, which is G-33; it is the floor that
// removes the most common case with no network dependency and no external
// service to fail.
var commonPasswords = map[string]bool{
	"password": true, "password1": true, "password123": true, "passw0rd": true,
	"12345678": true, "123456789": true, "1234567890": true, "12345678910": true,
	"qwerty123": true, "qwertyuiop": true, "abc12345": true, "abcdefg1": true,
	"iloveyou": true, "letmein1": true, "welcome1": true, "admin123": true,
	"football": true, "baseball": true, "trustno1": true, "whatever": true,
	"changeme": true, "pass1234": true, "test1234": true, "hello123": true,
}

// ValidatePassword reports whether a password may be set.
//
// It is deliberately the single place this decision is made. The check used to
// live inline at the registration and reset handlers, which meant a third entry
// point could be added without it and nothing would notice.
func ValidatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if n < MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}
	if len(password) > MaxPasswordLength {
		return fmt.Errorf("password must be at most %d bytes", MaxPasswordLength)
	}
	if len(strings.TrimSpace(password)) == 0 {
		return fmt.Errorf("password cannot be only whitespace")
	}
	if commonPasswords[strings.ToLower(password)] {
		// Named specifically: "too common" tells the user what to do about it,
		// where "invalid password" invites them to try "Password1".
		return fmt.Errorf("that password appears in known breach lists; choose something less common")
	}
	return nil
}
