-- 0034_trial_day_sessions.sql — G-48: the trial journey builds its sessions.
--
-- The seven-day journey declared what each day wants (two sessions on Day 2,
-- a Premium voice on Day 5, the builder on Day 6) but nothing built those
-- sessions; every day fell through to the generic builder. POST
-- /subscriptions/trial/today now builds the day's sessions when the listener
-- asks for them, and this table is what makes that safe to retry.
--
-- One row per (user, day, slot). Day 2 has slots 1 and 2 (morning, evening);
-- every other building day has slot 1. The row is written as a *claim* before
-- the session is built (session_id NULL) and completed with the session id
-- afterwards, so a double tap or a network retry returns the same sessions
-- instead of building a second set, and a concurrent duplicate request sees
-- the claim and backs off. A failed build releases its claim.
CREATE TABLE IF NOT EXISTS trial_day_sessions (
    id          TEXT PRIMARY KEY,
    trial_id    TEXT NOT NULL,
    user_id     TEXT NOT NULL,
    day         INTEGER NOT NULL,
    slot        INTEGER NOT NULL,
    session_id  TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    deleted_at  TEXT,
    row_version INTEGER NOT NULL DEFAULT 1,
    CONSTRAINT trial_day_sessions_day_check CHECK (day BETWEEN 1 AND 7),
    CONSTRAINT trial_day_sessions_slot_check CHECK (slot BETWEEN 1 AND 4),
    CONSTRAINT trial_day_sessions_user_day_slot_unique UNIQUE (user_id, day, slot),
    CONSTRAINT trial_day_sessions_trial_fk
        FOREIGN KEY (trial_id) REFERENCES trials(id) ON DELETE CASCADE,
    CONSTRAINT trial_day_sessions_user_fk
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT trial_day_sessions_session_fk
        FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_trial_day_sessions_trial
    ON trial_day_sessions (trial_id);
