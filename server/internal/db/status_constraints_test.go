package db_test

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/community"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/sessions"
)

// TestEveryStatusColumnIsConstrained closes G-2.
//
// Twenty-two tables carry a status column. For most of them the permitted
// values existed only in a trailing SQL comment, which PostgreSQL does not
// enforce: a typo wrote an invalid state and nothing noticed until a query
// filtered on it and quietly returned no rows.
func TestEveryStatusColumnIsConstrained(t *testing.T) {
	raw := dbtest.Raw(t)
	ctx := context.Background()

	// Every column literally named "status".
	rows, err := raw.QueryContext(ctx, `
		SELECT c.table_name
		FROM information_schema.columns c
		JOIN information_schema.tables t
		  ON t.table_name = c.table_name AND t.table_schema = c.table_schema
		WHERE c.table_schema = 'public'
		  AND t.table_type = 'BASE TABLE'
		  AND c.column_name = 'status'
		ORDER BY c.table_name`)
	if err != nil {
		t.Fatalf("list status columns: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate: %v", err)
	}
	if len(tables) == 0 {
		t.Fatal("no status columns found - this test would pass against an empty schema")
	}

	// Tables with a CHECK constraint that mentions "status".
	constrained := map[string]bool{}
	crows, err := raw.QueryContext(ctx, `
		SELECT con.conrelid::regclass::text
		FROM pg_constraint con
		JOIN pg_attribute att
		  ON att.attrelid = con.conrelid AND att.attnum = ANY (con.conkey)
		WHERE con.contype = 'c' AND att.attname = 'status'`)
	if err != nil {
		t.Fatalf("list check constraints: %v", err)
	}
	defer crows.Close()
	for crows.Next() {
		var name string
		if err := crows.Scan(&name); err != nil {
			t.Fatalf("scan constraint: %v", err)
		}
		constrained[strings.TrimSuffix(name, "_check")] = true
		constrained[name] = true
	}
	if err := crows.Err(); err != nil {
		t.Fatalf("iterate constraints: %v", err)
	}

	for _, tbl := range tables {
		if !constrained[tbl] && !constrained[tbl+"_status_check"] {
			t.Errorf("table %q has a status column with no CHECK constraint", tbl)
		}
	}
	t.Logf("%d status columns, all constrained", len(tables))
}

// TestDatabaseVocabularyMatchesGoConstants is the check that would have caught
// the error this migration was written with.
//
// The first draft of 0002 derived each vocabulary from string literals appearing
// in SQL. That misses every value held in a Go constant and passed as $1, which
// is how the store layer is written. The constraint for community_posts allowed
// four of the seven statuses the package declares, and two tests failed on
// insert. Comparing the constraint against the constants makes that a permanent
// failure rather than a lucky catch.
func TestDatabaseVocabularyMatchesGoConstants(t *testing.T) {
	raw := dbtest.Raw(t)
	ctx := context.Background()

	cases := []struct {
		table    string
		expected []string
	}{
		{"community_posts", []string{
			community.StatusDraft, community.StatusSubmitted, community.StatusUnderReview,
			community.StatusApproved, community.StatusRejected, community.StatusPublished,
			community.StatusArchived,
		}},
		{"sessions", []string{
			string(sessions.Draft), string(sessions.Ready), string(sessions.Scheduled),
			string(sessions.Starting), string(sessions.Active), string(sessions.Paused),
			string(sessions.Interrupted), string(sessions.Completed), string(sessions.Cancelled),
			string(sessions.Expired), string(sessions.Failed),
		}},
	}

	for _, tc := range cases {
		def, err := checkDefinition(ctx, raw, tc.table, "status")
		if err != nil {
			t.Fatalf("%s: %v", tc.table, err)
		}
		if def == "" {
			t.Errorf("%s: no CHECK constraint on status", tc.table)
			continue
		}

		// pg_get_constraintdef renders each value as 'draft'::text, so a naive
		// split on the quote character yields "::text," as a value.
		got := map[string]bool{}
		for _, m := range valueRe.FindAllStringSubmatch(def, -1) {
			got[m[1]] = true
		}
		if len(got) == 0 {
			t.Errorf("%s: parsed no values out of constraint %q", tc.table, def)
			continue
		}

		want := map[string]bool{}
		for _, v := range tc.expected {
			want[v] = true
			if !got[v] {
				t.Errorf("%s: Go declares status %q but the database constraint rejects it", tc.table, v)
			}
		}
		for v := range got {
			if !want[v] {
				t.Errorf("%s: database allows status %q which no Go constant declares", tc.table, v)
			}
		}
	}
}

var valueRe = regexp.MustCompile(`'([^']*)'(?:::\w+)?`)

func checkDefinition(ctx context.Context, raw *sql.DB, table, column string) (string, error) {
	var def sql.NullString
	err := raw.QueryRowContext(ctx, `
		SELECT pg_get_constraintdef(con.oid)
		FROM pg_constraint con
		JOIN pg_attribute att
		  ON att.attrelid = con.conrelid AND att.attnum = ANY (con.conkey)
		WHERE con.conrelid = $1::regclass AND con.contype = 'c' AND att.attname = $2
		LIMIT 1`, table, column).Scan(&def)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return def.String, nil
}
