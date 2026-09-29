package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/voiceengine"
)

func wav(seconds float64, rate int) []byte {
	n := int(seconds * float64(rate))
	b := make([]byte, 44+2*n)
	copy(b[0:], "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+2*n))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], uint32(rate))
	binary.LittleEndian.PutUint32(b[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(2*n))
	return b
}

func fakeEngine(t *testing.T, failing bool) string {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/capabilities":
			json.NewEncoder(w).Encode(voiceengine.ProviderCapabilities{ZeroShot: true, Languages: []string{"en"}, MaxChunkChars: 200, SelfHosted: true})
		case "/v1/synthesize":
			if failing {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"error":{"class":"gpu","message":"out of memory"}}`))
				return
			}
			w.Header().Set("Content-Type", "audio/wav")
			w.Header().Set("X-Sample-Rate", "8000")
			w.Write(wav(2, 8000))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s.URL
}

func TestBenchMeasuresEnginesAndNeverInventsQuality(t *testing.T) {
	gs, err := LoadGoldenSet("../../../docs/voice/golden_set.json")
	if err != nil {
		t.Fatal(err)
	}
	gs.Prompts = gs.Prompts[:3]
	out := t.TempDir()
	lic := map[string]*LicenseEntry{"cosyvoice": {Engine: "cosyvoice", License: "Apache-2.0"}}
	targets := []Target{
		{Name: "cosyvoice", Provider: voiceengine.NewCosyVoiceProvider(fakeEngine(t, false), "tok"), License: lic["cosyvoice"]},
		{Name: "voxcpm", Provider: voiceengine.NewVoxCPMProvider(fakeEngine(t, true), "tok")},
	}
	rep, err := Bench(context.Background(), gs, targets, Config{Runs: 2, OutDir: out})
	if err != nil {
		t.Fatal(err)
	}
	good, bad := rep.Engines[0], rep.Engines[1]
	if good.Runs != 6 || good.Failures != 0 || good.AudioSeconds != 12 || good.LatencyP50MS <= 0 || good.MeanRTF <= 0 {
		t.Fatalf("good engine summary: %+v", good)
	}
	if good.Score != nil || !strings.Contains(good.QualityNote, "not measured") {
		t.Fatalf("quality must be unmeasured without a scorer: %+v", good)
	}
	if !strings.Contains(good.LicenseNote, "NOT allowed") {
		t.Fatalf("licence note: %s", good.LicenseNote)
	}
	if bad.Failures != 6 || bad.FailureRate != 1 || bad.FailureByKind["gpu"] != 6 {
		t.Fatalf("failing engine summary: %+v", bad)
	}
	if !strings.Contains(bad.LicenseNote, "not in licence register") {
		t.Fatalf("unregistered engine must be flagged: %s", bad.LicenseNote)
	}
	// Renders saved; blind pack holds one clip per prompt with neutral labels.
	blind, _ := filepath.Glob(filepath.Join(out, "blind", "*.wav"))
	if len(blind) != 3 {
		t.Fatalf("blind pack: %v", blind)
	}
	for _, f := range blind {
		if strings.Contains(f, "cosyvoice") {
			t.Fatalf("blind file name leaks the engine: %s", f)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "blind_key.json")); err != nil {
		t.Fatal(err)
	}
	if md := rep.Markdown(); !strings.Contains(md, "| cosyvoice | 6 | 0.0 |") || !strings.Contains(md, "not measured") {
		t.Fatalf("markdown:\n%s", md)
	}
}

func TestScorerFeedsCompositeAndBadScorersAreRejected(t *testing.T) {
	gs := &GoldenSet{Version: 1, Prompts: []Prompt{{ID: "p1", Language: "en", Text: "Grace and peace."}}}
	dir := t.TempDir()
	good := filepath.Join(dir, "good.sh")
	os.WriteFile(good, []byte("#!/bin/sh\necho '{\"speaker_similarity\":0.8,\"intelligibility\":0.9}'\n"), 0o755)
	bad := filepath.Join(dir, "bad.sh")
	os.WriteFile(bad, []byte("#!/bin/sh\necho '{\"speaker_similarity\":96}'\n"), 0o755)
	target := []Target{{Name: "cosyvoice", Provider: voiceengine.NewCosyVoiceProvider(fakeEngine(t, false), "tok")}}

	rep, err := Bench(context.Background(), gs, target, Config{OutDir: filepath.Join(dir, "a"), Scorer: []string{good}})
	if err != nil {
		t.Fatal(err)
	}
	e := rep.Engines[0]
	if e.Score == nil || e.Score.Coverage >= 1 || e.Metrics["speaker_similarity"] != 0.8 {
		t.Fatalf("expected partial-coverage composite: %+v", e)
	}
	rep, _ = Bench(context.Background(), gs, target, Config{OutDir: filepath.Join(dir, "b"), Scorer: []string{bad}})
	if rep.Engines[0].Score != nil || !strings.Contains(rep.Detail[0].Error, "outside [0,1]") {
		t.Fatalf("a percentage-style scorer must be rejected, not clamped: %+v", rep.Detail[0])
	}
}

func TestGoldenSetIsValidAndCoversRequiredCategories(t *testing.T) {
	gs, err := LoadGoldenSet("../../../docs/voice/golden_set.json")
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, p := range gs.Prompts {
		have[p.Category] = true
	}
	for _, c := range []string{"short_prayer", "long_scripture", "pidgin", "nigerian_names_places", "numbers_dates_references", "long_form_stability"} {
		if !have[c] {
			t.Errorf("golden set lacks %s", c)
		}
	}
}

func TestWavDurationHandlesStreamHeaders(t *testing.T) {
	b := wav(1, 8000)
	if d := wavDurationMS(b); d != 1000 {
		t.Fatalf("duration %v", d)
	}
	s := append([]byte(nil), b...)
	binary.LittleEndian.PutUint32(s[4:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(s[40:], 0xFFFFFFFF)
	if d := wavDurationMS(s); d != 1000 {
		t.Fatalf("stream duration %v", d)
	}
	if d := wavDurationMS(fixStreamHeader(s)); d != 1000 {
		t.Fatalf("fixed duration %v", d)
	}
	if wavDurationMS([]byte("not audio")) != 0 {
		t.Fatal("garbage must be 0")
	}
}
