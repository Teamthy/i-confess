package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/voice"
)

// stubSynth is a provider that returns real, inspectable audio. The pipeline
// measures what a provider hands back, so a stub returning arbitrary bytes
// would no longer represent a provider that worked.
type stubSynth struct{ calls int }

func (s *stubSynth) Name() string { return "stub" }

func (s *stubSynth) Synthesize(context.Context, voice.SynthesisRequest) (*voice.SynthesisResult, error) {
	s.calls++
	return &voice.SynthesisResult{
		Audio: synthWAV(3), ContentType: "audio/wav",
		Codec: "pcm", BitrateKbps: 128, SampleRate: 8000, DurationSeconds: 3,
	}, nil
}

func synthWAV(seconds int) []byte {
	const rate, ch, bits = 8000, 1, 16
	byteRate := uint32(rate * ch * bits / 8)
	data := byteRate * uint32(seconds)

	b := []byte("RIFF")
	b = binary.LittleEndian.AppendUint32(b, 4+24+8+data)
	b = append(b, "WAVE"...)
	b = append(b, "fmt "...)
	b = binary.LittleEndian.AppendUint32(b, 16)
	b = binary.LittleEndian.AppendUint16(b, 1)
	b = binary.LittleEndian.AppendUint16(b, ch)
	b = binary.LittleEndian.AppendUint32(b, rate)
	b = binary.LittleEndian.AppendUint32(b, byteRate)
	b = binary.LittleEndian.AppendUint16(b, ch*bits/8)
	b = binary.LittleEndian.AppendUint16(b, bits)
	b = append(b, "data"...)
	b = binary.LittleEndian.AppendUint32(b, data)
	return append(b, make([]byte, data)...)
}

// qaHarness is the audio fixture plus a generation pipeline and an admin token.
type qaHarness struct {
	*audioFixture
	admin string
	synth *stubSynth
}

func newQAHarness(t *testing.T) *qaHarness {
	t.Helper()
	f := newAudioFixture(t)

	synth := &stubSynth{}
	f.h.SetPipeline(voice.NewPipeline(synth, f.store))

	admin := createAdminUser(t, f.db, f.h, f.router)
	return &qaHarness{audioFixture: f, admin: admin, synth: synth}
}

// grantRights gives the standard voice a licence permitting synthesis. Without
// one the rights gate refuses, which is the point of the gate.
func (h *qaHarness) grantRights(t *testing.T) {
	t.Helper()
	vrs := store.NewVoiceRightsStore(h.db)
	if err := vrs.Create(context.Background(), &models.VoiceRights{
		VoiceID: h.voiceStd, RightsHolder: "i-confess", AllowedUse: "tts",
		Territories: "GLOBAL", Status: "active", LicenseStatus: "active",
		CommercialUse: true, AIGenerationAllowed: true, ProviderVoiceID: "stub-voice-1",
	}); err != nil {
		t.Fatalf("grant rights: %v", err)
	}
}

func (h *qaHarness) do(t *testing.T, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

func (h *qaHarness) confessionID(t *testing.T) string {
	t.Helper()
	ids := h.confessionIDs(t)
	if len(ids) == 0 {
		t.Fatal("no confessions in the fixture")
	}
	return ids[0]
}

func (h *qaHarness) confessionIDs(t *testing.T) []string {
	t.Helper()
	rows, err := h.db.QueryContext(context.Background(), `SELECT id FROM confessions ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

// TestGenerationRecordsAJobAndEntersQA covers the directive's "generation
// requests", "generation status" and "QA workflow" end to end.
//
// Before this, a generation was a synchronous provider call with no record: no
// status to poll, nothing to retry, and the asset it produced entered
// 'processing' with no way out, because the codebase had no UPDATE audio_assets
// statement at all.
func TestGenerationRecordsAJobAndEntersQA(t *testing.T) {
	h := newQAHarness(t)
	h.grantRights(t)
	confID := h.confessionID(t)

	rec := h.do(t, http.MethodPost, "/admin/audio/generate", map[string]any{
		"confession_id": confID, "voice_id": h.voiceStd, "language": "en",
	}, h.admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("generate: %d %s", rec.Code, rec.Body.String())
	}
	if h.synth.calls != 1 {
		t.Errorf("provider calls = %d, want 1", h.synth.calls)
	}

	var out struct {
		Job struct {
			ID           string `json:"id"`
			Status       string `json:"status"`
			Provider     string `json:"provider"`
			AttemptCount int    `json:"attempt_count"`
			AudioAssetID string `json:"audio_asset_id"`
			RequestedBy  string `json:"requested_by"`
			StartedAt    string `json:"started_at"`
			CompletedAt  string `json:"completed_at"`
		} `json:"job"`
		Asset struct {
			ID               string `json:"id"`
			Status           string `json:"status"`
			DurationSeconds  int    `json:"duration_seconds"`
			ContentVersionID string `json:"content_version_id"`
		} `json:"asset"`
		ContentVersion int `json:"content_version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}

	if out.Job.ID == "" {
		t.Fatal("no job was recorded; the request is unobservable")
	}
	if out.Job.Status != "succeeded" {
		t.Errorf("job status = %q, want succeeded", out.Job.Status)
	}
	if out.Job.Provider != "stub" {
		t.Errorf("job provider = %q, want the adapter that did the work", out.Job.Provider)
	}
	if out.Job.AttemptCount != 1 || out.Job.StartedAt == "" || out.Job.CompletedAt == "" {
		t.Errorf("job timing was not recorded: %+v", out.Job)
	}
	if out.Job.AudioAssetID != out.Asset.ID {
		t.Errorf("the job was not linked to its asset: %q vs %q", out.Job.AudioAssetID, out.Asset.ID)
	}
	if out.ContentVersion != 1 {
		t.Errorf("content_version = %d, want 1: the exact text must be snapshotted", out.ContentVersion)
	}
	if out.Asset.ContentVersionID == "" {
		t.Error("the asset does not record which text it speaks")
	}

	// Generated audio enters QA, not the catalogue.
	if out.Asset.Status != "processing" {
		t.Errorf("asset status = %q, want processing", out.Asset.Status)
	}
	// Measured from the container, not taken from the provider's word.
	if out.Asset.DurationSeconds != 3 {
		t.Errorf("duration = %d, want 3 (measured)", out.Asset.DurationSeconds)
	}

	// The job is now pollable, which is what "generation status" means.
	list := h.do(t, http.MethodGet, "/admin/audio/jobs", nil, h.admin)
	if list.Code != http.StatusOK {
		t.Fatalf("list jobs: %d", list.Code)
	}
	var jobs []models.AudioJob
	if err := json.Unmarshal(list.Body.Bytes(), &jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != out.Job.ID {
		t.Fatalf("job list = %+v, want the one job %q", jobs, out.Job.ID)
	}

	one := h.do(t, http.MethodGet, "/admin/audio/jobs/"+out.Job.ID, nil, h.admin)
	if one.Code != http.StatusOK {
		t.Fatalf("get job: %d", one.Code)
	}

	// A bad status filter is rejected rather than silently ignored.
	if bad := h.do(t, http.MethodGet, "/admin/audio/jobs?status=bogus", nil, h.admin); bad.Code != http.StatusBadRequest {
		t.Errorf("?status=bogus -> %d, want 400", bad.Code)
	}

	// The fixture seeded its assets directly, so they carry no content version.
	// In production every asset comes from generation and always has one, which
	// is what the unique key treats as "the same render". Removing the seeded
	// rows makes this model production rather than the test's own shortcut.
	if _, err := h.db.ExecContext(context.Background(),
		`DELETE FROM audio_assets WHERE content_version_id IS NULL`); err != nil {
		t.Fatal(err)
	}

	// The fixture holds two confessions, and a session could legitimately be
	// built from the one whose audio was untouched. Rather than hope the
	// generated asset gets selected, put every confession's audio through
	// generation so the whole category is in QA - then nothing may be served.
	for _, id := range h.confessionIDs(t) {
		if id == confID {
			continue
		}
		if rec := h.do(t, http.MethodPost, "/admin/audio/generate", map[string]any{
			"confession_id": id, "voice_id": h.voiceStd, "language": "en",
		}, h.admin); rec.Code != http.StatusCreated {
			t.Fatalf("generate for %s: %d %s", id, rec.Code, rec.Body.String())
		}
	}

	// Stronger than locking items at playback: audio still in QA is invisible to
	// session building, so a session cannot be composed from it at all. The
	// engine selects only servable statuses, and 'processing' is not one.
	if code, _ := h.createSession(t, h.freeTok, h.voiceStd, 120); code != http.StatusUnprocessableEntity {
		t.Errorf("session built from audio still in QA: %d, want 422", code)
	}

	// Approving is what makes it servable, and it records who decided.
	approve := h.do(t, http.MethodPost, "/admin/audio/"+out.Asset.ID+"/qa/approve",
		map[string]any{"note": "clean render"}, h.admin)
	if approve.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", approve.Code, approve.Body.String())
	}
	var approved models.AudioAsset
	json.Unmarshal(approve.Body.Bytes(), &approved)
	if approved.Status != "ready" {
		t.Errorf("status after approval = %q, want ready", approved.Status)
	}
	if approved.QAReviewedBy == "" {
		t.Error("the approval was not attributed to anyone")
	}

	_, after := h.createSession(t, h.freeTok, h.voiceStd, 120)
	if n := countServed(after); n == 0 {
		t.Error("an approved render is still not servable")
	}
}

// TestGenerationIsRefusedWithoutVoiceRights covers "no unauthorized voice
// generation". A voice with no licence record must never reach the provider -
// and the refusal must be recorded on the job, not just returned.
func TestGenerationIsRefusedWithoutVoiceRights(t *testing.T) {
	h := newQAHarness(t) // no rights granted
	confID := h.confessionID(t)

	rec := h.do(t, http.MethodPost, "/admin/audio/generate", map[string]any{
		"confession_id": confID, "voice_id": h.voiceStd, "language": "en",
	}, h.admin)

	if rec.Code != http.StatusUnavailableForLegalReasons {
		t.Fatalf("status = %d, want 451: %s", rec.Code, rec.Body.String())
	}
	if h.synth.calls != 0 {
		t.Errorf("the provider was called %d times; an unlicensed voice must never be synthesized", h.synth.calls)
	}

	// The refused request is still visible, with the reason.
	list := h.do(t, http.MethodGet, "/admin/audio/jobs?status=failed", nil, h.admin)
	var jobs []models.AudioJob
	json.Unmarshal(list.Body.Bytes(), &jobs)
	if len(jobs) != 1 {
		t.Fatalf("expected the refused request to be recorded, got %+v", jobs)
	}
	if jobs[0].ErrorCode == "" {
		t.Error("the refusal was recorded with no reason")
	}
}

// TestQATransitionsAreEnforcedOverTheAPI covers the workflow's rules, including
// the one that matters most: a human rejection cannot be quietly overridden.
func TestQATransitionsAreEnforcedOverTheAPI(t *testing.T) {
	h := newQAHarness(t)
	h.grantRights(t)
	confID := h.confessionID(t)

	rec := h.do(t, http.MethodPost, "/admin/audio/generate", map[string]any{
		"confession_id": confID, "voice_id": h.voiceStd, "language": "en",
	}, h.admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("generate: %d", rec.Code)
	}
	var out struct {
		Asset struct {
			ID string `json:"id"`
		} `json:"asset"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	id := out.Asset.ID

	// A rejection with no reason cannot be acted on.
	noNote := h.do(t, http.MethodPost, "/admin/audio/"+id+"/qa/reject", map[string]any{}, h.admin)
	if noNote.Code != http.StatusUnprocessableEntity {
		t.Errorf("reject without a note -> %d, want 422", noNote.Code)
	}

	rejected := h.do(t, http.MethodPost, "/admin/audio/"+id+"/qa/reject",
		map[string]any{"note": "clipping on the final phrase"}, h.admin)
	if rejected.Code != http.StatusOK {
		t.Fatalf("reject: %d %s", rejected.Code, rejected.Body.String())
	}

	// The move that must not exist: rejected straight to servable.
	override := h.do(t, http.MethodPost, "/admin/audio/"+id+"/qa/approve", nil, h.admin)
	if override.Code != http.StatusConflict {
		t.Errorf("approving a rejected render -> %d, want 409", override.Code)
	}
	if !bytes.Contains(override.Body.Bytes(), []byte("qa_rejected")) {
		t.Errorf("409 should name the current state, got: %s", override.Body.String())
	}

	// Archiving works, and is how an asset is pulled from sessions that already
	// reference it, because the access check reads the current status.
	archived := h.do(t, http.MethodPost, "/admin/audio/"+id+"/archive",
		map[string]any{"note": "licence lapsed"}, h.admin)
	if archived.Code != http.StatusOK {
		t.Fatalf("archive: %d %s", archived.Code, archived.Body.String())
	}
	// Archived is terminal.
	if again := h.do(t, http.MethodPost, "/admin/audio/"+id+"/qa/approve", nil, h.admin); again.Code != http.StatusConflict {
		t.Errorf("approving an archived render -> %d, want 409", again.Code)
	}

	// An unknown asset is a 404, not a 409 or a 500.
	if missing := h.do(t, http.MethodPost, "/admin/audio/no-such-id/qa/approve", nil, h.admin); missing.Code != http.StatusNotFound {
		t.Errorf("unknown asset -> %d, want 404", missing.Code)
	}
}

// TestRepeatGenerationIsNotBilledTwice covers the idempotency key: a
// double-submitted form or a client retry must not pay the provider again.
func TestRepeatGenerationIsNotBilledTwice(t *testing.T) {
	h := newQAHarness(t)
	h.grantRights(t)
	confID := h.confessionID(t)

	body := map[string]any{"confession_id": confID, "voice_id": h.voiceStd, "language": "en"}
	first := h.do(t, http.MethodPost, "/admin/audio/generate", body, h.admin)
	if first.Code != http.StatusCreated {
		t.Fatalf("first: %d", first.Code)
	}
	if h.synth.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", h.synth.calls)
	}

	second := h.do(t, http.MethodPost, "/admin/audio/generate", body, h.admin)
	if second.Code != http.StatusOK {
		t.Fatalf("repeat: %d, want 200 (the existing job), body %s", second.Code, second.Body.String())
	}
	if h.synth.calls != 1 {
		t.Errorf("provider calls = %d after a repeated request; the provider was billed twice for one render", h.synth.calls)
	}
	var reused struct {
		Reused bool `json:"reused"`
		Job    struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	json.Unmarshal(second.Body.Bytes(), &reused)
	if !reused.Reused || reused.Job.ID == "" {
		t.Errorf("the repeat did not return the original job: %+v", reused)
	}

	// Force is an explicit request to render again, so it does reach the provider.
	forced := map[string]any{"confession_id": confID, "voice_id": h.voiceStd, "language": "en", "force": true}
	if rec := h.do(t, http.MethodPost, "/admin/audio/generate", forced, h.admin); rec.Code != http.StatusCreated {
		t.Fatalf("forced: %d %s", rec.Code, rec.Body.String())
	}
	if h.synth.calls != 2 {
		t.Errorf("provider calls = %d, want 2 after an explicit force", h.synth.calls)
	}
}

// TestVoiceRightsGrantSurvivesARoundTrip covers the headline PHASE 14 finding.
//
// PUT /admin/voices/{id}/rights already accepted ai_generation_allowed,
// commercial_use and provider_voice_id, checked the attestation, and returned
// 200 with an audit entry. But voice_rights had twelve columns and none of
// those fields were among them, so Create and Update dropped them and ByVoiceID
// read back zero values. rights.Evaluate denies UseSynthesis unless
// AIGenerationAllowed is true, so every licence read as forbidding AI
// generation - and no sequence of writes could make it otherwise.
//
// The gate was not too permissive. It was unsatisfiable: an admin could grant
// permission, be told it was granted, and the grant would not exist.
func TestVoiceRightsGrantSurvivesARoundTrip(t *testing.T) {
	h := newQAHarness(t)
	confID := h.confessionID(t)

	// Generation must fail before the grant exists.
	before := h.do(t, http.MethodPost, "/admin/audio/generate", map[string]any{
		"confession_id": confID, "voice_id": h.voiceStd, "language": "en",
	}, h.admin)
	if before.Code != http.StatusUnavailableForLegalReasons {
		t.Fatalf("generation before any grant: %d, want 451", before.Code)
	}

	put := h.do(t, http.MethodPut, "/admin/voices/"+h.voiceStd+"/rights", map[string]any{
		"rights_holder":         "i-confess",
		"allowed_use":           "tts",
		"territories":           "GLOBAL",
		"status":                "active",
		"commercial_use":        true,
		"ai_generation_allowed": true,
		"provider":              "stub",
		"provider_voice_id":     "stub-voice-1",
		"rights_attestation":    "signed voice agreement 2026-01-14",
	}, h.admin)
	if put.Code != http.StatusOK {
		t.Fatalf("grant rights: %d %s", put.Code, put.Body.String())
	}

	// Read it back through the store: this is the path the rights gate uses.
	vrs := store.NewVoiceRightsStore(h.db)
	vr, err := vrs.ByVoiceID(context.Background(), h.voiceStd)
	if err != nil {
		t.Fatalf("ByVoiceID: %v", err)
	}
	if !vr.AIGenerationAllowed {
		t.Error("ai_generation_allowed was accepted by the API and lost by the store")
	}
	if !vr.CommercialUse {
		t.Error("commercial_use was accepted by the API and lost by the store")
	}
	if vr.ProviderVoiceID != "stub-voice-1" {
		t.Errorf("provider_voice_id = %q, want the one that was granted", vr.ProviderVoiceID)
	}

	// And the grant must actually take effect at the gate.
	after := h.do(t, http.MethodPost, "/admin/audio/generate", map[string]any{
		"confession_id": confID, "voice_id": h.voiceStd, "language": "en",
	}, h.admin)
	if after.Code != http.StatusCreated {
		t.Fatalf("generation after granting rights: %d, want 201: %s", after.Code, after.Body.String())
	}
	if h.synth.calls != 1 {
		t.Errorf("provider calls = %d, want 1", h.synth.calls)
	}
}

// TestRightsUpdateAlsoPersistsTheFlags covers the update path, which dropped the
// same fields as create.
func TestRightsUpdateAlsoPersistsTheFlags(t *testing.T) {
	h := newQAHarness(t)
	h.grantRights(t)

	// Revoke AI generation through the API, then confirm the gate honours it.
	put := h.do(t, http.MethodPut, "/admin/voices/"+h.voiceStd+"/rights", map[string]any{
		"rights_holder":         "i-confess",
		"allowed_use":           "tts",
		"territories":           "GLOBAL",
		"status":                "active",
		"commercial_use":        true,
		"ai_generation_allowed": false,
		"provider_voice_id":     "stub-voice-1",
	}, h.admin)
	if put.Code != http.StatusOK {
		t.Fatalf("update rights: %d %s", put.Code, put.Body.String())
	}

	vrs := store.NewVoiceRightsStore(h.db)
	vr, err := vrs.ByVoiceID(context.Background(), h.voiceStd)
	if err != nil {
		t.Fatal(err)
	}
	if vr.AIGenerationAllowed {
		t.Error("an update could not withdraw AI generation; the flag did not persist")
	}

	rec := h.do(t, http.MethodPost, "/admin/audio/generate", map[string]any{
		"confession_id": h.confessionID(t), "voice_id": h.voiceStd, "language": "en",
	}, h.admin)
	if rec.Code != http.StatusUnavailableForLegalReasons {
		t.Errorf("generation after revoking AI rights: %d, want 451", rec.Code)
	}
}
