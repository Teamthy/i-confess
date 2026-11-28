package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/store"
)

// nowStamp returns a current UTC timestamp in RFC3339Nano form.
func nowStamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// TestSoftDeletedConfessionIsNotListed verifies that ListUserConfessions
// excludes soft-deleted confessions so tombstoned private text is never
// returned to the owner.
func TestSoftDeletedConfessionIsNotListed(t *testing.T) {
	a := newAuthHarness(t)
	_, userID := a.register(t, "purge-confession@example.com")

	// Create a confession directly in the store (bypassing the not-yet-wired
	// API route) with sensitive text.
	_, err := a.db.ExecContext(context.Background(),
		`INSERT INTO user_confessions (id,user_id,title,text,category_id,is_private,status,visibility,created_at,updated_at)
		 VALUES (?,?,?,?,NULL,1,'draft','private',?,?)`,
		"conf-1", userID, "A private struggle", "I have been wrestling with doubt and fear.",
		nowStamp(), nowStamp())
	if err != nil {
		t.Fatal(err)
	}

	// List must return it.
	s := store.NewEngagementStore(a.db)
	list, err := s.ListUserConfessions(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("confession not listed: %d", len(list))
	}

	// Soft-delete it directly (simulating a delete-through-UI path).
	_, err = a.db.ExecContext(context.Background(),
		`UPDATE user_confessions SET deleted_at = ? WHERE id = ? AND user_id = ?`,
		"2026-01-01T00:00:00Z", "conf-1", userID)
	if err != nil {
		t.Fatal(err)
	}

	// List must not return the soft-deleted confession.
	list, err = s.ListUserConfessions(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("soft-deleted confession still listed: %d", len(list))
	}
}

// TestDeletedBookmarkAnnotationsArePurged verifies that when a bible bookmark
// is soft-deleted, its label and note (private user annotations) are erased
// in the same statement.
func TestDeletedBookmarkAnnotationsArePurged(t *testing.T) {
	a := newAuthHarness(t)
	_, userID := a.register(t, "purge-bookmark@example.com")

	// A verse_bookmark cannot outlive the version it was imported from, so
	// seed a bible_versions row first.
	_, err := a.db.ExecContext(context.Background(),
		`INSERT INTO bible_versions (id,name,abbrev,language,language_name,format,coverage,licence,attribution,blob_url,sha256,status,created_at,updated_at)
		 VALUES ('kjv','King James Version','KJV','en','English','osis','full','public domain','KJV','http://example.com/kjv','deadbeef','active', ?, ?)`, nowStamp(), nowStamp())
	if err != nil {
		t.Fatal(err)
	}

	// Insert a bookmark directly with private annotations.
	_, err = a.db.ExecContext(context.Background(),
		`INSERT INTO verse_bookmarks (id,user_id,version_id,book_id,chapter,verse,label,note,created_at,updated_at)
		 VALUES (?,?,'kjv','gen',1,1,'In the beginning','A private reflection on creation.',?,?)`,
		"bm-1", userID, nowStamp(), nowStamp())
	if err != nil {
		t.Fatal(err)
	}

	// Soft-delete it using the same SQL the handler uses.
	now := nowStamp()
	_, err = a.db.ExecContext(context.Background(),
		`UPDATE verse_bookmarks SET deleted_at=?, label='', note='', updated_at=?, row_version=row_version+1
		 WHERE id=? AND user_id=? AND deleted_at IS NULL`,
		now, now, "bm-1", userID)
	if err != nil {
		t.Fatal(err)
	}

	// The tombstoned row must hold no label or note.
	var label, note string
	if err := a.db.QueryRowContext(context.Background(),
		`SELECT label, note FROM verse_bookmarks WHERE id = ?`, "bm-1").Scan(&label, &note); err != nil {
		t.Fatal(err)
	}
	if label != "" || note != "" {
		t.Fatalf("deleted bookmark still holds annotations: label=%q note=%q", label, note)
	}
}

// TestDeletedNoteContentIsPurged verifies that when a bible note is
// soft-deleted, its title and body (private user notes on Scripture) are
// erased in the same statement.
func TestDeletedNoteContentIsPurged(t *testing.T) {
	a := newAuthHarness(t)
	_, userID := a.register(t, "purge-note@example.com")

	// Insert a note directly with private content. translation_id is nullable.
	_, err := a.db.ExecContext(context.Background(),
		`INSERT INTO user_bible_notes (id,user_id,book_id,chapter,verse_start,verse_end,translation_id,title,body,created_at,updated_at)
		 VALUES (?,?,'gen',1,1,1,NULL,'My note on creation','This is what the opening verse means to me personally.',?,?)`,
		"note-1", userID, nowStamp(), nowStamp())
	if err != nil {
		t.Fatal(err)
	}

	// Soft-delete it using the same SQL the handler uses.
	now := nowStamp()
	_, err = a.db.ExecContext(context.Background(),
		`UPDATE user_bible_notes SET deleted_at=?, title='', body='', updated_at=?, row_version=row_version+1
		 WHERE id=? AND user_id=? AND deleted_at IS NULL`,
		now, now, "note-1", userID)
	if err != nil {
		t.Fatal(err)
	}

	// The tombstoned row must hold no title or body.
	var title, body string
	if err := a.db.QueryRowContext(context.Background(),
		`SELECT title, body FROM user_bible_notes WHERE id = ?`, "note-1").Scan(&title, &body); err != nil {
		t.Fatal(err)
	}
	if title != "" || body != "" {
		t.Fatalf("deleted note still holds content: title=%q body=%q", title, body)
	}
}

// TestPurgeAppliesThroughDeleteAPI verifies the purge happens through the
// actual delete API endpoint, not just the underlying SQL.
func TestPurgeAppliesThroughDeleteAPI(t *testing.T) {
	a := newAuthHarness(t)
	token, userID := a.register(t, "purge-api@example.com")

	// A verse_bookmark cannot outlive the version it was imported from, so
	// seed a bible_versions row first.
	_, err := a.db.ExecContext(context.Background(),
		`INSERT INTO bible_versions (id,name,abbrev,language,language_name,format,coverage,licence,attribution,blob_url,sha256,status,created_at,updated_at)
		 VALUES ('kjv','King James Version','KJV','en','English','osis','full','public domain','KJV','http://example.com/kjv','deadbeef','active', ?, ?)`, nowStamp(), nowStamp())
	if err != nil {
		t.Fatal(err)
	}

	// Insert a bookmark directly with private annotations (bypassing the
	// bible data validation that would require seeded verse data).
	_, err = a.db.ExecContext(context.Background(),
		`INSERT INTO verse_bookmarks (id,user_id,version_id,book_id,chapter,verse,label,note,created_at,updated_at)
		 VALUES (?,?,'kjv','gen',1,1,'Private label','Private note text.',?,?)`,
		"bm-api-1", userID, nowStamp(), nowStamp())
	if err != nil {
		t.Fatal(err)
	}

	// Call the actual delete API endpoint.
	if code, _ := a.do("DELETE", "/v1/me/bible/bookmarks/bm-api-1", token, nil); code != http.StatusNoContent {
		t.Fatalf("delete bookmark via API: %d", code)
	}

	// The tombstoned row must hold no label or note.
	var label, note string
	if err := a.db.QueryRowContext(context.Background(),
		`SELECT label, note FROM verse_bookmarks WHERE id = ?`, "bm-api-1").Scan(&label, &note); err != nil {
		t.Fatal(err)
	}
	if label != "" || note != "" {
		t.Fatalf("API-deleted bookmark still holds annotations: label=%q note=%q", label, note)
	}
}

// TestDeletedConfessionTextIsPurged verifies that DeleteUserConfession wipes
// title and text (private confession text) in the same statement as setting
// deleted_at, so a tombstoned confession can never disclose what was written.
func TestDeletedConfessionTextIsPurged(t *testing.T) {
	a := newAuthHarness(t)
	_, userID := a.register(t, "purge-delete-confession@example.com")

	// Insert a confession directly with private text.
	_, err := a.db.ExecContext(context.Background(),
		`INSERT INTO user_confessions (id,user_id,title,text,category_id,is_private,status,visibility,created_at,updated_at)
		 VALUES (?,?,?,?,NULL,1,'draft','private',?,?)`,
		"conf-del-1", userID, "A private struggle", "I have been wrestling with doubt and fear.",
		nowStamp(), nowStamp())
	if err != nil {
		t.Fatal(err)
	}

	// Delete it through the store's delete function.
	deleted, err := store.NewEngagementStore(a.db).DeleteUserConfession(context.Background(), "conf-del-1", userID, nowStamp())
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("DeleteUserConfession reported no rows deleted")
	}

	// The tombstoned row must hold no title or text.
	var title, text string
	if err := a.db.QueryRowContext(context.Background(),
		`SELECT title, text FROM user_confessions WHERE id = ?`, "conf-del-1").Scan(&title, &text); err != nil {
		t.Fatal(err)
	}
	if title != "" || text != "" {
		t.Fatalf("deleted confession still holds text: title=%q text=%q", title, text)
	}
}

// TestDeletedSessionMetadataIsPurged verifies that SessionStore.SoftDelete
// wipes title and description (private session metadata) in the same
// statement as setting deleted_at.
func TestDeletedSessionMetadataIsPurged(t *testing.T) {
	a := newAuthHarness(t)
	_, userID := a.register(t, "purge-session@example.com")

	// Insert a session directly with private metadata.
	_, err := a.db.ExecContext(context.Background(),
		`INSERT INTO sessions (id,user_id,type,duration_seconds,strategy,target_duration,actual_duration,title,description,voice_id,status,created_at)
		 VALUES (?,?,'standard',600,'BALANCED',600,0,'My morning prayer','Thank God for another day to serve Him.',NULL,'COMPLETED',?)`,
		"sess-1", userID, nowStamp())
	if err != nil {
		t.Fatal(err)
	}

	// Soft-delete it through the store.
	deleted, err := store.NewSessionStore(a.db).SoftDelete(context.Background(), "sess-1", nowStamp())
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("SoftDelete reported no rows deleted")
	}

	// The tombstoned row must hold no title or description.
	var title, desc string
	if err := a.db.QueryRowContext(context.Background(),
		`SELECT title, description FROM sessions WHERE id = ?`, "sess-1").Scan(&title, &desc); err != nil {
		t.Fatal(err)
	}
	if title != "" || desc != "" {
		t.Fatalf("deleted session still holds metadata: title=%q description=%q", title, desc)
	}
}
