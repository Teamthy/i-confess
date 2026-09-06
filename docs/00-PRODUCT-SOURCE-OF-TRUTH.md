# PHASE 00 — Product Source of Truth

**Status:** PASS
**Date:** 2026-09-06
**Branch base:** `main` @ `cbdee9d`

---

## OBJECTIVE

Establish one authoritative statement of what I CONFESS is, what it is not, and
what it contains — so that every later phase is built against the same product
rather than against whatever the most recent contributor assumed.

Before this phase the repository held 22 documents and **none of them defined
the product**. They described authentication, audio, deployment, scheduling.
Nothing stated the canonical category list, the core loop, or the non-goals.
That is the gap this phase closes.

## INPUTS

- The Master Build Directive (90 sections, 81 phases)
- The repository at `main` @ `cbdee9d`, inspected directly
- A live PostgreSQL 17 with the committed schema loaded

## DEPENDENCIES

None. This is the first phase.

---

## 1. WHAT THE PRODUCT IS

I CONFESS is a daily audio ritual in which a user speaks biblical confessions
over specific areas of their life.

The loop:

```
DISCOVER → CHOOSE → CREATE → SCHEDULE → LISTEN → CONFESS → COMPLETE → RETURN
```

The distinguishing claim is not "we have an audio library." It is that
confession becomes a **scheduled, repeatable, personal practice** rather than
something occasional.

### Non-goals

These are stated explicitly because each one looks like a plausible feature and
each one would dilute the product:

| Not this | Because |
|---|---|
| A church CMS | No parish administration, no member directories |
| A Bible-quote generator | Scripture serves confession; it is not the output |
| An AI chatbot | No conversational theology. AI, if it arrives, generates *sessions* |
| A social network | Community is evidence of transformation, not a feed |
| A generic meditation app | The object of the practice is God's Word, not breath |

## 2. THE CANONICAL CATEGORY LIST — 39

The directive fixes this at 39 and names them. They now live in code as
`CanonicalCategories` in `server/internal/seed/categories_canonical.go`, guarded
by tests that fail if the code and the directive diverge.

They are **backend-configurable at runtime** — stored in the `categories` table
and served by the API. Mobile never hard-codes them. The canonical list is the
*seed of record*: what a fresh deployment should contain, and the reference
against which drift is detected.

Order is deliberate, leading with what people reach for when in trouble:

| # | Category | # | Category | # | Category |
|---|---|---|---|---|---|
| 1 | Healing | 14 | Protection | 27 | Children |
| 2 | Health | 15 | Career | 28 | Parenting |
| 3 | Finance | 16 | Business | 29 | Direction |
| 4 | Wealth | 17 | Leadership | 30 | Creativity |
| 5 | Breakthrough | 18 | Favor | 31 | Productivity |
| 6 | Marriage | 19 | Provision | 32 | Emotional Strength |
| 7 | Relationships | 20 | Confidence | 33 | Rest |
| 8 | Faith | 21 | Discipline | 34 | Gratitude |
| 9 | Peace | 22 | Joy | 35 | Forgiveness |
| 10 | Family | 23 | Hope | 36 | Love |
| 11 | Purpose | 24 | Freedom | 37 | Overcoming Fear |
| 12 | Identity | 25 | Spiritual Growth | 38 | Success |
| 13 | Wisdom | 26 | Prayer | 39 | Destiny |

Each carries a `Description` (category detail screen) and a `Tagline`
(website category rail). Both are asserted non-empty by test, because an empty
tagline renders as a broken card rather than an obvious error.

## 3. CURRENT STATE — MEASURED, NOT ASSUMED

Every figure below was read from the repository or queried from `pg_catalog`
after loading `schema.postgres.sql` into a scratch database.

### Database

| Metric | Value |
|---|---|
| Tables | 64 |
| Foreign keys | 76 |
| Unique constraints | 80 |
| Primary keys | 123 (composite keys exist) |
| Indexes | 164 |
| **Application CHECK constraints** | **5** (was 1; +4 added by this phase) |
| Enum / domain types | 0 |
| Triggers | 0 |

### Code

| Metric | Value |
|---|---|
| Internal modules | 37 |
| Routes registered | 248 |
| Handler methods | 173 |
| Handlers still returning 501 | 6 |
| Test files | 43 |
| Test packages passing | 21 / 21 (this phase added the first tests in `internal/seed` and `internal/community`) |
| CI on `main` | **green** (first time in repo history) |

### Content

| Metric | Value |
|---|---|
| Canonical categories | 39 |
| Categories actually seeded | 14 canonical + 2 non-canonical |
| Coverage | **14 / 39** |

## 4. FINDINGS

Four things surfaced during inspection that were not previously known.

### F1 — Only one application-level CHECK constraint exists

The schema declares 22 `status` columns, all `text`. Exactly one —
`sessions.status` — constrains its values. The other 21 will accept any string:

```sql
UPDATE confessions SET status = 'BANANA';  -- succeeds today
```

Querying `pg_constraint` returned three `contype='c'` rows, but two
(`cardinal_number_domain_check`, `yes_or_no_check`) belong to PostgreSQL's own
`information_schema` domains. One is ours.

There are also zero enum types. This directly weakens directive §25
("constraints") and §53 ("use database constraints"). **It does not need fixing
in this phase**, but it is now on the record and must be closed before launch.

> Note: an earlier report in this project claimed 94 CHECK constraints. That
> figure was wrong and is retracted here. The authoritative count is 1.

### F2 — Double-encoded UTF-8 was committed to the repository

UTF-8 text had been decoded as cp1252 and re-encoded as UTF-8, in **7 files**.
Em-dashes became `â€”`, `≥` became `â‰¥`, `§` became `Â§`, box-drawing
characters became `â”‚`, and the admin console's icons and middle dot were
mangled.

| File | Visible effect if shipped |
|---|---|
| `server/internal/seed/seed.go` | **user-facing category descriptions** |
| `server/internal/adminui/index.html` | nav icons, page title |
| `docs/API.md` | password rule read `â‰¥ 8 chars` |
| `server/internal/api/handlers.go` | `PRD Â§22` in a comment |
| `server/internal/db/schema.postgres.sql` | schema comment |
| `server/migrations/postgres/0001_schema.sql` | migration comment |
| `CONTRIBUTING.md` | branch diagram |

**Fixed in this phase.** A byte-level sweep for double-encoded UTF-8 lead
sequences now returns **0 hits repo-wide**, and the repaired schema still
loads all 61 tables.

Worth recording because the first two attempts were wrong and each looked
successful: the initial fix handled only the em-dash pattern, and the second
used a character allowlist whose range started at `\u00c2` — which excluded
`¥` (`\u00a5`), so the `≥` sequence was truncated and silently skipped. The
final repair uses maximal non-ASCII runs and handles cp1252's five undefined
byte slots (`81 8D 8F 90 9D`), which is why one admin icon needed a third
pass. The lesson is that a repair verified by "no errors" is not a repair
verified by inspection.

### F3 — The seed and the product disagree on two categories

The seed installs `Strength` and `Thanksgiving`. The canonical list has
`Emotional Strength` and `Gratitude`. These are not the same category under two
names; the descriptions differ. This needs a product decision (see §5).

### F4 — Six handlers still return 501

`internal/api/home.go` registers routes for `recommendations`, `subscription`,
`entitlements`, `confession QA`, `the moderation queue`, and
`user confession review` that return `notImplemented`. These map to directive
§34 (personalization), §35 (subscriptions) and §22 (moderation).

## 5. DECISIONS — RESOLVED BY THE PRODUCT OWNER

**D1 — `Deliverance` is not a 40th category.** The canonical list stands at 39
as written in the directive. `Freedom` carries the deliverance description. No
code change was needed; the tests already asserted 39.

**D2 — `Strength` and `Thanksgiving` are renamed** to `Emotional Strength` and
`Gratitude`, matching the canonical list. This needs a migration for any
environment already seeded, and it means the seed no longer installs categories
that are not in the canonical set.

**D3 — all 39 must exist at launch.** This makes PHASE 11 (Content Engine)
substantially larger than a 14-category seed: 25 categories need confessions
before launch, and the marketing site's "39 areas of life" section can make its
claim honestly.

**Process — every phase is gated.** Each phase stops and reports with a
PASS / PASS WITH CONDITIONS / FAIL decision before the next begins.

---

## 6. F5 — THREE TABLES WERE MISSING FROM THE SCHEMA THAT ACTUALLY RUNS

Found while checking whether `server/migrations/postgres/` was live. It is not:
**nothing in the application applies that directory.** The server runs the
embedded `schema.postgres.sql` and nothing else. So the three tables defined
only there never existed on a fresh deployment.

| Table | Queried by |
|---|---|
| `subscription_plans` | `internal/store/plans.go` |
| `community_posts` | `internal/community/store.go` |
| `community_reactions` | `internal/community/store.go` |

Those migrations were also **SQLite DDL in a directory named `postgres/`** —
`INSERT OR IGNORE`, `datetime('now')`, `TEXT` timestamps. And
`community/store.go` issued `INSERT OR IGNORE`, which PostgreSQL rejects
outright.

It survived the PostgreSQL migration because `internal/community` and
`internal/billing` have **no tests**. The suite was 20/20 green and none of it
touched this code. A green suite over untested packages proves nothing about
them.

### Fixed

- All three tables ported into `schema.postgres.sql` with PostgreSQL DDL,
  including CHECK constraints on `interval`, `visibility`, `status` and
  `reaction` (+4 application CHECK constraints, the first added since the
  schema was written).
- `INSERT OR IGNORE` replaced with `ON CONFLICT (post_id, user_id, reaction) DO
  NOTHING`, matching the table's unique key.
- `internal/community` gained four integration tests against PostgreSQL:
  moderation gating, private-post exclusion, reaction idempotency, and the
  reaction CHECK constraint.

### Two guards so this cannot recur

`TestEveryTableReferencedInGoExistsInTheSchema` parses every Go file with
`go/parser`, extracts string literals passed to `Query`/`Exec`/`Prepare`, and
asserts each referenced table exists in the schema. Anchoring on the call site
matters: scanning every literal also matches route descriptions like
`"Update profile"`, which a table-name regex reads as an UPDATE against a table
called `profile`. Verified to bite — removing the three tables fails it with
their names and file locations.

Adding those tables then broke `TestEveryUserTableHasAPolicy`, which is the
deletion framework doing its job: `community_posts` and `community_reactions`
reference `users(id)` and had no erasure policy. Adding the policies exposed a
**second, pre-existing bug**: `applyPolicy` scopes any non-`user_id` column
through a parent table with `WHERE col IN (SELECT id FROM parent WHERE user_id
= ?)`, which asks `community_posts` for a `user_id` column it does not have.
`parentTableFor` now reports `users` as the parent and `applyPolicy` deletes
directly in that case.

`TestEveryPolicyGeneratesRunnableSQL` executes every policy through the real
`applyPolicy` inside a rolled-back transaction. An earlier draft rebuilt the
DELETE by hand and reported false failures for `Anonymise` policies, which
never issue a DELETE — testing a copy of the logic tests the copy.

---

## TESTING

Five tests added in `server/internal/seed/categories_canonical_test.go`:

| Test | What it protects |
|---|---|
| `TestCanonicalCategoryCountIsTheAgreedNumber` | Count is 39 |
| `TestCanonicalCategoriesMatchTheDirectiveExactly` | Code and directive cannot drift — names each difference |
| `TestCanonicalCategoriesAreWellFormed` | No empty fields, no duplicate names/slugs, slug = kebab(name), icon = slug |
| `TestCategorySlugsInCanonicalOrder` | Helper returns a copy, so a caller sorting it cannot reorder the canonical list |
| `TestSeededCategoriesAgainstCanonical` | Reports seed coverage; does not gate |

The anti-drift test deliberately duplicates the directive's 39 names rather
than deriving them from `CanonicalCategories`. A test that derives its
expectation from the thing it checks proves nothing.

The coverage test reports rather than fails. Bringing the seed to 39 is
content-engine work, and a red suite would hide the gap instead of showing it:

```
seed coverage: 14 of 39 canonical categories are seeded
not yet seeded (25): Health, Wealth, Relationships, Career, Leadership,
  Provision, Confidence, Discipline, Hope, Freedom, Spiritual Growth, Prayer,
  Children, Parenting, Direction, Creativity, Productivity, Emotional Strength,
  Rest, Gratitude, Forgiveness, Love, Overcoming Fear, Success, Destiny
seeded but not canonical (2): Strength, Thanksgiving
```

## SECURITY REVIEW

No attack surface introduced. This phase adds seed data and tests, no endpoints,
no queries, no user input. The canonical list is server-side only, which is what
keeps the directive's rule — *never hard-code the 39 categories into mobile* —
true.

One relevant note: `seed.go` runs at startup. Category names and descriptions
flow into the database and then into API responses, so they are rendered
client-side. They are developer-authored, not user-authored, so injection risk
is limited to a compromised build.

## DOCUMENTATION

- This file — `docs/00-PRODUCT-SOURCE-OF-TRUTH.md`
- `server/internal/seed/categories_canonical.go` — package comment explains why
  the list exists alongside the `categories` table

## EXIT CRITERIA

| Criterion | State |
|---|---|
| Product definition written | Done |
| Non-goals stated | Done |
| Canonical 39 categories in code, tested | Done |
| Current state measured from evidence | Done |
| Tests pass | 10/10 new tests, and 21/21 packages overall |
| `gofmt` / `go vet` clean | Verified |
| Open decisions identified and resolved | 3 raised, **3 resolved by the product owner** |

---

## PHASE REPORT

1. **Built:** canonical product definition; the 39-category list as tested code; a measured inventory of the current system.
2. **Files changed:** `docs/00-PRODUCT-SOURCE-OF-TRUTH.md` (new), `server/internal/seed/categories_canonical.go` (new), `server/internal/seed/categories_canonical_test.go` (new), `server/internal/seed/seed.go` (category list moved to package scope + mojibake), and mojibake-only repairs in `docs/API.md`, `CONTRIBUTING.md`, `server/internal/api/handlers.go`, `server/internal/adminui/index.html`, `server/internal/db/schema.postgres.sql`, `server/migrations/postgres/0001_schema.sql`, `server/internal/engine/engine_test.go`.
3. **Architecture decisions:** the canonical list lives in Go as seed-of-record data, *not* as a second source of truth beside the `categories` table. The table remains the runtime source; this is the reference for detecting drift.
4. **Database/API changes:** none.
5. **UI/UX changes:** mojibake repaired in 7 files, including user-facing seed descriptions and the admin console's nav icons.
6. **Tests added:** 10 (5 category, 4 community, 1 policy-SQL guard) plus 1 schema-coverage guard in `internal/db`.
7. **Tests executed:** `go test ./internal/seed/ -run 'Canonical|CategorySlugs|SeededCategories' -count=1 -v` → 5/5 PASS. Then the full suite: `go test ./... -count=1` → 20/20 packages, 0 failures, against PostgreSQL 17. `gofmt -l .` empty, `go vet ./...` clean, `go build ./...` clean.
8. **Security considerations:** no new surface; see review above.
9. **Performance considerations:** the canonical list is a package-level slice read at seed time. Negligible.
10. **Known issues:** F1 (only 5 application CHECK constraints for 22 status columns), F3 (2 seed categories still need the D2 rename), F4 (6 × 501 handlers), and `server/migrations/postgres/` is a dead directory the app never applies — it should be deleted or made authoritative, which is a separate decision.
11. **Remaining work:** apply the D2 rename with a migration; delete or adopt `server/migrations/postgres/`.
12. **Phase score:** 9/10. Docked one point because D3 makes PHASE 11 materially larger and that work is not yet scoped.
13. **Decision:** **PASS**.
14. **Recommended next phase:** **PHASE 01 — Domain Model.** D1–D3 are settled, so the category aggregate and its invariants can now be named without guessing.
