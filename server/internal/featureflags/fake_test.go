package featureflags

import (
	"context"
	"testing"
)

func TestFakeRegisterDisabledByDefault(t *testing.T) {
	f := NewFakeRegister()

	enabled, err := f.IsEnabled(context.Background(), FeatureNewPlayer)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("new_player should be disabled by default")
	}
}

func TestFakeRegisterEnableDisable(t *testing.T) {
	f := NewFakeRegister(FeatureCommunity)

	enabled, err := f.IsEnabled(context.Background(), FeatureCommunity)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("community should be enabled")
	}

	f.Disable(FeatureCommunity)
	enabled, err = f.IsEnabled(context.Background(), FeatureCommunity)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("community should be disabled after Disable")
	}

	f.Enable(FeatureCommunity)
	enabled, err = f.IsEnabled(context.Background(), FeatureCommunity)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("community should be re-enabled")
	}
}

func TestFakeRegisterEnabledKeys(t *testing.T) {
	f := NewFakeRegister(FeatureNewPlayer, FeaturePremiumVoices)

	keys := f.EnabledKeys()
	if !keys[FeatureNewPlayer] {
		t.Fatal("new_player missing from EnabledKeys")
	}
	if !keys[FeaturePremiumVoices] {
		t.Fatal("premium_voices missing from EnabledKeys")
	}
	if keys[FeatureCommunity] {
		t.Fatal("community should not be in EnabledKeys")
	}
}

func TestFakeRegisterUnknownFeature(t *testing.T) {
	f := NewFakeRegister()

	enabled, err := f.IsEnabled(context.Background(), "nonexistent_feature")
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("unknown feature should be disabled")
	}
}
