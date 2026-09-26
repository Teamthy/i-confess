package billing

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
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

func TestDevelopmentStubDoesNotInventAThirdProvider(t *testing.T) {
	ver, err := (NoopVerifier{}).Verify(context.Background(), "stripe", "valid_monthly")
	if err == nil && ver.Valid {
		t.Fatal("development verifier accepted a provider it does not verify")
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

func TestStagingRefusesStubReceipts(t *testing.T) {
	t.Setenv("ENV", "staging")
	t.Setenv("BILLING_VERIFIER", "")
	ver, err := VerifierFromEnv().Verify(context.Background(), "apple", "valid_monthly")
	if err == nil && ver.Valid {
		t.Errorf("ENV=staging accepted stub receipt as valid")
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

// ---------------------------------------------------------------------------
// Verifier selection (IC-003)
// ---------------------------------------------------------------------------
//
// The tests above pin that production refuses stub receipts. These pin the
// other half: that production is served by a verifier which genuinely checks
// the receipt, that a half-configured deployment fails closed rather than
// falling back to the development stub, and that a typo in BILLING_VERIFIER
// cannot silently disable verification.

// setAppleEnv configures a complete, usable App Store verification setup.
func setAppleEnv(t *testing.T) {
	t.Helper()
	t.Setenv(EnvAppleBundleID, "app.iconfess")
	t.Setenv(EnvAppleEnvironment, AppleEnvironmentProduction)
	t.Setenv(EnvAppleMonthly, "app.iconfess.premium.monthly")
	t.Setenv(EnvAppleAnnual, "app.iconfess.premium.annual")
	t.Setenv(EnvApplePlans, "")
	t.Setenv(EnvAppleRootsPath, "")
}

func clearBillingEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		EnvAppleBundleID, EnvAppleEnvironment, EnvAppleRootsPath, EnvApplePlans,
		EnvAppleMonthly, EnvAppleAnnual, EnvGooglePackage, EnvGoogleKeyPath,
		EnvGooglePlans, EnvGoogleMonthly, EnvGoogleAnnual, EnvFCMServiceAccount,
	} {
		t.Setenv(k, "")
	}
}

// The forged JWS from the audit, sent to a production deployment.
func forgedAppleReceipt() string {
	header := `{"alg":"ES256"}`
	payload := `{"transactionId":"1","productId":"app.iconfess.premium.annual","expiresDate":9999999999999,"bundleId":"app.iconfess","environment":"Production"}`
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(header)) + "." + enc([]byte(payload)) + ".not-a-signature"
}

func TestProductionVerifiesWithTheRealAppStoreVerifier(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv(EnvBillingVerifier, "apple")
	setAppleEnv(t)

	v := VerifierFromEnv()
	if _, ok := v.(*AppleVerifier); !ok {
		t.Fatalf("ENV=production BILLING_VERIFIER=apple resolved to %s, want the real App Store verifier",
			DescribeVerifier(v))
	}
	if !strings.Contains(DescribeVerifier(v), "Apple App Store") {
		t.Errorf("DescribeVerifier = %q", DescribeVerifier(v))
	}

	// The exact forgery that the previous implementation granted premium for.
	if got, err := v.Verify(context.Background(), "apple", forgedAppleReceipt()); err == nil && got.Valid {
		t.Fatal("production accepted an unsigned forged receipt")
	}
}

// A deployment that names the App Store but is missing its bundle id must fail
// closed. Falling back to the stub here would mean a misconfiguration converts
// production into free premium.
func TestProductionDoesNotFallBackToTheStubWhenTheStoreIsMisconfigured(t *testing.T) {
	cases := map[string]func(t *testing.T){
		"no bundle id": func(t *testing.T) {
			setAppleEnv(t)
			t.Setenv(EnvAppleBundleID, "")
		},
		"no products": func(t *testing.T) {
			setAppleEnv(t)
			t.Setenv(EnvAppleMonthly, "")
			t.Setenv(EnvAppleAnnual, "")
		},
		"no store environment": func(t *testing.T) {
			setAppleEnv(t)
			t.Setenv(EnvAppleEnvironment, "")
		},
		"unknown verifier name": func(t *testing.T) {
			setAppleEnv(t)
			t.Setenv(EnvBillingVerifier, "appl")
		},
		"play without a service account": func(t *testing.T) {
			t.Setenv(EnvBillingVerifier, "google")
			t.Setenv(EnvGooglePackage, "app.iconfess")
			t.Setenv(EnvGoogleMonthly, "premium_monthly")
			t.Setenv(EnvGoogleKeyPath, "/nonexistent/service-account.json")
		},
		"play without a package": func(t *testing.T) {
			t.Setenv(EnvBillingVerifier, "google")
			t.Setenv(EnvGooglePackage, "")
			t.Setenv(EnvGoogleMonthly, "premium_monthly")
		},
	}
	for name, configure := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("ENV", "production")
			t.Setenv(EnvBillingVerifier, "apple")
			clearBillingEnv(t)
			configure(t)

			v := VerifierFromEnv()
			if _, ok := v.(NoopVerifier); ok {
				t.Fatal("a production deployment resolved to the development stub verifier")
			}
			for _, receipt := range []string{"valid_monthly", "valid_annual", forgedAppleReceipt()} {
				if got, err := v.Verify(context.Background(), "apple", receipt); err == nil && got.Valid {
					t.Errorf("accepted %q with a misconfigured deployment", receipt)
				}
			}
			if desc := DescribeVerifier(v); !strings.Contains(desc, "fail-closed") {
				t.Errorf("DescribeVerifier = %q, want a fail-closed description", desc)
			}
		})
	}
}

// Staging is reachable and holds real data, so it must be treated like
// production: no stubs.
func TestStagingIsNotTreatedAsDevelopment(t *testing.T) {
	clearBillingEnv(t)
	t.Setenv("ENV", "staging")
	t.Setenv(EnvBillingVerifier, "")
	t.Setenv(EnvAppleBundleID, "")
	if v := VerifierFromEnv(); !strings.Contains(DescribeVerifier(v), "fail-closed") {
		t.Fatalf("staging resolved to %q", DescribeVerifier(v))
	}
}

// Development falls back to the stub so the mobile purchase flow can be
// exercised without store credentials - and says so, because a developer who
// believes they are testing real verification will not notice a forged receipt
// being accepted.
func TestDevelopmentFallsBackToTheStubVisibly(t *testing.T) {
	clearBillingEnv(t)
	t.Setenv("ENV", "development")
	t.Setenv(EnvBillingVerifier, "apple")

	v := VerifierFromEnv()
	if _, ok := v.(NoopVerifier); !ok {
		t.Fatalf("development with an unconfigured store resolved to %s", DescribeVerifier(v))
	}
	if !strings.Contains(DescribeVerifier(v), "development stub") {
		t.Errorf("DescribeVerifier = %q, want it to name the stub", DescribeVerifier(v))
	}
}

// The verifier is rebuilt per configuration, never per request. Rebuilding a
// Google verifier per request would mint a fresh access token every time,
// because the token cache lives inside the token source.
func TestVerifierIsReusedAcrossRequests(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv(EnvBillingVerifier, "apple")
	setAppleEnv(t)

	first := VerifierFromEnv()
	second := VerifierFromEnv()
	if first != second {
		t.Fatal("a second identical call built a second verifier - the Google token cache would be defeated")
	}

	// A different configuration must produce a different verifier rather than
	// reusing one built for something else.
	t.Setenv(EnvAppleBundleID, "app.iconfess.other")
	third := VerifierFromEnv()
	if third == first {
		t.Fatal("a changed bundle id reused the previous verifier")
	}
}

func TestProductPlanMapReadsBothSpellings(t *testing.T) {
	t.Setenv(EnvAppleMonthly, "sku.monthly")
	t.Setenv(EnvAppleAnnual, "sku.annual")
	t.Setenv(EnvApplePlans, "sku.promo=monthly, sku.family=annual, malformed, =monthly, sku.empty=")
	got := productPlanMap(EnvApplePlans, EnvAppleMonthly, EnvAppleAnnual)

	want := map[string]string{
		"sku.monthly": "monthly",
		"sku.annual":  "annual",
		"sku.promo":   "monthly",
		"sku.family":  "annual",
	}
	if len(got) != len(want) {
		t.Fatalf("parsed %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestMissingProductMappingsNamesWhatToSet(t *testing.T) {
	clearBillingEnv(t)
	t.Setenv(EnvBillingVerifier, "chained")
	missing := strings.Join(MissingProductMappings(), " ")
	for _, want := range []string{EnvAppleBundleID, EnvAppleMonthly, EnvGooglePackage, EnvGoogleKeyPath} {
		if !strings.Contains(missing, want) {
			t.Errorf("MissingProductMappings() = %v, want it to mention %s", missing, want)
		}
	}
}

// The boot gate: a deployment that cannot verify a receipt must fail the
// deploy, not the first purchase.
func TestRequireVerificationRefusesToBootUnconfigured(t *testing.T) {
	clearBillingEnv(t)

	// Development is allowed to run the stub, and says so.
	t.Setenv("ENV", "development")
	t.Setenv(EnvBillingVerifier, "")
	if err := RequireVerification(); err != nil {
		t.Errorf("development with no verifier: %v, want nil", err)
	}

	// Everything else is refused, because none of these can verify anything:
	// no verifier named, a name that is not a verifier, and a named provider
	// with no products to map.
	cases := []struct {
		name string
		mode string
	}{
		{"no verifier named", ""},
		{"a typo", "appel"},
		{"apple with no products", "apple"},
		{"google with no key", "google"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearBillingEnv(t)
			t.Setenv("ENV", "production")
			t.Setenv(EnvBillingVerifier, tc.mode)
			err := RequireVerification()
			if err == nil {
				t.Fatal("production booted with no working verifier - every paying customer would be told their receipt is invalid")
			}
			// The message has to name something an operator can set. A boot
			// refusal that says only "billing failed" costs more time than the
			// misconfiguration it caught.
			msg := err.Error()
			named := false
			for _, want := range []string{EnvBillingVerifier, EnvAppleBundleID, EnvAppleEnvironment, EnvAppleMonthly, EnvGooglePackage, EnvGoogleKeyPath, "not a known verifier"} {
				if strings.Contains(msg, want) {
					named = true
					break
				}
			}
			if !named {
				t.Errorf("error = %q, want it to name the configuration to fix", msg)
			}
		})
	}

	// Properly configured, it boots.
	clearBillingEnv(t)
	t.Setenv("ENV", "production")
	t.Setenv(EnvBillingVerifier, "apple")
	setAppleEnv(t)
	if err := RequireVerification(); err != nil {
		t.Errorf("configured production: %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// Payments-disabled staging mode
// ---------------------------------------------------------------------------
//
// Staging holds real data but may not have Apple or Google billing products
// yet. BILLING_VERIFIER=disabled with ENV=staging must boot and answer the
// verify route, fail closed - no receipt may ever become a paid entitlement -
// and the mode must be unusable everywhere else, production above all.

func TestStagingDisabledModeBootsButGrantsNothing(t *testing.T) {
	clearBillingEnv(t)
	t.Setenv("ENV", "staging")
	t.Setenv(EnvBillingVerifier, "disabled")

	v := VerifierFromEnv()
	if _, ok := v.(paymentsDisabled); !ok {
		t.Fatalf("ENV=staging BILLING_VERIFIER=disabled resolved to %s, want the payments-disabled verifier",
			DescribeVerifier(v))
	}
	if err := RequireVerification(); err != nil {
		t.Fatalf("staging with payments disabled must boot, got: %v", err)
	}
	if desc := DescribeVerifier(v); !strings.Contains(desc, "payments disabled") {
		t.Errorf("DescribeVerifier = %q, want it to name the disabled mode", desc)
	}

	// Fail closed: the development stub's magic receipts and the audit forgery
	// alike must produce an error that wraps ErrPaymentsDisabled and never a
	// paid entitlement.
	for _, receipt := range []string{"valid_monthly", "valid_annual", forgedAppleReceipt()} {
		ver, err := v.Verify(context.Background(), "apple", receipt)
		if !errors.Is(err, ErrPaymentsDisabled) {
			t.Errorf("Verify(%q) error = %v, want ErrPaymentsDisabled", receipt, err)
		}
		if ver.Valid || ver.PlanID != "" {
			t.Errorf("Verify(%q) granted an entitlement in payments-disabled staging: %+v", receipt, ver)
		}
	}
}

func TestProductionRefusesDisabledModeAtBoot(t *testing.T) {
	clearBillingEnv(t)
	t.Setenv("ENV", "production")
	t.Setenv(EnvBillingVerifier, "disabled")

	if _, ok := VerifierFromEnv().(paymentsDisabled); ok {
		t.Fatal("production resolved to the payments-disabled verifier")
	}
	if err := RequireVerification(); err == nil {
		t.Fatal("production booted with BILLING_VERIFIER=disabled - a staging-only mode")
	}
}

func TestDisabledModeIsStagingOnly(t *testing.T) {
	clearBillingEnv(t)
	for _, env := range []string{"development", "test", "production"} {
		t.Setenv("ENV", env)
		t.Setenv(EnvBillingVerifier, "disabled")
		if _, ok := VerifierFromEnv().(paymentsDisabled); ok {
			t.Errorf("ENV=%s resolved to the payments-disabled verifier; the mode is staging-only", env)
		}
		if err := RequireVerification(); err == nil {
			t.Errorf("ENV=%s booted with BILLING_VERIFIER=disabled; only staging may", env)
		}
	}
}
