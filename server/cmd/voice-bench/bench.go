package main

// Engine benchmark harness (spec sections 8, 19, 39).
//
// It runs a golden prompt set against each configured engine through the
// same VoiceProvider interface production uses, with the same markup
// chunking, and records what it can measure honestly:
//
//   - latency (p50/p95), time-to-first-audio for streaming engines,
//     real-time factor, failure rate by error class;
//   - every rendered WAV, plus a blind-labelled listening pack for humans;
//   - quality dimensions ONLY when an external scorer is supplied (ASR WER,
//     speaker-encoder similarity, MOS predictor...). Without one, quality is
//     reported as unmeasured - never guessed - and no ICF_VOICE_SCORE is
//     printed.
//
// It never decides which engine ships. It produces evidence for the humans
// who do (and for the licence review, which it surfaces alongside).

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/voiceengine"
	"github.com/Teamthy/i-confess/internal/voiceeval"
)

// Prompt is one golden-set entry.
type Prompt struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Language string `json:"language"`
	Locale   string `json:"locale"`
	Style    string `json:"style"`
	Text     string `json:"text"`
}

// GoldenSet is docs/voice/golden_set.json.
type GoldenSet struct {
	Version int      `json:"version"`
	Prompts []Prompt `json:"prompts"`
}

// LoadGoldenSet reads and validates a golden set.
func LoadGoldenSet(path string) (*GoldenSet, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var gs GoldenSet
	if err := json.Unmarshal(b, &gs); err != nil {
		return nil, fmt.Errorf("golden set: %w", err)
	}
	seen := map[string]bool{}
	for _, p := range gs.Prompts {
		if p.ID == "" || strings.TrimSpace(p.Text) == "" {
			return nil, fmt.Errorf("golden set: prompt with empty id or text")
		}
		if seen[p.ID] {
			return nil, fmt.Errorf("golden set: duplicate id %q", p.ID)
		}
		seen[p.ID] = true
		if _, err := voiceengine.ParseMarkup(p.Text); err != nil {
			return nil, fmt.Errorf("golden set: %s: %w", p.ID, err)
		}
	}
	if len(gs.Prompts) == 0 {
		return nil, errors.New("golden set: no prompts")
	}
	return &gs, nil
}

// Target is one engine under test.
type Target struct {
	Name     string
	Provider voiceengine.VoiceProvider
	// License is the register entry for this engine, if known.
	License *LicenseEntry
}

// LicenseEntry mirrors the fields of docs/model_licenses.json we surface.
type LicenseEntry struct {
	Engine string `json:"engine"`
	// License is the *reported* code licence; checkpoint weights can carry
	// a different licence and are tracked separately in the register.
	License           string `json:"code_license_reported"`
	ProductionAllowed bool   `json:"production_allowed"`
}

// LoadLicenses reads the licence register keyed by engine. Unknown shapes
// are tolerated: a missing entry is reported as "not in register".
func LoadLicenses(path string) map[string]*LicenseEntry {
	out := map[string]*LicenseEntry{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var doc struct {
		Engines []LicenseEntry `json:"engines"`
	}
	if json.Unmarshal(b, &doc) == nil {
		for i := range doc.Engines {
			e := doc.Engines[i]
			out[strings.ToLower(e.Engine)] = &e
		}
	}
	return out
}

// Config controls a benchmark run.
type Config struct {
	Runs      int
	Stream    bool
	Reference *voiceengine.Reference
	VoiceID   string
	OutDir    string
	// Scorer, when set, is run as: Scorer <wav> <textfile> [reference-uri]
	// and must print a JSON object of dimension -> value in [0,1].
	Scorer  []string
	Weights voiceeval.Weights
	Timeout time.Duration
}

// Run is one render attempt.
type Run struct {
	Engine     string             `json:"engine"`
	PromptID   string             `json:"prompt_id"`
	Category   string             `json:"category"`
	Attempt    int                `json:"attempt"`
	OK         bool               `json:"ok"`
	ErrorClass string             `json:"error_class,omitempty"`
	Error      string             `json:"error,omitempty"`
	LatencyMS  float64            `json:"latency_ms"`
	FirstAudio float64            `json:"first_audio_ms,omitempty"`
	AudioMS    float64            `json:"audio_ms"`
	RTF        float64            `json:"rtf,omitempty"`
	WAV        string             `json:"wav,omitempty"`
	Metrics    voiceeval.Metrics  `json:"metrics,omitempty"`
	Headers    map[string]float64 `json:"-"`
}

// EngineSummary aggregates one engine's runs.
type EngineSummary struct {
	Engine        string            `json:"engine"`
	Runs          int               `json:"runs"`
	Failures      int               `json:"failures"`
	FailureRate   float64           `json:"failure_rate"`
	FailureByKind map[string]int    `json:"failures_by_class,omitempty"`
	LatencyP50MS  float64           `json:"latency_p50_ms"`
	LatencyP95MS  float64           `json:"latency_p95_ms"`
	FirstAudioP50 float64           `json:"first_audio_p50_ms,omitempty"`
	MeanRTF       float64           `json:"mean_rtf"`
	AudioSeconds  float64           `json:"audio_seconds"`
	Metrics       voiceeval.Metrics `json:"metrics,omitempty"`
	Score         *voiceeval.Score  `json:"score,omitempty"`
	QualityNote   string            `json:"quality_note"`
	License       *LicenseEntry     `json:"license,omitempty"`
	LicenseNote   string            `json:"license_note"`
	Capabilities  any               `json:"capabilities"`
}

// Report is the full benchmark output.
type Report struct {
	StartedAt   string          `json:"started_at"`
	FinishedAt  string          `json:"finished_at"`
	GoldenSet   int             `json:"golden_set_version"`
	VoiceID     string          `json:"voice_id,omitempty"`
	Runs        int             `json:"runs_per_prompt"`
	Engines     []EngineSummary `json:"engines"`
	Detail      []Run           `json:"runs"`
	BlindPack   string          `json:"blind_pack,omitempty"`
	Disclaimers []string        `json:"disclaimers"`
}

// Bench runs every prompt against every target.
func Bench(ctx context.Context, gs *GoldenSet, targets []Target, cfg Config) (*Report, error) {
	if cfg.Runs <= 0 {
		cfg.Runs = 1
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.Weights == nil {
		cfg.Weights = voiceeval.DefaultWeights()
	}
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return nil, err
	}
	rep := &Report{StartedAt: time.Now().UTC().Format(time.RFC3339), GoldenSet: gs.Version, VoiceID: cfg.VoiceID, Runs: cfg.Runs,
		Disclaimers: []string{
			voiceeval.Disclaimer,
			"Latency includes network and worker queueing on the benchmark host; compare engines on the same hardware only.",
			"Quality dimensions are reported only when an external scorer measured them.",
		}}
	for _, t := range targets {
		caps := t.Provider.Capabilities(ctx)
		var runs []Run
		for _, p := range gs.Prompts {
			segs, _ := voiceengine.ParseMarkup(p.Text) // validated on load
			chunks := voiceengine.Render(segs, caps, voiceengine.ProsodyProfile{})
			for a := 1; a <= cfg.Runs; a++ {
				r := benchOne(ctx, t, caps, p, chunks, a, cfg)
				runs = append(runs, r)
				rep.Detail = append(rep.Detail, r)
			}
		}
		rep.Engines = append(rep.Engines, summarize(t, caps, runs, cfg.Weights))
	}
	pack, err := writeBlindPack(cfg.OutDir, rep.Detail)
	if err != nil {
		return nil, err
	}
	rep.BlindPack = pack
	rep.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	return rep, nil
}

func benchOne(ctx context.Context, t Target, caps voiceengine.ProviderCapabilities, p Prompt, chunks []voiceengine.Chunk,
	attempt int, cfg Config) Run {
	r := Run{Engine: t.Name, PromptID: p.ID, Category: p.Category, Attempt: attempt}
	req := voiceengine.GenerateRequest{
		GenerationID: fmt.Sprintf("bench-%s-%s-%d", t.Name, p.ID, attempt),
		VoiceID:      cfg.VoiceID, ModelID: "bench", ModelVersion: "bench",
		Chunks: chunks, Language: p.Language, Locale: p.Locale, Style: p.Style, Reference: cfg.Reference,
	}
	cctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	var audio []byte
	start := time.Now()
	if sp, ok := t.Provider.(voiceengine.StreamingProvider); ok && cfg.Stream && caps.Streaming {
		st, err := sp.GenerateStream(cctx, req)
		if err != nil {
			return fail(r, err, start)
		}
		var buf bytes.Buffer
		first := make([]byte, 4096)
		n, rerr := st.Body.Read(first)
		r.FirstAudio = ms(time.Since(start))
		buf.Write(first[:n])
		if rerr == nil {
			_, rerr = buf.ReadFrom(st.Body)
		}
		st.Body.Close()
		if rerr != nil && !errors.Is(rerr, os.ErrClosed) && rerr.Error() != "EOF" {
			return fail(r, rerr, start)
		}
		audio = fixStreamHeader(buf.Bytes())
	} else {
		res, err := t.Provider.Generate(cctx, req)
		if err != nil {
			return fail(r, err, start)
		}
		audio = res.Audio
	}
	r.LatencyMS = ms(time.Since(start))
	r.AudioMS = wavDurationMS(audio)
	if r.AudioMS <= 0 {
		r.ErrorClass, r.Error = string(voiceengine.ClassModel), "engine returned no decodable audio"
		return r
	}
	r.OK = true
	r.RTF = round6(r.LatencyMS / r.AudioMS)
	dir := filepath.Join(cfg.OutDir, "renders", t.Name)
	_ = os.MkdirAll(dir, 0o755)
	r.WAV = filepath.Join(dir, fmt.Sprintf("%s-%d.wav", p.ID, attempt))
	if err := os.WriteFile(r.WAV, audio, 0o644); err != nil {
		r.OK, r.ErrorClass, r.Error = false, string(voiceengine.ClassStorage), err.Error()
		return r
	}
	if len(cfg.Scorer) > 0 {
		m, err := score(ctx, cfg, r.WAV, p.Text)
		if err != nil {
			r.Error = "scorer: " + err.Error() // render still counts; quality stays unmeasured
		} else {
			r.Metrics = m
		}
	}
	return r
}

func fail(r Run, err error, start time.Time) Run {
	r.LatencyMS = ms(time.Since(start))
	r.ErrorClass, r.Error = string(voiceengine.Classify(err)), err.Error()
	return r
}

// score runs the external scorer. Values outside [0,1] or unknown
// dimensions are rejected rather than clamped, so a broken scorer cannot
// silently inflate a composite.
func score(ctx context.Context, cfg Config, wavPath, text string) (voiceeval.Metrics, error) {
	tf := wavPath + ".txt"
	if err := os.WriteFile(tf, []byte(text), 0o644); err != nil {
		return nil, err
	}
	args := append(append([]string{}, cfg.Scorer[1:]...), wavPath, tf)
	if cfg.Reference != nil {
		args = append(args, cfg.Reference.URI)
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(cctx, cfg.Scorer[0], args...).Output()
	if err != nil {
		return nil, err
	}
	var raw map[string]float64
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("scorer output is not a JSON object of numbers: %w", err)
	}
	known := map[string]bool{}
	for _, d := range voiceeval.AllDimensions {
		known[string(d)] = true
	}
	m := voiceeval.Metrics{}
	for k, v := range raw {
		if !known[k] {
			return nil, fmt.Errorf("scorer returned unknown dimension %q", k)
		}
		if math.IsNaN(v) || v < 0 || v > 1 {
			return nil, fmt.Errorf("scorer value %s=%v outside [0,1]", k, v)
		}
		m[voiceeval.Dimension(k)] = v
	}
	return m, nil
}

func summarize(t Target, caps voiceengine.ProviderCapabilities, runs []Run, w voiceeval.Weights) EngineSummary {
	s := EngineSummary{Engine: t.Name, Runs: len(runs), FailureByKind: map[string]int{}, Capabilities: caps, License: t.License}
	var lat, first, rtf []float64
	sums, counts := map[voiceeval.Dimension]float64{}, map[voiceeval.Dimension]int{}
	for _, r := range runs {
		if !r.OK {
			s.Failures++
			s.FailureByKind[r.ErrorClass]++
			continue
		}
		lat = append(lat, r.LatencyMS)
		rtf = append(rtf, r.RTF)
		if r.FirstAudio > 0 {
			first = append(first, r.FirstAudio)
		}
		s.AudioSeconds += r.AudioMS / 1000
		for d, v := range r.Metrics {
			sums[d] += v
			counts[d]++
		}
	}
	if s.Runs > 0 {
		s.FailureRate = round3(float64(s.Failures) / float64(s.Runs))
	}
	s.LatencyP50MS, s.LatencyP95MS = pct(lat, 50), pct(lat, 95)
	s.FirstAudioP50 = pct(first, 50)
	s.MeanRTF = round6(mean(rtf))
	s.AudioSeconds = round3(s.AudioSeconds)
	if len(sums) > 0 {
		s.Metrics = voiceeval.Metrics{}
		for d, v := range sums {
			s.Metrics[d] = round3(v / float64(counts[d]))
		}
		if sc, err := voiceeval.Compute(s.Metrics, w); err == nil {
			s.Score = &sc
			s.QualityNote = fmt.Sprintf("measured by external scorer; coverage %.0f%% of configured weight", 100*sc.Coverage)
		} else {
			s.QualityNote = "scorer metrics present but composite not computable: " + err.Error()
		}
	} else {
		s.QualityNote = "not measured (no scorer supplied) - use the blind pack for human rating"
	}
	switch {
	case t.License == nil:
		s.LicenseNote = "engine not in licence register - do not ship"
	case !t.License.ProductionAllowed:
		s.LicenseNote = "licence register: production NOT allowed (code reported " + t.License.License + "; weights licence must be verified separately)"
	default:
		s.LicenseNote = "licence register: production allowed (" + t.License.License + ")"
	}
	if len(s.FailureByKind) == 0 {
		s.FailureByKind = nil
	}
	return s
}

// writeBlindPack copies successful renders under shuffled labels per prompt
// so raters cannot tell engines apart. The key is written separately and
// must not be given to raters.
func writeBlindPack(outDir string, runs []Run) (string, error) {
	byPrompt := map[string][]Run{}
	for _, r := range runs {
		if r.OK && r.Attempt == 1 {
			byPrompt[r.PromptID] = append(byPrompt[r.PromptID], r)
		}
	}
	if len(byPrompt) == 0 {
		return "", nil
	}
	dir := filepath.Join(outDir, "blind")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	key := map[string]map[string]string{}
	ids := make([]string, 0, len(byPrompt))
	for id := range byPrompt {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		rs := byPrompt[id]
		clips := make([]voiceeval.Clip, len(rs))
		for i, r := range rs {
			clips[i] = voiceeval.Clip{ID: r.WAV, Source: r.Engine}
		}
		blinded, err := voiceeval.Blind(clips)
		if err != nil {
			return "", err
		}
		key[id] = map[string]string{}
		for _, c := range blinded {
			b, err := os.ReadFile(c.ID)
			if err != nil {
				return "", err
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%s.wav", id, c.BlindLabel)), b, 0o644); err != nil {
				return "", err
			}
			key[id][c.BlindLabel] = c.Source
		}
	}
	kb, _ := json.MarshalIndent(key, "", "  ")
	if err := os.WriteFile(filepath.Join(outDir, "blind_key.json"), kb, 0o600); err != nil {
		return "", err
	}
	return dir, nil
}

// Markdown renders a human summary.
func (r *Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Voice engine benchmark\n\n%s -> %s, golden set v%d, %d run(s) per prompt", r.StartedAt, r.FinishedAt, r.GoldenSet, r.Runs)
	if r.VoiceID != "" {
		fmt.Fprintf(&b, ", voice `%s`", r.VoiceID)
	}
	b.WriteString("\n\n| Engine | Runs | Fail % | p50 ms | p95 ms | First audio p50 | Mean RTF | ICF_VOICE_SCORE | Licence |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, e := range r.Engines {
		score := "not measured"
		if e.Score != nil {
			score = fmt.Sprintf("%.3f (cov %.0f%%)", e.Score.ICFVoiceScore, 100*e.Score.Coverage)
		}
		fa := "-"
		if e.FirstAudioP50 > 0 {
			fa = fmt.Sprintf("%.0f", e.FirstAudioP50)
		}
		fmt.Fprintf(&b, "| %s | %d | %.1f | %.0f | %.0f | %s | %.4f | %s | %s |\n", e.Engine, e.Runs, 100*e.FailureRate,
			e.LatencyP50MS, e.LatencyP95MS, fa, e.MeanRTF, score, e.LicenseNote)
	}
	b.WriteString("\nRTF = wall-clock latency / audio duration (lower is faster; < 1 is faster than real time).\n\n")
	for _, e := range r.Engines {
		if len(e.FailureByKind) > 0 {
			fmt.Fprintf(&b, "- **%s** failures by class: %v\n", e.Engine, e.FailureByKind)
		}
	}
	if r.BlindPack != "" {
		fmt.Fprintf(&b, "\nBlind listening pack: `%s` (give raters this folder only; `blind_key.json` maps labels to engines).\n", r.BlindPack)
	}
	b.WriteString("\n")
	for _, d := range r.Disclaimers {
		fmt.Fprintf(&b, "> %s\n", d)
	}
	return b.String()
}

// ---------------------------------------------------------------- helpers

func ms(d time.Duration) float64 { return round3(float64(d.Microseconds()) / 1000) }

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }

func round6(f float64) float64 { return math.Round(f*1e6) / 1e6 }

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// pct is nearest-rank percentile.
func pct(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	i := int(math.Ceil(p/100*float64(len(s)))) - 1
	if i < 0 {
		i = 0
	}
	return round3(s[i])
}

// wavDurationMS parses a PCM WAV and returns its duration, or 0 if the bytes
// are not a decodable WAV.
func wavDurationMS(b []byte) float64 {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return 0
	}
	var rate, blockAlign uint32
	for off := 12; off+8 <= len(b); {
		id := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		body := off + 8
		switch id {
		case "fmt ":
			if body+16 > len(b) {
				return 0
			}
			rate = binary.LittleEndian.Uint32(b[body+4 : body+8])
			blockAlign = uint32(binary.LittleEndian.Uint16(b[body+12 : body+14]))
		case "data":
			if rate == 0 || blockAlign == 0 {
				return 0
			}
			n := len(b) - body
			if size >= 0 && size < n && uint32(size) != 0xFFFFFFFF {
				n = size
			}
			return 1000 * float64(n/int(blockAlign)) / float64(rate)
		}
		if size < 0 || uint32(size) == 0xFFFFFFFF {
			return 0
		}
		off = body + size + size%2
	}
	return 0
}

// fixStreamHeader rewrites the "unknown length" sizes a streamed WAV header
// carries, so saved renders open in every player.
func fixStreamHeader(b []byte) []byte {
	if len(b) < 44 || string(b[0:4]) != "RIFF" || binary.LittleEndian.Uint32(b[4:8]) != 0xFFFFFFFF {
		return b
	}
	out := append([]byte(nil), b...)
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	if string(out[36:40]) == "data" {
		binary.LittleEndian.PutUint32(out[40:44], uint32(len(out)-44))
	}
	return out
}
