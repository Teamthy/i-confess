-- Bible integrity hardening. 0020–0022 have already been released: do not edit
-- their bytes or their migration ledger checksums. These additions are safe
-- for both new and existing PostgreSQL installations.

-- Keep the new Bible tables under the same retention/version contract as every
-- other application table. Existing rows take version 1; deletions remain
-- nullable so an operator can retire metadata without erasing study history.
ALTER TABLE bible_languages ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE bible_languages ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE bible_translation_books ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE bible_translation_books ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE bible_import_jobs ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE bible_import_jobs ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE bible_sync_jobs ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE bible_sync_jobs ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE bible_rights_reviews ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE bible_rights_reviews ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE bible_reading_plans ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE bible_reading_plans ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE bible_reading_plan_days ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE bible_reading_plan_days ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE bible_cross_references ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE bible_cross_references ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE bible_verse_of_day ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE bible_verse_of_day ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE bible_offline_packages ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE bible_offline_packages ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE bible_offline_packages ADD COLUMN source_content_hash TEXT;
ALTER TABLE bible_offline_packages ADD COLUMN package_schema_version INTEGER NOT NULL DEFAULT 1;
CREATE INDEX bible_offline_packages_current_source
    ON bible_offline_packages(translation_id,book_id,source_content_hash,package_schema_version)
    WHERE status='ready' AND deleted_at IS NULL;
-- Old packages include a request-time timestamp in their content hash and
-- cannot be safely shared/reused. Require a deterministic rebuild under 0023.
UPDATE bible_offline_packages SET status='revoked',row_version=row_version+1
WHERE status='ready' AND source_content_hash IS NULL;
UPDATE user_bible_offline_licenses SET revoked_at=now()
WHERE revoked_at IS NULL AND package_id IN
    (SELECT id FROM bible_offline_packages WHERE status='revoked');
ALTER TABLE bible_audio_assets ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE bible_audio_assets ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE user_bible_preferences ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE user_bible_history ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE user_bible_progress ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE user_bible_plan_enrollments ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE user_bible_plan_day_progress ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE user_bible_plan_day_progress ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE user_bible_offline_licenses ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE user_bible_offline_licenses ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE user_bible_collection_items ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE user_bible_sync_mutations ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE user_bible_sync_mutations ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE user_bible_sync_devices ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE user_bible_sync_devices ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;

-- A rights decision survives the reviewer's account erasure for compliance,
-- but the decision must no longer identify them. The original FK was RESTRICT
-- and NOT NULL, which made anonymisation impossible.
ALTER TABLE bible_rights_reviews ALTER COLUMN actor_id DROP NOT NULL;
ALTER TABLE bible_rights_reviews DROP CONSTRAINT IF EXISTS bible_rights_reviews_actor_id_fkey;
ALTER TABLE bible_rights_reviews ADD CONSTRAINT bible_rights_reviews_actor_id_fkey
    FOREIGN KEY(actor_id) REFERENCES users(id) ON DELETE SET NULL;

-- Cross-references can be proposed without being published. Preserve the
-- submitter and independently signed review evidence; both users can later
-- erase their accounts without losing the approved editorial reference.
ALTER TABLE bible_cross_references ADD COLUMN created_by TEXT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE bible_cross_references ADD COLUMN review_evidence_url TEXT;
ALTER TABLE bible_cross_references ADD COLUMN review_rationale TEXT;
ALTER TABLE bible_verse_of_day ADD COLUMN review_evidence_url TEXT;

ALTER TABLE bible_versions ADD CONSTRAINT bible_versions_status_check
    CHECK (status IN ('pending_review','active','rejected','suspended','removed'));

-- The old GIN index in 0021 included *all* stored text, including editions
-- whose search permission had never been granted. Move indexed lexemes into a
-- rights-scoped materialization. Revocation removes them in the SAME transaction
-- as the rights change; public queries also check current grants independently.
DROP INDEX IF EXISTS bible_verses_text_search;
CREATE TABLE bible_search_documents (
    verse_id BIGINT PRIMARY KEY REFERENCES bible_verses(id) ON DELETE CASCADE,
    version_id TEXT NOT NULL REFERENCES bible_versions(id) ON DELETE CASCADE,
    document TSVECTOR NOT NULL,
    deleted_at TIMESTAMPTZ,
    row_version INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX bible_search_documents_gin ON bible_search_documents USING GIN(document) WHERE deleted_at IS NULL;
CREATE INDEX bible_search_documents_version ON bible_search_documents(version_id);
INSERT INTO bible_search_documents(verse_id,version_id,document)
SELECT v.id,v.version_id,to_tsvector('simple',v.text)
FROM bible_verses v JOIN bible_versions t ON t.id=v.version_id
WHERE v.deleted_at IS NULL AND t.deleted_at IS NULL
  AND t.status='active' AND t.api_exposure_allowed=TRUE AND t.search_index_allowed=TRUE;

-- COPY inserts a translation in one statement; a statement-level transition
-- table avoids one index refresh/query per verse during the bulk import.
CREATE FUNCTION bible_index_inserted_verses() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO bible_search_documents(verse_id,version_id,document)
    SELECT v.id,v.version_id,to_tsvector('simple',v.text)
    FROM new_verses v JOIN bible_versions t ON t.id=v.version_id
    WHERE v.deleted_at IS NULL AND t.deleted_at IS NULL
      AND t.status='active' AND t.api_exposure_allowed=TRUE AND t.search_index_allowed=TRUE
    ON CONFLICT(verse_id) DO NOTHING;
    RETURN NULL;
END;
$$;
CREATE TRIGGER bible_index_new_verses AFTER INSERT ON bible_verses
    REFERENCING NEW TABLE AS new_verses FOR EACH STATEMENT
    EXECUTE FUNCTION bible_index_inserted_verses();

CREATE FUNCTION bible_refresh_search_rights() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.deleted_at IS NULL AND NEW.status='active'
       AND NEW.api_exposure_allowed=TRUE AND NEW.search_index_allowed=TRUE THEN
        INSERT INTO bible_search_documents(verse_id,version_id,document)
        SELECT v.id,v.version_id,to_tsvector('simple',v.text)
        FROM bible_verses v WHERE v.version_id=NEW.id AND v.deleted_at IS NULL
        ON CONFLICT(verse_id) DO NOTHING;
    ELSE
        DELETE FROM bible_search_documents WHERE version_id=NEW.id;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER bible_search_rights_changed
    AFTER UPDATE OF status,api_exposure_allowed,search_index_allowed,deleted_at ON bible_versions
    FOR EACH ROW EXECUTE FUNCTION bible_refresh_search_rights();

-- Legacy remote discoveries did not pin the upstream translation hash. Any
-- previously activated one must be suspended rather than served under a
-- mutable provider ID. A fresh digest requires explicit catalog sync/review.
UPDATE bible_versions SET status='suspended',api_exposure_allowed=FALSE,
    search_index_allowed=FALSE,offline_allowed=FALSE,audio_allowed=FALSE,
    copy_allowed=FALSE,share_allowed=FALSE,row_version=row_version+1
WHERE provider='helloao' AND status='active'
  AND (content_hash IS NULL OR content_hash !~* '^[0-9a-f]{64}$');

-- Rights or attribution changes invalidate issued offline licenses and the
-- package manifest atomically, even when the edit comes from an operator SQL
-- session instead of the admin HTTP endpoint. Already-downloaded bytes can
-- only be invalidated by a connected client or a locally checked expiry.
CREATE FUNCTION bible_revoke_offline_on_rights_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (OLD.status,OLD.api_exposure_allowed,OLD.offline_allowed,
        OLD.redistribution_allowed,OLD.commercial_use,OLD.offline_max_days,
        OLD.attribution_text,OLD.content_hash,OLD.deleted_at)
       IS DISTINCT FROM
       (NEW.status,NEW.api_exposure_allowed,NEW.offline_allowed,
        NEW.redistribution_allowed,NEW.commercial_use,NEW.offline_max_days,
        NEW.attribution_text,NEW.content_hash,NEW.deleted_at) THEN
        UPDATE bible_offline_packages SET status='revoked',row_version=row_version+1
        WHERE translation_id=NEW.id AND status='ready';
        UPDATE user_bible_offline_licenses SET revoked_at=now(),row_version=row_version+1
        WHERE revoked_at IS NULL AND package_id IN
            (SELECT id FROM bible_offline_packages WHERE translation_id=NEW.id);
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER bible_offline_rights_changed
    AFTER UPDATE OF status,api_exposure_allowed,offline_allowed,
      redistribution_allowed,commercial_use,offline_max_days,
      attribution_text,content_hash,deleted_at ON bible_versions
    FOR EACH ROW EXECUTE FUNCTION bible_revoke_offline_on_rights_change();

-- There are two visual themes, light and dark. Device brightness can select
-- either, but "system" is not a third stored or rendered theme.
UPDATE user_bible_preferences SET theme='light', row_version=row_version+1 WHERE theme='system';
ALTER TABLE user_bible_preferences ALTER COLUMN theme SET DEFAULT 'light';
ALTER TABLE user_bible_preferences DROP CONSTRAINT IF EXISTS user_bible_preferences_theme_check;
ALTER TABLE user_bible_preferences ADD CONSTRAINT user_bible_preferences_theme_check
    CHECK (theme IN ('light','dark'));

-- Once imported, a canonical verse and its source digest are immutable. A
-- correction to an upstream text is a new translation/version ID with a new
-- checksum, never a silent rewrite of verses referenced by saved study data.
CREATE FUNCTION bible_prevent_verse_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Bible verses are immutable; import a new translation ID';
END;
$$;
CREATE TRIGGER bible_verses_immutable
    BEFORE UPDATE OR DELETE ON bible_verses
    FOR EACH ROW EXECUTE FUNCTION bible_prevent_verse_change();

CREATE FUNCTION bible_prevent_source_replacement() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (OLD.sha256 IS DISTINCT FROM NEW.sha256
        OR OLD.content_hash IS DISTINCT FROM NEW.content_hash
        OR OLD.source_version IS DISTINCT FROM NEW.source_version)
       AND EXISTS (SELECT 1 FROM bible_verses WHERE version_id=OLD.id LIMIT 1) THEN
        RAISE EXCEPTION 'Bible translation content hash is immutable; use a new translation ID';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER bible_versions_immutable_source
    BEFORE UPDATE ON bible_versions
    FOR EACH ROW EXECUTE FUNCTION bible_prevent_source_replacement();

-- Seeded editorial suggestions are not human sign-off. Keep them visible in
-- the admin review queue, but do not expose them as reviewed public readings.
UPDATE bible_reading_plans SET status='review' WHERE status='published' AND reviewed_at IS NULL;
