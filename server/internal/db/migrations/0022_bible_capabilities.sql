-- Bible product capabilities: curated content, review audit, personal sync,
-- downloadable packages and rights-gated audio manifests. All content is
-- additive; source verse rows remain immutable.

ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS offline_max_days INTEGER NOT NULL DEFAULT 0;
UPDATE bible_versions SET offline_max_days=30 WHERE offline_allowed=TRUE AND provider='open-bibles' AND id IN ('kjv','web','asv','webbe','bsb','ylt','rv1909','almeida','ostervald','riveduta','tagalog');
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS audio_terms_url TEXT;
ALTER TABLE bible_versions ADD COLUMN IF NOT EXISTS search_language_config TEXT NOT NULL DEFAULT 'simple';

CREATE TABLE IF NOT EXISTS bible_rights_reviews (
    id TEXT PRIMARY KEY,
    translation_id TEXT NOT NULL REFERENCES bible_versions(id) ON DELETE CASCADE,
    actor_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    decision TEXT NOT NULL CHECK (decision IN ('approved','rejected','suspended')),
    grants JSONB NOT NULL DEFAULT '{}'::jsonb,
    evidence_url TEXT NOT NULL,
    evidence_sha256 TEXT,
    rationale TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS bible_rights_reviews_translation_recent
  ON bible_rights_reviews(translation_id,created_at DESC);

CREATE TABLE IF NOT EXISTS bible_reading_plans (
    id TEXT PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    language TEXT NOT NULL REFERENCES bible_languages(id),
    duration_days INTEGER NOT NULL CHECK(duration_days BETWEEN 1 AND 366),
    source_note TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','review','published','archived')),
    created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS bible_reading_plan_days (
    plan_id TEXT NOT NULL REFERENCES bible_reading_plans(id) ON DELETE CASCADE,
    day_number INTEGER NOT NULL CHECK(day_number > 0),
    title TEXT NOT NULL DEFAULT '',
    passage_references JSONB NOT NULL CHECK(jsonb_typeof(passage_references)='array'),
    PRIMARY KEY(plan_id,day_number)
);
CREATE TABLE IF NOT EXISTS user_bible_plan_enrollments (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_id TEXT NOT NULL REFERENCES bible_reading_plans(id) ON DELETE CASCADE,
    translation_id TEXT NOT NULL REFERENCES bible_versions(id) ON DELETE RESTRICT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    current_day INTEGER NOT NULL DEFAULT 1 CHECK(current_day > 0),
    row_version INTEGER NOT NULL DEFAULT 1,
    UNIQUE(user_id,plan_id)
);
CREATE TABLE IF NOT EXISTS user_bible_plan_day_progress (
    enrollment_id TEXT NOT NULL REFERENCES user_bible_plan_enrollments(id) ON DELETE CASCADE,
    day_number INTEGER NOT NULL CHECK(day_number > 0),
    completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(enrollment_id,day_number)
);
CREATE INDEX IF NOT EXISTS bible_plan_enrollments_owner
  ON user_bible_plan_enrollments(user_id,started_at DESC);

-- Verse text is always resolved from the selected approved translation. These
-- editorial references do not duplicate or rewrite canonical Scripture.
CREATE TABLE IF NOT EXISTS bible_cross_references (
    id BIGSERIAL PRIMARY KEY,
    source_book_id TEXT NOT NULL,
    source_chapter INTEGER NOT NULL CHECK(source_chapter>0),
    source_verse_start INTEGER NOT NULL CHECK(source_verse_start>0),
    source_verse_end INTEGER NOT NULL CHECK(source_verse_end>=source_verse_start),
    target_reference TEXT NOT NULL,
    editor_note TEXT NOT NULL DEFAULT '',
    reviewed_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    UNIQUE(source_book_id,source_chapter,source_verse_start,source_verse_end,target_reference)
);
INSERT INTO bible_cross_references(source_book_id,source_chapter,source_verse_start,source_verse_end,target_reference,editor_note) VALUES
 ('John',3,16,16,'Romans 5:8','The love of God made known in Christ'),
 ('John',3,16,16,'1 John 4:9-10','The love of God made known in Christ'),
 ('Ps',23,1,1,'John 10:11','Shepherd imagery'),
 ('Ps',23,1,1,'Hebrews 13:20','Shepherd imagery'),
 ('Isa',41,10,10,'Deuteronomy 31:8','God’s presence and help'),
 ('Matt',11,28,30,'John 6:37','Rest and welcome'),
 ('Rom',8,38,39,'John 10:28-29','Assurance of God’s love')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS bible_verse_of_day (
    day_index INTEGER PRIMARY KEY CHECK(day_index BETWEEN 1 AND 7),
    book_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter > 0),
    verse INTEGER NOT NULL CHECK(verse > 0),
    editor_note TEXT NOT NULL DEFAULT '',
    reviewed_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ
);
INSERT INTO bible_verse_of_day(day_index,book_id,chapter,verse,editor_note) VALUES
 (1,'Ps',23,1,'A beginning of trust'),
 (2,'Isa',41,10,'Courage in fear'),
 (3,'Matt',11,28,'Rest for the weary'),
 (4,'John',14,27,'Peace'),
 (5,'Rom',8,38,'Assurance'),
 (6,'Phil',4,6,'Prayer in anxiety'),
 (7,'Lam',3,22,'Mercy renewed')
ON CONFLICT(day_index) DO NOTHING;

CREATE TABLE IF NOT EXISTS bible_offline_packages (
    id TEXT PRIMARY KEY,
    translation_id TEXT NOT NULL REFERENCES bible_versions(id) ON DELETE CASCADE,
    book_id TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    storage_key TEXT NOT NULL UNIQUE,
    file_size_bytes BIGINT NOT NULL CHECK(file_size_bytes > 0),
    status TEXT NOT NULL DEFAULT 'ready' CHECK(status IN ('building','ready','revoked','failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(translation_id,book_id,content_hash)
);
CREATE TABLE IF NOT EXISTS user_bible_offline_licenses (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    package_id TEXT NOT NULL REFERENCES bible_offline_packages(id) ON DELETE CASCADE,
    issued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    UNIQUE(user_id,package_id)
);
CREATE INDEX IF NOT EXISTS bible_offline_licenses_owner
  ON user_bible_offline_licenses(user_id,expires_at DESC);

-- Bible audio is deliberately a distinct asset manifest, not an audio_assets
-- confession row. Publication/playback requires translation, voice and asset
-- rights to be independently active. Files stay in the shared object store.
CREATE TABLE IF NOT EXISTS bible_audio_assets (
    id TEXT PRIMARY KEY,
    translation_id TEXT NOT NULL REFERENCES bible_versions(id) ON DELETE CASCADE,
    book_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter > 0),
    verse_start INTEGER NOT NULL CHECK(verse_start > 0),
    verse_end INTEGER NOT NULL CHECK(verse_end >= verse_start),
    content_hash TEXT NOT NULL,
    voice_id TEXT NOT NULL REFERENCES voices(id) ON DELETE RESTRICT,
    audio_source TEXT NOT NULL CHECK(audio_source IN ('human_recording','licensed_master','synthetic')),
    source_rights_evidence_url TEXT NOT NULL,
    storage_key TEXT NOT NULL,
    checksum_sha256 TEXT NOT NULL,
    duration_ms BIGINT NOT NULL CHECK(duration_ms > 0),
    alignment JSONB NOT NULL DEFAULT '[]'::jsonb,
    attribution_text TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending_review' CHECK(status IN ('pending_review','ready','published','withdrawn','failed')),
    rights_reviewed_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    rights_reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(translation_id,book_id,chapter,verse_start,verse_end,content_hash,voice_id)
);
CREATE INDEX IF NOT EXISTS bible_audio_public_lookup
  ON bible_audio_assets(translation_id,book_id,chapter,status);

CREATE UNIQUE INDEX IF NOT EXISTS user_bible_history_chapter_key
  ON user_bible_history(user_id,translation_id,book_id,chapter) WHERE translation_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS user_bible_collections (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 80),
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    row_version INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS user_bible_collections_owner
  ON user_bible_collections(user_id,updated_at DESC) WHERE deleted_at IS NULL;
CREATE TABLE IF NOT EXISTS user_bible_collection_items (
    id TEXT PRIMARY KEY,
    collection_id TEXT NOT NULL REFERENCES user_bible_collections(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    translation_id TEXT REFERENCES bible_versions(id) ON DELETE SET NULL,
    book_id TEXT NOT NULL,
    chapter INTEGER NOT NULL CHECK(chapter>0),
    verse INTEGER NOT NULL CHECK(verse>0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS user_bible_collection_items_one_verse
  ON user_bible_collection_items(collection_id,book_id,chapter,verse) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS user_bible_collection_items_owner
  ON user_bible_collection_items(user_id,collection_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS user_bible_sync_mutations (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mutation_id TEXT NOT NULL,
    entity TEXT NOT NULL CHECK(entity IN ('bookmark','highlight','note','history','progress')),
    entity_id TEXT,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(user_id,mutation_id)
);
CREATE INDEX IF NOT EXISTS user_bible_sync_mutations_recent
  ON user_bible_sync_mutations(user_id,applied_at DESC);

CREATE TABLE IF NOT EXISTS user_bible_sync_devices (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL,
    last_sync_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(user_id,device_id)
);

-- Two genuinely curated, canonical-reference plans. The renderer resolves all
-- wording through the selected translation and displays its provenance.
INSERT INTO bible_reading_plans(id,slug,title,description,language,duration_days,source_note,status)
VALUES
 ('bible-plan-gospels-week','gospels-in-a-week','A Week in the Gospels','Seven short readings tracing the life, teaching, and resurrection of Jesus.','en',7,'Editorial selection of canonical passage references; verse text remains translation source content.','published'),
 ('bible-plan-rest-week','psalms-for-rest','Psalms for Rest','A week of readings from the Psalms for quiet reflection.','en',7,'Editorial selection of canonical passage references; verse text remains translation source content.','published')
ON CONFLICT(slug) DO NOTHING;
INSERT INTO bible_reading_plan_days(plan_id,day_number,title,passage_references) VALUES
 ('bible-plan-gospels-week',1,'The Word made flesh','["John 1:1-18"]'::jsonb),
 ('bible-plan-gospels-week',2,'A new beginning','["Mark 1:14-20"]'::jsonb),
 ('bible-plan-gospels-week',3,'The kingdom among us','["Matthew 5:1-16"]'::jsonb),
 ('bible-plan-gospels-week',4,'Mercy and welcome','["Luke 15:11-32"]'::jsonb),
 ('bible-plan-gospels-week',5,'Love one another','["John 15:1-17"]'::jsonb),
 ('bible-plan-gospels-week',6,'The way of service','["Mark 10:35-45"]'::jsonb),
 ('bible-plan-gospels-week',7,'Hope beyond the grave','["Luke 24:1-12"]'::jsonb),
 ('bible-plan-rest-week',1,'A shepherd’s care','["Psalm 23"]'::jsonb),
 ('bible-plan-rest-week',2,'A refuge','["Psalm 46:1-11"]'::jsonb),
 ('bible-plan-rest-week',3,'Quiet trust','["Psalm 62:1-8"]'::jsonb),
 ('bible-plan-rest-week',4,'Words and silence','["Psalm 19:7-14"]'::jsonb),
 ('bible-plan-rest-week',5,'A prayer for mercy','["Psalm 51:1-12"]'::jsonb),
 ('bible-plan-rest-week',6,'Shelter','["Psalm 91:1-8"]'::jsonb),
 ('bible-plan-rest-week',7,'Give thanks','["Psalm 103:1-14"]'::jsonb)
ON CONFLICT(plan_id,day_number) DO NOTHING;
