# PHASE 07 — Database Foundation: PostgreSQL

**Status: IN PROGRESS.** Step 1 (canonical schema) is complete and verified.
Steps 2–5 are not started. This document is the design record the phase gate
requires, not a completion claim.

Decision on record: one dialect everywhere, **including tests**. SQLite is being
removed, not kept as a convenience fallback. A second dialect is a second set of
behaviour to reason about, and the divergence always surfaces in production
rather than in CI.

---

## 1. Inspection (measured, not assumed)

| Fact | Value | Source |
|---|---|---|
| SQLite DDL | 1,140 lines, 61 tables, 620 columns | `internal/db/schema.sql` |
| Driver | `modernc.org/sqlite v1.57.0` (pure Go) | `server/go.mod` |
| `sql.Open` call sites | 3 (`internal/db/db.go`, 2 in tests) | grep |
| `?` placeholders | 663 across 14 store files | grep |
| `$N` placeholders | 0 | grep |
| Pre-existing Postgres migrations | 4 files, ~317 lines — partial, and `0008`–`0010` are TEXT-id "SQLite compatible", not Postgres-native | `migrations/postgres/` |
| Connection pooling | `SetMaxOpenConns(1)` — SQLite write serialisation | `db.go:57` |

The previous Postgres migrations covered roughly half the tables and were not
loadable as a set. They are superseded by this phase rather than extended.

## 2. Incompatibilities actually found

Loading `schema.sql` into PostgreSQL 17 produces 34 errors. 31 are cascade
failures; there are **three root causes**:

1. **`PRAGMA foreign_keys = ON`** (line 5) — SQLite-only, does not parse.
   PostgreSQL enforces foreign keys unconditionally, so nothing replaces it.
2. **Eight forward foreign-key references.** The schema references `users(id)`
   from line 298 while creating `users` at line 427. SQLite does not resolve
   forward references at `CREATE` time; PostgreSQL rejects them. This is why
   `users` "did not exist" while 54 other tables were created successfully — the
   symptom was misleading and the cause is ordering, not a missing table.
3. **`INSERT OR IGNORE`** (lines 981, 1049) — SQLite-only upsert spelling.

Also present: six `datetime('now')` calls in seed rows (→ `now()`).

Checked and found **not** to be problems:

- `STRICT` — every occurrence is `ON DELETE RESTRICT`, which PostgreSQL accepts.
- `?` inside SQL string literals — none. A literal-aware placeholder rebind is
  therefore safe, but is being written literal-aware regardless.
- Table-level `FOREIGN KEY (...)` clauses — none; all 73 are inline.

## 3. Decisions

**Foreign keys move to `ALTER TABLE ... ADD CONSTRAINT` after all tables.**
This resolves the ordering problem permanently instead of depending on someone
keeping 1,140 lines of DDL in dependency order. It also matches how the schema
already extends tables via a trailing `ALTER` block.

**Boolean-shaped `INTEGER NOT NULL DEFAULT 0` columns stay `INTEGER`.** There
are 44. PostgreSQL accepts them, and converting them to `BOOLEAN` changes what
every Go `Scan` expects — a blast radius that belongs in its own change, not
folded silently into a dialect migration. Tracked as a follow-up.

**The Postgres schema is generated once and committed as source of truth.** A
runtime transformer would keep two dialects alive in perpetuity, which is the
thing this phase removes.

## 4. Step 1 result — canonical schema (DONE, verified)

`internal/db/schema.postgres.sql` — 61 tables, 620 columns, 70 foreign keys
declared via `ALTER TABLE` plus 3 that ride inline on `ADD COLUMN` (valid in
PostgreSQL), 164 indexes.

Verified against a live PostgreSQL 17.11:

```
psql -v ON_ERROR_STOP=1 -f internal/db/schema.postgres.sql   → exit 0, no errors
tables: 61   indexes: 164   foreign keys: 73   seeded feature_flags: 6
```

Column parity was checked independently — Python's `sqlite3` parsed the SQLite
schema, `information_schema` was queried on the Postgres side:

```
sqlite tables: 61   postgres tables: 61
column counts identical for every table
total columns: sqlite=620 postgres=620
```

The FK count reconciles exactly: 73 inline `REFERENCES` in SQLite = 70 moved to
`ALTER TABLE` + 3 left inline on `ADD COLUMN`, and the live server reports 73
constraints.

## 5. Remaining steps

**Step 2 — placeholder rebind.** 663 `?` → `$N`. Approach: wrap `*sql.DB` in a
type whose `QueryContext`/`ExecContext`/`QueryRowContext` rebind, and switch
store fields to it. A single choke point beats editing 663 call sites, and the
scanner must skip string literals so a `?` inside a quoted value is never
renumbered.

**Step 3 — remove SQLite.** Drop `modernc.org/sqlite`, delete `db.Open`'s
file/WAL/`busy_timeout` path and `SetMaxOpenConns(1)`. PostgreSQL wants a real
pool; a cap of 1 would serialise the whole service behind one connection.

**Step 4 — tests on Postgres.** The two `sql.Open("sqlite", ":memory:")` sites
become a helper that connects to `TEST_DATABASE_URL` and isolates each test.
Isolation strategy is undecided between per-test schema and truncate-between-
tests; with 398 tests, creating a database per test is too slow to be the
default.

**Step 5 — CI.** Add a `services: postgres:17` container to `.github/workflows/
ci.yml` and export `TEST_DATABASE_URL`. Until this exists the suite cannot run
in CI, so steps 2–4 must not land without it.

## 6. Environment

PostgreSQL 17.11 (Debian) installed and running at `127.0.0.1:5432`; role
`iconfess`, database `iconfess_test`. Note for future sessions: root **is**
available in this sandbox — an earlier note claiming otherwise was wrong and
cost time.
