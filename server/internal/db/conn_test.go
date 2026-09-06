package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// The tests in this file run against a real PostgreSQL server. They are the
// only evidence that Rebind produces SQL PostgreSQL actually accepts: the unit
// tests in rebind_test.go check the string transformation, and a transformation
// can be internally consistent and still be wrong in a way only the server can
// detect.
//
// TEST_DATABASE_URL must point at a server the caller may create and drop
// databases on. When it is unset the tests skip rather than pass, so a green
// suite never silently means "PostgreSQL was never exercised".

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set - PostgreSQL integration test skipped")
	}
	return url
}

// newTestDB creates a throwaway database with the canonical schema loaded, and
// registers cleanup. A fresh database per test is slower than truncating shared
// tables, but it makes the tests independent and means a failure in one cannot
// be caused by residue from another.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	admin, err := sql.Open("postgres", testDatabaseURL(t))
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	defer admin.Close()

	name := fmt.Sprintf("iconfess_dbtest_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		_ = admin.Close()
		a2, err := sql.Open("postgres", testDatabaseURL(t))
		if err == nil {
			_, _ = a2.Exec("DROP DATABASE IF EXISTS " + name)
			_ = a2.Close()
		}
	})

	raw, err := sql.Open("postgres", replaceDatabaseName(testDatabaseURL(t), name))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })

	if err := raw.PingContext(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	if err := InitSchema(raw, SchemaPostgresSQL); err != nil {
		t.Fatalf("load postgres schema: %v", err)
	}
	return NewDB(raw)
}

// replaceDatabaseName swaps the dbname in a lib/pq connection string.
func replaceDatabaseName(url, name string) string {
	if strings.Contains(url, "dbname=") {
		parts := strings.Fields(url)
		for i, p := range parts {
			if strings.HasPrefix(p, "dbname=") {
				parts[i] = "dbname=" + name
			}
		}
		return strings.Join(parts, " ")
	}
	return url + " dbname=" + name
}

// TestPostgresSchemaLoads is the phase gate for the canonical schema: 61
// tables, every foreign key resolvable, seed rows applied.
func TestPostgresSchemaLoads(t *testing.T) {
	ctx := context.Background()
	d := newTestDB(t)

	var tables int
	if err := d.QueryRowContext(ctx,
		`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&tables); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tables != 61 {
		t.Errorf("expected 61 tables, got %d", tables)
	}

	var fks int
	if err := d.QueryRowContext(ctx,
		`SELECT count(*) FROM information_schema.table_constraints
		 WHERE constraint_type = 'FOREIGN KEY' AND table_schema = 'public'`).Scan(&fks); err != nil {
		t.Fatalf("count foreign keys: %v", err)
	}
	if fks != 73 {
		t.Errorf("expected 73 foreign keys, got %d", fks)
	}

	var flags int
	if err := d.QueryRowContext(ctx, `SELECT count(*) FROM feature_flags`).Scan(&flags); err != nil {
		t.Fatalf("count feature_flags: %v", err)
	}
	if flags == 0 {
		t.Error("feature_flags seed rows did not load")
	}
}

// TestRebindRoundTrip proves the placeholders survive a real round trip: write
// with '?' parameters, read them back, and confirm the values landed in the
// right columns.
func TestRebindRoundTrip(t *testing.T) {
	ctx := context.Background()
	d := newTestDB(t)

	const id = "rebind-test-user"
	if _, err := d.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, "rebind@example.com", "hash", "2026-09-06T00:00:00Z", "2026-09-06T00:00:00Z"); err != nil {
		t.Fatalf("insert with ? placeholders: %v", err)
	}

	var email, hash string
	if err := d.QueryRowContext(ctx,
		`SELECT email, password_hash FROM users WHERE id = ?`, id).Scan(&email, &hash); err != nil {
		t.Fatalf("select with ? placeholder: %v", err)
	}
	if email != "rebind@example.com" || hash != "hash" {
		t.Errorf("round trip mismatch: email=%q hash=%q", email, hash)
	}

	// A QueryContext with several parameters, to confirm ordering rather than
	// just presence.
	rows, err := d.QueryContext(ctx,
		`SELECT id FROM users WHERE email = ? AND password_hash = ?`,
		"rebind@example.com", "hash")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var got string
	if !rows.Next() {
		t.Fatal("expected one row")
	}
	if err := rows.Scan(&got); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got != id {
		t.Errorf("got id %q, want %q", got, id)
	}
}

// TestRebindWithLiteralQuestionMark is the end-to-end form of the property
// TestRebindDoesNotShiftNumbering checks on the string: a '?' inside a quoted
// value must not steal a parameter number. Against a real server the failure
// mode is a bind error or wrong data, so this is worth verifying for real.
func TestRebindWithLiteralQuestionMark(t *testing.T) {
	ctx := context.Background()
	d := newTestDB(t)

	const id = "literal-test-user"
	if _, err := d.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, created_at, updated_at)
		 VALUES (?, 'who?', ?, ?, ?)`,
		id, "hash", "2026-09-06T00:00:00Z", "2026-09-06T00:00:00Z"); err != nil {
		t.Fatalf("insert with literal '?': %v", err)
	}

	var email string
	if err := d.QueryRowContext(ctx,
		`SELECT email FROM users WHERE id = ? AND password_hash = ?`, id, "hash").Scan(&email); err != nil {
		t.Fatalf("select: %v", err)
	}
	if email != "who?" {
		t.Errorf("literal '?' was not preserved: got %q", email)
	}
}

// TestRebindInTransaction covers the Tx wrapper. A transaction that did not
// rebind would fail on its first parameter, and because the non-transactional
// paths all work, that failure is easy to miss in review.
func TestRebindInTransaction(t *testing.T) {
	ctx := context.Background()
	d := newTestDB(t)

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"tx-user", "tx@example.com", "hash", "2026-09-06T00:00:00Z", "2026-09-06T00:00:00Z"); err != nil {
		_ = tx.Rollback()
		t.Fatalf("tx exec with ? placeholders: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var email string
	if err := d.QueryRowContext(ctx, `SELECT email FROM users WHERE id = ?`, "tx-user").Scan(&email); err != nil {
		t.Fatalf("read back after commit: %v", err)
	}
	if email != "tx@example.com" {
		t.Errorf("got %q, want tx@example.com", email)
	}
}

// TestForeignKeyIsEnforced confirms the ALTER TABLE constraints are live, not
// merely declared. SQLite silently accepts orphan rows when foreign_keys is off;
// the schema's whole point is that PostgreSQL does not.
func TestForeignKeyIsEnforced(t *testing.T) {
	ctx := context.Background()
	d := newTestDB(t)

	_, err := d.ExecContext(ctx,
		`INSERT INTO session_progress (session_id, user_id, position_ms, completed_items, last_updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"no-such-session", "no-such-user", 0, 0, "2026-09-06T00:00:00Z")
	if err == nil {
		t.Fatal("expected a foreign key violation, insert succeeded")
	}
	if !strings.Contains(err.Error(), "foreign key") {
		t.Errorf("expected a foreign key error, got: %v", err)
	}
}
