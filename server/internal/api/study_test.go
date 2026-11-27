package api

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

// The journal and Scripture memory lived only in localStorage. They are now
// account data: private to the owner, edit-conflict safe, and exported.
func TestJournalIsPrivateAndConflictSafe(t *testing.T) {
	a := newAuthHarness(t)
	me, _ := a.register(t, "journal@example.com")
	other, _ := a.register(t, "snoop@example.com")

	if code, _ := a.do("POST", "/me/journal", me, map[string]string{"body": "  "}); code != http.StatusBadRequest {
		t.Fatalf("empty entry: %d, want 400", code)
	}
	code, e := a.do("POST", "/v1/me/journal", me, map[string]string{"body": "The words met me today.", "mood": "Grateful"})
	if code != http.StatusCreated || e["title"] == "" || e["row_version"].(float64) != 1 {
		t.Fatalf("create: %d %v", code, e)
	}
	id := e["id"].(string)

	if code, out := a.do("GET", "/me/journal", other, nil); code != http.StatusOK || len(out["entries"].([]any)) != 0 {
		t.Fatalf("another user sees entries: %d %v", code, out)
	}
	if code, _ := a.do("PATCH", "/me/journal/"+id, other, map[string]any{"body": "mine now", "row_version": 1}); code != http.StatusNotFound {
		t.Fatalf("another user edited: %d, want 404", code)
	}
	if code, _ := a.do("DELETE", "/me/journal/"+id, other, nil); code != http.StatusNotFound {
		t.Fatalf("another user deleted: %d, want 404", code)
	}

	if code, out := a.do("PATCH", "/me/journal/"+id, me, map[string]any{"body": "Edited.", "row_version": 1}); code != http.StatusOK || out["row_version"].(float64) != 2 {
		t.Fatalf("edit: %d %v", code, out)
	}
	if code, _ := a.do("PATCH", "/me/journal/"+id, me, map[string]any{"body": "Stale.", "row_version": 1}); code != http.StatusConflict {
		t.Fatalf("stale edit: %d, want 409", code)
	}
	if code, _ := a.do("DELETE", "/me/journal/"+id, me, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if _, out := a.do("GET", "/me/journal", me, nil); len(out["entries"].([]any)) != 0 {
		t.Fatalf("deleted entry still listed: %v", out)
	}
}

func TestMemoryVersesAreScheduledByTheServer(t *testing.T) {
	a := newAuthHarness(t)
	me, _ := a.register(t, "memory@example.com")

	add := map[string]string{"reference": "Isaiah 53:5", "text": "By His stripes we are healed.", "title": "I Am Healed"}
	code, out := a.do("POST", "/me/memory", me, add)
	if code != http.StatusCreated || out["created"] != true {
		t.Fatalf("add: %d %v", code, out)
	}
	id := out["verse"].(map[string]any)["id"].(string)
	if code, out := a.do("POST", "/me/memory", me, add); code != http.StatusOK || out["created"] != false {
		t.Fatalf("re-add should not duplicate: %d %v", code, out)
	}

	code, v := a.do("POST", "/me/memory/"+id+"/review", me, map[string]bool{"remembered": true})
	if code != http.StatusOK || v["box"].(float64) != 1 {
		t.Fatalf("review: %d %v", code, v)
	}
	due, _ := time.Parse(time.RFC3339, v["due_at"].(string))
	if d := time.Until(due); d < 23*time.Hour || d > 25*time.Hour {
		t.Fatalf("box 1 should return in a day, got %v", d)
	}
	if _, v := a.do("POST", "/me/memory/"+id+"/review", me, map[string]bool{"remembered": false}); v["box"].(float64) != 0 {
		t.Fatalf("a miss should reset to box 0: %v", v)
	}
	if code, _ := a.do("POST", "/me/memory/"+id+"/review", me, map[string]string{}); code != http.StatusBadRequest {
		t.Fatalf("review without a result: %d, want 400", code)
	}
	if code, _ := a.do("DELETE", "/me/memory/"+id, me, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if _, out := a.do("GET", "/me/memory", me, nil); len(out["verses"].([]any)) != 0 {
		t.Fatalf("removed verse still listed: %v", out)
	}
	// Re-adding a removed verse starts it over.
	if code, out := a.do("POST", "/me/memory", me, add); code != http.StatusCreated || out["verse"].(map[string]any)["box"].(float64) != 0 {
		t.Fatalf("re-add after delete: %d %v", code, out)
	}
}

func TestExportIncludesStudyDataAndHistory(t *testing.T) {
	a := newAuthHarness(t)
	me, _ := a.register(t, "export-study@example.com")
	a.do("POST", "/me/journal", me, map[string]string{"body": "Kept."})
	a.do("POST", "/me/memory", me, map[string]string{"reference": "John 3:16", "text": "For God so loved the world."})
	a.do("POST", "/community/posts", me, map[string]string{"body": "Mine."})
	code, out := a.do("GET", "/me/export", me, nil)
	if code != http.StatusOK {
		t.Fatalf("export: %d", code)
	}
	for _, k := range []string{"journal", "memory_verses", "community_posts", "listening_history"} {
		if _, ok := out[k]; !ok {
			t.Errorf("export is missing %q", k)
		}
	}
	if len(out["journal"].([]any)) != 1 || len(out["memory_verses"].([]any)) != 1 || len(out["community_posts"].([]any)) != 1 {
		t.Fatalf("export contents: %v", out)
	}
}

// The library index is what the website maps its confession slugs against,
// so favourites and history can be stored by id.
func TestLibraryIndexListsPublishedConfessionsWithSlugs(t *testing.T) {
	a := newAuthHarness(t)
	ctx := context.Background()
	content := store.NewContentStore(a.db)
	pub := &models.Category{Name: "Healing", Slug: "healing", Status: "published"}
	hidden := &models.Category{Name: "Draft Area", Slug: "draft-area", Status: "draft"}
	for _, c := range []*models.Category{pub, hidden} {
		if err := content.CreateCategory(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []*models.Confession{
		{CategoryID: pub.ID, Title: "I Am Healed!", Status: "published", Language: "en", ShortText: "By His stripes."},
		{CategoryID: pub.ID, Title: "Not Yet", Status: "draft", Language: "en", ShortText: "x"},
		{CategoryID: hidden.ID, Title: "Hidden Category", Status: "published", Language: "en", ShortText: "x"},
	} {
		if err := content.CreateConfession(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	code, out := a.do("GET", "/v1/confessions", "", nil)
	if code != http.StatusOK {
		t.Fatalf("index: %d %v", code, out)
	}
	list := out["confessions"].([]any)
	if len(list) != 1 {
		t.Fatalf("index = %v, want only the published confession in a published category", list)
	}
	c := list[0].(map[string]any)
	if c["slug"] != "i-am-healed" || c["category_slug"] != "healing" || c["short_text"] != "By His stripes." {
		t.Fatalf("entry = %v", c)
	}
}

// Web settings (speed, EQ, quiet hours…) reset on every new browser because
// they lived only in localStorage. They now round-trip through preferences.
func TestClientSettingsFollowTheAccount(t *testing.T) {
	a := newAuthHarness(t)
	me, _ := a.register(t, "settings@example.com")
	if code, out := a.do("GET", "/me/preferences", me, nil); code != http.StatusOK || out["client_settings"] == nil {
		t.Fatalf("defaults: %d %v", code, out)
	}
	body := map[string]any{"client_settings": map[string]any{"rate": 1.25, "eq": "night", "privacy": map[string]any{"pauseHistory": true}}}
	if code, out := a.do("PATCH", "/me/preferences", me, body); code != http.StatusOK {
		t.Fatalf("save: %d %v", code, out)
	}
	_, out := a.do("GET", "/v1/me/preferences", me, nil)
	cs, _ := out["client_settings"].(map[string]any)
	if cs["rate"] != 1.25 || cs["eq"] != "night" || cs["privacy"].(map[string]any)["pauseHistory"] != true {
		t.Fatalf("client_settings = %v", out["client_settings"])
	}
	if code, _ := a.do("PATCH", "/me/preferences", me, map[string]any{"client_settings": []int{1}}); code != http.StatusBadRequest {
		t.Fatalf("non-object settings: %d, want 400", code)
	}
}

// Empty lists are [], not null: the web sync crashed on a new account's null
// favourites before this.
func TestEmptyFavoritesAndHistoryAreArrays(t *testing.T) {
	a := newAuthHarness(t)
	me, _ := a.register(t, "empty-lists@example.com")
	for _, p := range []string{"/me/favorites", "/me/history"} {
		code, list := a.doList("GET", p, me)
		if code != http.StatusOK || list == nil {
			t.Fatalf("%s: %d %v, want 200 []", p, code, list)
		}
	}
}

// Older entries must stay reachable: the list pages with a cursor instead of
// stopping at the first 100.
func TestJournalPagesThroughEveryEntry(t *testing.T) {
	a := newAuthHarness(t)
	me, _ := a.register(t, "pages@example.com")
	for i := 0; i < 5; i++ {
		if code, _ := a.do("POST", "/me/journal", me, map[string]string{"body": "entry"}); code != http.StatusCreated {
			t.Fatalf("create %d: %d", i, code)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 10; page++ {
		path := "/me/journal?limit=2"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		code, out := a.do("GET", path, me, nil)
		if code != http.StatusOK {
			t.Fatalf("page %d: %d %v", page, code, out)
		}
		for _, e := range out["entries"].([]any) {
			id := e.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatalf("entry %s returned twice", id)
			}
			seen[id] = true
		}
		cursor, _ = out["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}
	if len(seen) != 5 {
		t.Fatalf("paged through %d entries, want 5", len(seen))
	}
	if code, _ := a.do("GET", "/me/journal?cursor=garbage", me, nil); code != http.StatusBadRequest {
		t.Fatalf("bad cursor: %d, want 400", code)
	}
}

// "Deleted" must mean the words are gone, not hidden: the tombstone row keeps
// no title, body or mood.
func TestDeletedJournalTextIsErased(t *testing.T) {
	a := newAuthHarness(t)
	me, _ := a.register(t, "erase@example.com")
	_, e := a.do("POST", "/me/journal", me, map[string]string{"title": "Private", "body": "A prayer only I should read.", "mood": "Heavy"})
	id := e["id"].(string)
	if code, _ := a.do("DELETE", "/me/journal/"+id, me, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	var title, body, mood string
	if err := a.db.QueryRowContext(context.Background(),
		`SELECT title, body, mood FROM journal_entries WHERE id = ?`, id).Scan(&title, &body, &mood); err != nil {
		t.Fatal(err)
	}
	if title != "" || body != "" || mood != "" {
		t.Fatalf("deleted entry still holds text: title=%q body=%q mood=%q", title, body, mood)
	}

	_, v := a.do("POST", "/me/memory", me, map[string]string{"reference": "Psalm 23:1", "text": "The Lord is my shepherd.", "title": "Shepherd"})
	vid := v["verse"].(map[string]any)["id"].(string)
	a.do("DELETE", "/me/memory/"+vid, me, nil)
	var text, vtitle string
	if err := a.db.QueryRowContext(context.Background(),
		`SELECT text, title FROM memory_verses WHERE id = ?`, vid).Scan(&text, &vtitle); err != nil {
		t.Fatal(err)
	}
	if text != "" || vtitle != "" {
		t.Fatalf("removed verse still holds text: %q %q", text, vtitle)
	}
}

// The export says it is everything; it must not stop at a page size.
func TestExportIsNotCappedAtAPage(t *testing.T) {
	a := newAuthHarness(t)
	me, _ := a.register(t, "export-all@example.com")
	const n = 205 // past the old 200 journal cap
	for i := 0; i < n; i++ {
		if code, _ := a.do("POST", "/me/journal", me, map[string]string{"body": "kept"}); code != http.StatusCreated {
			t.Fatalf("create %d: %d", i, code)
		}
	}
	code, out := a.do("GET", "/me/export", me, nil)
	if code != http.StatusOK {
		t.Fatalf("export: %d", code)
	}
	if got := len(out["journal"].([]any)); got != n {
		t.Fatalf("export holds %d journal entries, want %d", got, n)
	}
}
