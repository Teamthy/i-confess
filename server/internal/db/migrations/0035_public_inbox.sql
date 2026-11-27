-- 0035_public_inbox.sql — the website's two public forms.
--
-- The newsletter and contact forms on the site acknowledged submissions
-- ("Subscribed", "Message sent") and stored nothing: a receipt for data the
-- system did not have. These tables hold what they collect.
--
-- Neither is tied to an account (a visitor need not have one), so neither has
-- a user foreign key. Both carry deleted_at/row_version like every table.

CREATE TABLE IF NOT EXISTS newsletter_subscriptions (
    id              TEXT PRIMARY KEY,
    email           TEXT NOT NULL,
    source          TEXT NOT NULL DEFAULT 'web',
    unsubscribed_at TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    deleted_at      TEXT,
    row_version     INTEGER NOT NULL DEFAULT 1,
    CONSTRAINT newsletter_subscriptions_email_unique UNIQUE (email)
);

CREATE TABLE IF NOT EXISTS contact_messages (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    email       TEXT NOT NULL,
    message     TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'new',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    deleted_at  TEXT,
    row_version INTEGER NOT NULL DEFAULT 1,
    CONSTRAINT contact_messages_status_check CHECK (status IN ('new','answered','closed'))
);

CREATE INDEX IF NOT EXISTS idx_contact_messages_status ON contact_messages (status, created_at);
