package voicegov

import (
	"testing"
	"time"
)

var now = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

func fullGrant() *Grant {
	caps := map[Capability]bool{}
	for _, c := range AllCapabilities {
		caps[c] = true
	}
	return &Grant{VoiceID: "minister_001", Status: StatusApproved, Capabilities: caps, Version: 3}
}

func TestZeroGrantAuthorizesNothing(t *testing.T) {
	g := &Grant{Status: StatusApproved}
	for a := range actionCaps {
		if d := Authorize(g, Request{Action: a, At: now}); d.Allowed {
			t.Errorf("empty grant authorized %s", a)
		}
	}
	if d := Authorize(nil, Request{Action: ActionGenerate}); d.Allowed || d.Reason != ReasonNoGrant {
		t.Fatalf("nil grant: %+v", d)
	}
}

func TestNonAuthorizingStatusesDeny(t *testing.T) {
	cases := map[Status]Reason{
		StatusPending:     ReasonNotApproved,
		StatusRestricted:  ReasonNotApproved,
		StatusUnderReview: ReasonNotApproved,
		StatusSuspended:   ReasonSuspended,
		StatusRevoked:     ReasonRevoked,
		StatusExpired:     ReasonExpired,
		"approved":        ReasonNotApproved, // case matters; legacy lowercase is not accepted
	}
	for s, want := range cases {
		g := fullGrant()
		g.Status = s
		d := Authorize(g, Request{Action: ActionGenerate, Purpose: PurposeReflection, At: now})
		if d.Allowed || d.Reason != want {
			t.Errorf("status %s: got %+v, want reason %s", s, d, want)
		}
	}
}

// Holding permission to generate must not imply permission to train, and vice
// versa. This is the central requirement of §2.
func TestCapabilitiesAreIndependent(t *testing.T) {
	g := fullGrant()
	g.Capabilities[CanTrain] = false
	if d := Authorize(g, Request{Action: ActionGenerate, Purpose: PurposePrayer, At: now}); !d.Allowed {
		t.Fatalf("generation should not need training rights: %+v", d)
	}
	d := Authorize(g, Request{Action: ActionTrain, At: now})
	if d.Allowed || d.Reason != ReasonMissingCapability {
		t.Fatalf("training allowed without can_train: %+v", d)
	}
	if len(d.Missing) != 1 || d.Missing[0] != CanTrain {
		t.Fatalf("missing = %v", d.Missing)
	}
}

func TestPurposeCapabilities(t *testing.T) {
	g := fullGrant()
	g.Capabilities[CanUseInBibleAudio] = false
	if d := Authorize(g, Request{Action: ActionGenerate, Purpose: PurposeBible, At: now}); d.Allowed {
		t.Fatal("bible audio generated without can_use_in_bible_audio")
	}
	if d := Authorize(g, Request{Action: ActionGenerate, Purpose: PurposeConfession, At: now}); !d.Allowed {
		t.Fatalf("confession denied: %+v", d)
	}
}

func TestReportsEveryMissingCapability(t *testing.T) {
	g := &Grant{Status: StatusApproved, Capabilities: map[Capability]bool{CanGenerate: true}}
	d := Authorize(g, Request{Action: ActionGenerate, Purpose: PurposeReflection, ThirdParty: true, At: now})
	want := map[Capability]bool{CanClone: true, CanUseInReflections: true, CanCommercialize: true, CanUseThirdPartyInfra: true}
	if len(d.Missing) != len(want) {
		t.Fatalf("missing = %v, want %v", d.Missing, want)
	}
	for _, c := range d.Missing {
		if !want[c] {
			t.Errorf("unexpected missing %s", c)
		}
	}
}

func TestThirdPartyAndUserText(t *testing.T) {
	g := fullGrant()
	g.Capabilities[CanUseThirdPartyInfra] = false
	g.Capabilities[CanUseUserSubmittedText] = false
	if d := Authorize(g, Request{Action: ActionGenerate, Purpose: PurposeReflection, At: now}); !d.Allowed {
		t.Fatalf("in-house editorial generation denied: %+v", d)
	}
	if d := Authorize(g, Request{Action: ActionGenerate, Purpose: PurposeReflection, ThirdParty: true, At: now}); d.Allowed {
		t.Fatal("third-party inference allowed without can_use_third_party_infrastructure")
	}
	if d := Authorize(g, Request{Action: ActionGenerate, Purpose: PurposeReflection, UserSubmittedText: true, At: now}); d.Allowed {
		t.Fatal("user-submitted text allowed without can_use_user_submitted_text")
	}
}

// Expiry is evaluated live: a grant still stored as APPROVED stops at expiry.
func TestExpiryIsEvaluatedAtCallTime(t *testing.T) {
	g := fullGrant()
	exp := now.Add(time.Hour)
	g.ExpiresAt = &exp
	if d := Authorize(g, Request{Action: ActionGenerate, Purpose: PurposePrayer, At: now}); !d.Allowed {
		t.Fatalf("before expiry: %+v", d)
	}
	d := Authorize(g, Request{Action: ActionGenerate, Purpose: PurposePrayer, At: exp})
	if d.Allowed || d.Reason != ReasonExpired {
		t.Fatalf("at expiry (exclusive): %+v", d)
	}
	if EffectiveStatus(g, exp) != StatusExpired {
		t.Fatal("effective status should be EXPIRED")
	}
}

func TestNotYetEffective(t *testing.T) {
	g := fullGrant()
	from := now.Add(24 * time.Hour)
	g.EffectiveFrom = &from
	if d := Authorize(g, Request{Action: ActionGenerate, At: now}); d.Reason != ReasonNotYetEffective {
		t.Fatalf("got %+v", d)
	}
}

func TestRestrictedGenerationDeniedAndOtherActionsRemainScoped(t *testing.T) {
	g := fullGrant()
	g.Status = StatusRestricted
	g.Restrictions = []ContentPurpose{PurposeMarketing}
	for _, purpose := range []ContentPurpose{"", PurposeMarketing, PurposePrayer} {
		d := Authorize(g, Request{Action: ActionGenerate, Purpose: purpose, At: now})
		if d.Allowed || d.Reason != ReasonNotApproved {
			t.Errorf("restricted generation for %q: %+v", purpose, d)
		}
	}
	if d := Authorize(g, Request{Action: ActionStream, Purpose: PurposeMarketing, At: now}); d.Reason != ReasonRestricted {
		t.Fatalf("restricted distribution purpose: %+v", d)
	}
	if d := Authorize(g, Request{Action: ActionStream, Purpose: PurposePrayer, At: now}); !d.Allowed {
		t.Fatalf("separately permitted distribution: %+v", d)
	}
}

func TestScope(t *testing.T) {
	g := fullGrant()
	g.Territories = []string{"NG", "GB"}
	g.Languages = []string{"en"}
	if d := Authorize(g, Request{Action: ActionGenerate, Territory: "us", At: now}); d.Reason != ReasonTerritory {
		t.Fatalf("territory: %+v", d)
	}
	if d := Authorize(g, Request{Action: ActionGenerate, Language: "en-NG", Territory: "ng", At: now}); !d.Allowed {
		t.Fatalf("en covers en-NG: %+v", d)
	}
	if d := Authorize(g, Request{Action: ActionGenerate, Language: "yo", At: now}); d.Reason != ReasonLanguage {
		t.Fatalf("language: %+v", d)
	}
	g.Languages = []string{"en-NG"}
	if d := Authorize(g, Request{Action: ActionGenerate, Language: "en-US", At: now}); d.Allowed {
		t.Fatal("en-NG licence must not cover en-US")
	}
}

func TestUnknownActionAndPurpose(t *testing.T) {
	g := fullGrant()
	if d := Authorize(g, Request{Action: "teleport"}); d.Reason != ReasonUnknownAction {
		t.Fatalf("%+v", d)
	}
	if d := Authorize(g, Request{Action: ActionGenerate, Purpose: "karaoke"}); d.Reason != ReasonUnknownPurpose {
		t.Fatalf("%+v", d)
	}
}

func TestDecisionCarriesGrantVersion(t *testing.T) {
	d := Authorize(fullGrant(), Request{Action: ActionGenerate, At: now})
	if d.GrantVersion != 3 || len(d.Required) == 0 {
		t.Fatalf("%+v", d)
	}
}

func TestRevokedIsTerminal(t *testing.T) {
	for _, s := range AllStatuses {
		if CanTransition(StatusRevoked, s) {
			t.Errorf("REVOKED -> %s must be impossible", s)
		}
	}
	if !CanTransition(StatusApproved, StatusRevoked) || !CanTransition(StatusPending, StatusRevoked) {
		t.Fatal("revocation must be reachable")
	}
	if CanTransition(StatusPending, StatusApproved) {
		t.Fatal("approval must pass through review")
	}
}

func TestExistingAssetPolicy(t *testing.T) {
	g := fullGrant()
	if p := ExistingAssetPolicy(g, now); p != AssetRetain {
		t.Fatalf("active: %s", p)
	}
	g.Status = StatusSuspended
	g.PostTermination = AssetDelete
	if p := ExistingAssetPolicy(g, now); p != AssetArchive {
		t.Fatalf("suspended must archive, never delete: %s", p)
	}
	g.Status = StatusRevoked
	if p := ExistingAssetPolicy(g, now); p != AssetDelete {
		t.Fatalf("revoked+delete: %s", p)
	}
	g.PostTermination = AssetRetain
	g.Capabilities[CanRetainAfterTermination] = false
	if p := ExistingAssetPolicy(g, now); p != AssetUnpublish {
		t.Fatalf("retain without post-termination right: %s", p)
	}
	g.PostTermination = ""
	if p := ExistingAssetPolicy(g, now); p != AssetUnpublish {
		t.Fatalf("silent licence defaults to unpublish: %s", p)
	}
}

func TestEveryActionAndPurposeHasCapabilities(t *testing.T) {
	for a, caps := range actionCaps {
		if len(caps) == 0 {
			t.Errorf("action %s requires nothing", a)
		}
		for _, c := range caps {
			if !ValidCapability(c) {
				t.Errorf("action %s references unknown capability %s", a, c)
			}
		}
	}
	for p, caps := range purposeCaps {
		for _, c := range caps {
			if !ValidCapability(c) {
				t.Errorf("purpose %s references unknown capability %s", p, c)
			}
		}
	}
}

func TestRenderUseRechecksScopeAndClock(t *testing.T) {
	g := fullGrant()
	planned := now.Add(-time.Minute)
	u := RenderUse{VoiceID: g.VoiceID, GrantVersion: g.Version, Request: Request{Action: ActionStream, Purpose: PurposePrayer,
		Territory: "NG", Language: "pcm-NG", UserSubmittedText: true, ThirdParty: true, At: planned}}
	if d := u.Recheck(g, now); !d.Allowed {
		t.Fatal(d)
	}
	for _, tc := range []struct {
		name   string
		change func(*Grant)
		reason Reason
	}{
		{"different voice", func(g *Grant) { g.VoiceID = "other" }, ReasonNoGrant},
		{"new agreement", func(g *Grant) { g.Version++ }, ReasonStaleGrant},
		{"expiry at handoff", func(g *Grant) { exp := now; g.ExpiresAt = &exp }, ReasonExpired},
		{"future effective date", func(g *Grant) { future := now.Add(time.Second); g.EffectiveFrom = &future }, ReasonNotYetEffective},
		{"restricted cannot generate preview", func(g *Grant) { g.Status = StatusRestricted }, ReasonNotApproved},
		{"territory", func(g *Grant) { g.Territories = []string{"GB"} }, ReasonTerritory},
		{"language", func(g *Grant) { g.Languages = []string{"en-NG"} }, ReasonLanguage},
		{"purpose", func(g *Grant) { delete(g.Capabilities, CanUseInPrayers) }, ReasonMissingCapability},
		{"external provider", func(g *Grant) { delete(g.Capabilities, CanUseThirdPartyInfra) }, ReasonMissingCapability},
		{"private script", func(g *Grant) { delete(g.Capabilities, CanUseUserSubmittedText) }, ReasonMissingCapability},
		{"distribute", func(g *Grant) { delete(g.Capabilities, CanDistribute) }, ReasonMissingCapability},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := fullGrant()
			tc.change(g)
			if d := u.Recheck(g, now); d.Allowed || d.Reason != tc.reason {
				t.Fatalf("got %+v, want %s", d, tc.reason)
			}
		})
	}
}
