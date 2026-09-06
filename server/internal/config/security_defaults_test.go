package config

import (
	"strings"
	"testing"
)

// The published default for JWT_SECRET. If this ever reaches a non-development
// process, anyone who has read this repository can forge a session token for
// any user, including an administrator.
const publishedJWTSecret = "dev-only-change-me"

// TestUnsafeSecretsRefusedOutsideDevelopment is the regression test for a gate
// that was inverted the wrong way.
//
// Validate used to begin with "if !c.IsProduction() { return nil }", so every
// check ran only when ENV was exactly "production". Verified against the running
// code, these all booted with the published default secret:
//
//	ENV unset        -> Env "development"
//	ENV=staging      -> Env "staging"
//	ENV=prod         -> Env "prod"
//	ENV=Production   -> Env "Production"
//
// Forgetting or misspelling one environment variable is not an exotic mistake,
// and the consequence is total account takeover. The gate is now an allowlist:
// unsafe defaults are permitted only in development and test, and an
// unrecognised ENV is rejected rather than treated as permissive.
func TestUnsafeSecretsRefusedOutsideDevelopment(t *testing.T) {
	cases := []struct {
		env       string
		wantError bool
		why       string
	}{
		{"development", false, "local development is the one place a default secret is acceptable"},
		{"test", false, "the suite needs to boot without secrets"},
		{"staging", true, "staging is reachable and holds real data"},
		{"production", true, "obviously"},
		{"Production", true, "case must not change the answer"},
		{"PRODUCTION", true, "case must not change the answer"},
		{"prod", true, "a natural abbreviation must not silently mean 'no checks'"},
		{"prod ", true, "trailing whitespace must not smuggle past the allowlist"},
		{"qa", true, "unrecognised means refuse, not permit"},
		{"", false, "unset falls back to development, which is the documented default"},
	}

	for _, tc := range cases {
		t.Run("ENV="+tc.env, func(t *testing.T) {
			t.Setenv("ENV", tc.env)
			t.Setenv("JWT_SECRET", "")
			t.Setenv("AUDIO_SIGN_SECRET", "")

			c := Load()
			err := c.Validate()

			if tc.wantError && err == nil {
				t.Fatalf("ENV=%q booted with JWT_SECRET=%q and no error - %s",
					tc.env, c.JWTSecret, tc.why)
			}
			if !tc.wantError && err != nil {
				t.Fatalf("ENV=%q rejected, but it should be allowed: %v", tc.env, err)
			}
			// Note: c.JWTSecret still holds the default here. That is not the
			// vulnerability - Load() populates the struct and Validate() then
			// refuses to boot, and cmd/server/main.go aborts on that error. The
			// assertion that matters is the one above: the error must exist.
			_ = c
		})
	}
}

// TestEnvIsNormalised covers the specific failure where "Production" and
// "production" took different paths through a safety check.
func TestEnvIsNormalised(t *testing.T) {
	for _, in := range []string{"production", "Production", "PRODUCTION", "pRoDuCtIoN"} {
		t.Setenv("ENV", in)
		if got := Load().Env; got != "production" {
			t.Errorf("ENV=%q normalised to %q, want production", in, got)
		}
	}
}

// TestUnrecognisedEnvIsRejectedRatherThanPermissive pins the direction of the
// gate. The tempting fix for a typo is to treat the unknown value as
// development, which is exactly backwards.
func TestUnrecognisedEnvIsRejectedRatherThanPermissive(t *testing.T) {
	t.Setenv("ENV", "productionn") // one character off
	t.Setenv("JWT_SECRET", "a-genuinely-random-secret-value")
	t.Setenv("AUDIO_SIGN_SECRET", "another-genuinely-random-value")

	err := Load().Validate()
	if err == nil {
		t.Fatal("a misspelled ENV was accepted")
	}
	if !strings.Contains(err.Error(), "not a recognised environment") {
		t.Errorf("error does not explain the problem: %v", err)
	}
}

// TestTokenTTLDefaultIsShort guards the other half of this phase's finding. The
// previous default was 720h: a stolen access token stayed valid for 30 days.
func TestTokenTTLDefaultIsShort(t *testing.T) {
	t.Setenv("TOKEN_TTL", "")
	ttl := Load().TokenTTL
	if ttl != "24h" {
		t.Errorf("TOKEN_TTL default = %q, want 24h", ttl)
	}
}

// TestValidProductionConfigPasses is the counterpart: hardening that makes a
// correct deployment fail to boot gets reverted within a week.
func TestValidProductionConfigPasses(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("JWT_SECRET", "a-genuinely-random-secret-value")
	t.Setenv("AUDIO_SIGN_SECRET", "another-genuinely-random-value")
	t.Setenv("EMAIL_PROVIDER", "postmark")
	t.Setenv("POSTMARK_TOKEN", "pm-token")
	t.Setenv("PUBLIC_BASE_URL", "https://iconfess.app")
	t.Setenv("REDIS_ADDR", "redis:6379")

	if err := Load().Validate(); err != nil {
		t.Errorf("a correctly configured production deployment was rejected: %v", err)
	}
}

// TestDevelopmentDefaultIsThePublishedSecret is what makes the gate above
// matter. The development default is a literal in this repository, so anyone
// who has read the source can sign a valid token with it. If the default ever
// changes, this test fails and whoever changed it has to confirm the tests
// still describe reality.
func TestDevelopmentDefaultIsThePublishedSecret(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("JWT_SECRET", "")

	if got := Load().JWTSecret; got != publishedJWTSecret {
		t.Errorf("development default JWT_SECRET = %q, want %q - update the tests above if this changed intentionally",
			got, publishedJWTSecret)
	}
}
