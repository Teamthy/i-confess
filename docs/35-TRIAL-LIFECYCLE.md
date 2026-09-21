# PHASE 35 — Trial lifecycle (G-3)

**Status:** PASS
**Date:** 2026-09-21
**Depends on:** PHASE 04 (payments; the bypass this closes a sibling of), PHASE 30 (the `/subscriptions` surface), PHASE 12 (the vocabulary-parity discipline reused from G-5)
**Closes:** G-3

## OBJECTIVE

§36 specifies six trial states — `eligible | started | active | expiring |
expired | converted`. None of them existed. What existed instead was a
`trial` string in the subscription vocabulary and a
`GET /subscriptions/trial` that derived "which day is it" from
`users.created_at`. The consequences were all user-visible and none of them
were hypothetically bad — they were the product:

- everyone was on trial forever, because nothing ended it;
- a paid customer kept seeing a trial countdown, because the day arithmetic
  read account age, not any record;
- an expiring trial never announced itself, because there was no expiring;
- "has this account consumed its trial?" — the single question an
  introductory offer exists to gate — was unanswerable without log-diving.

The phase had to make the trial a **record with a clock and a state
machine**, not another computed view. A trial that is only a formula over
`created_at` cannot be refused, revoked, converted, or audited, and a formula
has no transitions to validate.

## IMPLEMENTATION

### The machine (`internal/billing/trial_lifecycle.go`)

`TrialStatus` is a closed set of six values with an explicit edge table:

| from | allowed next |
|---|---|
| `eligible` | `started` |
| `started` | `active`, `expired`, `converted` |
| `active` | `expiring`, `expired`, `converted` |
| `expiring` | `expired`, `converted` |
| `expired` | `converted` |
| `converted` | — terminal |

Every edge moves forward through the declared order; there is no `un-expire`,
no re-claim, no backwards ops door. `TrialDuration` is 7 days,
`TrialExpiringWindow` 24 hours (inclusive at the boundary — a trial that is
exactly one day from ending is already expiring). A refused move returns
`*TrialTransitionError` carrying the from-state and the sorted set of legal
targets, so callers answer "not yet" with the actual graph instead of a bare
409. `expired → converted` exists because the paywall stays open after the
clock runs out: conversion is a commercial event, not a temporal one, and a
late purchase still converts.

### The record (`internal/db/migrations/0014_trial_lifecycle.sql`, `internal/store/trials.go`)

The `trials` table is the one trial row an account will ever have:
`user_id UNIQUE`, status CHECK-constrained to exactly the six values,
`started_at / ends_at / converted_at / created_at / updated_at` stored as
UTC RFC3339 like every other timestamp here, and `CHECK (ends_at >
started_at)` — a clock that runs backwards makes the expiring window
unreachable, so it is refused at the database, not by convention. Schema
counts move to **66 tables, 77 foreign keys**.

The store is deliberately boring and deliberately stateful:

- `Current` lazily creates the `eligible` row and **repairs the status on
  read** from the clock (`advance`/`clockStep`): a row still marked `active`
  past `ends_at` is expired by the read, then persisted. The database and a
  cron can disagree; the user should never see the disagreement.
- `Start` transitions `eligible → started`, stamping `started_at` and
  `ends_at = started + 7d`. A second claim is impossible by graph, not by
  check-then-act: the row update is conditioned on the from-state.
- `MarkExpiring`, `Expire`, `Convert` all funnel through one `step`, which
  validates the edge before touching SQL.
- `Sweep(now)` is the batch clock: three parameterized UPDATEs (expire past
  ends, promote into the window, then promote `started → active`) returning
  the number of rows moved. Run first-pass and it converges on a second —
  a test pins the fixpoint.
- `SetStatus` exists for workers/ops and **still cannot jump the graph** —
  the test `TestSetStatusCannotJumpTheGraph` exists precisely because an
  "admin" entry point is the classic place a state machine grows a back
  door.
- `liveTrialDay` is the small helper with a big consequence: `day` is 1..7
  **only while the clock is live** (started/active/expiring). `eligible`,
  `expired` and `converted` report 0. Day is derived from the trial record's
  `started_at/ends_at`, never from account age again.
- Time parsing reuses the package's existing `parseTS` (from
  `internal/store/jobqueue.go`) rather than growing a second one; its
  zero-on-garbage semantics are exactly what the clock repair wants.

### The surface (`internal/api`)

Three routes per namespace (bare + `/v1` — six total, 306 in
`design/routes.json`, 102 paths in `openapi.json`):

- `GET /subscriptions/trial/lifecycle` — the record: status, day, timestamps,
  and `allowed[]` (what the graph will accept from here, so the client can
  render affordances it cannot invent). Lazily creates the `eligible` row.
- `POST /subscriptions/trial/start` — the claim. Refusals: 409
  `TRIAL_ALREADY_USED` (with `allowed[]`) for any account whose record has
  moved past `eligible`; 409 `TRIAL_UNAVAILABLE` for an account on an active
  **premium** subscription — and note the plan check, because every account
  has an active FREE subscription row and a status-only guard refused new
  users who had never touched the paywall (the test caught this before
  commit). Success returns the record with `day: 1`.
- `PATCH /subscriptions/trial` — worker/ops step: 400 `TRIAL_INVALID` for a
  status outside the vocabulary, 409 + `allowed[]` for a refused edge, 200
  for a legal move (including the same-state no-op, which keeps retries
  idempotent).

`GET /subscriptions/trial` keeps its journey shape (it feeds the existing
seven-day UI) but its `current_day` is now **record-derived**: the clock
answers, `created_at` is used only for accounts with no trial row at all.
The `POST /subscriptions/verify` hook converts the trial on a verified
receipt — best-effort and silent by design: a `TrialTransitionError` (nothing
to convert) or `ErrNoTrial` (a direct purchaser who never had the row
created) is logged at debug at most and **never** fails a paid customer's
receipt. A receipt is money; the trial note is bookkeeping.

`internal/deletion/policy.go` gained the `trials` entry, so account erasure
erases the trial — `TestEveryUserTableHasAPolicy` now also covers it.

### The client (`clients/dart`, `apps/mobile`)

- `endpoints.dart`: `getSubscriptionsTrialLifecycle`,
  `postSubscriptionsTrialStart`, `patchSubscriptionsTrial`.
- `models.dart`: `final class Trial` with status getters and a `label`
  switch that **degrades an unknown status to its raw string** — a newer
  server's state should be visible, not a parse exception in the paywall.
- `repository.dart`: `trialLifecycle()` (deliberately **never cached** — a
  five-minute-stale "expiring" misleads the only deadline the paywall shows)
  and `startTrial()` returning a `WriteResult<Trial>`.
- `premium_providers.dart`: `trialLifecycleProvider`, separate from the
  cacheable `trialProvider` (the journey list is the same seven days for
  everyone; the lifecycle answer is per-account and time-sensitive).
- `premium_screen.dart`: `_TrialCard` — eligible accounts get a real
  "Start free trial" button that reports the server's answer (including
  refusals); running accounts get the day and the server's `ends_at`;
  finished states get the truth and no button, because the client cannot
  offer what the server will not grant. No local countdown math — the clock
  is the server's.

`design/ia.json` wires the three new endpoints into the `paywall` screen:
**107 endpoints wired**, every user-facing live route used.

## VERIFICATION

- `internal/billing/trial_lifecycle_test.go` — the full edge table (every
  allowed edge, every refusal, 21 assertions' worth of pairs), the graph is
  closed and forward-only in declared order, `TrialAllowedFrom` is sorted
  (responses are compared verbatim), day math at boundaries including
  day-0-past-the-wire.
- `internal/store/trials_test.go` (PostgreSQL) — a full walk where reads
  advance the clock; the claim is once-per-account; `Convert` is never
  reachable from `eligible`; sweeps move whole rows and re-run to a
  fixpoint; sweep expires started clocks whose window already elapsed;
  `SetStatus` cannot jump the graph; one row per user at the DB; and
  `TestTrialVocabularyParityAgainstConstraint`, which parses the live
  `CHECK` out of `pg_get_constraintdef` and fails if it drifts from
  `billing.TrialStatuses()` — the G-5/G-2 discipline applied to a new table
  on the day it is born.
- `internal/api/trial_lifecycle_test.go` — 401 before any of it; the
  lifecycle GET lazily creates and reports `allowed[]`; claim stamps
  `ends_at = now+7d` and returns day 1; the second claim is 409
  `TRIAL_ALREADY_USED`; PATCH refuses backwards (409), gibberish (400),
  accepts forward (200); the journey is day 0 pre-claim and day 1
  post-claim; a verified receipt converts the trial and stamps
  `converted_at`, conversion is idempotent to re-verify; a direct purchaser
  with no trial row verifies cleanly with the hook silent.
- Two real bugs were caught by these tests before commit: the premium guard
  checking subscription **status** where it must check **plan**, and
  `current_day` continuing to tick on a converted trial whose window still
  contained "now" (fixed by `liveTrialDay`). Both are now pinned by named
  assertions, not prose.
- `go test ./...` green across all packages (24), `go vet` clean, `gofmt`
  clean, deletion + conn tests green at 66/77, `check_dart_symbols.py`
  73→**85** assertions, `design/test_ia.py` 107 wired, routes.json +
  openapi.json regenerate byte-identical.

## Conditions — what this phase deliberately does NOT do

1. **A running trial does not flip entitlements.** `entitlementsFor` still
   reads the subscription plan only. A trial that grants nothing would be a
   lie in the other direction, but granting it means deciding plan truth in
   a second place, and PHASE 36 (real store verification) is exactly the
   phase that rewrites how plan state is decided — the grant is one tested
   line there, not a pre-emptive fork of it here. A code comment at
   `startTrial` records the debt.
2. **Conversion has no public route.** `POST /subscriptions/trial/convert`
   does not exist — money converts a trial, verified server-side, or nothing
   does. The only writer of `converted` besides tests is the verify hook.
3. **`subscriptions` is untouched.** The trial's six states live in
   `trials.status`; the subscription vocabulary keeps its `trial` string for
   store-reported state. The two clocks merge in PHASE 36 with the
   verifier, not by making `trials` a second source of payment truth.
4. No un-expiring, no grace re-opens, no client-side trial math. The
   expiring boundary is inclusive — exactly-24h-out is `expiring`, and a
   fixture sitting on the boundary is a flaky test, as the sweep test learned
   by parking 48h out instead.
