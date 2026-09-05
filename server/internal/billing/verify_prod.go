package billing

import (
	"context"
	"errors"
	"os"
	"strings"
)

// EnvVerifier selects the real verifier from env. In production set
// BILLING_VERIFIER=apple|google|chained ; default is NoopVerifier (dev/test).
// Production verifiers are stubbed here and must be replaced with
// App Store Server API / Play Developer API calls before launch.
type EnvVerifier struct{ inner Verifier }

func VerifierFromEnv() Verifier {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("BILLING_VERIFIER")))
	switch mode {
	case "apple":
		return appleVerifier{}
	case "google":
		return googleVerifier{}
	case "chained":
		return ChainedVerifier{Verifiers: []Verifier{appleVerifier{}, googleVerifier{}, NoopVerifier{}}}
	default:
		return NoopVerifier{}
	}
}

type appleVerifier struct{}

func (appleVerifier) Verify(ctx context.Context, provider, receipt string) (Verification, error) {
	if strings.ToLower(provider) != "apple" {
		return Verification{}, ErrProviderError
	}
	// TODO: call App Store Server API — verify transactionId / JWS
	if receipt == "" {
		return Verification{}, ErrInvalidReceipt
	}
	if strings.HasPrefix(receipt, "apple_valid_") {
		plan := "monthly"
		if strings.Contains(receipt, "annual") {
			plan = "annual"
		}
		return Verification{Valid: true, PlanID: plan, Provider: "apple", Detail: "apple: verified (stub — replace with StoreKit 2)"}, nil
	}
	return Verification{}, errors.New("apple: not verified — set BILLING_VERIFIER to noop in dev, wire App Store Server API for prod")
}

type googleVerifier struct{}

func (googleVerifier) Verify(ctx context.Context, provider, receipt string) (Verification, error) {
	if strings.ToLower(provider) != "google" {
		return Verification{}, ErrProviderError
	}
	if receipt == "" {
		return Verification{}, ErrInvalidReceipt
	}
	if strings.HasPrefix(receipt, "google_valid_") {
		plan := "monthly"
		if strings.Contains(receipt, "annual") {
			plan = "annual"
		}
		return Verification{Valid: true, PlanID: plan, Provider: "google", Detail: "google: verified (stub — replace with Play Developer API)"}, nil
	}
	return Verification{}, errors.New("google: not verified — set BILLING_VERIFIER to noop in dev, wire Play Developer API for prod")
}
