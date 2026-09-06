package ai

import (
	"context"
	"errors"
	"strings"

	"github.com/Teamthy/i-confess/internal/engine"
)

// Builder §43, §106 — AI orchestrates, never invents theology.
// NLU extracts {categories[], duration, context} from user utterance,
// then the trusted Session Engine selects real Scripture-rooted content.
// Prompt guardrails: no body/Scripture invention, only category/voice/duration mapping.

type Request struct {
	Utterance  string   `json:"utterance"` // free text: "I need peace before surgery"
	Categories []string `json:"categories,omitempty"`
	Duration   int      `json:"duration_seconds,omitempty"`
	VoiceID    string   `json:"voice_id,omitempty"`
}

type Parsed struct {
	Categories []string `json:"categories"` // resolved category slugs (backend 39)
	Duration   int      `json:"duration_seconds"`
	VoiceID    string   `json:"voice_id"`
	Strategy   string   `json:"strategy"` // BALANCED default
	Confidence float64  `json:"confidence"`
	Reason     string   `json:"reason"`
}

// Parser is the NLU interface. Prod uses LLM with constrained output; dev uses keyword parser.
type Parser interface {
	Parse(ctx context.Context, req Request) (Parsed, error)
}

// KeywordParser is the dev/test parser — deterministic, no LLM call.
type KeywordParser struct{}

var keywordToCategory = map[string]string{
	"heal": "healing", "sick": "healing", "pain": "healing",
	"peace": "peace", "anxious": "peace", "anxiety": "peace", "calm": "peace",
	"faith": "faith", "trust": "faith",
	"purpose": "purpose", "direction": "purpose",
	"joy": "joy", "gratitude": "gratitude", "thank": "gratitude",
	"confidence": "confidence", "courage": "confidence",
	"wisdom": "wisdom", "sleep": "sleep", "future": "future", "love": "love",
}

func (KeywordParser) Parse(_ context.Context, req Request) (Parsed, error) {
	if req.Utterance == "" && len(req.Categories) == 0 {
		return Parsed{}, errors.New("utterance or categories required")
	}
	// If caller already supplied categories, trust them (editor flow)
	if len(req.Categories) > 0 {
		dur := req.Duration
		if dur == 0 {
			dur = 1800
		}
		return Parsed{Categories: req.Categories, Duration: dur, VoiceID: req.VoiceID, Strategy: engine.StrategyBalanced, Confidence: 1, Reason: "explicit categories"}, nil
	}
	lower := strings.ToLower(req.Utterance)
	var cats []string
	seen := map[string]bool{}
	for kw, cat := range keywordToCategory {
		if strings.Contains(lower, kw) && !seen[cat] {
			cats = append(cats, cat)
			seen[cat] = true
		}
	}
	if len(cats) == 0 {
		cats = []string{"peace"} // safe default
	}
	dur := req.Duration
	if dur == 0 {
		dur = 600
		if strings.Contains(lower, "long") || strings.Contains(lower, "30") {
			dur = 1800
		}
	}
	return Parsed{Categories: cats, Duration: dur, VoiceID: req.VoiceID, Strategy: engine.StrategyBalanced, Confidence: 0.7, Reason: "keyword parser"}, nil
}

// Build validates Parsed and returns an engine.Request for trusted selection.
func (p Parsed) ToEngineRequest() (engine.Request, error) {
	if len(p.Categories) == 0 {
		return engine.Request{}, errors.New("no categories")
	}
	if p.Duration < 60 || p.Duration > 3*3600 {
		return engine.Request{}, errors.New("duration out of range")
	}
	start := p.Strategy
	if start == "" {
		start = engine.StrategyBalanced
	}
	if !engine.IsValidStrategy(start) {
		return engine.Request{}, errors.New("invalid strategy")
	}
	return engine.Request{
		CategoryIDs:     p.Categories,
		DurationSeconds: p.Duration,
		Strategy:        engine.NormalizeStrategy(start),
		VoiceID:         p.VoiceID,
	}, nil
}
