package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
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

// migrationLockKey is a stable PostgreSQL advisory-lock key for this runner.
// Keep it unchanged across releases. The lock is database-scoped and requires
// a direct or session-pooled connection, never transaction pooling.
const migrationLockKey int64 = 0x4943464d494752

// Migrate serializes runners and applies each pending migration in its own
// transaction. Applied checksums are immutable; failures leave the current
// migration unrecorded and release the lock.
func Migrate(db *sql.DB) (result error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migration connection: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
		// Acquisition may have raced cancellation; discard the session rather
		// than risk returning a lock-bearing connection to the pool.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	cancel()
	defer func() {
		unlockCtx, unlockCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer unlockCancel()
		var unlocked bool
		err := conn.QueryRowContext(unlockCtx, `SELECT pg_advisory_unlock($1)`, migrationLockKey).Scan(&unlocked)
		if err == nil && !unlocked {
			err = errors.New("migration lock was not held by this session")
		}
		if err != nil {
			// Never return a possibly locked connection to the pool.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			result = errors.Join(result, fmt.Errorf("release migration lock: %w", err))
		}
	}()
	return migrateLocked(conn)
}

func migrateLocked(conn *sql.Conn) error {
	if _, err := conn.ExecContext(context.Background(), `CREATE TABLE IF NOT EXISTS `+ledgerTable+` (
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
					m.Version, m.Filename, want, m.Checksum)
			}
			continue
		}

		if err := applyOne(conn, m); err != nil {
			return fmt.Errorf("migration %s (%s): %w", m.Version, m.Filename, err)
		}
	}
	return nil
}

// VerifyMigrations checks the embedded migration ledger without making schema
// changes. Extra applied versions are allowed for rolling deployments; that
// does not guarantee backwards compatibility of a newer schema.
func VerifyMigrations(conn *sql.DB) error {
	applied, err := readLedger(conn)
	if err != nil {
		return fmt.Errorf("verify migrations (run the migration job first): %w", err)
	}
	ms, err := Migrations()
	if err != nil {
		return err
	}
	for _, m := range ms {
		checksum, ok := applied[m.Version]
		if !ok {
			return fmt.Errorf("verify migrations: missing %s (%s); run the migration job first", m.Version, m.Filename)
		}
		if checksum != m.Checksum {
			return fmt.Errorf("verify migrations: checksum mismatch for %s (%s)", m.Version, m.Filename)
		}
	}
	return nil
}

// readLedger returns version -> checksum for every migration already applied.
type ledgerReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readLedger(conn ledgerReader) (map[string]string, error) {
	rows, err := conn.QueryContext(context.Background(), `SELECT version, checksum FROM `+ledgerTable)
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

func applyOne(conn *sql.Conn, m Migration) error {
	tx, err := conn.BeginTx(context.Background(), nil)
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
