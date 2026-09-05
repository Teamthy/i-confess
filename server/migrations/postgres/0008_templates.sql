-- 0008_templates.sql — Phase 5 Custom sessions + Templates (shareable)
-- SQLite compatible via gendb; PG uses TEXT ids (UUID)

CREATE TABLE IF NOT EXISTS user_templates (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    description   TEXT,
    category_ids  TEXT NOT NULL, -- comma-separated, e.g. "healing,peace"
    weights       TEXT,          -- JSON map category_id -> float 0..1
    voice_id      TEXT,
    voice_rules   TEXT,          -- JSON e.g. {"prefer": "voice_123"}
    ordering      TEXT,          -- JSON e.g. {"strategy": "BALANCED"}
    is_public     INTEGER NOT NULL DEFAULT 0,
    share_token   TEXT UNIQUE,   -- short random for https://iconfess.app/t/{token}
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_templates_user ON user_templates(user_id);
CREATE INDEX IF NOT EXISTS idx_templates_share ON user_templates(share_token) WHERE share_token IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_templates_public ON user_templates(is_public) WHERE is_public=1;

-- History resume: playback_progress already exists; ensure deterministic merge index
CREATE INDEX IF NOT EXISTS idx_playback_progress_device ON playback_progress(user_id, audio_asset_id, device_id);
