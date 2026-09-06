package billing

import (
	"context"
	"errors"
	"os"
	"strings"
)

// prodBlocker rejects every receipt unconditionally.
//
// It exists because of a real defect, not as a hypothetical. POST
// /subscriptions/verify is an authenticated public route that grants premium
// from whatever the selected verifier accepts. Every verifier in this file is a
// stub: NoopVerifier accepts any receipt beginning "valid_", and appleVerifier
// and googleVerifier accept "apple_valid_" and "google_valid_". Nothing checked
// the environment, so a production deployment with BILLING_VERIFIER unset - the
// default - granted premium to anyone who posted {"receipt":"valid_monthly"}.
//
// Until a real App Store Server API / Play Developer API verifier exists, the
// only safe production behaviour is to refuse. Failing closed on payments costs
// a launch delay; failing open costs revenue and is a §37 violation, since the
// rule is that the server validates purchases and never trusts the client.
type prodBlocker struct{}

func (prodBlocker) Verify(context.Context, string, string) (Verification, error) {
	return Verification{}, errors.New(
		"billing: no real store verifier is configured; refusing receipts in production")
}

// VerifierFromEnv selects the verifier from BILLING_VERIFIER
// (apple|google|chained), defaulting to NoopVerifier for dev and test.
//
// In production every current option is a stub, so production gets prodBlocker
// instead. This is the guard that was missing.
func VerifierFromEnv() Verifier {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production") {
		return prodBlocker{}
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BILLING_VERIFIER"))) {
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
