-- PHASE 14: audio asset QA workflow.
--
-- Generated audio has always entered 'processing' (see adminGenerateAudio),
-- but there was no way out: the codebase contains no UPDATE audio_assets
-- statement at all. Status was only ever set at INSERT, so a render sat in
-- 'processing' forever and could never be served. The only escape was
-- re-posting the asset through POST /admin/audio with status "ready", which is
-- an upsert rather than a review and records neither approver nor reason.
--
-- This widens the status vocabulary to name a human rejection, and adds the
-- columns a QA decision needs. The lifecycle itself - which moves are legal,
-- and which statuses may reach a listener - lives in internal/audio/lifecycle.go,
-- which is now the authority both the CHECK and the queries read.

ALTER TABLE audio_assets DROP CONSTRAINT IF EXISTS audio_assets_status_check;
ALTER TABLE audio_assets ADD CONSTRAINT audio_assets_status_check
    CHECK (status IN ('uploading','processing','ready','published','failed','qa_rejected','archived'));

-- QA decision record. NULL means the asset has not been reviewed.
ALTER TABLE audio_assets ADD COLUMN IF NOT EXISTS qa_reviewed_by TEXT;
ALTER TABLE audio_assets ADD COLUMN IF NOT EXISTS qa_reviewed_at TEXT;
ALTER TABLE audio_assets ADD COLUMN IF NOT EXISTS qa_note        TEXT;

-- Generation jobs. The table has existed since the baseline schema, unused: no
-- code ever wrote to it, so a generation was a synchronous call inside an HTTP
-- request with no record, no status to poll and nothing to retry. These columns
-- link a job to the request that produced it and to its outcome.
ALTER TABLE audio_generation_jobs ADD COLUMN IF NOT EXISTS confession_id  TEXT;
ALTER TABLE audio_generation_jobs ADD COLUMN IF NOT EXISTS variant_id     TEXT;
ALTER TABLE audio_generation_jobs ADD COLUMN IF NOT EXISTS audio_asset_id TEXT;

CREATE INDEX IF NOT EXISTS idx_audio_generation_jobs_confession
    ON audio_generation_jobs (confession_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audio_generation_jobs_status
    ON audio_generation_jobs (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audio_assets_content_version
    ON audio_assets (content_version_id);

-- Voice rights: the columns the gate reads but the table could not store.
--
-- models.VoiceRights has carried CommercialUse, AIGenerationAllowed,
-- MarketingAllowed, ProviderVoiceID and friends since it was written, and
-- rights.Evaluate reads them: UseSynthesis is denied unless
-- AIGenerationAllowed is true, and commercial playback is denied unless
-- CommercialUse is true. But voice_rights had twelve columns and none of these
-- were among them, so Create and Update dropped the values and ByVoiceID read
-- back zero values. Every licence therefore read as forbidding AI generation
-- and forbidding commercial use, and no sequence of writes could make it
-- otherwise. The gate was not too permissive - it was unsatisfiable.
--
-- Booleans follow the convention already used in this schema (premium,
-- soft-delete flags): INTEGER 0/1.
ALTER TABLE voice_rights ADD COLUMN IF NOT EXISTS license_status        TEXT;
ALTER TABLE voice_rights ADD COLUMN IF NOT EXISTS commercial_use        INTEGER NOT NULL DEFAULT 0;
ALTER TABLE voice_rights ADD COLUMN IF NOT EXISTS ai_generation_allowed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE voice_rights ADD COLUMN IF NOT EXISTS marketing_allowed     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE voice_rights ADD COLUMN IF NOT EXISTS expiration_date       TEXT;
ALTER TABLE voice_rights ADD COLUMN IF NOT EXISTS revocation_terms      TEXT;
ALTER TABLE voice_rights ADD COLUMN IF NOT EXISTS provider              TEXT;
ALTER TABLE voice_rights ADD COLUMN IF NOT EXISTS provider_voice_id     TEXT;
ALTER TABLE voice_rights ADD COLUMN IF NOT EXISTS notes                 TEXT;
