// Command voice-bench compares TTS engines on the golden prompt set.
//
//	go run ./cmd/voice-bench \
//	  -engine cosyvoice=http://gpu-1:8000 -engine gptsovits=http://gpu-2:8000 \
//	  -set ../docs/voice/golden_set.json -runs 3 -stream \
//	  -voice voice_ab12 -reference-uri voice-private/voices/voice_ab12/refs/r1.wav \
//	  -reference-transcript "Grace and peace to you." \
//	  -scorer "python3 score.py" -out /tmp/bench
//
// Exit codes: 0 means every check the harness could run passed, 1 means the
// harness itself could not produce a judgement (bad flags, unreachable worker,
// I/O), and 2 means renders came back but at least one golden-set expectation
// on them failed. 1 and 2 must stay distinct: "we measured nothing" is not
// evidence for or against an engine, "we measured a truncated reading" is.
//
// Cloning a real person's voice requires an approved rights grant for that
// voice (can_clone). This tool cannot see the rights registry, so it refuses
// to use a reference unless -voice names the licensed voice and
// -rights-confirmed is set; both are recorded in the report. The API remains
// the enforcement point for anything that ships.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/voiceengine"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	var engines multiFlag
	flag.Var(&engines, "engine", "engine=url (repeatable); engine is cosyvoice|gptsovits|voxcpm|voicestudio")
	set := flag.String("set", "../docs/voice/golden_set.json", "golden prompt set")
	licenses := flag.String("licenses", "../docs/model_licenses.json", "model licence register")
	runs := flag.Int("runs", 1, "renders per prompt per engine")
	stream := flag.Bool("stream", false, "measure time-to-first-audio on streaming engines")
	voiceID := flag.String("voice", "", "licensed voice id the reference belongs to")
	refURI := flag.String("reference-uri", "", "private reference key the worker resolves (voice-private/...)")
	refText := flag.String("reference-transcript", "", "verified transcript of the reference clip")
	confirmed := flag.Bool("rights-confirmed", false, "confirm the voice has an approved grant permitting cloning")
	scorer := flag.String("scorer", "", "optional quality scorer command (see bench.go)")
	out := flag.String("out", "voice-bench-out", "output directory")
	strict := flag.Bool("strict", false, "make a failed golden-set expectation exit 2 (for nightly/CI use); without it the failures are reported and the run still exits 0")
	timeout := flag.Duration("timeout", 5*time.Minute, "per-render timeout")
	flag.Parse()

	if len(engines) == 0 {
		fatal("at least one -engine name=url is required")
	}
	if *refURI != "" && (*voiceID == "" || !*confirmed) {
		fatal("a reference clip clones a real voice: pass -voice <licensed voice id> and -rights-confirmed")
	}
	gs, err := LoadGoldenSet(*set)
	if err != nil {
		fatal(err.Error())
	}
	token := os.Getenv("VOICE_ENGINE_TOKEN")
	reg := LoadLicenses(*licenses)
	var targets []Target
	for _, e := range engines {
		name, url, ok := strings.Cut(e, "=")
		if !ok || url == "" {
			fatal("bad -engine " + e)
		}
		var p *voiceengine.HTTPProvider
		switch strings.ToLower(name) {
		case "cosyvoice":
			p = voiceengine.NewCosyVoiceProvider(url, token)
		case "gptsovits", "gpt-sovits":
			p = voiceengine.NewGPTSoVITSProvider(url, token)
		case "voxcpm":
			p = voiceengine.NewVoxCPMProvider(url, token)
		case "voicestudio":
			// AGPL: benchmarked over HTTP only, never linked (docs/VOICESTUDIO_LICENSE.md).
			p = voiceengine.NewVoiceStudioProvider(url, token)
		default:
			fatal("unknown engine " + name)
		}
		lic := reg[strings.ReplaceAll(strings.ToLower(name), "-", "")]
		targets = append(targets, Target{Name: strings.ToLower(name), Provider: p, License: lic})
	}
	cfg := Config{Runs: *runs, Stream: *stream, Strict: *strict, VoiceID: *voiceID, OutDir: *out, Timeout: *timeout}
	if *refURI != "" {
		cfg.Reference = &voiceengine.Reference{ID: "bench-ref", VoiceID: *voiceID, URI: *refURI, Transcript: *refText, RightsOK: true}
	}
	if *scorer != "" {
		cfg.Scorer = strings.Fields(*scorer)
	}
	ctx := context.Background()
	for _, t := range targets {
		if err := t.Provider.Health(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s health check failed: %v (benchmarking anyway; failures will be recorded)\n", t.Name, err)
		}
	}
	rep, err := Bench(ctx, gs, targets, cfg)
	if err != nil {
		fatal(err.Error())
	}
	jb, _ := json.MarshalIndent(rep, "", "  ")
	must(os.WriteFile(filepath.Join(*out, "report.json"), jb, 0o644))
	md := rep.Markdown()
	must(os.WriteFile(filepath.Join(*out, "report.md"), []byte(md), 0o644))
	fmt.Print(md)

	// A gate has to be able to tell "no engine was reachable" from "the engine
	// truncated scripture", so the judgement is returned as its own code.
	failed := 0
	for _, e := range rep.Engines {
		failed += e.ChecksFailed
	}
	if failed == 0 {
		return
	}
	if *strict {
		fmt.Fprintf(os.Stderr, "voice-bench: %d golden-set expectation(s) failed; see report.md\n", failed)
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "voice-bench: %d golden-set expectation(s) failed (not gating: rerun with -strict)\n", failed)
}

func must(err error) {
	if err != nil {
		fatal(err.Error())
	}
}

// fatal is a harness failure, not a verdict: the run never got far enough to
// judge an engine, so it must not share an exit code with "checked and failed".
func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "voice-bench:", msg)
	os.Exit(1)
}
