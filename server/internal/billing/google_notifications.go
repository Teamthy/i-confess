package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/playapi"
)

// Play Real Time Developer Notifications (IC-003, PR B).
//
// A Play notification says that something happened to a purchase token. It
// deliberately does not say what the subscription now is: Google's own guidance
// is that the state may lag the message, and the message body is not
// authenticated to this server beyond the audience and signature on the Pub/Sub
// envelope. So the notification is treated as a hint to go and ask, and the
// verdict comes from purchases.subscriptionsv2 - the same call, through the
// same evaluate(), that a client-presented receipt goes through.
//
// Two consequences worth stating plainly:
//
//   - A forged or replayed notification cannot grant anything. The worst it can
//     do is make this server ask Google about a token, and Google's answer is
//     what is applied.
//   - A REVOKED notification is applied as a revocation even if the API still
//     reports the subscription active, because that lag is the documented
//     behaviour for refunds, and waiting for the API to catch up would leave a
//     refunded subscription entitled.

// GoogleNotificationInput is what the transport layer learned from a Pub/Sub
// delivery, before anything has been verified.
type GoogleNotificationInput struct {
	// NotificationID is the Pub/Sub messageId: the idempotency key. Pub/Sub
	// retries a delivery until it is acknowledged, so the same message id
	// arrives more than once as a matter of course.
	NotificationID string
	// PackageName is the applicationId from the notification body.
	PackageName string
	// PurchaseToken names the subscription the notification is about.
	PurchaseToken string
	// Type is Google's numeric notification type, 0 when absent.
	Type int
	// EventTime is when Google says the event happened, from the body.
	EventTime time.Time
	// Test marks the console's test notification, which carries no purchase.
	Test bool
}

// GoogleNotificationConfig configures Play notification resolution.
type GoogleNotificationConfig struct {
	// PackageName is the applicationId the notification must belong to.
	// Required.
	PackageName string
	// Plans maps a Play product id to a plan in this server's catalogue.
	Plans map[string]string
	// Purchases is the Play Developer API client used to ask what the
	// subscription now is. Required.
	Purchases PlayPurchaseClient
	// Now is injectable for tests.
	Now func() time.Time
}

// GoogleNotification is a resolved Play notification.
type GoogleNotification struct {
	// NotificationID is the Pub/Sub message id: the idempotency key.
	NotificationID string
	// Type is Google's numeric type and TypeName its readable form.
	Type     int
	TypeName string
	// PurchaseToken is the subscription the notification names.
	PurchaseToken string
	// EventTime orders this event against the others for the same subscription.
	EventTime time.Time
	// Applies reports whether this notification may change entitlement.
	Applies bool
	// Verification is the state Google's API reports, in this server's
	// vocabulary.
	Verification Verification
	// Detail explains the outcome for the audit trail.
	Detail string
}

// GoogleNotificationVerifier resolves Play notifications against Google's API.
type GoogleNotificationVerifier struct {
	cfg      GoogleNotificationConfig
	verifier *GooglePlayVerifier
}

// NewGoogleNotificationVerifier builds a resolver, refusing a configuration
// that could not decide anything.
func NewGoogleNotificationVerifier(cfg GoogleNotificationConfig) (*GoogleNotificationVerifier, error) {
	// The Play verifier owns the package check and the product map. Building
	// one here means a notification and a receipt cannot disagree about which
	// package or which products this deployment sells.
	// The conversion is load-bearing, not a shortcut: Go only permits it while
	// the two configurations have identical fields in the same order, so a
	// setting added to one and forgotten in the other stops compiling instead
	// of silently becoming a notification path that verifies against a
	// different product map than the receipt path.
	verifier, err := NewGooglePlayVerifier(GooglePlayConfig(cfg))
	if err != nil {
		return nil, err
	}
	return &GoogleNotificationVerifier{cfg: cfg, verifier: verifier}, nil
}

// Resolve turns a notification into a verdict by asking Google.
func (v *GoogleNotificationVerifier) Resolve(ctx context.Context, in GoogleNotificationInput) (GoogleNotification, error) {
	out := GoogleNotification{
		NotificationID: in.NotificationID,
		Type:           in.Type,
		TypeName:       playapi.NotificationTypeName(in.Type),
		PurchaseToken:  in.PurchaseToken,
		EventTime:      in.EventTime,
	}
	if in.Test {
		out.Detail = "play: test notification from the Play Console - recorded, no entitlement change"
		return out, nil
	}
	// The package check mirrors the Apple bundle check: this endpoint is
	// reachable by anyone who knows the URL, and a notification about another
	// application must not be able to write rows here.
	if in.PackageName != v.cfg.PackageName {
		return GoogleNotification{}, fmt.Errorf("%w: notification is for package %q, not %q",
			ErrInvalidReceipt, in.PackageName, v.cfg.PackageName)
	}
	if !playapi.KnownNotificationType(in.Type) {
		out.Detail = fmt.Sprintf("play: %s is not a type this server acts on - recorded, no entitlement change",
			playapi.NotificationTypeName(in.Type))
		return out, nil
	}
	if in.PurchaseToken == "" {
		return GoogleNotification{}, fmt.Errorf("%w: play notification %s carries no purchase token",
			ErrInvalidReceipt, playapi.NotificationTypeName(in.Type))
	}

	// Ask. The notification is not the state; this call is.
	purchase, err := v.cfg.Purchases.Subscription(ctx, in.PurchaseToken)
	if err != nil {
		return GoogleNotification{}, translatePlayError(err)
	}
	ver, err := v.verifier.Evaluate(purchase)
	if err != nil {
		return GoogleNotification{}, err
	}
	ver.PurchaseToken = in.PurchaseToken

	// A revocation is a decision Google has already made. Apply it even when
	// the API has not caught up, which is the documented lag for refunds.
	if in.Type == playapi.NotificationRevoked {
		ver.Valid = false
		ver.State = models.SubscriptionRefunded
		ver.Detail = "play: SUBSCRIPTION_REVOKED - the purchase was refunded or charged back"
	}

	out.Verification = ver
	out.Applies = ver.State != ""
	if !out.Applies {
		out.Detail = "play: Google returned no subscription state - recorded, no entitlement change"
	}
	return out, nil
}
