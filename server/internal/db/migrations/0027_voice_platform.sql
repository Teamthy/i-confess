-- 0027_voice_platform.sql — Licensed minister voice platform (Voice Platform §4-§6, §10-§21, §49-§56).
--
-- The legacy voice_rights / voice_licenses tables stay: internal/rights still
-- gates the existing ElevenLabs pipeline on them. The tables below are the
-- granular model used by internal/voicegov. Granular capabilities live one
-- row per capability in voice_usage_permissions rather than as a JSON blob, so
-- "which voices may be trained on?" is a query and every change is a row diff.
--
-- Every status column is CHECK-constrained against the Go constants
-- (TestDatabaseVocabularyMatchesGoConstants), and every table carries the
-- deleted_at / row_version retention columns.

-- ---------------------------------------------------------------- voices
ALTER TABLE voices ADD COLUMN IF NOT EXISTS display_name        TEXT;
ALTER TABLE voices ADD COLUMN IF NOT EXISTS locale              TEXT;
ALTER TABLE voices ADD COLUMN IF NOT EXISTS accent              TEXT;
ALTER TABLE voices ADD COLUMN IF NOT EXISTS gender_presentation TEXT;
ALTER TABLE voices ADD COLUMN IF NOT EXISTS clone_enabled       INTEGER NOT NULL DEFAULT 0;
ALTER TABLE voices ADD COLUMN IF NOT EXISTS training_enabled    INTEGER NOT NULL DEFAULT 0;
ALTER TABLE voices ADD COLUMN IF NOT EXISTS generation_enabled  INTEGER NOT NULL DEFAULT 0;
ALTER TABLE voices ADD COLUMN IF NOT EXISTS synthetic           INTEGER NOT NULL DEFAULT 0;

-- ---------------------------------------------------------------- rights
CREATE TABLE IF NOT EXISTS voice_rights_grants (
    id               TEXT PRIMARY KEY,
    voice_id         TEXT NOT NULL UNIQUE REFERENCES voices(id),
    rights_holder    TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'PENDING'
                     CHECK (status IN ('PENDING','UNDER_REVIEW','APPROVED','RESTRICTED','EXPIRED','SUSPENDED','REVOKED')),
    effective_from   TEXT,
    expires_at       TEXT,
    restrictions     TEXT NOT NULL DEFAULT '',   -- comma-separated content purposes
    territories      TEXT NOT NULL DEFAULT '',   -- comma-separated ISO codes; empty = worldwide
    languages        TEXT NOT NULL DEFAULT '',   -- comma-separated BCP-47; empty = all
    post_termination TEXT NOT NULL DEFAULT 'unpublish'
                     CHECK (post_termination IN ('retain','archive','unpublish','delete')),
    grant_version    INTEGER NOT NULL DEFAULT 1,
    notes            TEXT,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    deleted_at       TEXT,
    row_version      INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_voice_rights_grants_status ON voice_rights_grants(status);
CREATE INDEX IF NOT EXISTS idx_voice_rights_grants_expiry ON voice_rights_grants(expires_at) WHERE expires_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS voice_usage_permissions (
    id            TEXT PRIMARY KEY,
    voice_id      TEXT NOT NULL REFERENCES voices(id),
    capability    TEXT NOT NULL,
    granted       INTEGER NOT NULL DEFAULT 0,
    grant_version INTEGER NOT NULL,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    deleted_at    TEXT,
    row_version   INTEGER NOT NULL DEFAULT 1,
    UNIQUE (voice_id, capability)
);

CREATE TABLE IF NOT EXISTS voice_rights_documents (
    id             TEXT PRIMARY KEY,
    voice_id       TEXT NOT NULL REFERENCES voices(id),
    document_type  TEXT NOT NULL,             -- voice_license | recording_release | amendment | termination
    title          TEXT NOT NULL,
    storage_key    TEXT NOT NULL,             -- private bucket key; never a public URL
    sha256         TEXT NOT NULL,
    signed_at      TEXT,
    effective_from TEXT,
    expires_at     TEXT,
    uploaded_by    TEXT,
    created_at     TEXT NOT NULL,
    deleted_at     TEXT,
    row_version    INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_voice_rights_documents_voice ON voice_rights_documents(voice_id);

-- Append-only: the store never issues UPDATE or DELETE against this table.
CREATE TABLE IF NOT EXISTS voice_rights_audit_logs (
    id            TEXT PRIMARY KEY,
    voice_id      TEXT NOT NULL,
    actor         TEXT NOT NULL,
    action        TEXT NOT NULL,
    model_id      TEXT,
    generation_id TEXT,
    grant_version INTEGER,
    decision      TEXT,                       -- allowed | denied | n/a
    reason        TEXT,
    detail        TEXT,
    remote_addr   TEXT,
    created_at    TEXT NOT NULL,
    deleted_at    TEXT,
    row_version   INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_voice_rights_audit_voice ON voice_rights_audit_logs(voice_id, created_at);

-- ---------------------------------------------------------------- recordings & datasets
CREATE TABLE IF NOT EXISTS voice_recordings (
    id                    TEXT PRIMARY KEY,
    voice_id              TEXT NOT NULL REFERENCES voices(id),
    source_type           TEXT NOT NULL,      -- upload | authorized_archive | studio_session
    source_url            TEXT,
    recording_title       TEXT,
    recording_date        TEXT,
    speaker               TEXT,
    rights_document_id    TEXT REFERENCES voice_rights_documents(id),
    rights_scope          TEXT,
    license_reference     TEXT,
    processing_permission INTEGER NOT NULL DEFAULT 0,
    training_allowed      INTEGER NOT NULL DEFAULT 0,
    storage_key           TEXT NOT NULL,
    sha256                TEXT NOT NULL,
    duration_ms           INTEGER,
    status                TEXT NOT NULL DEFAULT 'uploaded'
                          CHECK (status IN ('uploaded','processing','processed','rejected','failed')),
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL,
    deleted_at            TEXT,
    row_version           INTEGER NOT NULL DEFAULT 1,
    UNIQUE (voice_id, sha256)
);

CREATE TABLE IF NOT EXISTS voice_datasets (
    id              TEXT PRIMARY KEY,
    voice_id        TEXT NOT NULL REFERENCES voices(id),
    dataset_version TEXT NOT NULL,            -- dataset_v001
    manifest_key    TEXT,
    total_segments  INTEGER NOT NULL DEFAULT 0,
    usable_seconds  INTEGER NOT NULL DEFAULT 0,
    avg_quality     DOUBLE PRECISION,
    speaker_purity  DOUBLE PRECISION,
    manifest_sha256 TEXT,
    status          TEXT NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','validating','approved','frozen','rejected')),
    approved_by     TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    deleted_at      TEXT,
    row_version     INTEGER NOT NULL DEFAULT 1,
    UNIQUE (voice_id, dataset_version)
);

CREATE TABLE IF NOT EXISTS voice_dataset_segments (
    id                    TEXT PRIMARY KEY,
    dataset_id            TEXT NOT NULL REFERENCES voice_datasets(id),
    recording_id          TEXT NOT NULL REFERENCES voice_recordings(id),
    split                 TEXT NOT NULL CHECK (split IN ('train','validation','test')),
    start_ms              INTEGER NOT NULL,
    end_ms                INTEGER NOT NULL,
    audio_key             TEXT NOT NULL,
    style                 TEXT,
    raw_transcript        TEXT,
    normalized_transcript TEXT,
    verified_transcript   TEXT,
    asr_confidence        DOUBLE PRECISION,
    quality_score         DOUBLE PRECISION,
    snr_db                DOUBLE PRECISION,
    speaker_confidence    DOUBLE PRECISION,
    verified_by           TEXT,
    status                TEXT NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending','approved','rejected')),
    reject_reason         TEXT,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL,
    deleted_at            TEXT,
    row_version           INTEGER NOT NULL DEFAULT 1,
    CHECK (end_ms > start_ms)
);
CREATE INDEX IF NOT EXISTS idx_voice_dataset_segments_dataset ON voice_dataset_segments(dataset_id, split);

CREATE TABLE IF NOT EXISTS voice_references (
    id            TEXT PRIMARY KEY,
    voice_id      TEXT NOT NULL REFERENCES voices(id),
    recording_id  TEXT REFERENCES voice_recordings(id),
    style         TEXT NOT NULL,
    audio_key     TEXT NOT NULL,
    transcript    TEXT NOT NULL,
    duration_ms   INTEGER NOT NULL,
    quality_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    rights_ok     INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','retired')),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    deleted_at    TEXT,
    row_version   INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_voice_references_voice_style ON voice_references(voice_id, style);

-- ---------------------------------------------------------------- models & training
CREATE TABLE IF NOT EXISTS voice_training_runs (
    id               TEXT PRIMARY KEY,
    voice_id         TEXT NOT NULL REFERENCES voices(id),
    dataset_id       TEXT NOT NULL REFERENCES voice_datasets(id),
    dataset_version  TEXT NOT NULL,
    engine           TEXT NOT NULL,
    base_model       TEXT NOT NULL,
    hyperparameters  TEXT NOT NULL DEFAULT '{}',
    hardware         TEXT,
    duration_seconds INTEGER,
    final_loss       DOUBLE PRECISION,
    checkpoint_key   TEXT,
    checkpoint_sha256 TEXT,
    evaluation_score DOUBLE PRECISION,
    status           TEXT NOT NULL DEFAULT 'QUEUED'
                     CHECK (status IN ('QUEUED','RUNNING','FAILED','COMPLETED','EVALUATING','APPROVED','REJECTED','ARCHIVED')),
    requested_by     TEXT,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    deleted_at       TEXT,
    row_version      INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS voice_models (
    id                TEXT PRIMARY KEY,
    voice_id          TEXT NOT NULL REFERENCES voices(id),
    engine            TEXT NOT NULL,
    engine_version    TEXT NOT NULL,
    model_name        TEXT NOT NULL,
    model_version     TEXT NOT NULL,
    mode              TEXT NOT NULL DEFAULT 'zero_shot' CHECK (mode IN ('zero_shot','fine_tuned')),
    checkpoint_key    TEXT,                    -- private; never serialised to clients
    checkpoint_sha256 TEXT,
    dataset_version   TEXT,
    training_run_id   TEXT REFERENCES voice_training_runs(id),
    language          TEXT NOT NULL,
    locale            TEXT,
    accent            TEXT,
    speaker_handle    TEXT,                    -- opaque worker handle; embeddings stay on the worker
    style_support     TEXT NOT NULL DEFAULT '',
    icf_voice_score   DOUBLE PRECISION NOT NULL DEFAULT 0,
    fallback_approved INTEGER NOT NULL DEFAULT 0,
    license           TEXT,
    license_url       TEXT,
    license_reviewed  INTEGER NOT NULL DEFAULT 0,
    status            TEXT NOT NULL DEFAULT 'candidate'
                      CHECK (status IN ('candidate','evaluating','approved','production','retired','rejected')),
    approved_by       TEXT,
    promoted_at       TEXT,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    deleted_at        TEXT,
    row_version       INTEGER NOT NULL DEFAULT 1,
    UNIQUE (voice_id, model_version)
);
-- At most one production model per voice, enforced by the database.
CREATE UNIQUE INDEX IF NOT EXISTS uq_voice_models_one_production
    ON voice_models(voice_id) WHERE status = 'production' AND deleted_at IS NULL;

-- ---------------------------------------------------------------- evaluation
CREATE TABLE IF NOT EXISTS voice_evaluations (
    id              TEXT PRIMARY KEY,
    voice_id        TEXT NOT NULL REFERENCES voices(id),
    model_id        TEXT NOT NULL REFERENCES voice_models(id),
    golden_set      TEXT NOT NULL,
    metrics         TEXT NOT NULL,             -- JSON Metrics
    weights         TEXT NOT NULL,             -- JSON Weights used
    icf_voice_score DOUBLE PRECISION NOT NULL,
    coverage        DOUBLE PRECISION NOT NULL,
    verdict         TEXT NOT NULL CHECK (verdict IN ('PASS','FAIL','NEEDS_REVIEW')),
    report          TEXT,                      -- JSON gate result
    created_by      TEXT,
    created_at      TEXT NOT NULL,
    deleted_at      TEXT,
    row_version     INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS voice_blind_tests (
    id          TEXT PRIMARY KEY,
    voice_id    TEXT NOT NULL REFERENCES voices(id),
    title       TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed')),
    created_by  TEXT,
    created_at  TEXT NOT NULL,
    closed_at   TEXT,
    deleted_at  TEXT,
    row_version INTEGER NOT NULL DEFAULT 1
);

-- source is the secret ("REAL" or a model id); it is never returned while the
-- test is open.
CREATE TABLE IF NOT EXISTS voice_blind_clips (
    id          TEXT PRIMARY KEY,
    test_id     TEXT NOT NULL REFERENCES voice_blind_tests(id),
    blind_label TEXT NOT NULL,
    source      TEXT NOT NULL,
    audio_key   TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    deleted_at  TEXT,
    row_version INTEGER NOT NULL DEFAULT 1,
    UNIQUE (test_id, blind_label)
);

CREATE TABLE IF NOT EXISTS voice_blind_ratings (
    id           TEXT PRIMARY KEY,
    test_id      TEXT NOT NULL REFERENCES voice_blind_tests(id),
    clip_id      TEXT NOT NULL REFERENCES voice_blind_clips(id),
    evaluator_id TEXT NOT NULL,
    dimension    TEXT NOT NULL,
    rating       DOUBLE PRECISION NOT NULL CHECK (rating >= 1 AND rating <= 5),
    comments     TEXT,
    created_at   TEXT NOT NULL,
    deleted_at   TEXT,
    row_version  INTEGER NOT NULL DEFAULT 1,
    UNIQUE (clip_id, evaluator_id, dimension)
);

-- ---------------------------------------------------------------- language
CREATE TABLE IF NOT EXISTS pronunciation_dictionary (
    id                 TEXT PRIMARY KEY,
    term               TEXT NOT NULL,
    locale             TEXT NOT NULL DEFAULT '',  -- '' = all; 'en-NG' and 'en-NG-PIDGIN' are distinct
    respelling         TEXT,
    ipa                TEXT,
    aliases            TEXT NOT NULL DEFAULT '',
    provider_overrides TEXT NOT NULL DEFAULT '{}',
    category           TEXT,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL,
    deleted_at         TEXT,
    row_version        INTEGER NOT NULL DEFAULT 1,
    UNIQUE (term, locale)
);

-- ---------------------------------------------------------------- generations
-- One row per synthetic render (§29, §54, §55). content_hash is the cache key:
-- identical requests resolve to the same row and are never re-rendered.
CREATE TABLE IF NOT EXISTS voice_generations (
    id              TEXT PRIMARY KEY,
    content_hash    TEXT NOT NULL UNIQUE,
    voice_id        TEXT NOT NULL REFERENCES voices(id),
    model_id        TEXT,
    engine          TEXT,
    engine_version  TEXT,
    purpose         TEXT NOT NULL,
    style           TEXT NOT NULL,
    language        TEXT NOT NULL,
    locale          TEXT,
    text_sha256     TEXT NOT NULL,
    audio_sha256    TEXT,
    storage_key     TEXT,
    duration_ms     INTEGER,
    synthetic       INTEGER NOT NULL DEFAULT 1 CHECK (synthetic = 1),
    fell_back       INTEGER NOT NULL DEFAULT 0,
    fallback_reason TEXT,
    grant_version   INTEGER,
    queue           TEXT NOT NULL,
    job_id          TEXT,
    owner_user_id   TEXT,                       -- set for private (e.g. confession) renders
    visibility      TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','catalog')),
    status          TEXT NOT NULL DEFAULT 'QUEUED'
                    CHECK (status IN ('QUEUED','PROCESSING','GENERATED','POST_PROCESSING','UPLOADING','COMPLETED','FAILED','CANCELLED')),
    error_class     TEXT,
    error_message   TEXT,
    requested_by    TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    deleted_at      TEXT,
    row_version     INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_voice_generations_voice ON voice_generations(voice_id, status);
CREATE INDEX IF NOT EXISTS idx_voice_generations_owner ON voice_generations(owner_user_id) WHERE owner_user_id IS NOT NULL;
