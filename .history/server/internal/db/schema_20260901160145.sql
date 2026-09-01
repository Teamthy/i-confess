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

CREATE TABLE IF NOT EXISTS audio_assets (
    id               TEXT PRIMARY KEY,
    confession_id    TEXT NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    variant_id       TEXT REFERENCES confession_variants(id) ON DELETE SET NULL,
    voice_id         TEXT NOT NULL REFERENCES voices(id) ON DELETE CASCADE,
    url              TEXT NOT NULL,
    duration_seconds INTEGER,
    size_bytes       INTEGER,
    status           TEXT NOT NULL DEFAULT 'ready',      -- queued | processing | ready | failed | archived
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
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
    status           TEXT NOT NULL DEFAULT 'created',    -- created | playing | completed | abandoned
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
CREATE INDEX IF NOT EXISTS idx_audio_assets_confession_voice ON audio_assets(confession_id, voice_id);
CREATE INDEX IF NOT EXISTS idx_session_items_session ON session_items(session_id);
CREATE INDEX IF NOT EXISTS idx_schedules_user ON schedules(user_id);
CREATE INDEX IF NOT EXISTS idx_favorites_user ON favorites(user_id);
