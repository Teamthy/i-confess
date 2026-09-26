package bible

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/seed"
)

// Fixture tests.
//
// The repository does not carry Bible text - twelve translations are a quarter
// of a million verses and hundreds of megabytes, and shipping them in Git
// would make every clone and every diff pay for data the application loads
// from a database. What the repository does carry is this fixture: the verses
// the canonical confession corpus actually cites, sliced out of the King James
// Version, which is the translation those confessions quote.
//
// That makes the fixture a contract. If a confession cites a verse the fixture
// does not contain, one of two things is true: the fixture is stale, or the
// new citation does not exist. Both should stop a pull request, and
// TestEveryCorpusCitationResolves does exactly that.

const fixturePath = "testdata/cited-verses.osis.xml"

func loadFixture(t *testing.T) *ParsedTranslation {
	t.Helper()
	f, err := os.Open(fixturePath)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	tr, err := Parse(f, FormatOSIS, "kjv")
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return tr
}

// TestFixtureParsesWithTheHardDialect checks the fixture against the parser's
// most failure-prone path: milestone verses (sID/eID) rather than containers,
// which is how the real KJV file is written. A parser that returns one word
// per verse, or verses with no text at all, fails here.
func TestFixtureParsesWithTheHardDialect(t *testing.T) {
	tr := loadFixture(t)

	if tr.Milestones == 0 {
		t.Fatal("fixture parsed with no milestone verses; the milestone path is untested")
	}
	if tr.Containers != 0 {
		t.Errorf("fixture reported %d container verses, want 0", tr.Containers)
	}

	// A verse whose text is split across several milestones must come out
	// whole. Psalm 103:2 is emitted as one element here, so a regression in
	// merging shows up as a truncated string rather than a missing verse.
	text, ok := tr.Text("Ps", 103, 2)
	if !ok {
		t.Fatal("Psalm 103:2 is missing from the fixture")
	}
	if !strings.Contains(text, "Bless the LORD") || !strings.Contains(text, "benefits") {
		t.Errorf("Psalm 103:2 parsed as %q", text)
	}

	// Text must never carry markup or footnote fragments into a verse.
	switch {
	case strings.Contains(text, "<"), strings.Contains(text, ">"):
		t.Errorf("Psalm 103:2 contains markup: %q", text)
	case strings.Contains(text, "NU reads"):
		t.Errorf("Psalm 103:2 contains footnote text: %q", text)
	}

	// The fixture deliberately contains a front-matter division. It is not
	// Scripture and must not be imported under a book of its own.
	if tr.Book("FrontMatter") != nil {
		t.Error("the front-matter division was imported as a book")
	}
	if !containsString(tr.Skipped, "FrontMatter") {
		t.Errorf("front matter was not reported as skipped; skipped = %v", tr.Skipped)
	}
	if _, ok := tr.Text("FrontMatter", 1, 1); ok {
		t.Error("front-matter text is readable as Scripture")
	}
}

// TestFixtureCoverageMatchesTheCorpus derives the expected shape of the
// fixture from the corpus itself. Hard-coding counts here would let the
// fixture and the corpus drift apart while both tests kept passing.
func TestFixtureCoverageMatchesTheCorpus(t *testing.T) {
	tr := loadFixture(t)

	type chapterKey struct {
		book    string
		chapter int
	}
	books := map[string]bool{}
	chapters := map[chapterKey]bool{}
	distinct := map[string]bool{}

	for _, c := range seed.CanonicalConfessions {
		for _, s := range c.Scriptures {
			ref, err := NewReference(s.Book, s.Chapter, s.Verse)
			if err != nil {
				t.Fatalf("confession %q cites %s %d:%s: %v", c.Title, s.Book, s.Chapter, s.Verse, err)
			}
			books[ref.Book] = true
			chapters[chapterKey{ref.Book, ref.Chapter}] = true
			for _, n := range ref.Verses {
				text, ok := tr.Text(ref.Book, ref.Chapter, n)
				if !ok {
					t.Errorf("%s %d:%d is cited by %q but absent from the fixture; regenerate it with "+
						"`go run ./cmd/bible-import fixture`", ref.Name, ref.Chapter, n, c.Title)
					continue
				}
				if strings.TrimSpace(text) == "" {
					t.Errorf("%s %d:%d parsed as empty text", ref.Name, ref.Chapter, n)
				}
				distinct[fmt.Sprintf("%s.%d.%d", ref.Book, ref.Chapter, n)] = true
			}
		}
	}

	if got := len(tr.Books); got != len(books) {
		t.Errorf("fixture has %d books, corpus cites %d", got, len(books))
	}
	if got := countChapters(tr); got != len(chapters) {
		t.Errorf("fixture has %d chapters, corpus cites %d", got, len(chapters))
	}
	if got := tr.VerseCount(); got != len(distinct) {
		t.Errorf("fixture has %d verses, the corpus cites %d distinct verses", got, len(distinct))
	}
}

// TestEveryCorpusCitationResolves is the guard that keeps the confession
// library and the Bible in step: every Scripture reference in the canonical
// corpus must normalise, and the translation it names must be one the
// registry ships.
func TestEveryCorpusCitationResolves(t *testing.T) {
	tr := loadFixture(t)

	checked := 0
	for _, c := range seed.CanonicalConfessions {
		if len(c.Scriptures) == 0 {
			t.Errorf("confession %q has no Scripture reference", c.Title)
		}
		for _, s := range c.Scriptures {
			// The version the corpus cites has to exist in the registry, or
			// the reader cannot open the verse the confession points at.
			v, ok := VersionByID(strings.ToLower(strings.TrimSpace(s.Translation)))
			if !ok {
				t.Errorf("confession %q cites translation %q, which the registry does not ship",
					c.Title, s.Translation)
				continue
			}
			if !v.Default {
				t.Errorf("confession %q cites %s, but the fixture is generated from the default version %s",
					c.Title, v.ID, DefaultVersion().ID)
			}

			ref, err := NewReference(s.Book, s.Chapter, s.Verse)
			if err != nil {
				t.Errorf("confession %q cites %s %d:%s: %v", c.Title, s.Book, s.Chapter, s.Verse, err)
				continue
			}
			for _, n := range ref.Verses {
				if _, ok := tr.Text(ref.Book, ref.Chapter, n); !ok {
					t.Errorf("confession %q cites %s, absent from the %s fixture",
						c.Title, DisplayRef(ref.Book, ref.Chapter, []int{n}), v.Abbrev)
				}
			}
			checked++
		}
	}
	if checked < 100 {
		t.Errorf("only %d citations checked; the corpus looks empty", checked)
	}
}

// TestFixtureTextIsVerbatim compares the fixture against the full KJV source
// when it is available on disk. It is skipped otherwise, so CI can run
// without the 10MB corpus while an operator who has fetched the sources gets
// the stronger check that the slice was not edited in transit.
func TestFixtureTextIsVerbatim(t *testing.T) {
	dir := os.Getenv("BIBLE_SOURCES")
	if dir == "" {
		t.Skip("BIBLE_SOURCES is not set; skipping comparison against the full KJV source")
	}
	v := DefaultVersion()
	f, err := os.Open(filepath.Join(dir, v.SourceFile))
	if err != nil {
		t.Skipf("KJV source not present in BIBLE_SOURCES: %v", err)
	}
	defer f.Close()
	full, err := Parse(f, v.Format, v.ID)
	if err != nil {
		t.Fatalf("parse %s: %v", v.SourceFile, err)
	}

	tr := loadFixture(t)
	compared := 0
	for i := range tr.Books {
		b := &tr.Books[i]
		for j := range b.Chapters {
			ch := &b.Chapters[j]
			for _, vr := range ch.Verses {
				want, ok := full.Text(b.ID, ch.Number, vr.Number)
				if !ok {
					t.Errorf("%s %d:%d is in the fixture but not in the source", b.ID, ch.Number, vr.Number)
					continue
				}
				if want != vr.Text {
					t.Errorf("%s %d:%d differs from the source:\n fixture: %q\n source:  %q",
						b.ID, ch.Number, vr.Number, vr.Text, want)
				}
				compared++
			}
		}
	}
	if compared == 0 {
		t.Fatal("no verses compared")
	}
	t.Logf("compared %d verse texts against %s", compared, v.SourceFile)
}

func countChapters(tr *ParsedTranslation) int {
	n := 0
	for i := range tr.Books {
		n += len(tr.Books[i].Chapters)
	}
	return n
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if strings.Contains(x, want) {
			return true
		}
	}
	return false
}
