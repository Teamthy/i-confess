# PHASE 00 — Product Source of Truth

**Status:** PASS WITH CONDITIONS
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
| Tables | 61 |
| Foreign keys | 73 |
| Unique constraints | 80 |
| Primary keys | 123 (composite keys exist) |
| Indexes | 164 |
| **Application CHECK constraints** | **1** |
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
| Test packages passing | 20 / 20 (this phase added the first test in `internal/seed`) |
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

## 5. OPEN DECISIONS — FOUNDER INPUT REQUIRED

These cannot be resolved by engineering judgement and block a clean PASS.

**D1 — Is `Deliverance` a 40th category?**
The directive lists exactly 39 and does not include `Deliverance`. An earlier
decision in this project chose `Deliverance` as a 39th category, but that was
against a 38-category baseline. Taking the directive's list as written gives 39
without it. `Freedom` already carries the description "Confessions for
deliverance and release," which may make a separate category redundant.

**D2 — What happens to `Strength` and `Thanksgiving`?**
Three options: rename them to the canonical names (requires migrating existing
rows and any user favourites pointing at them), keep them as additional
categories (→ 41), or keep them as aliases that resolve to the canonical
category.

**D3 — Does the 39 apply at launch or is it a target?**
Content exists for 14. Shipping an explore screen with 25 empty categories is
worse than shipping 14 well-filled ones. This affects PHASE 11 and the
marketing site's "39 areas of life" section.

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
| Tests pass | 5/5, and 20/20 packages overall |
| `gofmt` / `go vet` clean | Verified |
| Open decisions identified | 3 raised, **0 resolved** |

---

## PHASE REPORT

1. **Built:** canonical product definition; the 39-category list as tested code; a measured inventory of the current system.
2. **Files changed:** `docs/00-PRODUCT-SOURCE-OF-TRUTH.md` (new), `server/internal/seed/categories_canonical.go` (new), `server/internal/seed/categories_canonical_test.go` (new), `server/internal/seed/seed.go` (category list moved to package scope + mojibake), and mojibake-only repairs in `docs/API.md`, `CONTRIBUTING.md`, `server/internal/api/handlers.go`, `server/internal/adminui/index.html`, `server/internal/db/schema.postgres.sql`, `server/migrations/postgres/0001_schema.sql`, `server/internal/engine/engine_test.go`.
3. **Architecture decisions:** the canonical list lives in Go as seed-of-record data, *not* as a second source of truth beside the `categories` table. The table remains the runtime source; this is the reference for detecting drift.
4. **Database/API changes:** none.
5. **UI/UX changes:** mojibake repaired in 7 files, including user-facing seed descriptions and the admin console's nav icons.
6. **Tests added:** 5.
7. **Tests executed:** `go test ./internal/seed/ -run 'Canonical|CategorySlugs|SeededCategories' -count=1 -v` → 5/5 PASS. Then the full suite: `go test ./... -count=1` → 20/20 packages, 0 failures, against PostgreSQL 17. `gofmt -l .` empty, `go vet ./...` clean, `go build ./...` clean.
8. **Security considerations:** no new surface; see review above.
9. **Performance considerations:** the canonical list is a package-level slice read at seed time. Negligible.
10. **Known issues:** F1 (1 CHECK constraint), F3 (seed/product disagreement), F4 (6 × 501).
11. **Remaining work:** resolve D1–D3.
12. **Phase score:** 8/10. Docked because three product decisions remain open and the phase cannot close cleanly without them.
13. **Decision:** **PASS WITH CONDITIONS** — conditions are D1, D2, D3.
14. **Recommended next phase:** **PHASE 01 — Domain Model**, which needs D1–D3 answered first because the domain model has to name the category aggregate and its invariants.
