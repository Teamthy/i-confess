-- 0013_favorites_unique.sql — PHASE 27.
--
-- The favourites table has carried no uniqueness constraint since the
-- baseline. `AddFavorite` generates a fresh id for every call and its
-- `ON CONFLICT(id) DO NOTHING` can therefore never fire: the id is new by
-- construction, so the conflict target is never hit and the insert always
-- succeeds. Tapping the heart three times wrote three rows.
--
-- That is not a cosmetic duplication. The Library's favourites tab lists
-- rows, so the same confession appeared three times; unfavouriting deleted
-- by (user, type, entity) and removed all three at once, so the count the
-- user saw and the count the delete affected disagreed. The session engine
-- also reads favourites as a preference signal (engine.FavoriteIDs), where a
-- triplicated entity silently weights that confession more heavily than the
-- listener asked for.
--
-- The duplicates are collapsed before the index is added, keeping the oldest
-- row of each group: the favourite dates from when it was first expressed,
-- not from the last accidental re-tap.

DELETE FROM favorites a
      USING favorites b
      WHERE a.user_id = b.user_id
        AND a.entity_type = b.entity_type
        AND a.entity_id = b.entity_id
        AND (a.created_at > b.created_at
             OR (a.created_at = b.created_at AND a.id > b.id));

CREATE UNIQUE INDEX IF NOT EXISTS favorites_one_per_entity
    ON favorites (user_id, entity_type, entity_id);
