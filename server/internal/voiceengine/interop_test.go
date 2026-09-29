package voiceengine

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/voicedata"
)

// TestPythonWorkerInterop drives a real voice-engine worker (dev-tone backend)
// through the Go clients. It is skipped unless VOICE_WORKER_INTEROP_URL is set;
// see voice-engine/README.md for the command that runs it.
//
// Expects in the worker's storage root:
//
//	voice-private/interop/rec.wav   (any speech-like WAV)
func TestPythonWorkerInterop(t *testing.T) {
	url := os.Getenv("VOICE_WORKER_INTEROP_URL")
	if url == "" {
		t.Skip("VOICE_WORKER_INTEROP_URL not set")
	}
	tok := os.Getenv("VOICE_WORKER_INTEROP_TOKEN")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	p := NewCosyVoiceProvider(url, tok) // the dev-tone worker speaks the same protocol
	if err := p.Health(ctx); err != nil {
		t.Fatalf("health: %v", err)
	}
	req := GenerateRequest{GenerationID: "g1", VoiceID: "v", ModelID: "m", Chunks: []Chunk{{Text: "Grace and peace.", SilenceAfterMS: 200}, {Text: "Amen."}},
		Language: "en", Style: "reflection", Prosody: ProsodyFor("reflection", 1)}
	res, err := p.Generate(ctx, req)
	if err != nil || len(res.Audio) < 1000 || res.SampleRate == 0 || res.DurationMS == 0 {
		t.Fatalf("generate: %v %+v", err, res)
	}
	st, err := p.GenerateStream(ctx, req)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	b, _ := io.ReadAll(st.Body)
	st.Body.Close()
	if len(b) < 1000 || string(b[:4]) != "RIFF" || st.SampleRate == 0 {
		t.Fatalf("stream bytes %d rate %d", len(b), st.SampleRate)
	}

	wc := NewWorkerClient(url, tok, time.Minute)
	ing, err := wc.Ingest(ctx, IngestRequest{RecordingID: "r", VoiceID: "v", AudioKey: "voice-private/interop/rec.wav",
		Language: "en", SegmentPrefix: "voice-private/interop/segments"})
	if err != nil || len(ing.Segments) == 0 || len(ing.Report) == 0 {
		t.Fatalf("ingest: %v %+v", err, ing)
	}
	seg := ing.Segments[0]
	if seg.SNRDB == nil || seg.SpeakerConfidence == nil || seg.Quality <= 0 {
		t.Fatalf("segment fields did not survive the wire: %+v", seg)
	}
	_ = voicedata.Screen(seg, voicedata.DefaultThresholds())

	if os.Getenv("VOICE_WORKER_INTEROP_TRAIN") == "1" {
		var entries []voicedata.ManifestEntry
		for i, s := range ing.Segments {
			sp := voicedata.SplitTrain
			if i == 0 {
				sp = voicedata.SplitTest
			}
			entries = append(entries, voicedata.ManifestEntry{SegmentID: s.AudioKey, AudioKey: s.AudioKey, Transcript: "Amen.", Split: sp})
		}
		if err := wc.StartTraining(ctx, TrainRequest{RunID: "interop_run", Engine: "gpt_sovits", BaseModel: "v2",
			Entries: entries, CheckpointKey: "voice-private/interop/ckpt"}); err != nil {
			t.Fatalf("train: %v", err)
		}
		deadline := time.Now().Add(30 * time.Second)
		for {
			s, err := wc.TrainingStatus(ctx, "interop_run")
			if err != nil {
				t.Fatal(err)
			}
			if s.Status == "COMPLETED" {
				if s.CheckpointSHA256 == "" || s.CheckpointKey == "" {
					t.Fatalf("completed without checkpoint: %+v", s)
				}
				break
			}
			if s.Status == "FAILED" || time.Now().After(deadline) {
				t.Fatalf("run: %+v", s)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}
