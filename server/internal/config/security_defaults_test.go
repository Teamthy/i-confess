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
	// Production audio must live in object storage (§6). STORAGE_PROVIDER
	// defaults to "local", which Validate now refuses outside development.
	t.Setenv("STORAGE_PROVIDER", "s3")
	t.Setenv("S3_BUCKET", "iconfess-audio")
	t.Setenv("S3_REGION", "eu-west-1")

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

// productionBase sets every variable a valid production deployment needs, so
// the storage tests below isolate the one variable each is about.
func productionBase(t *testing.T) {
	t.Helper()
	t.Setenv("ENV", "production")
	t.Setenv("JWT_SECRET", "a-genuinely-random-secret-value")
	t.Setenv("AUDIO_SIGN_SECRET", "another-genuinely-random-value")
	t.Setenv("EMAIL_PROVIDER", "postmark")
	t.Setenv("POSTMARK_TOKEN", "pm-token")
	t.Setenv("PUBLIC_BASE_URL", "https://iconfess.app")
	t.Setenv("REDIS_ADDR", "redis:6379")
}

// TestProductionRefusesLocalStorage covers the PHASE 13 boot guard.
//
// This is the failure the guard exists to prevent: STORAGE_PROVIDER defaulted
// to "local" and main.go hard-coded it, so a production deployment booted
// healthy and served audio from the container filesystem indefinitely. Nothing
// on that path complained. But the bytes die on restart and are invisible to
// every other replica, so audio generated on one instance is unreachable from
// the next — precisely what §6 forbids.
func TestProductionRefusesLocalStorage(t *testing.T) {
	productionBase(t)
	t.Setenv("STORAGE_PROVIDER", "local")

	err := Load().Validate()
	if err == nil {
		t.Fatal("production booted with STORAGE_PROVIDER=local")
	}
	if !strings.Contains(err.Error(), "local") {
		t.Errorf("error should name the rejected provider, got: %v", err)
	}
}

// TestProductionRejectsHalfConfiguredS3 covers the other silent failure: naming
// S3 without a bucket, which would otherwise fail at the first upload rather
// than at boot.
func TestProductionRejectsHalfConfiguredS3(t *testing.T) {
	productionBase(t)
	t.Setenv("STORAGE_PROVIDER", "s3")
	t.Setenv("S3_BUCKET", "")

	if err := Load().Validate(); err == nil {
		t.Fatal("production accepted STORAGE_PROVIDER=s3 with no S3_BUCKET")
	}
}

// TestUnimplementedProvidersAreRefusedAtBoot covers gcs and azure, which are
// named in the schema but have no implementation. Their previous stub methods
// satisfied the interface, so the provider constructed successfully and the
// failure surfaced at the first upload instead.
func TestUnimplementedProvidersAreRefusedAtBoot(t *testing.T) {
	for _, provider := range []string{"gcs", "azure"} {
		productionBase(t)
		t.Setenv("STORAGE_PROVIDER", provider)

		if err := Load().Validate(); err == nil {
			t.Errorf("production accepted STORAGE_PROVIDER=%s", provider)
		}
	}
}

// TestDevelopmentMayUseLocalStorage keeps the guard from breaking local work.
func TestDevelopmentMayUseLocalStorage(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("STORAGE_PROVIDER", "local")

	if err := Load().Validate(); err != nil {
		t.Errorf("development rejected local storage: %v", err)
	}
}
