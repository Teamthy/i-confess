package seed

import (
	"context"
	"testing"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/store"
)

// TestEnsureContentPopulatesAnEmptyProductionDatabase is the test for the
// defect this phase found: the catalogue used to be created only by Seed,
// which does not run in production, so a deployed server came up with zero
// categories and zero confessions.
func TestEnsureContentPopulatesAnEmptyProductionDatabase(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := context.Background()

	cats, confs, err := EnsureContent(ctx, conn)
	if err != nil {
		t.Fatalf("EnsureContent: %v", err)
	}

	if cats != len(CanonicalCategories) {
		t.Errorf("created %d categories, want %d", cats, len(CanonicalCategories))
	}
	if confs != len(CanonicalConfessions) {
		t.Errorf("created %d confessions, want %d", confs, len(CanonicalConfessions))
	}

	content := store.NewContentStore(conn)

	got, err := content.ListCategories(ctx, true)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(got) != len(CanonicalCategories) {
		t.Errorf("database holds %d categories, want %d", len(got), len(CanonicalCategories))
	}

	// Every category has to actually have confessions under it, not merely
	// exist as a row. An empty category renders as an empty screen.
	empty := 0
	for _, c := range got {
		list, err := content.ConfessionsByCategory(ctx, c.ID, true)
		if err != nil {
			t.Fatalf("ConfessionsByCategory(%s): %v", c.Slug, err)
		}
		if len(list) < 2 {
			empty++
			t.Errorf("category %q has %d published confessions, want >= 2", c.Slug, len(list))
		}
	}
	if empty > 0 {
		t.Errorf("%d categories are thin or empty", empty)
	}
}

// TestEnsureContentIsIdempotent covers the reason this can run on every boot.
// CreateCategory is a plain INSERT, so a second unguarded pass would either
// duplicate rows or fail on the unique slug index.
func TestEnsureContentIsIdempotent(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := context.Background()

	if _, _, err := EnsureContent(ctx, conn); err != nil {
		t.Fatalf("first pass: %v", err)
	}

	cats, confs, err := EnsureContent(ctx, conn)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if cats != 0 || confs != 0 {
		t.Errorf("second pass created %d categories and %d confessions, want 0 and 0", cats, confs)
	}

	content := store.NewContentStore(conn)
	got, err := content.ListCategories(ctx, true)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(got) != len(CanonicalCategories) {
		t.Errorf("after two passes the database holds %d categories, want %d", len(got), len(CanonicalCategories))
	}

	confs2, err := content.ListConfessions(ctx, false)
	if err != nil {
		t.Fatalf("ListConfessions: %v", err)
	}
	if len(confs2) != len(CanonicalConfessions) {
		t.Errorf("after two passes the database holds %d confessions, want %d", len(confs2), len(CanonicalConfessions))
	}
}

// TestEnsureCanonicalAudioCoversAllCanonicalConfessions closes G-34. The
// production EnsureContent path creates the catalogue in every environment;
// this second idempotent step guarantees every one of the 78 canonical rows
// has four object-backed duration assets and a content-version link.
func TestEnsureCanonicalAudioCoversAllCanonicalConfessions(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := context.Background()

	if _, _, err := EnsureContent(ctx, conn); err != nil {
		t.Fatalf("EnsureContent: %v", err)
	}
	objects, err := storage.New(&storage.StorageConfig{
		Provider: "local", LocalRootPath: t.TempDir(), SigningSecret: "canonical-audio-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := EnsureCanonicalAudio(ctx, conn, objects)
	if err != nil {
		t.Fatalf("EnsureCanonicalAudio: %v", err)
	}
	wantAssets := len(CanonicalConfessions) * 4
	if created != wantAssets {
		t.Errorf("created %d canonical assets, want %d", created, wantAssets)
	}

	var confessions, assets int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT content_id), COUNT(*) FROM audio_assets
		 WHERE audio_source='bootstrap_fixture' AND status IN ('ready','published')`).Scan(&confessions, &assets); err != nil {
		t.Fatal(err)
	}
	if confessions != len(CanonicalConfessions) || assets != wantAssets {
		t.Errorf("audio coverage = %d confessions/%d assets, want %d/%d", confessions, assets, len(CanonicalConfessions), wantAssets)
	}

	if again, err := EnsureCanonicalAudio(ctx, conn, objects); err != nil {
		t.Fatalf("second EnsureCanonicalAudio: %v", err)
	} else if again != 0 {
		t.Errorf("second audio ensure created %d assets, want 0", again)
	}

	rows, err := conn.QueryContext(ctx, `SELECT storage_key FROM audio_assets WHERE audio_source='bootstrap_fixture'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		exists, err := objects.Exists(ctx, key)
		if err != nil {
			t.Fatalf("object %q: %v", key, err)
		}
		if !exists {
			t.Errorf("database asset points at missing object %q", key)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

// TestEnsureContentCreatesNoAudio asserts the deliberate boundary. Seed writes
// placeholder tone bytes and marks the assets "ready"; in production that
// would serve users beeps labelled as confessions. Audio has to come from the
// generation pipeline behind the rights gate.
func TestEnsureContentCreatesNoAudio(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := context.Background()

	if _, _, err := EnsureContent(ctx, conn); err != nil {
		t.Fatalf("EnsureContent: %v", err)
	}

	raw := dbtest.Raw(t)
	var n int
	if err := raw.QueryRowContext(ctx, `SELECT COUNT(*) FROM audio_assets`).Scan(&n); err != nil {
		t.Fatalf("count audio_assets: %v", err)
	}
	if n != 0 {
		t.Errorf("EnsureContent created %d audio assets, want 0 - audio must come from the generation pipeline", n)
	}
}

// TestEnsureContentPublishesEveryDurationRung checks the variant ladder the
// session engine depends on. A confession missing a rung silently under-fills
// a queue built for that duration.
func TestEnsureContentPublishesEveryDurationRung(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := context.Background()

	if _, _, err := EnsureContent(ctx, conn); err != nil {
		t.Fatalf("EnsureContent: %v", err)
	}

	content := store.NewContentStore(conn)
	all, err := content.ListConfessions(ctx, false)
	if err != nil {
		t.Fatalf("ListConfessions: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("no confessions to check")
	}

	for _, c := range all {
		v, err := content.Variants(ctx, c.ID)
		if err != nil {
			t.Fatalf("Variants(%s): %v", c.Title, err)
		}
		if len(v) != 4 {
			t.Errorf("%q has %d duration variants, want 4", c.Title, len(v))
		}
		sc, err := content.Scriptures(ctx, c.ID)
		if err != nil {
			t.Fatalf("Scriptures(%s): %v", c.Title, err)
		}
		if len(sc) == 0 {
			t.Errorf("%q was stored with no scripture reference", c.Title)
		}
	}
}
