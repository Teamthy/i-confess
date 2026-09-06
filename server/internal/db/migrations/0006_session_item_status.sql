-- 0006_session_item_status.sql — PHASE 15.
--
-- 0002 migrated sessions.status onto the canonical state machine in
-- internal/sessions. session_items.status was left behind on the original
-- three-value list ('queued', 'played', 'skipped'), and the code moved on
-- without it: internal/sessions now defines the item vocabulary as QUEUED,
-- PLAYING, COMPLETED, SKIPPED and FAILED, and every playback handler writes
-- those values.
--
-- So the CHECK constraint rejected almost every write the playback lifecycle
-- makes. Skipping a track returned 500. A progress push carrying item_status
-- returned 500. startSession's SetPlayingItem and completeSession's item update
-- discard their error, so those failed silently and every item stayed 'queued'
-- forever: the queue endpoint always reported all items as waiting and
-- items_completed as 0, however far through a session the listener got.
--
-- Legacy rows are folded onto the canonical vocabulary first, exactly as the
-- itemLegacy map in internal/sessions/item.go folds them on read: 'played'
-- means the item ran to the end, which is COMPLETED, not SKIPPED. The order
-- matters - upper() alone would produce 'PLAYED', which is not a state.
UPDATE session_items SET status = 'COMPLETED' WHERE lower(status) = 'played';
UPDATE session_items SET status = upper(status) WHERE status <> upper(status);

ALTER TABLE session_items DROP CONSTRAINT IF EXISTS session_items_status_check;
ALTER TABLE session_items ADD CONSTRAINT session_items_status_check
    CHECK (status IN ('QUEUED', 'PLAYING', 'COMPLETED', 'SKIPPED', 'FAILED'));

-- The default had the same stale spelling. An INSERT that omits status would
-- otherwise write 'queued' and fail the new constraint.
ALTER TABLE session_items ALTER COLUMN status SET DEFAULT 'QUEUED';
