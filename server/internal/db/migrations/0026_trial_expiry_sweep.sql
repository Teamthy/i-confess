-- 0026_trial_expiry_sweep.sql — PHASE 44 / G-49.
--
-- The periodic trial expiry sweep scans only active/expiring rows whose clock
-- has elapsed. Keep that work on a partial due-date index rather than walking
-- the full trial history on every scheduler tick.
CREATE INDEX IF NOT EXISTS idx_trials_due_expiry
    ON trials (expires_at, user_id)
    WHERE state IN ('ACTIVE', 'EXPIRING');
