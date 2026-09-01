package mfa

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// TOTP is verified against RFC 6238's published test vectors. Matching the
// standard's own answers is stronger assurance than a self-consistent
// round-trip test, which would pass even if the algorithm were wrong.
func TestRFC6238Vectors(t *testing.T) {
	// The RFC uses the ASCII secret "12345678901234567890".
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).
		EncodeToString([]byte("12345678901234567890"))

	// Published 8-digit values, truncated to the 6 digits this package emits.
	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
	}
	for _, tc := range cases {
		got, err := Code(secret, time.Unix(tc.unix, 0).UTC())
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("Code at %d = %s, want %s", tc.unix, got, tc.want)
		}
	}
}

func TestGenerateSecretIsUsable(t *testing.T) {
	s1, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := GenerateSecret()
	if s1 == s2 {
		t.Fatal("two generated secrets were identical")
	}
	if len(s1) < 32 {
		t.Fatalf("secret is too short: %d chars", len(s1))
	}
	if _, err := Code(s1, time.Now()); err != nil {
		t.Fatalf("generated secret does not produce a code: %v", err)
	}
}

func TestVerifyAcceptsCurrentCode(t *testing.T) {
	secret, _ := GenerateSecret()
	now := time.Unix(1_700_000_000, 0).UTC()

	code, _ := Code(secret, now)
	if _, ok := Verify(secret, code, now); !ok {
		t.Fatal("current code rejected")
	}
}

// One step of tolerance either side handles ordinary clock drift; two must not
// be accepted, because widening the window multiplies an attacker's guesses.
func TestVerifySkewWindow(t *testing.T) {
	secret, _ := GenerateSecret()
	now := time.Unix(1_700_000_000, 0).UTC()

	for _, delta := range []time.Duration{-Period, 0, Period} {
		code, _ := Code(secret, now.Add(delta))
		if _, ok := Verify(secret, code, now); !ok {
			t.Fatalf("code at %v offset rejected within the skew window", delta)
		}
	}
	for _, delta := range []time.Duration{-3 * Period, 3 * Period} {
		code, _ := Code(secret, now.Add(delta))
		if _, ok := Verify(secret, code, now); ok {
			t.Fatalf("code at %v offset accepted outside the skew window", delta)
		}
	}
}

// Verify returns the matched counter so callers can reject replays: without it
// an observed code works for the rest of its 30-second window.
func TestVerifyReturnsCounterForReplayDefence(t *testing.T) {
	secret, _ := GenerateSecret()
	now := time.Unix(1_700_000_000, 0).UTC()

	code, _ := Code(secret, now)
	c1, ok := Verify(secret, code, now)
	if !ok {
		t.Fatal("code rejected")
	}
	c2, _ := Verify(secret, code, now.Add(5*time.Second))
	if c1 != c2 {
		t.Fatal("the same code reported different counters within one window")
	}

	// A later window must produce a different counter.
	later, _ := Code(secret, now.Add(2*Period))
	c3, ok := Verify(secret, later, now.Add(2*Period))
	if !ok || c3 == c1 {
		t.Fatalf("counter did not advance across windows: %d then %d", c1, c3)
	}
}

func TestVerifyRejectsBadInput(t *testing.T) {
	secret, _ := GenerateSecret()
	now := time.Now()

	for _, code := range []string{"", "12345", "1234567", "abcdef", "000000"} {
		if _, ok := Verify(secret, code, now); ok {
			t.Fatalf("bogus code accepted: %q", code)
		}
	}
	// A malformed secret must fail closed rather than panic.
	if _, ok := Verify("not!base32!", "123456", now); ok {
		t.Fatal("malformed secret produced a match")
	}
}

// A code from one secret must never validate against another.
func TestCodesDoNotCrossSecrets(t *testing.T) {
	a, _ := GenerateSecret()
	b, _ := GenerateSecret()
	now := time.Now()

	code, _ := Code(a, now)
	if _, ok := Verify(b, code, now); ok {
		t.Fatal("a code from one secret validated against a different secret")
	}
}

func TestProvisioningURI(t *testing.T) {
	uri := ProvisioningURI("JBSWY3DPEHPK3PXP", "grace@example.com", "i-confess")

	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Fatalf("wrong scheme: %s", uri)
	}
	// The issuer must appear both in the label and as a parameter, or the entry
	// shows up unlabelled in some apps.
	if !strings.Contains(uri, "issuer=i-confess") {
		t.Fatalf("issuer parameter missing: %s", uri)
	}
	// The label must be "issuer:account". url.PathEscape leaves ":" and "@"
	// unescaped, which is valid in a path segment and what apps expect.
	label := strings.TrimPrefix(uri, "otpauth://totp/")
	label = label[:strings.Index(label, "?")]
	if label != "i-confess:grace@example.com" {
		t.Fatalf("label = %q, want issuer-prefixed", label)
	}
	if !strings.Contains(uri, "secret=JBSWY3DPEHPK3PXP") {
		t.Fatalf("secret missing: %s", uri)
	}
}

// ---------------------------------------------------------------------------
// Recovery codes
// ---------------------------------------------------------------------------

func TestRecoveryCodesAreDistinctAndReadable(t *testing.T) {
	plain, hashes, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) != RecoveryCodeCount || len(hashes) != RecoveryCodeCount {
		t.Fatalf("got %d codes and %d hashes", len(plain), len(hashes))
	}

	seen := map[string]bool{}
	for _, c := range plain {
		if seen[c] {
			t.Fatalf("duplicate recovery code: %s", c)
		}
		seen[c] = true

		// Ambiguous characters must not appear: these get copied off paper.
		if strings.ContainsAny(c, "01OIL") {
			t.Fatalf("code contains easily-misread characters: %s", c)
		}
		// Grouped for legibility.
		if !strings.Contains(c, "-") {
			t.Fatalf("code is not grouped: %s", c)
		}
	}
}

// Only hashes are stored, so a leaked database must not yield working factors.
func TestRecoveryHashesDoNotRevealCodes(t *testing.T) {
	plain, hashes, _ := GenerateRecoveryCodes()
	for i, h := range hashes {
		if strings.Contains(h, NormaliseRecoveryCode(plain[i])) {
			t.Fatal("hash contains its plaintext")
		}
		if len(h) != 64 {
			t.Fatalf("hash is not a hex sha256: %q", h)
		}
	}
}

// Users retype these from paper, so formatting must not matter.
func TestRecoveryMatchingIsForgivingOfFormatting(t *testing.T) {
	plain, hashes, _ := GenerateRecoveryCodes()
	code := plain[3]

	variants := []string{
		code,
		strings.ToLower(code),
		strings.ReplaceAll(code, "-", ""),
		strings.ReplaceAll(code, "-", " "),
		"  " + code + "  ",
	}
	for _, v := range variants {
		idx, ok := MatchRecoveryCode(v, hashes)
		if !ok {
			t.Fatalf("valid code rejected when typed as %q", v)
		}
		if idx != 3 {
			t.Fatalf("matched index %d, want 3", idx)
		}
	}
}

func TestRecoveryRejectsWrongCode(t *testing.T) {
	_, hashes, _ := GenerateRecoveryCodes()

	for _, bad := range []string{"", "ABCD-EFGH-JKMN", "not-a-code", "0000-0000-0000"} {
		if _, ok := MatchRecoveryCode(bad, hashes); ok {
			t.Fatalf("invalid recovery code accepted: %q", bad)
		}
	}
}

// Enough entropy that guessing is not a viable path around MFA.
func TestRecoveryCodeEntropy(t *testing.T) {
	plain, _, _ := GenerateRecoveryCodes()
	normalised := NormaliseRecoveryCode(plain[0])
	if len(normalised) < 12 {
		t.Fatalf("recovery code has only %d significant characters", len(normalised))
	}
}
