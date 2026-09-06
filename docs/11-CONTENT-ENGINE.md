# PHASE 11 — CONTENT ENGINE

**Branch:** `feat/phase11-content` · **Base:** `1e1fc5a` (PHASE 10)
**Decision honoured:** D3 — all 39 categories must exist at launch
**Gate: PASS WITH CONDITIONS — 8/10**

---

## OBJECTIVE

Give the product an inventory. Decision D3 committed I CONFESS to 39 areas of
life at launch, and the marketing site's "39 areas of life" section depends on
that claim being true. This phase establishes what the library actually
contains and — more importantly — how it gets there.

---

## THE CENTRAL FINDING

**A production deployment came up with an empty catalogue.**

The content library was created by `seed.Seed`, and `Seed` is guarded:

```go
// cmd/server/main.go, before this phase
if cfg.Env == "development" || os.Getenv("SEED") == "1" {
    if err := seed.Seed(conn, objStore); err != nil {
        log.Printf("seed: %v", err)   // and merely logged, not fatal
    }
}
```

In production neither condition holds. The two migrations that do run —
`0001_baseline.sql` and `0002_status_constraints.sql` — contain **zero**
`INSERT INTO categories` and **zero** `INSERT INTO confessions` (verified by
grep). So a fresh production database had 39 columns of schema and no rows in
them.

This is not a content problem; it is a **classification** problem. Categories
and confessions were filed as demo data. They are the product's inventory. A
confession app with no confessions is not a degraded app, it is not the app.

### Fix: `seed.EnsureContent`

A new idempotent bootstrap that runs on **every boot, in every environment**:

```go
if _, _, err := seed.EnsureContent(context.Background(), conn); err != nil {
    log.Printf("content: FAILED to ensure the canonical library: %v", err)
}
```

`CreateCategory` is a plain non-idempotent `INSERT`, so `EnsureContent` reads
existing state first and only creates what is missing — categories keyed on
slug, confessions keyed on `(category_id, title)` so the same title under two
categories stays distinct.

### What it deliberately does NOT create: audio

`Seed` generates placeholder tone bytes and marks the assets `"ready"`. In a
demo that is convenient. In production it would serve users **beeps labelled
as confessions**. `EnsureContent` creates categories, confessions, duration
variants and scripture references, and stops there. A confession with no audio
asset is honest about being text-only until audio is generated.

Real audio comes from the generation pipeline behind the hard rights gate
(directive §5: GENERATE AUDIO → CHECK RIGHTS → authorised? no → STOP). A
bootstrap has no business manufacturing assets that gate is supposed to
control. This boundary is asserted by a test, not left to a comment.

---

## THE CONTENT

**78 confessions across all 39 categories — minimum two each.**

| Before | After |
|---|---|
| 16 confessions | **78** |
| 13 of 39 categories covered | **39 of 39** |
| 26 categories empty | **0 empty** |

62 confessions are new. The 16 existing were moved verbatim into the canonical
corpus so there is one authority rather than two.

Each confession carries:

- **Three lengths** — `Short`, `Medium`, `Long`. The session engine assembles a
  queue to fill a requested duration and picks the variant that best fits; a
  variant shorter than the one below it inverts that choice.
- **Four duration rungs** — 30s / 1m / 3m / 5m. A missing rung silently
  under-fills a queue built for that duration.
- **At least one scripture reference**, with `IsDirectQuote` set true only
  where the confession text quotes the verse closely enough that a reader
  comparing them would recognise the wording. Alluding to a verse is not
  quoting it, and claiming otherwise overstates the biblical warrant for words
  a user is being asked to speak aloud.
- **An intensity 1–5** — the sort key the explore screen and session engine
  both order by.

The corpus lives in `internal/seed/confessions_canonical.go` (703 lines) next
to `categories_canonical.go`, following the same pattern: content is a
reviewable, testable artefact in the repository, not rows typed into a
database by hand.

---

## TESTING

**16 tests in `internal/seed`, 10 of them new.**

Corpus invariants (`confessions_canonical_test.go`):

| Test | What it catches |
|---|---|
| `TestEveryCanonicalCategoryHasAtLeastTwoConfessions` | An empty or thin category — the launch requirement |
| `TestConfessionsOnlyReferenceCanonicalCategories` | A typo'd category name, which would silently orphan the confession |
| `TestConfessionTitlesAreUnique` | The same text pasted under two headings |
| `TestConfessionTextsAreOrderedByLength` | An inverted variant ladder the session engine depends on |
| `TestEveryConfessionCarriesScripture` | A confession with no text behind it |
| `TestIntensityIsInRange` | A value that sorts to the wrong end |

Bootstrap behaviour (`ensure_test.go`, against real PostgreSQL):

| Test | What it proves |
|---|---|
| `TestEnsureContentPopulatesAnEmptyProductionDatabase` | 39 categories and 78 confessions land, and **every** category has ≥2 |
| `TestEnsureContentIsIdempotent` | A second pass creates 0 and 0; no duplicate rows |
| `TestEnsureContentCreatesNoAudio` | `SELECT COUNT(*) FROM audio_assets` is **0** |
| `TestEnsureContentPublishesEveryDurationRung` | Every confession stored with 4 variants and ≥1 scripture ref |

**Full suite: 24 packages ok, 0 failures.** `make verify`: all checks passed,
0 lint issues.

### End-to-end proof, not just unit proof

The server binary was built and booted against a **fresh, empty, non-development
database** (`ENV=staging`, `SEED` unset — so `Seed` did not run):

```
content: ensured 39 categories, 78 confessions
i-confess backend listening on :8199 (env=staging, workers=4)
```

Resulting database state:

```
categories=39   confessions=78   scripture_refs=178
confession_variants=312          audio_assets=0   users=0
categories with <2 confessions: 0
```

312 = 78 × 4 rungs. `audio_assets=0` confirms the boundary held. `users=0`
confirms no demo accounts leaked into a non-dev environment. A **second boot**
printed no `content:` line and left the count at 39 — idempotent in the real
binary, not only in the test.

---

## SECOND FINDING — THE SEED WAS INSTALLING ACCOUNTS THE API WOULD REJECT

`Seed` created its demo users by calling `auth.HashPassword` directly:

```go
adminHash, _ := auth.HashPassword("admin12345")
userHash,  _ := auth.HashPassword("password123")
```

`password123` is on the blocklist PHASE 10 added. It passed here because the
policy lives on the register handler and this code path never touches it.
That is exactly how weak passwords survive an audit: the check is where the
user comes in, not where the row is written.

Now both go through `hashDemoPassword`, which calls `auth.ValidatePassword`
first and returns an error if the policy rejects it. A future weak demo
password fails at seed time instead of quietly creating a guessable account.

---

## EXIT CRITERIA

| Criterion | Result |
|---|---|
| All 39 categories have ≥2 confessions | ✅ 78 confessions, 0 thin categories |
| Content installs in production | ✅ verified by booting the real binary |
| Idempotent | ✅ verified in test and in a second real boot |
| No fake audio in production | ✅ asserted, and observed `audio_assets=0` |
| Full suite green | ✅ 24/24 packages |
| `make verify` | ✅ 0 issues |
| Documentation | ✅ this file + `PROJECT-STATUS.md` |

**GATE: PASS WITH CONDITIONS — 8/10**

### Conditions on this pass

1. **No audio exists for any of it.** Every one of the 78 confessions is
   text-only until the generation pipeline runs. This is correct by design, but
   it means launch still depends on PHASE 12+ producing real audio behind the
   rights gate. A user could reach a confession with nothing to play.
2. **The content is authored by me, not by a theological reviewer.** Scripture
   references were chosen and `IsDirectQuote` flags set with care, but directive
   §22 requires theological review before publication. `Author` is set to
   "i-confess content team", which overstates the provenance.
3. **`EnsureContent` failure is not fatal.** A boot can succeed with an empty
   catalogue, logged as an error. Deliberate — a crash loop is worse — but it
   means monitoring has to alert on that log line or the failure is silent.
4. **Content is versioned in code, not in the database.** Editing a confession
   now requires a deploy. That is the right trade for launch content and the
   wrong one once editors need to work through `admin.` — tracked as a
   deferred decision.
5. **78 confessions is a floor, not a library.** Two per category supports the
   marketing claim and fills a session. It is not yet the depth a subscription
   product needs to justify recurring payment.

---

## GAPS

- **G-34 (new, open)** — no audio generated for any canonical confession.
- **G-35 (new, open)** — canonical content has not had theological review
  (§22); `Author` claims a team that has not reviewed it.
- **G-36 (new, open)** — an `EnsureContent` failure boots silently; no alert
  wired to it.
