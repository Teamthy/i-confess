# PHASE 31 — MODERATION: REPORTING, THE QUEUE, SUBMISSION, REVIEW AND THE QA GATE

## OBJECTIVE

Close the last unimplemented HTTP surface on the backend. `docs/PROJECT-STATUS.md`
listed it verbatim: *"4 handlers still return 501: recommendations, confession QA,
moderation queue, user confession review."* PHASE 22/PR #36 took recommendations;
this phase takes the other three and the intake they are dead without:

1. **Reporting** — a signed-in user can flag content (`POST /reports`).
2. **The queue** — a moderator can see all pending work (`GET /admin/moderation/queue`).
3. **Submission** — an author can offer a personal confession for review
   (`POST /me/confessions/{id}/submit`).
4. **Review** — a moderator can decide a queued confession
   (`POST /admin/moderation/user-confessions/{id}/review`, approved|rejected).
5. **The QA gate** — `POST /admin/confessions/{id}/qa` runs the §75 audio QA
   checklist and is the one enforced edge of the editorial lifecycle:
   `audio_qa → approved`.

A queue that cannot be drained is not a queue, so deciding a report is in
scope: `POST /admin/moderation/reports/{id}/decision` (resolved|dismissed).

### Reconciliation note (read this before trusting anything below)

This phase exists twice. A previous delivery report commit
`d19a47aa2e4527d7baac2dfc02778d969dc3a6b0`
("PHASE 31 — moderation: reporting, the queue, submission, review and the QA
gate") was claimed as HEAD of branch `arena/01a0baf6-i-confess`, 13 commits
ahead of `origin/main`. In this sandbox, on 2026-09-19, every part of that
claim was verified false:

```
$ git rev-parse HEAD
5a94fd98542827ada914ff19406d691bdadb4ec0          # the merge of PR #39, 0 ahead of main
$ git cat-file -t d19a47aa2e4527d7baac2dfc02778d969dc3a6b0
fatal: git cat-file: could not get object info
$ git fetch origin '+refs/heads/*:refs/remotes/origin/*'
$ git log --all --oneline --grep="PHASE 31"        # no output
$ ls /home/user/arena-delivery/                    # does not exist
```

The remote branch `arena/01a0baf6-i-confess` exists but points at `62f3bf5`,
one chore commit on top of the PR #39 merge. The 13 commits, the local export
and the fallback export are unrecoverable from here. This document therefore
describes work **rebuilt from main** (`5a94fd9`) in session
`arena/01a0bb8b-i-confess`. The scope was taken from the lost commit's title
and from the route table, which never lies about what is missing.

## INPUTS

- `server/internal/api/home.go` — the three `notImplemented(...)` stubs and the
  comment admitting "the moderation queue has no store at all".
- `server/internal/api/router.go` — the three registered routes answering 501,
  and the absence of any `/reports` or `/submit` route.
- `server/internal/db/migrations/0001_baseline.sql` — `moderation_cases`,
  `reports`, `content_moderation_history`, the `user_confessions` review
  columns and `confessions.qa_passed_at` / `qa_report`: the schema designed in
  §10/§22/§75 terms before any of it ran.
- `server/internal/db/migrations/0002_status_constraints.sql` — the status
  vocabularies those tables were constrained to in PHASE 07.
- `internal/content/lifecycle.go` and `internal/audio/lifecycle.go` — the two
  existing authorities whose shape this phase mirrors (one vocabulary, one
  graph, one parity test against the live constraint).
- `docs/12-CONTENT-GOVERNANCE.md` — the editorial lifecycle this gate caps.
- `docs/ENGINEERING-PIPELINE.md` — the process this document follows.

## DEPENDENCIES

- Go 1.27.1 (go-bin wheel; the sandbox has no preinstalled toolchain and the
  module proxy is unreachable — see `.arena/state.json` environment_notes).
- PostgreSQL 17.10 at `TEST_DATABASE_URL`, used for every test claim below.
- Migration `0009_moderation.sql` (this phase) applied by the embedded
  migrator; all server tests build their database from the full chain.
- No changes to committed `go.mod`/`go.sum`; the sandbox-only
  `go.local.mod`/`go.local.sum` (x/crypto and x/{net,sys,term,text} wired to
  their github.com/golang mirrors) stays untracked.

## FINDINGS

- **F-1. The model hid the lifecycle.** `user_confessions` has carried
  `status`, `visibility`, `review_notes`, `reviewed_by`, `reviewed_at`,
  `rejection_reason`, `version` and `published_at` since the baseline schema,
  but `models.UserConfession` had none of them and the store never scanned
  them. A confession created through the API was a `draft` in the database and
  stateless in the client's eyes.
- **F-2. `content_moderation_history` had no writer.** The table exists to
  answer "who moved this confession, from what, to what, and why", and no
  query in the codebase inserted into it. `PATCH /admin/confessions/{id}` —
  the only canonical status write path — updated the row and recorded nothing.
- **F-3. The old PATCH destroyed provenance and 500'd on a phantom id.** Its
  UPDATE set `published_at = NULL` on every move except into `published`, so
  unpublishing erased the original publish date; and it never checked
  rows-affected, so a missing id returned 500 "failed to update confession".
- **F-4. `user_confessions.visibility` was the only closed vocabulary with no
  CHECK.** Every other constrained column got one in PHASE 07 for exactly this
  reason: an unconstrained vocabulary is whatever the first typo writes.
- **F-5. Reports had no decision columns.** `reports` could not record who
  decided, when, or why — so closing a report could only ever have been a
  silent status flip.
- **F-6. `POST /reports` is a queue-stuffing surface without a gate.** One
  scripted account could file unbounded duplicate complaints; each is a row a
  human must close. Two defences were needed: one open report per reporter per
  entity (a partial unique index), and a per-account rate limit.
- **F-7. The seeded demo catalogue fails its own QA gate.** Live smoke (below)
  shows the 78 seeded confessions' renders ready but their voice holding no
  `voice_rights` row: the `voices_licensed` check fails all four renders. The
  gate caught, on its first live run, the licence gap the generation-time
  rights gate was built for. Recorded as G-42.

## IMPLEMENTATION

One authority, one store, six handlers, one migration.

### `internal/moderation` (new package)

The single authority for the UGC moderation lifecycle, the report and case
vocabularies, and the QA checklist — mirroring how `internal/content` owns the
editorial lifecycle.

- **UGC lifecycle.** `draft submitted approved rejected published archived`,
  with the enforced graph: `draft/rejected → submitted`, `submitted →
  approved|rejected|published|archived`, everything else refused. Two rules
  carry the weight: a draft cannot be reviewed (deciding on something the
  author never offered would hide intent), and an approved confession cannot
  re-enter the queue (re-deciding rewrites history). A rejected one can be
  re-submitted — that is an author action, not a moderator reversal.
- **Visibility.** `private shared public` — §22's UGC vocabulary, deliberately
  not the collections vocabulary (`private unlisted public`); different
  entities, different lists, and each is now asserted against its own CHECK.
- **Reportables.** Only entities a signed-in user can honestly read:
  `confession` (published or deprecated — deprecated content still plays in
  existing sessions, so it can still offend) and `community_post`. A
  `user_confession` has no reader but its author today, so accepting reports
  against one would be validating ids nobody could have obtained; when UGC
  gains a public reader, `ReportableEntityTypes` is the one list to extend.
- **`RunQAChecklist`** is a pure function over gathered facts (text presence,
  ready renders, renders in flight, unlicensed voices), so the gate logic is
  testable without a database and the SQL only fetches.

### `internal/db/migrations/0009_moderation.sql`

1. Normalise + `CHECK` on `user_confessions.visibility` (F-4).
2. `reports` gains `reviewed_by`, `reviewed_at`, `resolution_note` (F-5),
   nullable so open rows stay valid and "undecided" stays representable.
3. Partial unique index: one **open** report per reporter per entity (F-6).
4. Partial unique index: one **open/in_review** case per entity — two reports
   are two reports and one queue entry; a fresh complaint after resolution
   reopens work instead of being swallowed by history.
5. `SELECT status`-side index on `reports` for the queue read.

### `internal/store` (`moderation.go`, plus edits)

- `ModerationStore`: reports (create-with-dedupe, decide), cases (open,
  resolve), UGC (submit, review), the queue read, the QA gate, and
  `UpdateConfessionStatusAudited`.
- Every multi-write operation runs in one transaction with the target row
  `FOR UPDATE`: a review cannot interleave with a resubmission into a state
  the graph forbids, and two concurrent decisions on one report cannot both
  close the case.
- **Approve honours the author's asked audience.** `visibility=public` lands
  in `published`; anything else lands in `approved`. Moderation gates
  publication; it does not grant an audience the author never requested.
- **Rejection requires a reason, twice.** The handler returns 422 and the
  store refuses again — same defence-in-depth contract the old
  `UpdateConfessionStatus` kept, because a rejection the author cannot act on
  is only closure for the moderator.
- **Dedupe is not an error.** On the unique-index conflict the store returns
  the existing open row with `alreadyExisted=true`; the reporter's intent is
  already on record, and failing would teach a repeat caller their first
  report vanished.
- **Cases close when the work is done.** Deciding the last open report on an
  entity resolves its case; reviewing a queued confession resolves its case.
  Both record `before`/`after`, actor and detail.
- **The QA gate writes its report either way** (`qa_report` JSON always;
  `qa_passed_at` only on pass) and, on a pass, transitions to `approved` and
  writes a `content_moderation_history` row. A failed gate leaves evidence —
  a missing report after a claimed run would mean the run never happened.
- **`UpdateConfessionStatusAudited`** fixes F-2 and F-3: `FOR UPDATE` read
  (missing id → `ErrNotFound` → 404, not 500), `published_at` preserved unless
  entering `published`, a history row per real move, and no row for a no-op
  PATCH (a no-op must not fabricate history). The old un audited store method
  is deleted; `PATCH` passes an optional `reason` through.
- `EngagementStore.CreateUserConfession` / `ListUserConfessions` now write and
  scan status/visibility, so F-1's drift cannot return: the API object is the
  database row.

### `internal/api/moderation.go`

Six handlers, replacing the three stubs and adding the three missing intake
routes:

| Route | Purpose |
|---|---|
| `POST /reports` (+`/v1`) | file a report; 201 new / 200 `already_reported`; 404 (vague, S80) when the entity is missing, unpublished or unseeable |
| `POST /me/confessions/{id}/submit` (+`/v1`) | offer a draft/rejected confession; 404 on a foreign id (§71, never 403); 409 `{status, allowed}` on an illegal move |
| `GET /admin/moderation/queue` | three sections (UGC, reports, editorial pending) oldest-first + counts incl. open cases; sections serialise `[]`, never `null` |
| `POST /admin/moderation/user-confessions/{id}/review` | approve → publish-if-public / reject-with-reason; 409 on a draft; review fields and case closure recorded |
| `POST /admin/moderation/reports/{id}/decision` | resolved|dismissed only — "reviewed" exists in the vocabulary for a future triage step, and closing a report must state an outcome; 409 on a closed report |
| `POST /admin/confessions/{id}/qa` | 404 missing · 409 wrong lifecycle state · 422 with the full checklist on fail · 200 with the report on pass |

`createReport` is throttled per account
(`ratelimit.ReportSubmission`: 8 per 10 minutes) — F-6's second defence,
keyed `report:acct:<user>` so it holds under NATs where per-IP would not.

`contracts/openapi.json` and `design/routes.json` were regenerated from the
live route table (286 routes; the six new endpoints, each in both prefix
families). The committed spec is never hand-edited.

## TESTING

Every command below was run in this sandbox on 2026-09-19 with Go 1.27.1
(`-modfile=go.local.mod`, `GOPROXY=file:///tmp/goproxy,direct`, `GOSUMDB=off`,
`GOFLAGS=-mod=mod`) and PostgreSQL 17.10 at `TEST_DATABASE_URL`. The one
pre-existing environmental failure (`internal/storage`
`TestContentTypeMapping`: this container's `/etc/mime.types` maps `.xyz` to
`chemical/x-xyz`; passes on CI runners) is unchanged from main and is not from
this phase.

### Gates

```
$ gofmt -l .                                   # (empty — clean)
$ go build -modfile=go.local.mod ./...         # BUILD-DONE rc=0
$ go vet -modfile=go.local.mod ./...           # (no findings)
$ go test -modfile=go.local.mod -race ./...
ok  github.com/Teamthy/i-confess/internal/api          40.121s
ok  github.com/Teamthy/i-confess/internal/moderation    3.085s
ok  github.com/Teamthy/i-confess/internal/store         6.873s
   ... (every other package ok)
FAIL github.com/Teamthy/i-confess/internal/storage — TestContentTypeMapping
     (pre-existing, environmental; /etc/mime.types on this image)
```

### What the new tests prove

`internal/moderation/moderation_test.go`:
- Vocabulary parity, both directions, read from the live database
  (`pg_get_constraintdef`): UGC statuses, UGC visibilities, report statuses,
  case statuses. The Go authority and the CHECK constraints cannot drift
  silently — the same contract `internal/content` keeps.
- Every UGC status and every UGC visibility is actually writable, not merely
  listed (the G-5 class of bug: five editorial states once passed validation
  and died at the constraint).
- The transition table: legal moves pass; `draft→approved`,
  `draft→published`, `approved→rejected`, `rejected→approved` (a rejection
  stands until resubmitted), `archived→submitted` are refused. A draft is not
  reviewable; an approved confession cannot re-enter the queue.
- The QA checklist: all-healthy facts pass; each single broken fact fails
  exactly its own check, with a non-empty actionable reason.

`internal/store/moderation_test.go` (14 tests against a real database):
- Report dedupe on the partial unique index (same row back, `alreadyExisted`),
  one case per entity across two reporters, decisions recorded with actor and
  note, the case closing with the last open report, a second decision on a
  closed report refused (`ErrReportNotOpen`), a fresh complaint after
  resolution reopening a case.
- Report entity validation: missing, unpublished and unseeable ids are not
  reportable.
- Submit: draft → submitted opens a case; double submit →
  `UGCTransitionError`; foreign id → `ErrForbidden` (the API renders it 404);
  a rejected confession resubmits and its old review fields clear.
- Review: public intent → `published` with `published_at`; private intent →
  `approved` without; reasonless rejection refused without side effects;
  reviewing a draft refused; the case resolves with before/after.
- The queue: starts with only editorial pending; submissions and reports show
  up with correct counts; drains to zero as work is decided; sections are
  empty slices, not nil.
- QA gate: pass moves `audio_qa→approved`, sets `qa_passed_at`, persists the
  report, writes one history row; a render still `processing` fails
  `audio_ready` + `no_render_in_flight` and transitions nothing while still
  persisting the failed report; a ready render on an unlicensed voice fails
  `voices_licensed`; wrong-state and missing ids return the typed errors.
- Audited PATCH: `published_at` set on publish and **preserved** on archive
  (the old UPDATE erased it); two real moves write two history rows; a no-op
  writes none; missing id → `ErrNotFound`.

`internal/api/moderation_test.go` (3 end-to-end suites over HTTP against a
live handler and database):
- `TestReportingEndToEnd` — 401 unauthenticated; 400 bad entity type; 422
  non-actionable reason; 404 unseeable id; 201 first filing; 200 +
  `already_reported` duplicate; queue counts; 400 non-outcome decision; 200
  decision recorded with moderator + note; 409 re-decision; queue drained.
- `TestUserConfessionSubmissionAndReview` — create/list expose the lifecycle;
  404 (not 403) on foreign submit; 409 double submit; 422 reasonless
  rejection with no side effect; resubmit; approval publishes with reviewer +
  `published_at`; reviewing a draft → 409; missing id → 404.
- `TestConfessionQAGate` — 422 with all four checks reported on a render in
  flight; status and `qa_passed_at` untouched while the failed report is
  persisted; 200 once the render is ready; 409 re-running from `approved`;
  404 missing; PATCH history (exactly 2 rows) and phantom-id 404.

### Fault injection (each mutation reverted to an md5-verified backup)

| # | Mutation | Expected | Observed |
|---|---|---|---|
| INJ-1 | QA `audio_ready` counts `processing` instead of `ready,published` | gate tests fail | `TestRunConfessionQAPassesAndRecords` FAIL ("no audio render has passed audio QA yet"); `TestRunConfessionQAFailsWithoutReadyAudio` FAIL — caught |
| INJ-2 | review maps public→`VisibilityPrivate` (publish never fires) | publish test fails | `TestReviewPublishesOnlyWhatTheAuthorOfferedPublicly` FAIL: `status = "approved", want published` — caught |
| INJ-3 | `CreateReport` never opens a case | case tests fail | `TestDecideReportClosesCaseWithLastOpenReport` FAIL ("case should be open, found 0"); `TestReportDeduplicatesPerReporterPerEntity` FAIL — caught. (The first run's `tail` truncated the second failure; re-run with `-run` confirmed it before concluding, per the pipeline rule that a MISSED injection must be verified, not assumed.) |

### Live smoke (real server, `ENV=development`, seeded `iconfess` database)

```
POST /me/confessions                       201  status=draft visibility=public
POST /me/confessions/14ea3086.../submit    200  status=submitted
GET  /admin/moderation/queue               200  counts{user_confessions:1, open_cases:1}
POST /admin/moderation/user-confessions/…/review 200  status=published (reviewed_by, published_at set)
POST /reports                              201  open / already_reported:false
POST /reports (same again)                 200  already_reported:true
POST /admin/moderation/reports/…/decision  200  dismissed (reviewed_by set)
GET  /admin/moderation/queue               200  counts{all 0}
POST /admin/confessions/{published}/qa     409  (gate fires from audio_qa only)
PATCH /admin/confessions/{id} → audio_qa   200
POST /admin/confessions/{id}/qa            422  passed:false — text_present ✓, audio_ready ✓
                                                (4 renders), no_render_in_flight ✓,
                                                voices_licensed ✗ "4 servable renders use
                                                voices with no active licence"  → F-7 / G-42
PATCH /admin/confessions/phantom-id        404  (was 500 before this phase)
```

The smoke database was dropped and recreated empty after the run, so the next
development boot re-seeds from scratch.

## SECURITY REVIEW

- **Authorisation.** The three new admin routes carry the same
  `RequireRoleWithSessions(super_admin)` wrapper as every other admin route;
  `TestEveryAdminRouteRejectsANonAdmin` enumerates the route table and hits
  each as a plain user — it does not hardcode a count, so the three additions
  are covered automatically.
- **Ownership and enumeration.** Submitting a foreign confession returns 404,
  not 403 (§71); a report against an unseeable id returns a vague 404 (S80),
  so the endpoint cannot be used to map unpublished or deleted content. User
  confessions remain invisible to everyone but their author; nothing in this
  phase adds a read path for them.
- **Abuse of the intake.** Reports are deduplicated per reporter per entity
  (partial unique index), throttled per account (8/10 min; Redis-backed in
  production like the auth limits), and length-bounded
  (`reason ≤ 140`, `detail ≤ 2000`) — unbounded user text in a moderator's
  inbox is a denial-of-wallet against a human.
- **Auditability.** Review and gate actors are written from verified JWT
  claims (`auth.FromContext(r).Sub`); FK `ON DELETE SET NULL` keeps history
  readable after an admin account is erased (the erasure policy already
  anonymises `content_moderation_history.actor`). No handler writes `admin`,
  `""` or a header-supplied identity.
- **Fail direction.** The QA gate fails closed twice over: it refuses to run
  from any lifecycle state but `audio_qa` (it cannot skip §22's reviews), and
  a failed checklist transitions nothing while still persisting the evidence.
  A licence revoked between generation and approval converts the gate from
  pass to fail (`voices_licensed`).
- **No new secrets, no new network surface, no production action.** All smoke
  action was local; seeded demo credentials were used locally only.

## DOCUMENTATION

- This document: `docs/31-MODERATION.md`.
- `docs/PROJECT-STATUS.md`: PHASE 31 entry; the "4 handlers return 501" row
  removed from Not done; open-gap register updated (G-2 de-duplicated out of
  the open list where it was listed both open and closed; G-40, G-41, G-42
  added below).
- `.arena/state.json`: rewritten for this session's branch and phase with the
  reconciliation recorded (see OBJECTIVE).
- `contracts/openapi.json` and `design/routes.json`: regenerated from the live
  route table (286 routes, diff purely additive: the six new endpoints).
- `design/ia.json` (CI follow-up): the regenerated `routes.json` made the IA
  checker's reverse check — every *user-facing* endpoint must be referenced by
  a screen — see three endpoints for the first time: the two new public ones
  and `DELETE /sessions/{id}`, a PHASE-17 orphan the stale `routes.json` had
  been hiding. `POST /reports` and `POST /me/confessions/{id}/submit` are now
  mapped to `confess/confession`; `DELETE /sessions/{id}` to
  `activity/session`. `python3 design/test_ia.py` →
  `IA CHECK PASSED: 37 screens, 8 entry points, 102 endpoints wired`
  (was 99).
- Inline: `internal/api/moderation.go`, `internal/moderation/*`,
  `internal/store/moderation.go` carry the why, not just the what.

## EXIT CRITERIA

| Criterion | Evidence |
|---|---|
| No route on the moderation surface answers 501 | `grep -rn "notImplemented(" internal/`: zero call sites remain; the helper is deleted |
| A user can report, a moderator can decide, and the intake cannot be flooded | `TestReportingEndToEnd`; `TestReportDeduplicatesPerReporterPerEntity`; `TestDecideReportClosesCaseWithLastOpenReport`; live smoke |
| An author can submit; review publishes only what was offered publicly; rejection needs a reason | `TestSubmitUserConfessionFlow`, `TestReviewPublishesOnlyWhatTheAuthorOfferedPublicly`, `TestReviewRequiresAReasonToReject`, `TestReviewRefusesADraft`; live smoke |
| The queue shows all three kinds of pending work and drains as it is done | `TestModerationQueueDrainsAsWorkIsDone`; live smoke |
| The §75 gate gates: wrong state 409, missing render 422, unlicensed voice 422, healthy 200 → `approved` with `qa_passed_at` and history | four `TestRunConfessionQA*` tests; `TestConfessionQAGate`; live smoke 422 |
| Canonical status changes are audited and `published_at` is never erased | `TestUpdateConfessionStatusAudited` |
| Vocabulary parity with the database, both directions | four parity tests + two writability tests in `internal/moderation` |
| Fault-injected protections fail the suite | INJ-1/2/3 table above |
| gofmt clean, build, vet, `-race` suite green (modulo the pre-existing environmental storage failure) | Gates block above |
| Generated artefacts regenerated, never hand-edited | genspec + `TestExportRouteTable` run above; diff verified purely additive |

## VERDICT

**PASS WITH CONDITIONS.**

- **C-1 (G-40): approved/published UGC has no reader.** The feed serves
  `community_posts`; nothing lists `user_confessions` with
  `status='published'` for other users. The pipeline now ends at
  "published", and publication changes no user's experience yet. Owner: the
  phase that builds the public UGC stream; precondition: none — it can start
  whenever scheduled.
- **C-2 (G-41): `audit_logs` still does not cover content and moderation
  actions.** This phase writes its audit to `moderation_cases` and
  `content_moderation_history` (with before/after, actor, reason), which is
  the trail moderators query; the admin-wide `audit_logs` sink remains wired
  only wherever earlier phases wired it. Owner: the observability/admin phase;
  precondition: decide one audit story rather than two.
- **C-3 (G-42): the seeded demo catalogue fails its own QA gate
  (`voices_licensed`).** The 78 seeded confessions' renders carry no
  `voice_rights` row, so reaching `approved` through the gate is impossible in
  a development seed without hand-inserting licences. Not a production blocker
  (no production content exists — G-34/G-35 stand), but the development
  workflow QA exercise needs a licensed fixture. Owner: the seed/content phase.
