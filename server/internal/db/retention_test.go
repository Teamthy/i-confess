package db_test

import (
	"context"
	"testing"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// TestEveryApplicationTableHasRetentionAndVersionColumns closes the broad
// §25 gap from PHASE 07. The assertion queries the installed schema rather than
// counting ALTER statements, and excludes only schema_migrations, which is the
// migration ledger rather than an application row table.
func TestEveryApplicationTableHasRetentionAndVersionColumns(t *testing.T) {
	raw := dbtest.Raw(t)
	ctx := context.Background()
	rows, err := raw.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema='public' AND table_type='BASE TABLE'
		  AND table_name <> 'schema_migrations'
		ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		count++
		var deleted, version bool
		if err := raw.QueryRowContext(ctx, `
			SELECT
				EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name='deleted_at'),
				EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name='row_version')`, table).
			Scan(&deleted, &version); err != nil {
			t.Fatal(err)
		}
		if !deleted || !version {
			t.Errorf("%s: deleted_at=%v row_version=%v, want both", table, deleted, version)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 68 {
		t.Errorf("application table count=%d, want 68", count)
	}
}

func TestRetentionColumnsHaveSafeDefaults(t *testing.T) {
	raw := dbtest.Raw(t)
	ctx := context.Background()
	var nullable string
	if err := raw.QueryRowContext(ctx, `
		SELECT is_nullable FROM information_schema.columns
		 WHERE table_schema='public' AND table_name='confessions' AND column_name='deleted_at'`).Scan(&nullable); err != nil {
		t.Fatal(err)
	}
	if nullable != "YES" {
		t.Errorf("confessions.deleted_at is_nullable=%q, want YES", nullable)
	}
	var defaultValue string
	if err := raw.QueryRowContext(ctx, `
		SELECT column_default FROM information_schema.columns
		 WHERE table_schema='public' AND table_name='confessions' AND column_name='row_version'`).Scan(&defaultValue); err != nil {
		t.Fatal(err)
	}
	if defaultValue == "" {
		t.Error("confessions.row_version has no default")
	}
}
