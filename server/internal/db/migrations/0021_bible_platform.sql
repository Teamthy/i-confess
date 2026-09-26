-- 0021_bible_platform.sql — multilingual Bible provenance and reader foundation.
-- Additive: migration 0020 and already-imported verse rows remain untouched.

CREATE TABLE IF NOT EXISTS bible_languages (
    id TEXT PRIMARY KEY,
    iso639_1 TEXT,
    iso639_2 TEXT,
    iso639_3 TEXT NOT NULL UNIQUE,
    bcp47 TEXT NOT NULL,
    name TEXT NOT NULL,
    native_name TEXT NOT NULL,
    region TEXT,
    direction TEXT NOT NULL DEFAULT 'ltr' CHECK (direction IN ('ltr','rtl')),
    script TEXT,
    font_family TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive','pending_review')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO bible_languages(id,iso639_1,iso639_2,iso639_3,bcp47,name,native_name,direction)
VALUES
 ('en','en','eng','eng','en','English','English','ltr'),
 ('sw','sw','swa','swa','sw','Swahili','Kiswahili','ltr'),
 ('es','es','spa','spa','es','Spanish','Español','ltr'),
 ('pt','pt','por','por','pt','Portuguese','Português','ltr'),
 ('fr','fr','fra','fra','fr','French','Français','ltr'),
 ('it','it','ita','ita','it','Italian','Italiano','ltr'),
 ('tl','tl','tgl','tgl','fil','Tagalog','Tagalog','ltr')
ON CONFLICT (id) DO NOTHING;

ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT 'open-bibles';
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS provider_translation_id TEXT;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS locale TEXT;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS country TEXT;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS dialect TEXT;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS publisher TEXT;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS copyright_text TEXT;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS public_domain BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS commercial_use BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS redistribution_allowed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS modification_allowed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS audio_allowed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS offline_allowed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS copy_allowed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS share_allowed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS search_index_allowed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS api_exposure_allowed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS attribution_required BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS attribution_text TEXT;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS source_url TEXT;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS source_version TEXT;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS import_version TEXT;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS content_hash TEXT;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending_review';
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS reviewed_by TEXT;

-- Backfill provenance fields only; never turn a license string into platform
-- permissions. Newly added permission columns remain default-deny until the
-- exact translation ID appears in the reviewed grants below.
UPDATE bible_versions SET
    provider_translation_id = COALESCE(provider_translation_id, id),
    locale = COALESCE(locale, language),
    copyright_text = COALESCE(copyright_text, licence),
    attribution_text = COALESCE(attribution_text, attribution),
    source_url = COALESCE(source_url, blob_url),
    source_version = COALESCE(source_version, sha256),
    import_version = COALESCE(import_version, 'legacy-0020'),
    content_hash = COALESCE(content_hash, sha256);

-- These exact open-bibles IDs have individually reviewed registry grants.
-- Audio intentionally remains disabled; this list is not derived from `licence`.
UPDATE bible_versions SET
    public_domain=TRUE, commercial_use=TRUE, redistribution_allowed=TRUE,
    modification_allowed=TRUE, audio_allowed=FALSE, offline_allowed=TRUE,
    copy_allowed=TRUE, share_allowed=TRUE, search_index_allowed=TRUE,
    api_exposure_allowed=TRUE, attribution_required=FALSE,
    status=CASE WHEN deleted_at IS NULL THEN 'active' ELSE 'removed' END
WHERE provider='open-bibles'
  AND id IN ('kjv','web','asv','webbe','bsb','ylt','rv1909','almeida','ostervald','riveduta','tagalog');

-- The source notes for this specific edition are ambiguous. Keep its content
-- stored but unavailable until a reviewer confirms independent usage grants.
UPDATE bible_versions SET
    public_domain=FALSE, commercial_use=FALSE, redistribution_allowed=FALSE,
    modification_allowed=FALSE, audio_allowed=FALSE, offline_allowed=FALSE,
    copy_allowed=FALSE, share_allowed=FALSE, search_index_allowed=FALSE,
    api_exposure_allowed=FALSE, attribution_required=TRUE, status='pending_review'
WHERE id='swahili' AND provider='open-bibles';

CREATE UNIQUE INDEX IF NOT EXISTS bible_versions_provider_translation_key
  ON bible_versions(provider, provider_translation_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS bible_versions_language_status
  ON bible_versions(language,status,sort_order) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS bible_translation_books (
    version_id TEXT NOT NULL REFERENCES bible_versions(id) ON DELETE CASCADE,
    book_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    testament TEXT NOT NULL CHECK(testament IN ('old','new','deuterocanon')),
    canonical_order INTEGER NOT NULL,
    chapter_count INTEGER NOT NULL DEFAULT 0,
    has_text BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY(version_id,book_id)
);
CREATE INDEX IF NOT EXISTS bible_translation_books_order
  ON bible_translation_books(version_id,canonical_order);

CREATE TABLE IF NOT EXISTS bible_import_jobs (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    provider_translation_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('queued','running','validated','failed','cancelled')),
    source_version TEXT,
    content_hash TEXT,
    books_seen INTEGER NOT NULL DEFAULT 0,
    chapters_seen INTEGER NOT NULL DEFAULT 0,
    verses_seen INTEGER NOT NULL DEFAULT 0,
    validation_report JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_code TEXT,
    requested_by TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS bible_import_jobs_recent ON bible_import_jobs(created_at DESC);

CREATE TABLE IF NOT EXISTS bible_sync_jobs (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('queued','running','succeeded','failed','cancelled')),
    sync_cursor TEXT,
    error_code TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_bible_notes (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter > 0),
    verse_start INTEGER NOT NULL CHECK(verse_start > 0),
    verse_end INTEGER NOT NULL CHECK(verse_end >= verse_start),
    translation_id TEXT REFERENCES bible_versions(id) ON DELETE SET NULL,
    title TEXT,
    body TEXT NOT NULL,
    device_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    row_version INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS user_bible_notes_owner_updated
  ON user_bible_notes(user_id,updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS user_bible_notes_reference
  ON user_bible_notes(user_id,book_id,chapter,verse_start,verse_end) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS user_bible_preferences (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    default_translation_id TEXT REFERENCES bible_versions(id) ON DELETE SET NULL,
    preferred_language TEXT REFERENCES bible_languages(id) ON DELETE SET NULL,
    font_size INTEGER NOT NULL DEFAULT 20 CHECK(font_size BETWEEN 14 AND 40),
    line_height REAL NOT NULL DEFAULT 1.75 CHECK(line_height BETWEEN 1.2 AND 2.5),
    theme TEXT NOT NULL DEFAULT 'system' CHECK(theme IN ('light','dark','system')),
    show_verse_numbers BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    row_version INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS user_bible_history (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    translation_id TEXT REFERENCES bible_versions(id) ON DELETE SET NULL,
    book_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter > 0),
    verse INTEGER NOT NULL DEFAULT 1 CHECK(verse > 0),
    scroll_offset REAL NOT NULL DEFAULT 0,
    device_id TEXT,
    read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    row_version INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS user_bible_history_recent ON user_bible_history(user_id,read_at DESC);

CREATE TABLE IF NOT EXISTS user_bible_progress (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    translation_id TEXT NOT NULL REFERENCES bible_versions(id) ON DELETE CASCADE,
    book_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter > 0),
    completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    row_version INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY(user_id,translation_id,book_id,chapter)
);

-- PostgreSQL's simple configuration is intentionally language-neutral: it
-- safely indexes imported Unicode without pretending English stemming applies
-- to every language. Rights control indexing at query and import time.
CREATE INDEX IF NOT EXISTS bible_verses_text_search
  ON bible_verses USING GIN (to_tsvector('simple', text))
  WHERE deleted_at IS NULL;

ALTER TABLE bible_versions ADD CONSTRAINT bible_versions_language_fkey
  FOREIGN KEY(language) REFERENCES bible_languages(id) ON DELETE RESTRICT;
ALTER TABLE verse_highlights ADD CONSTRAINT verse_highlights_version_fkey
  FOREIGN KEY(version_id) REFERENCES bible_versions(id) ON DELETE CASCADE;
ALTER TABLE verse_bookmarks ADD CONSTRAINT verse_bookmarks_version_fkey
  FOREIGN KEY(version_id) REFERENCES bible_versions(id) ON DELETE CASCADE;
