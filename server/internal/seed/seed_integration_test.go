package seed

import (
	"context"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/storage"
)

// nopStorage satisfies storage.ObjectStorage without touching a real bucket.
// Seed only calls Upload, but the interface is satisfied whole so the test
// breaks loudly if Seed starts relying on another method.
type nopStorage struct{ uploads int }

var _ storage.ObjectStorage = (*nopStorage)(nil)

func (n *nopStorage) Upload(context.Context, string, []byte, map[string]string) error {
	n.uploads++
	return nil
}
func (n *nopStorage) Download(context.Context, string) ([]byte, error) { return nil, nil }
func (n *nopStorage) Delete(context.Context, string) error             { return nil }
func (n *nopStorage) GenerateSignedURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (n *nopStorage) List(context.Context, string) ([]string, error) { return nil, nil }
func (n *nopStorage) Exists(context.Context, string) (bool, error)   { return false, nil }
func (n *nopStorage) GetSize(context.Context, string) (int64, error) { return 0, nil }
func (n *nopStorage) GetMetadata(context.Context, string) (map[string]string, error) {
	return nil, nil
}

// TestSeedInstallsEveryCanonicalCategory runs the real Seed against PostgreSQL.
//
// This is the test that makes D3 ("all 39 exist at launch") enforceable rather
// than aspirational. It exercises the actual seed path, so a category dropped
// from the loop, a name mismatch between the confession seeds and the canonical
// list, or a foreign key that rejects an empty category id all fail here rather
// than at first boot.
func TestSeedInstallsEveryCanonicalCategory(t *testing.T) {
	d := dbtest.New(t)
	store := &nopStorage{}

	if err := Seed(d, store); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var got int
	if err := d.QueryRow(`SELECT count(*) FROM categories`).Scan(&got); err != nil {
		t.Fatalf("count categories: %v", err)
	}
	if got != CanonicalCategoryCount {
		t.Errorf("seed installed %d categories, want %d", got, CanonicalCategoryCount)
	}

	// Every canonical slug must be present, not merely the right count - a
	// duplicated slug and a missing one would otherwise cancel out.
	for _, want := range CategorySlugsInCanonicalOrder() {
		var n int
		if err := d.QueryRow(`SELECT count(*) FROM categories WHERE slug=$1`, want).Scan(&n); err != nil {
			t.Fatalf("look up %s: %v", want, err)
		}
		if n != 1 {
			t.Errorf("category slug %q appears %d time(s), want exactly 1", want, n)
		}
	}

	if store.uploads == 0 {
		t.Error("Seed uploaded no audio; the placeholder-audio path did not run")
	}
}

// TestSeedConfessionsAllResolveToACategory is the guard for the D2 rename. Two
// confession seeds referenced "Strength" and "Thanksgiving", which are not
// canonical names; after the rename the lookup map no longer holds them, and an
// unresolved name would produce an empty category id.
func TestSeedConfessionsAllResolveToACategory(t *testing.T) {
	d := dbtest.New(t)
	if err := Seed(d, &nopStorage{}); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var orphaned int
	if err := d.QueryRow(`
		SELECT count(*) FROM confessions c
		LEFT JOIN categories k ON k.id = c.category_id
		WHERE c.category_id = '' OR k.id IS NULL`).Scan(&orphaned); err != nil {
		t.Fatalf("count orphaned confessions: %v", err)
	}
	if orphaned != 0 {
		t.Errorf("%d confession(s) do not resolve to a category - a seed references a name that is not canonical", orphaned)
	}

	var confessions int
	if err := d.QueryRow(`SELECT count(*) FROM confessions`).Scan(&confessions); err != nil {
		t.Fatalf("count confessions: %v", err)
	}
	if confessions == 0 {
		t.Error("Seed created no confessions")
	}
	t.Logf("seed installed %d confessions across %d categories", confessions, CanonicalCategoryCount)
}
