package retention

import (
	"context"
	"errors"
	"testing"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

func TestSoftDeleteAndRestoreAdvanceRowVersion(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := context.Background()

	if _, err := conn.ExecContext(ctx,
		`INSERT INTO categories (id,name,slug,status,sort_order,created_at,updated_at)
		 VALUES ($1,'Retention','retention','published',1,'2026-09-21','2026-09-21')`,
		"retention-category"); err != nil {
		t.Fatal(err)
	}

	s := NewStore(conn)
	if err := s.SoftDelete(ctx, "categories", "retention-category"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	var deletedAt string
	var version int
	if err := conn.QueryRowContext(ctx,
		`SELECT COALESCE(deleted_at,''), row_version FROM categories WHERE id=$1`,
		"retention-category").Scan(&deletedAt, &version); err != nil {
		t.Fatal(err)
	}
	if deletedAt == "" || version != 2 {
		t.Fatalf("tombstone/version = %q/%d, want non-empty/2", deletedAt, version)
	}
	if err := s.SoftDelete(ctx, "categories", "retention-category"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second SoftDelete = %v, want ErrNotFound", err)
	}

	if err := s.Restore(ctx, "categories", "retention-category"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if err := conn.QueryRowContext(ctx,
		`SELECT COALESCE(deleted_at,''), row_version FROM categories WHERE id=$1`,
		"retention-category").Scan(&deletedAt, &version); err != nil {
		t.Fatal(err)
	}
	if deletedAt != "" || version != 3 {
		t.Errorf("restored tombstone/version = %q/%d, want empty/3", deletedAt, version)
	}
}

func TestRetentionRejectsInjectedTableNames(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()

	if err := NewStore(conn).SoftDelete(context.Background(), "categories; DROP TABLE users", "x"); err == nil {
		t.Fatal("injected table name was accepted")
	}
}
