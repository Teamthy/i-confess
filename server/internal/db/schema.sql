-- i-confess â€” SQLite (dev/local) schema
-- Canonical production schema is PostgreSQL (migrations/postgres/0001_schema.sql)
-- The two are kept structurally equivalent (TEXT ids = UUID, RFC3339 TEXT = TIMESTAMPTZ)

PRAGMA foreign_keys = ON;

-- ============================= CONTENT =============================
CREATE TABLE IF NOT EXISTS collections (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT,
    premium     INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'draft',          -- draft | published | archived
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS categories (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT,
    icon        TEXT,
    premium     INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'draft',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

-- A category may belong to many collections (e.g. the "28" and "38" share categories).
CREATE TABLE IF NOT EXISTS collection_categories (
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    category_id   TEXT NOT NULL REFERENCES categories(id)  ON DELETE CASCADE,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (collection_id, category_id)
);

CREATE TABLE IF NOT EXISTS confessions (
    id          TEXT PRIMARY KEY,
    category_id TEXT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    short_text  TEXT,
    medium_text TEXT,
    long_text   TEXT,
    description TEXT,
    tags        TEXT,                                    -- comma-separated
    intensity   INTEGER NOT NULL DEFAULT 1,              -- 1..5
    language    TEXT NOT NULL DEFAULT 'en',
    status      TEXT NOT NULL DEFAULT 'draft',
    -- lifecycle: draft|content_review|theological_review|audio_production|audio_qa|approved|published|archived
    author        TEXT,
    version       INTEGER NOT NULL DEFAULT 1,
    published_at  TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS confession_variants (
    id               TEXT PRIMARY KEY,
    confession_id    TEXT NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    label            TEXT NOT NULL,                      -- '30s' | '1m' | '3m' | '5m' | '10m'
    duration_seconds INTEGER NOT NULL,
    sort_order       INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS scripture_references (
    id              TEXT PRIMARY KEY,
    confession_id   TEXT NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    book            TEXT NOT NULL,
    chapter         INTEGER,
    verse           TEXT,                                -- e.g. '5' or '3-6'
    translation     TEXT NOT NULL DEFAULT 'KJV',
    is_direct_quote INTEGER NOT NULL DEFAULT 0,          -- 1 = verbatim quotation, 0 = paraphrase inspired by scripture
    notes           TEXT,
    sort_order      INTEGER NOT NULL DEFAULT 0
);

-- ============================= AUDIO =============================
CREATE TABLE IF NOT EXISTS voices (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT,
    type        TEXT NOT NULL DEFAULT 'professional',    -- professional | minister | generic
    provider    TEXT,
    gender      TEXT,                                    -- male | female | neutral
    language    TEXT NOT NULL DEFAULT 'en',
    premium     INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'active',
    sample_url  TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS voice_licenses (
    id                  TEXT PRIMARY KEY,
    voice_id            TEXT NOT NULL UNIQUE REFERENCES voices(id) ON DELETE CASCADE,
    owner               TEXT,
    owner_name          TEXT,
    provider            TEXT,
    provider_voice_id   TEXT,
    license_status      TEXT NOT NULL DEFAULT 'none',       -- none | pending | active | expired | revoked
    license_start       TEXT,
    license_expiry      TEXT,
    allowed_regions     TEXT,
    territories         TEXT,                                -- comma-separated ISO codes
    languages           TEXT,                                -- comma-separated language codes
    commercial_usage    INTEGER NOT NULL DEFAULT 0,
    commercial_use      INTEGER NOT NULL DEFAULT 0,
    ai_generation_allowed INTEGER NOT NULL DEFAULT 0,
    marketing_allowed   INTEGER NOT NULL DEFAULT 0,
    revocation_terms    TEXT,
    notes               TEXT,
    created_at          TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Content versions for proper audio/content separation
CREATE TABLE IF NOT EXISTS content_versions (
    id              TEXT PRIMARY KEY,
    confession_id   TEXT NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    version_number  INTEGER NOT NULL,
    title           TEXT NOT NULL,
    short_text      TEXT,
    medium_text     TEXT,
    long_text       TEXT,
    language        TEXT NOT NULL DEFAULT 'en',
    status          TEXT NOT NULL DEFAULT 'draft',      -- draft | approved | published | archived
    approved_by     TEXT,
    approved_at     TEXT,
    published_at    TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    UNIQUE(confession_id, version_number)
);

-- Enhanced audio assets with complete metadata
CREATE TABLE IF NOT EXISTS audio_assets (
    id                      TEXT PRIMARY KEY,
    content_id              TEXT NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    content_version_id      TEXT REFERENCES content_versions(id) ON DELETE SET NULL,
    voice_id                TEXT NOT NULL REFERENCES voices(id) ON DELETE RESTRICT,
    
    -- Asset classification
    asset_type              TEXT NOT NULL DEFAULT 'stream',     -- source | master | stream | preview | download
    quality_tier            TEXT NOT NULL DEFAULT 'standard',   -- standard | high | lossless
    
    -- Storage
    storage_provider        TEXT NOT NULL DEFAULT 's3',         -- s3 | gcs | azure | local
    storage_key             TEXT NOT NULL,                      -- deterministic path
    cdn_path                TEXT,                               -- CDN-served path
    
    -- Format & codec
    format                  TEXT NOT NULL DEFAULT 'm4a',        -- m4a | mp3 | wav | flac | ogg
    codec                   TEXT NOT NULL DEFAULT 'aac',        -- aac | mp3 | pcm | flac | opus
    container               TEXT NOT NULL DEFAULT 'mp4',        -- mp4 | mpeg | wav | ogg
    
    -- Audio properties
    duration_seconds        INTEGER NOT NULL,
    file_size_bytes         INTEGER NOT NULL,
    sample_rate             INTEGER,                           -- Hz (44100, 48000, etc)
    bitrate                 INTEGER,                           -- kbps
    channels                INTEGER NOT NULL DEFAULT 2,         -- 1 (mono) | 2 (stereo)
    
    -- Audio quality metrics
    loudness_lufs           REAL,                              -- ITU-R BS.1770-4
    peak_db                 REAL,
    
    -- Integrity
    checksum_sha256         TEXT UNIQUE,
    
    -- Lifecycle
    status                  TEXT NOT NULL DEFAULT 'uploading',  -- uploading | processing | ready | published | failed | archived
    published_at            TEXT,
    archived_at             TEXT,
    
    created_at              TEXT NOT NULL,
    updated_at              TEXT NOT NULL,
    
    UNIQUE(content_id, content_version_id, voice_id, asset_type, quality_tier)
);

-- Audio generation jobs for TTS/recording
CREATE TABLE IF NOT EXISTS audio_generation_jobs (
    id                  TEXT PRIMARY KEY,
    content_version_id  TEXT NOT NULL REFERENCES content_versions(id) ON DELETE CASCADE,
    voice_id            TEXT NOT NULL REFERENCES voices(id) ON DELETE RESTRICT,
    
    -- Provider & configuration
    provider            TEXT NOT NULL,                       -- google-tts | azure-tts | openai | human-recording | aws-polly
    provider_job_id     TEXT,
    
    -- Quality & format
    quality_tier        TEXT NOT NULL DEFAULT 'standard',
    format              TEXT NOT NULL DEFAULT 'm4a',
    
    -- Execution tracking
    status              TEXT NOT NULL DEFAULT 'queued',      -- queued | processing | succeeded | failed | cancelled
    
    attempt_count       INTEGER NOT NULL DEFAULT 0,
    max_attempts        INTEGER NOT NULL DEFAULT 3,
    
    requested_by        TEXT,
    started_at          TEXT,
    completed_at        TEXT,
    
    -- Error handling
    error_code          TEXT,
    error_message       TEXT,
    
    -- Idempotency
    idempotency_key     TEXT UNIQUE,
    
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

-- Audio quality variants (standard, high, lossless)
CREATE TABLE IF NOT EXISTS audio_variants (
    id              TEXT PRIMARY KEY,
    audio_asset_id  TEXT NOT NULL REFERENCES audio_assets(id) ON DELETE CASCADE,
    
    quality_tier    TEXT NOT NULL,                        -- standard | high | lossless
    bitrate         INTEGER,
    file_size_bytes INTEGER,
    
    storage_key     TEXT NOT NULL,
    cdn_path        TEXT,
    
    checksum_sha256 TEXT UNIQUE,
    
    status          TEXT NOT NULL DEFAULT 'processing',   -- processing | ready | failed
    
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    
    UNIQUE(audio_asset_id, quality_tier)
);

-- Audio processing pipeline logs
CREATE TABLE IF NOT EXISTS audio_processing_logs (
    id              TEXT PRIMARY KEY,
    audio_asset_id  TEXT NOT NULL REFERENCES audio_assets(id) ON DELETE CASCADE,
    
    step            TEXT NOT NULL,                        -- validation | normalization | transcoding | publishing
    status          TEXT NOT NULL,                        -- started | completed | failed
    
    details         TEXT,                                 -- JSON details
    error_message   TEXT,
    
    duration_ms     INTEGER,
    
    created_at      TEXT NOT NULL
);

-- Audio checksums for integrity verification
CREATE TABLE IF NOT EXISTS audio_checksums (
    id              TEXT PRIMARY KEY,
    audio_asset_id  TEXT NOT NULL UNIQUE REFERENCES audio_assets(id) ON DELETE CASCADE,
    
    sha256          TEXT NOT NULL UNIQUE,
    md5             TEXT,
    crc32           TEXT,
    
    validated_at    TEXT NOT NULL,
    created_at      TEXT NOT NULL
);

-- Voice rights/authorization metadata
CREATE TABLE IF NOT EXISTS voice_rights (
    id                      TEXT PRIMARY KEY,
    voice_id                TEXT NOT NULL REFERENCES voices(id) ON DELETE CASCADE,
    
    rights_holder           TEXT NOT NULL,
    authorization_reference TEXT,
    
    allowed_use             TEXT NOT NULL,                -- tts | recording | streaming | commercial
    territories             TEXT NOT NULL DEFAULT 'GLOBAL', -- comma-separated ISO codes or GLOBAL
    
    start_date              TEXT,
    expiry_date             TEXT,
    status                  TEXT NOT NULL DEFAULT 'active',  -- pending | active | expired | revoked
    
    metadata                TEXT,                         -- JSON
    
    created_at              TEXT NOT NULL,
    updated_at              TEXT NOT NULL,
    
    UNIQUE(voice_id, rights_holder, allowed_use)
);

-- Signed URL tracking & caching
CREATE TABLE IF NOT EXISTS signed_urls (
    id              TEXT PRIMARY KEY,
    audio_asset_id  TEXT NOT NULL REFERENCES audio_assets(id) ON DELETE CASCADE,
    user_id         TEXT REFERENCES users(id) ON DELETE SET NULL,
    
    signed_url      TEXT NOT NULL UNIQUE,
    
    expires_at      TEXT NOT NULL,
    accessed_at     TEXT,
    access_count    INTEGER NOT NULL DEFAULT 0,
    
    created_at      TEXT NOT NULL
);

-- Detailed playback session tracking
CREATE TABLE IF NOT EXISTS audio_playback_sessions (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    audio_asset_id  TEXT NOT NULL REFERENCES audio_assets(id) ON DELETE CASCADE,
    session_id      TEXT REFERENCES sessions(id) ON DELETE SET NULL,
    
    started_at      TEXT NOT NULL,
    paused_at       TEXT,
    resumed_at      TEXT,
    completed_at    TEXT,
    
    status          TEXT NOT NULL DEFAULT 'playing',     -- playing | paused | completed | abandoned
    
    position_seconds INTEGER NOT NULL DEFAULT 0,
    duration_seconds INTEGER NOT NULL,
    
    device_id       TEXT,
    platform        TEXT,
    
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

-- Audio download management
CREATE TABLE IF NOT EXISTS audio_downloads (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    audio_asset_id  TEXT NOT NULL REFERENCES audio_assets(id) ON DELETE CASCADE,
    
    status          TEXT NOT NULL DEFAULT 'queued',      -- queued | downloading | downloaded | failed | removed
    
    downloaded_at   TEXT,
    removed_at      TEXT,
    expires_at      TEXT,                               -- License expiry for downloaded content
    
    local_path      TEXT,                               -- Platform-specific reference
    file_size_bytes INTEGER,
    
    retry_count     INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT,
    
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    
    UNIQUE(user_id, audio_asset_id)
);

-- Detailed playback progress tracking
CREATE TABLE IF NOT EXISTS playback_progress (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    audio_asset_id  TEXT NOT NULL REFERENCES audio_assets(id) ON DELETE CASCADE,
    
    position_seconds INTEGER NOT NULL,
    duration_seconds INTEGER NOT NULL,
    
    completed       INTEGER NOT NULL DEFAULT 0,
    
    device_id       TEXT,
    platform        TEXT,
    
    last_updated_at TEXT NOT NULL,
    
    created_at      TEXT NOT NULL,
    
    UNIQUE(user_id, audio_asset_id, device_id)
);

-- Playback analytics events
CREATE TABLE IF NOT EXISTS audio_events (
    id              TEXT PRIMARY KEY,
    user_id         TEXT REFERENCES users(id) ON DELETE SET NULL,
    audio_asset_id  TEXT REFERENCES audio_assets(id) ON DELETE SET NULL,
    
    event_type      TEXT NOT NULL,                      -- play_request | play_started | play_paused | play_resumed | play_seeked | play_completed | play_failed | download_started | download_completed | download_failed
    
    position_seconds INTEGER,
    duration_seconds INTEGER,
    
    device_id       TEXT,
    platform        TEXT,                              -- iOS | Android | Web
    app_version     TEXT,
    
    network_type    TEXT,                              -- wifi | cellular | unknown
    bandwidth_mbps  REAL,
    
    error_code      TEXT,
    error_message   TEXT,
    
    metadata        TEXT,                              -- JSON for extensibility
    
    created_at      TEXT NOT NULL
);

-- QoE (Quality of Experience) metrics
CREATE TABLE IF NOT EXISTS audio_qoe_metrics (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    audio_asset_id  TEXT NOT NULL REFERENCES audio_assets(id) ON DELETE CASCADE,
    
    startup_latency_ms  INTEGER,
    buffer_duration_ms  INTEGER,
    rebuffer_count      INTEGER NOT NULL DEFAULT 0,
    rebuffer_duration_ms INTEGER,
    
    completion_rate     REAL,                           -- 0.0 to 1.0
    failure_rate        REAL,
    
    quality_experienced TEXT,                           -- measured vs. requested
    
    created_at      TEXT NOT NULL,
    
    UNIQUE(user_id, audio_asset_id)
);

-- ============================= USERS =============================
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT,
    timezone      TEXT NOT NULL DEFAULT 'UTC',
    status        TEXT NOT NULL DEFAULT 'active',
    email_verified INTEGER NOT NULL DEFAULT 0,
    mfa_enabled   INTEGER NOT NULL DEFAULT 0,
    last_login_at TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_profiles (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    display_name  TEXT,
    username      TEXT,
    bio           TEXT,
    avatar_url    TEXT,
    cover_url     TEXT,
    timezone      TEXT NOT NULL DEFAULT 'UTC',
    locale        TEXT NOT NULL DEFAULT 'en-US',
    country_code  TEXT,
    language      TEXT NOT NULL DEFAULT 'en',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_preferences (
    id                       TEXT PRIMARY KEY,
    user_id                  TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    default_duration         INTEGER NOT NULL DEFAULT 1800,
    default_voice_id         TEXT,
    autoplay                 INTEGER NOT NULL DEFAULT 1,
    preferred_quality        TEXT NOT NULL DEFAULT 'standard',
    download_over_wifi       INTEGER NOT NULL DEFAULT 1,
    notifications_enabled    INTEGER NOT NULL DEFAULT 1,
    recommendations_enabled  INTEGER NOT NULL DEFAULT 1,
    personalization_enabled  INTEGER NOT NULL DEFAULT 1,
    language                 TEXT NOT NULL DEFAULT 'en',
    theme                    TEXT NOT NULL DEFAULT 'system',
    updated_at               TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_interests (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id TEXT NOT NULL,
    weight      REAL NOT NULL DEFAULT 0,
    source      TEXT NOT NULL DEFAULT 'EXPLICIT_SELECTION',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    UNIQUE(user_id, category_id, source)
);

CREATE TABLE IF NOT EXISTS user_voice_preferences (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    voice_id    TEXT NOT NULL,
    is_default  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    UNIQUE(user_id, voice_id)
);

CREATE TABLE IF NOT EXISTS user_identities (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider      TEXT NOT NULL,
    subject       TEXT NOT NULL,
    email         TEXT,
    email_verified INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL,
    UNIQUE(provider, subject),
    UNIQUE(user_id, provider)
);

CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose     TEXT NOT NULL DEFAULT 'email_verification',
    token       TEXT NOT NULL,
    expires_at  TEXT NOT NULL,
    used_at     TEXT,
    created_at  TEXT NOT NULL,
    UNIQUE(user_id, purpose, token)
);

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       TEXT NOT NULL,
    expires_at  TEXT NOT NULL,
    used_at     TEXT,
    created_at  TEXT NOT NULL,
    UNIQUE(user_id, token)
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash    TEXT NOT NULL UNIQUE,
    device_id     TEXT,
    device_name   TEXT,
    platform      TEXT,
    user_agent    TEXT,
    ip_address    TEXT,
    revoked_at    TEXT,
    replaced_by   TEXT,
    last_used_at  TEXT,
    expires_at    TEXT NOT NULL,
    created_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_devices (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id   TEXT NOT NULL,
    platform    TEXT,
    user_agent  TEXT,
    last_seen_at TEXT NOT NULL,
    revoked_at  TEXT,
    metadata    TEXT,
    created_at  TEXT NOT NULL,
    UNIQUE(user_id, device_id)
);

CREATE TABLE IF NOT EXISTS security_events (
    id          TEXT PRIMARY KEY,
    user_id     TEXT,
    event_type  TEXT NOT NULL,
    ip_address  TEXT,
    user_agent  TEXT,
    metadata    TEXT,
    created_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS consent_records (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category    TEXT NOT NULL,
    granted     INTEGER NOT NULL DEFAULT 0,
    version     TEXT NOT NULL,
    ip_address  TEXT,
    user_agent  TEXT,
    created_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS mfa_secrets (
    user_id     TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    secret      TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 0,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan       TEXT NOT NULL DEFAULT 'free',             -- free | premium
    status     TEXT NOT NULL DEFAULT 'active',
    started_at TEXT,
    ends_at    TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS session_preferences (
    user_id                  TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    default_duration_seconds INTEGER NOT NULL DEFAULT 1800,
    default_voice_id         TEXT,
    updated_at               TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS schedules (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label            TEXT NOT NULL,
    time             TEXT NOT NULL,                      -- 'HH:MM' local
    days_of_week     TEXT NOT NULL DEFAULT '1,2,3,4,5,6,7', -- 1=Mon .. 7=Sun
    timezone         TEXT NOT NULL DEFAULT 'UTC',
    duration_seconds INTEGER NOT NULL DEFAULT 1800,
    voice_id         TEXT,
    category_ids     TEXT,                               -- comma-separated category ids
    enabled          INTEGER NOT NULL DEFAULT 1,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

-- ============================= SESSIONS =============================
CREATE TABLE IF NOT EXISTS sessions (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type             TEXT NOT NULL DEFAULT 'standard',   -- quick | standard | deep | custom | personal
    duration_seconds INTEGER NOT NULL,
    voice_id         TEXT,
    status           TEXT NOT NULL DEFAULT 'READY'
        -- Canonical session lifecycle; owned by internal/sessions. Legacy rows
        -- may still hold created|playing|completed|abandoned and are folded
        -- onto these states on read, but nothing new writes them.
        CHECK (status IN ('DRAFT','READY','SCHEDULED','STARTING','ACTIVE','PAUSED',
                          'INTERRUPTED','COMPLETED','CANCELLED','EXPIRED','FAILED')),
    created_at       TEXT NOT NULL,
    started_at       TEXT,
    completed_at     TEXT
);

CREATE TABLE IF NOT EXISTS session_items (
    id               TEXT PRIMARY KEY,
    session_id       TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    confession_id    TEXT NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    variant_id       TEXT,
    voice_id         TEXT,
    audio_asset_id   TEXT,
    position         INTEGER NOT NULL,
    duration_seconds INTEGER NOT NULL,
    status           TEXT NOT NULL DEFAULT 'queued'      -- queued | played | skipped
);

-- ============================= ENGAGEMENT =============================
CREATE TABLE IF NOT EXISTS favorites (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,                           -- confession | category | session | voice
    entity_id   TEXT NOT NULL,
    created_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS playback_history (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id       TEXT,
    confession_id    TEXT,
    duration_seconds INTEGER,
    completed        INTEGER NOT NULL DEFAULT 0,
    skipped          INTEGER NOT NULL DEFAULT 0,
    listened_at      TEXT NOT NULL
);

-- ============================= USER CONTENT =============================
CREATE TABLE IF NOT EXISTS user_confessions (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    text        TEXT NOT NULL,
    category_id TEXT,
    is_private  INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_confession_audio (
    id                 TEXT PRIMARY KEY,
    user_confession_id TEXT NOT NULL REFERENCES user_confessions(id) ON DELETE CASCADE,
    voice_id           TEXT,
    url                TEXT,
    status             TEXT NOT NULL DEFAULT 'queued',
    created_at         TEXT NOT NULL
);

-- ============================= ADMIN =============================
CREATE TABLE IF NOT EXISTS admin_users (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'support',          -- super_admin | content_admin | audio_producer | theological_reviewer | support_admin | analytics_admin
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id            TEXT PRIMARY KEY,
    admin_user_id TEXT,
    action        TEXT NOT NULL,
    entity        TEXT NOT NULL,
    entity_id     TEXT,
    created_at    TEXT NOT NULL
);

-- ============================= JOB QUEUE =============================
CREATE TABLE IF NOT EXISTS jobs (
    id               TEXT PRIMARY KEY,
    type             TEXT NOT NULL,
    payload          TEXT,
    status           TEXT NOT NULL DEFAULT 'queued',  -- queued | running | completed | failed | dead_letter
    attempts         INTEGER NOT NULL DEFAULT 0,
    max_attempts     INTEGER NOT NULL DEFAULT 3,
    last_error       TEXT,
    idempotency_key  TEXT UNIQUE,
    retry_delay_ms   INTEGER DEFAULT 0,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_confessions_category ON confessions(category_id);
CREATE INDEX IF NOT EXISTS idx_confessions_status   ON confessions(status);

-- Audio indexes for performance
CREATE INDEX IF NOT EXISTS idx_audio_assets_content ON audio_assets(content_id);
CREATE INDEX IF NOT EXISTS idx_audio_assets_voice ON audio_assets(voice_id);
CREATE INDEX IF NOT EXISTS idx_audio_assets_status ON audio_assets(status);
CREATE INDEX IF NOT EXISTS idx_audio_assets_published ON audio_assets(published_at);
CREATE INDEX IF NOT EXISTS idx_audio_assets_content_voice_type ON audio_assets(content_id, voice_id, asset_type);
CREATE INDEX IF NOT EXISTS idx_audio_assets_checksum ON audio_assets(checksum_sha256);

CREATE INDEX IF NOT EXISTS idx_audio_generation_jobs_status ON audio_generation_jobs(status);
CREATE INDEX IF NOT EXISTS idx_audio_generation_jobs_content_voice ON audio_generation_jobs(content_version_id, voice_id);
CREATE INDEX IF NOT EXISTS idx_audio_generation_jobs_idempotency ON audio_generation_jobs(idempotency_key);

CREATE INDEX IF NOT EXISTS idx_audio_variants_asset ON audio_variants(audio_asset_id);
CREATE INDEX IF NOT EXISTS idx_audio_variants_quality ON audio_variants(quality_tier);

CREATE INDEX IF NOT EXISTS idx_signed_urls_audio ON signed_urls(audio_asset_id);
CREATE INDEX IF NOT EXISTS idx_signed_urls_expires ON signed_urls(expires_at);
CREATE INDEX IF NOT EXISTS idx_signed_urls_user ON signed_urls(user_id);

CREATE INDEX IF NOT EXISTS idx_playback_sessions_user ON audio_playback_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_playback_sessions_audio ON audio_playback_sessions(audio_asset_id);
CREATE INDEX IF NOT EXISTS idx_playback_sessions_status ON audio_playback_sessions(status);

CREATE INDEX IF NOT EXISTS idx_downloads_user ON audio_downloads(user_id);
CREATE INDEX IF NOT EXISTS idx_downloads_audio ON audio_downloads(audio_asset_id);
CREATE INDEX IF NOT EXISTS idx_downloads_status ON audio_downloads(status);

CREATE INDEX IF NOT EXISTS idx_playback_progress_user ON playback_progress(user_id);
CREATE INDEX IF NOT EXISTS idx_playback_progress_audio ON playback_progress(audio_asset_id);

CREATE INDEX IF NOT EXISTS idx_audio_events_user ON audio_events(user_id);
CREATE INDEX IF NOT EXISTS idx_audio_events_audio ON audio_events(audio_asset_id);
CREATE INDEX IF NOT EXISTS idx_audio_events_type ON audio_events(event_type);
CREATE INDEX IF NOT EXISTS idx_audio_events_created ON audio_events(created_at);

CREATE INDEX IF NOT EXISTS idx_audio_qoe_user ON audio_qoe_metrics(user_id);
CREATE INDEX IF NOT EXISTS idx_audio_qoe_audio ON audio_qoe_metrics(audio_asset_id);

-- Content versioning indexes
CREATE INDEX IF NOT EXISTS idx_content_versions_confession ON content_versions(confession_id);
CREATE INDEX IF NOT EXISTS idx_content_versions_status ON content_versions(status);

-- Voice rights indexes
CREATE INDEX IF NOT EXISTS idx_voice_rights_voice ON voice_rights(voice_id);
CREATE INDEX IF NOT EXISTS idx_voice_rights_status ON voice_rights(status);
CREATE INDEX IF NOT EXISTS idx_voice_rights_expiry ON voice_rights(expiry_date);

-- Existing indexes preserved
CREATE INDEX IF NOT EXISTS idx_audio_assets_confession_voice ON audio_assets(content_id, voice_id);
CREATE INDEX IF NOT EXISTS idx_session_items_session ON session_items(session_id);
CREATE INDEX IF NOT EXISTS idx_schedules_user ON schedules(user_id);
CREATE INDEX IF NOT EXISTS idx_favorites_user ON favorites(user_id);

-- ===================== USER LIBRARY (PRD S35-S37, S45, S46) =====================

-- User-created collections. Distinct from the editorial `collections` table:
-- these belong to a person, not to the catalogue.
CREATE TABLE IF NOT EXISTS user_collections (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT,
    cover_url   TEXT,
    visibility  TEXT NOT NULL DEFAULT 'private',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_user_collections_user ON user_collections(user_id);

CREATE TABLE IF NOT EXISTS user_collection_items (
    id            TEXT PRIMARY KEY,
    collection_id TEXT NOT NULL REFERENCES user_collections(id) ON DELETE CASCADE,
    confession_id TEXT NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    position      INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL,
    UNIQUE(collection_id, confession_id)
);
CREATE INDEX IF NOT EXISTS idx_user_collection_items ON user_collection_items(collection_id, position);

-- Notification preferences. Separate from user_preferences so adding a channel
-- does not require touching the main preference row.
CREATE TABLE IF NOT EXISTS notification_preferences (
    user_id            TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    scheduled_sessions INTEGER NOT NULL DEFAULT 1,
    new_content        INTEGER NOT NULL DEFAULT 1,
    recommendations    INTEGER NOT NULL DEFAULT 1,
    product_updates    INTEGER NOT NULL DEFAULT 0,
    updated_at         TEXT NOT NULL
);

-- ==================== ACCOUNT DELETION (PRD S40, S50, S84, S85) ====================
--
-- Deletion is two-phase. A request starts a grace period during which the
-- account is unusable but recoverable; erasure then anonymises the identity and
-- destroys personal data.
--
-- The grace period exists because account deletion is irreversible and is a
-- common target of account-takeover: an attacker who briefly holds a session
-- should not be able to destroy someone's account before they can react.

ALTER TABLE users ADD COLUMN deletion_requested_at TEXT;
ALTER TABLE users ADD COLUMN deleted_at            TEXT;
-- anonymised_at records when identifying fields were tombstoned, which is the
-- point after which the row exists only to satisfy foreign keys on retained
-- records.
ALTER TABLE users ADD COLUMN anonymised_at         TEXT;

CREATE INDEX IF NOT EXISTS idx_users_pending_deletion
    ON users(deletion_requested_at) WHERE deleted_at IS NULL;

-- A durable log of erasures, kept after the account is gone.
--
-- It holds no personal data: only the tombstoned id, timestamps and counts, so
-- the platform can demonstrate a deletion was honoured without retaining the
-- person it concerned.
CREATE TABLE IF NOT EXISTS deletion_records (
    id                 TEXT PRIMARY KEY,
    user_ref           TEXT NOT NULL,
    requested_at       TEXT NOT NULL,
    erased_at          TEXT,
    reason             TEXT,
    rows_deleted       INTEGER NOT NULL DEFAULT 0,
    tables_affected    TEXT,
    -- retained_categories names what was deliberately kept and why, so a
    -- regulator question has an answer that is not "we think nothing".
    retained_categories TEXT,
    created_at         TEXT NOT NULL
);

-- MFA enrolment state (PRD S41, S81).
ALTER TABLE mfa_secrets ADD COLUMN recovery_hashes TEXT;
-- last_counter defeats replay: a TOTP code stays valid for its whole 30-second
-- window, so without remembering the last accepted counter an observed code can
-- be reused within it.
ALTER TABLE mfa_secrets ADD COLUMN last_counter    INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mfa_secrets ADD COLUMN confirmed_at    TEXT;

-- ==================== PUSH NOTIFICATIONS (PRD S47, S19, S34) ====================

-- Push credentials live on the device row. Stored separately from `metadata`
-- so a token can be cleared on logout or unregistration without touching
-- anything else, and so it is never accidentally serialised with device info.
ALTER TABLE user_devices ADD COLUMN push_token    TEXT;
ALTER TABLE user_devices ADD COLUMN push_provider TEXT;   -- apns | fcm
-- push_failures counts consecutive delivery rejections. A token that keeps
-- failing is dead (app uninstalled, token rotated); continuing to send to it
-- wastes quota and harms sender reputation with the provider.
ALTER TABLE user_devices ADD COLUMN push_failures INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_user_devices_push
    ON user_devices(user_id) WHERE push_token IS NOT NULL AND revoked_at IS NULL;

-- Scheduled-notification dispatch log.
--
-- Exists to make delivery idempotent: a schedule fires once per local
-- occurrence, and a sweeper that runs twice (restart, overlapping tick, second
-- replica) must not wake someone up twice. The UNIQUE constraint is the
-- guarantee, not application logic.
CREATE TABLE IF NOT EXISTS scheduled_deliveries (
    id            TEXT PRIMARY KEY,
    schedule_id   TEXT NOT NULL REFERENCES schedules(id) ON DELETE CASCADE,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- occurrence_key is the schedule's local date+time, e.g. "2026-09-02T06:00".
    -- Keyed on local wall-clock rather than UTC so a timezone change does not
    -- create a duplicate for the same intended moment.
    occurrence_key TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'sent',   -- sent | failed | skipped
    detail        TEXT,
    created_at    TEXT NOT NULL,
    UNIQUE(schedule_id, occurrence_key)
);

CREATE INDEX IF NOT EXISTS idx_scheduled_deliveries_user
    ON scheduled_deliveries(user_id, created_at);

-- ==================== OFFLINE DOWNLOADS (PRD S28, S44) ====================
--
-- A download is a licence to hold audio locally for a bounded time, not a
-- permanent copy. The expiry is what makes a lapsed subscription eventually
-- stop working offline without the app needing to police it itself.
ALTER TABLE audio_downloads ADD COLUMN storage_key   TEXT;
ALTER TABLE audio_downloads ADD COLUMN confession_id TEXT;
ALTER TABLE audio_downloads ADD COLUMN voice_id      TEXT;
ALTER TABLE audio_downloads ADD COLUMN duration_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE audio_downloads ADD COLUMN checksum      TEXT;

CREATE INDEX IF NOT EXISTS idx_downloads_user_active
    ON audio_downloads(user_id) WHERE removed_at IS NULL;

-- Audit log detail (PRD S51). `actor` and `detail` record who did what and on
-- what authority, which is the difference between a log and evidence.
ALTER TABLE audit_logs ADD COLUMN actor  TEXT;
ALTER TABLE audit_logs ADD COLUMN detail TEXT;
ALTER TABLE audit_logs ADD COLUMN result TEXT;
CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_logs(entity, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_logs(created_at);

-- ==================== PHASE-1 FOUNDATION HARDENING (2026-09-05) ====================
-- Categories enrichment (§7) — cover image, color/theme, metadata, active/order
ALTER TABLE categories ADD COLUMN cover_image TEXT;
ALTER TABLE categories ADD COLUMN color TEXT;
ALTER TABLE categories ADD COLUMN theme TEXT; -- JSON stored as TEXT in SQLite
ALTER TABLE categories ADD COLUMN metadata TEXT; -- JSON
ALTER TABLE categories ADD COLUMN active INTEGER NOT NULL DEFAULT 1;
-- active mirrors status='published' but allows soft hide without archive; backfill
UPDATE categories SET active = CASE WHEN status='published' THEN 1 ELSE 1 END WHERE active IS NULL;
CREATE INDEX IF NOT EXISTS idx_categories_order ON categories(sort_order, name) WHERE active=1;
CREATE INDEX IF NOT EXISTS idx_categories_active ON categories(active) WHERE active=1;

-- Confessions spec §8 — slug, body, thematic fields, visibility, author, premium/featured
ALTER TABLE confessions ADD COLUMN slug TEXT;
ALTER TABLE confessions ADD COLUMN body TEXT;
ALTER TABLE confessions ADD COLUMN short_description TEXT;
ALTER TABLE confessions ADD COLUMN theme TEXT;
ALTER TABLE confessions ADD COLUMN difficulty INTEGER DEFAULT 1;
ALTER TABLE confessions ADD COLUMN duration INTEGER;
ALTER TABLE confessions ADD COLUMN visibility TEXT NOT NULL DEFAULT 'private';
ALTER TABLE confessions ADD COLUMN author_type TEXT;
ALTER TABLE confessions ADD COLUMN author_id TEXT;
ALTER TABLE confessions ADD COLUMN review_status TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE confessions ADD COLUMN premium INTEGER NOT NULL DEFAULT 0;
ALTER TABLE confessions ADD COLUMN featured INTEGER NOT NULL DEFAULT 0;
-- backfill slug for existing rows (temporary)
UPDATE confessions SET slug = 'c-' || substr(id, 1, 8) WHERE slug IS NULL;
CREATE INDEX IF NOT EXISTS idx_confessions_slug ON confessions(slug) WHERE slug IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_confessions_review ON confessions(review_status);
CREATE INDEX IF NOT EXISTS idx_confessions_featured ON confessions(featured) WHERE featured=1;
CREATE INDEX IF NOT EXISTS idx_confessions_visibility ON confessions(visibility);

-- M2M for multi-category + voice assignment (spec §8)
CREATE TABLE IF NOT EXISTS confession_categories (
    confession_id TEXT NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    category_id   TEXT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    weight        INTEGER NOT NULL DEFAULT 100,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (confession_id, category_id)
);
CREATE INDEX IF NOT EXISTS idx_confession_categories_cat ON confession_categories(category_id);
CREATE TABLE IF NOT EXISTS confession_voices (
    confession_id TEXT NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    voice_id      TEXT NOT NULL REFERENCES voices(id) ON DELETE RESTRICT,
    is_default    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (confession_id, voice_id)
);
-- backfill M2M from legacy single category_id
INSERT OR IGNORE INTO confession_categories (confession_id, category_id)
SELECT id, category_id FROM confessions WHERE category_id IS NOT NULL;

-- Voices display fields (§12)
ALTER TABLE voices ADD COLUMN display_name TEXT;
ALTER TABLE voices ADD COLUMN avatar TEXT;
ALTER TABLE voices ADD COLUMN voice_reference TEXT;
UPDATE voices SET display_name = COALESCE(display_name, name) WHERE display_name IS NULL;

-- Sessions 11-state + snapshot fields (§13-§15)
-- Duration strategy that produced the queue (see internal/engine). Persisted
-- so a created session stays reproducible.
ALTER TABLE sessions ADD COLUMN strategy TEXT NOT NULL DEFAULT 'BALANCED';
ALTER TABLE sessions ADD COLUMN title TEXT;
ALTER TABLE sessions ADD COLUMN description TEXT;
ALTER TABLE sessions ADD COLUMN target_duration INTEGER;
ALTER TABLE sessions ADD COLUMN actual_duration INTEGER;
ALTER TABLE sessions ADD COLUMN queue_id TEXT;
ALTER TABLE sessions ADD COLUMN current_item_id TEXT REFERENCES session_items(id) ON DELETE SET NULL;
ALTER TABLE sessions ADD COLUMN current_item_index INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN paused_at TEXT;
ALTER TABLE sessions ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
UPDATE sessions SET target_duration = COALESCE(target_duration, duration_seconds) WHERE target_duration IS NULL;
-- normalize legacy status values to spec upper-case
UPDATE sessions SET status = CASE status WHEN 'created' THEN 'READY' WHEN 'playing' THEN 'ACTIVE' WHEN 'completed' THEN 'COMPLETED' WHEN 'abandoned' THEN 'CANCELLED' ELSE UPPER(status) END WHERE status IN ('created','playing','completed','abandoned','queued');
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
CREATE INDEX IF NOT EXISTS idx_sessions_target ON sessions(target_duration);

-- Session items enrichment (§19)
ALTER TABLE session_items ADD COLUMN category_id TEXT REFERENCES categories(id) ON DELETE SET NULL;
ALTER TABLE session_items ADD COLUMN planned_duration INTEGER;
UPDATE session_items SET planned_duration = COALESCE(planned_duration, duration_seconds) WHERE planned_duration IS NULL;

-- Idempotency + outbox + feature flags (§47-§48, §111)
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    response_status INTEGER,
    response_body TEXT,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_idempotency_user ON idempotency_keys(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_idempotency_expires ON idempotency_keys(expires_at);

CREATE TABLE IF NOT EXISTS outbox_events (
    id TEXT PRIMARY KEY,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TEXT NOT NULL,
    published_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_outbox_unpublished ON outbox_events(published_at) WHERE published_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_outbox_aggregate ON outbox_events(aggregate_type, aggregate_id);

CREATE TABLE IF NOT EXISTS feature_flags (
    key TEXT PRIMARY KEY,
    enabled INTEGER NOT NULL DEFAULT 0,
    rollout_percent INTEGER NOT NULL DEFAULT 0,
    platforms TEXT NOT NULL DEFAULT '',
    min_version TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
INSERT OR IGNORE INTO feature_flags (key, enabled, rollout_percent, platforms, created_at, updated_at) VALUES
 ('new_player', 0, 0, '', datetime('now'), datetime('now')),
 ('ai_session_builder', 0, 0, '', datetime('now'), datetime('now')),
 ('community', 0, 0, '', datetime('now'), datetime('now')),
 ('offline_downloads', 1, 100, '', datetime('now'), datetime('now')),
 ('new_home', 0, 0, '', datetime('now'), datetime('now')),
 ('premium_voices', 1, 100, '', datetime('now'), datetime('now'));

-- ==================== PHASE-2 CONTENT & AUDIO GOVERNANCE (2026-09-05) ====================
-- §7–§12, §74–§75, §96–§97 — moderation queue, QA gate, version polish

-- user_confessions lifecycle expansion (§10)
ALTER TABLE user_confessions ADD COLUMN visibility TEXT NOT NULL DEFAULT 'private';
ALTER TABLE user_confessions ADD COLUMN status TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE user_confessions ADD COLUMN review_notes TEXT;
ALTER TABLE user_confessions ADD COLUMN reviewed_by TEXT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE user_confessions ADD COLUMN reviewed_at TEXT;
ALTER TABLE user_confessions ADD COLUMN rejection_reason TEXT;
ALTER TABLE user_confessions ADD COLUMN slug TEXT;
ALTER TABLE user_confessions ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE user_confessions ADD COLUMN published_at TEXT;
CREATE INDEX IF NOT EXISTS idx_user_conf_status ON user_confessions(status);
CREATE INDEX IF NOT EXISTS idx_user_conf_visibility ON user_confessions(visibility);

-- moderation_cases + reports (§10)
CREATE TABLE IF NOT EXISTS moderation_cases (
    id TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open',
    reason TEXT,
    actor TEXT REFERENCES users(id) ON DELETE SET NULL,
    before TEXT,
    after TEXT,
    detail TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_mod_entity ON moderation_cases(entity_type, entity_id);
CREATE TABLE IF NOT EXISTS reports (
    id TEXT PRIMARY KEY,
    reporter_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    reason TEXT NOT NULL,
    detail TEXT,
    status TEXT NOT NULL DEFAULT 'open',
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS content_moderation_history (
    id TEXT PRIMARY KEY,
    confession_id TEXT NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    from_status TEXT NOT NULL,
    to_status TEXT NOT NULL,
    actor TEXT REFERENCES users(id) ON DELETE SET NULL,
    reason TEXT,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cmh_conf ON content_moderation_history(confession_id, created_at);

-- Confessions QA support (§75)
ALTER TABLE confessions ADD COLUMN qa_passed_at TEXT;
ALTER TABLE confessions ADD COLUMN qa_report TEXT;
CREATE INDEX IF NOT EXISTS idx_cmh_report ON confessions(qa_passed_at) WHERE qa_passed_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_mod_status ON moderation_cases(status);
CREATE INDEX IF NOT EXISTS idx_reports_entity ON reports(entity_type, entity_id);

-- voice_rights expiry sweep support
CREATE INDEX IF NOT EXISTS idx_vr_status_expiry ON voice_rights(status, expiry_date) WHERE status IN ('pending','active');


-- ==================== PHASE-5 TEMPLATES (custom sessions) ====================
CREATE TABLE IF NOT EXISTS user_templates (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    description   TEXT,
    category_ids  TEXT NOT NULL,
    weights       TEXT,
    voice_id      TEXT,
    voice_rules   TEXT,
    ordering      TEXT,
    is_public     INTEGER NOT NULL DEFAULT 0,
    share_token   TEXT UNIQUE,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_templates_user ON user_templates(user_id);
CREATE INDEX IF NOT EXISTS idx_templates_share ON user_templates(share_token) WHERE share_token IS NOT NULL;

-- 39 categories are backend-controlled; see scripts/seed.sh and migrations 0002-0006.
-- No hard-coded taxonomy in mobile.
