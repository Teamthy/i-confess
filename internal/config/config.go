package config

import "os"

// Config holds runtime configuration sourced from environment variables.
type Config struct {
	Port       string
	DBPath     string
	JWTSecret  string
	TokenTTL   string
	Env        string
}

func Load() Config {
	return Config{
		Port:      getenv("PORT", "8080"),
		DBPath:    getenv("DB_PATH", "data/iconfess.db"),
		JWTSecret: getenv("JWT_SECRET", "dev-only-change-me"),
		TokenTTL:  getenv("TOKEN_TTL", "720h"),
		Env:       getenv("ENV", "development"),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
