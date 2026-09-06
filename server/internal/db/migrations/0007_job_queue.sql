-- 0007_job_queue.sql — PHASE 16.
--
-- The jobs table existed since the baseline but nothing wrote to it: the server
-- ran an in-memory queue, so every job still pending when the process stopped
-- was lost, and a restart is a deploy rather than an accident. This adds the
-- columns a durable, retried, parkable queue needs.
--
-- Timestamps are text in RFC3339 UTC throughout this schema. That format is
-- fixed-width and lexicographically ordered, so the claim query can compare
-- available_at directly. The empty-string default therefore means "available
-- immediately", because '' sorts before any timestamp.

-- When the job next becomes claimable. A job that failed against a provider
-- that is rate-limiting must not come straight back.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS available_at text NOT NULL DEFAULT '';

-- Who has the job claimed. Without this, a job stuck in 'running' after a
-- worker died is indistinguishable from one that is genuinely working.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS worker_id text;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS claimed_at text;

-- Why a job was parked. A dead-letter row with no reason is just a row that
-- stopped moving, and the point of parking rather than deleting is that a human
-- can look at it and decide.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS dead_letter_reason text;

-- The claim query filters on status and due-time and orders by created_at; this
-- is the index that keeps polling cheap as the table grows.
CREATE INDEX IF NOT EXISTS idx_jobs_claim ON jobs (status, available_at, created_at);

-- Stale-claim recovery scans for running jobs whose claim has gone quiet.
CREATE INDEX IF NOT EXISTS idx_jobs_running ON jobs (status, claimed_at);
