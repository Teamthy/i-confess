# PHASE 41 — Trial engagement and the seven-day journey

**Master-plan coverage:** item 37 remainder (trial engagement tracking;
Day 1–7 journey copy). Master-plan 36 (`G-29` receipt verification) and the
`G-37`/`G-38`/`G-39` content-lifecycle work were verified as already shipped in
PHASE 36 and PHASE 37 respectively, so this phase starts at the part of 37 that
was not built.

**Gap closed:** none of the numbered register. This phase closes an unnumbered
gap recorded here as **G-46** (the trial could be displayed but not measured)
and **G-47** (the seven-day journey taught six categories and none of the
product's capabilities).

**Verdict:** PASS

## OBJECTIVE

PHASE 36 made the trial persistent and gave it an explicit six-state lifecycle.
It did not make it measurable, and it did not make the journey mean anything.

Two defects, both verified against the code before building:

1. **No day completion existed.** `analytics.EventTrialDayViewed` was declared
   and `POST /analytics/batch` accepted it, but the handler's only sink was
   `analytics.LogSink`, whose `Track` body is `return nil`. Every event the
   server acknowledged with `202 {"accepted": n}` was discarded. There was no
   table to hold one. The only day number the API could produce was
   `trial.DayFor`, a clock reading — which reports "day 4 of 7" for an account
   that never opened the app, so a conversion funnel had no denominator.

2. **The journey copy taught the wrong things.** `billing.TrialJourney` read
   `Morning Healing`, `Peace at Noon`, `Faith Foundations`, `Purpose & Focus`,
   `Gratitude Evening`, `Confidence & Wisdom`, `Weekly Summary`. That is six
   categories and a summary. It never showed a listener the personalization, the
   Premium voice, or the custom builder — the three things the paywall asks them
   to pay for. Only Day 7 matched the specified journey.

A third defect was found by the tests written for the first, and is recorded
under **Found while building** below.

## INPUTS

- `server/internal/billing/trial.go`: the journey table and `DayFor`.
- `server/internal/trial/lifecycle.go`: the six-state graph, `StateAt`, `DayFor`.
- `server/internal/store/trials.go`: `TrialStore`, the one projection writer.
- `server/internal/api/analytics_batch.go`: the allowlist and the no-op sink.
- `server/internal/api/sessions_playback.go`: `completeSession`, the one path
  that reaches `COMPLETED`.
- `docs/36-BILLING-TRIAL.md`, `docs/37-CONTENT-LIFECYCLE.md`.

`docs/35-TRIAL-LIFECYCLE.md` is still absent from this checkout. Its
§Conditions were not read and no claim is made that they were; the state
machine they describe is the one in `internal/trial`, which is what the tests
assert against.

## IMPLEMENTATION

### The journey is a contract, not copy

`TrialDay` gains an `Intent` — the machine-checkable statement of what the day
is for — plus the flags that make that statement true rather than merely
advertised:

| Day | Intent | What makes it true |
|---|---|---|
| 1 | `first_confession` | one 600s session, short enough to finish |
| 2 | `morning_and_night` | `SessionCount: 2`, 480s each |
| 3 | `personalization` | `Personalized: true` — categories resolved from the listener's interests |
| 4 | `longer_session` | 1800s, asserted to be the longest of the week |
| 5 | `premium_voice` | `PremiumVoice: true` |
| 6 | `custom_session` | `Custom: true` — the builder, not a pre-built session |
| 7 | `weekly_summary` | `Summary: true` |

`TestTrialJourneyMatchesSpec` asserts the seven intents verbatim, so a reworded
title cannot silently drop a day's purpose, and asserts each flag against the
day that claims it. Every category named is checked against the seeded
catalogue, because a day that names a slug the engine cannot resolve advertises
a session that does not exist.

Day 3 is the only day that varies per account. `Handler.personalizedJourney`
replaces its fallback categories with the listener's interest slugs when they
have any, and `TestTrialJourneyResolvesDayThreeFromInterests` asserts that no
other day is flagged personalized — otherwise the week stops being comparable
or testable.

The typed client also gained the fields it had been silently dropping:
`TrialDay.cta` was declared in `clients/dart` and never sent by the server, so
every journey row rendered without a call to action. It is now sent, and the
paywall renders it.

### Day completion is derived, never asserted

`trial_day_completions` (migration 0018) records one row per `(user_id, day)`,
each naming the session that completed it. The day number comes from
`trial.DayFor` over the trial row, not from the caller, so a listener cannot
advance their own journey by asking for a later day.

The only writer is `TrialStore.CompleteDay`, and its only caller is
`Handler.completeSession` — the path that already refuses a forged completion
because `COMPLETED` is reachable only from `ACTIVE`, `PAUSED` or `INTERRUPTED`.
So a day is completed by playback, structurally.

`trial_day_completed` is deliberately **not** on the analytics batch allowlist.
`TestTrialDayCompletionCannotBeAssertedByAClient` posts it and asserts a 400 and
no persisted row: an app must not be able to report a day it did not listen
through.

Refusals are errors, not silent no-ops, where they are the caller's fault, and
silent where they are not: an account with no running trial, an expired trial,
or a second completion on the same day all fall out of
`recordTrialDayCompletion` without failing a completion the listener earned.

### Analytics is persisted

`analytics_events` holds what `POST /analytics/batch` acknowledges, via
`AnalyticsStore.Record`. The store also satisfies `analytics.Sink`, so the
server-side funnel events and the client batch share one writer.

The funnel's three lifecycle events are emitted by `TrialStore` rather than by
handlers, because expiry is a clock fact that no caller reliably observes — the
row moves to `EXPIRED` whenever the next request happens to refresh it. If the
transition did not record it, the churn side of the funnel would be missing
entirely. `TestTrialLifecycleEventsAreRecordedOnce` asserts each event is
persisted exactly once, including that a repeated `Start` and a repeated
`Refresh` do not double-count.

Cancellation comes from the store webhooks: `NotificationOutcome` now carries
the state it applied, and `recordSubscriptionCancellation` records
`subscription_cancelled` for an applied `cancelled`/`expired`/`refunded`
notification and for nothing else. A duplicate or stale delivery is not a
second cancellation.

`GET /subscriptions/trial/engagement` (and `/v1`) returns days completed, the
completion rate, the funnel counts and the events behind them. It is on the
paywall screen in `design/ia.json` because the completed-day progress is the
evidence shown before asking for payment.

### Deletion policy and schema counts

Both new tables reference `users`, so both are in `deletion.Policies` as
`Erase`; the coverage test that asserts the policy union covers every
user-referencing table would otherwise fail. The schema moves to **68 tables /
81 foreign keys**, and both `conn_test.go` and the retention audit were updated
to the numbers the live database actually reports.

## FOUND WHILE BUILDING

`TestTrialDayCompletionFollowsTheTrialClock` failed on its first run with
`invalid trial transition: trial transition EXPIRING -> ACTIVE is not allowed`.

That is not a test bug. `TrialStore.Refresh` computed a time-derived state and
fed it straight to the explicit lifecycle. Once a trial is `EXPIRING`, any
refresh carrying an *earlier* instant asks for a backward edge, which the state
machine correctly refuses — and `Refresh` returned that as an error, turning a
plain status read into a 500. Clock readings arrive out of order in any
multi-instance deployment: an NTP adjustment, a replayed request carrying a
stored timestamp, an operator replaying a past instant.

The fix is that time only ever moves a trial forward: if the desired state is
not a legal edge from the current one, the row keeps the state it has already
earned. The guard sits *after* the `ACTIVE→EXPIRING` walk, because
`ACTIVE→EXPIRED` is deliberately reached through `EXPIRING` and an earlier
placement would have stopped trials expiring at all.
`TestTrialRefreshIgnoresABackwardClock` is the regression: it drives the trial
to `EXPIRING`, refreshes with an earlier clock, asserts no error and no
movement, and then asserts the forward edge to `EXPIRED` still works.

## VERIFICATION

Commands run from `server/` with
`source /tmp/toolchain/env.sh` and
`TEST_DATABASE_URL="host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable"`:

```
go test -modfile=/tmp/local.mod -count=1 ./...
  → 34 ok packages, zero FAIL

go test -modfile=/tmp/local.mod -count=1 -run "TestTrialEngagement|TestTrialDayCompletion|TestAnalytics|TestTrialJourney" -v ./internal/api/ ./internal/billing/ ./internal/db/
  → PASS TestTrialEngagementMeasuresRealCompletions
  → PASS TestTrialDayCompletionCannotBeAssertedByAClient
  → PASS TestAnalyticsBatchPersistsWhatItAcknowledges
  → PASS TestTrialJourneyResolvesDayThreeFromInterests
  → PASS TestTrialJourneyMatchesSpec
  → PASS TestTrialDayCompletionsAreUniquePerUserAndDay

go test -modfile=/tmp/local.mod -count=1 -run "TestTrial|TestAnalyticsStore" ./internal/store/
  → ok (includes TestTrialRefreshIgnoresABackwardClock)

gofmt -l internal cmd      → no output
go vet ./...               → clean

EXPORT_ROUTES=1 EXPORT_ROUTES_PATH=<abs>/design/routes.json go test -count=1 -run TestExportRouteTable ./internal/api/
go run -modfile=/tmp/local.mod ./cmd/genspec ../contracts/openapi.json
  → 308 routes / 242 paths / 308 operations (was 306/240/306; +2 for the
    engagement route under both prefixes). Re-running both against the
    committed files leaves zero git diff.
```

From the repository root:

```
python3 design/test_ia.py         → 38 screens, 8 entry points, 108 endpoints wired
python3 design/generate.py --check → generated code up to date (120 tokens, 3 files)
python3 design/test_design.py      → 120 tokens, all Section 12 rules hold
python3 scripts/check_dart_symbols.py → 105 assertions, PASSED 105/105
```

`design/ia.json` was edited as text, not through `json.load`/`dump`: the diff is
two lines.

## CONDITIONS CARRIED

- **No Flutter SDK in this sandbox.** `flutter analyze` and `flutter test` could
  not be run; the mobile change is gated by `scripts/check_dart_symbols.py`
  (three new assertions) and owed to CI.
- **`TrialDay.PremiumVoice` and `Custom` are flags, not behaviour.** The server
  now declares that day 5 wants a Premium voice and day 6 wants the builder, but
  no server code yet selects a Premium voice for day 5 or opens the builder for
  day 6 — the flags are contract for the client. Building the session for a
  journey day is not implemented at all: the engine is reached through the
  normal builder. Recorded as **G-48**.
- **Day 2 asks for two sessions but nothing schedules the second.** `SessionCount`
  is advertised; creating the evening session is the client's job and is not
  done. Also **G-48**.
- **Expiry analytics depend on a read.** `trial_expired` is recorded when
  something next refreshes the trial. There is no sweeper, so an account that
  never returns is never counted as churned. This is a real limitation of the
  funnel and is recorded as **G-49** rather than described away.
