-- 0038: erase the text of study rows that were already deleted.
--
-- Deleting a journal entry or memory verse used to set deleted_at and keep
-- every word. Deletes now blank the private text in the same statement; this
-- clears any tombstone written before that change. The rows stay so other
-- devices still see the delete through row_version.
UPDATE journal_entries SET title = '', body = '', mood = '' WHERE deleted_at IS NOT NULL AND (title <> '' OR body <> '' OR mood <> '');
UPDATE memory_verses SET text = '', title = '' WHERE deleted_at IS NOT NULL AND (text <> '' OR title <> '');
