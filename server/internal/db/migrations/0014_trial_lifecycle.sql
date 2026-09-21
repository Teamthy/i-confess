-- 0014_trial_lifecycle.sql — PHASE 35.
--
-- G-3: §36 specifies six states for a trial — ELIGIBLE, STARTED, ACTIVE,
-- EXPIRING, EXPIRED, CONVERTED — and none of them existed. The system had a
-- `trial` string on subscriptions and a GET /subscriptions/trial that derived
-- "which day is it" from users.created_at. The consequences were all real:
-- everyone was on trial forever (nothing ended it), a paid customer kept
-- seeing a trial countdown, an expiring trial never announced itself, and
-- "has this account consumed its trial?" was unanswerable, which is the
-- question an introductory offer exists to gate.
--
-- The trial becomes a row with a clock and a CHECK-constrained vocabulary —
-- the G-2 discipline, one more table converted from comment into constraint.
-- The transition graph itself lives in billing.TrialTransitions and is
-- parity-tested against the constraint below, the same contract internal/
-- content and internal/moderation keep for their state machines.

CREATE TABLE IF NOT EXISTS trials (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status       TEXT NOT NULL DEFAULT 'eligible'
                 CHECK (status IN ('eligible','started','active','expiring','expired','converted')),
    -- Timestamps are stored the way every other table in this schema stores
    -- them: UTC RFC3339, which orders lexicographically. NULL means the
    -- transition that would set it has not happened yet.
    started_at   TEXT,
    ends_at      TEXT,
    converted_at TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    -- A clock that runs backwards turns EXPIRING into never: the window
    -- predicate (ends_at - now <= 24h) would fire before the trial began.
    -- The store computes both fields together, so this guards against a
    -- hand-written row, not a product path.
    CHECK (ends_at IS NULL OR started_at IS NULL OR ends_at > started_at)
);

-- One trial per account, ever. The eligibility question must answer with one
-- read, and a second row would be a second clock the product cannot arbitrate.
CREATE UNIQUE INDEX IF NOT EXISTS trials_one_per_user ON trials (user_id);

-- The sweep's working queries are "who is started but running" and "who is
-- active/expiring and past the wire", both keyed on (status, ends_at).
CREATE INDEX IF NOT EXISTS idx_trials_status_ends ON trials (status, ends_at);
