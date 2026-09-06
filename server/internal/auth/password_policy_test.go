package auth

import (
	"strings"
	"testing"
)

func TestValidatePasswordRejects(t *testing.T) {
	cases := []struct{ password, why string }{
		{"", "empty"},
		{"short", "below the minimum length"},
		{"1234567", "one short of the minimum"},
		{"password", "the single most common password in every breach corpus"},
		{"PASSWORD", "the blocklist is case-insensitive, so casing is not a bypass"},
		{"Password", "same"},
		{"12345678", "all digits, extremely common"},
		{"1234567890", "same"},
		{"qwerty123", "keyboard walk"},
		{"        ", "whitespace only - it passes a naive length check"},
		{"  \t\n  ", "whitespace variants"},
		{strings.Repeat("a", MaxPasswordLength+1), "over the maximum"},
	}

	for _, tc := range cases {
		if err := ValidatePassword(tc.password); err == nil {
			t.Errorf("accepted %q (%s)", truncate(tc.password), tc.why)
		}
	}
}

func TestValidatePasswordAccepts(t *testing.T) {
	cases := []struct{ password, why string }{
		{"correct horse battery", "long passphrase - exactly what the policy wants"},
		{"a-real-sentence-about-grace", "longer is better and must not be capped low"},
		{"Tr0ub4dor&3xkcd", "mixed everything, also fine"},
		{"八文字以上のパスワードです", "non-ASCII counts by rune, not byte"},
		{strings.Repeat("a", MaxPasswordLength), "exactly at the maximum"},
		{"password2", "not on the blocklist; the policy does not pretend to catch everything"},
	}

	for _, tc := range cases {
		if err := ValidatePassword(tc.password); err != nil {
			t.Errorf("rejected %q (%s): %v", truncate(tc.password), tc.why, err)
		}
	}
}

// TestNonASCIILengthIsCountedInRunes covers a real off-by-one: len() on a string
// counts bytes, so a CJK password of four characters would read as twelve and
// pass a length check it should fail.
func TestNonASCIILengthIsCountedInRunes(t *testing.T) {
	short := "あいう" // three runes, nine bytes
	if err := ValidatePassword(short); err == nil {
		t.Errorf("accepted %q: %d bytes but only 3 characters", short, len(short))
	}
	if len(short) >= MinPasswordLength {
		t.Logf("confirms the bug this guards: len() reports %d for a 3-character password", len(short))
	}
}

// TestNoCompositionRules is a deliberate assertion about what the policy does
// NOT do. NIST SP 800-63B advises against forcing symbols and mixed case: it
// does not raise guessing cost, it just makes people append "1!" to the same
// word. If someone later adds such a rule, this fails and they have to argue
// for it.
func TestNoCompositionRules(t *testing.T) {
	for _, pw := range []string{
		"alllowercaseletters",
		"ALLUPPERCASELETTERS",
		"nodigitsorsymbolshere",
		strings.Repeat("x", 40),
	} {
		if err := ValidatePassword(pw); err != nil {
			t.Errorf("rejected %q - the policy must not require digits or symbols: %v", truncate(pw), err)
		}
	}
}

func truncate(s string) string {
	if len(s) <= 24 {
		return s
	}
	return s[:21] + "..."
}
