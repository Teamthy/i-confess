-- PHASE 15: record which variant an audio asset was rendered for.
--
-- audio_assets had no variant_id column. UpsertAsset defaulted the model's
-- VariantID to "default" and never persisted it, and every read filled the
-- field with a '' literal. engine.matchAsset's first branch compares
-- a.VariantID against the variant being packed, so it could never match: every
-- selection fell through to the "any asset for this confession" fallback.
--
-- Generation is variant-specific - it renders textForVariant(conf, variantID) -
-- so the product could not record which render belongs to which length.
--
-- Adding a column to the unique key makes the constraint looser, not stricter,
-- so rows satisfying the old key also satisfy this one and the migration cannot
-- fail on existing data.
ALTER TABLE audio_assets ADD COLUMN IF NOT EXISTS variant_id TEXT;

ALTER TABLE audio_assets
    DROP CONSTRAINT IF EXISTS audio_assets_content_id_content_version_id_voice_id_asset_t_key;
ALTER TABLE audio_assets
    ADD CONSTRAINT audio_assets_identity_key
    UNIQUE (content_id, content_version_id, voice_id, variant_id, asset_type, quality_tier);

CREATE INDEX IF NOT EXISTS idx_audio_assets_selection
    ON audio_assets (content_id, voice_id, status, created_at DESC);
