# PHASE 07 — Database Foundation

**Status: PASS (9/10)**
**Date:** 2026-09-06
**Depends on:** PHASE 01 (domain model), PHASE 02 (C-5 PostgreSQL only)
**Blocks:** PHASE 08 and every phase that writes data

## Objective

Section 25 requires PostgreSQL with real relational modelling: primary keys,
foreign keys, constraints, indexes, transactions, migrations, soft delete and
versioning. The objective here was to find out which of those actually existed
and fix the ones that did not.

## What existed

| §25 requirement | State found |
|---|---|
| Primary keys | Present, UUID text |
| Foreign keys | 76 — present |
| Indexes | 73 — present |
| Transactions | Present in the store layer |
| **Migrations** | **Did not exist** |
| **Constraints** | **22 of 23 status columns unconstrained** |
| Soft delete | 2 `deleted_at` columns across 64 tables |
| Versioning | 2 `version` columns across 64 tables |

### The migration situation

Two schemas existed and they had diverged:

- `internal/db/schema.postgres.sql` — **64 tables**, applied at boot by
  `InitSchema`
- `migrations/postgres/` — **25 tables**, applied by nothing

The numbering in the second was `0001, 0008, 0009, 0010`. The gap implies
0002–0007 existed at some point and were lost. **Applying that directory would
have produced a database missing 39 tables**, including `refresh_tokens`,
`mfa_secrets`, `voice_rights` and `idempotency_keys`.

It is deleted rather than kept "for reference". A plausible-looking migration
directory that builds the wrong schema is worse than none, because someone will
eventually run it.

A correction to an earlier claim in this project's records: those files were
described as containing SQLite DDL. **They did not.** The DDL is PostgreSQL. What
was wrong with them is that they were stale and unreachable, not that they were
the wrong dialect.

### `InitSchema` could not evolve a database

The boot path ran `InitSchema`, which executes `CREATE TABLE IF NOT EXISTS`. On
an existing database that is a no-op for every table. **A column added to the
schema file never reached a database that already had the table.** Deploying a
schema change to production would have appeared to succeed and then failed on the
first query that touched the new column.

## Implementation

```
internal/db/migrations/0001_baseline.sql         the 64-table schema, moved not copied
internal/db/migrations/0002_status_constraints.sql   23 CHECK constraints
internal/db/migrate.go                           runner, ledger, checksums
internal/db/migrate_test.go                      ordering, idempotency, tamper detection
internal/db/status_constraints_test.go           coverage and vocabulary agreement
```

Migrations are embedded in the binary, so the schema travels with the code that
expects it. `Migrate` applies unapplied migrations in version order, **each in
its own transaction**, recording version, filename and SHA-256 in
`schema_migrations`. A migration that fails rolls back and leaves the ledger
untouched, so the next boot retries it rather than skipping past a half-applied
change.

Editing a released migration is a hard error. The ledger's checksum will not
match and `Migrate` refuses to start, naming both hashes. That is the guard
against the failure where two environments believe they are on the same version
and are not.

`db.SchemaPostgresSQL` is kept, but it is now assembled from the migrations
rather than read from a separate file. The tests that parse the schema as text —
coverage tests, the deletion sweep test — read the same bytes the database was
built from, so they cannot pass against a schema that no longer exists.

## Closing G-2

Twenty-three tables have a `status` column. Before this phase the permitted
values for 21 of them existed only in a trailing SQL comment, which PostgreSQL
does not enforce. A typo wrote an invalid state and nothing noticed until a query
filtered on it and quietly returned no rows.

All 23 are now constrained. Three things went wrong on the way, and each one is
worth more than the fix:

**1. I counted 22 status columns. There are 23.** The count came from scanning
`CREATE TABLE` bodies, but `user_confessions.status` is added by an `ALTER TABLE`
at line 1106 of the baseline. Counting a schema means reading all of it.

**2. `community_posts` got the wrong vocabulary.** The first draft allowed four
statuses; the package declares seven. The three I missed — `draft`, `published`,
`archived` — live in a `const` block and are passed as `$1`, so a grep for string
literals in SQL cannot see them. Two tests failed on insert and named the
constraint.

That failure became a permanent test. `TestDatabaseVocabularyMatchesGoConstants`
compares each constraint against the Go constants that define the vocabulary and
fails in both directions: a constant the database rejects, and a value the
database allows that no constant declares. Verified to bite — narrowing the
constraint produces:

```
community_posts: Go declares status "draft" but the database constraint rejects it
community_posts: Go declares status "published" but the database constraint rejects it
community_posts: Go declares status "archived" but the database constraint rejects it
```

**3. `user_confessions.status` is written by nothing.** It was added alongside
`reviewed_by`, `rejection_reason` and `reviewed_at` for a moderation lifecycle
that §10 describes and no handler implements. The constraint settles the
vocabulary before that code exists rather than after.

## Testing

| Test | Asserts |
|---|---|
| `TestEveryStatusColumnIsConstrained` | every `status` column in `information_schema` has a CHECK — counted from the database, not the file |
| `TestDatabaseVocabularyMatchesGoConstants` | constraint and Go constants agree, both directions |
| `TestMigrationsAreOrderedAndImmutable` | versions strictly increase, no duplicates, checksums are sha256, none empty |
| `TestSchemaSQLIncludesEveryMigration` | the text view of the schema still tracks the migrations |
| `TestLedgerRecordsEveryAppliedMigration` | every applied migration is in the ledger with a matching checksum |
| `TestMigrateIsIdempotent` | `Migrate` can run three times without error and without growing the ledger |
| `TestTamperedLedgerIsRejected` | a mismatched checksum stops the process and says why |

One bug found in my own test while writing it: parsing `pg_get_constraintdef`
by splitting on quote characters yields `::text,` as a value, because PostgreSQL
renders each literal as `'draft'::text`. The parser now matches the literal, and
fails loudly if it extracts zero values rather than passing vacuously.

## Security review

- **`subscriptions.status` now permits only `active`, `trial`, `expired`,
  `cancelled`.** This is deliberately narrow. `SetSubscription(ctx, userID, plan,
  status string)` accepts any string from any caller, which is the hole the
  PHASE 04 payment bypass went through. The constraint means an invalid state
  cannot be written even by a bug — but it does not fix the signature, which is
  PHASE 36's job.
- **§36's trial lifecycle is not in the vocabulary.** `ELIGIBLE`, `STARTED`,
  `EXPIRING` and `CONVERTED` do not exist as states (G-3). Adding them is a new
  migration, and that migration is the record that PHASE 37 happened.
- Migrations run at boot with the application's credentials. There is no separate
  migration role, which is acceptable while §47 keeps infrastructure simple and
  is worth revisiting before multi-instance deploys.

## Exit criteria

- [x] One schema, one source of truth
- [x] Migration runner with ledger, checksums and per-migration transactions
- [x] Idempotent — verified by running `Migrate` three times
- [x] Tamper detection — verified by corrupting a checksum
- [x] G-2 closed: 23 of 23 status columns constrained
- [x] Constraint vocabularies tied to the Go constants that define them
- [x] `make verify` green, 23 packages, 0 failures

## Carried forward

- **G-2 is closed.** The 22-vs-23 discrepancy is recorded above.
- **G-21** — soft delete exists on 2 of 64 tables. §25 asks for it generally.
  62 foreign keys use `ON DELETE CASCADE`, which is a defensible
  choice for a privacy-first product but is not what §25 says. Needs a decision,
  not a guess: either the directive is amended or 62 tables change.
- **G-22** — versioning exists on 2 of 64 tables (`confessions`,
  `user_confessions`). `content_versions` provides content versioning properly;
  the rest do not.
- **G-23** — `user_confessions` gained nine columns by `ALTER TABLE` for a
  moderation lifecycle no handler implements. Either PHASE 31 builds it or the columns come out.
- **G-24** — no migration role separation; the app migrates with its own
  credentials at boot.
