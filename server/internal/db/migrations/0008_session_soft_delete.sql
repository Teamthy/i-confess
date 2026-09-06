-- 0008_session_soft_delete.sql — PHASE 17.
--
-- DELETE /sessions/{id} did not exist, so a listener had no way to remove a
-- session from their history. Section 25 asks for soft delete rather than
-- removal, and for a session that is the right answer twice over: the row is the
-- record of what someone listened to, and destroying it would silently edit
-- listening history, streaks and any completion metric derived from it.
--
-- The timestamp is text like every other timestamp in this schema. NULL and the
-- empty string both mean "not deleted", because a NOT NULL DEFAULT '' column
-- would make "never deleted" indistinguishable from "deleted at the epoch".

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS deleted_at text;

-- History queries filter on user and deleted_at and order by created_at; this is
-- the index that keeps paging cheap as a daily listener accumulates sessions.
CREATE INDEX IF NOT EXISTS idx_sessions_history
    ON sessions (user_id, deleted_at, created_at);
