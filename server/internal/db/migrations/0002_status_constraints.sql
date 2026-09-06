-- 0002_status_constraints.sql — close G-2.
--
-- Twenty-two tables have a status column. Before this migration, the permitted
-- values for most of them existed only in a trailing SQL comment, which the
-- database does not enforce. A typo wrote an invalid state and nothing noticed
-- until a query filtered on it and silently returned no rows.
--
-- Each vocabulary below is taken from the code that writes the column, not
-- invented: the sessions list is the state machine in internal/sessions,
-- community_posts is the const block in internal/community/policy.go, and the rest are the values the store
-- layer and schema defaults actually use. Where the evidence did not cover a
-- state that a later phase needs - subscriptions in particular, whose trial
-- lifecycle is section 36 and does not exist yet (G-3) - the constraint is
-- deliberately narrow. Widening it is a new migration, and a migration that
-- must be widened is a record that the vocabulary was guessed.

ALTER TABLE collections               DROP CONSTRAINT IF EXISTS collections_status_check;
ALTER TABLE collections             ADD CONSTRAINT collections_status_check             CHECK (status IN ('draft','published','archived'));
ALTER TABLE categories                DROP CONSTRAINT IF EXISTS categories_status_check;
ALTER TABLE categories              ADD CONSTRAINT categories_status_check              CHECK (status IN ('draft','published','archived','pending_deletion','deleted'));
ALTER TABLE confessions               DROP CONSTRAINT IF EXISTS confessions_status_check;
ALTER TABLE confessions             ADD CONSTRAINT confessions_status_check             CHECK (status IN ('draft','published','archived','pending_deletion','deleted'));
ALTER TABLE voices                    DROP CONSTRAINT IF EXISTS voices_status_check;
ALTER TABLE voices                  ADD CONSTRAINT voices_status_check                  CHECK (status IN ('active','inactive','archived','pending_deletion','deleted'));
ALTER TABLE content_versions          DROP CONSTRAINT IF EXISTS content_versions_status_check;
ALTER TABLE content_versions        ADD CONSTRAINT content_versions_status_check        CHECK (status IN ('draft','approved','published','archived'));
ALTER TABLE audio_assets              DROP CONSTRAINT IF EXISTS audio_assets_status_check;
ALTER TABLE audio_assets            ADD CONSTRAINT audio_assets_status_check            CHECK (status IN ('uploading','processing','ready','published','failed','archived'));
ALTER TABLE audio_generation_jobs     DROP CONSTRAINT IF EXISTS audio_generation_jobs_status_check;
ALTER TABLE audio_generation_jobs   ADD CONSTRAINT audio_generation_jobs_status_check   CHECK (status IN ('queued','processing','succeeded','failed','cancelled'));
ALTER TABLE audio_variants            DROP CONSTRAINT IF EXISTS audio_variants_status_check;
ALTER TABLE audio_variants          ADD CONSTRAINT audio_variants_status_check          CHECK (status IN ('processing','ready','failed'));
ALTER TABLE audio_processing_logs     DROP CONSTRAINT IF EXISTS audio_processing_logs_status_check;
ALTER TABLE audio_processing_logs   ADD CONSTRAINT audio_processing_logs_status_check   CHECK (status IN ('started','completed','failed'));
ALTER TABLE voice_rights              DROP CONSTRAINT IF EXISTS voice_rights_status_check;
ALTER TABLE voice_rights            ADD CONSTRAINT voice_rights_status_check            CHECK (status IN ('pending','active','expired','revoked'));
ALTER TABLE audio_playback_sessions   DROP CONSTRAINT IF EXISTS audio_playback_sessions_status_check;
ALTER TABLE audio_playback_sessions ADD CONSTRAINT audio_playback_sessions_status_check CHECK (status IN ('playing','paused','completed','abandoned'));
ALTER TABLE audio_downloads           DROP CONSTRAINT IF EXISTS audio_downloads_status_check;
ALTER TABLE audio_downloads         ADD CONSTRAINT audio_downloads_status_check         CHECK (status IN ('queued','downloading','downloaded','failed','removed'));
ALTER TABLE session_items             DROP CONSTRAINT IF EXISTS session_items_status_check;
ALTER TABLE session_items           ADD CONSTRAINT session_items_status_check           CHECK (status IN ('queued','played','skipped'));
ALTER TABLE jobs                      DROP CONSTRAINT IF EXISTS jobs_status_check;
ALTER TABLE jobs                    ADD CONSTRAINT jobs_status_check                    CHECK (status IN ('queued','running','completed','failed','dead_letter'));
ALTER TABLE scheduled_deliveries      DROP CONSTRAINT IF EXISTS scheduled_deliveries_status_check;
ALTER TABLE scheduled_deliveries    ADD CONSTRAINT scheduled_deliveries_status_check    CHECK (status IN ('sent','failed','skipped'));
ALTER TABLE users                     DROP CONSTRAINT IF EXISTS users_status_check;
ALTER TABLE users                   ADD CONSTRAINT users_status_check                   CHECK (status IN ('active','suspended','pending_deletion','deleted'));
ALTER TABLE subscriptions             DROP CONSTRAINT IF EXISTS subscriptions_status_check;
ALTER TABLE subscriptions           ADD CONSTRAINT subscriptions_status_check           CHECK (status IN ('active','trial','expired','cancelled'));
-- user_confessions.status is added by an ALTER in the baseline, not by the
-- CREATE TABLE, which is why the first count of status columns came out at 22
-- instead of 23. Nothing in the Go code writes it: the columns added alongside
-- it (reviewed_by, rejection_reason, reviewed_at) belong to a moderation
-- lifecycle that section 10 describes and no handler implements. Constraining
-- it now settles the vocabulary before that code exists, rather than
-- reverse-engineering it from whatever the first implementation writes.
ALTER TABLE user_confessions          DROP CONSTRAINT IF EXISTS user_confessions_status_check;
ALTER TABLE user_confessions          ADD CONSTRAINT user_confessions_status_check      CHECK (status IN ('draft','submitted','approved','rejected','published','archived'));
ALTER TABLE user_confession_audio     DROP CONSTRAINT IF EXISTS user_confession_audio_status_check;
ALTER TABLE user_confession_audio   ADD CONSTRAINT user_confession_audio_status_check   CHECK (status IN ('queued','processing','ready','failed'));
ALTER TABLE moderation_cases          DROP CONSTRAINT IF EXISTS moderation_cases_status_check;
ALTER TABLE moderation_cases        ADD CONSTRAINT moderation_cases_status_check        CHECK (status IN ('open','in_review','resolved','dismissed'));
ALTER TABLE reports                   DROP CONSTRAINT IF EXISTS reports_status_check;
ALTER TABLE reports                 ADD CONSTRAINT reports_status_check                 CHECK (status IN ('open','reviewed','resolved','dismissed'));
ALTER TABLE community_posts           DROP CONSTRAINT IF EXISTS community_posts_status_check;
ALTER TABLE community_posts         ADD CONSTRAINT community_posts_status_check         CHECK (status IN ('draft','submitted','under_review','approved','rejected','published','archived'));

-- sessions is the one table whose vocabulary was already enforced, inline in the
-- column definition. It is restated here as a named constraint so every status
-- column is constrained the same way and can be audited from pg_constraint
-- without special-casing one table.
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_status_check;
ALTER TABLE sessions                  DROP CONSTRAINT IF EXISTS sessions_status_check;
ALTER TABLE sessions ADD CONSTRAINT sessions_status_check CHECK (status IN (
    'DRAFT','READY','SCHEDULED','STARTING','ACTIVE','PAUSED',
    'INTERRUPTED','COMPLETED','CANCELLED','EXPIRED','FAILED'
));
