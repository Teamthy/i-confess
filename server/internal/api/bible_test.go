package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// The reader's HTTP surface.
//
// These tests are about the promises a client depends on rather than the SQL:
// a translation that has not been imported is refused rather than half-served,
// a confession that cites a range answers for every verse in it, a highlight
// belongs to one account and one translation, and a verse a translation does
// not contain cannot be marked at all.

// seedBibleChapter loads a version and one chapter of it, so a test can exercise
// the read path without the 10MB source that the importer loads in production.
func (a *authHarness) seedBibleChapter(t *testing.T, versionID, book string, chapter int, verses map[int]string) {
	t.Helper()
	ctx := context.Background()
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO bible_versions (id,name,abbrev,language,language_name,format,coverage,licence,
		    attribution,blob_url,sha256,bytes,sort_order,is_default,book_count,chapter_count,
		    verse_count,imported_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT (id) DO UPDATE SET book_count = EXCLUDED.book_count,
		     chapter_count = EXCLUDED.chapter_count, verse_count = EXCLUDED.verse_count,
		     imported_at = EXCLUDED.imported_at`,
		versionID, "Test Version", "TST", "en", "English", "osis", "full", "Public Domain",
		"test", "https://example.invalid/test", "deadbeef", 1, 99, 0, 1, 1, len(verses),
		"2026-01-01", "2026-01-01", "2026-01-01"); err != nil {
		t.Fatalf("seed version: %v", err)
	}
	order := 19 // Psalms, which is what these fixtures use
	if book == "John" {
		order = 43
	}
	for n, text := range verses {
		if _, err := a.db.ExecContext(ctx,
			`INSERT INTO bible_verses (version_id,book_id,book_name,testament,canonical_order,
			    chapter,verse,text,created_at,updated_at)
			 VALUES (?,?,?,?,?,?,?,?,'2026-01-01','2026-01-01')
			 ON CONFLICT (version_id,book_id,chapter,verse) DO NOTHING`,
			versionID, book, bookNameFor(book), testamentFor(order), order, chapter, n, text); err != nil {
			t.Fatalf("seed verse %d: %v", n, err)
		}
	}
}

func bookNameFor(book string) string {
	if book == "John" {
		return "John"
	}
	return "Psalms"
}

func testamentFor(order int) string {
	if order >= 40 {
		return "new"
	}
	return "old"
}

// seedCitingConfession publishes a confession that cites a reference, with the
// normalised citation columns a real import would write.
func (a *authHarness) seedCitingConfession(t *testing.T, id, title, book string, chapter int, verse, bookID string, start, end int) {
	t.Helper()
	ctx := context.Background()
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO categories (id,name,slug,status,sort_order,created_at,updated_at)
		 VALUES ('cat-bib','Peace','peace','published',1,'2026-01-01','2026-01-01')
		 ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatalf("seed category: %v", err)
	}
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO confessions (id,category_id,title,short_text,intensity,language,status,author,version,created_at,updated_at)
		 VALUES (?,'cat-bib',?,'short',1,'en','published','test',1,'2026-01-01','2026-01-01')
		 ON CONFLICT (id) DO NOTHING`, id, title); err != nil {
		t.Fatalf("seed confession: %v", err)
	}
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO scripture_references (id,confession_id,book,chapter,verse,book_id,verse_start,verse_end,translation,is_direct_quote,sort_order)
		 VALUES (?,?,?,?,?,?,?,?,'KJV',1,1)`,
		"ref-"+id, id, book, chapter, verse, bookID, start, end); err != nil {
		t.Fatalf("seed reference: %v", err)
	}
}

// TestBibleVersionsAreOfferedWithTheirLicence is the picker's contract: every
// translation, its coverage, and the terms it arrives under.
func TestBibleVersionsAreOfferedWithTheirLicence(t *testing.T) {
	a := newAuthHarness(t)

	for _, path := range []string{"/bible/versions", "/v1/bible/versions"} {
		code, out := a.do("GET", path, "", nil)
		if code != http.StatusOK {
			t.Fatalf("%s: status %d", path, code)
		}
		versions, _ := out["versions"].([]any)
		if len(versions) < 12 {
			t.Fatalf("%s listed %d translations, want at least 12", path, len(versions))
		}
		if def, _ := out["default"].(string); def != "kjv" {
			t.Errorf("%s: default is %q, want kjv", path, def)
		}
		for _, raw := range versions {
			v, _ := raw.(map[string]any)
			if v["licence"] == nil || v["licence"] == "" {
				t.Errorf("%s: %v has no licence", path, v["id"])
			}
			if v["language_name"] == nil || v["language_name"] == "" {
				t.Errorf("%s: %v has no language name", path, v["id"])
			}
			// Nothing is imported in this test, so every version must say so
			// rather than pretending to be readable.
			if imported, _ := v["imported"].(bool); imported {
				t.Errorf("%s: %v claims to be imported with no text loaded", path, v["id"])
			}
		}
	}
}

// TestUnimportedTranslationIsRefusedRatherThanHalfServed pins the difference
// between "we ship this" and "this is readable".
func TestUnimportedTranslationIsRefusedRatherThanHalfServed(t *testing.T) {
	a := newAuthHarness(t)

	code, out := a.do("GET", "/bible/versions/kjv/books", "", nil)
	if code != http.StatusNotFound {
		t.Fatalf("books for an unimported version: status %d, want 404", code)
	}
	if out["code"] != "BIBLE_VERSION_UNAVAILABLE" {
		t.Errorf("code = %v, want BIBLE_VERSION_UNAVAILABLE", out["code"])
	}
	if code, _ := a.do("GET", "/bible/versions/niv/books", "", nil); code != http.StatusNotFound {
		t.Errorf("a translation the registry does not ship: status %d, want 404", code)
	}
}

// TestChapterCrossLinksVersesToConfessions is the feature's central promise: a
// verse that has a confessional use says so, and the confession it names really
// cites that verse.
func TestChapterCrossLinksVersesToConfessions(t *testing.T) {
	a := newAuthHarness(t)
	a.seedBibleChapter(t, "kjv", "Ps", 103, map[int]string{
		1: "Bless the LORD, O my soul",
		2: "Bless the LORD, O my soul, and forget not all his benefits",
		3: "Who forgiveth all thine iniquities",
	})
	a.seedCitingConfession(t, "conf-praise", "Morning praise", "Psalm", 103, "2-3", "Ps", 2, 3)

	code, out := a.do("GET", "/bible/versions/kjv/books/Ps/chapters/103", "", nil)
	if code != http.StatusOK {
		t.Fatalf("chapter: status %d (%v)", code, out)
	}
	chapter, _ := out["chapter"].(map[string]any)
	verses, _ := chapter["verses"].([]any)
	if len(verses) != 3 {
		t.Fatalf("chapter returned %d verses, want 3", len(verses))
	}

	// Both verses the citation covers are linked; the one it does not is not.
	for i, want := range []bool{false, true, true} {
		v, _ := verses[i].(map[string]any)
		links, _ := v["confessions"].([]any)
		if want && len(links) != 1 {
			t.Errorf("verse %v carries %d confessions, want 1", v["verse"], len(links))
		}
		if !want && len(links) != 0 {
			t.Errorf("verse %v carries %d confessions, want none", v["verse"], len(links))
		}
		if !want {
			continue
		}
		link, _ := links[0].(map[string]any)
		if link["confession_id"] != "conf-praise" {
			t.Errorf("verse %v links to %v", v["verse"], link["confession_id"])
		}
		if link["category_name"] == nil || link["category_name"] == "" {
			t.Errorf("cross-link carries no category: %v", link)
		}
		if link["reference"] != "Psalms 103:2-3" {
			t.Errorf("cross-link renders the citation as %v", link["reference"])
		}
		if speakable, _ := link["speakable"].(bool); !speakable {
			t.Error("a confession with no recording should be speakable")
		}
	}

	// The reference is rendered for the reader, not left as coordinates.
	first, _ := verses[0].(map[string]any)
	if first["reference"] != "Psalms 103:1" {
		t.Errorf("verse reference = %v, want Psalms 103:1", first["reference"])
	}
}

// TestPassageResolvesADeepLink covers the link a confession points at: a single
// verse, and a range.
func TestPassageResolvesADeepLink(t *testing.T) {
	a := newAuthHarness(t)
	a.seedBibleChapter(t, "kjv", "Ps", 103, map[int]string{
		2: "Bless the LORD, O my soul, and forget not all his benefits",
		3: "Who forgiveth all thine iniquities",
	})
	a.seedCitingConfession(t, "conf-praise", "Morning praise", "Psalm", 103, "2-3", "Ps", 2, 3)

	code, out := a.do("GET", "/bible/versions/kjv/passages/Ps.103.2", "", nil)
	if code != http.StatusOK {
		t.Fatalf("passage: status %d (%v)", code, out)
	}
	if out["reference"] != "Psalms 103:2" {
		t.Errorf("reference = %v", out["reference"])
	}
	verses, _ := out["verses"].([]any)
	if len(verses) != 1 {
		t.Fatalf("single-verse passage returned %d verses", len(verses))
	}
	verse, _ := verses[0].(map[string]any)
	if text, _ := verse["text"].(string); text == "" {
		t.Error("the passage carries no text")
	}
	links, _ := verse["confessions"].([]any)
	if len(links) != 1 {
		t.Errorf("the verse deep link carries %d confessions, want 1", len(links))
	}

	// A range comes back as the verses it names, and a book name in any dialect
	// resolves to the same one: "Psalm" here, "PSA" below.
	code, out = a.do("GET", "/v1/bible/versions/kjv/passages/PSA.103.2-3", "", nil)
	if code != http.StatusOK {
		t.Fatalf("range passage: status %d", code)
	}
	if out["reference"] != "Psalms 103:2-3" {
		t.Errorf("range reference = %v", out["reference"])
	}
	if verses, _ := out["verses"].([]any); len(verses) != 2 {
		t.Errorf("range returned %d verses, want 2", len(verses))
	}

	// A verse that does not exist is a 404, and an unreadable reference is a
	// 400: neither may resolve to something plausible.
	if code, _ := a.do("GET", "/bible/versions/kjv/passages/Ps.103.9", "", nil); code != http.StatusNotFound {
		t.Errorf("missing verse: status %d, want 404", code)
	}
	if code, _ := a.do("GET", "/bible/versions/kjv/passages/Ps.103.abc", "", nil); code != http.StatusBadRequest {
		t.Errorf("unreadable reference: status %d, want 400", code)
	}
	if code, _ := a.do("GET", "/bible/versions/kjv/passages/Hezekiah.1.1", "", nil); code != http.StatusBadRequest {
		t.Errorf("unknown book: status %d, want 400", code)
	}
}

// TestVerseConfessionsAreVersionIndependent: a confession cites a reference,
// not a translation, so the same answer comes back whatever version is being
// read.
func TestVerseConfessionsAreVersionIndependent(t *testing.T) {
	a := newAuthHarness(t)
	a.seedCitingConfession(t, "conf-praise", "Morning praise", "Psalm", 103, "2-3", "Ps", 2, 3)

	code, out := a.do("GET", "/bible/verses/Ps/103/3/confessions", "", nil)
	if code != http.StatusOK {
		t.Fatalf("status %d (%v)", code, out)
	}
	if out["reference"] != "Psalms 103:3" {
		t.Errorf("reference = %v", out["reference"])
	}
	if links, _ := out["confessions"].([]any); len(links) != 1 {
		t.Errorf("Psalm 103:3 matched %d confessions, want 1", len(links))
	}
	if code, out := a.do("GET", "/v1/bible/verses/Ps/103/4/confessions", "", nil); code != http.StatusOK {
		t.Errorf("status %d", code)
	} else if links, _ := out["confessions"].([]any); len(links) != 0 {
		t.Errorf("Psalm 103:4 matched %d confessions, want none", len(links))
	}
}

// TestHighlightLifecycle walks the gesture a reader makes, including the two
// rules that are easy to get wrong: a second colour changes the mark instead of
// making a second one, and a mark is private to its account.
func TestHighlightLifecycle(t *testing.T) {
	a := newAuthHarness(t)
	a.seedBibleChapter(t, "kjv", "Ps", 103, map[int]string{2: "Bless the LORD, O my soul"})
	token, _ := a.register(t, "reader-1@test.com")
	other, _ := a.register(t, "reader-2@test.com")

	path := "/me/bible/highlights/kjv/Ps/103/2"

	// Anonymous: refused, because a highlight has no owner otherwise.
	if code, _ := a.do("PUT", path, "", map[string]any{"color": "yellow"}); code != http.StatusUnauthorized {
		t.Errorf("anonymous highlight: status %d, want 401", code)
	}

	code, first := a.do("PUT", path, token, map[string]any{"color": "yellow"})
	if code != http.StatusOK {
		t.Fatalf("highlight: status %d (%v)", code, first)
	}
	if first["reference"] != "Psalms 103:2" {
		t.Errorf("highlight reference = %v", first["reference"])
	}

	// A second colour is the same highlight, re-coloured.
	code, second := a.do("PUT", path, token, map[string]any{"color": "green"})
	if code != http.StatusOK {
		t.Fatalf("second highlight: status %d", code)
	}
	if second["id"] != first["id"] {
		t.Errorf("re-colouring made a new highlight (%v then %v)", first["id"], second["id"])
	}

	_, list := a.do("GET", "/me/bible/highlights?version_id=kjv&book=Ps&chapter=103", token, nil)
	highlights, _ := list["highlights"].([]any)
	if len(highlights) != 1 {
		t.Fatalf("after re-colouring, %d highlights, want 1", len(highlights))
	}
	mark, _ := highlights[0].(map[string]any)
	if mark["color"] != "green" {
		t.Errorf("colour = %v, want green", mark["color"])
	}
	if text, _ := mark["text"].(string); text == "" {
		t.Error("the highlight list carries no verse text")
	}

	// Another account sees nothing, in the list and in the chapter payload.
	_, otherList := a.do("GET", "/v1/me/bible/highlights", other, nil)
	if theirs, _ := otherList["highlights"].([]any); len(theirs) != 0 {
		t.Errorf("a second account sees %d highlights, want none", len(theirs))
	}
	_, publicChapter := a.do("GET", "/bible/versions/kjv/books/Ps/chapters/103", "", nil)
	chapter, _ := publicChapter["chapter"].(map[string]any)
	verses, _ := chapter["verses"].([]any)
	verse, _ := verses[0].(map[string]any)
	if verse["highlight"] != nil {
		t.Error("the public chapter carried a reader's highlight")
	}

	// Clearing is idempotent and the verse disappears from the list.
	if code, _ := a.do("DELETE", path, token, nil); code != http.StatusNoContent {
		t.Errorf("delete: status %d, want 204", code)
	}
	if code, _ := a.do("DELETE", path, token, nil); code != http.StatusNoContent {
		t.Errorf("second delete: status %d, want 204", code)
	}
	_, after := a.do("GET", "/me/bible/highlights", token, nil)
	if remaining, _ := after["highlights"].([]any); len(remaining) != 0 {
		t.Errorf("after clearing, %d highlights remain", len(remaining))
	}

	// The palette is enforced at the edge: an unknown colour is a 400 rather
	// than a constraint violation the client reads as a server fault.
	if code, _ := a.do("PUT", path, token, map[string]any{"color": "chartreuse"}); code != http.StatusBadRequest {
		t.Errorf("unknown colour: status %d, want 400", code)
	}
	// A verse this translation does not have cannot be marked.
	if code, _ := a.do("PUT", "/me/bible/highlights/kjv/Ps/103/9", token, map[string]any{"color": "blue"}); code != http.StatusNotFound {
		t.Errorf("marking a missing verse: status %d, want 404", code)
	}
}

// TestBookmarkLifecycle covers the note-taking half: a label, a note, an edit,
// and the same privacy rules as a highlight.
func TestBookmarkLifecycle(t *testing.T) {
	a := newAuthHarness(t)
	a.seedBibleChapter(t, "kjv", "John", 3, map[int]string{16: "For God so loved the world"})
	token, _ := a.register(t, "reader-3@test.com")

	path := "/me/bible/bookmarks/kjv/John/3/16"
	code, created := a.do("PUT", path, token, map[string]any{"label": "gospel", "note": "read at baptisms"})
	if code != http.StatusOK {
		t.Fatalf("bookmark: status %d (%v)", code, created)
	}
	if created["reference"] != "John 3:16" {
		t.Errorf("reference = %v", created["reference"])
	}

	_, list := a.do("GET", "/v1/me/bible/bookmarks?version_id=kjv", token, nil)
	bookmarks, _ := list["bookmarks"].([]any)
	if len(bookmarks) != 1 {
		t.Fatalf("%d bookmarks, want 1", len(bookmarks))
	}
	mark, _ := bookmarks[0].(map[string]any)
	if mark["note"] != "read at baptisms" || mark["text"] == nil {
		t.Errorf("bookmark = %v", mark)
	}

	// Editing is an update, not a second bookmark.
	code, edited := a.do("PUT", path, token, map[string]any{"label": "gospel", "note": "shortened"})
	if code != http.StatusOK || edited["id"] != created["id"] {
		t.Errorf("editing produced %v (status %d), want the same row %v", edited["id"], code, created["id"])
	}
	_, list = a.do("GET", "/me/bible/bookmarks?version_id=kjv&book=John&chapter=3", token, nil)
	if bookmarks, _ = list["bookmarks"].([]any); len(bookmarks) != 1 {
		t.Fatalf("after editing, %d bookmarks, want 1", len(bookmarks))
	}

	// A note longer than the limit is refused before it reaches the database.
	code, _ = a.do("PUT", path, token, map[string]any{"note": strings.Repeat("a", 501)})
	if code != http.StatusBadRequest {
		t.Errorf("oversized note: status %d, want 400", code)
	}
	// Padding is not content: a note that is only whitespace is stored as empty
	// rather than as 500 spaces that count against the limit.
	code, padded := a.do("PUT", path, token, map[string]any{"note": strings.Repeat(" ", 501)})
	if code != http.StatusOK {
		t.Errorf("whitespace note: status %d, want 200", code)
	} else if note, _ := padded["note"].(string); note != "" {
		t.Errorf("whitespace note stored as %q, want empty", note)
	}

	if code, _ := a.do("DELETE", path, token, nil); code != http.StatusNoContent {
		t.Errorf("delete: status %d, want 204", code)
	}
	_, list = a.do("GET", "/me/bible/bookmarks", token, nil)
	if bookmarks, _ := list["bookmarks"].([]any); len(bookmarks) != 0 {
		t.Errorf("after deleting, %d bookmarks remain", len(bookmarks))
	}
}

// TestReadingSurfaceDoesNotRequireAuthentication: Scripture is not personal
// data, and a reader must be able to open a verse before signing in.
func TestReadingSurfaceDoesNotRequireAuthentication(t *testing.T) {
	a := newAuthHarness(t)
	a.seedBibleChapter(t, "kjv", "John", 3, map[int]string{16: "For God so loved the world"})

	for _, path := range []string{
		"/bible/versions",
		"/bible/versions/kjv",
		"/bible/versions/kjv/books",
		"/bible/versions/kjv/books/John/chapters/3",
		"/bible/versions/kjv/passages/John.3.16",
		"/bible/verses/John/3/16/confessions",
	} {
		if code, out := a.do("GET", path, "", nil); code != http.StatusOK {
			t.Errorf("%s: status %d (%v)", path, code, out)
		}
	}
	// The marks, by contrast, are session-validated.
	for _, path := range []string{"/me/bible/highlights", "/me/bible/bookmarks"} {
		if code, _ := a.do("GET", path, "", nil); code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", path, code)
		}
	}
}

// TestBooksListWhatIsLoaded: the picker has to describe the translation that
// exists, including a New Testament that is only a New Testament.
func TestBooksListWhatIsLoaded(t *testing.T) {
	a := newAuthHarness(t)
	a.seedBibleChapter(t, "swahili", "John", 3, map[int]string{16: "Kwa maana Mungu aliupenda ulimwengu"})

	code, out := a.do("GET", "/bible/versions/swahili/books", "", nil)
	if code != http.StatusOK {
		t.Fatalf("status %d (%v)", code, out)
	}
	version, _ := out["version"].(map[string]any)
	if version["coverage"] != "new_testament" {
		t.Errorf("swahili coverage = %v, want new_testament", version["coverage"])
	}
	// The label carries the declared omissions too: the Swahili New Testament
	// really is missing Philippians, and the reader is told rather than shown
	// a book with no verses in it.
	if label, _ := version["coverage_label"].(string); !strings.HasPrefix(label, "New Testament") {
		t.Errorf("coverage label = %q, want it to start with New Testament", label)
	}
	books, _ := out["books"].([]any)
	if len(books) != 1 {
		t.Fatalf("%d books listed, want 1", len(books))
	}
	book, _ := books[0].(map[string]any)
	if book["id"] != "John" || book["testament"] != "new" {
		t.Errorf("book = %v", book)
	}
	if chapters, _ := book["chapters"].(float64); int(chapters) != 1 {
		t.Errorf("chapters = %v, want 1", book["chapters"])
	}
	if canon, _ := book["canon_chapters"].(float64); int(canon) != 21 {
		t.Errorf("canon_chapters = %v, want 21", book["canon_chapters"])
	}
}
