# PHASE 44 — RBAC: roles open modules, modules own routes

**Master-plan coverage:** item 40 (admin platform) — the role-based access
control half. The command-centre surfaces are a later phase; this one makes the
roles mean something for them to be built on.

**Gap closed:** **G-57** (four of the seven administrative roles —
content_admin, theological_reviewer, support_admin, analytics_admin — could be
granted, appeared in `GET /admin/users/admins`, and opened no route at all).
Also closed on the way: the theological review outcome (PHASE 40's columns) had
no writer but the seed and no reader at all, and `isValidAdminRole` omitted
voice_manager so the API could never grant the product's highest-consequence
role.

**Verdict:** PASS

## OBJECTIVE

Verified against the code before building:

```
$ sed -n 22p server/internal/api/router.go
    admin := auth.RequireRoleWithSessions(h.cfg.JWTSecret, sv)
```

`RequireRoleWithSessions` admits super_admin plus whatever roles it is handed;
handed none, it admits super_admin alone. Forty routes were wrapped with that
`admin`. Three more were wrapped for `voice_manager` and eight for
`audio_producer,voice_manager`, by hand. Nothing named the other four roles
anywhere in the router, so `TestRoleScopingIsEnforced` asserted — correctly, for
the code as it stood — that a content admin is *denied* `/admin/categories`.

The route table already grouped the admin routes into tags (`admin-content`,
`admin-voice`, `admin-audio`, `admin-users`, `admin-ops`, `admin-queue`). The
tags described the modules; the wrappers ignored them. Two further findings
while reading:

- `theological_review_status` and its three companions were added in PHASE 40
  and written only by the seed. No endpoint recorded a review and none read one,
  so the "mandatory theological review" of directive §22 was a lifecycle state
  a content admin could move through by PATCHing past it.
- `isValidAdminRole` listed six roles. The seventh, voice_manager, was granted
  by the seed and could never be granted through `POST /admin/users/role`.

## IMPLEMENTATION

### One matrix, read by the gate and by the map

`internal/auth/rbac.go` is the authority. Eleven modules —
`dashboard users content review moderation voices audio subscriptions
analytics roles system` — two access levels (`read` is GET/HEAD; `write` is
everything else and includes read), and one table:

| role | modules |
|---|---|
| super_admin | everything, write (never listed; the middleware admits it unconditionally) |
| content_admin | dashboard r · **content w** · **moderation w** · review r |
| theological_reviewer | dashboard r · content r · **review w** |
| audio_producer | dashboard r · content r · **audio w** · voices r |
| voice_manager | dashboard r · **voices w** · **audio w** |
| support_admin | dashboard r · **users w** · subscriptions r |
| analytics_admin | dashboard r · analytics r · content r · subscriptions r · users r |

`roles` (grant/revoke roles, erase accounts) and `system` (queue, security
counters, audit log, process metrics) are held by nobody below super_admin: a
role that can grant roles is super_admin under another name.

A route declares its module in its auth label — `"admin:content"` — and
`routeAuth(level, method)` resolves the label and the method to
`auth.RolesWith(module, access)`, which is what `RequireRoleWithSessions` is
handed. The `admin`, `voiceMgr` and `audioMgr` wrappers in `router.go` are
gone; every one of the 84 administrative registrations (42 routes × 2 prefixes)
passes `nil` and is enforced from its label. A label that is administrative but
names no module — including the bare `"admin"` that used to mean everything —
panics at registration, which every API test performs, so a route cannot be
administrative and unmapped.

`GET /admin/access` returns the caller's role and the modules it holds, read
from the same table. The command centre will build its navigation from it
rather than from a copy, so it cannot show a locked door or hide an open one.

### The reviewer gets the review

`POST /admin/confessions/{id}/review` (module `review`) records an outcome —
`reviewed` or `needs_revision` — with the caller's email as reviewer of record
and optional notes; `needs_revision` requires notes, because sending text back
without saying what must change is not a review. `unreviewed` cannot be written
back: a review that happened is a fact, corrected by another review. `GET` on
the same path returns the record; content admins read it, reviewers write it.
`content.ReviewStatus` is the closed vocabulary; the seed's constants now alias
it and `TestTheologicalReviewVocabularyParityAgainstConstraint` compares it to
the live CHECK instead of to a literal list.

The outcome is load-bearing. `UpdateConfessionStatusAudited` refuses the
`theological_review → audio_production` edge unless the outcome is `reviewed`,
returning `ErrReviewRequired`, which the PATCH handler renders as `409
CONTENT_REVIEW_REQUIRED` with the current outcome so the admin knows whether to
chase a reviewer or a rewrite. That is the same shape as the §75 QA gate on
`audio_qa → approved`. `TestAdminCanMoveAConfessionThroughTheWholeLifecycle` now
records the review on the way through, and asserts the 409 first.

### The analyst gets a number

`GET /admin/analytics?days=N` (module `analytics`, 1–365, default 30) returns
`store.AnalyticsOverview`: events by name in the window, trial states and
subscriptions by status over the whole population, sessions created and
completed, distinct active listeners and new accounts in the window. Every
figure is a count over rows that already exist. It is the first route
analytics_admin has ever held, and the module the matrix names had to have a
door — `TestRBACMatrixIsEnforcedOnEveryAdminRoute` fails on any module with no
route.

### Small repairs

- `isValidAdminRole` defers to `auth.ValidRole`, which knows all seven roles.
- The dead `adminSetRole`/`validRole` pair in `admin.go` (a staticcheck U1000
  in the lint baseline) is deleted.
- The served admin page's role picker offers voice_manager.

## FOUND WHILE BUILDING

- **Registration is rate-limited per address at five per ten minutes**, and
  a matrix sweep signs up seven administrators. The sweep sends each
  registration from its own `X-Forwarded-For`, which is what seven
  administrators on seven laptops do; it does not raise the limit.
- **The lint stand-in was running the wrong check set.** CI's golangci-lint
  enables staticcheck's quickfix checks; the local `staticcheck` default does
  not, which is how a QF1001 in the PHASE 43 test reached CI. The stand-in now
  runs golangci's default set against a fresh main baseline (commit `a236e48`).

## VERIFICATION

Commands run from `server/` with `source /opt/tools/env.sh`:

```
gofmt -l .                       → no output
go build ./...                   → clean
go vet ./...                     → clean
/opt/tools/lint.sh               → no new findings (golangci's staticcheck
                                   check set, errcheck, ineffassign vs main)
go test -race ./... -count=1     → 35 ok packages, zero FAIL

Named:
  go test -run . ./internal/auth
     TestEveryRoleOpensAtLeastOneModule, TestSuperAdminHoldsEverythingAndIsNeverListed,
     TestRolesAndSystemAreSuperAdminOnly, TestWriteIncludesReadAndReadDoesNotIncludeWrite,
     TestTheMatrixMatchesTheTitles, TestAccessForMethods, TestAdminLabels,
     TestRolesAreExactlySeven, TestRolesWithIsDeterministic
  go test -run 'TestRBAC|TestAdminAccess|TestTheologicalReview|TestAdminAnalytics|TestRoleScoping|TestEveryAdminRoute' ./internal/api
     TestRBACMatrixIsEnforcedOnEveryAdminRoute
       → swept 294 role×route pairs; routes open per role:
         analytics_admin 8, audio_producer 15, content_admin 14, super_admin 42,
         support_admin 5, theological_reviewer 7, voice_manager 15;
         routes per module: analytics 1, audio 8, content 7, dashboard 2,
         moderation 4, review 2, roles 3, subscriptions 3, system 5, users 2, voices 5
     TestAdminAccessReportsTheCallersModules
     TestTheologicalReviewIsRecordedByTheReviewerAndGatesProduction
     TestAdminAnalyticsCountsThePopulation
     TestRoleScopingIsEnforced (8 cases, rewritten to the matrix)
     TestEveryAdminRouteRejectsANonAdmin (now keyed on auth.IsAdminLabel)
  go test -run TestTheologicalReviewVocabularyParityAgainstConstraint ./internal/db

EXPORT_ROUTES=1 EXPORT_ROUTES_PATH=<abs>/design/routes.json \
  go test -count=1 -run TestExportRouteTable ./internal/api/
go run ./cmd/genspec ../contracts/openapi.json
  → 328 routes / 256 paths / 328 operations (was 320/250/320; +8 for
    access, analytics, review GET and POST under both prefixes)
```

From the repository root:

```
python3 design/generate.py --check    → generated code up to date (120 tokens, 3 files)
python3 design/test_design.py         → 120 tokens, all Section 12 rules hold
python3 design/test_ia.py             → 38 screens, 8 entry points, 113 endpoints wired
python3 scripts/check_dart_symbols.py → PASSED 133/133
```

Schema unchanged (70 tables / 83 foreign keys); no migration. `design/ia.json`
untouched — administrative routes are outside the listener IA by design.

## CONDITIONS CARRIED

- **Modules with one door.** `analytics` has a single overview route and
  `users` has two (list admins, suspend/restore). The command-centre phase adds
  the read surfaces those modules need — user search, sessions, subscriptions,
  scriptures, system — each registered under the module that already governs
  it, so no new permission decision is deferred to the UI.
- **Subscription changes stay super_admin.** `POST /admin/users/subscription`
  and `PUT /admin/plans` are `subscriptions` writes and nothing below
  super_admin holds them. Support agents read plans. If comping a subscription
  becomes a support workflow, it should be a narrower endpoint with its own
  audit, not a widening of this module.
- **Read/write is the whole grain.** A role that should approve but not
  publish is a handler-level distinction, not a matrix one. None was needed
  in this phase.
