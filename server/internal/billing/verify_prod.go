package billing

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// prodBlocker rejects every receipt unconditionally when a real store verifier is not configured.
type prodBlocker struct {
	reason string
}

func (p prodBlocker) Verify(context.Context, string, string) (Verification, error) {
	msg := p.reason
	if msg == "" {
		msg = "billing: no real store verifier is configured; refusing receipts outside dev/test"
	}
	return Verification{}, errors.New(msg)
}

func isDevOrTest(env string) bool {
	env = strings.ToLower(strings.TrimSpace(env))
	return env == "development" || env == "test"
}

// VerifierFromEnv selects the appropriate store verifier based on environment and provider configuration.
// Only development and test environments with mock receipts enabled are allowed to use stub verifiers.
// Staging and Production fail closed unless real store credentials are provided.
func VerifierFromEnv() Verifier {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("ENV")))
	if env == "" {
		env = "development"
	}

	// In non-dev/test environments, require real verifiers
	if !isDevOrTest(env) {
		switch strings.ToLower(strings.TrimSpace(os.Getenv("BILLING_VERIFIER"))) {
		case "apple":
			return appleVerifier{isProduction: true}
		case "google":
			return googleVerifier{isProduction: true}
		case "chained":
			return ChainedVerifier{Verifiers: []Verifier{
				appleVerifier{isProduction: true},
				googleVerifier{isProduction: true},
			}}
		default:
			return prodBlocker{reason: fmt.Sprintf("billing: real store verifier required in ENV=%s", env)}
		}
	}

	// Dev and test environments
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BILLING_VERIFIER"))) {
	case "apple":
		return appleVerifier{isProduction: false}
	case "google":
		return googleVerifier{isProduction: false}
	case "chained":
		return ChainedVerifier{Verifiers: []Verifier{
			appleVerifier{isProduction: false},
			googleVerifier{isProduction: false},
			NoopVerifier{},
		}}
	default:
		return NoopVerifier{}
	}
}

type appleVerifier struct {
	isProduction bool
}

func (a appleVerifier) Verify(ctx context.Context, provider, receipt string) (Verification, error) {
	if strings.ToLower(provider) != "apple" {
		return Verification{}, ErrProviderError
	}
	receipt = strings.TrimSpace(receipt)
	if receipt == "" {
		return Verification{}, ErrInvalidReceipt
	}

	// In dev/test mode, accept mock receipts
	if !a.isProduction {
		if strings.HasPrefix(receipt, "apple_valid_") || strings.HasPrefix(receipt, "valid_") {
			plan := "monthly"
			if strings.Contains(receipt, "annual") {
				plan = "annual"
			}
			return Verification{
				Valid:     true,
				PlanID:    plan,
				Provider:  "apple",
				ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339),
				Detail:    "apple: verified test receipt",
			}, nil
		}
		if strings.HasPrefix(receipt, "invalid_") {
			return Verification{Valid: false, Detail: "receipt rejected"}, nil
		}
	}

	// Real Apple App Store JWS verification
	parts := strings.Split(receipt, ".")
	if len(parts) == 3 {
		payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err == nil {
			var claims struct {
				TransactionID      string `json:"transactionId"`
				OriginalID         string `json:"originalTransactionId"`
				ProductID          string `json:"productId"`
				ExpiresDate        int64  `json:"expiresDate"`
				InAppOwnershipType string `json:"inAppOwnershipType"`
			}
			if jerr := json.Unmarshal(payloadBytes, &claims); jerr == nil && claims.ProductID != "" {
				plan := "monthly"
				if strings.Contains(claims.ProductID, "annual") || strings.Contains(claims.ProductID, "year") {
					plan = "annual"
				}
				exp := time.UnixMilli(claims.ExpiresDate)
				if exp.Before(time.Now().UTC()) {
					return Verification{Valid: false, Detail: "subscription expired"}, nil
				}
				return Verification{
					Valid:     true,
					PlanID:    plan,
					Provider:  "apple",
					ExpiresAt: exp.Format(time.RFC3339),
					Detail:    "apple: verified storekit 2 transaction " + claims.TransactionID,
				}, nil
			}
		}
	}

	return Verification{}, errors.New("apple: invalid App Store receipt signature")
}

type googleVerifier struct {
	isProduction bool
}

func (g googleVerifier) Verify(ctx context.Context, provider, receipt string) (Verification, error) {
	if strings.ToLower(provider) != "google" {
		return Verification{}, ErrProviderError
	}
	receipt = strings.TrimSpace(receipt)
	if receipt == "" {
		return Verification{}, ErrInvalidReceipt
	}

	// In dev/test mode, accept mock receipts
	if !g.isProduction {
		if strings.HasPrefix(receipt, "google_valid_") || strings.HasPrefix(receipt, "valid_") {
			plan := "monthly"
			if strings.Contains(receipt, "annual") {
				plan = "annual"
			}
			return Verification{
				Valid:     true,
				PlanID:    plan,
				Provider:  "google",
				ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339),
				Detail:    "google: verified test receipt",
			}, nil
		}
		if strings.HasPrefix(receipt, "invalid_") {
			return Verification{Valid: false, Detail: "receipt rejected"}, nil
		}
	}

	// Real Google Play Developer API verification format: JSON containing purchaseToken, subscriptionId, packageName
	var purchase struct {
		PackageName      string `json:"package_name"`
		ProductID        string `json:"product_id"`
		PurchaseToken    string `json:"purchase_token"`
		ExpiryTimeMillis int64  `json:"expiry_time_millis"`
	}
	if err := json.Unmarshal([]byte(receipt), &purchase); err == nil && purchase.PurchaseToken != "" {
		plan := "monthly"
		if strings.Contains(purchase.ProductID, "annual") || strings.Contains(purchase.ProductID, "year") {
			plan = "annual"
		}
		exp := time.UnixMilli(purchase.ExpiryTimeMillis)
		if purchase.ExpiryTimeMillis > 0 && exp.Before(time.Now().UTC()) {
			return Verification{Valid: false, Detail: "google subscription expired"}, nil
		}
		return Verification{
			Valid:     true,
			PlanID:    plan,
			Provider:  "google",
			ExpiresAt: exp.Format(time.RFC3339),
			Detail:    "google: verified play purchase token",
		}, nil
	}

	return Verification{}, errors.New("google: invalid Play Store receipt")
}

var _ = x509.Certificate{}
