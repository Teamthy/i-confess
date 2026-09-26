-- 0018_trial_engagement.sql — PHASE 41 / master-plan 37 remainder.
--
-- The seven-day journey could be displayed but not measured. A trial whose day
-- completion is only ever computed from a clock reports "day 4 of 7" for an
-- account that never opened the app, so the conversion funnel had no
-- denominator: there was no way to distinguish a listener who finished six
-- days from one who started and never returned.
--
-- `trial_day_completions` records the fact that a real session was completed
-- on a given journey day. It is written by the server from the session
-- completion path, never asserted by a client, so a day cannot be claimed
-- without playback. One row per (user, day): the second completion on the same
-- day is a repeat, not a second day.
--
-- `analytics_events` persists the conversion and cancellation events that were
-- previously accepted by POST /analytics/batch and discarded by LogSink. An
-- event that is acknowledged with 202 and then dropped is not tracking; it is
-- a receipt for data the system does not have.
CREATE TABLE IF NOT EXISTS trial_day_completions (
    id           TEXT PRIMARY KEY,
    trial_id     TEXT NOT NULL,
    user_id      TEXT NOT NULL,
    day          INTEGER NOT NULL,
    session_id   TEXT,
    completed_at TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    deleted_at   TEXT,
    row_version  INTEGER NOT NULL DEFAULT 1,
    CONSTRAINT trial_day_completions_day_check CHECK (day BETWEEN 1 AND 7),
    CONSTRAINT trial_day_completions_user_day_unique UNIQUE (user_id, day),
    CONSTRAINT trial_day_completions_trial_fk
        FOREIGN KEY (trial_id) REFERENCES trials(id) ON DELETE CASCADE,
    CONSTRAINT trial_day_completions_user_fk
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT trial_day_completions_session_fk
        FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_trial_day_completions_trial
    ON trial_day_completions (trial_id);
CREATE INDEX IF NOT EXISTS idx_trial_day_completions_day
    ON trial_day_completions (day);

CREATE TABLE IF NOT EXISTS analytics_events (
    id          TEXT PRIMARY KEY,
    user_id     TEXT,
    name        TEXT NOT NULL,
    props       TEXT,
    occurred_at TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    deleted_at  TEXT,
    row_version INTEGER NOT NULL DEFAULT 1,
    CONSTRAINT analytics_events_user_fk
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_analytics_events_name ON analytics_events (name);
CREATE INDEX IF NOT EXISTS idx_analytics_events_user ON analytics_events (user_id, occurred_at);
