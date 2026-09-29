// Package voiceeval is the iCONFESS Voice Similarity Evaluation Framework
// (Voice Platform §3, §49-§52, §74).
//
// ICF_VOICE_SCORE is an INTERNAL composite. It is a weighted mean of
// dimension scores that the evaluation harness measured (speaker-embedding
// cosine similarity, MOS-style naturalness ratings, ASR-derived text accuracy
// and so on), each normalised to [0,1]. It is useful for comparing models and
// engines against each other on the same test set. It is not an industry
// percentage, and nothing in the product may present it as "N% like the
// minister".
//
// Promotion to production is never decided by the score alone: Gate returns
// NEEDS_REVIEW at best until a human approval and a live rights check are
// both present.
package voiceeval

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
)

// Dimension is one measured quality axis.
type Dimension string

const (
	SpeakerSimilarity  Dimension = "speaker_similarity"
	Naturalness        Dimension = "naturalness"
	Pronunciation      Dimension = "pronunciation"
	AccentPreservation Dimension = "accent_preservation"
	Prosody            Dimension = "prosody"
	EmotionalConsist   Dimension = "emotional_consistency"
	StyleConsistency   Dimension = "style_consistency"
	LongFormStability  Dimension = "long_form_stability"
	Intelligibility    Dimension = "intelligibility"
	TextAccuracy       Dimension = "text_accuracy"
	// ArtifactScore is 1 - artifact rate, so higher is better like every other
	// dimension and the weighted mean needs no special case.
	ArtifactScore Dimension = "artifact_score"
)

// AllDimensions in a stable order.
var AllDimensions = []Dimension{
	SpeakerSimilarity, Naturalness, Pronunciation, AccentPreservation, Prosody,
	EmotionalConsist, StyleConsistency, LongFormStability, Intelligibility,
	TextAccuracy, ArtifactScore,
}

// Weights maps dimensions to relative weights. They need not sum to 1; the
// score normalises by the total weight of dimensions actually measured.
type Weights map[Dimension]float64

// DefaultWeights is the starting configuration from the build spec. It is a
// starting point for the evaluation programme, not a finding.
func DefaultWeights() Weights {
	return Weights{
		SpeakerSimilarity:  0.30,
		Naturalness:        0.20,
		Pronunciation:      0.15,
		AccentPreservation: 0.10,
		Prosody:            0.10,
		StyleConsistency:   0.05,
		LongFormStability:  0.05,
		ArtifactScore:      0.05,
	}
}

// Validate rejects weights that would make the score meaningless.
func (w Weights) Validate() error {
	total := 0.0
	for d, v := range w {
		if !validDimension(d) {
			return fmt.Errorf("unknown dimension %q", d)
		}
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("weight for %s must be a finite non-negative number", d)
		}
		total += v
	}
	if total <= 0 {
		return errors.New("weights sum to zero")
	}
	return nil
}

// Metrics are measured dimension scores in [0,1].
type Metrics map[Dimension]float64

// Score is a computed composite with provenance.
type Score struct {
	ICFVoiceScore float64 `json:"icf_voice_score"`
	// Coverage is the fraction of configured weight that was actually
	// measured. A 0.95 score over 40% coverage is not comparable to one over
	// 100%, and the gate refuses low coverage outright.
	Coverage   float64  `json:"coverage"`
	Unmeasured []string `json:"unmeasured,omitempty"`
	Weights    Weights  `json:"weights"`
	Metrics    Metrics  `json:"metrics"`
	Disclaimer string   `json:"disclaimer"`
}

// Disclaimer is attached to every Score so it travels with the number.
const Disclaimer = "ICF_VOICE_SCORE is an internal comparative composite, not an objective or industry-standard similarity percentage."

// Compute returns the weighted composite over measured dimensions.
func Compute(m Metrics, w Weights) (Score, error) {
	if w == nil {
		w = DefaultWeights()
	}
	if err := w.Validate(); err != nil {
		return Score{}, err
	}
	var sum, measuredW, totalW float64
	var unmeasured []string
	for d, weight := range w {
		totalW += weight
		v, ok := m[d]
		if !ok {
			if weight > 0 {
				unmeasured = append(unmeasured, string(d))
			}
			continue
		}
		if v < 0 || v > 1 || math.IsNaN(v) {
			return Score{}, fmt.Errorf("metric %s = %v is outside [0,1]", d, v)
		}
		sum += v * weight
		measuredW += weight
	}
	sort.Strings(unmeasured)
	s := Score{Weights: w, Metrics: m, Unmeasured: unmeasured, Disclaimer: Disclaimer, Coverage: measuredW / totalW}
	if measuredW > 0 {
		s.ICFVoiceScore = round4(sum / measuredW)
	}
	s.Coverage = round4(s.Coverage)
	return s, nil
}

// Verdict is a quality-gate outcome.
type Verdict string

const (
	Pass        Verdict = "PASS"
	Fail        Verdict = "FAIL"
	NeedsReview Verdict = "NEEDS_REVIEW"
)

// Thresholds are per-dimension minimums plus a composite minimum. They must be
// set empirically from the evaluation programme (§74); the zero value checks
// nothing except the hard requirements in Gate.
type Thresholds struct {
	MinScore    float64               `json:"min_score"`
	MinCoverage float64               `json:"min_coverage"`
	Min         map[Dimension]float64 `json:"min"`
	// MaxRegression is the largest allowed drop per dimension against the
	// current production model on the same golden set.
	MaxRegression float64 `json:"max_regression"`
}

// GateInput is everything the promotion decision depends on.
type GateInput struct {
	Candidate Score
	// Baseline is the current production model's score on the same golden
	// set, if one exists.
	Baseline *Score
	// RightsAllowed is the live voicegov decision for generation.
	RightsAllowed bool
	// HumanApproved is an explicit sign-off by an authorised reviewer.
	HumanApproved bool
	// BlindEvalCompleted is true once a blind human evaluation has enough
	// ratings to be meaningful.
	BlindEvalCompleted bool
}

// GateResult explains itself.
type GateResult struct {
	Verdict  Verdict  `json:"verdict"`
	Failures []string `json:"failures,omitempty"`
	Pending  []string `json:"pending,omitempty"`
}

// Gate decides whether a candidate model may be promoted to production.
//
// Automated checks can FAIL a model, but can never PASS it on their own: a
// perfect score without human approval and a blind evaluation is
// NEEDS_REVIEW (§20, §51).
func Gate(in GateInput, th Thresholds) GateResult {
	var failures, pending []string
	if !in.RightsAllowed {
		failures = append(failures, "rights verification failed: voice is not currently licensed for generation")
	}
	minCov := th.MinCoverage
	if minCov == 0 {
		minCov = 0.8
	}
	if in.Candidate.Coverage < minCov {
		failures = append(failures, fmt.Sprintf("only %.0f%% of weighted dimensions were measured (need %.0f%%)", in.Candidate.Coverage*100, minCov*100))
	}
	if in.Candidate.ICFVoiceScore < th.MinScore {
		failures = append(failures, fmt.Sprintf("ICF_VOICE_SCORE %.4f below threshold %.4f", in.Candidate.ICFVoiceScore, th.MinScore))
	}
	dims := make([]Dimension, 0, len(th.Min))
	for d := range th.Min {
		dims = append(dims, d)
	}
	sort.Slice(dims, func(i, j int) bool { return dims[i] < dims[j] })
	for _, d := range dims {
		min := th.Min[d]
		v, ok := in.Candidate.Metrics[d]
		if !ok {
			failures = append(failures, fmt.Sprintf("%s has a threshold but was not measured", d))
			continue
		}
		if v < min {
			failures = append(failures, fmt.Sprintf("%s %.4f below threshold %.4f", d, v, min))
		}
	}
	if in.Baseline != nil && th.MaxRegression > 0 {
		for _, d := range AllDimensions {
			nv, ok1 := in.Candidate.Metrics[d]
			ov, ok2 := in.Baseline.Metrics[d]
			if ok1 && ok2 && ov-nv > th.MaxRegression {
				failures = append(failures, fmt.Sprintf("%s regressed %.4f -> %.4f against production", d, ov, nv))
			}
		}
	}
	if len(failures) > 0 {
		return GateResult{Verdict: Fail, Failures: failures}
	}
	if !in.BlindEvalCompleted {
		pending = append(pending, "blind human evaluation not completed")
	}
	if !in.HumanApproved {
		pending = append(pending, "human approval not recorded")
	}
	if len(pending) > 0 {
		return GateResult{Verdict: NeedsReview, Pending: pending}
	}
	return GateResult{Verdict: Pass}
}

// ---------------------------------------------------------------------------
// Blind evaluation (§50)
// ---------------------------------------------------------------------------

// Clip is one audio sample under evaluation. Source is the secret: "REAL" or
// a model id. Evaluators only ever see BlindLabel.
type Clip struct {
	ID         string `json:"id"`
	Source     string `json:"-"`
	BlindLabel string `json:"blind_label"`
}

// Blind assigns shuffled, non-descriptive labels ("A", "B", ...) so the
// presentation order and names leak nothing about which clip is real. It uses
// crypto/rand so the ordering cannot be predicted from a seed.
func Blind(clips []Clip) ([]Clip, error) {
	out := append([]Clip(nil), clips...)
	for i := len(out) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return nil, err
		}
		j := int(n.Int64())
		out[i], out[j] = out[j], out[i]
	}
	for i := range out {
		out[i].BlindLabel = label(i)
	}
	return out, nil
}

func label(i int) string {
	s := ""
	for {
		s = string(rune('A'+i%26)) + s
		i = i/26 - 1
		if i < 0 {
			return s
		}
	}
}

// Rating is one evaluator's judgement of one clip.
type Rating struct {
	EvaluatorID string    `json:"evaluator_id"`
	TestID      string    `json:"test_id"`
	ClipID      string    `json:"clip_id"`
	Dimension   Dimension `json:"dimension"`
	// Value is on a 1-5 MOS scale.
	Value    float64 `json:"value"`
	Comments string  `json:"comments,omitempty"`
}

// SourceSummary aggregates ratings by the (unblinded) source.
type SourceSummary struct {
	Source string                `json:"source"`
	N      int                   `json:"n"`
	MOS    map[Dimension]float64 `json:"mos"`
	// Normalised maps MOS 1..5 onto 0..1 for use as Metrics.
	Normalised Metrics `json:"normalised"`
}

// Unblind aggregates ratings per source once the test closes. Ratings for a
// clip not in the test are rejected rather than silently dropped.
func Unblind(clips []Clip, ratings []Rating) (map[string]*SourceSummary, error) {
	src := map[string]string{}
	for _, c := range clips {
		src[c.ID] = c.Source
	}
	type acc struct {
		sum float64
		n   int
	}
	agg := map[string]map[Dimension]*acc{}
	count := map[string]int{}
	for _, r := range ratings {
		s, ok := src[r.ClipID]
		if !ok {
			return nil, fmt.Errorf("rating for unknown clip %q", r.ClipID)
		}
		if r.Value < 1 || r.Value > 5 {
			return nil, fmt.Errorf("rating %v outside 1-5", r.Value)
		}
		if agg[s] == nil {
			agg[s] = map[Dimension]*acc{}
		}
		if agg[s][r.Dimension] == nil {
			agg[s][r.Dimension] = &acc{}
		}
		agg[s][r.Dimension].sum += r.Value
		agg[s][r.Dimension].n++
		count[s]++
	}
	out := map[string]*SourceSummary{}
	for s, dims := range agg {
		ss := &SourceSummary{Source: s, N: count[s], MOS: map[Dimension]float64{}, Normalised: Metrics{}}
		for d, a := range dims {
			mos := a.sum / float64(a.n)
			ss.MOS[d] = round4(mos)
			ss.Normalised[d] = round4((mos - 1) / 4)
		}
		out[s] = ss
	}
	return out, nil
}

// LongFormDurations are the stability test lengths from §52, in seconds.
var LongFormDurations = []int{10, 30, 60, 300, 600, 1800}

// ReferenceDurations are the zero-shot reference lengths benchmarked in §19.
var ReferenceDurations = []int{5, 10, 15, 30}

func validDimension(d Dimension) bool {
	for _, v := range AllDimensions {
		if v == d {
			return true
		}
	}
	return false
}

func round4(f float64) float64 { return math.Round(f*10000) / 10000 }
