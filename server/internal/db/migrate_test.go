package db_test

import (
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// TestMigrationsAreOrderedAndImmutable checks the properties that make a
// migration history safe to replay: strictly increasing versions, no duplicate
// version numbers, and content that produces a stable checksum.
func TestMigrationsAreOrderedAndImmutable(t *testing.T) {
	ms, err := db.Migrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(ms) == 0 {
		t.Fatal("no migrations - the binary would start with an empty schema")
	}

	seen := map[string]string{}
	var prev string
	for _, m := range ms {
		if m.Version <= prev {
			t.Errorf("migration %s sorts at or before %s; versions must strictly increase", m.Filename, prev)
		}
		prev = m.Version

		if other, dup := seen[m.Version]; dup {
			t.Errorf("version %s is used by both %s and %s", m.Version, other, m.Filename)
		}
		seen[m.Version] = m.Filename

		if len(m.Checksum) != 64 {
			t.Errorf("%s: checksum %q is not a sha256 hex digest", m.Filename, m.Checksum)
		}
		if m.SQL == "" {
			t.Errorf("%s is empty", m.Filename)
		}
	}
	t.Logf("%d migrations, %s .. %s", len(ms), ms[0].Version, ms[len(ms)-1].Version)
}

// TestSchemaSQLIncludesEveryMigration guards the callers that parse the schema
// as text rather than query it. They read db.SchemaPostgresSQL; if that ever
// stopped tracking the migrations, the coverage tests would pass against a
// schema the database was never built from.
func TestSchemaSQLIncludesEveryMigration(t *testing.T) {
	ms, err := db.Migrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	assembled, err := db.SchemaSQL()
	if err != nil {
		t.Fatalf("assemble schema: %v", err)
	}
	if assembled != db.SchemaPostgresSQL {
		t.Error("db.SchemaPostgresSQL does not match SchemaSQL() - the two views of the schema have diverged")
	}
	for _, m := range ms {
		if !strings.Contains(assembled, m.Filename) {
			t.Errorf("assembled schema does not include %s", m.Filename)
		}
	}
}

// TestLedgerRecordsEveryAppliedMigration proves the runner actually records
// what it ran. Without the ledger, every boot would replay every migration, and
// a migration that is not idempotent would fail on the second start.
func TestLedgerRecordsEveryAppliedMigration(t *testing.T) {
	raw := dbtest.Raw(t) // dbtest builds the schema through Migrate

	ms, err := db.Migrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}

	rows, err := raw.Query(`SELECT version, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	defer func() { _ = rows.Close() }()

	recorded := map[string]string{}
	for rows.Next() {
		var v, c string
		if err := rows.Scan(&v, &c); err != nil {
			t.Fatalf("scan: %v", err)
		}
		recorded[v] = c
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate: %v", err)
	}

	if len(recorded) != len(ms) {
		t.Errorf("ledger has %d rows but there are %d migrations", len(recorded), len(ms))
	}
	for _, m := range ms {
		got, ok := recorded[m.Version]
		if !ok {
			t.Errorf("migration %s (%s) was applied but is not in the ledger", m.Version, m.Filename)
			continue
		}
		if got != m.Checksum {
			t.Errorf("ledger checksum for %s is %s, binary has %s", m.Version, got[:12], m.Checksum[:12])
		}
	}
}

// TestMigrateIsIdempotent is the property that lets the process boot twice.
func TestMigrateIsIdempotent(t *testing.T) {
	raw := dbtest.Raw(t)
	if err := db.Migrate(raw); err != nil {
		t.Fatalf("second Migrate failed: %v", err)
	}
	if err := db.Migrate(raw); err != nil {
		t.Fatalf("third Migrate failed: %v", err)
	}

	var n int
	if err := raw.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("count ledger: %v", err)
	}
	ms, err := db.Migrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if n != len(ms) {
		t.Errorf("ledger grew to %d rows after re-running %d migrations", n, len(ms))
	}
}

// TestTamperedLedgerIsRejected covers the failure the checksum exists for: two
// environments that believe they are on the same version but are not.
func TestTamperedLedgerIsRejected(t *testing.T) {
	raw := dbtest.Raw(t)

	ms, err := db.Migrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	last := ms[len(ms)-1]

	if _, err := raw.Exec(`UPDATE schema_migrations SET checksum = $1 WHERE version = $2`,
		strings.Repeat("0", 64), last.Version); err != nil {
		t.Fatalf("tamper with ledger: %v", err)
	}

	err = db.Migrate(raw)
	if err == nil {
		t.Fatal("Migrate accepted a ledger whose checksum does not match the binary; " +
			"an edited migration would go undetected")
	}
	if !strings.Contains(err.Error(), "edited after it was applied") {
		t.Errorf("error does not explain the problem: %v", err)
	}
}
