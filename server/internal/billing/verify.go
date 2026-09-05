package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Verifier validates a store receipt server-side.
// The client never asserts a plan; only this package can grant premium.
type Verifier interface {
	Verify(ctx context.Context, provider, receipt string) (Verification, error)
}

type Verification struct {
	Valid     bool   `json:"valid"`
	PlanID    string `json:"plan_id"`    // monthly | annual
	Provider  string `json:"provider"`   // apple | google | stripe
	ExpiresAt string `json:"expires_at"` // RFC3339
	Detail    string `json:"detail,omitempty"`
}

var (
	ErrInvalidReceipt = errors.New("invalid receipt")
	ErrProviderError  = errors.New("provider verification failed")
)

// NoopVerifier is the dev/test verifier: accepts receipts starting with "valid_".
// Production replaces this with AppleAppStoreVerifier + GooglePlayVerifier.
type NoopVerifier struct{}

func (NoopVerifier) Verify(_ context.Context, provider, receipt string) (Verification, error) {
	receipt = strings.TrimSpace(receipt)
	if receipt == "" {
		return Verification{}, ErrInvalidReceipt
	}
	if strings.HasPrefix(receipt, "valid_") {
		// e.g. valid_monthly_ -> monthly
		plan := "monthly"
		if strings.Contains(receipt, "annual") {
			plan = "annual"
		}
		return Verification{Valid: true, PlanID: plan, Provider: provider, Detail: "noop: test receipt"}, nil
	}
	if strings.HasPrefix(receipt, "invalid_") {
		return Verification{Valid: false, Detail: "receipt rejected by noop"}, nil
	}
	return Verification{}, fmt.Errorf("%w: receipt must start with valid_ in dev", ErrInvalidReceipt)
}

// ChainedVerifier tries each verifier in order.
type ChainedVerifier struct {
	Verifiers []Verifier
}

func (c ChainedVerifier) Verify(ctx context.Context, provider, receipt string) (Verification, error) {
	for _, v := range c.Verifiers {
		ver, err := v.Verify(ctx, provider, receipt)
		if err == nil {
			return ver, nil
		}
		if errors.Is(err, ErrInvalidReceipt) {
			return Verification{}, err
		}
	}
	return Verification{}, ErrProviderError
}
