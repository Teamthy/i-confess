package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/storage"
)

// A ready database row whose object is missing is not an acceptable license
// source. The provider must be consulted again before any regeneration.
type unavailableChapterProvider struct{ bible.BibleProvider }

func (unavailableChapterProvider) GetChapter(context.Context, string, string, int) (bible.Chapter, error) {
	return bible.Chapter{}, bible.ErrUnavailable
}

func TestOfflineBiblePackageIsStableAndRevokedWhenRightsChange(t *testing.T) {
	conn := dbtest.New(t)
	inst := newCacheInstance(t, "Bible offline", conn, nil)
	objStore, err := storage.New(&storage.StorageConfig{
		Provider: "local", LocalRootPath: t.TempDir(), CDNDomain: "/media", SigningSecret: "offline-test-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	inst.h.SetSigner(objStore)
	user := registerAndSignIn(t, inst.srv, "offline-reader@example.com", "a-strong-enough-passphrase")
	ctx := context.Background()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Fixture wording is deliberately not a Bible verse; only the rights,
	// package and storage paths are under test.
	exec(`INSERT INTO bible_versions(id,name,abbrev,language,language_name,format,coverage,licence,attribution,blob_url,sha256,content_hash,created_at,updated_at)
		VALUES('offline-fixture','Fixture','TST','en','English','osis','full','test','Fixture','fixture://source','source-sha','source-sha','2026-01-01','2026-01-01')`)
	exec(`INSERT INTO bible_verses(version_id,book_id,book_name,testament,canonical_order,chapter,verse,text,created_at,updated_at)
		VALUES('offline-fixture','John','John','new',43,1,1,'Offline test fixture, not Scripture.','2026-01-01','2026-01-01')`)
	exec(`UPDATE bible_versions SET status='active',api_exposure_allowed=TRUE,offline_allowed=TRUE,
		redistribution_allowed=TRUE,commercial_use=TRUE,offline_max_days=7 WHERE id='offline-fixture'`)
	request := func(want int) (string, map[string]any) {
		t.Helper()
		status, response := doRequest(t, inst.srv, http.MethodPost, "/me/bible/offline", user,
			`{"translation_id":"offline-fixture","book_id":"John"}`)
		if status != want {
			t.Fatalf("offline request got %d, want %d: %s", status, want, response)
		}
		out := map[string]any{}
		if err := json.Unmarshal([]byte(response), &out); err != nil {
			t.Fatal(err)
		}
		return response, out
	}
	_, first := request(http.StatusOK)
	_, second := request(http.StatusOK)
	if first["package_id"] != second["package_id"] || first["content_hash"] != second["content_hash"] || first["license_id"] != second["license_id"] {
		t.Fatalf("repeated request rebuilt or relicensed identical content: %#v / %#v", first, second)
	}
	var count int
	var key, hash string
	if err := conn.QueryRow(`SELECT storage_key,content_hash FROM bible_offline_packages WHERE status='ready'`).Scan(&key, &hash); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT count(*) FROM bible_offline_packages`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("packages=%d: %v", count, err)
	}
	payload, err := objStore.Download(ctx, key)
	if err != nil || sha256Hex(payload) != hash || !strings.Contains(string(payload), "Offline test fixture") || strings.Contains(string(payload), "generated_at") {
		t.Fatalf("source package absent, corrupt, or nondeterministic: %s (%v)", key, err)
	}

	exec(`UPDATE bible_versions SET offline_allowed=FALSE WHERE id='offline-fixture'`)
	if err := conn.QueryRow(`SELECT count(*) FROM bible_offline_packages WHERE status='revoked'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("package not withdrawn atomically: %d (%v)", count, err)
	}
	if err := conn.QueryRow(`SELECT count(*) FROM user_bible_offline_licenses WHERE revoked_at IS NOT NULL`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("license not revoked atomically: %d (%v)", count, err)
	}
	request(http.StatusForbidden)
	exec(`UPDATE bible_versions SET offline_allowed=TRUE WHERE id='offline-fixture'`)
	_, renewed := request(http.StatusOK)
	if renewed["content_hash"] != first["content_hash"] {
		t.Fatalf("unchanged source produced different book bytes: %#v / %#v", first, renewed)
	}
	if err := conn.QueryRow(`SELECT count(*) FROM user_bible_offline_licenses WHERE revoked_at IS NULL`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("renewed license not active: %d (%v)", count, err)
	}
	status, licensesRaw := doRequest(t, inst.srv, http.MethodGet, "/me/bible/offline", user, "")
	if status != http.StatusOK || !strings.Contains(licensesRaw, `"package_status":"ready"`) {
		t.Fatalf("license listing did not expose current package status: %d %s", status, licensesRaw)
	}

	// Missing object + unavailable source must not mint a download URL from
	// a stale ready row. Once the source works again the identical bytes can
	// be restored under the same content-addressed key.
	if err := objStore.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	originalProvider := inst.h.bible
	inst.h.SetBibleProvider(unavailableChapterProvider{originalProvider})
	request(http.StatusServiceUnavailable)
	inst.h.SetBibleProvider(originalProvider)
	_, restored := request(http.StatusOK)
	if restored["content_hash"] != first["content_hash"] {
		t.Fatalf("recovered package has different bytes: %#v", restored)
	}
	if exists, err := objStore.Exists(ctx, key); err != nil || !exists {
		t.Fatalf("missing book was not restored: %v (%v)", exists, err)
	}
}
