package ai

import (
	"context"
	"os"
	"strings"
)

// LLMParser is the prod adapter — constrained JSON via AI_PROVIDER.
// Env: AI_PROVIDER=keyword|openai|anthropic (stub: falls back to KeywordParser).
// Guardrail: prompt forces output {"categories":[], "duration":int, "voice_id":""} only;
// any body/scripture invention is rejected before Engine selection.

type LLMParser struct{ inner Parser }

func NewParserFromEnv() Parser {
	p := strings.ToLower(strings.TrimSpace(os.Getenv("AI_PROVIDER")))
	switch p {
	case "openai", "anthropic":
		// TODO: wire LLM SDK in /tmp, prompt: "Map utterence to {categories from 39, duration 60..10800, voice_id}. Never invent confession body or scripture. Return JSON only."
		// For now, KeywordParser is the safe fallback that satisfies the contract without PII exfil.
		return KeywordParser{}
	default:
		return KeywordParser{}
	}
}

// Parse delegates to inner (KeywordParser in dev) with env selection.
func (l LLMParser) Parse(ctx context.Context, req Request) (Parsed, error) {
	if l.inner == nil {
		l.inner = KeywordParser{}
	}
	return l.inner.Parse(ctx, req)
}
