package billing

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/Teamthy/i-confess/internal/googleauth"
	"github.com/Teamthy/i-confess/internal/playapi"
)

// Which verifier runs, and why it is chosen this way (IC-003).
//
// Two failures had to be fixed at once, and they pull in opposite directions:
//
//   - Non-production environments accepted `{"receipt":"valid_monthly"}` and
//     granted premium, because the stub was reachable anywhere the ENV check let
//     it through.
//   - Production could never grant premium at all, because the fail-closed
//     guard refused everything — and the "real" verifiers behind it did not
//     actually verify anything.
//
// So the rule is: a stub verifier is reachable from development and test only,
// and every other environment is served by a verifier that either genuinely
// checks the receipt with the store or refuses the receipt. There is no third
// state, and in particular no environment where a receipt is believed because
// the client said so.

// Environment variables that configure store verification. Documented in
// docs/BILLING.md and .env.example.
const (
	// EnvBillingVerifier selects the provider: apple, google, chained, or empty
	// for the development stub.
	EnvBillingVerifier = "BILLING_VERIFIER"

	// App Store.
	EnvAppleBundleID    = "APPLE_BUNDLE_ID"
	EnvAppleEnvironment = "APPLE_ENVIRONMENT"
	EnvAppleRootsPath   = "APPLE_ROOT_CA_PATH"
	EnvApplePlans       = "APPLE_PRODUCT_PLANS"
	EnvAppleMonthly     = "APPLE_PRODUCT_MONTHLY"
	EnvAppleAnnual      = "APPLE_PRODUCT_ANNUAL"

	// Play Store.
	EnvGooglePackage     = "GOOGLE_PLAY_PACKAGE_NAME"
	EnvGoogleKeyPath     = "GOOGLE_PLAY_SERVICE_ACCOUNT"
	EnvGooglePlans       = "GOOGLE_PLAY_PRODUCT_PLANS"
	EnvGoogleMonthly     = "GOOGLE_PLAY_PRODUCT_MONTHLY"
	EnvGoogleAnnual      = "GOOGLE_PLAY_PRODUCT_ANNUAL"
	EnvFCMServiceAccount = "FCM_SERVICE_ACCOUNT"
)

// description is the operator-facing explanation of why verification is off.
func (p prodBlocker) description() error {
	if p.err == nil {
		return fmt.Errorf("%w: no real store verifier is configured", ErrUnconfigured)
	}
	return p.err
}

// prodBlocker rejects every receipt unconditionally when a real store verifier
// is not configured.
type prodBlocker struct {
	// err wraps ErrUnconfigured. The sentinel matters: the handler answers 503
	// VERIFIER_UNCONFIGURED for it and 502 PROVIDER_UNAVAILABLE for anything
	// else, and telling a paying customer "the provider is unavailable" when
	// the truth is that this deployment has no service account sends the
	// investigation in the wrong direction.
	err error
}

func (p prodBlocker) Verify(context.Context, string, string) (Verification, error) {
	if p.err == nil {
		return Verification{}, fmt.Errorf(
			"%w: no real store verifier is configured; refusing receipts outside dev/test", ErrUnconfigured)
	}
	return Verification{}, p.err
}

func isDevOrTest(env string) bool {
	env = strings.ToLower(strings.TrimSpace(env))
	return env == "development" || env == "test"
}

// verifierSettings is the resolved configuration, recorded so a verifier can be
// reused across requests.
type verifierSettings struct {
	env           string
	mode          string
	devOrTest     bool
	appleBundleID string
	appleEnv      string
	appleRoots    string
	applePlans    map[string]string
	playPackage   string
	playKeyPath   string
	playPlans     map[string]string
}

// settingsFromEnv reads the configuration.
func settingsFromEnv() verifierSettings {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("ENV")))
	if env == "" {
		env = "development"
	}
	return verifierSettings{
		env:           env,
		mode:          strings.ToLower(strings.TrimSpace(os.Getenv(EnvBillingVerifier))),
		devOrTest:     isDevOrTest(env),
		appleBundleID: strings.TrimSpace(os.Getenv(EnvAppleBundleID)),
		appleEnv:      strings.TrimSpace(os.Getenv(EnvAppleEnvironment)),
		appleRoots:    strings.TrimSpace(os.Getenv(EnvAppleRootsPath)),
		applePlans:    productPlanMap(EnvApplePlans, EnvAppleMonthly, EnvAppleAnnual),
		playPackage:   strings.TrimSpace(os.Getenv(EnvGooglePackage)),
		playKeyPath:   googleKeyPath(),
		playPlans:     productPlanMap(EnvGooglePlans, EnvGoogleMonthly, EnvGoogleAnnual),
	}
}

// googleKeyPath returns the service-account key for the Play Developer API.
//
// FCM_SERVICE_ACCOUNT is accepted as a fallback because one Google Cloud
// service account usually holds both roles, and requiring the same file path
// twice is a configuration trap. It is a fallback, not a default: the API call
// fails with a 403 and an operator-readable message if the account it names has
// no access to the Play Console.
func googleKeyPath() string {
	if p := strings.TrimSpace(os.Getenv(EnvGoogleKeyPath)); p != "" {
		return p
	}
	return strings.TrimSpace(os.Getenv(EnvFCMServiceAccount))
}

// productPlanMap builds a product-id to plan map from the environment.
//
// Two spellings are accepted on purpose. The named variables cover the two
// products this server actually sells and are what an operator will reach for;
// the list variable covers everything else (a promotional SKU, a second
// subscription group) without a code change:
//
//	APPLE_PRODUCT_MONTHLY=app.iconfess.premium.monthly
//	APPLE_PRODUCT_PLANS=app.iconfess.promo.monthly=monthly,app.iconfess.family=annual
func productPlanMap(listVar, monthlyVar, annualVar string) map[string]string {
	out := map[string]string{}
	add := func(product, plan string) {
		product = strings.TrimSpace(product)
		plan = strings.TrimSpace(plan)
		if product == "" || plan == "" {
			return
		}
		out[product] = plan
	}

	add(os.Getenv(monthlyVar), "monthly")
	add(os.Getenv(annualVar), "annual")

	for _, pair := range strings.Split(os.Getenv(listVar), ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		product, plan, ok := strings.Cut(pair, "=")
		if !ok {
			// A malformed entry is ignored rather than fatal: the named
			// variables still work, and a typo must not take billing down in a
			// way that is invisible. It is skipped loudly at boot instead (see
			// MissingProductMappings).
			continue
		}
		add(product, plan)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// cacheKey identifies a configuration. Verifiers are cached by it because
// VerifierFromEnv is called per request, and a Google verifier rebuilt per
// request would mint a fresh access token per request: the token cache lives
// inside the token source, so a new source is a new token every time, which
// Google rate-limits and which makes every verification slower than it needs to
// be.
func (s verifierSettings) cacheKey() string {
	products := func(m map[string]string) string {
		if len(m) == 0 {
			return ""
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k+"="+m[k])
		}
		sort.Strings(keys)
		return strings.Join(keys, ";")
	}
	return strings.Join([]string{
		s.env, s.mode, s.appleBundleID, s.appleEnv, s.appleRoots, products(s.applePlans),
		s.playPackage, s.playKeyPath, products(s.playPlans),
	}, "|")
}

var (
	verifierMu    sync.Mutex
	verifierCache = map[string]Verifier{}
)

// VerifierFromEnv returns the verifier for the current configuration, building
// it once per distinct configuration.
func VerifierFromEnv() Verifier {
	s := settingsFromEnv()
	key := s.cacheKey()

	verifierMu.Lock()
	defer verifierMu.Unlock()
	if v, ok := verifierCache[key]; ok {
		return v
	}
	v := s.build()
	verifierCache[key] = v
	return v
}

// build resolves the settings into a verifier, failing closed outside dev/test.
func (s verifierSettings) build() Verifier {
	switch s.mode {
	case "":
		if s.devOrTest {
			return NoopVerifier{}
		}
		return prodBlocker{err: fmt.Errorf(
			"%w: ENV=%s and %s is unset - no store verifier is configured, so no receipt can be verified",
			ErrUnconfigured, s.env, EnvBillingVerifier)}
	case "apple", "google", "chained":
	default:
		// A typo must not silently disable verification.
		return prodBlocker{err: fmt.Errorf(
			"%w: %s=%q is not a known verifier (want apple, google or chained)",
			ErrUnconfigured, EnvBillingVerifier, s.mode)}
	}

	var (
		verifiers []Verifier
		reasons   []string
	)

	if s.mode == "apple" || s.mode == "chained" {
		v, err := s.appleVerifier()
		if err != nil {
			reasons = append(reasons, err.Error())
		} else {
			verifiers = append(verifiers, v)
		}
	}
	if s.mode == "google" || s.mode == "chained" {
		v, err := s.googleVerifier()
		if err != nil {
			reasons = append(reasons, err.Error())
		} else {
			verifiers = append(verifiers, v)
		}
	}

	if len(verifiers) == 0 {
		reason := strings.Join(reasons, "; ")
		if s.devOrTest {
			// Development has no store credentials by definition, and the whole
			// point of the stub is that the mobile purchase flow can be
			// exercised locally. Say so loudly rather than letting a developer
			// believe they are testing real verification.
			log.Printf("%s - falling back to the development stub verifier (ENV=%s)", reason, s.env)
			return NoopVerifier{}
		}
		return prodBlocker{err: fmt.Errorf("%w: %s", ErrUnconfigured, reason)}
	}
	if len(verifiers) == 1 {
		return verifiers[0]
	}
	return ChainedVerifier{Verifiers: verifiers}
}

func (s verifierSettings) appleVerifier() (*AppleVerifier, error) {
	var extraRoots [][]byte
	if s.appleRoots != "" {
		pem, err := os.ReadFile(s.appleRoots)
		if err != nil {
			return nil, fmt.Errorf("%w: cannot read %s=%s: %v", ErrUnconfigured, EnvAppleRootsPath, s.appleRoots, err)
		}
		extraRoots = append(extraRoots, pem)
	}
	roots, err := AppleRootsWith(extraRoots)
	if err != nil {
		return nil, err
	}
	// Production must not accept a sandbox receipt. Sandbox subscriptions are
	// free and Apple's sandbox is open to anyone with a developer account, so
	// accepting one in production is a way to obtain premium without paying.
	appleEnv := s.appleEnv
	if appleEnv == "" && !s.devOrTest {
		return nil, fmt.Errorf("%w: %s must be set to %s outside dev/test",
			ErrUnconfigured, EnvAppleEnvironment, AppleEnvironmentProduction)
	}
	return NewAppleVerifier(AppleConfig{
		BundleID:     s.appleBundleID,
		Environment:  appleEnv,
		ProductPlans: s.applePlans,
		Roots:        roots,
	})
}

func (s verifierSettings) googleVerifier() (*GooglePlayVerifier, error) {
	if s.playKeyPath == "" {
		return nil, fmt.Errorf("%w: %s is required to verify Play purchases",
			ErrUnconfigured, EnvGoogleKeyPath)
	}
	key, err := os.ReadFile(s.playKeyPath)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read Google service account %s: %v",
			ErrUnconfigured, s.playKeyPath, err)
	}
	source, err := googleauth.New(key, googleauth.ScopeAndroidPublisher)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnconfigured, err)
	}
	return NewGooglePlayVerifier(GooglePlayConfig{
		PackageName: s.playPackage,
		Plans:       s.playPlans,
		Purchases: &playapi.Client{
			PackageName: s.playPackage,
			TokenSource: source.Token,
		},
	})
}

// DescribeVerifier names the active verifier for a boot log line.
//
// Billing that is silently disabled is the failure mode this whole file exists
// to remove, so the process says out loud which verifier it installed.
func DescribeVerifier(v Verifier) string {
	switch t := v.(type) {
	case NoopVerifier:
		return "development stub (receipts starting valid_ are accepted - never use this outside development or test)"
	case prodBlocker:
		return "fail-closed: " + t.description().Error()
	case *AppleVerifier:
		return fmt.Sprintf("Apple App Store (%s, environment %q)", t.cfg.BundleID, t.cfg.Environment)
	case *GooglePlayVerifier:
		return fmt.Sprintf("Google Play (%s)", t.cfg.PackageName)
	case ChainedVerifier:
		names := make([]string, 0, len(t.Verifiers))
		for _, inner := range t.Verifiers {
			names = append(names, DescribeVerifier(inner))
		}
		return "chain of " + strings.Join(names, " + ")
	default:
		return fmt.Sprintf("%T", v)
	}
}

// RequireVerification refuses to boot with a verifier that cannot verify.
//
// The runtime failure mode is already safe: a fail-closed verifier refuses every
// receipt rather than believing one. But it is silent until the first customer
// tries to pay, at which point the complaint is "the app will not accept my
// purchase" and the diagnosis starts from the beginning. A failed deploy is the
// cheap place to discover that the service account was never mounted.
//
// Empty and unknown BILLING_VERIFIER values are refused outside dev/test, as is
// a known provider with no product mappings: all three end in a prodBlocker, and
// all three mean no receipt can be verified.
func RequireVerification() error {
	if b, blocked := VerifierFromEnv().(prodBlocker); blocked {
		return b.description()
	}
	return nil
}

// MissingProductMappings reports configuration that would make verification
// impossible, so boot can refuse instead of accepting money it cannot verify.
func MissingProductMappings() []string {
	s := settingsFromEnv()
	var missing []string
	if s.mode == "apple" || s.mode == "chained" {
		if s.appleBundleID == "" {
			missing = append(missing, EnvAppleBundleID)
		}
		if len(s.applePlans) == 0 {
			missing = append(missing, EnvAppleMonthly+" or "+EnvAppleAnnual)
		}
	}
	if s.mode == "google" || s.mode == "chained" {
		if s.playPackage == "" {
			missing = append(missing, EnvGooglePackage)
		}
		if len(s.playPlans) == 0 {
			missing = append(missing, EnvGoogleMonthly+" or "+EnvGoogleAnnual)
		}
		if s.playKeyPath == "" {
			missing = append(missing, EnvGoogleKeyPath)
		}
	}
	return missing
}
