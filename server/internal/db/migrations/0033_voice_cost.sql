-- 0033: voice cost telemetry (audit VE-017).
--
-- The GPU worker spent seconds and nobody counted them. There was no
-- cost-per-render, no cost-per-voice, and no ceiling anywhere between an admin
-- clicking "generate 10,000 items" and the invoice - the only controls were a
-- per-user rate limit and the content-hash cache. These columns make spend
-- measurable; the batch budget (see internal/api/voice_cost.go) makes it
-- bounded.
--
-- inference_seconds is what the model was busy for; worker_seconds is the whole
-- request including mastering and WAV encoding. Both are nullable on purpose:
-- NULL means "this worker did not report", which is not the same fact as 0, and
-- a column that conflated them would make an un-instrumented engine look free.
--
-- cost_usd_micros is an *allocation*, not a bill: the deployed host's rate
-- spread over the renders it served, from VOICE_GPU_SECOND_COST_USD. It is
-- stored rather than derived so that changing the price later cannot quietly
-- rewrite what an operator was told a batch would cost at the time. Integer
-- micros because money in a float column is how rounding arguments start.
ALTER TABLE voice_generations ADD COLUMN IF NOT EXISTS inference_seconds DOUBLE PRECISION;
ALTER TABLE voice_generations ADD COLUMN IF NOT EXISTS worker_seconds DOUBLE PRECISION;
ALTER TABLE voice_generations ADD COLUMN IF NOT EXISTS cost_usd_micros BIGINT;

-- The planned spend of a batch, recorded when it is queued: the ceiling is only
-- auditable after the fact if the number the gate was given survives. basis
-- says whether that estimate came from measured renders or from the documented
-- real-time fallback, so a bad guess can be traced to its kind.
ALTER TABLE voice_batches ADD COLUMN IF NOT EXISTS est_inference_seconds DOUBLE PRECISION;
ALTER TABLE voice_batches ADD COLUMN IF NOT EXISTS est_cost_usd_micros BIGINT;
ALTER TABLE voice_batches ADD COLUMN IF NOT EXISTS estimate_basis TEXT;

-- Daily spend per voice is summed over the day's generations, and the existing
-- index is (voice_id, status) - which cannot serve a created_at range scan.
CREATE INDEX IF NOT EXISTS idx_voice_generations_voice_created
    ON voice_generations(voice_id, created_at);
