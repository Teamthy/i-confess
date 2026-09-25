-- 0020_bible.sql — the Bible reader.
--
-- Scripture is imported, never authored here: `bible-import load` writes
-- bible_versions and bible_verses from the public-domain sources described in
-- internal/bible/registry.go, after checking each file's SHA-256, parsing it
-- and verifying it against the canon. No Bible text is inserted by this
-- migration, and none is committed to the repository, so these two tables start
-- empty and stay empty until an operator runs the importer.
--
-- bible_verses is the largest table in the schema by an order of magnitude: a
-- complete Bible is about 31,000 verses, so twelve versions are roughly a third
-- of a million rows. The importer uses COPY rather than INSERT for that reason,
-- and the read path is one index rather than a scan.
--
-- The three columns added to scripture_references are what make a confession
-- link to a verse and a verse link back to a confession. They are derived,
-- normalised duplicates of what the corpus was authored with: `book` stays the
-- human spelling ("1 Peter", "Psalm") so the corpus remains readable to the
-- reviewers who check it against the text it cites, `book_id` is the canonical
-- OSIS identifier ("1Pet", "Ps"), and verse_start/verse_end turn an authored
-- range such as "22-24" into the two numbers an index can answer with. Deriving
-- them on every read instead would put a name normaliser in the query path,
-- where a spelling it has not seen becomes a verse nobody can open.

CREATE TABLE IF NOT EXISTS bible_versions (
    id            TEXT PRIMARY KEY,                       -- "kjv", "swahili"
    name          TEXT NOT NULL,
    abbrev        TEXT NOT NULL,
    language      TEXT NOT NULL,                          -- ISO 639-1
    language_name TEXT NOT NULL,
    format        TEXT NOT NULL,                          -- osis | usfx | zefania
    coverage      TEXT NOT NULL,                          -- full | new_testament
    year          TEXT,
    licence       TEXT NOT NULL,
    licence_url   TEXT,
    licence_note  TEXT,
    attribution   TEXT NOT NULL,
    blob_url      TEXT NOT NULL,
    sha256        TEXT NOT NULL,                          -- digest of the imported file
    bytes         BIGINT NOT NULL DEFAULT 0,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    is_default    INTEGER NOT NULL DEFAULT 0,
    book_count    INTEGER NOT NULL DEFAULT 0,
    chapter_count INTEGER NOT NULL DEFAULT 0,
    verse_count   INTEGER NOT NULL DEFAULT 0,
    imported_at   TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    deleted_at    TEXT,
    row_version   INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS bible_verses (
    id              BIGSERIAL PRIMARY KEY,
    version_id      TEXT NOT NULL,
    book_id         TEXT NOT NULL,                        -- canonical OSIS id
    book_name       TEXT NOT NULL,                        -- display name
    testament       TEXT NOT NULL,                        -- old | new
    canonical_order INTEGER NOT NULL,                     -- 1..66
    chapter         INTEGER NOT NULL,
    verse           INTEGER NOT NULL,
    text            TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    deleted_at      TEXT,
    row_version     INTEGER NOT NULL DEFAULT 1
);

-- The reader's query is always "this version, this chapter, in order", so the
-- index carries the sort rather than forcing a sort node on every chapter.
-- It is also unique, which makes the importer's re-import idempotent and stops
-- a second copy of a translation from doubling every verse.
CREATE UNIQUE INDEX IF NOT EXISTS bible_verses_chapter_key
    ON bible_verses (version_id, book_id, chapter, verse);

-- A single verse by reference - a deep link, a highlight, a bookmark - must not
-- read the whole chapter.
CREATE INDEX IF NOT EXISTS idx_bible_verses_ref
    ON bible_verses (version_id, book_id, chapter, verse)
    WHERE deleted_at IS NULL;

-- Bookmarks and highlights are listed in canonical order, not insertion order:
-- "Genesis 1:1, Genesis 1:3, Exodus 2:4" is the order a reader expects to find
-- their own marks in.
CREATE INDEX IF NOT EXISTS idx_bible_verses_book
    ON bible_verses (version_id, canonical_order);

-- Normalised citations, so a verse can find the confessions that point at it.
ALTER TABLE scripture_references ADD COLUMN IF NOT EXISTS book_id TEXT;
ALTER TABLE scripture_references ADD COLUMN IF NOT EXISTS verse_start INTEGER;
ALTER TABLE scripture_references ADD COLUMN IF NOT EXISTS verse_end INTEGER;

CREATE INDEX IF NOT EXISTS idx_scripture_references_verse
    ON scripture_references (book_id, chapter, verse_start, verse_end);

-- Highlights. A saved mark on a verse, scoped to the account and to the
-- translation it was made in: a yellow mark on John 3:16 in the KJV is a
-- different mark from the same colour in the Swahili New Testament, because the
-- reader made it against the words they were reading, and the two editions word
-- that verse differently.
CREATE TABLE IF NOT EXISTS verse_highlights (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    version_id  TEXT NOT NULL,
    book_id     TEXT NOT NULL,
    chapter     INTEGER NOT NULL,
    verse       INTEGER NOT NULL,
    color       TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    deleted_at  TEXT,
    row_version INTEGER NOT NULL DEFAULT 1,
    CONSTRAINT verse_highlights_color_check CHECK (color IS NULL OR color IN
        ('yellow','green','blue','pink','purple'))
);

-- One live highlight per verse per version per user. Choosing a second colour
-- changes the first mark rather than stacking a row the reader cannot see;
-- deleted rows are excluded so a cleared highlight can be made again.
CREATE UNIQUE INDEX IF NOT EXISTS verse_highlights_one_per_verse
    ON verse_highlights (user_id, version_id, book_id, chapter, verse)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_verse_highlights_user
    ON verse_highlights (user_id, deleted_at);
-- The verse-first direction: the marks on a chapter, read in one query while
-- the chapter renders.
CREATE INDEX IF NOT EXISTS idx_verse_highlights_verse
    ON verse_highlights (version_id, book_id, chapter);

-- Bookmarks. The same coordinates with the reader's own label and note, which
-- is why they are a separate table from highlights: a bookmark may exist with
-- no highlight, and it carries text of its own.
CREATE TABLE IF NOT EXISTS verse_bookmarks (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    version_id  TEXT NOT NULL,
    book_id     TEXT NOT NULL,
    chapter     INTEGER NOT NULL,
    verse       INTEGER NOT NULL,
    label       TEXT,
    note        TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    deleted_at  TEXT,
    row_version INTEGER NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX IF NOT EXISTS verse_bookmarks_one_per_verse
    ON verse_bookmarks (user_id, version_id, book_id, chapter, verse)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_verse_bookmarks_user
    ON verse_bookmarks (user_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_verse_bookmarks_verse
    ON verse_bookmarks (version_id, book_id, chapter);

-- Foreign keys. These are the three that make a stored mark meaningful: a verse
-- cannot outlive the version it was imported from, and a highlight or bookmark
-- cannot outlive the account that made it.
ALTER TABLE bible_verses ADD CONSTRAINT bible_verses_version_id_fkey
    FOREIGN KEY (version_id) REFERENCES bible_versions(id) ON DELETE CASCADE;
ALTER TABLE verse_highlights ADD CONSTRAINT verse_highlights_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE verse_bookmarks ADD CONSTRAINT verse_bookmarks_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
