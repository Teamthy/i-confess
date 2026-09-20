package billing

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/Teamthy/i-confess/internal/googleauth"
	"github.com/Teamthy/i-confess/internal/playapi"
)

// Building the notification path from configuration (IC-003, PR B).
//
// The rule is the same one verify_prod.go applies to receipts, for the same
// reason: an endpoint that cannot verify must refuse, and outside development
// and test a deployment that is missing its store credentials must fail loudly
// rather than accept unverifiable events. A webhook endpoint that "works" in
// production while checking nothing is worse than one that returns 503,
// because nobody investigates a 503 that never fires - and the numbers in the
// entitlement table look plausible either way.

// AppleNotificationFromEnv builds the App Store notification verifier.
//
// It reuses the receipt verifier's environment (APPLE_BUNDLE_ID,
// APPLE_ENVIRONMENT, APPLE_ROOT_CA_PATH, APPLE_PRODUCT_*) because the two paths
// must reach the same verdicts about the same subscriptions; a second set of
// variables would be a second chance to configure one of them wrong.
func AppleNotificationFromEnv() (*AppleNotificationVerifier, error) {
	s := settingsFromEnv()
	if s.appleBundleID == "" {
		return nil, fmt.Errorf("%w: %s is required to verify App Store notifications",
			ErrUnconfigured, EnvAppleBundleID)
	}
	if len(s.applePlans) == 0 {
		return nil, fmt.Errorf("%w: no App Store product ids are mapped to plans (%s or %s)",
			ErrUnconfigured, EnvAppleMonthly, EnvAppleAnnual)
	}
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
	appleEnv := s.appleEnv
	if appleEnv == "" && !s.devOrTest {
		return nil, fmt.Errorf("%w: %s must be set to %s outside dev/test",
			ErrUnconfigured, EnvAppleEnvironment, AppleEnvironmentProduction)
	}
	return NewAppleNotificationVerifier(AppleNotificationConfig{
		BundleID:     s.appleBundleID,
		Environment:  appleEnv,
		ProductPlans: s.applePlans,
		Roots:        roots,
	})
}

// PlayAcknowledger confirms a Play purchase with Google.
//
// It exists because Play refunds an unacknowledged purchase after three days.
// PR A detected the condition and logged it; a log line does not stop a refund,
// so the purchase was still lost - and lost in the quietest possible way, three
// days after a successful sale, with the customer still entitled and the money
// gone.
type PlayAcknowledger interface {
	// Acknowledge confirms a purchase. The product id is required because the
	// acknowledge endpoint is addressed by (product, token), unlike the v2
	// read that is addressed by token alone.
	Acknowledge(ctx context.Context, productID, purchaseToken string) error
}

// playAcknowledger adapts the API client to the interface above.
type playAcknowledger struct {
	client *playapi.Client
}

func (a playAcknowledger) Acknowledge(ctx context.Context, productID, purchaseToken string) error {
	if strings.TrimSpace(productID) == "" {
		// Without a product id the endpoint cannot be addressed. Reporting it
		// as ErrUnconfigured keeps "we could not do this" separate from "the
		// store refused it", which is the distinction the caller logs.
		return fmt.Errorf("%w: no product id is known for purchase token %s", ErrUnconfigured, purchaseToken)
	}
	return a.client.Acknowledge(ctx, productID, purchaseToken)
}

var (
	playClientMu    sync.Mutex
	playClientCache = map[string]*playapi.Client{}
)

// playClientFromEnv returns a cached Play API client for the current settings.
//
// Cached because the client holds the OAuth token source, and the token cache
// lives inside it: a client built per request would mint a fresh access token
// per request, which Google rate-limits and which makes every call slower than
// the verification it performs.
func playClientFromEnv() (*playapi.Client, error) {
	s := settingsFromEnv()
	if s.playKeyPath == "" {
		return nil, fmt.Errorf("%w: %s is required to call the Play Developer API",
			ErrUnconfigured, EnvGoogleKeyPath)
	}
	if s.playPackage == "" {
		return nil, fmt.Errorf("%w: %s is required to call the Play Developer API",
			ErrUnconfigured, EnvGooglePackage)
	}
	key := s.playKeyPath + "|" + s.playPackage

	playClientMu.Lock()
	defer playClientMu.Unlock()
	if c, ok := playClientCache[key]; ok {
		return c, nil
	}
	keyJSON, err := os.ReadFile(s.playKeyPath)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read Google service account %s: %v",
			ErrUnconfigured, s.playKeyPath, err)
	}
	source, err := googleauth.New(keyJSON, googleauth.ScopeAndroidPublisher)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnconfigured, err)
	}
	c := &playapi.Client{PackageName: s.playPackage, TokenSource: source.Token}
	playClientCache[key] = c
	return c, nil
}

// GoogleNotificationFromEnv builds the Play notification resolver.
func GoogleNotificationFromEnv() (*GoogleNotificationVerifier, error) {
	s := settingsFromEnv()
	if s.playPackage == "" {
		return nil, fmt.Errorf("%w: %s is required to verify Play notifications",
			ErrUnconfigured, EnvGooglePackage)
	}
	if len(s.playPlans) == 0 {
		return nil, fmt.Errorf("%w: no Play product ids are mapped to plans (%s or %s)",
			ErrUnconfigured, EnvGoogleMonthly, EnvGoogleAnnual)
	}
	client, err := playClientFromEnv()
	if err != nil {
		return nil, err
	}
	return NewGoogleNotificationVerifier(GoogleNotificationConfig{
		PackageName: s.playPackage,
		Plans:       s.playPlans,
		Purchases:   client,
	})
}

// PlayAcknowledgerFromEnv returns an acknowledger, or nil when Play is not
// configured.
//
// A nil acknowledger with a nil error is not a misconfiguration: development
// runs without store credentials by design, and the caller logs that the
// acknowledgement was skipped rather than failing the request. Outside
// dev/test, a missing acknowledger is reported as an error by the caller, which
// is what turns "we take money and lose it three days later" into something
// visible.
func PlayAcknowledgerFromEnv() (PlayAcknowledger, error) {
	client, err := playClientFromEnv()
	if err != nil {
		if s := settingsFromEnv(); s.devOrTest {
			return nil, nil
		}
		return nil, err
	}
	return playAcknowledger{client: client}, nil
}
