package bible

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func loadFixture(t *testing.T) *Translation {
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

func countChapters(tr *Translation) int {
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
