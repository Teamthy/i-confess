package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Teamthy/i-confess/internal/voicedata"
	"github.com/Teamthy/i-confess/internal/voiceengine"
)

func fl(v float64) *float64 { return &v }

func intakeFixture(t *testing.T) (*VoicePlatformStore, string, *Recording) {
	t.Helper()
	s, id := newVoiceFixture(t)
	approve(t, s, id, allCaps())
	ctx := context.Background()
	var docID string
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM voice_rights_documents WHERE voice_id = ?`, id).Scan(&docID); err != nil {
		t.Fatal(err)
	}
	rec := &Recording{VoiceID: id, SourceType: "studio_session", Title: "Session 1", RightsDocumentID: docID,
		ProcessingPermission: true, TrainingAllowed: true, StorageKey: "private/rec/1.wav", SHA256: "aa11", Language: "en", UploadedBy: "eng"}
	if err := s.CreateRecording(ctx, rec); err != nil {
		t.Fatal(err)
	}
	return s, id, rec
}

func TestRecordingRequiresPaperworkForThatVoice(t *testing.T) {
	s, id, rec := intakeFixture(t)
	ctx := context.Background()
	bad := &Recording{VoiceID: id, SourceType: "upload", Title: "x", RightsDocumentID: "doc_other", StorageKey: "k", SHA256: "bb"}
	if err := s.CreateRecording(ctx, bad); err == nil {
		t.Fatal("recording accepted without a rights document on file")
	}
	dup := *rec
	if err := s.CreateRecording(ctx, &dup); err == nil {
		t.Fatal("duplicate SHA-256 accepted")
	}
}

func TestIntakeReviewFreezeAndTrainingLifecycle(t *testing.T) {
	s, id, rec := intakeFixture(t)
	ctx := context.Background()

	var segs []voicedata.Segment
	for i := 0; i < 24; i++ {
		segs = append(segs, voicedata.Segment{StartMS: i * 6000, EndMS: i*6000 + 5000, AudioKey: fmt.Sprintf("intake/%d.wav", i),
			RawTranscript: "asr text", SNRDB: fl(32), Quality: 0.9})
	}
	segs = append(segs, voicedata.Segment{StartMS: 0, EndMS: 400, AudioKey: "intake/short.wav", Quality: 0.9})
	kept, rejected, err := s.CompleteIngestion(ctx, rec, segs, voicedata.DefaultThresholds(), 150000, []byte(`{"vad":"energy"}`))
	if err != nil || kept != 24 || rejected != 1 {
		t.Fatalf("kept=%d rejected=%d err=%v", kept, rejected, err)
	}
	got, _ := s.RecordingByID(ctx, rec.ID)
	if got.Status != "processed" || string(got.Report) != `{"vad":"energy"}` {
		t.Fatalf("recording %+v", got)
	}

	// Nothing is approved yet, so nothing can be frozen.
	if _, _, err := s.FreezeDataset(ctx, id, 1, "eng", func(v string) string { return v }); err == nil {
		t.Fatal("froze a dataset from unverified ASR output")
	}
	pending, _ := s.PoolSegments(ctx, id, "pending", 0)
	if len(pending) != 24 {
		t.Fatalf("pending %d", len(pending))
	}
	if err := s.ReviewSegment(ctx, pending[0].ID, true, "", "", "", "rev"); err == nil {
		t.Fatal("approved without a verified transcript")
	}
	for i, g := range pending {
		if i == 0 {
			if err := s.ReviewSegment(ctx, g.ID, false, "", "", "background music", "rev"); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := s.ReviewSegment(ctx, g.ID, true, "Grace and peace to you.", "reflection", "", "rev"); err != nil {
			t.Fatal(err)
		}
	}

	// Re-ingesting must not undo human decisions.
	if _, _, err := s.CompleteIngestion(ctx, rec, segs[:3], voicedata.DefaultThresholds(), 150000, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	approved, _ := s.PoolSegments(ctx, id, "approved", 0)
	if len(approved) != 23 {
		t.Fatalf("human approvals lost on re-ingest: %d", len(approved))
	}

	ds, manifest, err := s.FreezeDataset(ctx, id, 4, "eng", func(v string) string { return "datasets/" + id + "/" + v + ".json" })
	if err != nil {
		t.Fatal(err)
	}
	if ds.DatasetVersion != "dataset_v001" || ds.TotalSegments != 23 || ds.TestSegments == 0 || len(manifest) == 0 {
		t.Fatalf("dataset %+v", ds)
	}
	// Rebuilding the manifest from rows must reproduce the frozen hash.
	entries, _ := s.DatasetManifest(ctx, ds.ID)
	_, sum, _ := voicedata.Manifest{VoiceID: id, DatasetVersion: ds.DatasetVersion, GrantVersion: 4, Entries: entries}.Encode()
	if sum != ds.ManifestSHA256 {
		t.Fatal("frozen rows do not reproduce the manifest hash")
	}
	// Frozen rows are not reachable through pool review.
	if _, err := s.PoolSegmentByID(ctx, "nope"); !errors.Is(err, ErrIntakeNotFound) {
		t.Fatal(err)
	}

	if b, _ := s.BestZeroShotScore(ctx, id); b != nil {
		t.Fatal("unevaluated model counted as a zero-shot baseline")
	}
	m := &voiceengine.Model{VoiceID: id, Engine: voiceengine.EngineCosyVoice, EngineVersion: "3", ModelVersion: "zs-1", Mode: "zero_shot"}
	if err := s.CreateModel(ctx, m, "zs", "en", "eng"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveEvaluation(ctx, id, m.ID, "golden", nil, nil, nil, 0.72, 1, "FAIL", "eng"); err != nil {
		t.Fatal(err)
	}
	if b, _ := s.BestZeroShotScore(ctx, id); b == nil || *b != 0.72 {
		t.Fatalf("zero-shot baseline = %v, want 0.72", b)
	}

	run := &TrainingRun{VoiceID: id, DatasetID: ds.ID, DatasetVersion: ds.DatasetVersion, Engine: "gpt_sovits", BaseModel: "v2",
		Mode: "fine_tune", Justification: "accent drift", GrantVersion: 4, RequestedBy: "ml"}
	if err := s.CreateTrainingRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateTrainingRun(ctx, run.ID, RunUpdate{Status: voicedata.RunApproved}, "ml"); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("queued run jumped to approved: %v", err)
	}
	for _, st := range []voicedata.RunStatus{voicedata.RunRunning, voicedata.RunRunning, voicedata.RunCompleted} {
		if err := s.UpdateTrainingRun(ctx, run.ID, RunUpdate{Status: st, Progress: fl(0.5), CheckpointKey: "ckpt/x"}, "worker"); err != nil {
			t.Fatal(err)
		}
	}
	r, _ := s.TrainingRunByID(ctx, run.ID)
	if r.Status != "COMPLETED" || r.CheckpointKey != "ckpt/x" || *r.Progress != 0.5 {
		t.Fatalf("run %+v", r)
	}
}
