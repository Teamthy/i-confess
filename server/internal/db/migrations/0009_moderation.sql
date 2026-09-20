-- 0009_moderation.sql — PHASE 31.
--
-- The moderation schema was designed before any of it ran: moderation_cases,
-- reports and content_moderation_history exist since the baseline, and
-- user_confessions grew its review columns in the same file. Four things the
-- implementation needs were still missing, and each is added here rather than
-- worked around in code.
--
-- 1. user_confessions.visibility had no CHECK constraint. Every other column
--    with a closed vocabulary got one in PHASE 07 for exactly this reason: an
--    unconstrained vocabulary is a vocabulary whatever the first typo writes.
--    §22 names three levels for UGC - private, shared, public - and the Go
--    authority (internal/moderation) validates before the column is touched.
--    The normalise-first pattern comes from 0003: a deployment that
--    hand-edited rows should not have its migration abort.
UPDATE user_confessions SET visibility = 'private'
 WHERE visibility NOT IN ('private','shared','public');

ALTER TABLE user_confessions DROP CONSTRAINT IF EXISTS user_confessions_visibility_check;
ALTER TABLE user_confessions ADD CONSTRAINT user_confessions_visibility_check
    CHECK (visibility IN ('private','shared','public'));

-- 2. A report decision must record who decided, when, and why; reports was
--    written with none of those columns. Nullable, so existing open rows stay
--    valid and "not yet decided" remains representable.
ALTER TABLE reports ADD COLUMN IF NOT EXISTS reviewed_by TEXT;
ALTER TABLE reports ADD COLUMN IF NOT EXISTS reviewed_at TEXT;
ALTER TABLE reports ADD COLUMN IF NOT EXISTS resolution_note TEXT;

-- 3. One open report per reporter per entity. Without this the queue is a
--    spam surface: nothing stops one account filing a thousand copies of the
--    same complaint, and every copy is a row a moderator must close. The
--    index is partial so a resolved report never blocks a fresh complaint
--    about the same content. The Go path treats the conflict as
--    "already reported" and returns the existing row.
CREATE UNIQUE INDEX IF NOT EXISTS reports_one_open_per_reporter
    ON reports (reporter_id, entity_type, entity_id) WHERE status = 'open';

-- 4. One open case per entity. Two reports about the same confession are two
--    reports and one queue entry; the case is the unit a moderator works. A
--    resolved or dismissed case leaves the index, so new trouble reopens
--    work instead of being swallowed by history.
CREATE UNIQUE INDEX IF NOT EXISTS moderation_cases_one_open_per_entity
    ON moderation_cases (entity_type, entity_id) WHERE status IN ('open','in_review');

-- The queue reads open reports on every moderation page load; the entity
-- index alone cannot serve that filter cheaply on a busy table.
CREATE INDEX IF NOT EXISTS idx_reports_status ON reports (status);
