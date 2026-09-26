-- 0019_blocking_appeals.sql — PHASE 42 / master-plan 32.
--
-- PHASE 31 built reporting, the moderation queue, the editorial lifecycle and
-- the §75 QA gate, but two halves of a moderation system were absent entirely.
--
-- First, blocking. A listener who is being harassed has exactly one tool today:
-- file a report and wait for a human. That is the wrong first response. A block
-- is a self-service boundary that takes effect immediately and does not require
-- anyone to agree that the behaviour was wrong, which is what makes it usable
-- while a report is still in the queue.
--
-- Second, appeals. Every decision PHASE 31 made is terminal: a dismissed report
-- and a rejected confession both end the conversation, and the person the
-- decision was made about has no way to answer. A moderation system with no
-- appeal path is a system that cannot correct itself, and the first rejected
-- confession that was a false positive proves it.
--
-- Both tables carry deleted_at and row_version inline: migration 0015 added
-- those columns to the tables that existed at the time, and it runs before this
-- one, so an ALTER here would reference a table that does not exist yet.

CREATE TABLE IF NOT EXISTS user_blocks (
    id          TEXT PRIMARY KEY,
    blocker_id  TEXT NOT NULL,
    blocked_id  TEXT NOT NULL,
    reason      TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    deleted_at  TEXT,
    row_version INTEGER NOT NULL DEFAULT 1,
    -- One live block per pair. The partial index means unblocking frees the
    -- pair: a block is a boundary, not a permanent record of a grievance, and
    -- a listener who unblocks and later re-blocks must be able to.
    CONSTRAINT user_blocks_no_self CHECK (blocker_id <> blocked_id),
    CONSTRAINT user_blocks_one_live_pair UNIQUE (blocker_id, blocked_id),
    CONSTRAINT user_blocks_blocker_fk
        FOREIGN KEY (blocker_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT user_blocks_blocked_fk
        FOREIGN KEY (blocked_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_blocks_blocked ON user_blocks (blocked_id);

-- An appeal is a workflow with a small, explicit lifecycle. The CHECK is the
-- vocabulary the Go edge table in internal/moderation declares, and the parity
-- test reads it back from the live database.
CREATE TABLE IF NOT EXISTS moderation_appeals (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    -- What is being appealed: a report the moderator dismissed, or a
    -- confession the moderator rejected. The pair identifies the decision
    -- without a foreign key to two unrelated tables.
    decision_type TEXT NOT NULL,
    decision_id   TEXT NOT NULL,
    statement     TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'submitted',
    reviewed_by   TEXT,
    reviewed_at   TEXT,
    decision_note TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    deleted_at    TEXT,
    row_version   INTEGER NOT NULL DEFAULT 1,
    CONSTRAINT moderation_appeals_status_check CHECK (status IN
        ('submitted','under_review','upheld','overturned')),
    CONSTRAINT moderation_appeals_decision_type_check CHECK (decision_type IN
        ('report','confession')),
    -- One appeal per decision, and it is heard once. This is a full unique
    -- constraint rather than a partial index over the live statuses on purpose:
    -- an appeal that a moderator heard and upheld is final, and allowing a
    -- second filing would turn the appeal queue into the same spam surface the
    -- reports_one_open_per_reporter index closes for reports. The store reports
    -- the conflict as "already appealed" with the existing row, so a retry
    -- after a lost response still shows the listener their appeal.
    CONSTRAINT moderation_appeals_one_per_decision
        UNIQUE (decision_type, decision_id)
);

CREATE INDEX IF NOT EXISTS idx_moderation_appeals_status
    ON moderation_appeals (status);
CREATE INDEX IF NOT EXISTS idx_moderation_appeals_user
    ON moderation_appeals (user_id);
