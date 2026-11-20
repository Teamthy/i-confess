package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/voiceengine"
)

const otherRevision = "3+sha256.bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// Planning and rendering can hit different replicas of one worker service.
// A 200/WAV from the wrong revision must not turn into a completed cache row.
func TestVoiceRejectsDifferentRenderingReplica(t *testing.T) {
	h := newVoicePlatformHarness(t)
	h.worker.reportedRevision.Store(otherRevision)
	uploads := &testUploadStore{ObjectStorage: h.store}
	h.h.SetSigner(uploads)
	voiceID := h.onboard(t)
	genID, jobID := h.submitRender(t, voiceID)
	err := h.queue.ProcessNext(context.Background())
	g := h.generation(t, genID)
	if !jobs.IsPermanent(err) || g.Status != "FAILED" || g.ErrorClass != "identity" || uploads.calls != 0 || g.StorageKey != "" {
		t.Fatalf("wrong revision: error=%v status=%s class=%s uploads=%d", err, g.Status, g.ErrorClass, uploads.calls)
	}
	if status := h.must(t, 200, "GET", "/v1/audio/jobs/"+genID, nil, h.freeTok); status["audioUrl"] != nil || status["variants"] != nil {
		t.Fatal("failed render offered for delivery")
	}
	h.worker.reportedRevision.Store(testVoiceRevision) // repair the misrouted renderer
	if _, next := h.submitRender(t, voiceID); next == jobID {
		t.Fatal("returned bad render as a cache hit")
	}
	if err := h.queue.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := h.generation(t, genID); got.Status != "COMPLETED" || got.EngineVersion != testVoiceRevision || uploads.calls != 1 {
		t.Fatal("repair did not publish exactly one verified master")
	}
}

func TestVoiceRegistrationRequiresIndependentRevision(t *testing.T) {
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)
	for _, revision := range []string{"", "latest"} {
		h.must(t, 400, "POST", "/v1/admin/voices/"+id+"/models", map[string]any{"engine": "cosyvoice", "engineVersion": revision, "modelVersion": "legacy", "licenseReviewed": true}, h.admin)
	}
}

func TestVoiceIdentityCheckedOnCacheHitAndStream(t *testing.T) {
	h := newVoicePlatformHarness(t)
	voiceID := h.onboard(t)
	req := map[string]any{"voiceId": voiceID, "text": "Grace and peace.", "purpose": "reflection"}
	h.submitRender(t, voiceID)
	if err := h.queue.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A generic Idempotency-Key replay must not hand out an old signed URL
	// before the live pin (and rights) checks. The render key already dedups.
	cached := func() *httptest.ResponseRecorder {
		b, _ := json.Marshal(req)
		r := httptest.NewRequest("POST", "/v1/voices/generate", bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+h.freeTok)
		r.Header.Set("Idempotency-Key", "repeat-the-render")
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, r)
		return w
	}
	if w := cached(); w.Code != 200 || !strings.Contains(w.Body.String(), `"audioUrl"`) {
		t.Fatal("expected a verified hit")
	}
	h.worker.capsRevision.Store(otherRevision)
	w := cached()
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"voice_not_ready"`) || strings.Contains(w.Body.String(), `"audioUrl"`) || h.worker.calls.Load() != 1 || len(h.queue.List()) != 1 {
		t.Fatalf("unverified cache hit: HTTP %d, worker calls %d", w.Code, h.worker.calls.Load())
	}
	h.worker.capsRevision.Store(testVoiceRevision)
	h.worker.reportedRevision.Store(otherRevision)
	if out := h.must(t, 503, "POST", "/v1/voices/stream", req, h.freeTok); out["code"] != "voice_not_ready" || h.worker.calls.Load() != 2 {
		t.Fatalf("bad stream was not refused before headers: %v", out)
	}
}

func (h *voicePlatformHarness) reviewedTestModel(t *testing.T, voiceID string, engine voiceengine.Engine, revision string, fallback bool) *voiceengine.Model {
	t.Helper()
	m := &voiceengine.Model{VoiceID: voiceID, Engine: engine, EngineVersion: revision, ModelVersion: "zs-2", LicenseReviewed: true}
	ctx := context.Background()
	if err := h.h.vplat.CreateModel(ctx, m, "fixture", "en", "reviewer"); err != nil {
		t.Fatal(err)
	}
	if err := h.h.vplat.SetModelEvaluation(ctx, m.ID, 0.89, "PASS", fallback, "reviewer"); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestVoicePromotionWhileQueuedRequiresNewPlan(t *testing.T) {
	h := newVoicePlatformHarness(t)
	voiceID := h.onboard(t)
	id, _ := h.submitRender(t, voiceID)
	m := h.reviewedTestModel(t, voiceID, voiceengine.EngineCosyVoice, otherRevision, false)
	h.must(t, 200, "POST", "/v1/admin/voices/"+voiceID+"/models/"+m.ID+"/promote", nil, h.admin)
	h.worker.capsRevision.Store(otherRevision) // worker AND registry now agree, but the queued key doesn't
	err := h.queue.ProcessNext(context.Background())
	g := h.generation(t, id)
	if !jobs.IsPermanent(err) || g.ErrorClass != "identity" || g.StorageKey != "" || h.worker.calls.Load() != 0 {
		t.Fatalf("replanned a queued render: error=%v status=%s calls=%d", err, g.Status, h.worker.calls.Load())
	}
}

func (h *voicePlatformHarness) registerTestFallback(t *testing.T, voiceID string) (*voiceengine.Model, *fakeGPUWorker) {
	t.Helper()
	// The training/rights register and the Go registry have historically used
	// two spellings for GPT-SoVITS. They must resolve to exactly this family.
	m := h.reviewedTestModel(t, voiceID, "gpt_sovits", otherRevision, true)
	fallback := &fakeGPUWorker{engine: voiceengine.EngineGPTSoVITS}
	fallback.capsRevision.Store(otherRevision)
	srv := httptest.NewServer(fallback)
	t.Cleanup(srv.Close)
	if err := h.h.vorch.Registry.Register(voiceengine.EngineGPTSoVITS, voiceengine.NewGPTSoVITSProvider(srv.URL, "tok")); err != nil {
		t.Fatal(err)
	}
	h.h.vorch.Fallback = voiceengine.FallbackPolicy{Enabled: true}
	return m, fallback
}

func TestVoiceVerifiedFallbackDoesNotPopulatePrimaryCache(t *testing.T) {
	h := newVoicePlatformHarness(t)
	voiceID := h.onboard(t)
	m, fallback := h.registerTestFallback(t, voiceID)
	h.worker.busyThrough.Store(1)
	ctx := context.Background()
	id, _ := h.submitRender(t, voiceID)
	primaryHash := h.generation(t, id).ContentHash
	if err := h.queue.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	g := h.generation(t, id)
	var sent voiceengine.GenerateRequest
	if err := json.Unmarshal([]byte(fallback.lastReq.Load().(string)), &sent); err != nil {
		t.Fatal(err)
	}
	if !g.FellBack || g.Status != "COMPLETED" || g.ModelID != m.ID || g.Engine != "gptsovits" || g.EngineVersion != otherRevision || g.ContentHash == primaryHash || !strings.HasSuffix(g.StorageKey, g.ContentHash+".wav") || sent.ExpectedWorker != (voiceengine.WorkerIdentity{Engine: voiceengine.EngineGPTSoVITS, Revision: otherRevision}) {
		t.Fatalf("substitution not isolated/pinned: %+v", g)
	}
	id2, _ := h.submitRender(t, voiceID)
	if id == id2 || h.generation(t, id2).ContentHash != primaryHash {
		t.Fatal("fallback occupied primary key")
	}
	if err := h.queue.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	if primary := h.generation(t, id2); primary.FellBack || primary.EngineVersion != testVoiceRevision || primary.Status != "COMPLETED" || primary.StorageKey == g.StorageKey {
		t.Fatalf("primary did not recover: %+v", primary)
	}
}
