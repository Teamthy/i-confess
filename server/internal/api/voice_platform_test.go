package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/voiceengine"
	"github.com/Teamthy/i-confess/internal/voiceeval"
)

// fakeGPUWorker speaks the voice-engine worker wire protocol. It records every
// synthesize call so tests can prove what did (and did not) reach inference.
type fakeGPUWorker struct {
	calls   atomic.Int32
	lastReq atomic.Value // string
	// reportCosts mirrors a real worker's X-Inference-Seconds / X-Worker-Seconds
	// headers. Cleared in one test to cover the other half of the contract: a
	// worker that measures nothing must leave the cost columns NULL, not 0.
	reportCosts bool
}

func (f *fakeGPUWorker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/health":
		w.Write([]byte(`{"status":"ok"}`))
	case "/v1/capabilities":
		w.WriteHeader(http.StatusNotFound) // adapter falls back to its defaults
	case "/v1/synthesize":
		b, _ := io.ReadAll(r.Body)
		f.lastReq.Store(string(b))
		f.calls.Add(1)
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("X-Sample-Rate", "8000")
		w.Header().Set("X-Duration-Ms", "3000")
		w.Header().Set("X-Engine-Version", "3.0-test")
		if f.reportCosts {
			w.Header().Set("X-Inference-Seconds", "2.5")
			w.Header().Set("X-Worker-Seconds", "3.25")
		}
		w.Write(synthWAV(3))
	default:
		http.NotFound(w, r)
	}
}

type voicePlatformHarness struct {
	*qaHarness
	worker *fakeGPUWorker
	queue  *jobs.MemoryQueue
	docID  string
	ws     *httptest.Server
}

func newVoicePlatformHarness(t *testing.T) *voicePlatformHarness {
	t.Helper()
	qh := newQAHarness(t)
	worker := &fakeGPUWorker{reportCosts: true}
	ws := httptest.NewServer(worker)
	t.Cleanup(ws.Close)

	reg := voiceengine.NewRegistry(false)
	if err := reg.Register(voiceengine.EngineCosyVoice, voiceengine.NewCosyVoiceProvider(ws.URL, "tok")); err != nil {
		t.Fatal(err)
	}
	q := jobs.NewMemoryQueue()
	qh.h.SetQueue(q)
	qh.h.SetVoiceOrchestrator(&voiceengine.Orchestrator{Registry: reg})
	qh.h.SetVoiceThresholds(voiceeval.Thresholds{MinScore: 0.7, MinCoverage: 0.8, MaxRegression: 0.03})
	qh.h.RegisterVoiceJobs()
	return &voicePlatformHarness{qaHarness: qh, worker: worker, queue: q, ws: ws}
}

func (h *voicePlatformHarness) must(t *testing.T, want int, method, path string, body any, tok string) map[string]any {
	t.Helper()
	rec := h.do(t, method, path, body, tok)
	if rec.Code != want {
		t.Fatalf("%s %s: status %d, want %d; body %s", method, path, rec.Code, want, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

// onboard walks a minister voice through the full governed lifecycle over the
// API: register -> terms -> document -> review -> approve -> reference ->
// model -> evaluation gate -> promotion.
func (h *voicePlatformHarness) onboard(t *testing.T, extraCaps ...string) string {
	t.Helper()
	v := h.must(t, 201, "POST", "/v1/admin/minister-voices",
		map[string]any{"name": "Minister A", "language": "en", "locale": "en-NG", "rightsHolder": "Minister A Ministries"}, h.admin)
	id := v["voice"].(map[string]any)["id"].(string)

	caps := map[string]bool{}
	for _, c := range []string{"can_record", "can_store_recordings", "can_process_recordings", "can_extract_voice",
		"can_create_embedding", "can_clone", "can_generate", "can_stream", "can_use_in_reflections",
		"can_use_in_confessions", "can_use_user_submitted_text", "can_store_model_checkpoints", "can_distribute", "can_commercialize"} {
		caps[c] = true
	}
	for _, c := range extraCaps {
		caps[c] = true
	}
	h.must(t, 200, "PUT", "/v1/admin/voices/"+id+"/grant", map[string]any{
		"rightsHolder": "Minister A Ministries", "capabilities": caps, "attestation": true}, h.admin)
	doc := h.must(t, 201, "POST", "/v1/admin/voices/"+id+"/rights-documents", map[string]any{
		"documentType": "voice_license", "title": "Signed voice licence", "storageKey": "legal/a.pdf",
		"sha256": strings.Repeat("ab", 32)}, h.admin)
	h.docID = doc["documentId"].(string)
	h.must(t, 200, "POST", "/v1/admin/voices/"+id+"/review", nil, h.admin)
	h.must(t, 200, "POST", "/v1/admin/voices/"+id+"/approve", nil, h.admin)

	h.must(t, 201, "POST", "/v1/admin/voices/"+id+"/references", map[string]any{
		"style": "reflection", "audioKey": "refs/a/reflection.wav", "transcript": "Be still and know.",
		"durationMs": 8000, "quality": 0.9, "rightsOk": true}, h.admin)
	m := h.must(t, 201, "POST", "/v1/admin/voices/"+id+"/models", map[string]any{
		"engine": "cosyvoice", "engineVersion": "3", "modelVersion": "zs-1", "mode": "zero_shot",
		"licenseReviewed": true, "checkpointKey": "private/ckpt/zs-1"}, h.admin)
	modelID := m["model"].(map[string]any)["id"].(string)

	metrics := map[string]float64{}
	for _, d := range []string{"speaker_similarity", "naturalness", "pronunciation", "accent_preservation", "prosody",
		"emotional_consistency", "style_consistency", "long_form_stability", "intelligibility", "text_accuracy", "artifact_score"} {
		metrics[d] = 0.85
	}
	ev := h.must(t, 200, "POST", "/v1/admin/voices/"+id+"/evaluate", map[string]any{
		"modelId": modelID, "goldenSet": "golden-v1", "metrics": metrics,
		"humanApproved": true, "blindEvalCompleted": true}, h.admin)
	if v := ev["gate"].(map[string]any)["verdict"]; v != "PASS" {
		t.Fatalf("gate verdict %v: %v", v, ev["gate"])
	}
	h.must(t, 200, "POST", "/v1/admin/voices/"+id+"/models/"+modelID+"/promote", nil, h.admin)
	return id
}

func TestMinisterVoiceGenerationEndToEnd(t *testing.T) {
	h := newVoicePlatformHarness(t)
	ctx := context.Background()

	// A registered-but-unapproved voice authorizes nothing.
	v := h.must(t, 201, "POST", "/v1/admin/minister-voices",
		map[string]any{"name": "Pending Minister", "rightsHolder": "X"}, h.admin)
	pending := v["voice"].(map[string]any)["id"].(string)
	h.must(t, 403, "POST", "/v1/voices/generate", map[string]any{
		"voiceId": pending, "text": "Grace and peace.", "style": "reflection", "purpose": "reflection"}, h.freeTok)

	id := h.onboard(t)
	req := map[string]any{"voiceId": id, "text": "Grace and peace to you.", "style": "reflection", "purpose": "reflection"}

	out := h.must(t, 202, "POST", "/v1/voices/generate", req, h.freeTok)
	if out["synthetic"] != true {
		t.Fatalf("generation response must be marked synthetic: %v", out)
	}
	genID := out["generation"].(map[string]any)["generationId"].(string)
	if h.worker.calls.Load() != 0 {
		t.Fatal("the API process must not run inference; only the queued worker may")
	}
	if err := h.queue.ProcessNext(ctx); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if h.worker.calls.Load() != 1 {
		t.Fatalf("worker calls = %d, want 1", h.worker.calls.Load())
	}
	job := h.must(t, 200, "GET", "/v1/audio/jobs/"+genID, nil, h.freeTok)
	if st := job["generation"].(map[string]any)["status"]; st != "COMPLETED" {
		t.Fatalf("status %v: %v", st, job)
	}
	if job["audioUrl"] == nil || job["audioUrl"] == "" {
		t.Fatalf("completed job must carry a signed URL: %v", job)
	}

	// Private (user-text) renders are invisible to other users.
	other := h.register(t, "other@test.com")
	if rec := h.do(t, "GET", "/v1/audio/jobs/"+genID, nil, other); rec.Code == 200 {
		t.Fatal("another user could read a private render")
	}

	// Identical request: served from the content-hash cache, no new inference.
	h.must(t, 200, "POST", "/v1/voices/generate", req, h.freeTok)
	if h.worker.calls.Load() != 1 {
		t.Fatal("cache hit must not reach the GPU worker")
	}
	if strings.Contains(h.worker.lastReq.Load().(string), "Minister A Ministries") {
		t.Fatal("legal detail leaked to the worker")
	}

	// Revocation stops generation immediately, including queued work.
	h.must(t, 202, "POST", "/v1/voices/generate",
		map[string]any{"voiceId": id, "text": "Queued before revocation.", "style": "reflection", "purpose": "reflection"}, h.freeTok)
	h.must(t, 200, "POST", "/v1/admin/voices/"+id+"/revoke", map[string]any{"reason": "licence terminated"}, h.admin)
	_ = h.queue.ProcessNext(ctx)
	if h.worker.calls.Load() != 1 {
		t.Fatal("a job queued before revocation still reached inference")
	}
	h.must(t, 403, "POST", "/v1/voices/generate",
		map[string]any{"voiceId": id, "text": "After revocation.", "style": "reflection", "purpose": "reflection"}, h.freeTok)

	audit := h.must(t, 200, "GET", "/v1/admin/voices/"+id+"/audit", nil, h.admin)
	if b, _ := json.Marshal(audit); !strings.Contains(string(b), "REVOKED") {
		t.Fatalf("revocation not in audit log: %s", b)
	}
}

func TestVoiceGenerationRequiresRoleForAdminRoutes(t *testing.T) {
	h := newVoicePlatformHarness(t)
	if rec := h.do(t, "POST", "/v1/admin/minister-voices",
		map[string]any{"name": "X", "rightsHolder": "Y"}, h.freeTok); rec.Code != http.StatusForbidden {
		t.Fatalf("listener registered a voice: %d", rec.Code)
	}
}

func TestVoiceGenerationWithoutEngineIs503(t *testing.T) {
	h := newQAHarness(t)
	rec := h.do(t, "POST", "/v1/voices/generate", map[string]any{"voiceId": "x", "text": "hi", "purpose": "reflection"}, h.freeTok)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
}
