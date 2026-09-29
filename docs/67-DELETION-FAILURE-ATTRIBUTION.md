# PHASE 67 — Deletion failure attribution (G-52)

## Finding

`Service.Erase` previously treated any database error whose text contained
"no such table" or "does not exist" as a missing table and continued. That
string match also catches PostgreSQL's missing-column errors. More critically,
PostgreSQL marks the transaction aborted after a statement error, so a later
policy statement then reports only that the transaction is aborted. The later
table is blamed, hiding the original policy/schema defect.

## Changes

- Added `checkPolicySchema` in `internal/deletion/schema_check.go`. Before any
  policy runs, it reads the current schema's table/column catalog once and
  validates every policy reference. Missing tables and columns now fail with
  direct, named diagnostics before destructive SQL starts.
- Removed `isMissingTable` string matching and all error swallowing from
  `Erase`. Every policy execution error is fatal and names the policy table that
  actually failed.
- Extracted ordered execution to `applyPolicies`, which stops immediately at
  the first error. Later statements cannot replace the original failure with
  PostgreSQL's “current transaction is aborted” response.
- Updated the static schema-coverage test to recognize `information_schema` and
  `pg_catalog` as PostgreSQL metadata namespaces, not application tables.

## Tests

- `TestPolicySchemaCheckReportsMissingTableBeforeErasure`
- `TestPolicySchemaCheckReportsMissingColumnBeforeErasure`
- `TestEraseFailsBeforeAnyPolicyWhenSchemaIsIncomplete` — proves preflight
  leaves earlier data untouched.
- `TestPolicyExecutionStopsAtTheFailureThatAbortsPostgresTransaction` — a
  deliberately invalid first policy is attributed to itself, not the following
  policy.
- Existing `TestEveryPolicyGeneratesRunnableSQL` continues to execute all
  generated policy statements against the canonical PostgreSQL schema.

## Verification

With PostgreSQL 17.10 and Redis 7.4 configured:

```
go test -modfile=/tmp/local.mod -race -count=1 ./...
  → all packages passed

go build -modfile=/tmp/local.mod ./...
  → passed

go vet -modfile=/tmp/local.mod ./...
  → passed
```

The first full-suite run exposed a static schema-coverage false positive for the
new PostgreSQL metadata query; the scanner now excludes the two system schema
namespaces while continuing to validate application table references. The
corrected full race suite then passed.

## Result

**G-52 closed.** Schema drift is identified before erasure begins, and runtime
statement failures stop immediately with their true policy context. This phase
implements the fix in the fixed Arena session branch; it does not claim to have
restored the absent handoff commits verbatim.
