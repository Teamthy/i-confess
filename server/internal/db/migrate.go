package db

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrations live in internal/db/migrations and are embedded in the binary.
//
// There is exactly one source of truth for the schema. The repository
// previously had two: internal/db/schema.postgres.sql, which the application
// actually applied, and migrations/postgres/, which nothing applied and which
// had drifted to 25 tables against the live 64. Applying that directory would
// have produced a database missing 39 tables, including refresh_tokens,
// mfa_secrets, voice_rights and idempotency_keys. It is deleted rather than
// kept "for reference", because a plausible-looking migration directory that
// builds the wrong schema is worse than none.
//
// Migrations are forward-only. A released migration is never edited: the
// checksum below detects it, and the fix for a bad migration is a new one.

const ledgerTable = "schema_migrations"

// Migration is one numbered, immutable schema change.
type Migration struct {
	Version  string
	Filename string
	SQL      string
	Checksum string
}

// Migrations returns every embedded migration in version order.
func Migrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	out := make([]Migration, 0, len(names))
	for _, name := range names {
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		version := strings.SplitN(name, "_", 2)[0]
		if version == "" {
			return nil, fmt.Errorf("migration %s has no version prefix; expected NNNN_description.sql", name)
		}
		out = append(out, Migration{
			Version:  version,
			Filename: name,
			SQL:      string(body),
			Checksum: fmt.Sprintf("%x", sha256.Sum256(body)),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no migrations found - the binary would start with an empty schema")
	}
	return out, nil
}

// SchemaSQL concatenates every migration. Callers that need the schema as one
// string - coverage tests that parse CREATE TABLE statements - use this, so
// they read the same bytes the database was built from.
func SchemaSQL() (string, error) {
	ms, err := Migrations()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, m := range ms {
		b.WriteString("-- " + m.Filename + "\n")
		b.WriteString(m.SQL)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// Migrate applies every migration that has not run yet, in order.
//
// Each migration runs in its own transaction: a migration that fails halfway
// rolls back and leaves the ledger untouched, so the next run retries it rather
// than skipping past a half-applied change. The ledger records the checksum,
// and a migration whose contents changed after being applied is a hard error -
// editing a released migration is how two environments end up with schemas
// that no test can reconcile.
func Migrate(conn *sql.DB) error {
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS ` + ledgerTable + ` (
		version    TEXT PRIMARY KEY,
		filename   TEXT NOT NULL,
		checksum   TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}

	applied, err := readLedger(conn)
	if err != nil {
		return err
	}

	ms, err := Migrations()
	if err != nil {
		return err
	}

	for _, m := range ms {
		if want, ok := applied[m.Version]; ok {
			if want != m.Checksum {
				return fmt.Errorf(
					"migration %s (%s) has been edited after it was applied: "+
						"checksum %s in the ledger, %s in the binary. "+
						"Migrations are immutable once released - add a new migration instead",
					m.Version, m.Filename, want[:12], m.Checksum[:12])
			}
			continue
		}

		if err := applyOne(conn, m); err != nil {
			return fmt.Errorf("migration %s (%s): %w", m.Version, m.Filename, err)
		}
	}
	return nil
}

// readLedger returns version -> checksum for every migration already applied.
func readLedger(conn *sql.DB) (map[string]string, error) {
	rows, err := conn.Query(`SELECT version, checksum FROM ` + ledgerTable)
	if err != nil {
		return nil, fmt.Errorf("read migration ledger: %w", err)
	}
	defer func() { _ = rows.Close() }()

	applied := map[string]string{}
	for rows.Next() {
		var v, c string
		if err := rows.Scan(&v, &c); err != nil {
			return nil, fmt.Errorf("scan ledger row: %w", err)
		}
		applied[v] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ledger: %w", err)
	}
	return applied, nil
}

func applyOne(conn *sql.DB, m Migration) error {
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := InitSchema(tx, m.SQL); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO `+ledgerTable+` (version, filename, checksum) VALUES ($1, $2, $3)`,
		m.Version, m.Filename, m.Checksum); err != nil {
		return fmt.Errorf("record in ledger: %w", err)
	}
	return tx.Commit()
}

// schemaExecutor is the subset of *sql.DB that InitSchema needs, so a
// migration can run inside a transaction.
type schemaExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
}
