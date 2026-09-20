-- 0010_subscription_provider_state.sql — IC-003.
--
-- The subscriptions table was written before any store integration existed, and
-- it shows. Three things made real billing impossible to implement on top of it:
--
--   1. Nothing identified what was bought. There was no provider, no
--      transaction id, no product id and no auto-renew flag, so a renewal or a
--      refund could only be matched to a user by expiry date - ambiguous the
--      moment anyone buys twice - and a refunded purchase could not be
--      recognised at all.
--
--   2. ends_at existed since the baseline and was never written and never read.
--      Entitlement came from the status string alone, so a subscription whose
--      period had ended stayed premium until something else happened to write
--      the row. Nothing did. The column is used rather than duplicated: a
--      second expiry column beside an unused one is how a schema acquires two
--      answers to one question.
--
--   3. One user could accumulate rows. SetSubscription issued an unguarded
--      UPDATE that modified every row for that user, and inserted a new row
--      when none existed, so "the current subscription" was whatever the query
--      happened to order first. A unique index makes the upsert meaningful.

-- 1. Store identity and verification bookkeeping.
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS provider TEXT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS provider_transaction_id TEXT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS original_transaction_id TEXT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS product_id TEXT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS store_environment TEXT;
-- Nullable on purpose: NULL is "the store did not say", which is not the same
-- as "auto-renewal is switched off". A prepaid plan has no auto-renewal at all,
-- and recording that as false would make a healthy subscription look cancelled.
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS auto_renew BOOLEAN;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS last_verified_at TEXT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS updated_at TEXT;

-- 2. Widen the status vocabulary.
--
-- 0002 constrained this column to ('active','trial','expired','cancelled') and
-- said so explicitly: the vocabulary was narrow because the store lifecycle did
-- not exist yet. It does now, and two of its states cannot be expressed in the
-- old set.
--
--   grace     - the store is retrying a failed payment and entitlement must
--               continue. This is not 'active' (the payment did fail) and not
--               'expired' (access has not ended), and collapsing it into either
--               produces a wrong answer to a customer who is still paying.
--   refunded  - Apple revoked the transaction, or Play refunded it. Distinct
--               from 'expired': the period may still be running, and an
--               operator investigating a complaint needs to see which happened.
--   suspended - Play account hold, a paused subscription, or a purchase whose
--               payment has not settled. Not entitled, and not the same event
--               as an expiry: the customer has not stopped paying, they have
--               stopped being able to.
--
-- Normalise before constraining, the pattern from 0003 and 0009: a deployment
-- that wrote a value this migration does not know about must not have its
-- migration abort.
UPDATE subscriptions SET status = 'expired'
 WHERE status IS NULL OR status NOT IN ('active','trial','grace','cancelled','expired','refunded','suspended');

ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS subscriptions_status_check;
ALTER TABLE subscriptions ADD CONSTRAINT subscriptions_status_check
    CHECK (status IN ('active','trial','grace','cancelled','expired','refunded','suspended'));

-- 3. One subscription row per user.
--
-- Deduplicate first: the unique index cannot be created over data that already
-- violates it, and the rows that exist were written by an unguarded UPDATE.
-- The newest row survives, since it is the one that records the most recent
-- thing the store said; ties break on id so the migration is deterministic
-- rather than dependent on physical row order.
DELETE FROM subscriptions older
 USING subscriptions newer
 WHERE older.user_id = newer.user_id
   AND (older.created_at < newer.created_at
        OR (older.created_at = newer.created_at AND older.id < newer.id));

CREATE UNIQUE INDEX IF NOT EXISTS subscriptions_user_id_key
    ON subscriptions (user_id);

-- 4. One store subscription belongs to one account.
--
-- A receipt is a bearer token: whoever holds it can present it. Without this,
-- the same genuine purchase can be redeemed by any number of accounts, which is
-- both a revenue leak and a way to launder entitlement across banned accounts.
-- The partial index only constrains rows that actually carry an identity, so
-- administrative grants (which have none) are unaffected.
CREATE UNIQUE INDEX IF NOT EXISTS subscriptions_provider_transaction_key
    ON subscriptions (provider, original_transaction_id)
 WHERE original_transaction_id IS NOT NULL;
