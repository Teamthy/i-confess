package featureflags

import (
	"context"
	"testing"

	"github.com/Teamthy/i-confess/internal/db"
)

func TestDBStoreLoadsSeedFlags(t *testing.T) {
	dsn := "host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable"
	dbt, err := db.Open(dsn)
	if err != nil {
		t.Skipf("cannot open test database: %v", err)
	}

	ctx := context.Background()
	s := NewDBStore(ctx, dbt)

	// The migration seeds 6 flags; offline_downloads and premium_voices are
	// enabled at 100%.
	enabled, err := s.IsEnabled(ctx, FeatureOfflineDownloads)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("offline_downloads should be enabled (seeded at 100%)")
	}

	enabled, err = s.IsEnabled(ctx, FeaturePremiumVoices)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("premium_voices should be enabled (seeded at 100%)")
	}

	// new_player, ai_session_builder, community, new_home are seeded disabled.
	enabled, err = s.IsEnabled(ctx, FeatureNewPlayer)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("new_player should be disabled (seeded at 0%)")
	}

	// Unknown features are disabled by default.
	enabled, err = s.IsEnabled(ctx, "does_not_exist")
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("unknown feature should be disabled")
	}

	// EnabledKeys should include the two enabled flags.
	keys := s.EnabledKeys()
	if !keys[FeatureOfflineDownloads] || !keys[FeaturePremiumVoices] {
		t.Fatalf("EnabledKeys missing expected flags: %v", keys)
	}
	if keys[FeatureNewPlayer] {
		t.Fatal("new_player should not be in EnabledKeys")
	}
}
