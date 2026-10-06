// Tests for the benchmark oracle (audit VE-020): the golden set's expect
// blocks and the code that judges a render against them.
//
// The point of these tests is that a gate which cannot fail is worse than no
// gate, because it is read as evidence. So besides unit tests of each check,
// there is an end-to-end run against a fake worker that returns real PCM, in
// which the same harness must both pass a plausible render and reject a
// truncated one.
package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/voiceengine"
)

const goldenSetPath = "../../../docs/voice/golden_set.json"

// ---------------------------------------------------------------- golden set --

func TestShippedGoldenSetIsJudgeable(t *testing.T) {
	gs, err := LoadGoldenSet(goldenSetPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(gs.Prompts) == 0 {
		t.Fatal("no prompts")
	}
	for _, p := range gs.Prompts {
		if p.Expect == nil {
			t.Errorf("%s: no expect block, so a render of it is never judged", p.ID)
			continue
		}
		if p.Expect.AudioSecondsMin <= 0 || p.Expect.AudioSecondsMax <= p.Expect.AudioSecondsMin {
			t.Errorf("%s: audio bounds %g-%g are not usable", p.ID, p.Expect.AudioSecondsMin, p.Expect.AudioSecondsMax)
		}
		if p.Expect.MaxRealTimeFactor <= 0 {
			t.Errorf("%s: no latency bound", p.ID)
		}
		if !p.Expect.NoClipping {
			t.Errorf("%s: clipping is not asserted; the mastering ceiling is a promise, not a preference", p.ID)
		}
	}

	// Every normalised_contains phrase must be a thing this pipeline can say.
	// Without this the phrase could be aspirational ("john three sixteen") and
	// the benchmark would fail every engine forever - or be edited to match a
	// regression. This checks the *file*; TestBenchSendsNormalisedText checks the
	// *harness*, which is the half that could otherwise drift silently.
	for _, p := range gs.Prompts {
		if p.Expect == nil || len(p.Expect.NormalisedContains) == 0 {
			continue
		}
		segs, err := voiceengine.ParseMarkup(p.Text)
		if err != nil {
			t.Fatalf("%s: parse: %v", p.ID, err)
		}
		segs = voiceengine.NormaliseSegments(segs, p.Language, p.Locale)
		spoken := spokenText(voiceengine.Render(segs, voiceengine.ProviderCapabilities{}, voiceengine.ProsodyProfile{}))
		for _, want := range p.Expect.NormalisedContains {
			if !strings.Contains(spoken, strings.ToLower(want)) {
				t.Errorf("%s: the pipeline does not say %q; text sent to the engine: %s", p.ID, want, spoken)
			}
		}
	}
}

func writeSet(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "golden.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBrokenExpectationsAreRejectedAtLoad(t *testing.T) {
	cases := []struct {
		name   string
		expect string
		want   string
	}{
		{"inverted", `{"audio_seconds_min": 20, "audio_seconds_max": 5}`, "audio_seconds_min"},
		{"negative", `{"audio_seconds_min": -1}`, "cannot be negative"},
		{"floor longer than slow speech", `{"audio_seconds_min": 900, "audio_seconds_max": 901}`, "40-600 wpm"},
		// Ten words cannot be spoken within 0.5 s by anything a TTS engine is;
		// a ceiling that tight can only ever fail, so it is a typo.
		{"ceiling tighter than fast speech", `{"audio_seconds_min": 0.4, "audio_seconds_max": 0.5}`, "40-600 wpm"},
		{"silence ratio", `{"max_silence_ratio": 1.5}`, "within [0,1]"},
		{"negative rtf", `{"max_real_time_factor": -2}`, "cannot be negative"},
		// The one a reviewer cannot see in a diff: the check silently does not
		// exist, and the report keeps saying the prompt passed.
		{"typo", `{"audio_seconds_mnin": 8}`, "unknown expectation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeSet(t, fmt.Sprintf(`{"version": 1, "prompts": [{"id": "p1", "language": "en", "locale": "en-NG",
			  "style": "devotional", "text": "One two three four five six seven eight nine ten.", "expect": %s}]}`, tc.expect))
			_, err := LoadGoldenSet(path)
			if err == nil {
				t.Fatalf("expected a load-time error for %s, got none", tc.expect)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
			if !strings.Contains(err.Error(), "p1") {
				t.Errorf("error %q does not name the prompt", err)
			}
		})
	}

	// A prompt with no expect block at all stays legal: an unjudged render is
	// disclosed by the report's "checks failed" column, and rejecting bare
	// prompts would make the file unusable while new prompts are being drafted.
	path := writeSet(t, `{"version": 1, "prompts": [{"id": "p1", "language": "en", "text": "hello there friend"}]}`)
	if _, err := LoadGoldenSet(path); err != nil {
		t.Errorf("prompt without expectations should load: %v", err)
	}
}

func TestBenchSendsNormalisedText(t *testing.T) {
	gs, err := LoadGoldenSet(goldenSetPath)
	if err != nil {
		t.Fatal(err)
	}
	var numbers Prompt
	for _, p := range gs.Prompts {
		if p.ID == "numbers-refs-01" {
			numbers = p
		}
	}
	if numbers.ID == "" {
		t.Fatal("golden set lost numbers-refs-01")
	}
	gs.Prompts = []Prompt{numbers}

	var sent string
	srv := fakeWorker(t, false, 0, &sent)
	defer srv.Close()
	targets := []Target{{Name: "cosyvoice", Provider: voiceengine.NewCosyVoiceProvider(srv.URL, "test-token")}}
	if _, err := Bench(context.Background(), gs, targets, Config{Runs: 1, VoiceID: "v", OutDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	// What the worker received is the only text that matters: the author wrote
	// "John 3:16", and an engine asked to speak "john three sixteen" is the
	// audible defect VE-004 is about. This is the assertion the harness itself
	// has to satisfy - a benchmark that normalises in the checker but not in the
	// request would otherwise certify its own fix.
	for _, want := range numbers.Expect.NormalisedContains {
		if !strings.Contains(sent, strings.ToLower(want)) {
			t.Errorf("the engine was not sent %q; it received: %s", want, sent)
		}
	}
	for _, mustNot := range []string{"3:16", "23:1", "8:28", "1250", "7:30 pm"} {
		if strings.Contains(sent, mustNot) {
			t.Errorf("the engine was sent raw notation %q; normalisation did not run: %s", mustNot, sent)
		}
	}
}

// ------------------------------------------------------------------- signals --

// pcmWAV builds a mono 16-bit WAV the way the worker masters one, so the
// analyser is exercised on bytes with known properties.
func pcmWAV(t *testing.T, seconds float64, fn func(i int) int16) []byte {
	t.Helper()
	const rate = 22050
	frames := int(seconds * rate)
	data := make([]byte, frames*2)
	for i := 0; i < frames; i++ {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(fn(i)))
	}
	out := make([]byte, 44+len(data))
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(36+len(data)))
	copy(out[8:], "WAVE")
	copy(out[12:], "fmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1) // PCM
	binary.LittleEndian.PutUint16(out[22:], 1) // mono
	binary.LittleEndian.PutUint32(out[24:], rate)
	binary.LittleEndian.PutUint32(out[28:], rate*2)
	binary.LittleEndian.PutUint16(out[32:], 2)
	binary.LittleEndian.PutUint16(out[34:], 16)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(len(data)))
	copy(out[44:], data)
	return out
}

func TestAnalyseWAVMeasuresWhatItClaims(t *testing.T) {
	t.Run("tone", func(t *testing.T) {
		// Amplitude 0.5 -> peak -6.02 dBFS, no clipping, and a sine is never silent.
		wav := pcmWAV(t, 2.0, func(i int) int16 {
			return int16(0.5 * 32767 * math.Sin(2*math.Pi*180*float64(i)/22050))
		})
		sig := analyseWAV(wav)
		if math.Abs(sig.Seconds-2.0) > 0.01 {
			t.Errorf("seconds = %v, want 2.0", sig.Seconds)
		}
		if math.Abs(sig.PeakDBFS-(-6.02)) > 0.2 {
			t.Errorf("peak = %v dBFS, want about -6.02", sig.PeakDBFS)
		}
		if sig.Clipped != 0 {
			t.Errorf("clipped = %d, want 0", sig.Clipped)
		}
		if sig.SilenceRate != 0 {
			t.Errorf("silence ratio = %v, want 0", sig.SilenceRate)
		}
	})

	t.Run("silence", func(t *testing.T) {
		sig := analyseWAV(pcmWAV(t, 2.0, func(int) int16 { return 0 }))
		if sig.SilenceRate != 1 {
			t.Errorf("silence ratio = %v, want 1", sig.SilenceRate)
		}
		if sig.PeakDBFS > -40 {
			t.Errorf("peak of digital silence = %v dBFS, want below the -40 dBFS floor", sig.PeakDBFS)
		}
	})

	t.Run("clipped", func(t *testing.T) {
		// A square wave pinned to full scale: exactly what the limiter prevents,
		// so the analyser has to see it.
		sig := analyseWAV(pcmWAV(t, 1.0, func(i int) int16 {
			if i%2 == 0 {
				return 32767
			}
			return -32768
		}))
		if sig.Clipped == 0 {
			t.Error("full-scale square wave reported no clipped samples")
		}
	})

	t.Run("not a wav", func(t *testing.T) {
		sig := analyseWAV([]byte("this is not audio"))
		if sig.Seconds != 0 {
			t.Errorf("seconds = %v for garbage input, want 0 (a length check must then fail)", sig.Seconds)
		}
	})

	t.Run("truncated file", func(t *testing.T) {
		wav := pcmWAV(t, 4.0, func(i int) int16 { return int16(0.5 * 32767 * math.Sin(2*math.Pi*180*float64(i)/22050)) })
		sig := analyseWAV(wav[:len(wav)/2])
		if math.Abs(sig.Seconds-2.0) > 0.02 {
			t.Errorf("seconds = %v, want the 2.0 s actually present", sig.Seconds)
		}
	})
}

func TestChecksFailLoudlyAndNameTheNumbers(t *testing.T) {
	e := &Expect{AudioSecondsMin: 6.8, AudioSecondsMax: 13.6, MaxRealTimeFactor: 10, MinPeakDBFS: -40,
		MaxSilenceRatio: 0.85, NoClipping: true, NormalisedContains: []string{"john chapter three verse sixteen"}}
	// A render that is short, silent, clipped, slow and missing the words.
	r := Run{RTF: 40}
	sig := audioSignal{Seconds: 1.2, PeakDBFS: -71, Clipped: 7, SilenceRate: 0.98}
	bad := e.checks(r, sig, "read john 3:16 now")
	for _, want := range []string{"1.2s is shorter than the expected 6.8s", "40.00 exceeds the bound 10.00",
		"-71.0 dBFS", "0.98 exceeds the bound 0.85", "7 sample(s) at full scale", "john chapter three verse sixteen"} {
		found := false
		for _, b := range bad {
			if strings.Contains(b, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("failure %q not reported; got %v", want, bad)
		}
	}
	if len(bad) != 6 {
		t.Errorf("got %d failures, want one per broken bound: %v", len(bad), bad)
	}

	// The same expectations on a good render must be silent.
	ok := e.checks(Run{RTF: 0.4}, audioSignal{Seconds: 9.5, PeakDBFS: -9.2, Clipped: 0, SilenceRate: 0.3},
		"read john chapter three verse sixteen before we pray")
	if len(ok) != 0 {
		t.Errorf("good render reported failures: %v", ok)
	}
}

// ------------------------------------------------------------------ end to end --

// fakeWorker is a GPU worker that renders a tone whose length is proportional
// to the words it was sent, which is all the oracle can observe.
// fakeWorker renders a tone whose length is proportional to the words it was
// sent. truncate/floorSeconds bend the audio into the shapes the oracle exists
// to catch; sentText, when non-nil, receives the text the worker actually got.
func fakeWorker(t *testing.T, truncate bool, floorSeconds float64, sentText *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/synthesize":
			var req struct {
				Chunks []struct {
					Text string `json:"text"`
				} `json:"chunks"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			words := 0
			var got []string
			for _, c := range req.Chunks {
				words += len(strings.Fields(c.Text))
				got = append(got, c.Text)
			}
			if sentText != nil {
				*sentText = strings.Join(strings.Fields(strings.ToLower(strings.Join(got, " "))), " ")
			}
			seconds := float64(words) / 150 * 60 // 150 wpm
			if truncate {
				// the failure mode VE-002 and VE-020 are about: a render that
				// stops early and reports its own shortened length honestly
				seconds = 0.5
			}
			if floorSeconds > seconds {
				// ... or an engine that clips the tail off every prompt.
				seconds = floorSeconds
			}
			buf := pcmWAV(t, seconds, func(i int) int16 {
				return int16(0.5 * 32767 * math.Sin(2*math.Pi*180*float64(i)/22050))
			})
			w.Header().Set("Content-Type", "audio/wav")
			w.Header().Set("X-Sample-Rate", "22050")
			w.Header().Set("X-Duration-Ms", fmt.Sprintf("%d", int(seconds*1000)))
			w.Header().Set("X-Engine-Version", "fake-1")
			w.Header().Set("X-Inference-Seconds", "1.5")
			_, _ = w.Write(buf)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestBenchGatePassesAGoodRenderAndRejectsATruncatedOne(t *testing.T) {
	gs, err := LoadGoldenSet(goldenSetPath)
	if err != nil {
		t.Fatal(err)
	}
	// One short prompt keeps the run fast; it carries real expectations.
	gs.Prompts = []Prompt{gs.Prompts[0]}

	for _, tc := range []struct {
		name      string
		truncate  bool
		floor     float64
		wantFail  int
		wantInLog string
	}{
		{"plausible render", false, 0, 0, ""},
		// The engine answers, the HTTP call succeeds, the file decodes, the
		// duration header looks self-consistent - and the audio is a truncation.
		// Before this oracle the harness reported that as a success.
		{"truncated render", true, 0, 1, "shorter than the expected"},
		// The engine keeps talking past the end of the sentence and its duration
		// header honestly matches the bytes. Nothing in the latency statistics
		// looks wrong; only the length bound can see it.
		{"runaway render", false, 25.0, 1, "longer than the expected 17.0s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fakeWorker(t, tc.truncate, tc.floor, nil)
			defer srv.Close()
			out := t.TempDir()
			cfg := Config{Runs: 1, Strict: true, VoiceID: "voice_bench", OutDir: out}
			targets := []Target{{Name: "cosyvoice", Provider: voiceengine.NewCosyVoiceProvider(srv.URL, "test-token")}}
			rep, err := Bench(context.Background(), gs, targets, cfg)
			if err != nil {
				t.Fatalf("bench: %v", err)
			}
			if got := rep.Engines[0].ChecksFailed; got != tc.wantFail {
				t.Errorf("ChecksFailed = %d, want %d (detail: %+v)", got, tc.wantFail, rep.Detail)
			}
			if rep.Strict != true {
				t.Error("report does not record that the run was a gate")
			}
			// Markdown() is what the CI annotation and the artefact are built
			// from, so assert on it directly rather than on a file main.go writes.
			md := rep.Markdown()
			if tc.wantInLog == "" {
				if strings.Contains(md, "Failed expectations:") {
					t.Errorf("clean run reports a failure:\n%s", md)
				}
			}
			if tc.wantInLog != "" {
				if !strings.Contains(md, tc.wantInLog) {
					t.Errorf("report does not name the failure %q; got:\n%s", tc.wantInLog, md)
				}
				if !strings.Contains(md, "-strict") {
					t.Error("report does not say the run was gating")
				}
				if !strings.Contains(md, "Failed expectations:") {
					t.Error("report has no failure section")
				}
				if rep.Engines[0].FailedByPrompt[gs.Prompts[0].ID] != 1 {
					t.Errorf("failures are not attributed to the prompt: %v", rep.Engines[0].FailedByPrompt)
				}
			}
			// The judgement has to be about the audio, not about the harness.
			if rep.Detail[0].OK != true {
				t.Error("a render with audio in it is OK regardless of what the oracle concluded")
			}
			jb, _ := json.Marshal(rep)
			var round Report
			if err := json.Unmarshal(jb, &round); err != nil {
				t.Fatalf("report.json shape: %v", err)
			}
			if round.Engines[0].ChecksFailed != tc.wantFail {
				t.Errorf("serialised ChecksFailed = %d, want %d", round.Engines[0].ChecksFailed, tc.wantFail)
			}
		})
	}
}
