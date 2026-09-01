package rights

import (
	"testing"
	"time"
)

var now = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// fullGrant is a maximally permissive licence; tests revoke one field at a time
// so each rule is proven to be load-bearing on its own.
func fullGrant() *License {
	start := now.Add(-30 * 24 * time.Hour)
	exp := now.Add(300 * 24 * time.Hour)
	return &License{
		VoiceID: "v1", Owner: "Pastor A", Provider: "elevenlabs",
		Status: StatusActive, Start: &start, Expiry: &exp,
		CommercialUse: true, AIGenerationAllowed: true,
		MarketingAllowed: true, UserContentAllowed: true,
		ProviderVoiceID: "el_abc123",
	}
}

func TestFullGrantAllowsEveryUse(t *testing.T) {
	for _, u := range []Use{UsePlayback, UseSynthesis, UseMarketing, UseUserContent} {
		d := Evaluate(fullGrant(), Request{Use: u, At: now})
		if !d.Allowed {
			t.Fatalf("use %q denied unexpectedly: %s (%s)", u, d.Reason, d.Detail)
		}
	}
}

// The central rule of §13.
func TestSynthesisRequiresExplicitAIGrant(t *testing.T) {
	lic := fullGrant()
	lic.AIGenerationAllowed = false

	// Playback of existing recordings remains fine...
	if d := Evaluate(lic, Request{Use: UsePlayback, At: now}); !d.Allowed {
		t.Fatalf("playback should still be allowed: %s", d.Reason)
	}
	// ...but synthesizing new audio must be refused.
	d := Evaluate(lic, Request{Use: UseSynthesis, At: now})
	if d.Allowed {
		t.Fatal("synthesis allowed without an AI-generation grant")
	}
	if d.Reason != ReasonNoAIGeneration {
		t.Fatalf("reason = %q, want %q", d.Reason, ReasonNoAIGeneration)
	}
}

func TestNilLicenseDeniesEverything(t *testing.T) {
	for _, u := range []Use{UsePlayback, UseSynthesis, UseMarketing, UseUserContent} {
		if d := Evaluate(nil, Request{Use: u, At: now}); d.Allowed || d.Reason != ReasonNoLicense {
			t.Fatalf("nil licence allowed %q (reason %q)", u, d.Reason)
		}
	}
}

// A zero-value License must authorize nothing: fail closed, not open.
func TestZeroValueLicenseDeniesEverything(t *testing.T) {
	for _, u := range []Use{UsePlayback, UseSynthesis, UseMarketing, UseUserContent} {
		if d := Evaluate(&License{}, Request{Use: u, At: now}); d.Allowed {
			t.Fatalf("zero-value licence allowed %q", u)
		}
	}
}

func TestLifecycleStates(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*License)
		want   Reason
	}{
		{"revoked", func(l *License) { l.Status = StatusRevoked }, ReasonRevoked},
		{"pending", func(l *License) { l.Status = StatusPending }, ReasonNotActive},
		{"none", func(l *License) { l.Status = StatusNone }, ReasonNotActive},
		{"marked expired", func(l *License) { l.Status = StatusExpired }, ReasonExpired},
		{"empty status", func(l *License) { l.Status = "" }, ReasonNotActive},
		{"past expiry", func(l *License) { e := now.Add(-time.Hour); l.Expiry = &e }, ReasonExpired},
		{"not yet effective", func(l *License) { s := now.Add(48 * time.Hour); l.Start = &s }, ReasonNotYetEffective},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lic := fullGrant()
			tc.mutate(lic)
			d := Evaluate(lic, Request{Use: UseSynthesis, At: now})
			if d.Allowed {
				t.Fatal("expected denial")
			}
			if d.Reason != tc.want {
				t.Fatalf("reason = %q, want %q", d.Reason, tc.want)
			}
		})
	}
}

// Expiry is exclusive: the instant a licence expires, it is invalid.
func TestExpiryBoundaryIsExclusive(t *testing.T) {
	lic := fullGrant()
	exp := now
	lic.Expiry = &exp

	if d := Evaluate(lic, Request{Use: UseSynthesis, At: now}); d.Allowed {
		t.Fatal("licence allowed at the exact expiry instant")
	}
	if d := Evaluate(lic, Request{Use: UseSynthesis, At: now.Add(-time.Second)}); !d.Allowed {
		t.Fatalf("licence denied one second before expiry: %s", d.Reason)
	}
}

func TestCommercialGrantRequired(t *testing.T) {
	lic := fullGrant()
	lic.CommercialUse = false
	for _, u := range []Use{UsePlayback, UseSynthesis, UseMarketing} {
		d := Evaluate(lic, Request{Use: u, At: now})
		if d.Allowed || d.Reason != ReasonNoCommercial {
			t.Fatalf("use %q: allowed=%v reason=%q, want commercial refusal", u, d.Allowed, d.Reason)
		}
	}
}

// User-submitted text is synthesis PLUS its own grant, because the words cannot
// be reviewed in advance.
func TestUserContentNeedsItsOwnGrant(t *testing.T) {
	lic := fullGrant()
	lic.UserContentAllowed = false

	if d := Evaluate(lic, Request{Use: UseSynthesis, At: now}); !d.Allowed {
		t.Fatalf("editorial synthesis should still be allowed: %s", d.Reason)
	}
	d := Evaluate(lic, Request{Use: UseUserContent, At: now})
	if d.Allowed || d.Reason != ReasonNoUserContent {
		t.Fatalf("allowed=%v reason=%q, want user-content refusal", d.Allowed, d.Reason)
	}
}

func TestMarketingNeedsItsOwnGrant(t *testing.T) {
	lic := fullGrant()
	lic.MarketingAllowed = false
	if d := Evaluate(lic, Request{Use: UseMarketing, At: now}); d.Allowed || d.Reason != ReasonNoMarketing {
		t.Fatalf("allowed=%v reason=%q", d.Allowed, d.Reason)
	}
	if d := Evaluate(lic, Request{Use: UseSynthesis, At: now}); !d.Allowed {
		t.Fatal("marketing refusal must not block editorial synthesis")
	}
}

func TestSynthesisRequiresProviderVoiceID(t *testing.T) {
	lic := fullGrant()
	lic.ProviderVoiceID = "   "
	d := Evaluate(lic, Request{Use: UseSynthesis, At: now})
	if d.Allowed || d.Reason != ReasonNoProviderVoice {
		t.Fatalf("allowed=%v reason=%q", d.Allowed, d.Reason)
	}
	// Playback of pre-existing audio needs no provider id.
	if d := Evaluate(lic, Request{Use: UsePlayback, At: now}); !d.Allowed {
		t.Fatal("playback should not require a provider voice id")
	}
}

func TestTerritoryScope(t *testing.T) {
	lic := fullGrant()
	lic.Territories = []string{"NG", "GH", "GB"}

	if d := Evaluate(lic, Request{Use: UseSynthesis, Territory: "ng", At: now}); !d.Allowed {
		t.Fatalf("licensed territory denied (case-insensitive): %s", d.Reason)
	}
	d := Evaluate(lic, Request{Use: UseSynthesis, Territory: "US", At: now})
	if d.Allowed || d.Reason != ReasonTerritory {
		t.Fatalf("allowed=%v reason=%q, want territory refusal", d.Allowed, d.Reason)
	}
	// Empty territory list means worldwide.
	lic.Territories = nil
	if d := Evaluate(lic, Request{Use: UseSynthesis, Territory: "US", At: now}); !d.Allowed {
		t.Fatal("empty territory list should mean worldwide")
	}
}

func TestLanguageScope(t *testing.T) {
	lic := fullGrant()
	lic.Languages = []string{"en"}

	// A licence for "en" covers regional variants.
	if d := Evaluate(lic, Request{Use: UseSynthesis, Language: "en-GB", At: now}); !d.Allowed {
		t.Fatalf("en licence should cover en-GB: %s", d.Reason)
	}
	if d := Evaluate(lic, Request{Use: UseSynthesis, Language: "de", At: now}); d.Allowed {
		t.Fatal("en licence must not cover de")
	}

	// A region-specific licence does not widen to siblings.
	lic.Languages = []string{"en-GB"}
	if d := Evaluate(lic, Request{Use: UseSynthesis, Language: "en-US", At: now}); d.Allowed {
		t.Fatal("en-GB licence must not cover en-US")
	}
	if d := Evaluate(lic, Request{Use: UseSynthesis, Language: "en-GB", At: now}); !d.Allowed {
		t.Fatal("en-GB licence must cover en-GB")
	}
}

func TestUnknownUseIsDenied(t *testing.T) {
	if d := Evaluate(fullGrant(), Request{Use: Use("exfiltrate"), At: now}); d.Allowed || d.Reason != ReasonUnknownUse {
		t.Fatalf("allowed=%v reason=%q", d.Allowed, d.Reason)
	}
}

func TestExpiringWithin(t *testing.T) {
	lic := fullGrant()
	soon := now.Add(10 * 24 * time.Hour)
	lic.Expiry = &soon

	if !lic.ExpiringWithin(30*24*time.Hour, now) {
		t.Fatal("expected licence to be flagged as expiring soon")
	}
	if lic.ExpiringWithin(5*24*time.Hour, now) {
		t.Fatal("licence should not be flagged outside the window")
	}
	// Already-expired licences are not "expiring": they are expired.
	past := now.Add(-time.Hour)
	lic.Expiry = &past
	if lic.ExpiringWithin(30*24*time.Hour, now) {
		t.Fatal("expired licence should not be reported as expiring")
	}
	// Perpetual licences never expire.
	lic.Expiry = nil
	if lic.ExpiringWithin(30*24*time.Hour, now) {
		t.Fatal("perpetual licence should not be reported as expiring")
	}
}
