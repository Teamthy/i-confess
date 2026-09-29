-- 0029: audio sessions, admin batch generation, and the two remaining voice
-- RBAC roles.
--
-- Session and batch items store only a hash of their text. The text itself
-- travels in the queued job payload and is never persisted here, because a
-- session may carry a user's private confession (§35).

-- ---------------------------------------------------------------- roles (§65)
-- ML engineers run datasets and training. Auditors read logs, and nothing
-- else. Both are narrower than voice_manager, which still owns rights.
INSERT INTO rbac_roles (id, name, display_name, description, permissions, is_system, created_at, updated_at)
VALUES
('system:ml_engineer', 'ml_engineer', 'ML Engineer', 'Freezes datasets, runs and cancels training, reads models and evaluations',
 '["voice:read","audio:read","voice:train","system:read","queue:read"]', 1, now(), now()),
('system:auditor', 'auditor', 'Auditor', 'Read-only access to voice rights audit logs, metrics and models',
 '["voice:read","voice:rights:read","audio:read","audit:read","system:read","system:metrics"]', 1, now(), now())
ON CONFLICT(name) DO NOTHING;

-- ---------------------------------------------------------------- sessions (§33)
CREATE TABLE IF NOT EXISTS audio_sessions (
    id            TEXT PRIMARY KEY,
    owner_user_id TEXT NOT NULL,
    voice_id      TEXT NOT NULL REFERENCES voices(id),
    title         TEXT,
    purpose       TEXT NOT NULL,
    visibility    TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','catalog')),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    deleted_at    TEXT,
    row_version   INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_audio_sessions_owner ON audio_sessions(owner_user_id, created_at);

-- A PAUSE section has no generation; every other section points at the
-- (possibly shared, cached) render that voices it.
CREATE TABLE IF NOT EXISTS audio_session_items (
    id             TEXT PRIMARY KEY,
    session_id     TEXT NOT NULL REFERENCES audio_sessions(id),
    position       INTEGER NOT NULL,
    section_type   TEXT NOT NULL CHECK (section_type IN ('INTRO','SCRIPTURE','REFLECTION','PAUSE','PRAYER','CONFESSION','ENCOURAGEMENT','TEACHING','CLOSING')),
    style          TEXT,
    generation_id  TEXT REFERENCES voice_generations(id),
    pause_ms       INTEGER NOT NULL DEFAULT 0 CHECK (pause_ms >= 0 AND pause_ms <= 120000),
    text_sha256    TEXT,
    created_at     TEXT NOT NULL,
    deleted_at     TEXT,
    row_version    INTEGER NOT NULL DEFAULT 1,
    UNIQUE (session_id, position),
    CHECK ((section_type = 'PAUSE') = (generation_id IS NULL))
);

-- ---------------------------------------------------------------- batches (§83, §84)
CREATE TABLE IF NOT EXISTS voice_batches (
    id          TEXT PRIMARY KEY,
    voice_id    TEXT NOT NULL REFERENCES voices(id),
    title       TEXT NOT NULL,
    purpose     TEXT NOT NULL,
    style       TEXT NOT NULL,
    kind        TEXT NOT NULL DEFAULT 'batch' CHECK (kind IN ('batch','pregeneration')),
    total       INTEGER NOT NULL,
    refused     INTEGER NOT NULL DEFAULT 0,
    created_by  TEXT,
    created_at  TEXT NOT NULL,
    deleted_at  TEXT,
    row_version INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_voice_batches_voice ON voice_batches(voice_id, created_at);

CREATE TABLE IF NOT EXISTS voice_batch_items (
    id            TEXT PRIMARY KEY,
    batch_id      TEXT NOT NULL REFERENCES voice_batches(id),
    position      INTEGER NOT NULL,
    label         TEXT,
    generation_id TEXT REFERENCES voice_generations(id),
    error         TEXT,
    created_at    TEXT NOT NULL,
    deleted_at    TEXT,
    row_version   INTEGER NOT NULL DEFAULT 1,
    UNIQUE (batch_id, position)
);
CREATE INDEX IF NOT EXISTS idx_voice_batch_items_gen ON voice_batch_items(generation_id);
