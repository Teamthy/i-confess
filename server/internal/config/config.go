package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// Config holds runtime configuration sourced from environment variables.
type Config struct {
	Port string
	// DatabaseURL is a lib/pq connection string. PostgreSQL is the only
	// supported dialect; there is no fallback.
	DatabaseURL  string
	JWTSecret    string
	TokenTTL     string
	Env          string
	MediaDir     string
	PostgresDSN  string
	RedisAddr    string
	QueueWorkers int

	// MediaBaseURL is the URL prefix signed audio links point at. Set this to
	// the CDN hostname in production.
	MediaBaseURL string
	// AudioSignSecret signs audio URLs. Distinct from JWTSecret so either can
	// be rotated without invalidating the other. MUST be set in production.
	AudioSignSecret string
	// ElevenLabsAPIKey enables synthesis. Server-side only; never sent to a
	// client (PRD S14).
	ElevenLabsAPIKey string

	// --- Email (PRD S57, S58) ---
	// EmailProvider selects the transactional sender: "postmark" or "log".
	EmailProvider string
	// PostmarkToken authenticates with Postmark. Server-side only.
	PostmarkToken string
	// EmailFrom is the envelope sender for authentication mail.
	EmailFrom string
	// EmailSupport is offered as a reply path in security mail.
	EmailSupport string
	// PublicBaseURL is the origin used to build verification and reset links.
	PublicBaseURL string
	// AppName appears in email subjects and bodies.
	AppName string

	// RedisPassword authenticates with Redis when required.
	RedisPassword string

	// --- Social sign-in (PRD S36, S37) ---
	// GoogleClientID is the audience social tokens must be issued for. Empty
	// disables Google sign-in rather than accepting tokens for any audience.
	GoogleClientID string
	// AppleClientID is the Services ID / bundle id for Sign in with Apple.
	AppleClientID string

	// --- Push notifications (PRD S47) ---
	// APNsKeyPath is the .p8 signing key. Token auth rather than certificates:
	// one key serves every environment and does not expire annually.
	APNsKeyPath    string
	APNsKeyID      string
	APNsTeamID     string
	APNsTopic      string
	APNsProduction bool
	// FCMServiceAccountPath is a Google service-account JSON key. The server
	// mints and refreshes its own OAuth2 tokens from it; the project id is
	// read from the key so it cannot be misconfigured separately.
	FCMServiceAccountPath string
}

func Load() Config {
	return Config{
		Port:         getenv("PORT", "8080"),
		DatabaseURL:  getenv("DATABASE_URL", "host=127.0.0.1 port=5432 user=iconfess password=iconfess dbname=iconfess sslmode=disable"),
		JWTSecret:    getenv("JWT_SECRET", "dev-only-change-me"),
		TokenTTL:     getenv("TOKEN_TTL", "720h"),
		Env:          getenv("ENV", "development"),
		MediaDir:     getenv("MEDIA_DIR", "data/media"),
		PostgresDSN:  getenv("POSTGRES_DSN", ""),
		RedisAddr:    getenv("REDIS_ADDR", ""),
		QueueWorkers: intEnv("QUEUE_WORKERS", 4),

		MediaBaseURL:     getenv("MEDIA_BASE_URL", "/media"),
		AudioSignSecret:  getenv("AUDIO_SIGN_SECRET", "dev-only-audio-secret"),
		ElevenLabsAPIKey: os.Getenv("ELEVENLABS_API_KEY"),

		EmailProvider: getenv("EMAIL_PROVIDER", "log"),
		PostmarkToken: os.Getenv("POSTMARK_TOKEN"),
		EmailFrom:     getenv("EMAIL_FROM", "no-reply@iconfess.local"),
		EmailSupport:  getenv("EMAIL_SUPPORT", "support@iconfess.local"),
		PublicBaseURL: getenv("PUBLIC_BASE_URL", "http://localhost:8080"),
		AppName:       getenv("APP_NAME", "i-confess"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),

		GoogleClientID: os.Getenv("GOOGLE_CLIENT_ID"),
		AppleClientID:  os.Getenv("APPLE_CLIENT_ID"),

		APNsKeyPath:           os.Getenv("APNS_KEY_PATH"),
		APNsKeyID:             os.Getenv("APNS_KEY_ID"),
		APNsTeamID:            os.Getenv("APNS_TEAM_ID"),
		APNsTopic:             getenv("APNS_TOPIC", "app.iconfess"),
		APNsProduction:        os.Getenv("APNS_PRODUCTION") == "true",
		FCMServiceAccountPath: os.Getenv("FCM_SERVICE_ACCOUNT"),
	}
}

// IsProduction reports whether unsafe defaults must be rejected at boot.
func (c Config) IsProduction() bool { return c.Env == "production" }

// Validate refuses to start production with development secrets.
//
// Failing loudly at boot is far better than silently signing audio with a
// publicly known default, which would let anyone mint valid links.
func (c Config) Validate() error {
	if !c.IsProduction() {
		return nil
	}
	if c.JWTSecret == "" || c.JWTSecret == "dev-only-change-me" {
		return errors.New("JWT_SECRET must be set to a non-default value in production")
	}
	if c.AudioSignSecret == "" || c.AudioSignSecret == "dev-only-audio-secret" {
		return errors.New("AUDIO_SIGN_SECRET must be set to a non-default value in production")
	}
	// The log sender prints verification and reset links. In production that
	// would write working credentials to the log stream, so it is refused.
	if c.EmailProvider == "log" {
		return errors.New("EMAIL_PROVIDER must not be \"log\" in production: it writes reset links to logs")
	}
	if c.EmailProvider == "postmark" && c.PostmarkToken == "" {
		return errors.New("POSTMARK_TOKEN is required when EMAIL_PROVIDER=postmark")
	}
	if strings.HasPrefix(c.PublicBaseURL, "http://") {
		return errors.New("PUBLIC_BASE_URL must be https in production: links carry one-time tokens")
	}
	// Without Redis, each replica enforces its own limits, so N replicas allow
	// N times the intended rate.
	if c.RedisAddr == "" {
		return errors.New("REDIS_ADDR is required in production so rate limits hold across replicas")
	}
	return nil
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
