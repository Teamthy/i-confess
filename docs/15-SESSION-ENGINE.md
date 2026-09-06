# PHASE 13 — SESSION ENGINE

## OBJECTIVE

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
