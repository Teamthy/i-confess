# Report a person (G-54)

**Date:** 2026-09-29

**Scope:** extend the existing reporting entity vocabulary and typed client. No
database schema or route shape changed.

## Behavior

- `entity_type: "user"` reports a person's conduct as one case, allowing a
  moderator to assess a pattern across multiple posts rather than requiring one
  report per post.
- The store accepts a target only if the user exists and is not soft-deleted or
  in the terminal `deleted` state. The response for an unavailable user remains
  the existing generic 404.
- A listener cannot report their own account. The request is rejected with
  `REPORT_INVALID` before it consumes the per-account filing limit.
- Person reports use the existing deduplication, rate limit, audit, moderation
  case queue, and resolve/dismiss lifecycle. No new persistence structure was
  necessary: report and case entity types are already stored as text.
- The Dart client adds `ReportableEntityType` (`confession`, `communityPost`,
  `user`) and the repository serializes its wire value. The route summaries now
  say that people can be reported.

## Evidence

Targeted PostgreSQL race tests passed:

```text
cd server
TEST_DATABASE_URL="host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable" \
REDIS_ADDR=127.0.0.1:6379 \
  go test -modfile=/tmp/local.mod -race -count=1 ./internal/moderation ./internal/store ./internal/api \
  -run 'TestReportableEntityTypes|TestReportRejectsUnseeableEntities|TestReportingAUserCreatesOneModerationCase|TestReportingEndToEnd'
```

`TestReportingAUserCreatesOneModerationCase` covers self-report rejection,
missing-user 404, filing a report, creation of one open user case, and moderator
resolution. Store coverage checks existing, missing, and deleted user targets.
The live route table and OpenAPI summaries were regenerated from the handlers.
`python3 design/test_ia.py` passed (39 screens, 8 entry points, 162 endpoints),
and `python3 scripts/check_dart_symbols.py` passed (133/133).

A Dart repository test asserts `ReportableEntityType.user` serializes to
`"user"`; Dart/Flutter SDKs are absent from this sandbox, so that test and
`flutter analyze` are owed to CI. The full Go race suite is run separately for
this ledger and recorded in `PROJECT-STATUS.md`.

## Gap status

G-54 is closed: an authenticated listener can file a person-level report, which
is validated and enters the existing moderator queue without changing the API
shape or persistence schema.
