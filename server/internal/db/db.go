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
// Split on statement terminators only, not semicolons inside quoted values,
// identifiers, comments or PostgreSQL dollar-quoted blocks. Migration 0022
// contains punctuation inside an editorial note; changing that released SQL
// would invalidate its checksum, so the migration runner must parse it safely.
func InitSchema(conn schemaExecutor, schema string) error {
	statements, err := splitSQLStatements(schema)
	if err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	for _, stmt := range statements {
		if _, err := conn.Exec(stmt); err != nil {
			return fmt.Errorf("init schema: %w (stmt: %.60s...)", err, stmt)
		}
	}
	return nil
}

func splitSQLStatements(schema string) ([]string, error) {
	statements := []string{}
	var statement strings.Builder
	var single, double, lineComment bool
	var blockDepth int
	var dollarTag string
	for i := 0; i < len(schema); i++ {
		c := schema[i]
		if lineComment {
			if c == '\n' {
				lineComment = false
				statement.WriteByte(c)
			}
			continue
		}
		if blockDepth > 0 {
			if strings.HasPrefix(schema[i:], "/*") {
				blockDepth++
				i++
			} else if strings.HasPrefix(schema[i:], "*/") {
				blockDepth--
				i++
			}
			continue
		}
		if dollarTag != "" {
			if strings.HasPrefix(schema[i:], dollarTag) {
				statement.WriteString(dollarTag)
				i += len(dollarTag) - 1
				dollarTag = ""
			} else {
				statement.WriteByte(c)
			}
			continue
		}
		if single || double {
			statement.WriteByte(c)
			quote := byte('\'')
			if double {
				quote = '"'
			}
			if c == quote {
				if i+1 < len(schema) && schema[i+1] == quote {
					statement.WriteByte(quote)
					i++
				} else {
					single, double = false, false
				}
			}
			continue
		}
		switch {
		case strings.HasPrefix(schema[i:], "--"):
			lineComment = true
			i++
		case strings.HasPrefix(schema[i:], "/*"):
			blockDepth = 1
			i++
		case c == '\'':
			single = true
			statement.WriteByte(c)
		case c == '"':
			double = true
			statement.WriteByte(c)
		case c == '$':
			// Dollar quotes may be untagged ($$) or tagged ($body_1$).
			j := i + 1
			for j < len(schema) && (schema[j] >= 'a' && schema[j] <= 'z' || schema[j] >= 'A' && schema[j] <= 'Z' || schema[j] >= '0' && schema[j] <= '9' || schema[j] == '_') {
				j++
			}
			if j < len(schema) && schema[j] == '$' && (j == i+1 || schema[i+1] < '0' || schema[i+1] > '9') {
				dollarTag = schema[i : j+1]
				statement.WriteString(dollarTag)
				i = j
			} else {
				statement.WriteByte(c)
			}
		case c == ';':
			if stmt := strings.TrimSpace(statement.String()); stmt != "" {
				statements = append(statements, stmt)
			}
			statement.Reset()
		default:
			statement.WriteByte(c)
		}
	}
	if single || double || dollarTag != "" || blockDepth > 0 {
		return nil, fmt.Errorf("unterminated SQL quote or comment")
	}
	if stmt := strings.TrimSpace(statement.String()); stmt != "" {
		statements = append(statements, stmt)
	}
	return statements, nil
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
