# PHASE 43 — Personalization: the seven listener signals

**Master-plan coverage:** item 34 (personalization). The master plan names
seven signals — categories listened to, completion rate, time of day, session
duration, favourites, skips, repeat listening — and requires deterministic rules
over them, no ML.

**Gap closed:** **G-55** (the recommendations endpoint read one thing —
explicit interests — and claimed `personalized: true` for it; none of the seven
behavioural signals was read, and repeat listening did not exist anywhere in the
server).

**Verdict:** PASS

**Provenance.** This phase was first built in a previous session as commit
`e32fb2c` with a handoff commit `3c878f0` and `docs/HANDOFF-AFTER-43.md`. Neither
commit reached the remote: `git cat-file -t e32fb2c` fails against a full,
unshallowed fetch of every branch, and the branch they were said to be on
(`arena/01a0c367-i-confess`, merged as PR #61) carries only a drag-handle fix.
The handoff document does not exist in any reachable ref. The phase was rebuilt
from the master-plan text and the current code; where this document differs
from what the lost commit did, this document is the record.

## OBJECTIVE

Verified against the code before building:

```
$ grep -rn "repeat" server/internal/api/home.go server/internal/store/*.go   → no matches
$ sed -n 175,300p server/internal/api/home.go
    // v1 is deterministic: rank by explicit interests ... Future versions
    // may weigh completion rates, time of day and skips.
    "personalized": len(interests) > 0,
```

`recommendations` in `home.go` ranked categories by the weight of the
listener's explicit interests and put the interest-matched confessions first.
That is a reasonable v1, but `personalized: true` was a claim the code could not
back: it was true for anyone who had tapped a category during onboarding and
false for a listener who had finished two hundred sessions without doing so.
The seven signals of item 34 were a comment.

The session engine already recorded everything needed. `session_items` holds a
per-item `COMPLETED`/`SKIPPED` status (PHASE 15), `sessions` holds
`started_at` and `actual_duration`, `favorites` is keyed by entity kind, and
`user_profiles.timezone` says whose morning is morning. Nothing new had to be
written on the listening path; the work was to read it honestly.

## IMPLEMENTATION

### A pure package, ranked by rules that are constants

`internal/personalization` has no database and no clock of its own. It takes
`Signals` (the evidence) and `Input` (the catalogue, explicit interests and the
current time) and returns a `Recommendation`. Every rule is a named constant at
the top of `rank.go`:

| Signal | Evidence | Rule |
|---|---|---|
| categories listened to | completed items per category | `+1.0` per completion, capped at 10 |
| completion rate | completed ÷ (completed + skipped) over ≥ 3 items | ≥ 0.9 steps the suggested duration up one rung; < 0.5 steps it down and halves the category boost |
| time of day | listener-local hour of `sessions.started_at`, bucketed night/morning/afternoon/evening | `+2.0` to categories the listener plays in the current daypart |
| session duration | median `actual_duration` of completed sessions | snapped to the 5/10/15/20/30/45/60-minute ladder |
| favourites | `favorites` rows by kind | `+3.0` category, `+2.0` confession, voice recorded for the client |
| skips | skipped items per confession / category | `−1.5` per skip, capped at −6 per category; a confession skipped ≥ 2 times sinks to the bottom |
| repeat listening | distinct completed sessions per confession, ≥ 2 | `+1.5` per repeat to the category, `+2.0` to the confession; feeds the `listen_again` rail with the count |

Repeat listening is the one the master plan lists and the code most obviously
lacked. It is a **count** — the number of separate sessions in which the same
confession was finished — because "listened twice" and "listened forty times"
are different facts about a listener whose product premise is repetition.
`TestSevenListenerSignalsAreTheProductContract` builds a fixed catalogue and
proves, signal by signal, that each one on its own changes the ranking; a
signal that cannot move the result is not a signal. `TestRankingIsDeterministic`
runs the same input a hundred times and requires the same output, and every
tie is broken by catalogue order and then by id so the result cannot depend on
map iteration.

Explicit interests remain a signal too (`+4.0` per unit weight — a listener
saying what they want still outranks what we inferred), but they are not a
*behavioural* signal and do not make `personalized` true on their own.

### The store reads; it does not keep a second ledger

`store.SignalStore.Signals(ctx, userID, tz)` returns the evidence in one round
trip per source: the newest 2000 session items joined to their live sessions
(`deleted_at IS NULL`) and their confession's category, the last 200 completed
session durations, and the favourites split by kind. Statuses go through
`sessions.NormalizeItemStatus`; only `COMPLETED` and `SKIPPED` count. `QUEUED`
and `PLAYING` are undecided and `FAILED` is the platform's fault, not a
preference. An unknown timezone name degrades to UTC rather than failing the
request: a wrong daypart is a weaker recommendation, a 500 is none.
`TestSignalStoreReadsDecidedItemsOfLiveSessions` seeds every kind of row —
canonical statuses, an open session, a soft-deleted one, another user's — and
checks exactly which reach the ranker.

### The endpoint says why

`GET /recommendations` (and `/v1/recommendations`) now lives in
`internal/api/recommendations.go`. The v1 shape is unchanged — `categories`,
`confessions`, `count`, `personalized` — so the shipped mobile client keeps
working. `personalized` is true only when at least one behavioural signal had
evidence. Added, all additive:

- `signals` — `present[]` naming the signals that had evidence, and the
  quantities: `categories_listened_to`, `completion_rate`, `completion_sample`,
  `preferred_daypart`, `typical_duration_seconds`, `favourites`, `skips`,
  `repeat_listening` (a count of confessions repeated, never a flag).
- `reasons.categories[id][]` and `reasons.confessions[id][]` — short tokens
  such as `listened 3`, `repeated 3`, `skipped 1`, `favourite`, `morning`,
  `interest`. A client can render *why*; a reviewer can read them in a log.
- `listen_again[]` — `{confession, times}` for repeated confessions, most
  repeated first.
- `suggested_duration_seconds` and `daypart`.
- `preferences` — which of the two listener switches were honoured.

The two switches are the listener's own and both are enforced here (PRD S14):
`personalization_enabled=false` stops the behavioural signals being *read* —
`SignalStore` is never called — and the catalogue is ranked on explicit
interests exactly as v1 did; `recommendations_enabled=false` answers with the
catalogue in its own order and no reasons at all.
`TestRecommendationsRankOnWhatTheListenerDid` drives the real playback
endpoints (three `completeASession`, one `/skip`, one favourite) and asserts
the ranking flips, the counts are exact (`completion_rate: 0.75`,
`repeat_listening: 1`, `times: 3`) and the switch removes every signal.

### Client

`clients/dart` `Recommendations` gains `listenAgain`, `categoryReasons`,
`confessionReasons`, `suggestedDurationSeconds`, `daypart` and a typed
`ListenerSignals`; a v1 payload without any of them still decodes
(`test/recommendations_test.dart`). No route, contract or IA change: the
endpoint already existed and was already wired to the home screen.

## FOUND WHILE BUILDING

- **The lost `TestSevenListenerSignalsAreTheProductContract` had the wrong
  premise.** As described in the handoff it asserted the *names* of seven
  signals in the response and collapsed repeat listening into a boolean
  (`api/recommendations.go:144` in the lost commit). A test that a list has
  seven strings in it proves nothing about ranking. The rewritten test lives in
  the pure package and proves each signal alters the result on its own.
- **`session_items.status` admits only canonical values.** Migration 0006
  folded the legacy `played`/`queued` spellings, so a store test cannot seed
  them; the read path still normalises defensively but the test seeds what the
  engine writes.

## VERIFICATION

Commands run from `server/` with `source /opt/tools/env.sh` (Go 1.25.5 via
`GOWORK`, PostgreSQL 17.10 local cluster, `TEST_DATABASE_URL` set):

```
gofmt -l .                       → no output
go build ./...                   → clean
go vet ./...                     → clean
/opt/tools/lint.sh               → no new findings (staticcheck, errcheck,
                                   ineffassign against a main baseline, with
                                   .golangci.yml's errcheck exclusions mirrored;
                                   golangci-lint v2.13 itself is not installable
                                   in this sandbox and is owed to CI)
go test -race ./... -count=1     → 35 ok packages, zero FAIL
                                   (internal/personalization is the 35th)

Named:
  go test -run . ./internal/personalization
     TestSevenListenerSignalsAreTheProductContract (7 subtests),
     TestRankingIsDeterministic, TestSuggestedDurationLadder,
     TestDaypartsFollowTheListenersTimezone, TestDaypartBoundaries
  go test -run TestSignalStore ./internal/store
     TestSignalStoreReadsDecidedItemsOfLiveSessions
  go test -run TestRecommendations ./internal/api
     TestRecommendationsRankOnWhatTheListenerDid,
     TestRecommendationsHonourTheRecommendationsSwitch

EXPORT_ROUTES=1 EXPORT_ROUTES_PATH=<abs>/design/routes.json \
  go test -count=1 -run TestExportRouteTable ./internal/api/   → ok, zero diff
go run ./cmd/genspec ../contracts/openapi.json                → zero diff
                                   (320 routes / 250 paths / 320 operations,
                                   unchanged: no route was added)
```

From the repository root:

```
python3 design/generate.py --check    → generated code up to date (120 tokens, 3 files)
python3 design/test_design.py         → 120 tokens, all Section 12 rules hold
python3 design/test_ia.py             → 38 screens, 8 entry points, 113 endpoints wired
python3 scripts/check_dart_symbols.py → PASSED 133/133
```

`dart test` / `flutter analyze` are owed to CI (PHASE 23 C-1).

## CONDITIONS CARRIED

- **The home screen does not yet render the reasons.** The mobile home shows
  the ranked lists as before; `listen_again`, `reasons` and
  `suggested_duration_seconds` are decoded by the client and unused by any
  screen. Raised as **G-56**.
- **Voice favourites are collected but not ranked.** Recommendations rank
  categories and confessions; a favourite voice is reported in `signals` so a
  client can preselect it, but nothing in this phase changes voice selection.
- **No schema change.** Every signal is derived from tables that other
  features already maintain. If a future phase wants per-item listen
  timestamps (today the daypart uses the session's `started_at`), that is a
  column on `session_items`, not a new table.
