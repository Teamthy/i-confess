-- 0011_store_notifications.sql — IC-003 (PR B): store notification webhooks.
--
-- PR A made the server verify a receipt the client presented. That covers the
-- happy path and nothing after it. A subscription changes for reasons the
-- client never sees and never reports: it renews at 3am, the card fails and
-- Google retries for a week, Apple refunds it after a support call, the user
-- cancels from the App Store settings screen on another device. Until the
-- server hears about those, the entitlement row is a snapshot of the moment
-- the app last launched.
--
-- Both stores push those events (App Store Server Notifications V2, Play Real
-- Time Developer Notifications). Two properties have to hold before a push
-- stream can be allowed to write entitlement, and neither is expressible in
-- the subscriptions table alone:
--
--   1. IDEMPOTENCY. Both stores retry, and both document that duplicates are
--      possible — at-least-once delivery is the contract. A refund applied
--      twice is harmless only by luck; a renewal applied twice is not. The
--      notification's own id is the key, so the second delivery of one event
--      is recognised as the same event rather than as a new one.
--
--   2. ORDERING. Retries and parallel delivery mean events arrive out of
--      order. A DID_RENEW from yesterday can land after an EXPIRED from today,
--      and applying them in arrival order leaves the row entitled for a
--      subscription that has ended. Every store event carries a timestamp, and
--      `subscriptions.last_store_event_at` is the watermark: an event older
--      than the last one applied is recorded and refused, so the final state
--      is the newest event's, whatever order the network chose.
--
-- The ledger is also the audit trail. When a customer disputes a refund, "we
-- applied REFUND at 14:02 with this notification id" is an answer; a log line
-- that has rotated away is not.

-- 1. What the store told us, whether or not it changed anything.
--
-- Rows are retained after they are applied. They are the record of why the
-- subscriptions row says what it says, and the duplicate that arrives ten
-- seconds later is recognised by this table's unique index rather than by a
-- cache that a restart would empty.
CREATE TABLE IF NOT EXISTS store_notifications (
    id                      TEXT PRIMARY KEY,
    provider                TEXT NOT NULL,          -- apple | google
    notification_id         TEXT NOT NULL,          -- the store's own id: Apple's notificationUUID, Pub/Sub's messageId
    notification_type       TEXT,                   -- Apple's string type, or Play's numeric type as text
    subtype                 TEXT,                   -- Apple's subtype (GRACE_PERIOD, AUTO_RENEW_DISABLED, ...)
    original_transaction_id TEXT,                   -- Apple's originalTransactionId, or Play's linked purchase token
    -- Play identifies a subscription by its purchase token. It is not the same
    -- value as the order id or the linked token, and it is the only key the
    -- RTDN payload carries, so it is stored in its own column and indexed: the
    -- alternative is matching a notification to an account by product id,
    -- which is ambiguous with more than one subscriber.
    purchase_token          TEXT,
    event_time              TEXT NOT NULL,          -- when the store says it happened, RFC3339 UTC
    received_at             TEXT NOT NULL,          -- when this server saw it
    -- applied     - verified, newer than the watermark, wrote the subscription
    -- duplicate   - the same notification id was already recorded
    -- stale       - verified, but older than the last event already applied
    -- unmatched   - verified, but no account has redeemed this purchase yet
    -- ignored     - verified, and could not change entitlement (a test
    --                notification, or a type this server does not act on)
    status                  TEXT NOT NULL,
    detail                  TEXT,
    CONSTRAINT store_notifications_status_check
        CHECK (status IN ('applied','duplicate','stale','unmatched','ignored'))
);

-- The idempotency gate. Delivery of the same event twice must reach this index
-- and stop there, not reach the code that writes a subscription.
CREATE UNIQUE INDEX IF NOT EXISTS store_notifications_provider_notification_key
    ON store_notifications (provider, notification_id);

-- "What has the store ever told us about this purchase?" - the refund
-- investigation query, and the reason the ledger is worth keeping.
CREATE INDEX IF NOT EXISTS store_notifications_purchase_idx
    ON store_notifications (provider, original_transaction_id);

-- 2. The ordering watermark, per subscription.
--
-- NULL means no store event has been applied to this row yet, which is the
-- case for a receipt verified through the API and for an administrative grant.
-- A NULL watermark accepts the first event at any time; after that only newer
-- events move the row.
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS last_store_event_at TEXT;

-- 3. The Play purchase token.
--
-- PR A recorded Play identity as (latestOrderId, linkedPurchaseToken), which
-- is what subscriptionsv2 returns. Neither is the purchase token the RTDN
-- payload carries, so a notification could not be matched to a row at all.
-- Storing it also closes the same hole the Apple original-transaction index
-- closes: one purchase token entitles exactly one account, so a token shared
-- between accounts cannot redeem the purchase twice.
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS purchase_token TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS subscriptions_purchase_token_key
    ON subscriptions (provider, purchase_token)
 WHERE purchase_token IS NOT NULL;
