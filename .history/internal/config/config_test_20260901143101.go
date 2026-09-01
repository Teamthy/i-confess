package config

import (
	"os"
	"testing"
)

func TestLoadReadsEnvironment(t *testing.T) {
	old := map[string]*string{
		"PORT":      ptr(os.Getenv("PORT")),
		"DB_PATH":   ptr(os.Getenv("DB_PATH")),
		"JWT_SECRET": ptr(os.Getenv("JWT_SECRET")),
		"TOKEN_TTL": ptr(os.Getenv("TOKEN_TTL")),
		"ENV":       ptr(os.Getenv("ENV")),
		"MEDIA_DIR": ptr(os.Getenv("MEDIA_DIR")),
	}
	defer func() {
		for k, v := range old {
			if v == nil || *v == "" {
				_ = os.Unsetenv(k)
				continue
			}
			_ = os.Setenv(k, *v)
		}
	}()

	_ = os.Setenv("PORT", "9090")
	_ = os.Setenv("DB_PATH", "tmp/test.db")
	_ = os.Setenv("JWT_SECRET", "super-secret")
	_ = os.Setenv("TOKEN_TTL", "24h")
	_ = os.Setenv("ENV", "production")
	_ = os.Setenv("MEDIA_DIR", "/tmp/media")

	cfg := Load()
	if cfg.Port != "9090" { t.Fatalf("Port = %q; want 9090", cfg.Port) }
	if cfg.DBPath != "tmp/test.db" { t.Fatalf("DBPath = %q; want tmp/test.db", cfg.DBPath) }
	if cfg.JWTSecret != "super-secret" { t.Fatalf("JWTSecret = %q; want super-secret", cfg.JWTSecret) }
	if cfg.TokenTTL != "24h" { t.Fatalf("TokenTTL = %q; want 24h", cfg.TokenTTL) }
	if cfg.Env != "production" { t.Fatalf("Env = %q; want production", cfg.Env) }
	if cfg.MediaDir != "/tmp/media" { t.Fatalf("MediaDir = %q; want /tmp/media", cfg.MediaDir) }
}

func ptr(s string) *string { return &s }
