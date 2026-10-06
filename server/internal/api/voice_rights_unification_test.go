package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/voicegov"
)

// This file is the runtime evidence for audit VE-001: the platform had two
// rights models, and a capability revoked in the granular one did not affect
// the cloud-TTS path, which asked the coarse four-boolean record instead.
//
// The tests below drive the real admin endpoint against a real database. They
// assert the two properties that matter:
//
//  1. a hosted render needs can_use_third_party_infrastructure, and revoking it
//     refuses the very next request - with 451, before the provider is billed;
//  2. the granular grant governs even when the coarse record is permissive, so
//     an operator cannot grant themselves around a revoked capability by
//     editing the other table.

// ministerVoiceWithGrant registers a synthetic minister voice, puts a signed
// rights document on file, grants exactly caps, and takes the grant through the
// real lifecycle to APPROVED.
//
// It also writes a deliberately permissive coarse voice_rights record, because
// that record is still where the provider's own voice id lives. Its
// permissiveness is the point: the granular grant must win.
func (h *qaHarness) ministerVoiceWithGrant(t *testing.T, caps map[voicegov.Capability]bool) string {
	t.Helper()
	ctx := context.Background()
	vp := store.NewVoicePlatformStore(h.db)

	v := &store.MinisterVoice{Name: "Chidi", DisplayName: "Chidi", Language: "en", Locale: "en-NG"}
	if err := vp.CreateMinisterVoice(ctx, v, "i-confess", "test"); err != nil {
		t.Fatalf("create minister voice: %v", err)
	}
	if _, err := vp.AddRightsDocument(ctx, v.ID, "voice_licence", "Signed voice licence",
		"rights/"+v.ID+"/licence.pdf", "sha256:0000", "test"); err != nil {
		t.Fatalf("add rights document: %v", err)
	}
	h.setGrantCaps(t, vp, v.ID, caps)
	for _, step := range []voicegov.Status{voicegov.StatusUnderReview, voicegov.StatusApproved} {
		if err := vp.TransitionGrant(ctx, v.ID, step, "test", "approved in test", ""); err != nil {
			t.Fatalf("transition to %s: %v", step, err)
		}
	}

	vrs := store.NewVoiceRightsStore(h.db)
	if err := vrs.Create(ctx, &models.VoiceRights{
		VoiceID: v.ID, RightsHolder: "i-confess", AllowedUse: "tts",
		Territories: "GLOBAL", Status: "active", LicenseStatus: "active",
		CommercialUse: true, AIGenerationAllowed: true,
		ProviderVoiceID: "stub-voice-chidi",
	}); err != nil {
		t.Fatalf("create coarse rights record: %v", err)
	}
	return v.ID
}

// setGrantCaps replaces the grant's capability set, using the same store call
// the admin rights endpoint uses.
func (h *qaHarness) setGrantCaps(t *testing.T, vp *store.VoicePlatformStore, voiceID string, caps map[voicegov.Capability]bool) {
	t.Helper()
	if _, err := vp.UpdateGrantTerms(context.Background(), voiceID, store.GrantTerms{
		Capabilities: caps, PostTermination: voicegov.AssetUnpublish,
	}, "test", ""); err != nil {
		t.Fatalf("update grant terms: %v", err)
	}
}

// hostedGenerationCaps is what a cloud render of a confession needs: the
// generation capabilities, the confession purpose, commercial use (the product
// is subscription-funded) and permission to run on infrastructure the platform
// does not operate.
func hostedGenerationCaps() map[voicegov.Capability]bool {
	return map[voicegov.Capability]bool{
		voicegov.CanClone:              true,
		voicegov.CanGenerate:           true,
		voicegov.CanCommercialize:      true,
		voicegov.CanUseInConfessions:   true,
		voicegov.CanUseThirdPartyInfra: true,
	}
}

// grantRefusal is what the 451 body carries. Field names are part of the API,
// so the test reads them the way an operator's tooling would.
type grantRefusal struct {
	Error   string   `json:"error"`
	Reason  string   `json:"reason"`
	Detail  string   `json:"detail"`
	Missing []string `json:"missing"`
	JobID   string   `json:"job_id"`
}

func TestRevokingThirdPartyCapabilityRefusesHostedGeneration(t *testing.T) {
	h := newQAHarness(t)
	vp := store.NewVoicePlatformStore(h.db)
	caps := hostedGenerationCaps()
	voiceID := h.ministerVoiceWithGrant(t, caps)
	conf := h.confessionID(t)

	// 1. With the capability in place the hosted render goes through, so the
	//    refusal below cannot be explained by anything else in the fixture.
	rec := h.do(t, http.MethodPost, "/admin/audio/generate", map[string]any{
		"confession_id": conf, "voice_id": voiceID, "language": "en",
	}, h.admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("licensed render: %d %s", rec.Code, rec.Body.String())
	}
	if h.synth.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", h.synth.calls)
	}

	// 2. The rights holder withdraws permission to use third-party
	//    infrastructure. Nothing else about the licence changes.
	delete(caps, voicegov.CanUseThirdPartyInfra)
	h.setGrantCaps(t, vp, voiceID, caps)

	// 3. The same render is now refused for legal reasons, and the provider is
	//    never contacted. force makes it a fresh request rather than a replay
	//    of the idempotent one above.
	rec = h.do(t, http.MethodPost, "/admin/audio/generate", map[string]any{
		"confession_id": conf, "voice_id": voiceID, "language": "en", "force": true,
	}, h.admin)
	if rec.Code != http.StatusUnavailableForLegalReasons {
		t.Fatalf("revoked capability: status = %d, want 451 (%s)", rec.Code, rec.Body.String())
	}
	var refusal grantRefusal
	if err := json.Unmarshal(rec.Body.Bytes(), &refusal); err != nil {
		t.Fatalf("decode refusal: %v", err)
	}
	if refusal.Reason != string(voicegov.ReasonMissingCapability) {
		t.Fatalf("reason = %q, want %q", refusal.Reason, voicegov.ReasonMissingCapability)
	}
	found := false
	for _, c := range refusal.Missing {
		if c == string(voicegov.CanUseThirdPartyInfra) {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing = %v, want it to name %s", refusal.Missing, voicegov.CanUseThirdPartyInfra)
	}
	if refusal.JobID == "" {
		t.Error("a refused render must still leave a job record to audit")
	}
	if h.synth.calls != 1 {
		t.Errorf("provider calls = %d, want 1: a refusal must not reach a paid provider", h.synth.calls)
	}
}

// The granular grant is the authority even when the coarse record says
// otherwise. Without this, revoking a capability would be undone by editing
// voice_rights - the exact hole the audit found.
func TestGranularGrantOutranksAPermissiveCoarseRecord(t *testing.T) {
	h := newQAHarness(t)
	caps := hostedGenerationCaps()
	delete(caps, voicegov.CanUseThirdPartyInfra)
	voiceID := h.ministerVoiceWithGrant(t, caps)

	rec := h.do(t, http.MethodPost, "/admin/audio/generate", map[string]any{
		"confession_id": h.confessionID(t), "voice_id": voiceID, "language": "en",
	}, h.admin)
	if rec.Code != http.StatusUnavailableForLegalReasons {
		t.Fatalf("status = %d, want 451: the coarse record's AIGenerationAllowed must not "+
			"outrank a granular grant that withholds the third-party capability (%s)",
			rec.Code, rec.Body.String())
	}
	if h.synth.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", h.synth.calls)
	}
}

// A revoked grant stops the hosted path as well, through the same lifecycle the
// GPU path uses. Revocation has to be one action with one consequence.
func TestRevokedGranularGrantRefusesHostedGeneration(t *testing.T) {
	h := newQAHarness(t)
	vp := store.NewVoicePlatformStore(h.db)
	voiceID := h.ministerVoiceWithGrant(t, hostedGenerationCaps())

	if err := vp.TransitionGrant(context.Background(), voiceID, voicegov.StatusRevoked, "test", "consent withdrawn", ""); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	rec := h.do(t, http.MethodPost, "/admin/audio/generate", map[string]any{
		"confession_id": h.confessionID(t), "voice_id": voiceID, "language": "en",
	}, h.admin)
	if rec.Code != http.StatusUnavailableForLegalReasons {
		t.Fatalf("status = %d, want 451 (%s)", rec.Code, rec.Body.String())
	}
	var refusal grantRefusal
	if err := json.Unmarshal(rec.Body.Bytes(), &refusal); err != nil {
		t.Fatalf("decode refusal: %v", err)
	}
	if refusal.Reason != string(voicegov.ReasonRevoked) {
		t.Fatalf("reason = %q, want %q", refusal.Reason, voicegov.ReasonRevoked)
	}
	if h.synth.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", h.synth.calls)
	}
}

// A voice that predates the granular model keeps working: its coarse record is
// projected into a grant, so the migration does not silence voices that were
// legitimately licensed before it. The counterpart to the tests above - the
// bridge must not fail closed on the old shape.
func TestCoarseRecordIsProjectedIntoAGrant(t *testing.T) {
	h := newQAHarness(t)
	h.grantRights(t)

	rec := h.do(t, http.MethodPost, "/admin/audio/generate", map[string]any{
		"confession_id": h.confessionID(t), "voice_id": h.voiceStd, "language": "en",
	}, h.admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: a coarse licence that permits synthesis must keep "+
			"permitting it while the platform migrates (%s)", rec.Code, rec.Body.String())
	}
	if h.synth.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", h.synth.calls)
	}
}
