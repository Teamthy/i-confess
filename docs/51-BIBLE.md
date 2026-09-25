# Ledger 51 — The Bible: importer, registry and verification

**Date:** 2026-09-25
**Branch:** `arena/01a0d8ae-i-confess`
**Depends on:** `docs/01-DOMAIN-MODEL.md` (`scripture_references`), `docs/07-DATABASE-FOUNDATION.md`
(migrations), PHASE 07 §7.1 (the store layer), `docs/40-THEOLOGICAL-REVIEW.md` (the corpus that
cites Scripture).
**Scope:** the Scripture corpus, its registry and its verification, plus migration `0020`.
Reader APIs and the two clients are the next ledger; section 9 records exactly what is not built yet.

---

## 1. What was asked for

The feature is "a Bible in the app", with seven requirements that have to hold together:

1. Twelve versions.
2. Five languages.
3. "Fully verified" — licence and data integrity, not a claim.
4. Every confession's Scripture reference opens the verse it names.
5. Verses can be highlighted.
6. Verses can be bookmarked.
7. A verse that has a confessional category or a confession written from it links back to that
   confession, and the confession can be spoken aloud.

This ledger delivers the data layer underneath all seven: the canonical table, the source registry,
the parsers, the validator, the importer, the schema and the tests. A wrong verse here would show up
as a wrong verse in every one of the seven, so verification is the substance of the work rather than
a step at the end of it.

## 2. Where the text comes from, and why not from anywhere else

Scripture is redistributed — stored, served, read aloud — so only public-domain text can ship.
The large crawled corpora were each surveyed and each rejected on their own terms:

| Source | What it has | Why it is not used |
|---|---|---|
| `thiagobodruk/bible` | 90 versions, 35 languages | README: "All the Bible versions are property of their respective owners. All rights reserved." Also 87/90 files carry Portuguese book names and three fail to parse. |
| `scrollmapper/bible_databases` | 140 translations, 56 languages | MIT repository licence, but no African languages at all and no per-translation terms. |
| `getbible/v2` | 92,462 files | Zero matches for Yoruba, Igbo, Hausa or Swahili. |
| `christos-c/bible-corpus` | CC0-1.0, 108 files | Usable, but only English and a Swahili New Testament overlap with what is needed — and both are already available from the chosen source. |
| **`seven1m/open-bibles`** | **49 translations** | **Chosen.** The only surveyed source publishing a per-file licence column: every file states `Public Domain`, `CC BY 4.0` or `CC BY-SA 4.0`. |

Twelve public-domain translations across seven languages ship. The registry
(`server/internal/bible/registry.go`) carries, per version: the licence and its URL, an attribution
line naming the upstream file, the SHA-256 of that file, its byte size and its declared coverage.

## 3. The twelve versions, and what was measured

Structural audit of every source file, against the canon (66 books / 1,189 chapters, KJV reference
distribution of 31,102 verses):

| ID | Version | Lang | Fmt | Books | Chapters | Verses | vs reference | Notes |
|---|---|---|---|---|---|---|---|---|
| `kjv` | King James Version | en | OSIS | 66 | 1189 | 31102 | 0 | baseline; the translation the corpus cites |
| `web` | World English Bible | en | USFX | 66 | 1189 | 31098 | 10 (0.03%) | Romans doxology at 14:24-26 rather than 16:25-27; 5 empty placeholders pruned (Luke 17:36, Acts 8:37, Acts 15:34, Acts 24:7, …) |
| `asv` | American Standard Version | en | Zefania | 66 | 1189 | 31102 | 0 | exact |
| `webbe` | World English Bible (British) | en | USFX | 66 | 1189 | 31098 | 10 (0.03%) | as WEB, including the same five pruned placeholders |
| `bsb` | Berean Standard Bible | en | USFX | 66 | 1189 | 31085 | −17 (0.055%) | omitted verses absent, not empty |
| `ylt` | Young's Literal Translation | en | Zefania | 27 | 260 | 7957 | 0 | New Testament |
| `swahili` | Swahili Bible | sw | OSIS | 26 | 255 | 7815 | 3 (0.04%) | no Philippians, no Matthew 23 — declared; 3 John 1:15 and Revelation 12:18 carry verses the reference does not |
| `rv1909` | Reina Valera 1909 | es | USFX | 66 | 1189 | 31084 | 18 (0.058%) | 18 empty placeholders pruned |
| `almeida` | João Ferreira de Almeida | pt | USFX | 66 | 1189 | 31098 | 12 (0.039%) | |
| `ostervald` | Ostervald Bible | fr | OSIS | 66 | 1189 | 31172 | 126 (0.404%) | largest variance shipped |
| `riveduta` | Italian Riveduta 1927 | it | OSIS | 66 | 1189 | 31102 | 0 | exact |
| `tagalog` | Tagalog Bible (Ang Dating Biblia) | tl | OSIS | 66 | 1189 | 31102 | 0 | exact |

Every file's last verse is Revelation 22:21, which is the cheapest end-to-end check that a parser
read the whole document. The `var` column is the validator's own count of verses that differ from
the reference distribution, not a percentage of a claimed total, and `dropped` is the number of
empty placeholders pruned - a source that keeps a numbered line for a verse it does not have.

**Total shipped text: 326,815 verses across twelve translations**, loaded into PostgreSQL in one
`bible-import load` run and read back through the store layer in the tests.

Six of the twelve reproduce the reference distribution exactly. The others differ by versification
the way real editions do — the WEB family moves the Romans doxology, the BSB omits verses modern
translations drop — and each difference is reported per chapter by the validator rather than being
smoothed away.

## 4. What "fully verified" means here, as a computation

`internal/bible/validate.go` is the definition; the importer will not load a version that fails it.
The rules are:

**Structural, absolute:**

- Every book the version claims to cover is present.
- Every chapter the canon has is present, unless the registry declares it missing — and a declared
  omission that the file actually contains is itself an error, so the registry cannot drift away
  from the files it describes.
- Verse numbers ascend without duplicates and are positive.
- No verse is empty. Empty placeholders are pruned during parsing rather than stored, because a
  reader must never be shown an empty verse as though it were Scripture.
- The file's SHA-256 matches the registry before any of the above runs.

**Measured, bounded:**

- Verse-count variance against the reference distribution, reported per chapter, bounded at 1.0%.
  The shipped versions land at or below 0.41%; the sources rejected in section 5 land at 1.0% and
  5.5%. The threshold sits in that gap, which is the gap between "different edition" and "broken
  import".

**Declared:**

- Coverage (`full` or `new_testament`) is a claim about the file, validated in both directions, and
  surfaced by the API and the reader: the Swahili New Testament tells the reader it has no
  Philippians instead of showing a book that opens onto nothing.

Three parser behaviours are load-bearing and are pinned by tests:

1. **Milestones and containers.** The KJV file stores one `<verse sID=.../>` per poetry line, so a
   single verse arrives in several pieces and the pieces must be merged, not overwritten. A parser
   that flushes on the element's end event produces a Bible where verses hold one fragment each.
2. **Text lives in siblings.** In USFX, `<v id="16"/>` is a milestone: the text follows as the tails
   of subsequent elements until `<ve/>`. In WEBBE and BSB every word is wrapped in a `<w>` element,
   and footnote subtrees sit inside the verse — dropping a skipped element's tail silently deletes
   the words around every footnote.
3. **Discard the non-canonical.** The KJV file has 81 book divisions: 66 canonical plus front
   matter and apocrypha. Div counts are never trusted; every file is validated against the canonical
   table, and divisions that are not canonical are reported.

## 5. What was excluded, and the measurement that excluded it

The registry records these next to the versions it ships, so the next person to consider one of
these files sees what happened:

- **`eng-bbe.usfx.xml` (Bible in Basic English)** — truncated upstream. The file ends mid-document
  after Revelation 22:21 with no closing tags. Re-downloading returns the identical SHA-256, so the
  defect is in the published file, not in the download.
- **`eng-dra.zefania.xml` (Douay-Rheims 1899)** — chapter numbering does not follow the canon: the
  file labels the Hebrew letter heading "Aleph" as Psalm 119:1 and shifts the psalm's 176 verses
  into Psalm 120; Psalms has 152 chapters and Daniel 14. 760 verses (2.4%) sit in a chapter the canon
  numbers differently, so every deep link into the Psalms would open the wrong text.
- **`eng-gb-oeb.osis.xml`, `eng-us-oeb.osis.xml` (Open English Bible)** — 42 books: a complete New
  Testament plus 15 Old Testament books. Shippable behind the same declared-coverage mechanism the
  Swahili file uses, but the twelve slots buy more as complete Bibles in other languages.
- **`deu-luther1912.osis.xml` (Luther Bible 1912)** — structurally different versification: Joel has
  4 chapters against the canon's 3, Malachi 3 against 4, and individual chapters differ by up to 16
  verses. Requires a per-version chapter mapping that does not exist.
- **`lat-clementine.usfx.xml` (Clementine Vulgate)** — 1,197 chapters against 1,189, because Psalms
  9/10 and 113/114/115 are divided differently. 1,735 verses (5.5%) are affected.

### The languages that are not here

Yoruba, Igbo and Hausa were searched for on every reachable source and **no public-domain text
exists**. The modern editions are Bible Society copyright; the repositories that carry them state
that the translations remain the property of their owners. eBible.org, which hosts the largest
collection of African-language Scripture, is not reachable from this environment at all.

So the Nigeria-first language set is served as far as verifiable text allows: English and Swahili
ship now, and Yoruba, Igbo and Hausa are a data drop — one registry entry, one fetch, one verify,
one load each — the moment licensed text is available. Nothing in the feature is language-specific:
the reader, the reference normaliser, highlights, bookmarks and the confession cross-links all work
per version, so a new language changes the picker and nothing else.

## 5a. What the first real import found

Verification is only worth what it catches. Running `bible-import verify` and then `load` against
the twelve real files for the first time found four defects that no amount of reading the parser
would have surfaced, and each is now impossible to reintroduce quietly:

1. **Zefania files parsed as zero chapters.** `<CHAPTER cnumber="3">` numbers its chapters in an
   attribute of its own, and the reader looked only for OSIS's `osisID`/`sID`. Every verse then had
   no chapter to belong to, and the American Standard Version and Young's Literal Translation
   "verified" as 66 books, 0 chapters, 0 verses — silent emptiness that the validator then reported
   as 67 errors. Fixed, and `asv` now reproduces the reference distribution exactly.
2. **Empty book divisions counted as present books.** The Swahili New Testament carries a division
   for every Old Testament book with nothing under it, so a coverage check that counted divisions
   concluded the file contained the whole canon and rejected it. Divisions with no text are now
   pruned during parsing and reported (`EmptyDivisions`), so the reader cannot open a book with no
   verses and an operator can see the source is a partial one.
3. **The "New Testament only" check was wrong by construction.** It asked whether any required book
   was missing, and a New Testament never lacks a New Testament book — so it failed every correct
   file. It now asks whether the file carries Old Testament books, which is the actual claim:
   a New Testament that also has the Old is a complete Bible and must not be labelled otherwise.
4. **`is_default` bound a Go bool to an INTEGER column.** Postgres refuses `true` as an integer, so
   the first real `load` failed on its first insert. A test with a stub database would not have
   caught it; a real one did, immediately.

The four are the reason the import is run end to end rather than trusted: the first two produce a
Bible that looks plausible and is wrong, which is the failure mode this feature cannot have.

## 6. Data model

Migration `0020_bible.sql`:

- **`bible_versions`** — the registry as rows: licence, licence URL and note, attribution, blob URL,
  checksum, byte size, coverage, counts, import time.
- **`bible_verses`** — the text, one row per verse per version, with the canonical book id, display
  name, testament and canonical order denormalised so the reader's query is one index scan and no
  joins. This is the largest table in the schema by an order of magnitude: a complete Bible is
  ~31,000 verses and a full import is ~3 million rows, which is why the importer uses `COPY` and
  why the read path is three indexes rather than a scan.
- **`scripture_references`** gains `book_id`, `verse_start`, `verse_end` — normalised alongside the
  authored `book`/`chapter`/`verse` text. Both are kept deliberately: rewriting the corpus's human
  spellings would make the text unreadable to the reviewers who check it against the verses it
  cites, and normalising on every read would put a name matcher in the query path where an unseen
  spelling becomes a verse nobody can open.
- **`verse_highlights`** and **`verse_bookmarks`** — the reader's own marks, scoped to the version
  they were made in, unique per verse while live, with soft delete and row versioning like every
  other application table, and erase-on-account-deletion policies.

## 7. Operating it

```
bible-import fetch  -sources ./bible-src          # download, verify checksums, delete mismatches
bible-import verify -sources ./bible-src [-json]  # checksum + parse + validate every version
bible-import fixture -out internal/bible/testdata/cited-verses.osis.xml
bible-import load   -sources ./bible-src -dsn "$DATABASE_URL"
```

`load` verifies every selected version before writing any of them: a half-imported set would leave
some translations current and others stale with nothing in the data saying which. Each version is
written in one transaction — version row upsert, delete, `COPY` — so a reader mid-request sees the
old text or the new text and never a chapter emptied by a delete.

`BIBLE_SOURCES` sets the default source directory. The sources are **not** in Git: twelve
translations are hundreds of megabytes and the checksum in the registry is what makes a re-download
safe.

## 8. Tests

- `canon_test.go` — the canonical table as data: shape, testament split, spot checks on the
  chapters transcription errors hit first (Psalm 119's 176 verses, Obadiah's 21), book-name
  normalisation across the spellings the corpus uses, USFM codes and Roman numerals, verse-range
  expansion, and rejection of references outside the canon.
- `fixture_test.go` — the committed 49KB fixture is the verses the canonical corpus actually cites,
  sliced from the KJV. `TestEveryCorpusCitationResolves` walks all 178 Scripture references in the
  corpus, normalises each one, checks the translation it names is one the registry ships, and fails
  if the verse is not in the fixture. Adding a confession that cites a verse nobody imported breaks
  the build, which is the only reliable way to keep a citation and a corpus in step.
- `TestFixtureTextIsVerbatim` compares every fixture verse against the full KJV source when
  `BIBLE_SOURCES` is set, so an operator with the corpus gets the stronger check that the slice was
  not edited in transit.

## 9. What is not built yet

Recorded plainly, because the feature is not done:

1. **The reader in the web app and the mobile app** — book/chapter/verse navigation with the version
   picker, highlight and bookmark affordances, and the coverage label ("New Testament, without
   Philippians"). The API they read is built (`/bible/*`, `/me/bible/*`).
3. **Speaking a confession** — synthesising the confession's text through the existing voice
   pipeline on demand, cached as an audio asset, triggered from the verse and the confession.
4. **Admin surfacing** — the verification report for each imported version, so an operator can see
   what was loaded and when.
