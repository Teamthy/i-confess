-- 0017_theological_review.sql — PHASE 40 / G-35.
--
-- Canonical text is repository content, not a named author or an ecclesial
-- endorsement. Review provenance must be stored separately from `author`, and
-- the closed review vocabulary must be database-enforced.
ALTER TABLE confessions ADD COLUMN IF NOT EXISTS theological_review_status TEXT NOT NULL DEFAULT 'unreviewed';
ALTER TABLE confessions ADD COLUMN IF NOT EXISTS theological_reviewer TEXT;
ALTER TABLE confessions ADD COLUMN IF NOT EXISTS theological_reviewed_at TEXT;
ALTER TABLE confessions ADD COLUMN IF NOT EXISTS theological_review_notes TEXT;

ALTER TABLE confessions DROP CONSTRAINT IF EXISTS confessions_theological_review_status_check;
ALTER TABLE confessions ADD CONSTRAINT confessions_theological_review_status_check
    CHECK (theological_review_status IN ('unreviewed','reviewed','needs_revision'));

CREATE INDEX IF NOT EXISTS idx_confessions_theological_review
    ON confessions (theological_review_status);
