-- 0014_trial_lifecycle.sql — PHASE 36 / G-29.
--
-- A seven-day catalogue journey is not a subscription receipt. Keeping the
-- trial state in subscriptions made it impossible to distinguish an account
-- that had never started from one that had expired, and using users.created_at
-- as the start time granted a trial to every account from its first sign-in.
-- The trial row records the six-state journey from directive §36. The
-- subscriptions row remains the entitlement projection: a start writes
-- plan=premium, status=trial, and the normal entitlement clock decides whether
-- that projection is currently live.
CREATE TABLE IF NOT EXISTS trials (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL UNIQUE,
    state        TEXT NOT NULL DEFAULT 'ELIGIBLE',
    started_at   TEXT,
    expires_at   TEXT,
    converted_at TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    CONSTRAINT trials_state_check CHECK (state IN
        ('ELIGIBLE','STARTED','ACTIVE','EXPIRING','EXPIRED','CONVERTED')),
    CONSTRAINT trials_user_fk FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_trials_state ON trials (state);
