# PHASE 12 — CONTENT GOVERNANCE

**Branch:** `feat/phase12-governance` · **Base:** `46a8daa` (PHASE 11)
**Gaps closed:** G-5 (two unenforced vocabularies), G-6 (no `DEPRECATED` state), G-4 (`PUBLIC` visibility)
**Gate: PASS WITH CONDITIONS — 8/10**

---

## OBJECTIVE

PHASE 01 recorded that the content lifecycle existed in two vocabularies and
neither was enforced, and assigned the resolution to this phase: *"must pick
one, put it in a CHECK constraint, and migrate to it. It cannot be resolved by
adding a state."*

---

## THE CENTRAL FINDING

**It was three vocabularies, not two — and the third one made the governance
workflow unimplementable.**

| Source | Permitted states on `confessions.status` |
|---|---|
| Schema comment | `draft content_review theological_review audio_production audio_qa approved published archived` (8) |
| Go validator `validConfessionStatus` | the same 8 |
| CHECK constraint (added PHASE 07) | `draft published archived pending_deletion deleted` (5) |

Only **three** values were valid in both. I wrote a probe against a real
PostgreSQL to measure it rather than infer it:

```
draft                  both accept
content_review         API=ACCEPT  DB=REJECT  <-- mismatch
theological_review     API=ACCEPT  DB=REJECT  <-- mismatch
audio_production       API=ACCEPT  DB=REJECT  <-- mismatch
audio_qa               API=ACCEPT  DB=REJECT  <-- mismatch
approved               API=ACCEPT  DB=REJECT  <-- mismatch
published              both accept
archived               both accept
rejected               both reject
submitted              both reject
under_review           both reject
pending_deletion       API=reject  DB=accept  <-- mismatch
deleted                API=reject  DB=accept  <-- mismatch
```

**Five of the eight governance states could not be persisted.** An
administrator moving a confession into `theological_review` passed validation
in Go, was refused by the database, and the handler returned **500 "failed to
update confession"**. The review workflow directive §22 requires — content
review, theological review, audio QA — could not be executed at all.

And the same three create handlers (`category`, `confession`, `voice`) took
`req.Status` **raw, with no validation whatsoever**, so any invalid value
reached the constraint and surfaced as a 500 there too.

### Why nothing caught it

The validator was tested against itself. The schema tests *counted* constraints
rather than reading them. Both suites passed while the two vocabularies
disagreed. This is the same failure mode as the route table's `Auth` field in
PHASE 10: a record of intent mistaken for enforcement.

---

## THE FIX

### 1. One authority: `internal/content/lifecycle.go`

A new leaf package holding the nine states, `All()`, `Valid()`, and
`IsServedToNewSessions()`. `validConfessionStatus` is deleted with a comment
saying what it was and why it went.

### 2. `migrations/0003_content_lifecycle.sql`

Replaces `confessions_status_check` with the nine canonical states, normalising
any out-of-set row to `archived` first so the `ADD CONSTRAINT` cannot abort on
a hand-edited deployment.

**`pending_deletion` and `deleted` are removed from this column deliberately.**
They belong to `users.status`, where `internal/deletion` writes them. Nothing
has ever written them to confessions — a confession disappears when its
author's account is erased, through the foreign key. They arrived on this
column by copy-paste across the 23 status columns PHASE 07 constrained.

### 3. Validated at every write path

Handler (create × 2, PATCH) **and** store layer, so a caller added later cannot
reach the constraint. A vocabulary violation now returns **400**, not 500.

### 4. The parity test reads the live database

`TestLifecycleMatchesTheDatabase` queries `pg_get_constraintdef` and diffs it
against `content.All()` in **both** directions.

That choice is deliberate. The existing `sessions` parity test parses the
migration *source*. Reading source proves what was written; reading
`pg_get_constraintdef` proves what the database enforces after every migration
has run — including one a later migration silently replaced.

**I verified the test catches the defect it claims to.** Injecting a bogus
state into the Go list produces:

```
--- FAIL: TestLifecycleMatchesTheDatabase
    statuses the code accepts but the database rejects
    (these fail at write time as a 500): bishops_review
```

### 5. G-6 — `deprecated` is distinct from `archived`, and the distinction is load-bearing

- **`archived`** — withdrawn. Not served, not eligible for new sessions.
- **`deprecated`** — still playable, **and still present in sessions that
  already contain it**, but never offered to a new session.

§9 requires that a created session's queue never mutates when content changes.
Archiving a confession that a user has a scheduled session containing would
either silently rewrite their queue or break their next playback. Deprecating
it does neither. That is the whole distinction, and it is why the directive's
word was adopted rather than treating `archived` as covering it.

### 6. G-4 — the missing `PUBLIC` visibility

`community/policy.go` defined `private` and `shared`. With no `public`, the
§22 moderation pipeline had **nothing to publish to** — approved UGC could only
ever reach the author's own circle. Added `VisibilityPublic`,
`IsValidVisibility`, and `FilterPublic` (the feed the pipeline publishes to).
`FilterFeed` now serves shared + public; private stays excluded regardless of
status. Migration 0003 widens `community_posts_visibility_check` to match.

Note `community_posts.status` was already correct — its constraint matched the
Go const block exactly. UGC and editorial content *should* have different
lifecycles; the defect was never that two exist.

---

## TESTING

**8 new tests.**

`internal/content/lifecycle_test.go` (5):

| Test | What it proves |
|---|---|
| `TestLifecycleMatchesTheDatabase` | Go list and live constraint agree, both directions |
| `TestEveryLifecycleStatusIsWritable` | all 9 statuses insert into a real PostgreSQL |
| `TestInvalidStatusIsRejected` | the constraint does something — 6 bad values refused |
| `TestPendingDeletionIsNoLongerAConfessionStatus` | the deliberate removal stayed removed |
| `TestVisibilityVocabularyMatchesTheDatabase` | `private`/`shared`/`public`, exactly 3 |

`internal/api/content_lifecycle_test.go` (3):

| Test | What it proves |
|---|---|
| `TestAdminCanMoveAConfessionThroughTheWholeLifecycle` | all 9 states round-trip over HTTP with 200 — the direct G-5 regression test |
| `TestAdminRejectsAnUnknownStatusWithABadRequest` | 6 bad values → 400, never 500 |
| `TestAdminCreateRejectsAnUnknownStatus` | the creation path validates too |

**Full suite: 25 packages ok, 0 failures** (was 24 — `internal/content` is new).
**`make verify`: all checks passed, 0 lint issues.**

---

## ALSO FIXED

Two Python bytecode files were committed to the repository
(`design/__pycache__/*.cpython-313.pyc`) and `__pycache__` was not in
`.gitignore`, so every run of the design check showed them as modified.
Untracked and ignored. Generated artefacts are not source.

---

## EXIT CRITERIA

| Criterion | Result |
|---|---|
| One vocabulary, enforced in DB and code | ✅ 9 states, parity-tested both directions |
| Migration is safe on existing data | ✅ normalises before constraining |
| Every write path validates | ✅ 3 handlers + store layer |
| Bad input returns 400, not 500 | ✅ asserted |
| G-6 resolved with a real distinction | ✅ tied to the §9 snapshot invariant |
| G-4 resolved | ✅ `public` end to end |
| Full suite green | ✅ 25/25 packages |
| `make verify` | ✅ 0 issues |
| Test catches its own defect | ✅ verified by injection |

**GATE: PASS WITH CONDITIONS — 8/10**

### Conditions on this pass

1. **No transition graph is enforced.** The lifecycle is a vocabulary, not a
   state machine: nothing stops a confession going `published → draft`. This is
   deliberate — adding transitions without a workflow engine means rejecting
   legitimate corrections, and the rules would be argued in review rather than
   derived from how editors work. But "governance" is not complete until the
   path is constrained, only the positions.
2. **`deprecated` is defined but nothing acts on it.** `IsServedToNewSessions`
   exists and excludes it, but no session-building code calls that function
   yet. Until it does, a deprecated confession behaves exactly like a published
   one.
3. **The other 20 status columns were not audited.** PHASE 07 constrained 23.
   I verified `confessions` (broken), `community_posts` (correct) and
   `sessions` (already parity-tested). The remaining 20 may have the same
   copy-paste problem; they were out of scope here.
4. **No existing confession moves to a review state.** The 78 from PHASE 11 are
   all `published`. The workflow now works, but nothing has been put through
   it — which is consistent with G-35 (no theological review has happened).
5. **`PUBLIC` visibility has no moderation pipeline behind it.** The value now
   exists and `FilterPublic` can select it, but `internal/moderation` still
   does not exist (G-26). Nothing can currently promote a post to `public`.

---

## GAPS

- **G-37 (new, open)** — the content lifecycle has no enforced transition
  graph; only the vocabulary is constrained.
- **G-38 (new, open)** — `deprecated` is defined but unread: no session-building
  code calls `IsServedToNewSessions`.
- **G-39 (new, open)** — the other 20 constrained `status` columns were not
  audited against the code that writes them.
