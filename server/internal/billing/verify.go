package billing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
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

	// Provider identifiers, carried so the caller can persist what the store
	// actually said rather than only the plan it maps to. Without these a
	// renewal or a refund cannot be matched to a user except by expiry date,
	// which is ambiguous the moment one user buys twice.
	TransactionID         string `json:"transaction_id,omitempty"`
	OriginalTransactionID string `json:"original_transaction_id,omitempty"`
	ProductID             string `json:"product_id,omitempty"`
	// Environment is the store environment that issued the receipt: Sandbox or
	// Production for Apple, Production or Test for Play.
	Environment string `json:"environment,omitempty"`
	// State is the subscription state in this server's vocabulary (see
	// internal/models). The store's own words differ between providers, and
	// keeping them would push that difference into the database column that
	// decides entitlement.
	State string `json:"state,omitempty"`
	// AutoRenew is nil when the provider did not say. Absent is not false: a
	// prepaid plan has no auto-renewal at all, and recording that as "switched
	// off" would make a healthy subscription look cancelled.
	AutoRenew *bool `json:"auto_renew,omitempty"`
	// NeedsAcknowledgement marks a Play purchase that has not been acknowledged
	// yet. Play refunds unacknowledged purchases after three days, so a
	// deployment that ignores this flag takes money and then loses it.
	NeedsAcknowledgement bool `json:"needs_acknowledgement,omitempty"`
}

var (
	ErrInvalidReceipt = errors.New("invalid receipt")
	ErrProviderError  = errors.New("provider verification failed")
	// ErrUnconfigured means the server has no usable verifier: no credentials,
	// no product mapping, or no bundle identifier. It is distinct from
	// ErrInvalidReceipt because the fault is ours, not the caller's, and it
	// must never be reported to a user as "your receipt is invalid".
	ErrUnconfigured = errors.New("billing verifier is not configured")
)

// NoopVerifier is the development and test verifier.
//
// It accepts receipts starting with "valid_", rejects "invalid_", and reports
// "revoked_" as a refund. It is reachable from development and test only, which
// VerifierFromEnv enforces — a stub reachable from staging is what made premium
// free in the first place (IC-003).
//
// It fabricates two things on purpose, because a verifier that returns nothing
// but a plan makes the code around it untestable outside production:
//
//   - an expiry, so the entitlement clock is exercised rather than every
//     subscription being written with no end date;
//   - a stable original transaction id derived from the receipt, so "the same
//     purchase cannot be redeemed by two accounts" is exercised too.
//
// The id is a hash of the receipt rather than the receipt itself, so fixtures
// and logs stay readable and a development receipt pasted into a bug report
// does not become a credential.
type NoopVerifier struct{}

// NoopExpiry is how long a development receipt is valid for.
const (
	noopMonthlyDays = 30
	noopAnnualDays  = 365
)

func (NoopVerifier) Verify(_ context.Context, provider, receipt string) (Verification, error) {
	receipt = strings.TrimSpace(receipt)
	if receipt == "" {
		return Verification{}, ErrInvalidReceipt
	}

	plan, days := "monthly", noopMonthlyDays
	if strings.Contains(receipt, "annual") {
		plan, days = "annual", noopAnnualDays
	}
	// The outcome prefix is not part of the purchase's identity. A store reports
	// one original transaction id as active when it is bought and as refunded
	// later, so valid_monthly and revoked_monthly have to name the same purchase
	// here too: otherwise the revocation path could only be exercised with a
	// real refund from Apple or Google.
	identity := noopIdentity(strings.ToLower(strings.TrimSpace(provider)), noopSubject(receipt))

	switch {
	case strings.HasPrefix(receipt, "valid_"):
		return Verification{
			Valid:                 true,
			PlanID:                plan,
			Provider:              provider,
			State:                 models.SubscriptionActive,
			ExpiresAt:             time.Now().UTC().AddDate(0, 0, days).Format(time.RFC3339),
			TransactionID:         identity,
			OriginalTransactionID: identity,
			ProductID:             "dev." + plan,
			Environment:           "Development",
			Detail:                "noop: test receipt",
		}, nil
	case strings.HasPrefix(receipt, "revoked_"):
		// A refund: genuine, but it must not entitle. Reachable in development
		// so the revocation path has a test that does not need Apple's servers.
		return Verification{
			Valid:                 false,
			Provider:              provider,
			State:                 models.SubscriptionRefunded,
			TransactionID:         identity,
			OriginalTransactionID: identity,
			ProductID:             "dev." + plan,
			Environment:           "Development",
			Detail:                "noop: refunded test receipt",
		}, nil
	case strings.HasPrefix(receipt, "invalid_"):
		return Verification{Valid: false, Detail: "receipt rejected by noop"}, nil
	default:
		return Verification{}, fmt.Errorf("%w: receipt must start with valid_ in dev", ErrInvalidReceipt)
	}
}

// noopSubject strips the outcome prefix from a development receipt, leaving the
// purchase the receipt refers to.
func noopSubject(receipt string) string {
	for _, prefix := range []string{"valid_", "revoked_"} {
		if strings.HasPrefix(receipt, prefix) {
			return strings.TrimPrefix(receipt, prefix)
		}
	}
	return receipt
}

// noopIdentity derives a stable pseudo transaction id from a development
// receipt: the same string always produces the same id, which is what makes the
// duplicate-receipt rule observable in development.
func noopIdentity(provider, receipt string) string {
	sum := sha256.Sum256([]byte(provider + "|" + receipt))
	return "dev-" + hex.EncodeToString(sum[:8])
}

// ChainedVerifier tries each verifier in order.
//
// A verifier that does not handle the provider returns ErrInvalidReceipt, which
// means "not mine" rather than "forged" — so a chain must keep going rather than
// stopping at the first refusal. Only when every verifier has refused does the
// chain report a rejection, and it reports the operator-facing error when one
// occurred: a missing credential must not be disguised as a bad receipt, or a
// misconfigured deployment looks like a stream of fraudulent users.
type ChainedVerifier struct {
	Verifiers []Verifier
}

func (c ChainedVerifier) Verify(ctx context.Context, provider, receipt string) (Verification, error) {
	var (
		lastInvalid   error
		providerFault error
	)
	for _, v := range c.Verifiers {
		ver, err := v.Verify(ctx, provider, receipt)
		switch {
		case err == nil:
			return ver, nil
		case errors.Is(err, ErrInvalidReceipt):
			// Keep the first rejection so the caller learns why the receipt was
			// refused, but keep looking: a later verifier may own the provider.
			if lastInvalid == nil {
				lastInvalid = err
			}
		default:
			// ErrUnconfigured or a provider outage. Remember it, because if
			// nothing accepts the receipt this is the truth worth reporting.
			if providerFault == nil {
				providerFault = err
			}
		}
	}
	if providerFault != nil {
		return Verification{}, providerFault
	}
	if lastInvalid != nil {
		return Verification{}, lastInvalid
	}
	return Verification{}, fmt.Errorf("%w: no verifier is configured", ErrUnconfigured)
}
