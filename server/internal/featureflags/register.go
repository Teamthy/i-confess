package featureflags

import "context"

// Known feature flag keys, matching the seeds in migration 0001_baseline.sql.
//
// These are the canonical names used throughout the codebase. A feature flag
// that is not in this map is treated as disabled by default.
const (
	FeatureNewPlayer        = "new_player"
	FeatureAISessionBuilder = "ai_session_builder"
	FeatureCommunity        = "community"
	FeatureOfflineDownloads = "offline_downloads"
	FeatureNewHome          = "new_home"
	FeaturePremiumVoices    = "premium_voices"
)

// Feature is a single feature flag row from the database.
type Feature struct {
	Key           string
	Enabled       bool
	RolloutPercent int
	Platforms     string
	MinVersion    string
}

// Register is the feature-status register: the single source of truth for
// whether a feature is enabled. It is loaded once from the database at
// startup and consulted on every feature-gated request.
//
// A feature not present in the register is disabled by default: the absence
// of a row means "do not release this to anyone".
type Register interface {
	// IsEnabled reports whether the named feature is enabled for the given
	// request context. It accounts for the enabled flag, rollout percentage
	// and platform allowlist.
	IsEnabled(ctx context.Context, key string) (bool, error)

	// EnabledKeys returns the set of feature keys that are fully enabled
	// (enabled=1, rollout=100 or no rollout constraint). Used for admin
	// views and startup diagnostics.
	EnabledKeys() map[string]bool
}

// NoopRegister is a register where every feature is disabled. Used as a safe
// default when the feature flag service is not yet wired.
type NoopRegister struct{}

func (NoopRegister) IsEnabled(ctx context.Context, key string) (bool, error) {
	return false, nil
}

func (NoopRegister) EnabledKeys() map[string]bool {
	return map[string]bool{}
}

// compile-time check.
var _ Register = NoopRegister{}
