-- 0003_content_lifecycle.sql — close G-5 and G-6, and part of G-4.
--
-- PHASE 07 constrained 23 status columns. For most of them the vocabulary was
-- read off the code that writes the column. For confessions it was not: it was
-- given a generic draft/published/archived/pending_deletion/deleted list that
-- contradicted both the documented editorial lifecycle and the validator in
-- the admin handler.
--
-- The result was reachable in production. Five of the eight governance states
-- - content_review, theological_review, audio_production, audio_qa, approved -
-- passed validation in Go and were then refused by this constraint, so an
-- administrator moving a confession into theological review received a 500
-- "failed to update confession". The review workflow in directive section 22
-- could not be executed, and no test caught it: the validator was tested
-- against itself and the schema tests counted constraints without reading them.
--
-- This migration replaces the constraint with the lifecycle in
-- internal/content/lifecycle.go, which is now the single authority.
-- lifecycle_test.go reads this constraint back out of the live database and
-- compares it to the Go list, so the two cannot drift silently again.
--
-- pending_deletion and deleted are dropped from this column on purpose. They
-- belong to users.status, where internal/deletion writes them. Nothing has
-- ever written them to confessions: a confession is removed when its author's
-- account is erased, through the foreign key, not through a status change.
-- They arrived here by copy-paste across the 23 columns.

-- Any row holding a value outside the new set would make the ADD CONSTRAINT
-- fail, so normalise first. pending_deletion/deleted cannot occur (see above),
-- but a deployment that hand-edited rows should not have its migration abort.
UPDATE confessions SET status = 'archived'
 WHERE status NOT IN (
    'draft','content_review','theological_review','audio_production',
    'audio_qa','approved','published','archived','deprecated'
 );

ALTER TABLE confessions DROP CONSTRAINT IF EXISTS confessions_status_check;
ALTER TABLE confessions ADD CONSTRAINT confessions_status_check CHECK (status IN (
    'draft','content_review','theological_review','audio_production',
    'audio_qa','approved','published','archived','deprecated'
));

-- G-4: directive section 22 describes a public moderation pipeline, and
-- section 70 lists three visibility levels. community/policy.go defined two:
-- private and shared. With no PUBLIC value there was nothing for the
-- moderation pipeline to publish to, so approved UGC could only ever reach
-- the author's own circle.
ALTER TABLE community_posts DROP CONSTRAINT IF EXISTS community_posts_visibility_check;
ALTER TABLE community_posts ADD CONSTRAINT community_posts_visibility_check
    CHECK (visibility IN ('private','shared','public'));
