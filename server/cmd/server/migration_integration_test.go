package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/lib/pq"
)

// TestSeparateMigrationServerStartup launches the real server against an
// isolated database. TEST_DATABASE_URL must allow CREATE DATABASE and ROLE.
// Both application roles are unprivileged logins, not the test administrator.
func TestSeparateMigrationServerStartup(t *testing.T) {
	dsn := dbtest.URL(t)
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	suffix := fmt.Sprintf("%d_%d", os.Getpid(), time.Now().UnixNano())
	database, migrator, role := "startup_db_"+suffix, "migrator_"+suffix, "runtime_"+suffix
	password := "test_" + suffix
	for _, name := range []string{migrator, role} {
		if _, err := admin.Exec("CREATE ROLE " + name + " LOGIN NOINHERIT PASSWORD '" + password + "'"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = admin.Exec("DROP ROLE " + name) })
	}
	if _, err := admin.Exec("CREATE DATABASE " + database); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE " + database) })
	setup, err := sql.Open("postgres", testDSN(dsn, database, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer setup.Close()
	for _, stmt := range []string{
		"REVOKE CREATE ON SCHEMA public FROM PUBLIC",
		"GRANT USAGE, CREATE ON SCHEMA public TO " + migrator,
		"GRANT USAGE ON SCHEMA public TO " + role,
	} {
		if _, err := setup.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	migratorDSN := testDSN(dsn, database, migrator, password)
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
	grants, err := os.ReadFile("../../ops/runtime-smoke-grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(strings.ReplaceAll(string(grants), `:"runtime_role"`, pq.QuoteIdentifier(role))); err != nil {
		t.Fatalf("grant smoke-test access: %v", err)
	}
	assertRestrictedRole(t, runtimeDSN, setup, migrator, role)
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
		"ENV=test", "SEED=0", "DATABASE_URL="+runtimeDSN, "DB_SCHEMA_MODE=verify", "MIGRATION_DATABASE_URL=",
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
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
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
		response, err := client.Get(baseURL + "/health/ready")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	assertAccountSmoke(t, baseURL, owner)
}

func assertRestrictedRole(t *testing.T, runtimeDSN string, setup *sql.DB, migrator, role string) {
	t.Helper()
	restricted, err := sql.Open("postgres", runtimeDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer restricted.Close()
	// A background rights poll can fail without making the HTTP probe red.
	if _, err := store.NewVoicePlatformStore(db.NewDB(restricted)).VoicesDueForExpiry(context.Background()); err != nil {
		t.Fatalf("initial voice rights poll with smoke grants: %v", err)
	}
	for _, stmt := range []string{
		"CREATE TABLE public.forbidden (id int)",
		"DELETE FROM public.schema_migrations WHERE false",
		"UPDATE public.schema_migrations SET checksum='forbidden' WHERE false",
		"ALTER TABLE public.users ADD COLUMN forbidden int",
	} {
		var pgErr *pq.Error
		if _, err := restricted.Exec(stmt); !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("expected insufficient_privilege for %q, got %v", stmt, err)
		}
	}
	// A column-only grant is invisible to has_table_privilege. And NOINHERIT
	// does not make membership of the migration owner safe for a runtime login.
	for _, tc := range []struct{ grant, revoke string }{
		{"GRANT UPDATE(checksum) ON public.schema_migrations TO " + role, "REVOKE UPDATE(checksum) ON public.schema_migrations FROM " + role},
		{"GRANT " + migrator + " TO " + role, "REVOKE " + migrator + " FROM " + role},
	} {
		if _, err := setup.Exec(tc.grant); err != nil {
			t.Fatal(err)
		}
		conn, err := db.OpenWithSchemaMode(runtimeDSN, "verify")
		if err == nil {
			_ = conn.Close()
			t.Fatal("verify accepted an over-privileged runtime login")
		}
		if !strings.Contains(err.Error(), "restricted runtime role") {
			t.Fatalf("unexpected verify failure: %v", err)
		}
		if _, err := setup.Exec(tc.revoke); err != nil {
			t.Fatal(err)
		}
	}
}

func assertAccountSmoke(t *testing.T, baseURL string, owner *sql.DB) {
	t.Helper()
	// A green probe is insufficient: actually write an account, validate its
	// session on a second request, and read back the same user.
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Post(baseURL+"/auth/register", "application/json",
		strings.NewReader(`{"email":"role-test@example.test","password":"Test-Only-Signup-4850"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var signup struct{ Token string }
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&signup) != nil || signup.Token == "" {
		t.Fatalf("registration with runtime role failed (HTTP %d)", response.StatusCode)
	}
	req, err := http.NewRequest(http.MethodGet, baseURL+"/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+signup.Token)
	me, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer me.Body.Close()
	var account struct{ User struct{ Email string } }
	if me.StatusCode != http.StatusOK || json.NewDecoder(me.Body).Decode(&account) != nil || account.User.Email != "role-test@example.test" {
		t.Fatalf("authenticated read with runtime role failed (HTTP %d)", me.StatusCode)
	}
	// Registration has some best-effort inserts. A 200 alone would mask a
	// missing grant and leave a partially initialized account.
	var initialized bool
	if err := owner.QueryRow(`SELECT EXISTS (
		SELECT 1 FROM users u JOIN subscriptions s ON s.user_id = u.id
		JOIN session_preferences p ON p.user_id = u.id
		JOIN email_verification_tokens v ON v.user_id = u.id
		WHERE u.email = 'role-test@example.test' AND s.plan = 'free'
	)`).Scan(&initialized); err != nil || !initialized {
		t.Fatalf("account defaults or verification token missing: %v", err)
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
