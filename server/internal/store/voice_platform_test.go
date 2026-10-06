package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/voiceengine"
	"github.com/Teamthy/i-confess/internal/voicegov"
)

func allCaps() map[voicegov.Capability]bool {
	m := map[voicegov.Capability]bool{}
	for _, c := range voicegov.AllCapabilities {
		m[c] = true
	}
	return m
}

func newVoiceFixture(t *testing.T) (*VoicePlatformStore, string) {
	t.Helper()
	s := NewVoicePlatformStore(dbtest.New(t))
	v := &MinisterVoice{Name: "Minister Voice 001", DisplayName: "Authorized Minister", Language: "en", Locale: "en-NG", Accent: "Nigerian English", GenderPresentation: "male"}
	if err := s.CreateMinisterVoice(context.Background(), v, "Estate of Minister 001", "admin_1"); err != nil {
		t.Fatal(err)
	}
	return s, v.ID
}

func approve(t *testing.T, s *VoicePlatformStore, voiceID string, caps map[voicegov.Capability]bool) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.UpdateGrantTerms(ctx, voiceID, GrantTerms{Capabilities: caps, Languages: []string{"en"}}, "admin_1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddRightsDocument(ctx, voiceID, "voice_license", "Licence", "private/rights/x.pdf", "abc123", "admin_1"); err != nil {
		t.Fatal(err)
	}
	for _, st := range []voicegov.Status{voicegov.StatusUnderReview, voicegov.StatusApproved} {
		if err := s.TransitionGrant(ctx, voiceID, st, "admin_1", "review complete", ""); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNewVoiceAuthorizesNothing(t *testing.T) {
	s, id := newVoiceFixture(t)
	ctx := context.Background()
	g, err := s.Grant(ctx, id)
	if err != nil || g == nil {
		t.Fatalf("%v %v", g, err)
	}
	if g.Status != voicegov.StatusPending {
		t.Fatalf("status %s", g.Status)
	}
	if d := voicegov.Authorize(g, voicegov.Request{Action: voicegov.ActionGenerate}); d.Allowed {
		t.Fatal("fresh voice authorized generation")
	}
	pub, _ := s.ListMinisterVoices(ctx, true)
	if len(pub) != 0 {
		t.Fatal("unapproved voice listed publicly")
	}
}

func TestGrantRoundTripAndApproval(t *testing.T) {
	s, id := newVoiceFixture(t)
	ctx := context.Background()
	caps := allCaps()
	caps[voicegov.CanTrain] = false
	exp := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Microsecond)
	v, err := s.UpdateGrantTerms(ctx, id, GrantTerms{Capabilities: caps, ExpiresAt: &exp, Territories: []string{"NG"},
		Restrictions: []voicegov.ContentPurpose{voicegov.PurposeMarketing}, PostTermination: voicegov.AssetArchive}, "admin_1", "10.0.0.1")
	if err != nil || v != 2 {
		t.Fatalf("version %d %v", v, err)
	}
	// Approval without a document on file is refused.
	_ = s.TransitionGrant(ctx, id, voicegov.StatusUnderReview, "admin_1", "", "")
	if err := s.TransitionGrant(ctx, id, voicegov.StatusApproved, "admin_1", "", ""); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("approved without document: %v", err)
	}
	_, _ = s.AddRightsDocument(ctx, id, "voice_license", "L", "private/k", "sha", "admin_1")
	if err := s.TransitionGrant(ctx, id, voicegov.StatusApproved, "admin_1", "", ""); err != nil {
		t.Fatal(err)
	}
	g, _ := s.Grant(ctx, id)
	if g.Has(voicegov.CanTrain) || !g.Has(voicegov.CanGenerate) || g.ExpiresAt == nil || !g.ExpiresAt.Equal(exp) ||
		g.PostTermination != voicegov.AssetArchive || len(g.Restrictions) != 1 || g.Territories[0] != "NG" {
		t.Fatalf("%+v", g)
	}
	if d := voicegov.Authorize(g, voicegov.Request{Action: voicegov.ActionGenerate, Purpose: voicegov.PurposePrayer}); !d.Allowed {
		t.Fatalf("%+v", d)
	}
	if d := voicegov.Authorize(g, voicegov.Request{Action: voicegov.ActionTrain}); d.Allowed {
		t.Fatal("training allowed")
	}
	vo, _ := s.MinisterVoiceByID(ctx, id)
	if !vo.GenerationEnabled || vo.TrainingEnabled {
		t.Fatalf("flags %+v", vo)
	}
	pub, _ := s.ListMinisterVoices(ctx, true)
	if len(pub) != 1 {
		t.Fatal("approved voice not listed")
	}
}

func TestRevocationIsImmediateAndTerminal(t *testing.T) {
	s, id := newVoiceFixture(t)
	ctx := context.Background()
	approve(t, s, id, allCaps())
	if err := s.TransitionGrant(ctx, id, voicegov.StatusRevoked, "admin_2", "licensor withdrew consent", ""); err != nil {
		t.Fatal(err)
	}
	g, _ := s.Grant(ctx, id)
	if d := voicegov.Authorize(g, voicegov.Request{Action: voicegov.ActionGenerate}); d.Allowed || d.Reason != voicegov.ReasonRevoked {
		t.Fatalf("%+v", d)
	}
	vo, _ := s.MinisterVoiceByID(ctx, id)
	if vo.GenerationEnabled || vo.CloneEnabled {
		t.Fatal("flags survived revocation")
	}
	if err := s.TransitionGrant(ctx, id, voicegov.StatusApproved, "admin_1", "", ""); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("revoked grant re-approved: %v", err)
	}
	if _, err := s.UpdateGrantTerms(ctx, id, GrantTerms{Capabilities: allCaps()}, "admin_1", ""); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("revoked grant edited: %v", err)
	}
	log, _ := s.RightsAudit(ctx, id, 50)
	found := false
	for _, e := range log {
		found = found || (e.Action == "RIGHTS_STATUS_REVOKED" && e.Actor == "admin_2" && e.Detail == "licensor withdrew consent")
	}
	if !found {
		t.Fatalf("revocation not audited: %+v", log)
	}
}

func TestGrantRejectsUnknownVocabulary(t *testing.T) {
	s, id := newVoiceFixture(t)
	ctx := context.Background()
	if _, err := s.UpdateGrantTerms(ctx, id, GrantTerms{Capabilities: map[voicegov.Capability]bool{"can_everything": true}}, "a", ""); err == nil {
		t.Fatal("unknown capability accepted")
	}
	if _, err := s.UpdateGrantTerms(ctx, id, GrantTerms{Restrictions: []voicegov.ContentPurpose{"karaoke"}}, "a", ""); err == nil {
		t.Fatal("unknown purpose accepted")
	}
	if err := s.TransitionGrant(ctx, id, "APPROVED_PLEASE", "a", "", ""); err == nil {
		t.Fatal("unknown status accepted")
	}
}

func TestModelPromotionAndRollbackPersist(t *testing.T) {
	s, id := newVoiceFixture(t)
	ctx := context.Background()
	m1 := &voiceengine.Model{VoiceID: id, Engine: voiceengine.EngineCosyVoice, EngineVersion: "3", ModelVersion: "v1", LicenseReviewed: true}
	m2 := &voiceengine.Model{VoiceID: id, Engine: voiceengine.EngineCosyVoice, EngineVersion: "3", ModelVersion: "v2", LicenseReviewed: true}
	for _, m := range []*voiceengine.Model{m1, m2} {
		if err := s.CreateModel(ctx, m, "minister_001_model", "en", "ml_1"); err != nil {
			t.Fatal(err)
		}
	}
	dup := &voiceengine.Model{VoiceID: id, Engine: voiceengine.EngineCosyVoice, ModelVersion: "v1"}
	if err := s.CreateModel(ctx, dup, "x", "en", "ml_1"); err == nil {
		t.Fatal("model version overwritten")
	}
	for _, m := range []*voiceengine.Model{m1, m2} {
		if err := s.SetModelEvaluation(ctx, m.ID, 0.9, "PASS", false, "reviewer"); err != nil {
			t.Fatal(err)
		}
	}
	promote := func(target string) {
		models, _ := s.Models(ctx, id)
		ch, err := voiceengine.Promote(models, target)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ApplyModelChanges(ctx, id, ch, "admin", "MODEL_PROMOTED"); err != nil {
			t.Fatal(err)
		}
	}
	promote(m1.ID)
	time.Sleep(2 * time.Millisecond)
	promote(m2.ID)
	models, _ := s.Models(ctx, id)
	st := map[string]voiceengine.ModelStatus{}
	for _, m := range models {
		st[m.ID] = m.Status
	}
	if st[m2.ID] != voiceengine.ModelProduction || st[m1.ID] != voiceengine.ModelRetired {
		t.Fatalf("%v", st)
	}
	ch, err := voiceengine.Rollback(models, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyModelChanges(ctx, id, ch, "admin", "MODEL_ROLLED_BACK"); err != nil {
		t.Fatal(err)
	}
	models, _ = s.Models(ctx, id)
	for _, m := range models {
		if m.ID == m1.ID && m.Status != voiceengine.ModelProduction {
			t.Fatal("rollback did not restore v1")
		}
	}
	// The database itself refuses two production models.
	if _, err := s.db.ExecContext(ctx, `UPDATE voice_models SET status='production' WHERE voice_id = ?`, id); err == nil {
		t.Fatal("two production models allowed")
	}
}

func TestGenerationDedupAndLifecycle(t *testing.T) {
	s, id := newVoiceFixture(t)
	ctx := context.Background()
	g := &Generation{ContentHash: "h1", VoiceID: id, Purpose: "reflection", Style: "reflection", Language: "en", TextSHA256: "t", Queue: "tts.high", Visibility: "private"}
	a, created, err := s.CreateOrGetGeneration(ctx, g)
	if err != nil || !created {
		t.Fatalf("%v %v", created, err)
	}
	b, created, err := s.CreateOrGetGeneration(ctx, &Generation{ContentHash: "h1", VoiceID: id, Purpose: "reflection", Style: "reflection", Language: "en", TextSHA256: "t", Queue: "tts.high", Visibility: "private"})
	if err != nil || created || b.ID != a.ID {
		t.Fatalf("dedup failed: %v %v %v", b, created, err)
	}
	if err := s.AdvanceGeneration(ctx, a.ID, voiceengine.GenProcessing); err != nil {
		t.Fatal(err)
	}
	if err := s.FailGeneration(ctx, a.ID, "gpu", "oom"); err != nil {
		t.Fatal(err)
	}
	// A failed render is retried by a new identical request.
	c, created, err := s.CreateOrGetGeneration(ctx, &Generation{ContentHash: "h1", VoiceID: id, Purpose: "reflection", Style: "reflection", Language: "en", TextSHA256: "t", Queue: "tts.high", Visibility: "private"})
	if err != nil || !created || c.ID != a.ID || c.Status != "QUEUED" {
		t.Fatalf("%+v %v %v", c, created, err)
	}
	if ok, _ := s.CancelGeneration(ctx, a.ID); !ok {
		t.Fatal("queued job not cancellable")
	}
	if err := s.AdvanceGeneration(ctx, a.ID, voiceengine.GenProcessing); err == nil {
		t.Fatal("cancelled job advanced")
	}
}

func TestReferencesAndPronunciations(t *testing.T) {
	s, id := newVoiceFixture(t)
	ctx := context.Background()
	if _, err := s.AddReference(ctx, voiceengine.Reference{VoiceID: id, Style: "prayer", URI: "private/ref.wav", Transcript: "Let us pray.", DurationMS: 9000, Quality: 0.93, RightsOK: true}, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddReference(ctx, voiceengine.Reference{VoiceID: id, Style: "dubstep"}, "a"); err == nil {
		t.Fatal("unknown style accepted")
	}
	refs, _ := s.References(ctx, id)
	if len(refs) != 1 || !refs[0].RightsOK {
		t.Fatalf("%+v", refs)
	}
	if err := s.UpsertPronunciation(ctx, voiceengine.PronunciationEntry{Term: "Habakkuk", Respelling: "ha-BAK-kuk", ProviderOverrides: map[voiceengine.Engine]string{"voxcpm": "ha bak uk"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPronunciation(ctx, voiceengine.PronunciationEntry{Term: "Habakkuk", Respelling: "HAB-a-kuk"}); err != nil {
		t.Fatal(err)
	}
	entries, ver, err := s.Pronunciations(ctx)
	if err != nil || ver == "" {
		t.Fatalf("pronunciations: %d entries version %q err %v", len(entries), ver, err)
	}

	// The dictionary is no longer empty at rest: migration 0031 ships the
	// en-NG seed (VE-005). The assertions are therefore scoped to the rows
	// this test wrote, and they pin the two properties the seed relies on:
	// an upsert replaces only its own (term, locale), and the seeded en-NG
	// entry for the same term survives untouched beside it.
	if len(entries) < 150 {
		t.Fatalf("seeded dictionary missing: only %d entries", len(entries))
	}
	var universal, seededNG bool
	for _, e := range entries {
		if e.Term != "Habakkuk" {
			continue
		}
		switch e.Locale {
		case "":
			if universal {
				t.Errorf("two universal Habakkuk rows: %+v", e)
			}
			universal = true
			if e.Respelling != "HAB-a-kuk" {
				t.Errorf("universal Habakkuk respelling = %q, want the later upsert to win", e.Respelling)
			}
		case "en-NG":
			if seededNG {
				t.Errorf("two en-NG Habakkuk rows: %+v", e)
			}
			seededNG = true
			if e.Respelling == "" || e.Respelling == "HAB-a-kuk" {
				t.Errorf("seeded en-NG Habakkuk was overwritten by the universal upsert: %+v", e)
			}
		}
	}
	if !universal || !seededNG {
		t.Fatalf("Habakkuk rows: universal=%v en-NG=%v, want both", universal, seededNG)
	}
}
