package store

import (
	"context"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/models"
)

// The directive is explicit about this in §9: "Once a session is created, its
// queue should become deterministic. Do not let later content changes
// unexpectedly mutate an already-created session. Use SESSION SNAPSHOTS."
//
// It was violated. duration_seconds and audio_asset_id were snapshotted, but
// title, category name and text were read live through a JOIN to confessions,
// so an editor fixing a typo on Monday night silently changed what a user heard
// from the session they had already built and scheduled for Tuesday morning.
// Repetition is the product's premise, so the words a user committed to have to
// be the words they receive.
//
// This test is the reason the snapshot columns exist. Without it the invariant
// is a comment, and comments do not fail a build.

func seedConfession(t *testing.T, d *db.DB, catID, confID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := d.Exec(
		`INSERT INTO users (id,email,password_hash,created_at,updated_at) VALUES ($1,$2,$3,$4,$5)`,
		"user-1", "snapshot@example.com", "x", now, now); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO categories (id,name,slug,description,status,sort_order,created_at,updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		catID, "Healing", "healing", "Wholeness.", "published", 1, now, now); err != nil {
		t.Fatalf("insert category: %v", err)
	}
	if _, err := d.Exec(
		`INSERT INTO confessions (id,category_id,title,short_text,medium_text,long_text,intensity,language,status,author,created_at,updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		confID, catID, "I Am Healed", "By His stripes.",
		"By the stripes of Jesus I am healed.",
		"By the stripes of Jesus I am healed in every cell.",
		3, "en", "published", "content team", now, now); err != nil {
		t.Fatalf("insert confession: %v", err)
	}
}

func TestQueueIsASnapshotAndDoesNotMutateWithContent(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	seedConfession(t, d, "cat-1", "conf-1")

	sessions := NewSessionStore(d)
	now := time.Now().UTC().Format(time.RFC3339)
	sess := &models.Session{
		ID: "sess-1", UserID: "user-1", Type: "USER_CREATED",
		DurationSeconds: 60, Strategy: "BALANCED", Status: "READY", CreatedAt: now,
		Items: []models.SessionItem{{
			ConfessionID: "conf-1", Position: 1, DurationSeconds: 60, Status: "queued",
		}},
	}
	if err := sessions.Create(ctx, sess); err != nil {
		t.Fatalf("Create session: %v", err)
	}

	before, err := sessions.Items(ctx, "sess-1")
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("queue has %d item(s), want 1", len(before))
	}
	if before[0].Title != "I Am Healed" || before[0].Text != "By the stripes of Jesus I am healed." {
		t.Fatalf("snapshot not taken at build time: got title %q text %q", before[0].Title, before[0].Text)
	}
	if before[0].Category != "Healing" {
		t.Fatalf("snapshot stored category %q, want the name \"Healing\"", before[0].Category)
	}

	// An editor rewrites the confession and renames the category. This is the
	// change that used to leak into already-built sessions.
	if _, err := d.Exec(
		`UPDATE confessions SET title=$1, short_text=$2, medium_text=$3, long_text=$4 WHERE id=$5`,
		"REWRITTEN TITLE", "New short.", "Completely different words.", "Different long text.", "conf-1"); err != nil {
		t.Fatalf("update confession: %v", err)
	}
	if _, err := d.Exec(`UPDATE categories SET name=$1 WHERE id=$2`, "Renamed Category", "cat-1"); err != nil {
		t.Fatalf("update category: %v", err)
	}

	after, err := sessions.Items(ctx, "sess-1")
	if err != nil {
		t.Fatalf("Items after edit: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("queue has %d item(s) after edit, want 1", len(after))
	}

	if after[0].Title != before[0].Title {
		t.Errorf("queue title changed from %q to %q - the session is not a snapshot", before[0].Title, after[0].Title)
	}
	if after[0].Text != before[0].Text {
		t.Errorf("queue text changed from %q to %q - the session is not a snapshot", before[0].Text, after[0].Text)
	}
	if after[0].Category != before[0].Category {
		t.Errorf("queue category changed from %q to %q - the session is not a snapshot", before[0].Category, after[0].Category)
	}
	if after[0].DurationSeconds != before[0].DurationSeconds {
		t.Errorf("queue duration changed from %d to %d", before[0].DurationSeconds, after[0].DurationSeconds)
	}
}

// TestNewSessionsSeeCurrentContent is the other half of the contract. A
// snapshot must not become a cache: a session built *after* an edit has to see
// the edited content, otherwise fixing a typo would never reach anyone.
func TestNewSessionsSeeCurrentContent(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	seedConfession(t, d, "cat-1", "conf-1")

	sessions := NewSessionStore(d)
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := d.Exec(`UPDATE confessions SET title=$1, medium_text=$2 WHERE id=$3`,
		"Corrected Title", "Corrected words.", "conf-1"); err != nil {
		t.Fatalf("update confession: %v", err)
	}

	sess := &models.Session{
		ID: "sess-2", UserID: "user-1", Type: "USER_CREATED",
		DurationSeconds: 60, Strategy: "BALANCED", Status: "READY", CreatedAt: now,
		Items: []models.SessionItem{{ConfessionID: "conf-1", Position: 1, DurationSeconds: 60, Status: "queued"}},
	}
	if err := sessions.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	items, err := sessions.Items(ctx, "sess-2")
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if items[0].Title != "Corrected Title" || items[0].Text != "Corrected words." {
		t.Errorf("a session built after the edit saw title %q text %q; the snapshot is behaving like a stale cache",
			items[0].Title, items[0].Text)
	}
}

// TestSessionCreationFailsOnUnknownConfession covers the resolver's error path.
// A queue pointing at a confession that does not exist must fail at build time
// rather than persist an item with an empty title that renders as a blank card
// during playback.
func TestSessionCreationFailsOnUnknownConfession(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	sessions := NewSessionStore(d)
	now := time.Now().UTC().Format(time.RFC3339)

	sess := &models.Session{
		ID: "sess-3", UserID: "user-1", Type: "USER_CREATED",
		DurationSeconds: 60, Strategy: "BALANCED", Status: "READY", CreatedAt: now,
		Items: []models.SessionItem{{ConfessionID: "does-not-exist", Position: 1, DurationSeconds: 60, Status: "queued"}},
	}
	if err := sessions.Create(ctx, sess); err == nil {
		t.Fatal("Create succeeded with an item referencing a nonexistent confession")
	}

	var n int
	if err := d.QueryRow(`SELECT count(*) FROM sessions WHERE id=$1`, "sess-3").Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if n != 0 {
		t.Errorf("the session was persisted despite the failure - the transaction did not roll back")
	}
}
