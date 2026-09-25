package store

import (
	"context"
	"errors"
	"testing"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/models"
)

// The reader's store, against a real database.
//
// Every test here runs on PostgreSQL rather than a stand-in, because the parts
// worth testing are the parts that only exist in the database: the partial
// unique index that makes a second highlight an update, the interval
// containment that makes a confession citing "Psalm 103:2-3" answer for verse 3,
// and the ordering that makes a bookmark list read like Scripture.

// seedBibleVersion ensures the version row a verse must hang off exists. The
// text and the version are separate tables with a foreign key between them, so
// a test that inserts only verses fails the constraint rather than the
// assertion it was written to make.
func seedBibleVersion(t *testing.T, s *BibleStore, versionID string) {
	t.Helper()
	if _, err := s.db.ExecContext(context.Background(),
		`INSERT INTO bible_versions (id,name,abbrev,language,language_name,format,coverage,licence,
		    attribution,blob_url,sha256,bytes,sort_order,is_default,book_count,chapter_count,
		    verse_count,imported_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT (id) DO NOTHING`,
		versionID, "Test Version", "TST", "en", "English", "osis", "full", "Public Domain",
		"test", "https://example.invalid/test", "deadbeef", 1, 99, 0, 1, 1, 1, now(), now(), now()); err != nil {
		t.Fatalf("seed version: %v", err)
	}
}

func seedVerse(t *testing.T, s *BibleStore, versionID, book string, chapter, verse int, text string) {
	t.Helper()
	seedBibleVersion(t, s, versionID)
	if _, err := s.db.ExecContext(context.Background(),
		`INSERT INTO bible_verses (version_id,book_id,book_name,testament,canonical_order,
		    chapter,verse,text,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		versionID, book, "Psalms", "old", 19, chapter, verse, text, now(), now()); err != nil {
		t.Fatalf("seed verse: %v", err)
	}
}

// TestVersionRegistryMergesWithWhatIsLoaded pins the rule the picker depends
// on: a translation the registry ships is always offered, and only says
// Imported once its text is actually there.
func TestVersionRegistryMergesWithWhatIsLoaded(t *testing.T) {
	s := NewBibleStore(dbtest.New(t))
	ctx := context.Background()

	versions, err := s.Versions(ctx)
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	if len(versions) < 12 {
		t.Fatalf("expected at least the twelve registered versions, got %d", len(versions))
	}

	var kjv models.BibleVersion
	for _, v := range versions {
		if v.ID == "kjv" {
			kjv = v
		}
		if v.Licence == "" {
			t.Errorf("%s has no licence; the reader is shown a verse without its terms", v.ID)
		}
		if v.Imported {
			t.Errorf("%s reports Imported with no text loaded", v.ID)
		}
	}
	if kjv.Name != "King James Version" || !kjv.Default {
		t.Errorf("kjv came back as %q (default=%v)", kjv.Name, kjv.Default)
	}
	if kjv.CoverageLabel == "" {
		t.Error("kjv has no coverage label")
	}

	// Load a verse and the same version must now report itself as imported.
	seedVerse(t, s, "kjv", "Ps", 103, 2, "Bless the LORD, O my soul")
	if _, err := s.db.ExecContext(ctx,
		`UPDATE bible_versions SET book_count=1, chapter_count=1, verse_count=1 WHERE id='kjv'`); err != nil {
		t.Fatalf("update counts: %v", err)
	}

	v, err := s.Version(ctx, "kjv")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if !v.Imported || v.VerseCount != 1 || v.BookCount != 1 {
		t.Errorf("after loading one verse, kjv = imported:%v books:%d verses:%d",
			v.Imported, v.BookCount, v.VerseCount)
	}

	if _, err := s.Version(ctx, "niv"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a translation the registry does not ship returned %v, want ErrNotFound", err)
	}
}

// TestChapterReadsInOrderAndCarriesNeighbours checks the reader's main query:
// verses ascending, and the chapters either side of this one.
func TestChapterReadsInOrderAndCarriesNeighbours(t *testing.T) {
	s := NewBibleStore(dbtest.New(t))
	ctx := context.Background()

	seedVerse(t, s, "kjv", "Ps", 103, 3, "third")
	seedVerse(t, s, "kjv", "Ps", 103, 1, "first")
	seedVerse(t, s, "kjv", "Ps", 103, 2, "second")
	seedVerse(t, s, "kjv", "Ps", 102, 1, "previous chapter")
	seedVerse(t, s, "kjv", "Ps", 104, 1, "next chapter")

	ch, err := s.Chapter(ctx, "kjv", "Ps", 103)
	if err != nil {
		t.Fatalf("chapter: %v", err)
	}
	if len(ch.Verses) != 3 {
		t.Fatalf("got %d verses, want 3", len(ch.Verses))
	}
	for i, want := range []int{1, 2, 3} {
		if ch.Verses[i].Verse != want {
			t.Errorf("verse %d is %d, want %d", i, ch.Verses[i].Verse, want)
		}
	}
	if ch.Verses[0].Reference != "Psalms 103:1" {
		t.Errorf("reference rendered as %q", ch.Verses[0].Reference)
	}
	if ch.Prev == nil || ch.Prev.Chapter != 102 {
		t.Errorf("prev = %+v, want Psalm 102", ch.Prev)
	}
	if ch.Next == nil || ch.Next.Chapter != 104 {
		t.Errorf("next = %+v, want Psalm 104", ch.Next)
	}

	if _, err := s.Chapter(ctx, "kjv", "Ps", 119); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing chapter returned %v, want ErrNotFound", err)
	}
}

// TestChapterCrossesIntoTheNextBook covers the case a reader hits by accident
// and notices immediately if it is wrong: the last chapter of a book.
func TestChapterCrossesIntoTheNextBook(t *testing.T) {
	s := NewBibleStore(dbtest.New(t))
	ctx := context.Background()

	seedBibleVersion(t, s, "kjv")
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO bible_verses (version_id,book_id,book_name,testament,canonical_order,chapter,verse,text,created_at,updated_at)
		 VALUES ('kjv','Mal','Malachi','old',39,4,1,'last of the old testament',?,?),
		        ('kjv','Matt','Matthew','new',40,1,1,'first of the new',?,?)`, now(), now(), now(), now()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ch, err := s.Chapter(ctx, "kjv", "Mal", 4)
	if err != nil {
		t.Fatalf("chapter: %v", err)
	}
	if ch.Next == nil || ch.Next.Book != "Matt" || ch.Next.Chapter != 1 {
		t.Errorf("next after Malachi 4 = %+v, want Matthew 1", ch.Next)
	}
	if ch.Prev != nil {
		t.Errorf("prev = %+v, want none for the only chapter of the book", ch.Prev)
	}

	back, err := s.Chapter(ctx, "kjv", "Matt", 1)
	if err != nil {
		t.Fatalf("chapter: %v", err)
	}
	if back.Prev == nil || back.Prev.Book != "Mal" {
		t.Errorf("prev before Matthew 1 = %+v, want Malachi", back.Prev)
	}
}

// TestHighlightUpsertChangesTheColourRatherThanStacking is the behaviour the
// partial unique index exists for.
func TestHighlightUpsertChangesTheColourRatherThanStacking(t *testing.T) {
	d := dbtest.New(t)
	s := NewBibleStore(d)
	ctx := context.Background()
	seedUser(t, d, "user-1", "user-1@example.com")
	seedVerse(t, s, "kjv", "Ps", 103, 2, "Bless the LORD, O my soul")

	first := &models.VerseHighlight{VersionID: "kjv", Book: "Ps", Chapter: 103, Verse: 2, Color: "yellow"}
	if err := s.UpsertHighlight(ctx, "user-1", first); err != nil {
		t.Fatalf("first highlight: %v", err)
	}

	second := &models.VerseHighlight{VersionID: "kjv", Book: "Ps", Chapter: 103, Verse: 2, Color: "green"}
	if err := s.UpsertHighlight(ctx, "user-1", second); err != nil {
		t.Fatalf("second highlight: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("the second colour made a new row (%s vs %s) instead of changing the first", second.ID, first.ID)
	}

	marks, err := s.HighlightsInChapter(ctx, "user-1", "kjv", "Ps", 103)
	if err != nil {
		t.Fatalf("highlights in chapter: %v", err)
	}
	if len(marks) != 1 || marks[2] != "green" {
		t.Errorf("chapter marks = %v, want one green mark on verse 2", marks)
	}

	listed, err := s.ListHighlights(ctx, "user-1", "kjv", "Ps", 103)
	if err != nil {
		t.Fatalf("list highlights: %v", err)
	}
	if len(listed) != 1 || listed[0].Reference != "Psalms 103:2" || listed[0].Text == "" {
		t.Errorf("listed highlights = %+v, want one named, resolved mark", listed)
	}

	// The mark belongs to one reader. Another reader sees nothing.
	if other, err := s.ListHighlights(ctx, "user-2", "", "", 0); err != nil || len(other) != 0 {
		t.Errorf("a second user saw %d highlights (err=%v)", len(other), err)
	}

	if err := s.DeleteHighlight(ctx, "user-1", "kjv", "Ps", 103, 2); err != nil {
		t.Fatalf("delete: %v", err)
	}
	after, err := s.ListHighlights(ctx, "user-1", "", "", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("after clearing, %d highlights remain", len(after))
	}

	// Clearing is idempotent, and the same verse can be marked again: the
	// uniqueness applies to live rows only.
	if err := s.DeleteHighlight(ctx, "user-1", "kjv", "Ps", 103, 2); err != nil {
		t.Errorf("clearing an unmarked verse errored: %v", err)
	}
	again := &models.VerseHighlight{VersionID: "kjv", Book: "Ps", Chapter: 103, Verse: 2, Color: "blue"}
	if err := s.UpsertHighlight(ctx, "user-1", again); err != nil {
		t.Errorf("re-marking a cleared verse failed: %v", err)
	}
}

// TestBookmarksAreStoredAndListedInCanonicalOrder pins the ordering rule: a
// saved-verse list reads like Scripture, not like a log of taps.
func TestBookmarksAreStoredAndListedInCanonicalOrder(t *testing.T) {
	d := dbtest.New(t)
	s := NewBibleStore(d)
	ctx := context.Background()
	seedUser(t, d, "user-1", "user-1@example.com")

	// Added out of canonical order on purpose.
	seedBibleVersion(t, s, "kjv")
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO bible_verses (version_id,book_id,book_name,testament,canonical_order,chapter,verse,text,created_at,updated_at)
		 VALUES ('kjv','John','John','new',43,3,16,'For God so loved the world',?,?),
		        ('kjv','Ps','Psalms','old',19,103,2,'Bless the LORD, O my soul',?,?)`,
		now(), now(), now(), now()); err != nil {
		t.Fatalf("seed verses: %v", err)
	}

	john := &models.VerseBookmark{VersionID: "kjv", Book: "John", Chapter: 3, Verse: 16, Label: "gospel"}
	if err := s.UpsertBookmark(ctx, "user-1", john); err != nil {
		t.Fatalf("bookmark John: %v", err)
	}
	psalm := &models.VerseBookmark{VersionID: "kjv", Book: "Ps", Chapter: 103, Verse: 2, Note: "morning prayer"}
	if err := s.UpsertBookmark(ctx, "user-1", psalm); err != nil {
		t.Fatalf("bookmark Psalm: %v", err)
	}

	marks, err := s.ListBookmarks(ctx, "user-1", "kjv", "", 0, 0)
	if err != nil {
		t.Fatalf("list bookmarks: %v", err)
	}
	if len(marks) != 2 {
		t.Fatalf("got %d bookmarks, want 2", len(marks))
	}
	if marks[0].Book != "Ps" || marks[1].Book != "John" {
		t.Errorf("bookmarks came back as %s then %s, want canonical order",
			marks[0].Reference, marks[1].Reference)
	}
	if marks[0].Text == "" || marks[1].Label != "gospel" {
		t.Errorf("bookmark list is missing resolved text or the label: %+v", marks)
	}

	// Editing a bookmark changes it rather than creating a second.
	edited := &models.VerseBookmark{VersionID: "kjv", Book: "John", Chapter: 3, Verse: 16, Label: "gospel", Note: "read at baptisms"}
	if err := s.UpsertBookmark(ctx, "user-1", edited); err != nil {
		t.Fatalf("edit bookmark: %v", err)
	}
	if edited.ID != john.ID {
		t.Errorf("editing made a new bookmark (%s vs %s)", edited.ID, john.ID)
	}
	one, err := s.ListBookmarks(ctx, "user-1", "kjv", "John", 3, 0)
	if err != nil {
		t.Fatalf("filtered list: %v", err)
	}
	if len(one) != 1 || one[0].Note != "read at baptisms" {
		t.Errorf("filtered bookmarks = %+v, want the edited note", one)
	}

	if err := s.DeleteBookmark(ctx, "user-1", "kjv", "John", 3, 16); err != nil {
		t.Fatalf("delete bookmark: %v", err)
	}
	remaining, err := s.ListBookmarks(ctx, "user-1", "", "", 0, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Book != "Ps" {
		t.Errorf("after deleting John 3:16, %+v remain", remaining)
	}
}

// TestConfessionsForVerseMatchesRanges is the cross-link the feature exists for:
// a confession quoting "Psalm 103:2-3" must answer for verse 3 as well as 2.
func TestConfessionsForVerseMatchesRanges(t *testing.T) {
	d := dbtest.New(t)
	s := NewBibleStore(d)
	ctx := context.Background()

	seedPublishedConfession(t, d, "conf-1")
	if _, err := d.ExecContext(ctx,
		`INSERT INTO scripture_references (id,confession_id,book,chapter,verse,book_id,verse_start,verse_end,translation,sort_order)
		 VALUES ('ref-1','conf-1','Psalm',103,'2-3','Ps',2,3,'KJV',1)`); err != nil {
		t.Fatalf("seed reference: %v", err)
	}

	for _, verse := range []int{2, 3} {
		links, err := s.ConfessionsForVerse(ctx, "Ps", 103, verse)
		if err != nil {
			t.Fatalf("confessions for verse %d: %v", verse, err)
		}
		if len(links) != 1 {
			t.Fatalf("verse %d matched %d confessions, want 1", verse, len(links))
		}
		if links[0].ConfessionID != "conf-1" || links[0].CategorySlug == "" {
			t.Errorf("verse %d linked to %+v", verse, links[0])
		}
		if links[0].Reference != "Psalms 103:2-3" {
			t.Errorf("verse %d rendered the citation as %q", verse, links[0].Reference)
		}
	}

	// A verse the citation does not cover must not match.
	links, err := s.ConfessionsForVerse(ctx, "Ps", 103, 4)
	if err != nil {
		t.Fatalf("confessions for verse 4: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("Psalm 103:4 matched %d confessions, want none", len(links))
	}

	// The chapter view marks every verse the citation covers, in one query.
	chapterLinks, err := s.ConfessionsInChapter(ctx, "Ps", 103)
	if err != nil {
		t.Fatalf("chapter cross-links: %v", err)
	}
	if len(chapterLinks) != 2 || len(chapterLinks[2]) != 1 || len(chapterLinks[3]) != 1 {
		t.Errorf("chapter cross-links = %v, want verses 2 and 3 marked", chapterLinks)
	}

	// An unpublished confession is invisible to the reader.
	if _, err := d.ExecContext(ctx, `UPDATE confessions SET status='draft' WHERE id='conf-1'`); err != nil {
		t.Fatalf("unpublish: %v", err)
	}
	links, err = s.ConfessionsForVerse(ctx, "Ps", 103, 2)
	if err != nil {
		t.Fatalf("confessions after unpublish: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("a draft confession is still linked from Scripture: %+v", links)
	}
}

// TestVerseExistsGuardsAMarkAgainstAnUnknownCoordinate keeps a highlight from
// outliving its text: a mark on a verse a translation does not have would be a
// row no reader can ever be shown.
func TestVerseExistsGuardsAMarkAgainstAnUnknownCoordinate(t *testing.T) {
	s := NewBibleStore(dbtest.New(t))
	ctx := context.Background()
	seedVerse(t, s, "kjv", "Ps", 103, 2, "Bless the LORD")

	exists, err := s.VerseExists(ctx, "kjv", "Ps", 103, 2)
	if err != nil || !exists {
		t.Fatalf("VerseExists(Ps 103:2) = %v, %v", exists, err)
	}
	misses := []struct {
		version, book string
		chapter, v    int
	}{
		{"kjv", "Ps", 103, 3},
		{"kjv", "John", 3, 16},
		{"swahili", "Ps", 103, 2},
	}
	for _, m := range misses {
		got, err := s.VerseExists(ctx, m.version, m.book, m.chapter, m.v)
		if err != nil {
			t.Fatalf("VerseExists(%s %s %d:%d): %v", m.version, m.book, m.chapter, m.v, err)
		}
		if got {
			t.Errorf("VerseExists(%s %s %d:%d) = true, want false", m.version, m.book, m.chapter, m.v)
		}
	}
}
