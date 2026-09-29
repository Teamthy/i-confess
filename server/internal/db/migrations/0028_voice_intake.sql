-- 0028: recording intake and training orchestration.
--
-- Segments produced by ingestion live in a per-voice intake pool
-- (dataset_id IS NULL) until a human verifies them. Freezing a dataset copies
-- approved pool segments into an immutable, versioned dataset, so later edits
-- to the pool can never alter a dataset that a model was trained on.

ALTER TABLE voice_dataset_segments ALTER COLUMN dataset_id DROP NOT NULL;
ALTER TABLE voice_dataset_segments ADD COLUMN IF NOT EXISTS voice_id TEXT REFERENCES voices(id);
ALTER TABLE voice_dataset_segments ADD COLUMN IF NOT EXISTS screen_flags TEXT;
ALTER TABLE voice_dataset_segments ADD COLUMN IF NOT EXISTS source_segment_id TEXT;
ALTER TABLE voice_dataset_segments ALTER COLUMN split DROP NOT NULL;
ALTER TABLE voice_dataset_segments DROP CONSTRAINT IF EXISTS voice_dataset_segments_split_check;
ALTER TABLE voice_dataset_segments ADD CONSTRAINT voice_dataset_segments_split_check
    CHECK (split IS NULL OR split IN ('train','validation','test'));
-- A frozen dataset segment must have a split and a verified transcript.
ALTER TABLE voice_dataset_segments ADD CONSTRAINT voice_dataset_segments_frozen_check
    CHECK (dataset_id IS NULL OR (split IS NOT NULL AND verified_transcript IS NOT NULL));
CREATE INDEX IF NOT EXISTS idx_voice_dataset_segments_pool
    ON voice_dataset_segments(voice_id, status) WHERE dataset_id IS NULL;

ALTER TABLE voice_recordings ADD COLUMN IF NOT EXISTS language TEXT;
ALTER TABLE voice_recordings ADD COLUMN IF NOT EXISTS ingest_report TEXT;
ALTER TABLE voice_recordings ADD COLUMN IF NOT EXISTS error_message TEXT;
ALTER TABLE voice_recordings ADD COLUMN IF NOT EXISTS job_id TEXT;
ALTER TABLE voice_recordings ADD COLUMN IF NOT EXISTS uploaded_by TEXT;

ALTER TABLE voice_datasets ADD COLUMN IF NOT EXISTS grant_version INTEGER;

ALTER TABLE voice_training_runs ADD COLUMN IF NOT EXISTS mode TEXT NOT NULL DEFAULT 'fine_tune';
ALTER TABLE voice_training_runs ADD COLUMN IF NOT EXISTS justification TEXT;
ALTER TABLE voice_training_runs ADD COLUMN IF NOT EXISTS baseline_score DOUBLE PRECISION;
ALTER TABLE voice_training_runs ADD COLUMN IF NOT EXISTS progress DOUBLE PRECISION;
ALTER TABLE voice_training_runs ADD COLUMN IF NOT EXISTS error_message TEXT;
ALTER TABLE voice_training_runs ADD COLUMN IF NOT EXISTS job_id TEXT;
ALTER TABLE voice_training_runs ADD COLUMN IF NOT EXISTS grant_version INTEGER;
ALTER TABLE voice_training_runs ADD COLUMN IF NOT EXISTS model_id TEXT;
