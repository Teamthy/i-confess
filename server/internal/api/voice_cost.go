package api

// Voice cost telemetry and the batch budget ceiling (audit VE-017).
//
// Before this file the answer to "what did that render cost?" was "unknown",
// in both directions: nothing recorded how long a GPU spent on a render, and
// nothing limited how many renders one admin click could demand. The only
// controls were a per-user rate limit and the content-hash cache, so a
// 10,000-item batch - explicitly allowed by the batch endpoint - was an
// unbounded spend with a 202 in front of it.
//
// Two halves, deliberately separate:
//
//   - Telemetry is *measured*: the worker times its own inference and its whole
//     request, the API stores both on the generation, and the aggregate is
//     exposed in metrics. Nothing here invents a number when a worker did not
//     report one.
//   - A ceiling is a *policy*: it has to be enforceable before the work is
//     queued, so it uses an estimate. An estimate is only honest if it names
//     its basis, so every estimate carries one - measured from this
//     deployment's own completed renders where those exist, and otherwise an
//     explicitly documented assumption. The assumption is real-time
//     (one GPU-second per audio-second), which over-counts a fast engine and
//     under-counts none: when a guess has to bound spending, it should err
//     toward refusing work rather than toward paying for it.
//
// The price is never guessed either. VOICE_GPU_SECOND_COST_USD defaults to
// unset, and an unset price means the deployment sees seconds and no dollars.

import (
	"context"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/voiceengine"
)

// estSecondsPerAudioSecond is the fallback cost ratio: one second of GPU time
// per second of finished audio. Real engines are usually faster than that, so
// the fallback over-estimates spend, which is the safe direction for a ceiling.
const estSecondsPerAudioSecond = 1.0

// estWordsPerMinute is how fast an unmeasured engine is assumed to speak when
// converting a script into seconds of audio (132 wpm is deliberate liturgical
// pace; conversational speech is faster, so this also leans conservative).
const estWordsPerMinute = 132.0

type voiceCostConfig struct {
	// USD per GPU-second. 0 means "not configured": telemetry continues, cost
	// stays unpriced, and the API says so instead of reporting $0.00.
	PricePerSecond float64
	// Ceiling for one batch, in inference seconds. 0 disables the per-batch
	// check. The default is one GPU-hour, which is ~£1-2 of A10 time and
	// already three orders of magnitude above a Psalm-and-a-bit batch.
	BatchCeilingSeconds float64
	// Ceiling for one voice's queued work in the current UTC day. 0 disables.
	DailyCeilingSeconds float64
}

func envFloatDefault(key string, def float64) float64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			return f
		}
	}
	return def
}

func voiceCostFromEnv() voiceCostConfig {
	return voiceCostConfig{
		PricePerSecond:      envFloatDefault("VOICE_GPU_SECOND_COST_USD", 0),
		BatchCeilingSeconds: envFloatDefault("VOICE_BATCH_GPU_BUDGET_SECONDS", 3600),
		DailyCeilingSeconds: envFloatDefault("VOICE_GPU_DAILY_BUDGET_SECONDS", 0),
	}
}

// price converts seconds to integer micro-dollars. ok is false when no price is
// configured, which callers must render as "unpriced" rather than as zero.
func (c voiceCostConfig) price(seconds float64) (int64, bool) {
	if c.PricePerSecond <= 0 || seconds <= 0 {
		return 0, false
	}
	return int64(math.Round(seconds * c.PricePerSecond * 1e6)), true
}

func (c voiceCostConfig) pricePtr(seconds float64) *int64 {
	if micros, ok := c.price(seconds); ok {
		return &micros
	}
	return nil
}

// scriptAudioSeconds estimates the length of finished audio for one script:
// words at the assumed pace plus any {pause} the markup asks for. Markup is
// parsed the same way generation parses it, so a script the generator would
// reject is not silently cheaper here.
func scriptAudioSeconds(markup string) float64 {
	segs, _ := voiceengine.ParseMarkup(markup)
	words := float64(len(strings.Fields(voiceengine.PlainText(segs))))
	s := words / estWordsPerMinute * 60.0
	for _, seg := range segs {
		if seg.PauseMS > 0 {
			s += float64(seg.PauseMS) / 1000
		}
	}
	if s < 1 {
		s = 1 // an empty or one-word script still costs a request
	}
	return s
}

// voiceCostEstimate is a plan with its provenance.
type voiceCostEstimate struct {
	Items          int     `json:"items"`
	AudioSeconds   float64 `json:"audioSeconds"`
	InferenceSecs  float64 `json:"inferenceSeconds"`
	CostMicros     *int64  `json:"costUsdMicros,omitempty"`
	Basis          string  `json:"basis"`
	RatioPerAudioS float64 `json:"secondsPerAudioSecond"`
}

// estimateVoiceCost prices a list of scripts using this deployment's measured
// history when it has any. history is the (voice-scoped) spend aggregate; when
// it cannot yield a ratio the documented real-time assumption is used and said.
func estimateVoiceCost(texts []string, history store.AudioSpend, cfg voiceCostConfig) voiceCostEstimate {
	e := voiceCostEstimate{Items: len(texts)}
	for _, t := range texts {
		e.AudioSeconds += scriptAudioSeconds(t)
	}
	if r, ok := history.SecondsPerAudioSecond(); ok {
		e.RatioPerAudioS, e.Basis = r, "measured: "+strconv.FormatFloat(r, 'f', 3, 64)+
			" GPU-seconds per audio-second over "+strconv.Itoa(history.Metered)+" completed render(s)"
	} else {
		e.RatioPerAudioS, e.Basis = estSecondsPerAudioSecond,
			"assumed: no completed render has reported a duration on this deployment, so 1.0 GPU-second per audio-second is used (over-counts a fast engine, under-counts none)"
	}
	e.InferenceSecs = e.AudioSeconds * e.RatioPerAudioS
	e.CostMicros = cfg.pricePtr(e.InferenceSecs)
	return e
}

// budgetVerdict is the answer a bulk caller gets before anything is queued.
type budgetVerdict struct {
	Estimate  voiceCostEstimate `json:"estimate"`
	SpentDay  float64           `json:"spentTodayInferenceSeconds"`
	Remaining float64           `json:"remainingSeconds"`
	ItemsFit  int               `json:"itemsFit"`
	// History and Config are carried so a caller can re-estimate over a
	// smaller set without asking the database what it already told us.
	History store.AudioSpend `json:"-"`
	Config  voiceCostConfig  `json:"-"`
}

// checkBatchBudget answers "may this batch be queued at all?". It is a brake,
// not an accounting lock: concurrent submitters each see the spend as of their
// own read, so two batches can both pass and overshoot. Closing that would
// need a reservation the queue holds across a render, which is a different
// subsystem; the ceiling's job is to stop one click buying a GPU-week, and it
// does that deterministically.
func (h *Handler) checkBatchBudget(ctx context.Context, voiceID string, texts []string) (budgetVerdict, error) {
	cfg := voiceCostFromEnv()
	v := budgetVerdict{}
	hist, err := h.vplat.VoiceSpend(ctx, voiceID, "")
	if err != nil {
		return v, err
	}
	v.History, v.Config = hist, cfg
	v.Estimate = estimateVoiceCost(texts, hist, cfg)
	if cfg.DailyCeilingSeconds > 0 {
		since := time.Now().UTC().Truncate(24 * time.Hour).Format(time.RFC3339)
		today, terr := h.vplat.VoiceSpend(ctx, voiceID, since)
		if terr != nil {
			return v, terr
		}
		v.SpentDay = today.InferenceSeconds
	}
	remaining := math.Inf(1)
	if cfg.BatchCeilingSeconds > 0 {
		remaining = cfg.BatchCeilingSeconds - v.Estimate.InferenceSecs
	}
	if cfg.DailyCeilingSeconds > 0 {
		if r := cfg.DailyCeilingSeconds - v.SpentDay; r < remaining {
			remaining = r
		}
	}
	if math.IsInf(remaining, 1) {
		v.Remaining = -1 // no ceiling configured
		v.ItemsFit = len(texts)
		return v, nil
	}
	v.Remaining = remaining
	per := 0.0
	if len(texts) > 0 {
		per = v.Estimate.InferenceSecs / float64(len(texts))
	}
	if per > 0 {
		v.ItemsFit = int(remaining / per)
	}
	if remaining < 0 {
		v.ItemsFit = 0
	}
	if v.ItemsFit > len(texts) {
		v.ItemsFit = len(texts)
	}
	if v.ItemsFit < len(texts) {
		return v, &budgetExceeded{verdict: v, cfg: cfg}
	}
	return v, nil
}

type budgetExceeded struct {
	verdict budgetVerdict
	cfg     voiceCostConfig
}

func (b *budgetExceeded) Error() string {
	v := b.verdict
	s := "this batch would spend " + strconv.FormatFloat(v.Estimate.InferenceSecs, 'f', 1, 64) +
		" GPU-seconds across " + strconv.Itoa(v.Estimate.Items) + " item(s)"
	if b.cfg.BatchCeilingSeconds > 0 && v.Estimate.InferenceSecs > b.cfg.BatchCeilingSeconds {
		s += "; the per-batch ceiling is " + strconv.FormatFloat(b.cfg.BatchCeilingSeconds, 'f', 0, 64) + "s"
	}
	if b.cfg.DailyCeilingSeconds > 0 {
		s += "; this voice has used " + strconv.FormatFloat(v.SpentDay, 'f', 1, 64) + "s of its " +
			strconv.FormatFloat(b.cfg.DailyCeilingSeconds, 'f', 0, 64) + "s daily ceiling"
	}
	return s
}

// writeBudgetError answers a bulk request the budget refuses. The response
// names the number that fits, because "no" without a threshold the operator can
// act on just produces a smaller request and the same conversation.
func (h *Handler) writeBudgetError(w http.ResponseWriter, r *http.Request, voiceID, actor string, b *budgetExceeded) {
	// Counted here rather than at each call site: this is the one place a
	// refusal becomes visible to a client, so icf_voice_budget_refusals_total
	// cannot drift out of step with the 422s that were actually sent.
	voiceMetrics.budgetRefused()
	_ = h.vplat.AppendRightsAudit(r.Context(), store.RightsAuditEntry{VoiceID: voiceID, Actor: actor,
		Action: "VOICE_BATCH_REFUSED_BUDGET", Decision: "denied", Reason: "budget", Detail: b.Error(),
		RemoteAddr: clientIP(r)})
	httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
		"error":   b.Error(),
		"code":    "voice_budget_exceeded",
		"refused": "before_queueing",
		"budget": map[string]any{"batch_ceiling_seconds": b.cfg.BatchCeilingSeconds,
			"daily_ceiling_seconds":         b.cfg.DailyCeilingSeconds,
			"spent_today_inference_seconds": b.verdict.SpentDay,
			"remaining_seconds":             b.verdict.Remaining},
		"estimate":  b.verdict.Estimate,
		"items_fit": b.verdict.ItemsFit,
		"priced":    b.cfg.PricePerSecond > 0,
		"next": "queue at most " + strconv.Itoa(b.verdict.ItemsFit) +
			" item(s), shorten the scripts, or raise VOICE_BATCH_GPU_BUDGET_SECONDS (0 disables the per-batch ceiling)",
	})
}

// recordRenderCost stores what the worker measured, and accounts it against the
// in-process metrics. A worker that reports nothing leaves the columns NULL:
// "unmeasured" and "free" are different facts and only one of them is a
// reason to run another batch.
func (h *Handler) recordRenderCost(g *store.Generation, res *voiceengine.GenerateResult, cfg voiceCostConfig) {
	if res.InferenceSeconds > 0 {
		v := res.InferenceSeconds
		g.InferenceSeconds = &v
	}
	if res.WorkerSeconds > 0 {
		v := res.WorkerSeconds
		g.WorkerSeconds = &v
	}
	if micros, ok := cfg.price(res.InferenceSeconds); ok {
		g.CostUSDMicros = &micros
	}
	voiceMetrics.cost(res.InferenceSeconds, res.WorkerSeconds, float64(res.DurationMS)/1000, g.CostUSDMicros)
}
