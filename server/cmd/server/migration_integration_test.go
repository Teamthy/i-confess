package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// TestSeparateMigrationServerStartup launches the real server against an
// isolated database. TEST_DATABASE_URL must allow CREATE DATABASE and ROLE.
// It exercises startup, not all runtime endpoints or production service grants.
func TestSeparateMigrationServerStartup(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set - PostgreSQL integration test skipped")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	suffix := fmt.Sprintf("%d_%d", os.Getpid(), time.Now().UnixNano())
	database, role := "startup_db_"+suffix, "startup_role_"+suffix
	password := "test_" + suffix
	if _, err := admin.Exec("CREATE ROLE " + role + " LOGIN PASSWORD '" + password + "'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP ROLE " + role) })
	if _, err := admin.Exec("CREATE DATABASE " + database); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE " + database) })
	migratorDSN := testDSN(dsn, database, "", "")
	runtimeDSN := testDSN(dsn, database, role, password)
	binary := filepath.Join(t.TempDir(), "iconfess-server")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build server: %v\n%s", err, out)
	}
	migration := exec.Command(binary, "--migrate")
	migration.Env = append(os.Environ(), "MIGRATION_DATABASE_URL="+migratorDSN)
	if out, err := migration.CombinedOutput(); err != nil {
		t.Fatalf("migration: %v\n%s", err, out)
	}
	owner, err := sql.Open("postgres", migratorDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	for _, stmt := range []string{
		"REVOKE CREATE ON SCHEMA public FROM PUBLIC",
		"GRANT USAGE ON SCHEMA public TO " + role,
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO " + role,
		"REVOKE INSERT, UPDATE, DELETE ON public.schema_migrations FROM " + role,
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO " + role,
	} {
		if _, err := owner.Exec(stmt); err != nil {
			t.Fatalf("grant runtime access: %v", err)
		}
	}
	restricted, err := sql.Open("postgres", runtimeDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer restricted.Close()
	if _, err := restricted.Exec("CREATE TABLE public.forbidden (id int)"); err == nil {
		t.Fatal("runtime role can create schema objects")
	}
	if _, err := restricted.Exec("DELETE FROM public.schema_migrations"); err == nil {
		t.Fatal("runtime role can change the migration ledger")
	}
	deniedMigration := exec.Command(binary, "--migrate")
	deniedMigration.Env = append(os.Environ(), "MIGRATION_DATABASE_URL="+runtimeDSN)
	if out, err := deniedMigration.CombinedOutput(); err == nil {
		t.Fatalf("runtime role unexpectedly ran migrations: %s", out)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	logFile, err := os.Create(filepath.Join(t.TempDir(), "server.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := exec.CommandContext(ctx, binary)
	server.Env = append(os.Environ(),
		"ENV=test", "SEED=0", "DATABASE_URL="+runtimeDSN, "DB_SCHEMA_MODE=verify",
		fmt.Sprintf("PORT=%d", port), "MEDIA_DIR="+t.TempDir(), "STORAGE_PROVIDER=local",
		"REDIS_ADDR=", "TTS_PROVIDER=", "BIBLE_PROVIDER=local")
	server.Stdout, server.Stderr = logFile, logFile
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Wait() }()
	t.Cleanup(func() { cancel(); <-done })
	client := &http.Client{Timeout: time.Second}
	deadline := time.After(30 * time.Second)
	for {
		select {
		case err := <-done:
			// Put the result back so cleanup does not block.
			done <- err
			log, _ := os.ReadFile(logFile.Name())
			t.Fatalf("server exited before readiness: %v\n%s", err, log)
		case <-deadline:
			log, _ := os.ReadFile(logFile.Name())
			t.Fatalf("server did not become ready\n%s", log)
		default:
		}
		response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health/live", port))
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func testDSN(dsn, database, user, password string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err == nil {
			u.Path = "/" + database
			if user != "" {
				u.User = url.UserPassword(user, password)
			}
			return u.String()
		}
	}
	result := dsn + " dbname=" + database
	if user != "" {
		result += " user=" + user + " password=" + password
	}
	return result
}
