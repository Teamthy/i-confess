package billing

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// App Store Server Notifications V2 (IC-003, PR B).
//
// A receipt tells this server what the client bought at the moment the client
// asked. Everything that happens afterwards - the renewal at 3am, the card that
// failed and then recovered, the refund Apple granted after a support call - is
// pushed to this endpoint, and until it is handled the entitlement row is a
// snapshot of the last app launch.
//
// The trust model is the same as the receipt verifier's, and deliberately uses
// the same code: Apple signs the notification as a JWS whose x5c chain ends at
// the pinned Apple Root CA - G3, exactly as it signs a transaction. There is no
// shared secret to configure and nothing for a caller to guess, which is why
// this endpoint needs no authentication header - a request without a valid
// Apple signature is not a request from Apple.
//
// The nested payload matters: data.signedTransactionInfo is itself a JWS, and
// the entitlement decision is made from *that*, not from the notificationType
// string. The type says why Apple is writing to us; the transaction says what
// the subscription now is. Reading the type alone would mean maintaining a
// second, parallel copy of the rules in apple.go - and two decision paths for
// one subscription is how a webhook and a receipt end up disagreeing.

// Apple notification types this server acts on.
//
// The list is closed on purpose. A type that is not here is recorded and
// changes nothing, so an unknown future notification cannot grant or revoke
// entitlement by accident. Both failure directions are expensive: granting on
// an unrecognised type is free premium, and revoking on one is a paid
// subscriber losing access because Apple added a field.
const (
	AppleNotificationSubscribed          = "SUBSCRIBED"
	AppleNotificationDidRenew            = "DID_RENEW"
	AppleNotificationDidFailToRenew      = "DID_FAIL_TO_RENEW"
	AppleNotificationExpired             = "EXPIRED"
	AppleNotificationGracePeriodExpired  = "GRACE_PERIOD_EXPIRED"
	AppleNotificationRefund              = "REFUND"
	AppleNotificationRevoke              = "REVOKE"
	AppleNotificationRenewalExtended     = "RENEWAL_EXTENDED"
	AppleNotificationOfferRedeemed       = "OFFER_REDEEMED"
	AppleNotificationPriceIncrease       = "PRICE_INCREASE"
	AppleNotificationChangeRenewalStatus = "DID_CHANGE_RENEWAL_STATUS"
	AppleNotificationChangeRenewalPref   = "DID_CHANGE_RENEWAL_PREF"
)

// appleNotificationTypes is the set whose verdict is applied. Anything else -
// TEST, CONSUMPTION_REQUEST, and whatever Apple adds next - is recorded and
// ignored.
var appleNotificationTypes = map[string]bool{
	AppleNotificationSubscribed:          true,
	AppleNotificationDidRenew:            true,
	AppleNotificationDidFailToRenew:      true,
	AppleNotificationExpired:             true,
	AppleNotificationGracePeriodExpired:  true,
	AppleNotificationRefund:              true,
	AppleNotificationRevoke:              true,
	AppleNotificationRenewalExtended:     true,
	AppleNotificationOfferRedeemed:       true,
	AppleNotificationPriceIncrease:       true,
	AppleNotificationChangeRenewalStatus: true,
	AppleNotificationChangeRenewalPref:   true,
}

// AppleNotificationConfig configures notification verification.
//
// It mirrors AppleConfig because it has to reach the same verdicts, and it is
// built from the same environment variables (see AppleNotificationFromEnv).
type AppleNotificationConfig struct {
	// BundleID is the app the notification must be about. Required.
	BundleID string
	// Environment restricts which store environment is accepted, "Sandbox" or
	// "Production". Empty accepts either, which is appropriate only where the
	// receipt verifier also accepts either (development and test).
	Environment string
	// ProductPlans maps a product id to a plan in this server's catalogue. A
	// product that is not mapped does not grant anything.
	ProductPlans map[string]string
	// Roots overrides the trust anchors. Nil means the pinned Apple root.
	Roots *x509.CertPool
	// Now is injectable for tests.
	Now func() time.Time
}

// AppleNotification is a verified App Store notification.
type AppleNotification struct {
	// NotificationID is Apple's notificationUUID: the idempotency key.
	NotificationID string
	// Type and Subtype are Apple's strings, recorded verbatim.
	Type    string
	Subtype string
	// EventTime is when Apple signed the notification, which orders it against
	// every other event for the same subscription.
	EventTime time.Time
	// Applies reports whether this notification can change entitlement.
	Applies bool
	// Verification is the subscription state the signed transaction resolves
	// to, in this server's vocabulary. Zero when the notification carries no
	// transaction.
	Verification Verification
	// Detail is a human explanation for the audit trail.
	Detail string
}

// AppleNotificationVerifier verifies and decodes App Store notifications.
type AppleNotificationVerifier struct {
	cfg     AppleNotificationConfig
	receipt *AppleVerifier
}

// NewAppleNotificationVerifier builds a verifier, refusing a configuration that
// could not decide anything.
func NewAppleNotificationVerifier(cfg AppleNotificationConfig) (*AppleNotificationVerifier, error) {
	// The receipt verifier already validates the configuration and owns the
	// product map. Building one here rather than duplicating the checks means
	// the two paths cannot disagree about what a valid configuration is.
	receipt, err := NewAppleVerifier(AppleConfig{
		BundleID:     cfg.BundleID,
		Environment:  cfg.Environment,
		ProductPlans: cfg.ProductPlans,
		Roots:        cfg.Roots,
		Now:          cfg.Now,
	})
	if err != nil {
		return nil, err
	}
	return &AppleNotificationVerifier{cfg: cfg, receipt: receipt}, nil
}

func (v *AppleNotificationVerifier) now() time.Time {
	if v.cfg.Now != nil {
		return v.cfg.Now().UTC()
	}
	return time.Now().UTC()
}

// appleNotificationEnvelope is the outer JSON Apple POSTs.
type appleNotificationEnvelope struct {
	SignedPayload string `json:"signedPayload"`
}

// appleNotificationPayload is the decoded notification.
//
// Pointers are used for the optional timestamps for the same reason as in
// apple.go: absent and zero are different facts.
type appleNotificationPayload struct {
	NotificationType string `json:"notificationType"`
	Subtype          string `json:"subtype"`
	NotificationUUID string `json:"notificationUUID"`
	Version          string `json:"version"`
	SignedDate       *int64 `json:"signedDate"`
	Data             *struct {
		AppAppleID            int64  `json:"appAppleId"`
		BundleID              string `json:"bundleId"`
		Environment           string `json:"environment"`
		SignedTransactionInfo string `json:"signedTransactionInfo"`
		SignedRenewalInfo     string `json:"signedRenewalInfo"`
		Status                *int64 `json:"status"`
	} `json:"data"`
}

// Decode verifies a notification body and returns what it means.
//
// The body is the raw request body: {"signedPayload": "..."}. Nothing outside
// the signed payload is read, because nothing outside it is covered by Apple's
// signature.
func (v *AppleNotificationVerifier) Decode(body []byte) (AppleNotification, error) {
	var envelope appleNotificationEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return AppleNotification{}, fmt.Errorf("%w: notification body is not JSON: %v", ErrInvalidReceipt, err)
	}
	if strings.TrimSpace(envelope.SignedPayload) == "" {
		return AppleNotification{}, fmt.Errorf("%w: notification carries no signed payload", ErrInvalidReceipt)
	}

	// Layer 1: the notification itself.
	payload, err := v.verifyJWS(envelope.SignedPayload)
	if err != nil {
		return AppleNotification{}, err
	}
	var note appleNotificationPayload
	if err := json.Unmarshal(payload, &note); err != nil {
		return AppleNotification{}, fmt.Errorf("%w: notification payload is not JSON: %v", ErrInvalidReceipt, err)
	}

	// The id is the idempotency key for the whole feature. Without it a retry
	// is indistinguishable from a new event, so a notification that has none is
	// refused rather than applied with a fabricated id.
	if strings.TrimSpace(note.NotificationUUID) == "" {
		return AppleNotification{}, fmt.Errorf("%w: notification has no notificationUUID", ErrInvalidReceipt)
	}
	if strings.TrimSpace(note.NotificationType) == "" {
		return AppleNotification{}, fmt.Errorf("%w: notification has no type", ErrInvalidReceipt)
	}
	if note.SignedDate == nil || *note.SignedDate <= 0 {
		// Apple always signs notifications. A missing date would leave the
		// event with no place in the ordering, and defaulting it to "now" would
		// let a replayed old notification win against a new one.
		return AppleNotification{}, fmt.Errorf("%w: notification has no signed date", ErrInvalidReceipt)
	}
	if note.Data == nil {
		return AppleNotification{}, fmt.Errorf("%w: notification carries no data", ErrInvalidReceipt)
	}
	// The bundle check is what stops a notification from another app - or
	// another team's app - from writing entitlement here. It runs before any
	// decision, exactly as it does in the receipt verifier.
	if note.Data.BundleID != v.cfg.BundleID {
		return AppleNotification{}, fmt.Errorf("%w: notification is for bundle %q, not %q",
			ErrInvalidReceipt, note.Data.BundleID, v.cfg.BundleID)
	}
	switch note.Data.Environment {
	case AppleEnvironmentSandbox, AppleEnvironmentProduction:
	default:
		return AppleNotification{}, fmt.Errorf("%w: notification has unknown environment %q",
			ErrInvalidReceipt, note.Data.Environment)
	}
	if v.cfg.Environment != "" && note.Data.Environment != v.cfg.Environment {
		return AppleNotification{}, fmt.Errorf("%w: notification is from %s but this deployment accepts %s",
			ErrInvalidReceipt, note.Data.Environment, v.cfg.Environment)
	}

	out := AppleNotification{
		NotificationID: note.NotificationUUID,
		Type:           note.NotificationType,
		Subtype:        note.Subtype,
		EventTime:      time.UnixMilli(*note.SignedDate).UTC(),
	}

	// Layer 2: the transaction inside the notification. Its verdict is the
	// decision; the notification type only explains why Apple sent it.
	if strings.TrimSpace(note.Data.SignedTransactionInfo) == "" {
		out.Detail = fmt.Sprintf("apple: %s carries no transaction - recorded, no entitlement change", note.NotificationType)
		return out, nil
	}
	txPayload, err := v.verifyJWS(note.Data.SignedTransactionInfo)
	if err != nil {
		return AppleNotification{}, fmt.Errorf("apple: signedTransactionInfo: %w", err)
	}
	var tx appleTransaction
	if err := json.Unmarshal(txPayload, &tx); err != nil {
		return AppleNotification{}, fmt.Errorf("%w: signedTransactionInfo is not a transaction: %v", ErrInvalidReceipt, err)
	}
	ver, err := v.receipt.evaluate(tx)
	if err != nil {
		return AppleNotification{}, err
	}

	// A refund or a revocation is a decision Apple has made, and the
	// transaction's own revocation date is normally how it is reported. The
	// type is applied on top because REFUND and REVOKE are the notifications
	// Apple sends *because* of that decision: if a transaction ever arrives
	// without the revocation fields filled in, believing the type is the
	// behaviour that revokes a refunded subscription instead of leaving it
	// premium for the rest of the year.
	switch note.NotificationType {
	case AppleNotificationRefund, AppleNotificationRevoke:
		ver.Valid = false
		ver.State = "refunded"
		if ver.Detail == "" {
			ver.Detail = "apple: " + note.NotificationType
		}
	case AppleNotificationExpired, AppleNotificationGracePeriodExpired:
		ver.Valid = false
		ver.State = "expired"
		ver.Detail = "apple: " + note.NotificationType
	}

	out.Verification = ver
	// Only the types this server understands may change entitlement. Anything
	// else is recorded for the audit trail and stops there.
	out.Applies = appleNotificationTypes[note.NotificationType] && ver.State != ""
	if !out.Applies {
		out.Detail = fmt.Sprintf("apple: %s%s is not a type this server acts on - recorded, no entitlement change",
			note.NotificationType, subtypeSuffix(note.Subtype))
	}
	return out, nil
}

// verifyJWS checks a three-part JWS signed by Apple and returns its payload.
//
// The order is the same one apple.go uses for receipts, and for the same
// reason: chain, then signature, then claims. The chain is validated at
// signedDate, because Apple's signing certificates rotate and a notification
// signed while a certificate was valid stays valid after it expires.
func (v *AppleNotificationVerifier) verifyJWS(signed string) ([]byte, error) {
	parts := strings.Split(strings.TrimSpace(signed), ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: apple payload is not a three-part JWS", ErrInvalidReceipt)
	}
	header, err := decodeAppleHeader(parts[0])
	if err != nil {
		return nil, err
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: apple payload is not base64url", ErrInvalidReceipt)
	}
	leaf, err := verifyAppleChain(header.X5c, v.cfg.Roots, signingTime(payload, v.now()))
	if err != nil {
		return nil, err
	}
	if err := verifyJWSES256(leaf, parts[0], parts[1], parts[2]); err != nil {
		return nil, err
	}
	return payload, nil
}

// subtypeSuffix renders a subtype for a log or audit line, or nothing.
func subtypeSuffix(subtype string) string {
	if strings.TrimSpace(subtype) == "" {
		return ""
	}
	return " (" + subtype + ")"
}
