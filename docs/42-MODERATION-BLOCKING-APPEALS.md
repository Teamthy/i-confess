# PHASE 42 — Moderation completeness: blocking and appeals

**Master-plan coverage:** item 32 (moderation) remainder. PHASE 31 shipped
reporting, the moderation queue, the editorial lifecycle and the §75 QA gate.
Two halves of a moderation system were absent entirely, and PHASE 31 should not
have been described as done without them.

**Gap closed:** **G-50** (a listener being harassed has one tool — file a report
and wait for a human — and no self-service boundary) and **G-51** (every
moderation decision is terminal; the person it was made about cannot answer).

**Verdict:** PASS

## OBJECTIVE

Verified against the code before building, not against the phase document:

```
$ grep -rn "appeal" server/internal/ --include=*.go     → no matches
$ grep -rn "block" server/internal/api/router.go        → no matches
```

`moderation.ReportableEntityTypes` is `{"confession", "community_post"}` — you
could not even report a *user*. And `DecideReport` and `ReviewUserConfession`
both write terminal states with no path back.

Two consequences, each of which is a real defect rather than a missing nicety:

1. **A block is the wrong thing to wait for a moderator about.** A listener who
   is being harassed has exactly one tool: file a report and wait for a human to
   agree that the behaviour was wrong. A block is a self-service boundary that
   takes effect immediately, requires nobody's agreement, and is reversible by
   the person who set it.

2. **A moderation system with no appeal path cannot correct itself.** A dismissed
   report and a rejected confession both end the conversation. The first
   rejection that was a false positive proves the system has no way to find out.

## IMPLEMENTATION

### Blocking is a boundary, not a punishment

`user_blocks` (migration 0019) records `(blocker_id, blocked_id)` with a
`blocker_id <> blocked_id` CHECK and a unique pair. `internal/moderation/blocking.go`
states the rule the whole feature rests on: a block is not a moderation action.
It deletes nothing, penalises nobody, and is not shown to the blocked account. It
changes what one account is served.

That framing is what the effects are built from, and it is what stops the
feature becoming a harassment tool in the other direction — which is why
`BlockStore` is deliberately not part of `ModerationStore`.

Two effects, both enforced server-side and both tested:

| Effect | Where | Test |
|---|---|---|
| A blocked author's published testimony disappears from the blocker's public reader, and stays in everyone else's | `ListPublishedUserConfessionsExcluding`, called by `feedUserConfessions` | `TestBlockingFiltersThePublicReader` |
| A reaction is refused in **both** directions when a block exists between the reactor and the post's author | `BlocksBetween`, called by `reactCommunity` | `TestBlockingIsABoundaryAndNotARecord` |

Both directions matter and they are not the same case. If I blocked you, your
reaction reaching me is what the block was for. If *you* blocked *me*, my
reaction is unwanted contact with someone who asked not to hear from me — and
honouring only one direction makes the block half a boundary.

The feed filter runs in SQL rather than in Go on purpose. Filtering after the
read means a limit of 20 can return 14 rows because six were dropped in memory,
and the client renders a short feed with no way to tell whether the filter
worked or the platform ran out of content.

### Appeals are heard once

`moderation_appeals` carries an explicit four-state lifecycle in
`internal/moderation/appeals.go`, in the same shape as `internal/trial` and
`internal/content`:

```
submitted → under_review → upheld | overturned
          ↘ upheld | overturned            (both terminal)
```

There is no re-filing, and the unique constraint is a full one rather than a
partial index over the live statuses: an appeal a moderator heard and upheld is
final, and allowing a second filing would turn the appeal queue into the same
spam surface `reports_one_open_per_reporter` closes for reports.

Three refusals run before a row is written, because an appeal that passes
without them is worse than no appeal:

- the decision must belong to the caller — which is also what stops an account
  reading a moderator's reasoning about a stranger (`ErrAppealNotOwned`, 403);
- the decision must have been made: a report still `open` has not been
  dismissed, a confession still `submitted` has not been rejected
  (`ErrAppealableDecisionNotFound`, 409);
- the statement is bounded, because a moderator reads it (400).

**Overturning is not a grant.** A dismissed report goes back to `open` and its
entity gets a queue entry again; a rejected confession goes back to `submitted`,
which re-enters the review queue. Neither is published and neither is resolved,
because an appeal succeeding means the first decision was wrong — it does not
mean the opposite decision is right. That call still belongs to the moderator,
with the content in front of them. `TestAppealOfARejectedConfessionDoesNotPublish`
is the test that keeps an appeal from becoming a publication back door.

Upholding changes nothing at all, and `TestAppealUpheldChangesNothing` proves
it: a system that quietly softens the original decision on appeal has no
decisions.

### Queue, audit and erasure

Pending appeals are in `ModerationQueue.Appeals` and in `counts["appeals"]`,
oldest first, so an appeal is queue work in the same sense a report is — a
person waiting on a human. Filing and deciding both write `audit_logs` through
the one `recordAudit` sink PHASE 33 established.

Both new tables reference `users`, so both are in `deletion.Policies` as
`Erase`.

## FOUND WHILE BUILDING

**The deletion package reported an innocent table for a different table's bug.**
`internal/deletion` failed with
`apply policy for moderation_appeals: current transaction is aborted`. The
message named `moderation_appeals`; the fault was `user_blocks`.

`applyPolicy`'s generic child branch scopes a child table through its parent:

```sql
DELETE FROM <table> WHERE <column> IN (SELECT id FROM <parent> WHERE user_id = ?)
```

`parentTableFor` did not know that `user_blocks.blocker_id` is a *direct*
reference to `users(id)`, so it generated a subquery asking `user_blocks` for a
`user_id` column it does not have. The resulting error contains the words "does
not exist", which `isMissingTable` matches — so the loop tolerated it and
carried on, with the transaction already aborted. Every later statement then
failed, and the first one to report it was an unrelated table.

Two fixes, both worth having:

- `parentTableFor("user_blocks") → "users"`, alongside `community_posts`,
  `security_events` and `audit_logs`, which are direct references for the same
  reason;
- the lesson is recorded here because the masking behaviour remains: any
  tolerated error inside `Erase` aborts the transaction and misattributes the
  failure. Recorded as **G-52**.

**`reports` has no `updated_at` column.** My first reopen wrote one and
PostgreSQL refused it. The reopen now clears `reviewed_by`, `reviewed_at` and
`resolution_note` instead, which is also the honest result: an overturned
dismissal means the report is undecided again, and the moderator's original
reasoning survives on the appeal row rather than being overwritten.

## VERIFICATION

Commands run from `server/` with `source /tmp/toolchain/env.sh` and
`TEST_DATABASE_URL="host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable"`:

```
go test -modfile=/tmp/local.mod -count=1 ./...
  → 34 ok packages, zero FAIL

Named trio for the new state machine:
  go test -run "TestAppeal|TestBlocking" ./internal/moderation   → ok
     TestAppealTransitions, TestAppealTerminalStatesAreExactlyTheDecisions,
     TestAppealStatusVocabularyIsClosed, TestAppealDecisionsAreTheTwoOutcomes,
     TestOnlyDecisionsThatEndAConversationAreAppealable,
     TestAppealStatementBoundsAreEnforced, TestBlockingRules
  go test -run "TestAppeal|TestBlocking|TestBlockedAuthors" ./internal/store  → ok
     TestAppealLifecycle, TestAppealUpheldChangesNothing,
     TestAppealRefusesWhatWasNeverDecided, TestAppealOfADismissedReportReopensIt,
     TestAppealsAppearInTheModerationQueue, TestBlockingIsABoundaryAndNotARecord,
     TestBlockedAuthorsAreExcludedInSQL
  go test -run "TestBlocking|TestAppeal" ./internal/api   → ok
     TestBlockingEndToEnd, TestBlockingFiltersThePublicReader,
     TestAppealEndToEnd, TestAppealOfARejectedConfessionDoesNotPublish

Parity against the live constraints (TestTrialVocabularyParityAgainstConstraint pattern):
  TestAppealVocabularyParityAgainstConstraint
  TestAppealDecisionTypeParityAgainstConstraint
  TestUserBlocksRefuseSelfBlockingAtTheDatabase
  TestUserBlocksAllowOneLiveBlockPerPair
  TestAppealsAreOnePerDecision

gofmt -l internal cmd    → no output
go vet ./...             → clean

EXPORT_ROUTES=1 EXPORT_ROUTES_PATH=<abs>/design/routes.json go test -count=1 -run TestExportRouteTable ./internal/api/
go run -modfile=/tmp/local.mod ./cmd/genspec ../contracts/openapi.json
  → 320 routes / 250 paths / 320 operations (was 308/242/308; +12 for the six
    new routes under both prefixes). Re-running both against the committed
    files leaves zero git diff.
```

From the repository root:

```
python3 design/test_ia.py          → 38 screens, 8 entry points, 113 endpoints wired
python3 design/generate.py --check → generated code up to date (120 tokens, 3 files)
python3 design/test_design.py      → 120 tokens, all Section 12 rules hold
python3 scripts/check_dart_symbols.py → 130 assertions, PASSED 130/130
```

Schema moves to **70 tables / 83 foreign keys**; `conn_test.go` and the
retention audit were updated to the numbers the live database reports.
`design/ia.json` was edited as text: the diff is four lines.

## CONDITIONS CARRIED

- **No mobile UI for blocking or appeals.** The typed client now carries the
  whole surface (`ModerationRepository`) and `check_dart_symbols.py` gates it
  (25 new assertions), but no screen renders it. Blocking and appealing are both
  gestures a listener needs in the moment, so this is a real gap: **G-53**.
- **No Flutter SDK in this sandbox.** `flutter analyze`/`flutter test` could not
  run; owed to CI.
- **`POST /reports` had no typed client method at all** until this phase, so a
  client could not file the report an appeal is the answer to. Added; the
  absence is noted here because it was invisible until appeals made it matter.
- **A report still cannot name a user.** `ReportableEntityTypes` remains
  `{confession, community_post}`. Blocking now covers the harassment case that
  motivated it, but "report this person" is still not a thing the product can
  do: **G-54**.
- **No admin UI for appeals.** They are in the queue API and in `counts`, but
  `apps/admin` is still the retired scaffold, so a moderator decides them with
  curl. This is the PHASE 40 master-plan item, not something this phase can
  close.
