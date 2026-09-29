# API handler composition refactor (G-25)

**Date:** 2026-09-29

**Scope:** Go source organization only. No route, schema, request/response contract,
or handler behavior was intentionally changed.

## Change

Split the endpoint methods out of `server/internal/api/handlers.go` into
cohesive package files: authentication, admin roles, catalog, sessions,
schedules, library/playback, user confessions, and auth sessions. The
composition-root file now holds the `Handler` and `Config` definitions,
construction, dependency setters, and shared wiring.

`handlers.go` is now 247 lines (was 1,656); the handler methods moved to eight
files. A function-inventory comparison against the parent version found no
missing functions. The APIs remain in package `api`, so receiver types and route
registrations are unchanged.

## Evidence

```text
cd server
gofmt -l .                         # no output
go build -modfile=/tmp/local.mod ./...
go vet -modfile=/tmp/local.mod ./...
TEST_DATABASE_URL="host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable" \
REDIS_ADDR=127.0.0.1:6379 \
  go test -modfile=/tmp/local.mod -race -count=1 ./...
```

All passed on the refactored working tree. In particular,
`TestRouteParityBetweenPrefixes` and the full `internal/api` race tests passed.
The route export, OpenAPI output, and schema were not edited.

## Gap status

G-25 is closed as a composition-file-size/refactor finding: the 1,656-line
file is now 247 lines, with endpoint responsibilities in named files. This is
not a claim that all API handlers are small or that future handler growth is
prevented. The next change adding an API surface should update the relevant
cohesive handler file rather than restoring a monolithic composition file.
