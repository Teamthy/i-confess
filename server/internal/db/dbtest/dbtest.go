// Package dbtest gives tests a real PostgreSQL database.
//
// The project runs one dialect everywhere, tests included. A test suite that
// runs against SQLite while production runs PostgreSQL verifies the code
// against a database that will never serve a request: the two disagree on
// placeholder syntax, foreign key enforcement by default, type coercion and
// upsert spelling, and every one of those differences is a place a test can
// pass and production can fail.
//
// TEST_DATABASE_URL must point at a server the caller may create and drop
// databases on, in lib/pq keyword form, for example:
//
//	host=127.0.0.1 port=5432 user=iconfess password=iconfess dbname=postgres sslmode=disable
//
// When it is unset, New fails the test rather than skipping it. Skipping would
// let a green suite mean "PostgreSQL was never exercised", which is exactly the
// false confidence this package exists to remove. CI sets the variable, so a
// missing local server is a setup problem to fix, not a condition to hide.
package dbtest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
	_ "github.com/lib/pq"
)

var (
	adminOnce sync.Once
	adminDB   *sql.DB
	adminErr  error
	adminURL  string

	// templateName holds a database preloaded with the schema. Cloning from a
	// template is a filesystem-level copy in PostgreSQL, so giving every test
	// its own isolated database costs roughly what a truncate would.
	templateOnce sync.Once
	templateName string
	templateErr  error

	counter int64
	mu      sync.Mutex
)

// URL returns the configured test server, or fails the test.
func URL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("TEST_DATABASE_URL is not set. Tests run against PostgreSQL; start a server " +
			"and export e.g. TEST_DATABASE_URL=\"host=127.0.0.1 port=5432 user=iconfess " +
			"password=iconfess dbname=postgres sslmode=disable\"")
	}
	return url
}

func admin(t *testing.T) *sql.DB {
	t.Helper()
	adminOnce.Do(func() {
		adminURL = URL(t)
		adminDB, adminErr = sql.Open("postgres", adminURL)
		if adminErr != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		adminErr = adminDB.PingContext(ctx)
	})
	if adminErr != nil {
		t.Fatalf("connect to test PostgreSQL: %v", adminErr)
	}
	return adminDB
}

// template builds (once per process) a database holding the canonical schema.
func template(t *testing.T) string {
	t.Helper()
	templateOnce.Do(func() {
		a := admin(t)
		name := fmt.Sprintf("iconfess_tpl_%d", os.Getpid())
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		_, _ = a.ExecContext(ctx, "DROP DATABASE IF EXISTS "+name)
		if _, err := a.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
			templateErr = fmt.Errorf("create template database: %w", err)
			return
		}
		templateName = name

		conn, err := sql.Open("postgres", withDatabase(adminURL, name))
		if err != nil {
			templateErr = err
			return
		}
		defer conn.Close()
		if err := conn.PingContext(ctx); err != nil {
			templateErr = err
			return
		}
		if err := db.InitSchema(conn, db.SchemaPostgresSQL); err != nil {
			templateErr = fmt.Errorf("load schema into template: %w", err)
			return
		}
	})
	if templateErr != nil {
		t.Fatalf("prepare template database: %v", templateErr)
	}
	return templateName
}

// New returns a DB connected to a fresh database containing the canonical
// schema and nothing else. The database is dropped when the test ends, so
// tests cannot leak state into each other.
func New(t *testing.T) *db.DB {
	t.Helper()
	a := admin(t)
	tpl := template(t)

	mu.Lock()
	counter++
	n := counter
	mu.Unlock()

	name := fmt.Sprintf("iconfess_test_%d_%d", os.Getpid(), n)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := a.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", name, tpl)); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		_, _ = a.ExecContext(cctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})

	conn, err := sql.Open("postgres", withDatabase(adminURL, name))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.PingContext(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	return db.NewDB(conn)
}

// Raw is New without the db.DB wrapper, for tests that need to assert on
// driver-level behaviour.
func Raw(t *testing.T) *sql.DB {
	t.Helper()
	d := New(t)
	return d.DB
}

// withDatabase rewrites the dbname in a lib/pq keyword connection string.
func withDatabase(connStr, name string) string {
	if strings.Contains(connStr, "dbname=") {
		fields := strings.Fields(connStr)
		for i, f := range fields {
			if strings.HasPrefix(f, "dbname=") {
				fields[i] = "dbname=" + name
			}
		}
		return strings.Join(fields, " ")
	}
	return connStr + " dbname=" + name
}
