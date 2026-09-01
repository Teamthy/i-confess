package config

import (
	"os"
	"strconv"
)

// Config holds runtime configuration sourced from environment variables.
type Config struct {
	Port         string
	DBPath       string
	JWTSecret    string
	TokenTTL     string
	Env          string
	MediaDir     string
	PostgresDSN  string
	RedisAddr    string
	QueueWorkers int
}

func Load() Config {
	return Config{
		Port:         getenv("PORT", "8080"),
		DBPath:       getenv("DB_PATH", "data/iconfess.db"),
		JWTSecret:    getenv("JWT_SECRET", "dev-only-change-me"),
		TokenTTL:     getenv("TOKEN_TTL", "720h"),
		Env:          getenv("ENV", "development"),
		MediaDir:     getenv("MEDIA_DIR", "data/media"),
		PostgresDSN:  getenv("POSTGRES_DSN", ""),
		RedisAddr:    getenv("REDIS_ADDR", ""),
		QueueWorkers: intEnv("QUEUE_WORKERS", 4),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func intEnv(k string, def int) int {
	v := getenv(k, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	return n
}
