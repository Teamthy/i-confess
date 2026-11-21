package config

import (
	"strings"
	"testing"
)

// AI_PROVIDER once accepted "openai" and "anthropic" and silently ran the
// keyword parser for both. A provider that cannot run must stop the boot.
func TestAIProviderIsValidatedAtBoot(t *testing.T) {
	cases := []struct {
		provider, key, wantErr string
	}{
		{"", "", ""},
		{"keyword", "", ""},
		{"anthropic", "sk-test", ""},
		{"anthropic", "", "requires ANTHROPIC_API_KEY"},
		{"openai", "", "not one of keyword, anthropic"},
		{"claude", "sk-test", "not one of keyword, anthropic"},
	}
	for _, tc := range cases {
		t.Run(tc.provider+"/"+tc.key, func(t *testing.T) {
			// Strict checks run outside development; in development the
			// server's ai.NewParser call refuses the same values at boot.
			t.Setenv("ENV", "staging")
			t.Setenv("DATABASE_URL", "host=db")
			t.Setenv("JWT_SECRET", "a-genuinely-random-secret-value")
			t.Setenv("AUDIO_SIGN_SECRET", "another-genuinely-random-value")
			t.Setenv("AI_PROVIDER", tc.provider)
			t.Setenv("ANTHROPIC_API_KEY", tc.key)
			err := Load().Validate()
			if tc.wantErr == "" {
				// Later, unrelated checks may still fail; none may be about AI.
				if err != nil && (strings.Contains(err.Error(), "AI_PROVIDER") || strings.Contains(err.Error(), "ANTHROPIC")) {
					t.Fatalf("unexpected AI error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}
