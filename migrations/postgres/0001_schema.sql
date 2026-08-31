-- i-confess — canonical PostgreSQL schema (production target)
-- Mirrors internal/db/schema.sql (SQLite dev schema). Use this for `goose`/`atlas`/`golang-migrate`.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================= CONTENT =============================
CREATE TABLE collections (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT,
    premium     BOOLEAN NOT NULL DEFAULT FALSE,
    status      TEXT NOT NULL DEFAULT 'draft',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE categories (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT,
    icon        TEXT,
    premium     BOOLEAN NOT NULL DEFAULT FALSE,
    status      TEXT NOT NULL DEFAULT 'draft',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE collection_categories (
    collection_id UUID NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    category_id   UUID NOT NULL REFERENCES categories(id)  ON DELETE CASCADE,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (collection_id, category_id)
);

CREATE TABLE confessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id  UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    title        TEXT NOT NULL,
    short_text   TEXT,
    medium_text  TEXT,
    long_text    TEXT,
    description  TEXT,
    tags         TEXT[],
    intensity    SMALLINT NOT NULL DEFAULT 1 CHECK (intensity BETWEEN 1 AND 5),
    language     TEXT NOT NULL DEFAULT 'en',
    status       TEXT NOT NULL DEFAULT 'draft',
    author       TEXT,
    version      INTEGER NOT NULL DEFAULT 1,
    published_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_confessions_category ON confessions(category_id);
CREATE INDEX idx_confessions_status   ON confessions(status);

CREATE TABLE confession_variants (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    confession_id    UUID NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    label            TEXT NOT NULL,
    duration_seconds INTEGER NOT NULL,
    sort_order       INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE scripture_references (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    confession_id   UUID NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    book            TEXT NOT NULL,
    chapter         INTEGER,
    verse           TEXT,
    translation     TEXT NOT NULL DEFAULT 'KJV',
    is_direct_quote BOOLEAN NOT NULL DEFAULT FALSE,
    notes           TEXT,
    sort_order      INTEGER NOT NULL DEFAULT 0
);

-- ============================= AUDIO =============================
CREATE TABLE voices (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    description TEXT,
    type        TEXT NOT NULL DEFAULT 'professional',
    provider    TEXT,
    gender      TEXT,
    language    TEXT NOT NULL DEFAULT 'en',
    premium     BOOLEAN NOT NULL DEFAULT FALSE,
    status      TEXT NOT NULL DEFAULT 'active',
    sample_url  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE voice_licenses (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    voice_id         UUID NOT NULL REFERENCES voices(id) ON DELETE CASCADE,
    owner            TEXT,
    provider         TEXT,
    license_status   TEXT NOT NULL DEFAULT 'none',
    license_start    TIMESTAMPTZ,
    license_expiry   TIMESTAMPTZ,
    allowed_regions  TEXT[],
    commercial_usage BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE audio_assets (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    confession_id    UUID NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    variant_id       UUID REFERENCES confession_variants(id) ON DELETE SET NULL,
    voice_id         UUID NOT NULL REFERENCES voices(id) ON DELETE CASCADE,
    url              TEXT NOT NULL,
    duration_seconds INTEGER,
    size_bytes       BIGINT,
    status           TEXT NOT NULL DEFAULT 'ready',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audio_assets_confession_voice ON audio_assets(confession_id, voice_id);

-- ============================= USERS =============================
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         CITEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT,
    timezone      TEXT NOT NULL DEFAULT 'UTC',
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE subscriptions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan       TEXT NOT NULL DEFAULT 'free',
    status     TEXT NOT NULL DEFAULT 'active',
    started_at TIMESTAMPTZ,
    ends_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE session_preferences (
    user_id                  UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    default_duration_seconds INTEGER NOT NULL DEFAULT 1800,
    default_voice_id         UUID,
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE schedules (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label            TEXT NOT NULL,
    time             TIME NOT NULL,
    days_of_week     SMALLINT[] NOT NULL DEFAULT '{1,2,3,4,5,6,7}',
    timezone         TEXT NOT NULL DEFAULT 'UTC',
    duration_seconds INTEGER NOT NULL DEFAULT 1800,
    voice_id         UUID,
    category_ids     UUID[],
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_schedules_user ON schedules(user_id);

-- ============================= SESSIONS =============================
CREATE TABLE sessions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type             TEXT NOT NULL DEFAULT 'standard',
    duration_seconds INTEGER NOT NULL,
    voice_id         UUID,
    status           TEXT NOT NULL DEFAULT 'created',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at       TIMESTAMPTZ,
    completed_at     TIMESTAMPTZ
);

CREATE TABLE session_items (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id       UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    confession_id    UUID NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    variant_id       UUID,
    voice_id         UUID,
    audio_asset_id   UUID,
    position         INTEGER NOT NULL,
    duration_seconds INTEGER NOT NULL,
    status           TEXT NOT NULL DEFAULT 'queued'
);
CREATE INDEX idx_session_items_session ON session_items(session_id);

-- ============================= ENGAGEMENT =============================
CREATE TABLE favorites (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    entity_id   UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_favorites_user ON favorites(user_id);

CREATE TABLE playback_history (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id       UUID,
    confession_id    UUID,
    duration_seconds INTEGER,
    completed        BOOLEAN NOT NULL DEFAULT FALSE,
    skipped          BOOLEAN NOT NULL DEFAULT FALSE,
    listened_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================= USER CONTENT =============================
CREATE TABLE user_confessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    text        TEXT NOT NULL,
    category_id UUID,
    is_private  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_confession_audio (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_confession_id UUID NOT NULL REFERENCES user_confessions(id) ON DELETE CASCADE,
    voice_id           UUID,
    url                TEXT,
    status             TEXT NOT NULL DEFAULT 'queued',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================= ADMIN =============================
CREATE TABLE admin_users (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'support',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_user_id UUID,
    action        TEXT NOT NULL,
    entity        TEXT NOT NULL,
    entity_id     UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
