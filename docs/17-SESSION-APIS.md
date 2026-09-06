# PHASE 17 — SESSION APIs

## OBJECTIVE

Expose the session domain through secure APIs — creation, retrieval, update,
delete, start, pause, resume, complete, skip, queue, progress — with tests.

## INPUTS

`docs/00`–`16`, the directive's PHASE 17 definition, and the session machinery
built and audited in PHASE 15.

## DEPENDENCIES

PHASE 15. The state machine, snapshots, planning strategies and selection fixes
this phase exposes were audited there; this phase is the surface over them.

---

## Audit of the existing surface

Eleven of the twelve operations were already routed. Checking each against the
directive rather than assuming:

| Operation | Route | Status coming in |
|---|---|---|
| creation | `POST /sessions` | present, tested |
| retrieval (one) | `GET /sessions/{id}` | present, ownership checked |
| retrieval (history) | `GET /sessions` | present, **capped at a hardcoded 50 with no way past it** |
| update | `PATCH /sessions/{id}` | present, transition-validated |
| **delete** | — | **did not exist** |
| start | `POST /sessions/{id}/start` | present, **zero tests** |
| pause | `POST /sessions/{id}/pause` | present, **zero tests** |
| resume | `POST /sessions/{id}/resume` | present, **zero tests** |
| complete | `POST /sessions/{id}/complete` | present, tested |
| skip | `POST /sessions/{id}/skip` | present, tested |
| queue | `GET /sessions/{id}/queue` | present, tested |
| progress | `POST /sessions/{id}/progress` | present, tested in PHASE 15 |

Two real gaps, not cosmetic ones.

**Delete did not exist.** `grep` for `DELETE /sessions` and `deleteSession`
returned nothing. A listener had no way to remove a session from their history.

**The dedicated playback endpoints were never exercised.** Every existing test
drove pause and resume through `PATCH /sessions/{id}`, the generic status
endpoint. `POST /pause` and `POST /resume` — the routes a player actually calls —
were registered and untested.

---

## IMPLEMENTATION

### 1. `DELETE /sessions/{id}`

**Soft delete**, per §25. `0008_session_soft_delete.sql` adds `sessions.deleted_at`
and `idx_sessions_history (user_id, deleted_at, created_at)`.

Soft rather than hard because the row is the record of what someone listened to.
Streaks and completion metrics are derived from these rows, so a deletion a
listener can perform must not silently edit the numbers. The queue snapshot stays
with it, so a history entry remains reconstructable.

The column is nullable rather than `NOT NULL DEFAULT ''`: an empty-string default
would make "never deleted" indistinguishable from "deleted at the epoch".

**A live session is cancelled first, through the state machine** rather than by
writing a status. Deleting an `ACTIVE` session without cancelling it leaves a
player somewhere holding a session it believes is playing and nothing that can
tell it otherwise. `CANCELLED` is reachable from eight states and is terminal, so
this always succeeds for a session that can be deleted.

Ownership is checked before anything is written, via the same `ownSession` the
playback endpoints use. A second `DELETE` returns 404 rather than confirming a
deletion that did not happen.

`SessionStore.ByID` and `ListByUser` now exclude deleted rows, so a deleted
session is 404 everywhere — read, resume, skip, queue — and not merely hidden
from the list. Only those two read paths exist, which is what made the change
safe to make in one place.

### 2. Keyset pagination on `GET /sessions`

`limit` (1–100, default 20) and an opaque `cursor`. The response is now an
envelope — `sessions`, `next_cursor`, `limit` — rather than a bare array.
Nothing consumed the old shape: the Dart client has no session code and no test
asserted on it.

Paging is by keyset on `(created_at, id)` rather than `OFFSET`. An offset scan
gets slower the deeper the listener pages, and a session created while they were
reading shifts every later row, so page 2 can repeat one already seen. The `id`
tiebreaker is what makes the order total: `created_at` carries second precision
and ties constantly — the pagination test creates seven sessions inside one
second and pages through them three at a time.

The cursor is base64 of the position, not a page number, so the encoding can
change without breaking anyone who stored one. A cursor is offered only when the
page was full; a short page means the end of the history.

### 3. Tests for the whole surface

`TestSessionAPISurfaceIsComplete` encodes the directive as an assertion over the
**live route table**, so a route registered under a typo'd pattern does not count
as present.

---

## TESTING

Sixteen new tests.

| Test | Covers |
|---|---|
| `TestSessionAPISurfaceIsComplete` | all twelve operations are routed |
| `TestOwnerCanDeleteASession` | 204, then 404 on read, absent from history |
| `TestDeleteIsSoft` | the row and its queue survive |
| `TestDeleteCancelsALiveSession` | an active session ends `CANCELLED` |
| `TestDeletedSessionCannotBeResumed` | 404 on start/pause/resume/complete/skip/queue |
| `TestCannotDeleteAnotherUsersSession` | 403, and the victim's session survives |
| `TestDeleteIsIdempotent` | a double tap gets 404, not a second 204 |
| `TestDeleteUnknownSessionIsNotFound` / `TestDeleteRequiresAuthentication` | 404 / 401 |
| `TestDedicatedPlaybackEndpoints` | start → pause → resume → pause → complete over the real routes |
| `TestPlaybackEndpointRejectsIllegalTransitions` | 409 from the state machine, not around it |
| `TestPlaybackEndpointsAreAuthorised` | 403 on every playback route for another user's session |
| `TestHistoryPaginatesWithoutRepeats` | 7 sessions, 3 per page, each exactly once |
| `TestHistoryRejectsBadLimitAndCursor` | 400 on `limit=0`, `101`, `abc`, a bad cursor |
| `TestHistoryIsEmptyForANewUser` / `TestHistoryIsScopedToTheCaller` | empty envelope, no cross-user leakage |

**Defect injection — six for six:**

| Injection | Result |
|---|---|
| `deleted_at` filter removed from `ByID` | `TestOwnerCanDeleteASession` and `TestDeletedSessionCannotBeResumed` FAIL |
| `SoftDelete` made a hard `DELETE` | `TestDeleteIsSoft` FAIL |
| cancel-before-delete removed | `TestDeleteCancelsALiveSession` FAIL |
| keyset cursor filter removed | `TestHistoryPaginatesWithoutRepeats` FAIL — 3 of 7 seen, one repeated 10× |
| `DELETE /sessions/{id}` route removed | `TestSessionAPISurfaceIsComplete` FAIL |
| ownership check removed from `ownSession` | `TestPlaybackEndpointsAreAuthorised` FAIL — 200 on another user's session |

One injection initially broke the build rather than failing a test, which proves
nothing; it was rewritten as a whole-function replacement before it counted.

Full suite: **28 packages, 0 failures.** `make verify`: **all checks passed.**

---

## SECURITY REVIEW

- **Authorisation on every route.** `deleteSession` and all six playback handlers
  go through `ownSession`, which verifies ownership before any write. Proven by
  injection: removing the check yields 200 on another user's session.
- **Unauthenticated access is 401**, verified for `DELETE`.
- **No new privileged surface.** Both new routes are `user`-scoped; the delete is
  on the caller's own resource.
- **`/v1/` twins added** for the delete, so the route-parity guard passes
  (278 → 280 registrations).
- **The cursor is opaque and validated.** A malformed or empty-position cursor is
  a 400, not a 500 or a full table scan. `limit` is bounded to 100.
- **Deletion is not destructive**, so a mistaken or malicious delete cannot erase
  listening history or the metrics derived from it.

## DOCUMENTATION

This file. `docs/PROJECT-STATUS.md` updated with the PHASE 17 verdict.

---

## EXIT CRITERIA

| Criterion | Status |
|---|---|
| Creation, retrieval, update, delete | PASS — delete added |
| Start, pause, resume, complete, skip | PASS — dedicated routes now tested |
| Queue and progress | PASS — verified from PHASE 15 |
| Every route has a `/v1/` twin | PASS — parity guard |
| Authorisation on every route | PASS — proven by injection |
| Retrieval pages past the first 50 | PASS — keyset |
| Tests fail without the fix | PASS — 6 of 6 injections |
| `make verify` clean | PASS |

## VERDICT

**PASS WITH CONDITIONS — 9/10**

- **C-1** `GET /sessions` changed shape from a bare array to an envelope. Nothing
  in the repo consumed it, but any external client written against the old shape
  will break. Worth a note in the API changelog before the mobile client is built.
- **C-2** `PATCH /sessions/{id}` updates status only. Title and description exist
  on the model and in the schema but cannot be changed after creation.
- **C-3** Soft-deleted sessions are unreachable through the API with no restore
  path. A "recently deleted" view would make the soft delete useful to a
  listener rather than only to the database.
- **C-4** Carried from PHASE 16: the admin generate endpoint is still
  synchronous, and `notification.send` still has no producer.
