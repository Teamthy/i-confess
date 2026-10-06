package rights

import (
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/voicegov"
)

var projectionNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// coarseLicense is a licence that permits synthesis, which is what most voices
// on the platform carry today.
func coarseLicense() *License {
	exp := projectionNow.Add(365 * 24 * time.Hour)
	return &License{
		VoiceID: "voice-1", Owner: "i-confess", Status: StatusActive, Expiry: &exp,
		CommercialUse: true, AIGenerationAllowed: true, UserContentAllowed: true,
		ProviderVoiceID: "el_pastor_a",
	}
}

func authorizeOn(g *voicegov.Grant, req voicegov.Request) voicegov.Decision {
	req.Action = voicegov.ActionGenerate
	req.At = projectionNow
	return voicegov.Authorize(g, req)
}

func TestToGrantProjectsWhatTheCoarseRecordAuthorized(t *testing.T) {
	g := coarseLicense().ToGrant()
	if g == nil {
		t.Fatal("an active licence projected to a nil grant")
	}
	if g.Status != voicegov.StatusApproved {
		t.Fatalf("status = %q, want %q", g.Status, voicegov.StatusApproved)
	}
	if g.VoiceID != "voice-1" || g.RightsHolder != "i-confess" {
		t.Fatalf("identity not carried: %+v", g)
	}
	if g.ExpiresAt == nil || !g.ExpiresAt.Equal(*coarseLicense().Expiry) {
		t.Fatalf("expiry not carried: %v", g.ExpiresAt)
	}

	// The same hosted confession render the coarse pipeline performed.
	d := authorizeOn(g, voicegov.Request{
		Purpose: voicegov.PurposeConfession, ThirdParty: true, Language: "en",
	})
	if !d.Allowed {
		t.Fatalf("projected grant refused the render the coarse licence permitted: %s %s", d.Reason, d.Detail)
	}
}

// The projection is a bridge, not a widening: capabilities the coarse record
// has no opinion on must be absent, or the migration would manufacture consent.
func TestToGrantNeverInfersCapabilitiesTheCoarseRecordCannotGrant(t *testing.T) {
	g := coarseLicense().ToGrant()
	lic := coarseLicense()
	lic.MarketingAllowed = false

	for _, c := range []voicegov.Capability{
		voicegov.CanTrain, voicegov.CanFineTune, voicegov.CanStoreModelCheckpoints,
		voicegov.CanCreateDerivativeModels, voicegov.CanStream, voicegov.CanDownload,
		voicegov.CanDistribute, voicegov.CanUseForResearch, voicegov.CanRetainAfterTermination,
		voicegov.CanUseInMarketing,
	} {
		if g.Has(c) {
			t.Errorf("projection inferred %s, which the coarse record cannot grant", c)
		}
	}
}

func TestToGrantStatusAndScope(t *testing.T) {
	past := projectionNow.Add(-time.Hour)

	cases := []struct {
		name   string
		mutate func(*License)
		reason voicegov.Reason
	}{
		{"revoked", func(l *License) { l.Status = StatusRevoked }, voicegov.ReasonRevoked},
		{"marked expired", func(l *License) { l.Status = StatusExpired }, voicegov.ReasonExpired},
		{"pending", func(l *License) { l.Status = StatusPending }, voicegov.ReasonNotApproved},
		{"unknown status", func(l *License) { l.Status = "escalated" }, voicegov.ReasonNotApproved},
		{"lapsed by date", func(l *License) { l.Expiry = &past }, voicegov.ReasonExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lic := coarseLicense()
			tc.mutate(lic)
			d := authorizeOn(lic.ToGrant(), voicegov.Request{Purpose: voicegov.PurposeConfession, ThirdParty: true})
			if d.Allowed {
				t.Fatal("a non-authorizing coarse record projected into an authorizing grant")
			}
			if d.Reason != tc.reason {
				t.Fatalf("reason = %q, want %q (%s)", d.Reason, tc.reason, d.Detail)
			}
		})
	}
}

func TestToGrantCarriesScopeAndPurposeGates(t *testing.T) {
	// Commercial use is required for every listener-facing use; the projection
	// must not smuggle synthesis past that.
	nonCommercial := coarseLicense()
	nonCommercial.CommercialUse = false
	d := authorizeOn(nonCommercial.ToGrant(), voicegov.Request{Purpose: voicegov.PurposeConfession, ThirdParty: true})
	if d.Allowed {
		t.Fatal("a non-commercial coarse licence projected into a commercial render")
	}
	if d.Reason != voicegov.ReasonMissingCapability || !hasCap(d.Missing, voicegov.CanCommercialize) {
		t.Fatalf("reason = %q missing = %v, want %s missing", d.Reason, d.Missing, voicegov.CanCommercialize)
	}

	// A licence that does not permit AI generation authorizes no synthesis,
	// whatever else it says.
	noAI := coarseLicense()
	noAI.AIGenerationAllowed = false
	if d := authorizeOn(noAI.ToGrant(), voicegov.Request{Purpose: voicegov.PurposeConfession, ThirdParty: true}); d.Allowed {
		t.Fatal("a licence without AIGenerationAllowed projected into an authorizing grant")
	}

	// User-submitted text is its own grant and must not be implied.
	noUserText := coarseLicense()
	noUserText.UserContentAllowed = false
	d = authorizeOn(noUserText.ToGrant(), voicegov.Request{
		Purpose: voicegov.PurposeConfession, ThirdParty: true, UserSubmittedText: true,
	})
	if d.Allowed {
		t.Fatal("user-submitted text was allowed without UserContentAllowed")
	}
	if !hasCap(d.Missing, voicegov.CanUseUserSubmittedText) {
		t.Fatalf("missing = %v, want %s", d.Missing, voicegov.CanUseUserSubmittedText)
	}

	// Territories and languages travel with the grant.
	scoped := coarseLicense()
	scoped.Territories = []string{"NG"}
	scoped.Languages = []string{"en"}
	if d := authorizeOn(scoped.ToGrant(), voicegov.Request{
		Purpose: voicegov.PurposeConfession, ThirdParty: true, Territory: "US",
	}); d.Reason != voicegov.ReasonTerritory {
		t.Fatalf("territory reason = %q, want %q", d.Reason, voicegov.ReasonTerritory)
	}
	if d := authorizeOn(scoped.ToGrant(), voicegov.Request{
		Purpose: voicegov.PurposeConfession, ThirdParty: true, Language: "de",
	}); d.Reason != voicegov.ReasonLanguage {
		t.Fatalf("language reason = %q, want %q", d.Reason, voicegov.ReasonLanguage)
	}
}

// Sending a voice to someone else's servers is a separate clause. The coarse
// record is only evidence of it when it names a provider voice at all.
func TestToGrantThirdPartyInfraFollowsTheConfiguredProviderVoice(t *testing.T) {
	withProvider := coarseLicense().ToGrant()
	if !withProvider.Has(voicegov.CanUseThirdPartyInfra) {
		t.Fatal("a record naming a provider voice did not carry the third-party capability")
	}
	hosted := authorizeOn(withProvider, voicegov.Request{Purpose: voicegov.PurposeConfession, ThirdParty: true})
	if !hosted.Allowed {
		t.Fatalf("hosted render refused: %s %s", hosted.Reason, hosted.Detail)
	}

	noProvider := coarseLicense()
	noProvider.ProviderVoiceID = ""
	d := authorizeOn(noProvider.ToGrant(), voicegov.Request{Purpose: voicegov.PurposeConfession, ThirdParty: true})
	if d.Allowed {
		t.Fatal("a hosted render was allowed with no provider voice configured")
	}
	if !hasCap(d.Missing, voicegov.CanUseThirdPartyInfra) {
		t.Fatalf("missing = %v, want %s", d.Missing, voicegov.CanUseThirdPartyInfra)
	}
	// The same record is fine when synthesis stays in-house.
	if d := authorizeOn(noProvider.ToGrant(), voicegov.Request{
		Purpose: voicegov.PurposeConfession, ThirdParty: false,
	}); !d.Allowed {
		t.Fatalf("self-hosted render refused: %s %s", d.Reason, d.Detail)
	}
}

func TestNilLicenseProjectsToNilGrant(t *testing.T) {
	var l *License
	if g := l.ToGrant(); g != nil {
		t.Fatalf("nil licence projected to %+v, want nil so Authorize reports no_grant", g)
	}
	if d := authorizeOn(l.ToGrant(), voicegov.Request{Purpose: voicegov.PurposeConfession}); d.Reason != voicegov.ReasonNoGrant {
		t.Fatalf("reason = %q, want %q", d.Reason, voicegov.ReasonNoGrant)
	}
}

func hasCap(caps []voicegov.Capability, want voicegov.Capability) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}
