package api

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Teamthy/i-confess/internal/analytics"
	"github.com/Teamthy/i-confess/internal/billing"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/playapi"
	"github.com/Teamthy/i-confess/internal/pubsub"
	"github.com/Teamthy/i-confess/internal/store"
)

// Store notification webhooks (IC-003, PR B).
//
// These two endpoints are how subscriptions change after the client has stopped
// looking: renewals, failed payments, grace periods, cancellations made from
// the store's settings screen, and refunds. Without them the entitlement row
// only moves when the app calls the verify endpoint, which is to say almost
// never.
//
// Both are public routes. That is not an oversight and not a weakness: they
// carry no session, because the caller is a store rather than a user, and their
// authentication is the signature on the payload - Apple's certificate chain
// for one, Google's OIDC token and a signed API call for the other. A shared
// secret in a URL would be strictly worse: it cannot be rotated per
// deployment without coordinating with Apple's console, and it proves only that
// the caller knows a string.
//
// Both answer 200 for anything they have finished with, including a duplicate
// or an event older than one already applied, because both stores retry
// non-2xx responses - and retrying a stale event forever is not a behaviour
// that improves anything. Refusals that are worth retrying (a missing
// credential, an unreachable key set) answer 503.

// appleNotificationVerifier and googleNotificationResolver are the store
// notification collaborators the endpoints need.
//
// They are interfaces rather than concrete types so a test can supply a
// verifier pinned to a generated certificate authority, and so a deployment
// that loads credentials from somewhere other than the environment can supply
// one. Production leaves them nil and the endpoints build theirs from the
// environment, which is the configuration the boot checks already validate.
type appleNotificationVerifier interface {
	Decode(body []byte) (billing.AppleNotification, error)
}

type googleNotificationResolver interface {
	Resolve(ctx context.Context, in billing.GoogleNotificationInput) (billing.GoogleNotification, error)
}

// SetAppleNotificationVerifier overrides the App Store notification verifier.
func (h *Handler) SetAppleNotificationVerifier(v appleNotificationVerifier) { h.appleNotify = v }

// SetGoogleNotificationResolver overrides the Play notification resolver.
func (h *Handler) SetGoogleNotificationResolver(v googleNotificationResolver) { h.googleNotify = v }

// SetPlayAcknowledger overrides the acknowledger used for Play purchases.
func (h *Handler) SetPlayAcknowledger(a billing.PlayAcknowledger) { h.playAck = a }

// appleVerifier returns the configured App Store verifier, building one from
// the environment when none was supplied.
func (h *Handler) appleVerifier() (appleNotificationVerifier, error) {
	if h.appleNotify != nil {
		return h.appleNotify, nil
	}
	return billing.AppleNotificationFromEnv()
}

// googleVerifier returns the configured Play resolver, building one from the
// environment when none was supplied.
func (h *Handler) googleVerifier() (googleNotificationResolver, error) {
	if h.googleNotify != nil {
		return h.googleNotify, nil
	}
	return billing.GoogleNotificationFromEnv()
}

// maxNotificationBody bounds a notification. Real ones are a few kilobytes;
// this is generous and still refuses a body that is only there to be parsed.
const maxNotificationBody = 256 << 10

// notificationResponse is the shape both endpoints answer with. It is small on
// purpose: the store reads the status code, and the body is for whoever is
// reading logs at 3am.
type notificationResponse struct {
	Provider string `json:"provider"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
}

// appleStoreWebhook handles App Store Server Notifications V2.
func (h *Handler) appleStoreWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxNotificationBody))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "could not read the notification body")
		return
	}

	verifier, err := h.appleVerifier()
	if err != nil {
		// Not configured: nothing here can verify a signature, so nothing here
		// may change entitlement. 503 rather than 200 so Apple retries - this
		// is the one refusal that a deployment fix resolves.
		log.Printf("billing: App Store notification refused, verification is not configured: %v", err)
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "store notification verification is not configured on this deployment",
			"code":  "VERIFIER_UNCONFIGURED",
		})
		return
	}

	note, err := verifier.Decode(body)
	if err != nil {
		// A body that does not verify is not from Apple. There is nothing to
		// retry and nothing to record: the request is refused, and the log line
		// is what an operator would use to see a forgery attempt.
		log.Printf("billing: App Store notification rejected: %v", err)
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{
			"error":  "the notification signature could not be verified",
			"code":   "INVALID_NOTIFICATION",
			"detail": err.Error(),
		})
		return
	}

	outcome, err := h.users.ApplyStoreNotification(r.Context(), store.StoreNotification{
		Provider:              "apple",
		NotificationID:        note.NotificationID,
		NotificationType:      note.Type,
		Subtype:               note.Subtype,
		OriginalTransactionID: note.Verification.OriginalTransactionID,
		EventTime:             note.EventTime.Format(time.RFC3339),
		Applies:               note.Applies,
		Plan:                  entitlementsPlan(note.Verification),
		State:                 note.Verification.State,
		ExpiresAt:             note.Verification.ExpiresAt,
		TransactionID:         note.Verification.TransactionID,
		ProductID:             note.Verification.ProductID,
		Environment:           note.Verification.Environment,
		AutoRenew:             note.Verification.AutoRenew,
		Detail:                note.Detail,
	})
	if err != nil {
		log.Printf("billing: could not record App Store notification %s: %v", note.NotificationID, err)
		// 500 so Apple retries: a database fault is transient, and the
		// notification is idempotent when it arrives again.
		httpx.WriteError(w, http.StatusInternalServerError, "could not record the notification")
		return
	}

	logNotification("apple", note.NotificationID, note.Type, outcome)
	h.recordSubscriptionCancellation(r.Context(), "apple", outcome)
	httpx.WriteJSON(w, http.StatusOK, notificationResponse{
		Provider: "apple",
		Status:   outcome.Status,
		Detail:   outcome.Detail,
	})
}

// googleStoreWebhook handles Play Real Time Developer Notifications delivered
// by a Pub/Sub push subscription.
func (h *Handler) googleStoreWebhook(w http.ResponseWriter, r *http.Request) {
	// Authentication first. The body is attacker-controlled until this passes,
	// and reading it before checking only gives an unauthenticated caller more
	// of this process than they are entitled to.
	verifier, err := pubsub.OIDCFromEnv()
	if err != nil {
		log.Printf("billing: Play notification refused, push authentication is not configured: %v", err)
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "push authentication is not configured on this deployment",
			"code":  "PUSH_AUTH_UNCONFIGURED",
		})
		return
	}
	if err := verifier.VerifyRequest(r.Context(), r); err != nil {
		switch {
		case errors.Is(err, pubsub.ErrUnavailable):
			// Google's key set was unreachable. The request may well be
			// genuine; Pub/Sub should try again.
			log.Printf("billing: Play notification could not be authenticated: %v", err)
			httpx.WriteError(w, http.StatusServiceUnavailable, "could not verify the push token")
		default:
			// Do not echo why into the response body. The log line is for us;
			// the caller learns only that it was refused.
			log.Printf("billing: Play notification rejected: %v", err)
			httpx.WriteError(w, http.StatusUnauthorized, "push request is not authenticated")
		}
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxNotificationBody))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "could not read the delivery body")
		return
	}
	envelope, err := pubsub.Decode(body)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "delivery is not a Pub/Sub push message")
		return
	}
	payload, err := envelope.Payload()
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "delivery payload is not base64")
		return
	}
	notification, err := playapi.ParseRTDN(payload)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "delivery is not a Play developer notification")
		return
	}

	eventTime, err := notification.EventTime()
	if err != nil {
		if notification.IsTest() {
			// A test notification from the Play Console is the one delivery
			// that legitimately has no event time. Date it from the envelope so
			// the audit row is not the zero time.
			if published, ok := envelope.PublishTime(); ok {
				eventTime = published
			} else {
				eventTime = time.Now().UTC()
			}
		} else {
			httpx.WriteError(w, http.StatusBadRequest, "notification has no usable event time")
			return
		}
	}

	resolver, err := h.googleVerifier()
	if err != nil {
		log.Printf("billing: Play notification refused, verification is not configured: %v", err)
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "store notification verification is not configured on this deployment",
			"code":  "VERIFIER_UNCONFIGURED",
		})
		return
	}

	// The notification is a hint; Google's API is the state. Resolve asks, and
	// only its answer is applied.
	note, err := resolver.Resolve(r.Context(), billing.GoogleNotificationInput{
		NotificationID: envelope.Message.MessageID,
		PackageName:    notification.PackageName,
		PurchaseToken:  notification.PurchaseToken(),
		Type:           notification.NotificationType(),
		EventTime:      eventTime,
		Test:           notification.IsTest(),
	})
	if err != nil {
		switch {
		case errors.Is(err, billing.ErrInvalidReceipt):
			log.Printf("billing: Play notification %s rejected: %v", envelope.Message.MessageID, err)
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{
				"error": "the notification does not describe a purchase this server sells",
				"code":  "INVALID_NOTIFICATION",
			})
		default:
			// Google's API or our credentials. Retryable, and worth a 503 so
			// Pub/Sub delivers again rather than dropping the event.
			log.Printf("billing: could not resolve Play notification %s: %v", envelope.Message.MessageID, err)
			httpx.WriteError(w, http.StatusServiceUnavailable, "could not confirm the purchase with Google")
		}
		return
	}

	outcome, err := h.users.ApplyStoreNotification(r.Context(), store.StoreNotification{
		Provider:              "google",
		NotificationID:        note.NotificationID,
		NotificationType:      strconv.Itoa(note.Type),
		Subtype:               note.TypeName,
		OriginalTransactionID: note.Verification.OriginalTransactionID,
		PurchaseToken:         note.PurchaseToken,
		EventTime:             note.EventTime.Format(time.RFC3339),
		Applies:               note.Applies,
		Plan:                  entitlementsPlan(note.Verification),
		State:                 note.Verification.State,
		ExpiresAt:             note.Verification.ExpiresAt,
		TransactionID:         note.Verification.TransactionID,
		ProductID:             note.Verification.ProductID,
		Environment:           note.Verification.Environment,
		AutoRenew:             note.Verification.AutoRenew,
		Detail:                note.Detail,
	})
	if err != nil {
		log.Printf("billing: could not record Play notification %s: %v", note.NotificationID, err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not record the notification")
		return
	}

	// A purchase Google is still waiting for an acknowledgement on is refunded
	// three days later. The client normally acknowledges by calling the verify
	// endpoint; doing it here as well covers the purchase whose app never made
	// it that far.
	if note.Applies && note.Verification.NeedsAcknowledgement {
		h.acknowledgePlayPurchase(r.Context(), outcome.UserID, note.Verification.ProductID, note.PurchaseToken)
	}

	logNotification("google", note.NotificationID, note.TypeName, outcome)
	h.recordSubscriptionCancellation(r.Context(), "google", outcome)
	httpx.WriteJSON(w, http.StatusOK, notificationResponse{
		Provider: "google",
		Status:   outcome.Status,
		Detail:   outcome.Detail,
	})
}

// entitlementsPlan maps a verifier's catalogue plan to the plan stored on the
// subscription row.
//
// The catalogue plan ("monthly", "annual") describes what was bought; the
// entitlement plan is what the product grants, and everything resolves from
// "premium". A notification about a refund states no plan at all, and an empty
// value leaves the stored one alone rather than blanking the record of what was
// bought.
func entitlementsPlan(ver billing.Verification) string {
	if ver.State == "" {
		return ""
	}
	if !ver.Valid {
		// Not entitled. The plan column keeps whatever it held: the purchase is
		// still the purchase, and erasing it would lose the answer to "what was
		// refunded?".
		return ""
	}
	return "premium"
}

// acknowledgePlayPurchase confirms a Play purchase, returning whether Google
// accepted it.
//
// A failure is logged rather than returned. The purchase is real and the
// customer is entitled either way; what a failure costs is the money, three
// days later, which is exactly the kind of thing that must not be invisible.
func (h *Handler) acknowledgePlayPurchase(ctx context.Context, userID, productID, purchaseToken string) bool {
	if h.playAck != nil {
		if err := h.playAck.Acknowledge(ctx, productID, purchaseToken); err != nil {
			log.Printf("billing: failed to acknowledge Play purchase %s for user %s: %v - Play refunds unacknowledged purchases after 3 days",
				productID, userID, err)
			return false
		}
		log.Printf("billing: acknowledged Play purchase %s for user %s", productID, userID)
		return true
	}
	acknowledger, err := billing.PlayAcknowledgerFromEnv()
	if err != nil {
		log.Printf("billing: cannot acknowledge Play purchase %s: %v - Play refunds unacknowledged purchases after 3 days",
			productID, err)
		return false
	}
	if acknowledger == nil {
		log.Printf("billing: Play acknowledgement skipped for %s (user %s) - no service account is configured in this environment",
			productID, userID)
		return false
	}
	if err := acknowledger.Acknowledge(ctx, productID, purchaseToken); err != nil {
		log.Printf("billing: failed to acknowledge Play purchase %s for user %s: %v - Play refunds unacknowledged purchases after 3 days",
			productID, userID, err)
		return false
	}
	log.Printf("billing: acknowledged Play purchase %s for user %s", productID, userID)
	return true
}

// logNotification records what a notification did, at a level that matches
// whether it changed anything.
//
// A duplicate is the expected consequence of at-least-once delivery and is not
// worth an error line every few minutes; an applied change or anything
// unexpected is.
// recordSubscriptionCancellation writes the cancellation side of the
// subscription funnel.
//
// A cancellation is the one lifecycle fact that cannot be recovered later: the
// next renewal overwrites subscriptions.status, so if the store notification
// that ended the subscription does not record it, the account simply looks
// like one that renewed. Only an applied notification records the event - a
// duplicate or stale delivery is not a second cancellation.
func (h *Handler) recordSubscriptionCancellation(ctx context.Context, provider string, outcome store.NotificationOutcome) {
	if !outcome.Applied() || outcome.UserID == "" {
		return
	}
	switch outcome.State {
	case models.SubscriptionCancelled, models.SubscriptionExpired, models.SubscriptionRefunded:
	default:
		return
	}
	_ = h.analytics.Record(ctx, analytics.Event{
		Name:   analytics.EventSubscriptionCancelled,
		UserID: outcome.UserID,
		Props: map[string]any{
			"provider": provider,
			"state":    outcome.State,
		},
	})
}

func logNotification(provider, id, kind string, outcome store.NotificationOutcome) {
	switch outcome.Status {
	case store.NotificationDuplicate:
		log.Printf("billing: %s notification %s (%s) already handled - no second effect", provider, id, kind)
	case store.NotificationStale:
		log.Printf("billing: %s notification %s (%s) is older than the state of record - not applied: %s",
			provider, id, kind, outcome.Detail)
	default:
		log.Printf("billing: %s notification %s (%s) -> %s (user %s): %s",
			provider, id, kind, outcome.Status, outcome.UserID, outcome.Detail)
	}
}
