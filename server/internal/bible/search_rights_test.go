package bible

import (
	"context"
	"testing"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// Indexed lexemes can reveal an edition's content even when a public query is
// gated. Grant/revoke therefore changes the physical search corpus as well as
// the public result filter, in the same database transaction.
func TestSearchIndexFollowsRightsAndNeverIndexesPendingText(t *testing.T) {
	conn := dbtest.New(t)
	ctx := context.Background()
	provider := &LocalBibleProvider{DB: conn}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	const id = "rights-fixture"
	exec(`INSERT INTO bible_versions
		(id,name,abbrev,language,language_name,format,coverage,licence,attribution,blob_url,sha256,created_at,updated_at)
		VALUES(?, 'Rights test fixture','TEST','en','English','osis','full','test','','fixture://test','fixture-checksum','2026-01-01','2026-01-01')`, id)
	exec(`INSERT INTO bible_verses(version_id,book_id,book_name,testament,canonical_order,chapter,verse,text,created_at,updated_at)
		VALUES(?,'John','John','new',43,3,16,'Unpublished fixture vocabulary','2026-01-01','2026-01-01')`, id)
	countDocs := func(want int) {
		t.Helper()
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM bible_search_documents WHERE version_id=?`, id).Scan(&count); err != nil || count != want {
			t.Fatalf("indexed documents = %d, want %d (%v)", count, want, err)
		}
	}
	search := func(want int) {
		t.Helper()
		results, err := provider.Search(ctx, "fixture", id, "", 20)
		if err != nil || len(results) != want {
			t.Fatalf("search results = %d, want %d (%v)", len(results), want, err)
		}
	}
	countDocs(0)
	search(0)
	exec(`UPDATE bible_versions SET status='active',api_exposure_allowed=TRUE WHERE id=?`, id)
	countDocs(0) // API exposure alone never implies a search grant
	search(0)
	exec(`UPDATE bible_versions SET search_index_allowed=TRUE WHERE id=?`, id)
	countDocs(1)
	search(1)
	exec(`INSERT INTO bible_verses(version_id,book_id,book_name,testament,canonical_order,chapter,verse,text,created_at,updated_at)
		VALUES(?,'John','John','new',43,3,17,'Another fixture entry','2026-01-01','2026-01-01')`, id)
	countDocs(2) // new verses inherit a previously granted permission
	search(2)
	exec(`UPDATE bible_versions SET search_index_allowed=FALSE WHERE id=?`, id)
	countDocs(0)
	search(0)
	exec(`UPDATE bible_versions SET search_index_allowed=TRUE WHERE id=?`, id)
	countDocs(2)
	exec(`UPDATE bible_versions SET status='suspended' WHERE id=?`, id)
	countDocs(0)
	search(0)
	exec(`UPDATE bible_versions SET status='active',api_exposure_allowed=FALSE WHERE id=?`, id)
	countDocs(0)
	exec(`UPDATE bible_versions SET api_exposure_allowed=TRUE WHERE id=?`, id)
	countDocs(2)
	exec(`UPDATE bible_versions SET deleted_at=now() WHERE id=?`, id)
	countDocs(0)
	search(0)
}
