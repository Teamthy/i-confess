package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/models"
)

// Store notification ledger (IC-003, PR B).
//
// The two properties this file exists to hold, and how it holds them:
//
//   - Idempotency. Both stores deliver at least once. The notification's own id
//     is inserted into store_notifications under a unique index *before*
//     anything else happens in the transaction, so the second delivery of one
//     event inserts nothing and is reported as a duplicate. It never reaches
//     the statement that writes a subscription. Doing this with a read-then-
//     write ("have I seen this id?") would be a race between two concurrent
//     retries, which is the exact traffic pattern a retry storm produces.
//
//   - Ordering. Events arrive in any order. subscriptions.last_store_event_at
//     is the watermark: an event whose store timestamp is older than the last
//     one applied is recorded as stale and refused, so the row ends up holding
//     the newest event's outcome regardless of arrival order.
//
// Everything happens in one transaction, because a ledger row written without
// the subscription update (or the other way round) is precisely the state that
// makes a replay apply twice.

// Statuses recorded in store_notifications.status.
const (
	// NotificationApplied means the notification was verified, newer than the
	// watermark, and wrote the subscription.
	NotificationApplied = "applied"
	// NotificationDuplicate means this notification id was already recorded.
	NotificationDuplicate = "duplicate"
	// NotificationStale means the notification was verified but is older than
	// an event already applied to that subscription.
	NotificationStale = "stale"
	// NotificationUnmatched means no account has redeemed this purchase yet.
	NotificationUnmatched = "unmatched"
	// NotificationIgnored means the notification cannot change entitlement: a
	// store test notification, or a type this server does not act on.
	NotificationIgnored = "ignored"
)

// StoreNotification is a verified store report about a subscription.
//
// Every field here has already been checked against a signature or the store's
// API by the caller; nothing in it is client-asserted. It carries the
// notification's identity (for idempotency), its timestamp (for ordering) and
// the subscription state the store resolved to.
type StoreNotification struct {
	Provider string
	// NotificationID is the store's own id: Apple's notificationUUID, or the
	// Pub/Sub messageId for a Play notification. Required; without it there is
	// no way to tell a retry from a new event, and the notification is refused
	// rather than applied.
	NotificationID string
	// NotificationType and Subtype are recorded verbatim for the audit trail.
	NotificationType string
	Subtype          string

	OriginalTransactionID string
	PurchaseToken         string

	// EventTime is when the store says the event happened (RFC3339, UTC). It
	// orders events, and it comes from the store's signed payload.
	EventTime string

	// Applies reports whether this notification may change entitlement. A
	// store test notification, or a type this server does not act on, is
	// recorded but changes nothing.
	Applies bool

	// The verified subscription state, when the notification can change it.
	Plan          string
	State         string
	ExpiresAt     string
	TransactionID string
	ProductID     string
	Environment   string
	AutoRenew     *bool

	// Detail is a human explanation, stored for the audit trail.
	Detail string
}

// NotificationOutcome reports what a notification did.
type NotificationOutcome struct {
	// Status is one of the Notification* constants.
	Status string
	// UserID is the account the notification was applied to, empty otherwise.
	UserID string
	Detail string
	// State is the subscription state the notification wrote, and is only set
	// when the notification applied. Carrying it lets the caller act on the
	// outcome - a cancellation is the one thing a listener's journey analytics
	// cannot recover from the row alone once a later renewal overwrites it.
	State string
}

// Applied reports whether the notification changed entitlement.
func (o NotificationOutcome) Applied() bool { return o.Status == NotificationApplied }

// ApplyStoreNotification records a verified store notification and, when it is
// the newest event for a known purchase, applies it to the subscription.
//
// The returned outcome is for logging and tests; it is not an error channel.
// A duplicate or a stale event is a normal, expected outcome of at-least-once
// delivery, and the caller answers the store with 200 in every one of those
// cases so it stops retrying.
func (s *UserStore) ApplyStoreNotification(ctx context.Context, n StoreNotification) (NotificationOutcome, error) {
	if err := n.validate(); err != nil {
		return NotificationOutcome{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return NotificationOutcome{}, err
	}
	defer func() { _ = tx.Rollback() }()

	received := now()
	// 1. Claim the notification. The unique index decides; a conflict means
	//    this exact event was already handled and must not be handled again.
	res, err := tx.ExecContext(ctx,
		`INSERT INTO store_notifications
		     (id, provider, notification_id, notification_type, subtype,
		      original_transaction_id, purchase_token, event_time, received_at, status, detail)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT (provider, notification_id) DO NOTHING`,
		newID(), n.Provider, n.NotificationID, n.NotificationType, n.Subtype,
		n.OriginalTransactionID, n.PurchaseToken, n.EventTime, received, NotificationDuplicate, "")
	if err != nil {
		return NotificationOutcome{}, fmt.Errorf("store: record notification: %w", err)
	}
	if claimed, cerr := res.RowsAffected(); cerr == nil && claimed == 0 {
		// Nothing was written, so there is nothing to commit. The previous
		// delivery already recorded this event and applied (or deliberately
		// did not apply) its effect.
		return NotificationOutcome{Status: NotificationDuplicate, Detail: "this notification id has already been handled"}, nil
	}

	// 2. Which account is this about? A notification can arrive before any
	//    client has redeemed the purchase - Apple notifies the server the
	//    moment the subscription is bought, and the app may not have launched
	//    since. There is no account to write yet, and inventing one from the
	//    product id is how a notification lands on the wrong subscriber.
	outcome, err := s.applyNotificationTx(ctx, tx, n, received)
	if err != nil {
		return NotificationOutcome{}, err
	}
	if err := finishNotification(ctx, tx, n, outcome); err != nil {
		return NotificationOutcome{}, err
	}
	if err := tx.Commit(); err != nil {
		return NotificationOutcome{}, err
	}
	return outcome, nil
}

// applyNotificationTx resolves the purchase to an account and applies the
// event, inside the caller's transaction.
func (s *UserStore) applyNotificationTx(ctx context.Context, tx *db.Tx, n StoreNotification, received string) (NotificationOutcome, error) {
	var (
		userID    string
		plan      sql.NullString
		lastEvent sql.NullString
	)
	// The purchase is matched on the store identity that was stored when the
	// receipt was verified: the original transaction id for Apple, the purchase
	// token for Play. Not on the expiry, and not on the product: a product id
	// is shared by every subscriber.
	query := `SELECT user_id, plan, last_store_event_at FROM subscriptions
	           WHERE provider = ? AND original_transaction_id = ? LIMIT 1`
	args := []any{n.Provider, n.OriginalTransactionID}
	if strings.TrimSpace(n.OriginalTransactionID) == "" {
		query = `SELECT user_id, plan, last_store_event_at FROM subscriptions
		          WHERE provider = ? AND purchase_token = ? LIMIT 1`
		args = []any{n.Provider, n.PurchaseToken}
	}
	err := tx.QueryRowContext(ctx, query, args...).Scan(&userID, &plan, &lastEvent)
	if errors.Is(err, sql.ErrNoRows) && n.PurchaseToken != "" && n.OriginalTransactionID != "" {
		// Play stores the purchase token separately from the linked token; a
		// notification carries the former, so fall back to it before deciding
		// the purchase is unknown.
		err = tx.QueryRowContext(ctx,
			`SELECT user_id, plan, last_store_event_at FROM subscriptions
			  WHERE provider = ? AND purchase_token = ? LIMIT 1`,
			n.Provider, n.PurchaseToken).Scan(&userID, &plan, &lastEvent)
	}
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return NotificationOutcome{
			Status: NotificationUnmatched,
			Detail: "no account has redeemed this purchase yet; recorded for the audit trail",
		}, nil
	case err != nil:
		return NotificationOutcome{}, fmt.Errorf("store: resolve notification purchase: %w", err)
	}

	// 3. Ordering. An event older than the newest one already applied must not
	//    move the row backwards - that is the whole failure mode of reordered
	//    delivery, and it is silent without this check.
	if prev, ok := parseStoreTime(lastEvent.String); ok {
		if event, ok := parseStoreTime(n.EventTime); ok && event.Before(prev) {
			return NotificationOutcome{
				Status: NotificationStale,
				UserID: userID,
				Detail: fmt.Sprintf("event time %s is older than the last applied event %s", n.EventTime, lastEvent.String),
			}, nil
		}
	}

	if !n.Applies {
		return NotificationOutcome{
			Status: NotificationIgnored,
			UserID: userID,
			Detail: n.Detail,
		}, nil
	}

	// 4. Apply. plan keeps its previous value when the notification does not
	//    name one: a refund says nothing about what was bought, and blanking
	//    the column would erase the record of the purchase that was refunded.
	if _, err := tx.ExecContext(ctx,
		`UPDATE subscriptions SET
		     plan = COALESCE(NULLIF(?, ''), plan),
		     status = ?,
		     ends_at = NULLIF(?, ''),
		     provider_transaction_id = COALESCE(NULLIF(?, ''), provider_transaction_id),
		     product_id = COALESCE(NULLIF(?, ''), product_id),
		     store_environment = COALESCE(NULLIF(?, ''), store_environment),
		     auto_renew = COALESCE(?, auto_renew),
		     purchase_token = COALESCE(NULLIF(?, ''), purchase_token),
		     last_verified_at = ?,
		     last_store_event_at = ?,
		     updated_at = ?
		   WHERE user_id = ?`,
		n.Plan, n.State, n.ExpiresAt, n.TransactionID, n.ProductID, n.Environment,
		n.AutoRenew, n.PurchaseToken, received, n.EventTime, received, userID); err != nil {
		return NotificationOutcome{}, fmt.Errorf("store: apply notification: %w", err)
	}

	detail := n.Detail
	if detail == "" {
		detail = "applied " + n.Provider + " " + n.NotificationType
	}
	return NotificationOutcome{Status: NotificationApplied, UserID: userID, Detail: detail, State: n.State}, nil
}

// finishNotification writes the outcome into the ledger row claimed in step 1.
func finishNotification(ctx context.Context, tx *db.Tx, n StoreNotification, outcome NotificationOutcome) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE store_notifications SET status = ?, detail = ? WHERE provider = ? AND notification_id = ?`,
		outcome.Status, outcome.Detail, n.Provider, n.NotificationID)
	if err != nil {
		return fmt.Errorf("store: record notification outcome: %w", err)
	}
	return nil
}

// validate refuses a notification that cannot be stored safely.
//
// The id and the timestamp are the two fields the guarantees rest on: without
// an id there is no idempotency, and without a timestamp there is no ordering.
// A notification missing either is rejected rather than applied with a
// fabricated value, because a fabricated timestamp would silently win or lose
// against real events.
func (n StoreNotification) validate() error {
	switch {
	case strings.TrimSpace(n.Provider) == "":
		return errors.New("store: notification has no provider")
	case strings.TrimSpace(n.NotificationID) == "":
		return errors.New("store: notification has no id - it cannot be deduplicated")
	case strings.TrimSpace(n.EventTime) == "":
		return errors.New("store: notification has no event time - it cannot be ordered")
	}
	if _, err := time.Parse(time.RFC3339, n.EventTime); err != nil {
		return fmt.Errorf("store: notification event time %q is not RFC3339: %w", n.EventTime, err)
	}
	if n.Applies && strings.TrimSpace(n.State) == "" {
		return errors.New("store: notification would change entitlement but carries no state")
	}
	if n.Applies {
		switch n.State {
		case models.SubscriptionActive, models.SubscriptionTrial, models.SubscriptionGrace,
			models.SubscriptionCancelled, models.SubscriptionExpired, models.SubscriptionRefunded,
			models.SubscriptionSuspended:
		default:
			// The column has a CHECK constraint with exactly this vocabulary;
			// failing here names the offending state instead of surfacing a
			// constraint violation from the driver.
			return fmt.Errorf("store: notification state %q is not a known subscription state", n.State)
		}
	}
	return nil
}

// StoreNotificationsFor returns the ledger entries recorded for one purchase,
// newest first. It backs the operator question "what did the store tell us?",
// which is the first thing to ask when a customer disputes a refund.
func (s *UserStore) StoreNotificationsFor(ctx context.Context, provider, originalTransactionID string) ([]StoreNotification, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT provider, notification_id, COALESCE(notification_type,''), COALESCE(subtype,''),
		        COALESCE(original_transaction_id,''), COALESCE(purchase_token,''),
		        event_time, status, COALESCE(detail,'')
		   FROM store_notifications
		  WHERE provider = ? AND (original_transaction_id = ? OR purchase_token = ?)
		  ORDER BY event_time DESC, received_at DESC`,
		provider, originalTransactionID, originalTransactionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []StoreNotification
	for rows.Next() {
		var (
			n      StoreNotification
			status string
		)
		if err := rows.Scan(&n.Provider, &n.NotificationID, &n.NotificationType, &n.Subtype,
			&n.OriginalTransactionID, &n.PurchaseToken, &n.EventTime, &status, &n.Detail); err != nil {
			return nil, err
		}
		n.Applies = status == NotificationApplied
		out = append(out, n)
	}
	return out, rows.Err()
}

// parseStoreTime parses a stored RFC3339 timestamp, reporting whether it was
// usable. An unparsable watermark is treated as absent rather than as "now":
// the alternative would be to refuse every future notification because one row
// holds a corrupt timestamp.
func parseStoreTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}
