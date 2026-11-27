-- 0037_journal_memory.sql — private study data that only lived in a browser.
--
-- The web app's journal and Scripture-memory pages kept everything in
-- localStorage: a new device, a cleared browser or another app showed nothing,
-- and an account export or deletion could not include what the server never
-- had. Both are private to their owner and erased with the account.

CREATE TABLE IF NOT EXISTS journal_entries (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    body        TEXT NOT NULL,
    mood        TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    deleted_at  TEXT,
    row_version INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_journal_entries_user ON journal_entries (user_id, created_at DESC) WHERE deleted_at IS NULL;

-- A verse in spaced practice. box is the Leitner box (0..5); due_at is when it
-- next comes back. One row per reference per user.
CREATE TABLE IF NOT EXISTS memory_verses (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reference   TEXT NOT NULL,
    text        TEXT NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    box         INTEGER NOT NULL DEFAULT 0,
    due_at      TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    deleted_at  TEXT,
    row_version INTEGER NOT NULL DEFAULT 1,
    CONSTRAINT memory_verses_box_check CHECK (box BETWEEN 0 AND 5),
    CONSTRAINT memory_verses_user_reference_unique UNIQUE (user_id, reference)
);
CREATE INDEX IF NOT EXISTS idx_memory_verses_due ON memory_verses (user_id, due_at) WHERE deleted_at IS NULL;

-- Web playback and display settings (speed, EQ, ambient sound, quiet hours…)
-- have no column of their own. They are stored as one bounded JSON object so
-- they follow the account to another browser instead of resetting.
ALTER TABLE user_preferences ADD COLUMN IF NOT EXISTS client_settings TEXT NOT NULL DEFAULT '{}';
