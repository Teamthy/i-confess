# PHASE 37 — Content lifecycle enforcement

**Gap closed:** `G-37 (PHASE 37: explicit content transition graph)`,
`G-38 (PHASE 37: deprecated content is excluded from new sessions)`, and
`G-39 (PHASE 37: all constrained status vocabularies are audited)`

**Verdict:** PASS

## OBJECTIVE

Complete the editorial lifecycle that PHASE 12 deliberately left as a
vocabulary-only contract. A confession status is now a directed workflow
movement, not an ordinal that a caller may jump through. The status writer used
by the audited admin route enforces the same graph the domain package exposes.
The session selector continues to use the served-status authority, so
`deprecated` is playable only when already present in a queue and is not offered
to a newly built session.

## INPUTS

- `server/internal/content/lifecycle.go`: nine content statuses and the
  served-status rule.
- `server/internal/store/moderation.go`: the audited confession status writer
  and moderation history transaction.
- `server/internal/api/admin.go`: the admin PATCH boundary.
- `server/internal/db/migrations/0003_content_lifecycle.sql`: the installed
  `confessions.status` CHECK.
- `server/internal/db/status_constraints_test.go`: the existing live-schema
  status constraint audit.
- `docs/12-CONTENT-GOVERNANCE.md`: the three explicit conditions carried from
  PHASE 12.

The checkout does not contain `docs/35-TRIAL-LIFECYCLE.md`; it was not treated
as an input or represented as read.

## IMPLEMENTATION

### Explicit edge table

`content.forwardEdges` is the complete directed graph. It is intentionally not
derived from the order returned by `All()`:

```text
draft → content_review → theological_review → audio_production → audio_qa
     → approved → published → deprecated → archived
                                  └────────→ archived  (withdrawal)
```

`archived` is terminal. A self-transition is not an edge; the audited handler
recognises an unchanged PATCH before it writes history, preserving idempotent
admin retries without making the state machine permissive. The exported
`Edges()` method returns a defensive copy, while `CanTransition` and
`Transition` provide the write-path contract and a typed sentinel for the API.

The two published exits are both deliberate. Deprecation preserves playback
for queues already materialised; direct archival is the emergency withdrawal
edge. Neither path can reopen content or skip a review state.

### Audited status updates

`ModerationStore.UpdateConfessionStatusAudited` locks and reads the current
status, validates a real move through `content.Transition`, then updates the
row and writes `content_moderation_history` in one transaction. No-op PATCHes
remain history-free. An illegal but known status movement returns the domain
sentinel and the admin route maps it to HTTP 409; an unknown vocabulary value
still returns HTTP 400 at request validation. This means callers cannot bypass
the graph by calling the store directly, and the database CHECK remains the
last backstop rather than the workflow engine.

### Served-status authority

`IsServedToNewSessions` and `ServedStatuses` remain the only content-selection
authority. The session-building query consumes that authority, and the existing
regression test inserts one confession in every lifecycle state, proving that
only `published` reaches a new queue while the unfiltered admin read still sees
all states. This closes the previously theoretical `deprecated` distinction
without mutating existing session snapshots.

### Status vocabulary audit

`TestContentVocabularyParityAgainstConstraint` reads PostgreSQL's installed
`confessions.status` CHECK with `pg_get_constraintdef` and compares it to
`content.All()` in both directions. It does not parse migration source.

`TestDatabaseVocabularyMatchesGoConstants` now covers all 23 tables with a
literal `status` column: collections, categories, confessions, voices,
content versions, audio and voice lifecycles, user and subscription states,
sessions and items, UGC, jobs, delivery, moderation, reports, and community
posts. `TestTrialVocabularyParityAgainstConstraint` applies the same live
constraint pattern to the trial `state` column introduced in PHASE 36.

No table or foreign key was added in PHASE 37. The live schema remains **66
tables / 77 foreign keys**.

## TESTING

Named tests:

- `TestContentTransitions` enumerates every pair of the content vocabulary and
  proves all reverse, skipped, self, and terminal moves are rejected.
- `TestAdminCanMoveAConfessionThroughTheWholeLifecycle` walks every permitted
  path over HTTP, including `published→deprecated→archived`.
- `TestAdminRejectsABackwardContentTransition` proves a valid status is still
  rejected when the directed movement is invalid.
- `TestOnlyServedStatusesReachSessionBuilding` proves deprecated content is not
  selected for a new session.
- `TestContentVocabularyParityAgainstConstraint` compares content code to the
  live PostgreSQL CHECK.
- `TestDatabaseVocabularyMatchesGoConstants` audits all constrained status
  columns; `TestTrialVocabularyParityAgainstConstraint` covers trial state.

The phase proving commands are:

```sh
source /tmp/toolchain/env.sh
export TEST_DATABASE_URL='host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable'
cd server
GOFLAGS=-modfile=/tmp/local.mod go test ./internal/content ./internal/db ./internal/store ./internal/api -count=1
gofmt -l internal cmd
go vet ./...
```

The targeted suite passed against PostgreSQL. Formatting produced no file names,
and `go vet ./...` passed.

The repository-wide contract checks remain:

```sh
EXPORT_ROUTES=1 EXPORT_ROUTES_PATH=/home/user/i-confess/design/routes.json \
  GOFLAGS=-modfile=/tmp/local.mod go test ./internal/api -run '^TestExportRouteTable$' -count=1
GOFLAGS=-modfile=/tmp/local.mod go run ./cmd/genspec /home/user/i-confess/contracts/openapi.json
cd ..
python3 design/test_ia.py
python3 scripts/check_dart_symbols.py
```

These remain **306 route entries / 240 OpenAPI paths / 306 operations** and
**85/85 Dart symbols**. PHASE 37 adds no client route, so `design/ia.json`,
`design/routes.json`, and `contracts/openapi.json` remain byte-identical.

## SECURITY AND DATA REVIEW

- A known status cannot be used to jump from draft to published or to reopen
  terminal content.
- The route returns a conflict rather than a generic database error for an
  illegal movement, without creating an audit row for the refused operation.
- Direct emergency archival is explicit and auditable; it does not erase the
  confession or rewrite existing session queues.
- The parity tests query the live database, so a later migration that changes a
  CHECK without changing Go fails in the test database.
- The status audit covers the 23 `status` columns while treating the six-state
  trial `state` column as its own vocabulary. The schema-count claim is not
  conflated with a status-column count.

## EXIT CRITERIA

- [x] The content lifecycle has an explicit forward-only edge table; proving
      command: `go test ./internal/content -run '^TestContentTransitions$'`.
- [x] The audited status writer enforces the graph; proving command:
      `go test ./internal/store ./internal/api -run 'Test(UpdateConfessionStatusAudited|AdminRejectsABackwardContentTransition)'`.
- [x] Named session-serving regression covers deprecated content; proving
      command: `go test ./internal/store -run '^TestOnlyServedStatusesReachSessionBuilding$'`.
- [x] Live CHECK parity covers content and every constrained status vocabulary;
      proving command: `go test ./internal/content ./internal/db -run 'Test(ContentVocabularyParityAgainstConstraint|DatabaseVocabularyMatchesGoConstants|TrialVocabularyParityAgainstConstraint)'`.
- [x] Schema counts are reconciled at 66 tables / 77 foreign keys; proving
      command: `go test ./internal/db -run '^TestPostgresSchemaLoads$'`.

## VERDICT

**PASS — `G-37`, `G-38`, and `G-39` are closed by PHASE 37.**
