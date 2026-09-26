package playapi

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Real Time Developer Notifications (RTDN).
//
// Google does not call this server directly. It publishes a message to a Pub/Sub
// topic and a push subscription delivers it here, so the body that arrives is a
// Pub/Sub envelope and the notification is base64 inside it. Decoding the
// envelope is transport work and lives in internal/pubsub; the notification's
// own vocabulary lives here, next to the API client that is asked to confirm
// what it says.
//
// Nothing in this payload is trusted for entitlement. It says *that* something
// happened to a purchase token; the current state is read from
// purchases.subscriptionsv2. That is deliberate: a notification is a
// suggestion to look, and Google's own documentation says the state may lag
// the message.

// Notification types, as Google numbers them. The names are Google's.
const (
	NotificationRecovered               = 1
	NotificationRenewed                 = 2
	NotificationCanceled                = 3
	NotificationPurchased               = 4
	NotificationOnHold                  = 5
	NotificationInGracePeriod           = 6
	NotificationRestarted               = 7
	NotificationPriceChangeConfirmed    = 8
	NotificationDeferred                = 9
	NotificationPaused                  = 10
	NotificationPauseScheduleChanged    = 11
	NotificationRevoked                 = 12
	NotificationExpired                 = 13
	NotificationPendingPurchaseCanceled = 20
)

// notificationNames is closed: a number that is not here is a type this server
// has never seen, and it is recorded as "type 47" rather than guessed at.
var notificationNames = map[int]string{
	NotificationRecovered:               "SUBSCRIPTION_RECOVERED",
	NotificationRenewed:                 "SUBSCRIPTION_RENEWED",
	NotificationCanceled:                "SUBSCRIPTION_CANCELED",
	NotificationPurchased:               "SUBSCRIPTION_PURCHASED",
	NotificationOnHold:                  "SUBSCRIPTION_ON_HOLD",
	NotificationInGracePeriod:           "SUBSCRIPTION_IN_GRACE_PERIOD",
	NotificationRestarted:               "SUBSCRIPTION_RESTARTED",
	NotificationPriceChangeConfirmed:    "SUBSCRIPTION_PRICE_CHANGE_CONFIRMED",
	NotificationDeferred:                "SUBSCRIPTION_DEFERRED",
	NotificationPaused:                  "SUBSCRIPTION_PAUSED",
	NotificationPauseScheduleChanged:    "SUBSCRIPTION_PAUSE_SCHEDULE_CHANGED",
	NotificationRevoked:                 "SUBSCRIPTION_REVOKED",
	NotificationExpired:                 "SUBSCRIPTION_EXPIRED",
	NotificationPendingPurchaseCanceled: "SUBSCRIPTION_PENDING_PURCHASE_CANCELED",
}

// NotificationTypeName renders a notification type for logs and the audit
// trail, naming an unknown number as such.
func NotificationTypeName(t int) string {
	if name, ok := notificationNames[t]; ok {
		return name
	}
	return "UNKNOWN_TYPE_" + strconv.Itoa(t)
}

// KnownNotificationType reports whether this server recognises the type.
func KnownNotificationType(t int) bool {
	_, ok := notificationNames[t]
	return ok
}

// RTDN is the decoded developer notification.
//
// eventTimeMillis stays a string because that is what Google sends: it is a
// string field in the JSON, and decoding it into an int64 would fail on the
// whole payload if a future version ever sent something else.
type RTDN struct {
	Version                  string `json:"version"`
	PackageName              string `json:"packageName"`
	EventTimeMillis          string `json:"eventTimeMillis"`
	SubscriptionNotification *struct {
		Version          string `json:"version"`
		NotificationType int    `json:"notificationType"`
		PurchaseToken    string `json:"purchaseToken"`
		SubscriptionID   string `json:"subscriptionId"`
	} `json:"subscriptionNotification"`
	TestNotification *struct {
		Version string `json:"version"`
	} `json:"testNotification"`
	VoidedPurchaseNotification *struct {
		PurchaseToken string `json:"purchaseToken"`
		OrderID       string `json:"orderId"`
		RefundType    int    `json:"refundType"`
	} `json:"voidedPurchaseNotification"`
}

// ParseRTDN decodes a notification payload.
func ParseRTDN(data []byte) (RTDN, error) {
	var out RTDN
	if err := json.Unmarshal(data, &out); err != nil {
		return RTDN{}, fmt.Errorf("play: developer notification is not JSON: %w", err)
	}
	if strings.TrimSpace(out.PackageName) == "" {
		return RTDN{}, fmt.Errorf("play: developer notification names no package")
	}
	return out, nil
}

// EventTime is when Google says the event happened.
func (n RTDN) EventTime() (time.Time, error) {
	raw := strings.TrimSpace(n.EventTimeMillis)
	if raw == "" {
		return time.Time{}, fmt.Errorf("play: developer notification has no eventTimeMillis")
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || ms <= 0 {
		return time.Time{}, fmt.Errorf("play: developer notification has an unusable eventTimeMillis %q", raw)
	}
	return time.UnixMilli(ms).UTC(), nil
}

// PurchaseToken is the token the notification is about. It is empty for a test
// notification, which has no purchase.
func (n RTDN) PurchaseToken() string {
	if n.SubscriptionNotification != nil {
		return strings.TrimSpace(n.SubscriptionNotification.PurchaseToken)
	}
	if n.VoidedPurchaseNotification != nil {
		return strings.TrimSpace(n.VoidedPurchaseNotification.PurchaseToken)
	}
	return ""
}

// NotificationType is the numeric type, or 0 when there is none.
func (n RTDN) NotificationType() int {
	if n.SubscriptionNotification != nil {
		return n.SubscriptionNotification.NotificationType
	}
	return 0
}

// IsTest reports a test notification: the message the Play Console sends to
// prove the endpoint is reachable. It carries no purchase and must never touch
// entitlement.
func (n RTDN) IsTest() bool { return n.TestNotification != nil }
