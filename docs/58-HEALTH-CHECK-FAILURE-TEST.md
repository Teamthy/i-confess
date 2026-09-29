# Health-check failure-path test (G-9)

**Date:** 2026-09-29  
**Scope:** test-only; no health-check behavior or route changed.

## Finding

`internal/health.Checker.Handler` already returned HTTP 503 when the database
check failed, but `internal/health` had no test. That left the readiness failure
signal operationally important and unproven.

## Change

Added `TestHandlerReportsDatabaseHealth`, a table-driven HTTP-level test of the
handler's healthy and database-unavailable cases. It checks the status code,
JSON content type, encoded overall and database statuses, and that a failing
check includes an error. The healthy case uses `TEST_DATABASE_URL`; without it,
only that subtest skips while the 503 case still runs against a deliberately
unreachable local port.

## Evidence

```text
cd server
TEST_DATABASE_URL="host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable" \
  go test -modfile=/tmp/local.mod -race -count=1 ./internal/health
ok  github.com/Teamthy/i-confess/internal/health
```

The full Go race suite also passed on the checked-out tree after the toolchain
was restored:

```text
TEST_DATABASE_URL="host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable" \
REDIS_ADDR=127.0.0.1:6379 \
  go test -modfile=/tmp/local.mod -race -count=1 ./...
```

The baseline's `internal/deletion/schema_check.go` and
`docs/56-DELETION-FAILURE-ATTRIBUTION.md` are absent, so these results do **not**
verify the described G-52 patch. See the repository/session discrepancy in the
handoff notes; do not count this baseline run as G-52 evidence.

## Gap status

G-9 is closed for the `internal/health.Checker.Handler` 503 path by the test
above. This does not claim that every separately implemented readiness route
or deployment probe has been exercised.
