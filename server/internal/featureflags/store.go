package featureflags

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Teamthy/i-confess/internal/db"
)

// DBStore loads feature flags from the database and caches them in memory.
// It implements Register.
//
// Flags are loaded once at construction: the register is a snapshot, not a
// live subscription. This is deliberate — a deployment that toggles a flag
// should restart its replicas so every instance sees the same state, rather
// than having flags flip mid-request across a rolling rollout.
//
// Construction never fails: if the database is unavailable or the table is
// missing, the store holds zero flags and every feature reports disabled
// (fail-closed). Callers that need to distinguish "not loaded" from "all
// disabled" should use EnabledKeys() at startup.
type DBStore struct {
	dbt   *db.DB
	flags map[string]Feature
}

// NewDBStore wraps a database handle and loads every non-deleted feature
// flag into memory. If the query fails, the store holds zero flags (fail
// closed) rather than returning an error, so NewHandler can construct it
// without propagating a database error at startup.
func NewDBStore(ctx context.Context, dbt *db.DB) *DBStore {
	s := &DBStore{dbt: dbt, flags: map[string]Feature{}}
	rows, err := dbt.QueryContext(ctx,
		`SELECT key, enabled, rollout_percent, platforms, min_version
		 FROM feature_flags
		 WHERE deleted_at IS NULL`)
	if err != nil {
		// Fail closed: no flags loaded, every feature reports disabled.
		return s
	}
	defer rows.Close()

	for rows.Next() {
		var f Feature
		var enabledInt int
		if err := rows.Scan(&f.Key, &enabledInt, &f.RolloutPercent, &f.Platforms, &f.MinVersion); err != nil {
			continue
		}
		f.Enabled = enabledInt == 1
		s.flags[f.Key] = f
	}
	_ = rows.Err()
	return s
}

// IsEnabled reports whether the named feature is enabled.
//
// A feature is enabled when:
//   - a row exists with enabled = 1, and
//   - rollout_percent is 100 (fully on; 0 means fully off),
//   - platforms is empty (all platforms).
//
// Gradual rollout (1-99%) and platform allowlists are accepted as data but
// treated as disabled until the rollout engine is implemented.
func (s *DBStore) IsEnabled(ctx context.Context, key string) (bool, error) {
	f, ok := s.flags[key]
	if !ok {
		return false, nil
	}
	if !f.Enabled {
		return false, nil
	}
	if f.RolloutPercent == 0 || f.RolloutPercent < 100 {
		return false, nil
	}
	if f.Platforms != "" {
		return false, nil
	}
	return true, nil
}

// EnabledKeys returns the set of feature keys that are fully enabled.
func (s *DBStore) EnabledKeys() map[string]bool {
	out := map[string]bool{}
	for key, f := range s.flags {
		if f.Enabled && f.RolloutPercent >= 100 && f.Platforms == "" {
			out[key] = true
		}
	}
	return out
}

// All returns a copy of every feature flag in the register.
func (s *DBStore) All() map[string]Feature {
	out := make(map[string]Feature, len(s.flags))
	for k, v := range s.flags {
		out[k] = v
	}
	return out
}

// compile-time check that DBStore implements Register.
var _ Register = (*DBStore)(nil)
