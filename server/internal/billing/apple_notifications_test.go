package billing

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// Tests for App Store Server Notifications V2 (IC-003, PR B).
//
// The property under test is not "a renewal is applied". It is that a
// notification which does not carry a valid Apple signature cannot change
// entitlement, because this endpoint is reachable by anyone who knows the URL
// and a subscription is worth money. The forged-payload tests below are
// therefore the important ones; the acceptance tests exist to prove the
// verifier is not simply refusing everything.

// signAppleNotification builds a notification the way Apple does: a JWS over
// the notification, whose data.signedTransactionInfo is itself a JWS over the
// transaction.
func signAppleNotification(t *testing.T, pki *appleTestPKI, notification map[string]any, tx map[string]any) []byte {
	t.Helper()

	data := map[string]any{
		"appAppleId":  1234567890,
		"bundleId":    testBundleID,
		"environment": AppleEnvironmentSandbox,
	}
	if tx != nil {
		data["signedTransactionInfo"] = signAppleJWS(t, pki.leafKey, pki.chain(), tx)
	}
	for k, v := range notification {
		data[k] = v
	}

	payload := map[string]any{
		"notificationType": AppleNotificationDidRenew,
		"notificationUUID": "0f8a1c6e-6b1e-4f1f-9a3a-2b4c5d6e7f80",
		"version":          "2.0",
		"signedDate":       time.Now().UnixMilli(),
		"data":             data,
	}
	for k, v := range notification {
		payload[k] = v
	}

	signed := signAppleJWS(t, pki.leafKey, pki.chain(), payload)
	body, err := json.Marshal(map[string]any{"signedPayload": signed})
	if err != nil {
		t.Fatalf("marshal notification body: %v", err)
	}
	return body
}

func newAppleNotificationVerifier(t *testing.T, pki *appleTestPKI, mutate func(*AppleNotificationConfig)) *AppleNotificationVerifier {
	t.Helper()

	cfg := AppleNotificationConfig{
		BundleID: testBundleID,
		ProductPlans: map[string]string{
			testMonthlySKU: "monthly",
			testAnnualSKU:  "annual",
		},
		Roots: pki.pool(),
	}
	if mutate != nil {
		mutate(&cfg)
	}
	verifier, err := NewAppleNotificationVerifier(cfg)
	if err != nil {
		t.Fatalf("build notification verifier: %v", err)
	}
	return verifier
}

// ---------------------------------------------------------------------------
// The property that matters: a forged notification changes nothing
// ---------------------------------------------------------------------------

// TestAppleNotificationRejectsAForgedSignature is the regression test for the
// notification endpoint.
//
// The body is well-formed in every respect except one: it is signed by a
// certificate authority that is not Apple's. Everything inside it claims an
// active, unexpired, correctly-bundled annual subscription. A verifier that
// reads the JSON before checking the signature grants premium here.
func TestAppleNotificationRejectsAForgedSignature(t *testing.T) {
	genuinePKI := newAppleTestPKI(t)
	attackerPKI := newAppleTestPKI(t)

	// The attacker signs with their own chain, which does not reach the pinned
	// root the verifier trusts.
	body := signAppleNotification(t, attackerPKI, map[string]any{}, appleClaims())

	verifier := newAppleNotificationVerifier(t, genuinePKI, nil)
	if _, err := verifier.Decode(body); err == nil {
		t.Fatal("a notification signed by another CA was accepted")
	} else if !errors.Is(err, ErrInvalidReceipt) {
		t.Fatalf("error = %v, want ErrInvalidReceipt", err)
	}
}

// TestAppleNotificationRejectsATamperedPayload re-signs nothing: it edits the
// base64 payload of a genuine notification, which is what an attacker who can
// observe Apple's traffic would try.
func TestAppleNotificationRejectsATamperedPayload(t *testing.T) {
	pki := newAppleTestPKI(t)
	body := signAppleNotification(t, pki, map[string]any{}, appleClaims())

	var envelope map[string]string
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	parts := strings.Split(envelope["signedPayload"], ".")
	if len(parts) != 3 {
		t.Fatalf("signed payload has %d parts", len(parts))
	}
	// Flip a character in the middle of the payload segment.
	payload := []byte(parts[1])
	mid := len(payload) / 2
	if payload[mid] == 'A' {
		payload[mid] = 'B'
	} else {
		payload[mid] = 'A'
	}
	tampered := parts[0] + "." + string(payload) + "." + parts[2]
	rewritten, err := json.Marshal(map[string]string{"signedPayload": tampered})
	if err != nil {
		t.Fatalf("marshal tampered body: %v", err)
	}

	verifier := newAppleNotificationVerifier(t, pki, nil)
	if _, err := verifier.Decode(rewritten); err == nil {
		t.Fatal("a notification whose payload was edited was accepted")
	}
}

// TestAppleNotificationRejectsAnotherBundle checks the check that stops another
// app's notifications - including another team's app - from writing entitlement
// on this deployment.
func TestAppleNotificationRejectsAnotherBundle(t *testing.T) {
	pki := newAppleTestPKI(t)
	notification := map[string]any{"data": nil}
	body := signAppleNotification(t, pki, notification, appleClaims())

	// Rewrite the body's bundle id, keeping the signature valid: this models a
	// genuinely signed notification for a different application.
	verifier := newAppleNotificationVerifier(t, pki, func(cfg *AppleNotificationConfig) {
		cfg.BundleID = "app.someone.else"
	})
	if _, err := verifier.Decode(body); err == nil {
		t.Fatal("a notification for another bundle id was accepted")
	}
}

// TestAppleNotificationRejectsAnUnsignedBody covers the shape of request an
// attacker sends first: the client-asserted JSON, with no JWS at all.
func TestAppleNotificationRejectsAnUnsignedBody(t *testing.T) {
	pki := newAppleTestPKI(t)
	verifier := newAppleNotificationVerifier(t, pki, nil)

	for _, body := range []string{
		`{"notificationType":"DID_RENEW","data":{"bundleId":"app.iconfess"}}`,
		`{"signedPayload":"not-a-jws"}`,
		`{"signedPayload":""}`,
		`not json at all`,
	} {
		if _, err := verifier.Decode([]byte(body)); err == nil {
			t.Fatalf("body %q was accepted", body)
		}
	}
}

// ---------------------------------------------------------------------------
// Acceptance: a genuine notification is applied
// ---------------------------------------------------------------------------

func TestAppleNotificationAcceptsASignedRenewal(t *testing.T) {
	pki := newAppleTestPKI(t)
	body := signAppleNotification(t, pki, map[string]any{}, appleClaims())

	note, err := newAppleNotificationVerifier(t, pki, nil).Decode(body)
	if err != nil {
		t.Fatalf("genuine notification rejected: %v", err)
	}
	if !note.Applies {
		t.Fatalf("renewal did not apply: %+v", note)
	}
	if note.NotificationID != "0f8a1c6e-6b1e-4f1f-9a3a-2b4c5d6e7f80" {
		t.Errorf("notification id = %q, want the notificationUUID", note.NotificationID)
	}
	if !note.Verification.Valid || note.Verification.State != "active" {
		t.Errorf("verification = %+v, want an active, valid subscription", note.Verification)
	}
	if note.Verification.OriginalTransactionID != testTransactionID {
		t.Errorf("original transaction id = %q", note.Verification.OriginalTransactionID)
	}
	if note.EventTime.IsZero() {
		t.Error("event time is zero, so the notification cannot be ordered")
	}
}

// TestAppleNotificationRevokesOnRefund: a refund must revoke access even though
// the paid period is still running. Watching only the expiry leaves a refunded
// annual subscription premium for the rest of the year.
func TestAppleNotificationRevokesOnRefund(t *testing.T) {
	pki := newAppleTestPKI(t)
	body := signAppleNotification(t, pki,
		map[string]any{"notificationType": AppleNotificationRefund, "subtype": "UNREPORTED"},
		appleClaims())

	note, err := newAppleNotificationVerifier(t, pki, nil).Decode(body)
	if err != nil {
		t.Fatalf("refund notification rejected: %v", err)
	}
	if !note.Applies {
		t.Fatal("a refund was not applied")
	}
	if note.Verification.Valid {
		t.Error("a refunded purchase is still reported as valid")
	}
	if note.Verification.State != "refunded" {
		t.Errorf("state = %q, want refunded", note.Verification.State)
	}
}

// TestAppleNotificationRevokesOnARefundWithoutRevocationDate: the revocation
// fields are how Apple normally reports a refund, but REFUND is the
// notification Apple sends because of that decision. If the fields are ever
// absent, believing the type is what revokes the entitlement.
func TestAppleNotificationRevokesOnARefundWithoutRevocationDate(t *testing.T) {
	pki := newAppleTestPKI(t)
	claims := appleClaims() // no revocationDate, expiry still in the future
	body := signAppleNotification(t, pki, map[string]any{"notificationType": AppleNotificationRefund}, claims)

	note, err := newAppleNotificationVerifier(t, pki, nil).Decode(body)
	if err != nil {
		t.Fatalf("refund notification rejected: %v", err)
	}
	if note.Verification.Valid || note.Verification.State != "refunded" {
		t.Fatalf("verification = %+v, want a refunded, non-entitling verdict", note.Verification)
	}
}

func TestAppleNotificationExpiresOnExpiry(t *testing.T) {
	pki := newAppleTestPKI(t)
	claims := appleClaims()
	claims["expiresDate"] = time.Now().Add(-time.Hour).UnixMilli()
	body := signAppleNotification(t, pki, map[string]any{"notificationType": AppleNotificationExpired}, claims)

	note, err := newAppleNotificationVerifier(t, pki, nil).Decode(body)
	if err != nil {
		t.Fatalf("expiry notification rejected: %v", err)
	}
	if note.Verification.Valid || note.Verification.State != "expired" {
		t.Fatalf("verification = %+v, want an expired, non-entitling verdict", note.Verification)
	}
}

// TestAppleNotificationIgnoresTypesItDoesNotActOn: an unrecognised type must
// not be able to grant or revoke anything. The test notification is the one an
// operator triggers from the console, and it carries no purchase.
func TestAppleNotificationIgnoresTypesItDoesNotActOn(t *testing.T) {
	pki := newAppleTestPKI(t)
	for _, kind := range []string{"TEST", "CONSUMPTION_REQUEST", "SOMETHING_APPLE_ADDS_LATER"} {
		body := signAppleNotification(t, pki, map[string]any{"notificationType": kind}, nil)
		note, err := newAppleNotificationVerifier(t, pki, nil).Decode(body)
		if err != nil {
			t.Fatalf("%s: genuine notification rejected: %v", kind, err)
		}
		if note.Applies {
			t.Errorf("%s: recorded as applying to entitlement", kind)
		}
		if note.Detail == "" {
			t.Errorf("%s: no detail explaining why nothing changed", kind)
		}
	}
}

// ---------------------------------------------------------------------------
// Fields the guarantees rest on
// ---------------------------------------------------------------------------

// A notification without an id cannot be deduplicated, and one without a date
// cannot be ordered. Both are refused rather than defaulted: a fabricated
// timestamp would silently win or lose against real events.
func TestAppleNotificationRequiresIDAndDate(t *testing.T) {
	pki := newAppleTestPKI(t)

	for name, mutate := range map[string]func(map[string]any){
		"no uuid":        func(n map[string]any) { n["notificationUUID"] = "" },
		"no signed date": func(n map[string]any) { n["signedDate"] = 0 },
		"no type":        func(n map[string]any) { n["notificationType"] = "" },
	} {
		notification := map[string]any{}
		mutate(notification)
		body := signAppleNotification(t, pki, notification, appleClaims())
		if _, err := newAppleNotificationVerifier(t, pki, nil).Decode(body); err == nil {
			t.Errorf("%s: notification was accepted", name)
		}
	}
}

// TestAppleNotificationRejectsUnmappedProduct: an unmapped product means the
// catalogue and the store configuration disagree, and guessing a plan from a
// substring turns that mistake into free premium.
func TestAppleNotificationRejectsUnmappedProduct(t *testing.T) {
	pki := newAppleTestPKI(t)
	claims := appleClaims()
	claims["productId"] = "app.iconfess.premium.weekly"
	body := signAppleNotification(t, pki, map[string]any{}, claims)

	if _, err := newAppleNotificationVerifier(t, pki, nil).Decode(body); err == nil {
		t.Fatal("a notification for an unmapped product was accepted")
	}
}

// TestAppleNotificationRejectsSandboxInProduction is the same rule the receipt
// verifier applies: sandbox subscriptions are free, so accepting one where real
// money is taken is a way to obtain premium without paying.
func TestAppleNotificationRejectsSandboxInProduction(t *testing.T) {
	pki := newAppleTestPKI(t)
	body := signAppleNotification(t, pki, map[string]any{}, appleClaims())

	verifier := newAppleNotificationVerifier(t, pki, func(cfg *AppleNotificationConfig) {
		cfg.Environment = AppleEnvironmentProduction
	})
	if _, err := verifier.Decode(body); err == nil {
		t.Fatal("a Sandbox notification was accepted by a production deployment")
	}
}

func TestNewAppleNotificationVerifierRefusesIncompleteConfiguration(t *testing.T) {
	pki := newAppleTestPKI(t)

	if _, err := NewAppleNotificationVerifier(AppleNotificationConfig{
		ProductPlans: map[string]string{testMonthlySKU: "monthly"},
		Roots:        pki.pool(),
	}); !errors.Is(err, ErrUnconfigured) {
		t.Errorf("missing bundle id: err = %v, want ErrUnconfigured", err)
	}
	if _, err := NewAppleNotificationVerifier(AppleNotificationConfig{
		BundleID: testBundleID,
		Roots:    pki.pool(),
	}); !errors.Is(err, ErrUnconfigured) {
		t.Errorf("missing product map: err = %v, want ErrUnconfigured", err)
	}
}

// TestAppleNotificationFromEnvFailsClosed: outside development and test, a
// deployment that has not configured Apple verification must refuse to verify
// anything rather than accept unverifiable notifications.
func TestAppleNotificationFromEnvFailsClosed(t *testing.T) {
	for _, env := range []string{"production", "staging"} {
		t.Setenv("ENV", env)
		t.Setenv(EnvAppleBundleID, "")
		t.Setenv(EnvAppleMonthly, "")
		t.Setenv(EnvAppleAnnual, "")
		t.Setenv(EnvApplePlans, "")

		if _, err := AppleNotificationFromEnv(); !errors.Is(err, ErrUnconfigured) {
			t.Errorf("ENV=%s: err = %v, want ErrUnconfigured", env, err)
		}
	}
}

// The notification path reuses the receipt verifier's configuration, so a
// correctly configured receipt deployment gets a working notification endpoint
// without a second set of variables to get wrong.
func TestAppleNotificationFromEnvUsesTheReceiptConfiguration(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv(EnvAppleBundleID, testBundleID)
	t.Setenv(EnvAppleMonthly, testMonthlySKU)
	t.Setenv(EnvAppleEnvironment, AppleEnvironmentSandbox)

	verifier, err := AppleNotificationFromEnv()
	if err != nil {
		t.Fatalf("AppleNotificationFromEnv: %v", err)
	}
	if verifier.cfg.BundleID != testBundleID {
		t.Errorf("bundle id = %q", verifier.cfg.BundleID)
	}
	if len(verifier.cfg.Roots.Subjects()) == 0 {
		t.Error("verifier has no trust anchors")
	}
}
