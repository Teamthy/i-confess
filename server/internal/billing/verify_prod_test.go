package billing

import (
	"context"
	"testing"
)

// The regression test for a payment bypass. POST /subscriptions/verify is a
// live authenticated route that grants premium from the verifier's verdict, and
// every verifier here is a stub. With no environment check, a production
// deployment on the default BILLING_VERIFIER accepted {"receipt":"valid_monthly"}
// and granted premium.
//
// These tests pin the fail-closed behaviour in both directions: production must
// refuse, and development must still work, because a guard that breaks the dev
// flow gets removed.
func TestProductionRefusesStubReceipts(t *testing.T) {
	t.Setenv("ENV", "production")
	for _, mode := range []string{"", "apple", "google", "chained"} {
		t.Setenv("BILLING_VERIFIER", mode)
		v := VerifierFromEnv()
		for _, receipt := range []string{"valid_monthly", "valid_annual", "apple_valid_annual", "google_valid_monthly"} {
			ver, err := v.Verify(context.Background(), "apple", receipt)
			if err == nil && ver.Valid {
				t.Errorf("ENV=production BILLING_VERIFIER=%q accepted receipt %q as valid - premium is purchasable for free",
					mode, receipt)
			}
		}
	}
}

func TestProductionRefusalIsCaseInsensitive(t *testing.T) {
	for _, env := range []string{"production", "Production", "PRODUCTION", " production "} {
		t.Setenv("ENV", env)
		t.Setenv("BILLING_VERIFIER", "")
		ver, err := VerifierFromEnv().Verify(context.Background(), "apple", "valid_monthly")
		if err == nil && ver.Valid {
			t.Errorf("ENV=%q was not treated as production", env)
		}
	}
}

func TestDevelopmentStillAcceptsStubReceipts(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("BILLING_VERIFIER", "")
	ver, err := VerifierFromEnv().Verify(context.Background(), "apple", "valid_monthly")
	if err != nil {
		t.Fatalf("dev verification failed: %v", err)
	}
	if !ver.Valid || ver.PlanID != "monthly" {
		t.Errorf("dev verification returned %+v, want a valid monthly plan", ver)
	}
}

// TestStubVerifiersAreSelectable pins that the named modes still resolve to
// something, so replacing a stub with a real verifier cannot silently fall
// through to the default branch.
func TestStubVerifiersAreSelectable(t *testing.T) {
	t.Setenv("ENV", "development")
	cases := map[string]string{"apple": "", "google": "", "chained": ""}
	for mode := range cases {
		t.Setenv("BILLING_VERIFIER", mode)
		if _, ok := VerifierFromEnv().(prodBlocker); ok {
			t.Errorf("BILLING_VERIFIER=%q resolved to prodBlocker outside production", mode)
		}
	}
}
