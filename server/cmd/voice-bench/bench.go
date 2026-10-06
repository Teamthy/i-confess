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
	// Expect is this prompt's oracle (audit VE-020): what a finished render
	// has to look like, expressed as measurements rather than opinions.
	Expect *Expect `json:"expect,omitempty"`
}

// Expect is the set of checks the harness can run on its own, with no external
// scorer and no GPU. That is the whole point: the benchmark used to describe
// renders and never judge one, so a run that produced three seconds of silence
// for a ninety-second scripture reading was reported as a success with a
// suspicious-looking number in it.
//
// What these cannot do is answer "does it sound like the minister". Speaker
// similarity, naturalness and pronunciation against a model still need
// -scorer, and the report says which of the two it measured.
type Expect struct {
	// Audio length bounds catch truncation and runaway repetition, the two
	// failure modes that are invisible to latency statistics.
	AudioSecondsMin float64 `json:"audio_seconds_min,omitempty"`
	AudioSecondsMax float64 `json:"audio_seconds_max,omitempty"`
	// MaxRealTimeFactor bounds wall-clock latency per second of audio.
	MaxRealTimeFactor float64 `json:"max_real_time_factor,omitempty"`
	// MinPeakDBFS catches "the engine produced a file that is silence".
	MinPeakDBFS float64 `json:"min_peak_dbfs,omitempty"`
	// MaxSilenceRatio catches a render that is mostly padding.
	MaxSilenceRatio float64 `json:"max_silence_ratio,omitempty"`
	// NoClipping asserts the mastering ceiling held. It should never fail,
	// which is exactly why it is worth a check: the limiter is the guarantee
	// every downstream delivery relies on.
	NoClipping bool `json:"no_clipping,omitempty"`
	// NormalisedContains are strings that must appear, case-insensitively, in
	// the text actually sent to the engine after normalisation. This is the
	// VE-004 oracle: "John 3:16" must reach inference as "john chapter three
	// verse sixteen", and a regression in the normaliser is audible to anyone
	// but measurable by nobody without it.
	NormalisedContains []string `json:"normalised_contains,omitempty"`
}

// expectKeys is every field Expect accepts. A golden set is edited by hand, and
// a typo ("audio_seconds_mnin") would otherwise disable a check while looking
// like it added one.
var expectKeys = []string{"audio_seconds_min", "audio_seconds_max", "max_real_time_factor", "min_peak_dbfs",
	"max_silence_ratio", "no_clipping", "normalised_contains"}

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
	if err := gs.validate(b); err != nil {
		return nil, err
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

// validate rejects an expectation that cannot be honoured: an inverted range,
// an unknown key, or bounds so tight that they measure the prompt rather than a
// render. A golden set with a broken oracle is worse than one without, because
// the report then reads as though something was checked.
func (g *GoldenSet) validate(src []byte) error {
	// Unknown keys are found in the source, not in the decoded struct: json
	// ignores what it does not know, so "audio_seconds_mnin" would have already
	// vanished by the time validate sees the prompt.
	var doc struct {
		Prompts []struct {
			ID     string                     `json:"id"`
			Expect map[string]json.RawMessage `json:"expect"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(src, &doc); err != nil {
		return fmt.Errorf("golden set: %w", err)
	}
	known := map[string]bool{}
	for _, k := range expectKeys {
		known[k] = true
	}
	for _, raw := range doc.Prompts {
		for k := range raw.Expect {
			if !known[k] {
				return fmt.Errorf("golden set: %s: unknown expectation %q (typos disable a check while looking like they add one)",
					raw.ID, k)
			}
		}
	}
	for _, p := range g.Prompts {
		if p.Expect == nil {
			continue
		}
		lo, hi := p.Expect.AudioSecondsMin, p.Expect.AudioSecondsMax
		if lo < 0 || hi < 0 {
			return fmt.Errorf("golden set: %s: audio bounds cannot be negative", p.ID)
		}
		if lo > 0 && hi > 0 && lo >= hi {
			return fmt.Errorf("golden set: %s: audio_seconds_min %g >= audio_seconds_max %g", p.ID, lo, hi)
		}
		if words := len(strings.Fields(p.Text)); lo > 0 && hi > 0 && words > 0 {
			// The floor cannot demand more audio than 40 wpm of speech fills,
			// and the ceiling cannot be tighter than 600 wpm: either way the
			// bound is a typo, and a typo here means every engine "fails" (or
			// every engine passes) for reasons that have nothing to do with audio.
			if lo/float64(words) > 60.0/40 || hi/float64(words) < 60.0/600 {
				return fmt.Errorf("golden set: %s: audio bounds %g-%gs for %d words imply a speaking rate outside 40-600 wpm",
					p.ID, lo, hi, words)
			}
		}
		if p.Expect.MaxRealTimeFactor < 0 {
			return fmt.Errorf("golden set: %s: max_real_time_factor cannot be negative", p.ID)
		}
		if p.Expect.MaxSilenceRatio < 0 || p.Expect.MaxSilenceRatio > 1 {
			return fmt.Errorf("golden set: %s: max_silence_ratio must be within [0,1]", p.ID)
		}
	}
	return nil
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
	Runs   int
	Stream bool
	// Strict makes a failed expectation an exit status rather than a line in a
	// report; nightly runs want a gate, exploratory runs want the numbers.
	Strict    bool
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
	// Signal is what the rendered bytes measured out to, whether or not the
	// prompt declared expectations. Recording it is what makes a later
	// tightening of the bounds evidence-based rather than a guess.
	Signal *audioSignal `json:"signal,omitempty"`
	// ChecksFailed lists the prompt's expectations this render missed. A run
	// can be OK (audio came back) and still fail its oracle; collapsing the two
	// would hide truncation inside a success rate.
	ChecksFailed []string `json:"checks_failed,omitempty"`
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
	ChecksFailed  int               `json:"checks_failed"`
	// FailedByPrompt says *which* prompt an engine cannot satisfy. One prompt
	// failing ten times is a different problem from ten prompts failing once.
	FailedByPrompt map[string]int `json:"failed_by_prompt,omitempty"`
	QualityNote    string         `json:"quality_note"`
	License        *LicenseEntry  `json:"license,omitempty"`
	LicenseNote    string         `json:"license_note"`
	Capabilities   any            `json:"capabilities"`
}

// Report is the full benchmark output.
type Report struct {
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	GoldenSet  int    `json:"golden_set_version"`
	VoiceID    string `json:"voice_id,omitempty"`
	Runs       int    `json:"runs_per_prompt"`
	// Strict is recorded so a report says whether anyone was meant to act on
	// it: the same failures are a gate under -strict and a note without it.
	Strict      bool            `json:"strict,omitempty"`
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
		Strict: cfg.Strict,
		Disclaimers: []string{
			voiceeval.Disclaimer,
			"Latency includes network and worker queueing on the benchmark host; compare engines on the same hardware only.",
			"Quality dimensions are reported only when an external scorer measured them.",
			"The golden set's expect blocks are signal-level checks (length, silence, clipping, normalisation), not perceptual quality.",
		}}
	for _, t := range targets {
		caps := t.Provider.Capabilities(ctx)
		var runs []Run
		for _, p := range gs.Prompts {
			segs, _ := voiceengine.ParseMarkup(p.Text) // validated on load
			// Normalise exactly where the production orchestrator does
			// (voiceengine.Orchestrator.apply), otherwise the benchmark would
			// measure a different prompt from the one users get - and the
			// normalisation checks below would pass on text the engine never saw.
			segs = voiceengine.NormaliseSegments(segs, p.Language, p.Locale)
			chunks := voiceengine.Render(segs, caps, voiceengine.ProsodyProfile{})
			for a := 1; a <= cfg.Runs; a++ {
				r := benchOne(ctx, t, caps, p, spokenText(chunks), chunks, a, cfg)
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

func benchOne(ctx context.Context, t Target, caps voiceengine.ProviderCapabilities, p Prompt, spoken string,
	chunks []voiceengine.Chunk, attempt int, cfg Config) Run {
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
	sig := analyseWAV(audio)
	r.Signal = &sig
	if p.Expect != nil {
		r.ChecksFailed = p.Expect.checks(r, sig, spoken)
	}
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
		if len(r.ChecksFailed) > 0 {
			s.ChecksFailed += len(r.ChecksFailed)
			if s.FailedByPrompt == nil {
				s.FailedByPrompt = map[string]int{}
			}
			s.FailedByPrompt[r.PromptID] += len(r.ChecksFailed)
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
	b.WriteString("\n\n| Engine | Runs | Fail % | Checks failed | p50 ms | p95 ms | First audio p50 | Mean RTF | ICF_VOICE_SCORE | Licence |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, e := range r.Engines {
		score := "not measured"
		if e.Score != nil {
			score = fmt.Sprintf("%.3f (cov %.0f%%)", e.Score.ICFVoiceScore, 100*e.Score.Coverage)
		}
		fa := "-"
		if e.FirstAudioP50 > 0 {
			fa = fmt.Sprintf("%.0f", e.FirstAudioP50)
		}
		fmt.Fprintf(&b, "| %s | %d | %.1f | %d | %.0f | %.0f | %s | %.4f | %s | %s |\n", e.Engine, e.Runs, 100*e.FailureRate,
			e.ChecksFailed, e.LatencyP50MS, e.LatencyP95MS, fa, e.MeanRTF, score, e.LicenseNote)
	}
	b.WriteString("\nRTF = wall-clock latency / audio duration (lower is faster; < 1 is faster than real time).\n\n")
	b.WriteString("Checks are the golden set's `expect` blocks: measured properties of the rendered bytes and of " +
		"the normalised text sent to the engine. They are not perceptual quality; they catch truncation, silence, " +
		"clipping, runaway length and a broken text-normaliser.\n\n")
	var failed []string
	for _, d := range r.Detail {
		if len(d.ChecksFailed) == 0 {
			continue
		}
		failed = append(failed, fmt.Sprintf("- `%s` / %s attempt %d:", d.Engine, d.PromptID, d.Attempt))
		for _, c := range d.ChecksFailed {
			failed = append(failed, "  - "+c)
		}
	}
	for _, e := range r.Engines {
		if len(e.FailureByKind) > 0 {
			fmt.Fprintf(&b, "- **%s** failures by class: %v\n", e.Engine, e.FailureByKind)
		}
	}
	if len(failed) > 0 {
		// Whether the run was gating changes what the reader does next, and it
		// belongs next to the failures rather than as a header nobody reads.
		b.WriteString("\nFailed expectations:\n")
		b.WriteString(strings.Join(failed, "\n"))
		b.WriteString("\n")
		if r.Strict {
			b.WriteString("\nThis run was gating (-strict), so the process exits 2.\n")
		} else {
			b.WriteString("\nThis run was not gating: the same failures exit 2 under -strict.\n")
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

// spokenText is the text the engine will be asked to speak: every chunk joined
// in order. Containment is checked against this rather than against one chunk,
// because a phrase can straddle a sentence boundary; the engine speaks the
// concatenation, so the concatenation is what a promise about wording applies to.
func spokenText(chunks []voiceengine.Chunk) string {
	var sb strings.Builder
	for _, c := range chunks {
		sb.WriteString(c.Text)
		sb.WriteString(" ")
	}
	return strings.Join(strings.Fields(strings.ToLower(sb.String())), " ")
}

// audioSignal is what one can learn about a render from its bytes.
type audioSignal struct {
	Seconds     float64 `json:"seconds"`
	PeakDBFS    float64 `json:"peak_dbfs"`
	Clipped     int     `json:"clipped_samples"`
	SilenceRate float64 `json:"silence_ratio"`
}

// analyseWAV decodes PCM16 (the format the worker masters to) and measures it.
// Anything it cannot parse comes back as a zero signal with Seconds 0, which
// the length check turns into a failure rather than a silent pass.
func analyseWAV(b []byte) audioSignal {
	const silenceDBFS = -45.0
	out := audioSignal{Seconds: wavDurationMS(b) / 1000}
	if len(b) < 44 || string(b[0:4]) != "RIFF" {
		return out
	}
	var rate, blockAlign uint32
	for off := 12; off+8 <= len(b); {
		id := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		body := off + 8
		if id == "fmt " && body+16 <= len(b) {
			rate = binary.LittleEndian.Uint32(b[body+4 : body+8])
			blockAlign = uint32(binary.LittleEndian.Uint16(b[body+12 : body+14]))
		}
		if id == "data" {
			end := len(b)
			if size >= 0 && body+size < end {
				end = body + size
			}
			if blockAlign == 0 || rate == 0 {
				return out
			}
			frames := (end - body) / int(blockAlign)
			chans := int(blockAlign / 2)
			if chans < 1 {
				chans = 1
			}
			var peak float64
			clipped, silent, windows := 0, 0, 0
			win := int(rate) / 50 // 20 ms
			if win < 1 {
				win = 1
			}
			var energy float64
			count := 0
			for i := 0; i < frames; i++ {
				base := body + i*int(blockAlign)
				if base+2*chans > len(b) {
					break
				}
				var frame float64
				for c := 0; c < chans; c++ {
					v := float64(int16(binary.LittleEndian.Uint16(b[base+2*c:base+2*c+2]))) / 32768
					if math.Abs(v) > peak {
						peak = math.Abs(v)
					}
					if math.Abs(v) >= 0.999 {
						clipped++
					}
					frame += v * v
				}
				frame /= float64(chans)
				energy += frame
				count++
				if count == win {
					windows++
					if energy/float64(count) < math.Pow(10, silenceDBFS/10) {
						silent++
					}
					energy, count = 0, 0
				}
			}
			if windows > 0 {
				out.SilenceRate = round3(float64(silent) / float64(windows))
			} else {
				out.SilenceRate = 1
			}
			out.Clipped = clipped
			if peak > 0 {
				out.PeakDBFS = round3(20 * math.Log10(peak))
			} else {
				out.PeakDBFS = -120
			}
			return out
		}
		if size < 0 || uint32(size) == 0xFFFFFFFF {
			break
		}
		off = body + size + size%2
	}
	return out
}

// checks runs this prompt's oracle against one render. It returns the failures,
// naming the threshold and the measured value for each: a report that says
// "expectation failed" without the numbers cannot be acted on, only re-run.
func (e *Expect) checks(r Run, sig audioSignal, normalised string) []string {
	var bad []string
	fail := func(format string, args ...any) { bad = append(bad, fmt.Sprintf(format, args...)) }
	if e.AudioSecondsMin > 0 && sig.Seconds < e.AudioSecondsMin {
		fail("audio %.1fs is shorter than the expected %.1fs (truncation?)", sig.Seconds, e.AudioSecondsMin)
	}
	if e.AudioSecondsMax > 0 && sig.Seconds > e.AudioSecondsMax {
		fail("audio %.1fs is longer than the expected %.1fs (repetition or runaway?)", sig.Seconds, e.AudioSecondsMax)
	}
	if e.MaxRealTimeFactor > 0 && r.RTF > e.MaxRealTimeFactor {
		fail("real-time factor %.2f exceeds the bound %.2f", r.RTF, e.MaxRealTimeFactor)
	}
	if e.MinPeakDBFS != 0 && sig.PeakDBFS < e.MinPeakDBFS {
		fail("peak %.1f dBFS is below the floor %.1f (near-silence render)", sig.PeakDBFS, e.MinPeakDBFS)
	}
	if e.MaxSilenceRatio > 0 && sig.SilenceRate > e.MaxSilenceRatio {
		fail("silence ratio %.2f exceeds the bound %.2f", sig.SilenceRate, e.MaxSilenceRatio)
	}
	if e.NoClipping && sig.Clipped > 0 {
		fail("%d sample(s) at full scale: the mastering ceiling did not hold", sig.Clipped)
	}
	for _, want := range e.NormalisedContains {
		if !strings.Contains(strings.ToLower(normalised), strings.ToLower(want)) {
			fail("normalised text handed to the engine does not contain %q", want)
		}
	}
	return bad
}

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
