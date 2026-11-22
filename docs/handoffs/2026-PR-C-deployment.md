# PR C continuation handoff — 2026-09-20

## Repository state

Work stays on `arena/01a0be8a-i-confess`. The original `/home/user/PR-B-HANDOFF.md`
was absent from this sandbox, and the initial tree was clean at `3eb85cc`.
The unrecoverable original hashes were not recreated. Instead:

- Recovered PR B unchanged from #44 (`b1400b6`, `4d10fb9`), then recovered its
  PR C commit (`07c8bab`). Ran gofmt/vet/full database-backed tests before push.
- #44 was merged externally while work was underway. Merged its updated main.
- #45 was subsequently merged externally at `a6835a7`, while mobile CI was red.
  Merged that updated main too. **Do not assume merged means verified.**
- Follow-up #46 is draft, on the same session branch. Check its current head and
  checks before starting another workstream. No branch switches or force pushes
  are needed. PR B's implementation and security criteria remain unchanged.

## Bootstrap and verification

`scripts/sandbox-bootstrap.sh` is the recovered toolchain recipe. It installs
Go and embedded PostgreSQL from reachable package mirrors and writes
`/tmp/toolchain/env.sh` and `/tmp/local.mod`. No sandbox dependency replacements
belong in `server/go.mod` or `server/go.sum`.

In an agent sandbox, start PostgreSQL with the background process tool rather
than relying on a daemon spawned by a one-shot command. After the script's
initialization stages, the foreground command is:

```sh
source /tmp/toolchain/env.sh
exec "$PGBIN/postgres" -D "$PGDATA" -p 5432 -k /tmp -c listen_addresses=127.0.0.1
```

PostgreSQL is test infrastructure, not a browser preview. Loopback is intentional.

```sh
source /tmp/toolchain/env.sh
cd server
export TEST_DATABASE_URL='host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable'
gofmt -l .
go vet -modfile=/tmp/local.mod ./...
go test -modfile=/tmp/local.mod -count=1 ./...
go test -modfile=/tmp/local.mod -race -count=1 ./...
```

All four passed locally in this continuation (including the full race suite).
Design generation, token and IA checks passed too. Official Go and Dart CI have
also passed; Flutter analysis now passes. Check #46 for the latest Flutter test
result — earlier runs exposed app compilation errors and stale test assertions.

Flutter/Dart dependency resolution and execution are on CI because pub.dev is
unreachable here. `.github/workflows/ci.yml` runs both suites, even when analysis
fails, but still fails the job on any analyzer error/warning or test failure.
Only pre-existing informational lints are non-fatal. Do not skip failing tests.

```sh
gh pr checks 46
gh run list --branch arena/01a0be8a-i-confess
# Log archives may be unreachable; failing commands publish chunked diagnostics:
gh api repos/Teamthy/i-confess/check-runs/CHECK_RUN_ID/annotations --paginate
```

`gh pr edit` currently errors on deprecated classic Projects fields. Use
`gh api -X PATCH repos/Teamthy/i-confess/pulls/46` for title/body if needed.

## Important integration traps fixed here

- Android build ID is not a handset ID: use a persisted random installation id,
  with single-flight initialization. iOS uses the persisted app-scoped identity.
- The backend routes iOS to direct APNs: register `getAPNSToken()`, not FCM's
  token. Firebase tap callbacks do not cover non-FCM APNs; AppDelegate has a
  dedicated event channel. Buffer cold-start taps until listeners subscribe.
- A restored session precedes provider setup: the auth listener must fire
  immediately. Rotation after sign-out cannot register; teardown cancels it.
- Request notification permission only from a user action. Account preferences
  use `scheduled_sessions`/`new_content`, not `scheduled`/`content`.
- StoreKit 2 supplies signed JWS receipts. Do not enable StoreKit 1. Only the
  server's verified entitlement response controls Premium presentation.
- `/subscriptions/verify` and `/subscription` have different response shapes.
  Store plan alone never proves active entitlement.
- Native store prices replace illustrative API prices. No catalogue-only trial
  promises. An authentication/provider/network failure must not finish a paid
  transaction; Restore is the explicit retry path. Subscribe is guarded while busy.
- Riverpod 3 moved StateProvider/StateNotifierProvider to `legacy.dart`.
  AsyncValue.valueOrNull is gone; the separate API Loadable.valueOrNull remains.
- Dart extension endpoint methods need `iconfess_api.dart` in scope at the
  caller, even when the ApiClient provider itself is imported.
- AppScaffold scrolls by default. ListView/Expanded screens need
  `scrollable: false`, otherwise navigation raises unbounded-layout exceptions.

## Remaining release gates / next workstream

Native delivery and store transactions still require configured signed builds
and physical-device/provider sandbox tests. See `PUSH_AND_SCHEDULING.md` and
`BILLING.md`; unit fakes cannot validate signing, provisioning or live delivery.

PR D has **not** been implemented. Next: numeric non-root Docker user and owned
writable directories compatible with the production k8s security context,
deploy workflows, and CDN asset delivery. Keep this separate from PR B's signed
webhook/idempotency/ordering/server-entitlement rules. Do not deploy live
resources just to exercise a workflow.
