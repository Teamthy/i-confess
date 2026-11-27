-- 0036_newsletter_unsubscribe.sql — a way off the list.
--
-- Every newsletter mail carries a one-click unsubscribe link. The link holds
-- a random token; only its SHA-256 is stored, the same as verification and
-- reset tokens, so a database read does not hand out working links. The token
-- is rotated whenever the address subscribes again.

ALTER TABLE newsletter_subscriptions ADD COLUMN IF NOT EXISTS unsubscribe_token_hash TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_newsletter_unsubscribe_token
    ON newsletter_subscriptions (unsubscribe_token_hash)
    WHERE unsubscribe_token_hash IS NOT NULL;
