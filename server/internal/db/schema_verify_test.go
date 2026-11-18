package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpenWithSchemaModeRejectsInvalidMode(t *testing.T) {
	if _, err := OpenWithSchemaMode("unused", "typo"); err == nil || !strings.Contains(err.Error(), "DB_SCHEMA_MODE") {
		t.Fatalf("invalid mode: %v", err)
	}
}

// Exercises the read-only startup check with a role that cannot create objects
// or change the ledger. TEST_DATABASE_URL must have CREATE ROLE privileges.
func TestVerifyMigrationsWithRestrictedRole(t *testing.T) {
	conn, err := sql.Open("postgres", testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)
	suffix := fmt.Sprintf("%d_%d", os.Getpid(), time.Now().UnixNano())
	schema, role := "verify_schema_"+suffix, "verify_role_"+suffix
	for _, stmt := range []string{
		"CREATE ROLE " + role + " NOLOGIN",
		"CREATE SCHEMA " + schema,
		"GRANT USAGE ON SCHEMA " + schema + " TO " + role,
		"SET search_path TO " + schema,
	} {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		_, _ = conn.Exec("RESET ROLE")
		_, _ = conn.Exec("DROP SCHEMA " + schema + " CASCADE")
		_, _ = conn.Exec("DROP ROLE " + role)
	}()
	if _, err := conn.Exec("SET ROLE " + role); err != nil {
		t.Fatal(err)
	}
	if err := VerifyMigrations(conn); err == nil {
		t.Fatal("missing ledger accepted")
	}
	if _, err := conn.Exec("CREATE TABLE should_not_exist (id int)"); err == nil {
		t.Fatal("restricted role can create tables")
	}
	if _, err := conn.Exec("RESET ROLE"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("CREATE TABLE schema_migrations (version text PRIMARY KEY, filename text NOT NULL, checksum text NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("GRANT SELECT ON schema_migrations TO " + role); err != nil {
		t.Fatal(err)
	}
	ms, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if _, err := conn.Exec("INSERT INTO schema_migrations VALUES ($1,$2,$3)", m.Version, m.Filename, m.Checksum); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := conn.Exec("SET ROLE " + role); err != nil {
		t.Fatal(err)
	}
	if err := VerifyMigrations(conn); err != nil {
		t.Fatalf("verify restricted role: %v", err)
	}
	if _, err := conn.Exec("DELETE FROM schema_migrations"); err == nil {
		t.Fatal("restricted role can modify ledger")
	}
	if _, err := conn.Exec("RESET ROLE"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("DELETE FROM schema_migrations WHERE version=$1", ms[0].Version); err != nil {
		t.Fatal(err)
	}
	if err := VerifyMigrations(conn); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing migration: %v", err)
	}
	if _, err := conn.Exec("INSERT INTO schema_migrations VALUES ($1,$2,$3)", ms[0].Version, ms[0].Filename, "wrong"); err != nil {
		t.Fatal(err)
	}
	if err := VerifyMigrations(conn); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("checksum: %v", err)
	}
	if _, err := conn.Exec("UPDATE schema_migrations SET checksum=$1 WHERE version=$2", ms[0].Checksum, ms[0].Version); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("INSERT INTO schema_migrations VALUES ('future', 'future.sql', 'future')"); err != nil {
		t.Fatal(err)
	}
	if err := VerifyMigrations(conn); err != nil {
		t.Fatalf("newer ledger: %v", err)
	}
}

func TestSeparateMigrationAndVerifyStartup(t *testing.T) {
	dsn := testDatabaseURL(t)
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := fmt.Sprintf("verify_db_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec("DROP DATABASE " + name) }()
	databaseURL := replaceDatabaseName(dsn, name)
	if conn, err := OpenWithSchemaMode(databaseURL, "verify"); err == nil {
		_ = conn.Close()
		t.Fatal("verify started on an unmigrated database")
	}
	migration, err := OpenWithSchemaMode(databaseURL, "migrate")
	if err != nil {
		t.Fatal(err)
	}
	if err := migration.Close(); err != nil {
		t.Fatal(err)
	}
	runtime, err := OpenWithSchemaMode(databaseURL, "verify")
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationLockSerializesAndReleases(t *testing.T) {
	dsn := testDatabaseURL(t)
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := fmt.Sprintf("lock_db_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec("DROP DATABASE " + name) }()
	databaseURL := replaceDatabaseName(dsn, name)
	pool, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	pool.SetMaxOpenConns(1) // migrations must not require a second pool connection
	holder, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	holder.SetMaxOpenConns(1)
	if _, err := holder.Exec("SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- Migrate(pool) }()
	select {
	case err := <-result:
		t.Fatalf("migration did not wait for the held lock: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	if _, err := holder.Exec("SELECT pg_advisory_unlock($1)", migrationLockKey); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("migration after lock release: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("migration did not resume after lock release")
	}
	ms, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec("UPDATE schema_migrations SET checksum='bad' WHERE version=$1", ms[0].Version); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(pool); err == nil || !strings.Contains(err.Error(), "edited") {
		t.Fatalf("corrupt ledger: %v", err)
	}
	var obtained bool
	if err := holder.QueryRow("SELECT pg_try_advisory_lock($1)", migrationLockKey).Scan(&obtained); err != nil || !obtained {
		t.Fatalf("lock leaked after migration error: obtained=%v err=%v", obtained, err)
	}
	if _, err := holder.Exec("SELECT pg_advisory_unlock($1)", migrationLockKey); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentFreshMigrations(t *testing.T) {
	dsn := testDatabaseURL(t)
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := fmt.Sprintf("concurrent_db_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec("DROP DATABASE " + name) }()
	pool, err := sql.Open("postgres", replaceDatabaseName(dsn, name))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			results <- Migrate(pool)
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatalf("concurrent migration: %v", err)
		}
	}
	if err := VerifyMigrations(pool); err != nil {
		t.Fatalf("verify concurrent migrations: %v", err)
	}
}
