package rights

import (
	"testing"
	"time"
)

// The adapter reconciles two overlapping generations of stored fields. These
// tests pin the rule that ambiguity always resolves against the platform.

func TestFromSourceNilIsNil(t *testing.T) {
	if FromSource(nil) != nil {
		t.Fatal("nil source should produce a nil licence, which denies everything")
	}
}

func TestNormaliseStatusPrefersTheRestrictiveReading(t *testing.T) {
	cases := []struct {
		status, legacy string
		want           LicenseStatus
	}{
		{"active", "active", StatusActive},
		{"active", "", StatusActive},
		{"", "active", StatusActive},
		// Disagreement must not resolve to active.
		{"active", "revoked", StatusRevoked},
		{"revoked", "active", StatusRevoked},
		{"active", "expired", StatusExpired},
		{"expired", "active", StatusExpired},
		{"pending", "active", StatusPending},
		{"", "", StatusNone},
	}
	for _, tc := range cases {
		got := normaliseStatus(tc.status, tc.legacy)
		if got != tc.want {
			t.Fatalf("normaliseStatus(%q,%q) = %q, want %q", tc.status, tc.legacy, got, tc.want)
		}
	}
}

// When the two expiry fields disagree, the sooner one governs.
func TestEarliestExpiryWins(t *testing.T) {
	src := &Source{
		VoiceID: "v1", Status: "active", CommercialUse: true,
		AIGenerationAllowed: true, ProviderVoiceID: "p1", AllowedUse: "tts",
		ExpiryDate:     "2030-01-01T00:00:00Z",
		ExpirationDate: "2026-01-01T00:00:00Z",
	}
	lic := FromSource(src)
	if lic.Expiry == nil {
		t.Fatal("expiry not parsed")
	}
	if lic.Expiry.Year() != 2026 {
		t.Fatalf("expiry year = %d, want the earlier 2026", lic.Expiry.Year())
	}

	// And it must actually deny after that date.
	at := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if d := Evaluate(lic, Request{Use: UseSynthesis, At: at}); d.Allowed {
		t.Fatal("licence allowed past its earliest expiry")
	}
}

// "recording" permission is not synthesis permission, regardless of the flag.
func TestAllowedUseRecordingBlocksSynthesis(t *testing.T) {
	src := &Source{
		VoiceID: "v1", Status: "active", CommercialUse: true,
		AIGenerationAllowed: true, UserContentAllowed: true,
		ProviderVoiceID: "p1", AllowedUse: "recording",
	}
	lic := FromSource(src)
	if lic.AIGenerationAllowed {
		t.Fatal("a recording-only licence must not permit AI generation")
	}

	at := time.Now().UTC()
	if d := Evaluate(lic, Request{Use: UseSynthesis, At: at}); d.Allowed {
		t.Fatal("synthesis allowed on a recording-only licence")
	}
	// Playback of the recordings themselves is still fine.
	if d := Evaluate(lic, Request{Use: UsePlayback, At: at}); !d.Allowed {
		t.Fatalf("playback should remain allowed: %s", d.Reason)
	}
}

func TestAllowedUseTTSPermitsSynthesis(t *testing.T) {
	src := &Source{
		VoiceID: "v1", Status: "active", CommercialUse: true,
		AIGenerationAllowed: true, ProviderVoiceID: "p1", AllowedUse: "tts",
	}
	if d := Evaluate(FromSource(src), Request{Use: UseSynthesis, At: time.Now().UTC()}); !d.Allowed {
		t.Fatalf("synthesis denied on a tts licence: %s (%s)", d.Reason, d.Detail)
	}
}

func TestGlobalTerritoryMeansWorldwide(t *testing.T) {
	src := &Source{
		VoiceID: "v1", Status: "active", CommercialUse: true,
		AIGenerationAllowed: true, ProviderVoiceID: "p1", AllowedUse: "tts",
		Territories: "GLOBAL",
	}
	lic := FromSource(src)
	if len(lic.Territories) != 0 {
		t.Fatalf("GLOBAL should map to an empty list, got %v", lic.Territories)
	}
	if d := Evaluate(lic, Request{Use: UseSynthesis, Territory: "NG", At: time.Now().UTC()}); !d.Allowed {
		t.Fatalf("GLOBAL licence refused a territory: %s", d.Reason)
	}
}

func TestTerritoryListIsHonoured(t *testing.T) {
	src := &Source{
		VoiceID: "v1", Status: "active", CommercialUse: true,
		AIGenerationAllowed: true, ProviderVoiceID: "p1", AllowedUse: "tts",
		Territories: "NG, GH ,GB",
	}
	lic := FromSource(src)
	at := time.Now().UTC()
	if d := Evaluate(lic, Request{Use: UseSynthesis, Territory: "GH", At: at}); !d.Allowed {
		t.Fatalf("licensed territory denied: %s", d.Reason)
	}
	if d := Evaluate(lic, Request{Use: UseSynthesis, Territory: "US", At: at}); d.Allowed {
		t.Fatal("unlicensed territory allowed")
	}
}

// A stored record with no grants must authorize nothing once adapted.
func TestEmptySourceDeniesEverything(t *testing.T) {
	lic := FromSource(&Source{VoiceID: "v1"})
	for _, u := range []Use{UsePlayback, UseSynthesis, UseMarketing, UseUserContent} {
		if d := Evaluate(lic, Request{Use: u, At: time.Now().UTC()}); d.Allowed {
			t.Fatalf("empty rights record allowed %q", u)
		}
	}
}

func TestDateOnlyFormatsParse(t *testing.T) {
	src := &Source{
		VoiceID: "v1", Status: "active", CommercialUse: true,
		AIGenerationAllowed: true, ProviderVoiceID: "p1", AllowedUse: "tts",
		StartDate: "2026-01-01", ExpiryDate: "2030-12-31",
	}
	lic := FromSource(src)
	if lic.Start == nil || lic.Expiry == nil {
		t.Fatal("date-only values should parse")
	}
}
