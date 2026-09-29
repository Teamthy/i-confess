package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Teamthy/i-confess/internal/voicedata"
	"github.com/Teamthy/i-confess/internal/voiceengine"
)

// fakeIntakeWorker implements /v1/ingest, /v1/train* and /v1/synthesize/stream.
type fakeIntakeWorker struct {
	ingests atomic.Int32
	polls   atomic.Int32
	trained atomic.Value // TrainRequest
	cancels atomic.Int32
	next    http.Handler
}

func (f *fakeIntakeWorker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/v1/ingest":
		f.ingests.Add(1)
		var req voiceengine.IngestRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		var segs []voicedata.Segment
		snr := 34.0
		for i := 0; i < 12; i++ {
			segs = append(segs, voicedata.Segment{StartMS: i * 6000, EndMS: i*6000 + 5000,
				AudioKey: fmt.Sprintf("%s/%03d.wav", req.SegmentPrefix, i), RawTranscript: "asr guess", SNRDB: &snr, Quality: 0.9})
		}
		segs = append(segs, voicedata.Segment{StartMS: 90000, EndMS: 90300, AudioKey: req.SegmentPrefix + "/short.wav", Quality: 0.9})
		json.NewEncoder(w).Encode(map[string]any{"duration_ms": 95000, "segments": segs,
			"report": map[string]any{"vad": "energy", "asr": "not_run", "diarization": "reference_similarity"}})
	case r.URL.Path == "/v1/train":
		var req voiceengine.TrainRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.trained.Store(req)
		w.WriteHeader(http.StatusAccepted)
	case strings.HasSuffix(r.URL.Path, "/cancel"):
		f.cancels.Add(1)
		w.WriteHeader(http.StatusOK)
	case strings.HasPrefix(r.URL.Path, "/v1/train/"):
		if f.polls.Add(1) < 2 {
			w.Write([]byte(`{"status":"RUNNING","progress":0.4,"loss":1.2}`))
			return
		}
		req := f.trained.Load().(voiceengine.TrainRequest)
		json.NewEncoder(w).Encode(map[string]any{"status": "COMPLETED", "progress": 1.0, "loss": 0.31,
			"checkpoint_key": req.CheckpointKey + "/model.pth", "checkpoint_sha256": strings.Repeat("c", 64),
			"hardware": "1x test-gpu", "duration_seconds": 42})
	case r.URL.Path == "/v1/synthesize/stream":
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("X-Sample-Rate", "8000")
		w.Write([]byte("RIFF-stream-chunk-1"))
		w.(http.Flusher).Flush()
		w.Write([]byte("chunk-2"))
	case r.URL.Path == "/v1/capabilities":
		// Declare streaming so the orchestrator allows it.
		json.NewEncoder(w).Encode(voiceengine.ProviderCapabilities{Streaming: true, ZeroShot: true,
			Languages: []string{"en"}, MaxChunkChars: 300, SelfHosted: true})
	default:
		f.next.ServeHTTP(w, r)
	}
}

func newIntakeHarness(t *testing.T) (*voicePlatformHarness, *fakeIntakeWorker) {
	t.Helper()
	h := newVoicePlatformHarness(t)
	fw := &fakeIntakeWorker{next: h.worker}
	h.ws.Config.Handler = fw // same worker URL serves synthesis and intake
	h.h.SetVoiceWorker(voiceengine.NewWorkerClient(h.ws.URL, "tok", 0), VoiceIntakeConfig{
		FineTune: voicedata.FineTunePolicy{MinUsableSeconds: 30, ZeroShotSufficientScore: 0.8},
	})
	h.h.RegisterVoiceIntakeJobs()
	return h, fw
}

func (h *voicePlatformHarness) upload(t *testing.T, voiceID string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "sermon.wav")
	fw.Write(synthWAV(2))
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	mw.Close()
	req := httptest.NewRequest("POST", "/v1/admin/voices/"+voiceID+"/recordings", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+h.admin)
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

func (h *voicePlatformHarness) drain(t *testing.T) {
	t.Helper()
	for i := 0; i < 10; i++ {
		if err := h.queue.ProcessNext(context.Background()); err != nil {
			t.Fatalf("job failed: %v", err)
		}
	}
}

func TestIntakeToTrainedCandidateEndToEnd(t *testing.T) {
	h, fw := newIntakeHarness(t)
	id := h.onboard(t, "can_train", "can_fine_tune", "can_create_derivative_models")

	fields := map[string]string{"sourceType": "studio_session", "title": "Sunday sermon", "rightsDocumentId": h.docID,
		"processingPermission": "true", "trainingAllowed": "true"}
	if rec := h.upload(t, id, fields); rec.Code != http.StatusBadRequest {
		t.Fatalf("upload without source attestation: %d", rec.Code)
	}
	fields["sourceAttestation"] = "true"
	rec := h.upload(t, id, fields)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	var up map[string]any
	json.Unmarshal(rec.Body.Bytes(), &up)
	recID := up["recording"].(map[string]any)["id"].(string)
	if strings.Contains(rec.Body.String(), "private/voices") {
		t.Fatal("storage key leaked in API response")
	}
	h.drain(t)
	if fw.ingests.Load() != 1 {
		t.Fatalf("ingests=%d", fw.ingests.Load())
	}
	got := h.must(t, 200, "GET", "/v1/admin/recordings/"+recID, nil, h.admin)["recording"].(map[string]any)
	if got["status"] != "processed" || got["report"].(map[string]any)["asr"] != "not_run" {
		t.Fatalf("recording %v", got)
	}

	segs := h.must(t, 200, "GET", "/v1/admin/voices/"+id+"/segments?status=pending", nil, h.admin)["segments"].([]any)
	if len(segs) != 12 {
		t.Fatalf("pending segments %d (short one should be auto-rejected)", len(segs))
	}
	if segs[0].(map[string]any)["audioUrl"] == "" {
		t.Fatal("reviewer needs a signed clip URL")
	}
	for i, s := range segs {
		sid := s.(map[string]any)["id"].(string)
		if i == 0 {
			h.must(t, 400, "POST", "/v1/admin/segments/"+sid+"/review", map[string]any{"approve": true}, h.admin)
			h.must(t, 200, "POST", "/v1/admin/segments/"+sid+"/review", map[string]any{"approve": false, "reason": "cough"}, h.admin)
			continue
		}
		h.must(t, 200, "POST", "/v1/admin/segments/"+sid+"/review",
			map[string]any{"approve": true, "verifiedTranscript": "Be still and know.", "style": "reflection"}, h.admin)
	}

	ds := h.must(t, 201, "POST", "/v1/admin/voices/"+id+"/datasets", nil, h.admin)["dataset"].(map[string]any)
	if ds["datasetVersion"] != "dataset_v001" || ds["totalSegments"].(float64) != 11 {
		t.Fatalf("dataset %v", ds)
	}
	dsID := ds["id"].(string)

	// Zero-shot already scored 0.85 in onboarding, above the 0.8 "sufficient" bar.
	body := map[string]any{"datasetId": dsID, "engine": "gpt_sovits", "baseModel": "v2", "justification": "loses the accent on long passages"}
	h.must(t, 409, "POST", "/v1/admin/voices/"+id+"/training-runs", body, h.admin)
	body["override"] = true
	run := h.must(t, 202, "POST", "/v1/admin/voices/"+id+"/training-runs", body, h.admin)["run"].(map[string]any)
	runID := run["id"].(string)
	h.drain(t)

	tr := fw.trained.Load().(voiceengine.TrainRequest)
	if len(tr.Entries) != 11 || tr.ManifestSHA256 != ds["manifestSha256"] {
		t.Fatalf("worker got %d entries, hash %s", len(tr.Entries), tr.ManifestSHA256)
	}
	got = h.must(t, 200, "GET", "/v1/admin/training-runs/"+runID, nil, h.admin)["run"].(map[string]any)
	if got["status"] != "COMPLETED" || got["modelId"] == "" {
		t.Fatalf("run %v", got)
	}
	models := h.must(t, 200, "GET", "/v1/admin/voices/"+id+"/models", nil, h.admin)["models"].([]any)
	var ft map[string]any
	for _, m := range models {
		if mm := m.(map[string]any); mm["ID"] == got["modelId"] || mm["id"] == got["modelId"] {
			ft = mm
		}
	}
	if ft == nil {
		t.Fatalf("fine-tuned candidate not registered: %v", models)
	}
	if b, _ := json.Marshal(ft); strings.Contains(string(b), "production") {
		t.Fatalf("fine-tuned model auto-promoted: %s", b)
	}

	// Streaming: relayed bytes, marked synthetic, not cached.
	sreq := httptest.NewRequest("POST", "/v1/voices/stream", strings.NewReader(
		`{"voiceId":"`+id+`","text":"Grace and peace.","style":"reflection","purpose":"reflection"}`))
	sreq.Header.Set("Authorization", "Bearer "+h.freeTok)
	srec := httptest.NewRecorder()
	h.router.ServeHTTP(srec, sreq)
	sb, _ := io.ReadAll(srec.Body)
	if srec.Code != 200 || string(sb) != "RIFF-stream-chunk-1chunk-2" || srec.Header().Get("X-Synthetic") != "true" {
		t.Fatalf("stream %d %q %v", srec.Code, sb, srec.Header())
	}

	// Revocation: no more streaming, no more intake.
	h.must(t, 200, "POST", "/v1/admin/voices/"+id+"/revoke", map[string]any{"reason": "licence ended"}, h.admin)
	h.must(t, 403, "POST", "/v1/voices/stream", map[string]any{"voiceId": id, "text": "x y z.", "style": "reflection", "purpose": "reflection"}, h.freeTok)
	fields["title"] = "after revocation"
	if rec := h.upload(t, id, fields); rec.Code != http.StatusForbidden {
		t.Fatalf("upload after revocation: %d", rec.Code)
	}
}

func TestTrainingRequiresZeroShotBaselineAndStopsOnRevocation(t *testing.T) {
	h, fw := newIntakeHarness(t)
	// A voice with training rights but no evaluated zero-shot model.
	v := h.must(t, 201, "POST", "/v1/admin/minister-voices", map[string]any{"name": "B", "rightsHolder": "B"}, h.admin)
	_ = v
	id := h.onboard(t, "can_train", "can_fine_tune")
	fields := map[string]string{"sourceType": "upload", "title": "t", "rightsDocumentId": h.docID,
		"processingPermission": "true", "trainingAllowed": "true", "sourceAttestation": "true"}
	if rec := h.upload(t, id, fields); rec.Code != 201 {
		t.Fatal(rec.Body.String())
	}
	h.drain(t)
	for _, s := range h.must(t, 200, "GET", "/v1/admin/voices/"+id+"/segments?status=pending", nil, h.admin)["segments"].([]any) {
		h.must(t, 200, "POST", "/v1/admin/segments/"+s.(map[string]any)["id"].(string)+"/review",
			map[string]any{"approve": true, "verifiedTranscript": "Amen."}, h.admin)
	}
	ds := h.must(t, 201, "POST", "/v1/admin/voices/"+id+"/datasets", nil, h.admin)["dataset"].(map[string]any)
	run := h.must(t, 202, "POST", "/v1/admin/voices/"+id+"/training-runs", map[string]any{"datasetId": ds["id"],
		"engine": "gpt_sovits", "baseModel": "v2", "justification": "accent", "override": true}, h.admin)["run"].(map[string]any)

	// Submit, then revoke before the first poll.
	if err := h.queue.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.must(t, 200, "POST", "/v1/admin/voices/"+id+"/suspend", map[string]any{"reason": "dispute"}, h.admin)
	_ = h.queue.ProcessNext(context.Background())
	got := h.must(t, 200, "GET", "/v1/admin/training-runs/"+run["id"].(string), nil, h.admin)["run"].(map[string]any)
	if got["status"] != "FAILED" || fw.cancels.Load() != 1 || fw.polls.Load() != 0 {
		t.Fatalf("run after suspension: %v cancels=%d polls=%d", got, fw.cancels.Load(), fw.polls.Load())
	}
}
