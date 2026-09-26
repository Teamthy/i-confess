-- 0016_audio_provenance.sql — PHASE 39 / G-34.
--
-- Canonical catalogue coverage is bootstrapped with deterministic, playable
-- audio fixtures when a deployment has not yet rendered the approved voice.
-- Labeling that provenance prevents a fixture from being mistaken for a human
-- recording or a provider render, while keeping the audio asset itself a real
-- object with a signed delivery key.
ALTER TABLE audio_assets ADD COLUMN IF NOT EXISTS audio_source TEXT NOT NULL DEFAULT 'generated';
ALTER TABLE audio_assets DROP CONSTRAINT IF EXISTS audio_assets_audio_source_check;
ALTER TABLE audio_assets ADD CONSTRAINT audio_assets_audio_source_check
    CHECK (audio_source IN ('generated','human_recorded','bootstrap_fixture'));
