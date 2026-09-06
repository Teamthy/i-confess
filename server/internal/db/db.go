package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// SchemaPostgresSQL is the full schema as one string: every migration,
// concatenated in order. It exists for the callers that parse the schema rather
// than query it - coverage tests that extract CREATE TABLE statements, and the
// deletion test that walks every table. They read the same bytes the database
// was built from, so they cannot pass against a schema that no longer exists.
var SchemaPostgresSQL string

func init() {
	s, err := SchemaSQL()
	if err != nil {
		panic("db: cannot assemble schema from migrations: " + err.Error())
	}
	SchemaPostgresSQL = s
}

// InitSchema executes a schema script statement by statement.
//
// It fails on the first error rather than continuing. A partially applied
// schema is worse than no schema: the process would start and then fail on the
// first query that touched a missing column, which is much harder to diagnose
// than a startup failure that names the statement.
//
// Statements are split on ';' after stripping full-line comments. That is
// sufficient for the committed schema, which contains no semicolons inside
// string literals and no dollar-quoted function bodies. If either is ever
// added, this needs a real SQL scanner — see the note in schema.postgres.sql.
func InitSchema(conn schemaExecutor, schema string) error {
	var lines []string
	for _, line := range strings.Split(schema, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		lines = append(lines, line)
	}
	clean := strings.Join(lines, "\n")

	for _, stmt := range strings.Split(clean, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := conn.Exec(stmt); err != nil {
			return fmt.Errorf("init schema: %w (stmt: %.60s...)", err, stmt)
		}
	}
	return nil
}

// Open connects to PostgreSQL and applies the canonical schema.
//
// dsn is a lib/pq connection string, either keyword form
// ("host=... port=... user=... password=... dbname=... sslmode=require") or a
// postgres:// URL.
//
// The returned *DB rebinds '?' placeholders to PostgreSQL's '$N' form, which is
// what lets the store layer keep the SQL it was written with.
func Open(dsn string) (*DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("open db: empty connection string")
	}

	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// PostgreSQL handles concurrent connections; capping the pool at one, as the
	// SQLite build had to, would serialise the entire service behind a single
	// connection and turn every request queue into a throughput cliff.
	conn.SetMaxOpenConns(envInt("DB_MAX_OPEN_CONNS", 25))
	conn.SetMaxIdleConns(envInt("DB_MAX_IDLE_CONNS", 5))
	conn.SetConnMaxLifetime(envDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute))
	conn.SetConnMaxIdleTime(envDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	if err := Migrate(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return NewDB(conn), nil
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}
