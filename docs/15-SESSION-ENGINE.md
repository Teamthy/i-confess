# PHASE 15 — SESSION ENGINE

## OBJECTIVE

Audit the Session Engine against the directive — creation, configuration,
validation, state transitions, content selection, duration planning, voice
selection, snapshots, completion, cancellation, interruption — and close what
the audit found. Also close **C-1** carried from PHASE 14: the engine did not
prefer the current content version.

## INPUTS

`docs/00`–`14`, the directive's PHASE 15 definition, and the code already in
`internal/sessions`, `internal/engine`, `internal/store/sessions.go`,
`internal/store/audio.go` and `internal/api/sessions_playback.go`.

## DEPENDENCIES

PHASE 13 (audio infrastructure: asset statuses, served-status authority) and
PHASE 14 (voice platform: content versioning, generation, QA). Both are what
make two of the three findings below reachable at all — before versioning
existed there was no "superseded render" to select wrongly, and before the QA
workflow existed the item vocabulary had no reason to grow.

---

## Findings

### 1. The engine served the OLDEST render (closes PHASE 14 C-1)

`AudioStore.AssetsFor` ordered `created_at, id` **ascending**, and
`engine.matchAsset` falls back to `assets[0]` when no asset matches the variant
being packed. Ascending order plus a first-element fallback means "oldest wins".

**Proved, not inferred.** A throwaway probe inserted a real `content_versions`
v2 row and a newer `ready` asset for the same confession, then created a session
through the HTTP API. The engine still returned the pre-existing asset
(`103a3300…`), not the new render.

The consequence: edit a confession, re-voice it, and **every new session plays
audio rendered from the superseded text**. The listener hears words that are no
longer what the confession says, and nothing in the system surfaces it.

### 2. Variant matching was dead code

`audio_assets` had **no `variant_id` column**. `assetColumns` and the
`AssetsFor` select both filled the field with a `''` literal, so
`matchAsset`'s variant branch — the specific, correct match — could never
succeed. Every selection fell through to the positional fallback above. Its doc
comment described a branch that could not run.

This matters because generation is variant-specific (`voice.textForVariant`): a
confession voiced at several lengths produced several renders that the engine
then treated as interchangeable.

### 3. `session_items.status` was constrained to a vocabulary the code no longer writes

Migration `0002` moved `sessions.status` onto the canonical state machine.
`session_items.status` was left on the original three-value list:

```
CHECK (status IN ('queued','played','skipped'))
```

while `internal/sessions` had moved to `QUEUED / PLAYING / COMPLETED / SKIPPED /
FAILED`. Every write the playback lifecycle makes was therefore rejected:

| Call site | Writes | Result |
|---|---|---|
| `startSession` → `SetPlayingItem` | `PLAYING` | CHECK violation, error discarded with `_ =` |
| `completeSession` → `UpdateItemStatus` | `COMPLETED` | CHECK violation, error discarded with `_ =` |
| `skipSessionItem` → `UpdateItemStatus` | `SKIPPED` | CHECK violation → **500** |
| `syncProgress` → `UpdateItemStatus` | normalised | CHECK violation → **500** |

`SetPlayingItem`'s demotion query also filtered on `status='QUEUED'`, which
never matched the stored lowercase `'queued'`.

The two swallowed errors are the reason this survived: **a listener could play
and complete a session and the database would still say nothing had been
played.** `GET /sessions/{id}/queue` always reported every item waiting and
`items_completed` as 0, however far through the session the listener got.
`completeSession` computed its count in memory, so it reported a number the
table did not hold — the completion screen said something untrue, and a
subsequent queue read contradicted it.

### 4. `syncProgress` accepted another user's queue item

`skipSessionItem` deliberately verifies the submitted item belongs to the session
being addressed. `syncProgress` did not — it passed `queue_item_id` straight to
`UpdateItemStatus`, which is keyed on the item alone. A user with any session of
their own could rewrite an item in someone else's session, or claim it as their
own resume point, through a session its owner never authorised.

This was masked by finding 3: the write 500'd before it could do damage. Fixing
the vocabulary without fixing the boundary would have made it live.

---

## IMPLEMENTATION

### 1. Version-aware, variant-aware asset selection

**`0005_audio_variant.sql`** adds `audio_assets.variant_id`, drops the old
five-column unique key and restates it as
`(content_id, content_version_id, voice_id, variant_id, asset_type,
quality_tier)`, plus `idx_audio_assets_selection`. Adding a column to a unique
key makes the constraint *looser*, so it cannot fail on existing rows. The
variant column is never written NULL: Postgres treats NULLs as distinct in
unique indexes, so a nullable key column would silently permit duplicates.

**`store.AssetsFor`** now `LEFT JOIN content_versions` and orders
`cv.version_number DESC NULLS LAST, a.created_at DESC, a.id` — newest content
version, then newest render, then id for stability. Unattributed legacy assets
sort **last** rather than first, so a versioned render always wins over one that
predates versioning. It reads the real `variant_id` instead of a literal.

`UpsertAsset` persists `VariantID` (18 placeholders, six-column conflict
target), so a render is attributable to the variant and version it was made from.

### 2. The item vocabulary reaches the database

**`0006_session_item_status.sql`** folds legacy rows onto the canonical
vocabulary — `played` means the item ran to the end, so it is `COMPLETED`, not
`SKIPPED` — then restates the CHECK over the five canonical states and moves the
column default from `'queued'` to `'QUEUED'`. The order matters: `upper()` alone
would produce `PLAYED`, which is not a state.

`store.CreateSession` and `store.UpdateItemStatus` now fold spellings through
`sessions.NormalizeItemStatus` rather than passing them to the constraint. A
caller cannot fail the CHECK on a status the state machine already understands,
and one it does not understand is **rejected with an error**, not quietly stored
as something else. Fixing the four call sites alone would have left the fifth
caller free to reintroduce the fault.

The two stale `"queued"` literals in `engine.go` and `store/sessions.go` were
replaced with `string(sessions.ItemQueued)`.

### 3. The session boundary is enforced on progress

`syncProgress` now establishes that a client-supplied `queue_item_id` belongs to
the addressed session *before* any write, via the new `Handler.itemInSession`.
The check covers both the item status update and the stored resume point. An
`item_status` without a `queue_item_id` is now a 400 rather than a silent no-op.

---

## VERIFIED SOUND — NOT RE-IMPLEMENTED

- **All twelve session routes exist with `/v1/` twins** (create, get, patch,
  start, pause, resume, complete, queue, progress, skip, preview, list).
- **`updateSessionStatus` is correct**: ownership check, vocabulary validation,
  `sessions.CanTransition`, 409 `INVALID_TRANSITION` naming from-state and
  to-state, and it returns the canonical spelling so clients converge.
- **Every playback handler goes through `ownSession`** — no IDOR at the session
  level.
- **The 11-state machine** with terminal `CANCELLED`, and snapshots that survive
  content deletion (`confession_id` is `TEXT` with no FK).
- **All five planning strategies** with `BALANCED` default, and the plan ceiling
  enforced inside `Engine.Build` for all four callers.

---

## TESTING

Six new tests, all of which fail when their fix is reverted.

| Test | Covers |
|---|---|
| `TestNewestVersionIsSelected` | finding 1, over HTTP |
| `TestAssetsAreSelectedByVariant` | finding 2 |
| `TestPlaybackLifecyclePersistsItemStatuses` | finding 3 — start → skip → complete, read back from the table |
| `TestQueueCountsMatchTheTable` | finding 3 — the view a player renders from |
| `TestProgressCannotRewriteAnotherUsersQueue` | finding 4 |
| `TestProgressStillRecordsYourOwnQueue` | finding 4's fix did not break the legitimate push |

`TestPlaybackLifecyclePersistsItemStatuses` reads `session_items` directly
rather than a response body, and asserts the number `completeSession` **reports**
equals the number the table **holds** — the specific way finding 3 could hide.

**Defect injection — four for four:**

| Injection | Result |
|---|---|
| `ORDER BY a.created_at, a.id` restored | `TestNewestVersionIsSelected` FAIL |
| `COALESCE(a.variant_id,'')` → `''` in `AssetsFor` | `TestAssetsAreSelectedByVariant` FAIL |
| item CHECK reverted to the three-value list | `TestPlaybackLifecyclePersistsItemStatuses` FAIL |
| `itemInSession` guard removed | `TestProgressCannotRewriteAnotherUsersQueue` FAIL |

The second injection initially passed, which is how the audit found that
`AssetsFor` carries its own select list rather than sharing `assetColumns` —
the first injection targeted the wrong line and proved nothing until it was
redirected.

Full suite: **26 packages, 0 failures.** `make verify`: **all checks passed.**

---

## SECURITY REVIEW

- **IDOR closed** on `POST /sessions/{id}/progress` (finding 4).
- No new endpoints, no new auth surface, no new dependencies.
- `0005` restates a unique key in the looser direction; `0006` rewrites existing
  rows only to fold stale spellings onto their canonical equivalent, so no row
  changes meaning.
- Progress writes remain idempotency-wrapped and are resolved by timestamp, so a
  second device still cannot rewind the first.

## DOCUMENTATION

This file. `docs/PROJECT-STATUS.md` updated with the PHASE 15 verdict and the
closed items.

---

## EXIT CRITERIA

| Criterion | Status |
|---|---|
| Creation, configuration, validation present and tested | PASS |
| State transitions centralised and enforced | PASS — verified, 409 with reason |
| Content selection picks the current version and the right variant | PASS — findings 1, 2 fixed |
| Duration planning: all strategies, plan ceiling | PASS — verified, unchanged |
| Voice selection and downgrade | PASS — verified, unchanged |
| Snapshots survive content change and deletion | PASS — verified, unchanged |
| Completion persists what was heard | PASS — finding 3 fixed |
| Cancellation and interruption reachable | PASS — 11 states, resume from PAUSED and INTERRUPTED |
| PHASE 14 C-1 closed | PASS |
| No IDOR on any session endpoint | PASS — finding 4 fixed |
| Tests fail without the fix | PASS — 4 of 4 injections |
| `make verify` clean | PASS |

## VERDICT

**PASS WITH CONDITIONS — 8/10**

Conditions carried forward:

- **C-1** `AssetsFor` maintains its own select list instead of sharing
  `assetColumns`, so the two can drift — exactly how one defect injection passed
  while targeting the wrong line. Consolidate them.
- **C-2** Generation is still synchronous (PHASE 16), so a variant-specific
  render is produced in-request rather than by a worker.
- **C-3** No client exercises `syncProgress` yet (`apps/mobile` cannot play
  audio — G-12), so the multi-device timestamp resolution is tested only at the
  API boundary.
- **C-4** ElevenLabs remains the only voice provider (carried from PHASE 14).

---
---

# APPENDIX — groundwork delivered earlier under the misnumbered label

The material below is the PHASE 15 groundwork shipped as **PR #22** while the
phase numbering was still wrong, filed at the time as "PHASE 13 — SESSION
ENGINE". It is preserved unedited; the header is restated to match the
authoritative numbering.

## Objective (as filed)

Harden the Session Engine against plan-limit bypass, close gap **G-38**
(hardcoded served-status in the store), and verify the snapshot and state
machine contracts.

---

## Finding

**`Engine.Build` had four callers but only one validated duration.**

`internal/engine.Engine.Build` is the single entry point that constructs a
session queue. Its callers:

| Caller | Route | Validated duration? |
|---|---|---|
| `createSession` | `POST /sessions` | Yes |
| `createSessionPreview` | `POST /sessions/preview` | **No** |
| `saveTemplateFromRequest` | `POST /templates` | **No** |
| `triggerSchedule` | `POST /schedules/{id}/start` | **No** |

Duration is a **plan capability**: `entitlements.Audio.MaxSessionSeconds()`
returns 900 s for free and 10 800 s for premium (§35 — the server determines
entitlement). Only `createSession` read it.

**Proved by a throwaway test over HTTP as a free user (900 s limit):**

| Request | Before |
|---|---|
| `POST /sessions` at 10 800 s | **402** `session_duration_exceeds_plan_limit` |
| `POST /schedules` at 10 800 s | **201 accepted** |
| `POST /schedules/{id}/start` | **201 — `target=10800 actual=10800`** |

A free user got a full 3-hour session, **12× the limit**, by scheduling it.
`createSchedule` also defaulted duration to 1800 with **no bound at all**, so a
stored schedule could carry any value.

**A second dimension:** even with validation at creation, a user could
subscribe, save a 3-hour schedule, downgrade, and keep getting 3 hours on every
trigger. The authoritative check therefore has to be at **build** time, not at
save time.

---

## Changes

### 1. The engine enforces its own bounds

`engine.Request` gains `MaxDurationSeconds`, plus package bounds
`MinSessionSeconds = 60` and `MaxSessionSeconds = 3 * 3600` (§20).
`validateDuration` runs inside `Build` and returns `ErrDurationTooShort`,
`ErrDurationTooLong`, or `ErrDurationExceedsPlan`.

The engine is now the last line of defence regardless of what any caller
forgets. A caller passing `MaxDurationSeconds: 0` still cannot build outside the
package bounds.

### 2. All four callers pass the plan ceiling

`createSession`, `createSessionPreview`, `saveTemplateFromRequest` and
`triggerSchedule` all set `MaxDurationSeconds: ent.MaxSessionSeconds()`.
`triggerSchedule` previously never computed entitlements at all; it now does and
answers **402** before touching the engine.

### 3. One plan-limit response

Three call sites had drifted inline 402 blocks — one said "the requested length
exceeds what your plan allows", two said "upgrade to Premium to build longer
sessions", and one leaked the numeric limit in the message. All seven sites now
share `api/plan_limit.go:writePlanLimit`, emitting:

```json
{"error":{"code":"session_duration_exceeds_plan_limit",
          "message":"Session length exceeds what your plan allows",
          "details":{"requested_duration":10800,"max_duration":900}}}
```

### 4. `createSchedule` validates at save time

Bounds plus the plan ceiling, with the same 402. Documented as a UX
convenience — the authoritative check stays in the engine, because the plan can
change between save and trigger.

### 5. G-38 closed

`store.ConfessionsByCategory(publishedOnly)` hardcoded `status = 'published'`
while the authority lived in `internal/content`. The behaviour was right by
accident. It now builds `status IN (...)` from the new
`content.ServedStatuses()`, so a future change to what is served cannot fail to
reach session building.

---

## Verified sound — not re-implemented

**The 11-state machine is correct.** `store.UpdateStatus` deliberately does not
re-validate transitions (documented in its own comment): its only two callers —
`patchSessionStatus` and `playSession` — both check `sessions.CanTransition`
first and answer **409 `INVALID_TRANSITION`** with the engine's `Reason`.

**Snapshots are intact.** `session_items` denormalises `title`,
`category_name` and `text`, and `confession_id` is `TEXT` with **no foreign
key** — so a confession can be deleted or archived and an existing session's
queue survives unchanged. `snapshot_test.go` already covers mutation, staleness
and integrity.

**All five strategies exist** with `BALANCED` as default, and the eight presets
plus `custom` are validated at the boundary.

---

## Testing

`internal/api/session_entitlement_test.go` — 4 tests, 7 assertions, all PASS.

| Test | Covers |
|---|---|
| `TestFreeUserCannotExceedPlanLengthThroughASchedule` | the exact bypass, via HTTP |
| `TestCreateScheduleRefusesAnOverPlanDuration` | save-time UX |
| `TestScheduleWithinThePlanStillBuilds` | the endpoint is not refusing everything |
| `TestEngineRefusesDurationsOutsideItsBounds` | engine bounds and the plan ceiling |

`internal/store/content_served_test.go` — `TestOnlyServedStatusesReachSessionBuilding`
creates one confession in each of the nine lifecycle states and asserts the query
returns exactly what `content.ServedStatuses()` says, cross-checked against
`IsServedToNewSessions`.

**Defect injection:** removing both the handler check and the engine cap made
the first test fail by name, returning 201 with a 180-item, 10 800 s session for
a free user. Restoring the fix made it pass.

Full suite: 25 packages, 0 failures. `make verify`: all checks passed, 0 issues.

---

## Exit criteria

| Criterion | Status |
|---|---|
| No path can build a session longer than the plan allows | PASS — engine-bound |
| Duration validated on every `Engine.Build` caller | PASS — 4 of 4 |
| One plan-limit response shape | PASS — `writePlanLimit`, 7 sites |
| G-38 closed | PASS |
| Snapshot and state-machine contracts intact | PASS — verified, unchanged |
| Tests prove the defect and fail without the fix | PASS |
| `make verify` clean | PASS |
