# PHASE 38 — Retention and row versioning

**Gap closed:** `G-21 (PHASE 38: soft-delete coverage across application
tables)` and `G-22 (PHASE 38: row-version coverage across application tables)`

**Verdict:** PASS

## OBJECTIVE

Finish the §25 data contract that PHASE 07 left on only a small subset of the
schema. Every application table now has the same nullable deletion tombstone
and an integer `row_version` with a default of one. The existing domain
`version` columns remain untouched because they carry content-version numbers
or consent-policy versions, not row concurrency state. A small retention store
provides the safe write path: deletion is reversible, retries are idempotent,
and every delete or restore advances the row version.

## INPUTS

- `docs/07-DATABASE-FOUNDATION.md`: G-21 and G-22, the original §25 audit.
- `server/internal/db/migrations/0001_baseline.sql` through `0014_trial_lifecycle.sql`:
  the 66-table, 77-FK schema.
- `server/internal/store/sessions.go`: the existing session-specific tombstone
  implementation.
- `server/internal/deletion`: account erasure, which remains a separate legal
  hard-erasure/anonymisation policy rather than being silently changed into a
  reversible account deletion.

## IMPLEMENTATION

### Migration 0015

`0015_retention_versioning.sql` adds, with `IF NOT EXISTS`, to all 66
application tables:

```text
deleted_at  TEXT NULL
row_version INTEGER NOT NULL DEFAULT 1
```

The statements are explicit rather than hidden in a dynamic PL/pgSQL block so
the repository's `InitSchema` compatibility bootstrap and the transactional
migration runner execute the same SQL. No table or foreign key is added; the
live schema remains **66 tables / 77 foreign keys**.

The universal column is named `row_version` to avoid corrupting existing
meanings of `version`: confession and user-confession `version` values describe
content revisions, `content_versions.version_number` describes the immutable
content snapshot, and `consent_records.version` identifies a policy revision.
`row_version` is the concurrency/retention version for every row.

### Retention store

`internal/retention` validates table identifiers before interpolation and keeps
all row values parameterised. `SoftDelete`:

1. writes the first UTC tombstone only when `deleted_at IS NULL`;
2. increments `row_version` in the same UPDATE; and
3. returns `ErrNotFound` for a missing or already deleted row.

`Restore` clears the tombstone and increments the same counter. Both operations
are retry-safe. The account deletion policy is deliberately not routed through
this generic reversible API: legal erasure still erases or anonymises the exact
personal rows prescribed by `internal/deletion`.

Primary content and audio reads now hide tombstoned collections, categories,
confessions, variants, scriptures, voices, and audio assets. Existing session
reads already excluded deleted sessions. This prevents a soft-deleted row from
being accidentally offered merely because a caller forgot a status predicate.

## TESTING

Named tests:

- `TestEveryApplicationTableHasRetentionAndVersionColumns` queries
  `information_schema` and proves all 66 application tables have both columns.
- `TestRetentionColumnsHaveSafeDefaults` proves `deleted_at` is nullable and
  `row_version` has a default on the installed PostgreSQL schema.
- `TestSoftDeleteAndRestoreAdvanceRowVersion` proves the tombstone, idempotent
  retry, restore, and versions 1→2→3 on a real category row.
- `TestRetentionRejectsInjectedTableNames` proves the generic table surface
  rejects an identifier injection rather than interpolating it.

The phase proving commands are:

```sh
source /tmp/toolchain/env.sh
export TEST_DATABASE_URL='host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable'
cd server
export GOFLAGS=-modfile=/tmp/local.mod
go test ./internal/db ./internal/retention ./internal/store -count=1
gofmt -l internal cmd
go vet ./...
```

They passed against PostgreSQL 17. The full contract checks remain:

```sh
EXPORT_ROUTES=1 EXPORT_ROUTES_PATH=/home/user/i-confess/design/routes.json \
  go test ./internal/api -run '^TestExportRouteTable$' -count=1
go run ./cmd/genspec /home/user/i-confess/contracts/openapi.json
cd ..
python3 design/test_ia.py
python3 scripts/check_dart_symbols.py
```

The route and client contracts are unchanged: **306 route entries / 240
OpenAPI paths / 306 operations**, and **85/85 Dart symbols**. The migration
changes no table or foreign-key count.

## SECURITY AND DATA REVIEW

- Soft-deleted rows are not absent from PostgreSQL; administrative recovery and
  retention jobs can still inspect them. Public/content reads use an explicit
  `deleted_at IS NULL` predicate.
- The table identifier allowlist blocks SQL injection in the generic API.
- `row_version` changes in the same statement as the tombstone, so a consumer
  cannot observe a deletion without a new version.
- A repeated delete does not move the first-deletion timestamp, preventing a
  retry from extending a retention window silently.
- Account erasure remains the stronger legal operation and is not weakened by
  adding a reversible retention primitive.

## EXIT CRITERIA

- [x] All 66 application tables have `deleted_at`; proving command:
      `go test ./internal/db -run '^TestEveryApplicationTableHasRetentionAndVersionColumns$'`.
- [x] All 66 application tables have `row_version`; proving command: the same
      live information-schema test plus `TestRetentionColumnsHaveSafeDefaults`.
- [x] Soft-delete and restore advance the version and are retry-safe; proving
      command: `go test ./internal/retention -run 'TestSoftDeleteAndRestoreAdvanceRowVersion|TestRetentionRejectsInjectedTableNames'`.
- [x] Primary content/audio reads hide tombstones; proving command:
      `go test ./internal/store -count=1`.
- [x] Counts are reconciled at 66 tables / 77 foreign keys; proving command:
      `go test ./internal/db -run '^TestPostgresSchemaLoads$'`.

## VERDICT

**PASS — `G-21` and `G-22` are closed by PHASE 38.**
