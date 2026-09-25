package seed

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/bible"
)

// The fixture contract, tested where the corpus lives.
//
// internal/bible carries the fixture and the parser; this file carries the two
// checks that need both the fixture and the canonical corpus, which is why they
// live here rather than beside the parser: internal/seed imports internal/store,
// which imports internal/bible, so a test in that package cannot import the
// corpus without a cycle.
//
// The contract they enforce is the one that keeps the library and the Bible in
// step: every Scripture reference in every canonical confession resolves to a
// verse the fixture actually contains. Add a citation and the fixture has to
// grow with it, or the build stops.

// loadFixture parses the committed slice of the King James Version. It lives
// beside the corpus tests rather than in the parser's own package because the
// fixture is the corpus's contract with the Bible, and because only this
// package can see both sides of it.
const fixturePath = "../bible/testdata/cited-verses.osis.xml"

func loadFixture(t *testing.T) *bible.Translation {
	t.Helper()
	f, err := os.Open(fixturePath)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	tr, err := bible.Parse(f, bible.FormatOSIS, "kjv")
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return tr
}

func countChapters(tr *bible.Translation) int {
	n := 0
	for i := range tr.Books {
		n += len(tr.Books[i].Chapters)
	}
	return n
}

// TestFixtureCoverageMatchesTheCorpus derives the expected shape of the
// fixture from the corpus itself. Hard-coding counts here would let the
// fixture and the corpus drift apart while both tests kept passing.
func TestBibleFixtureCoverageMatchesTheCorpus(t *testing.T) {
	tr := loadFixture(t)

	type chapterKey struct {
		book    string
		chapter int
	}
	books := map[string]bool{}
	chapters := map[chapterKey]bool{}
	distinct := map[string]bool{}

	for _, c := range CanonicalConfessions {
		for _, s := range c.Scriptures {
			ref, err := bible.NewReference(s.Book, s.Chapter, s.Verse)
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
func TestEveryCorpusCitationResolvesAgainstTheFixture(t *testing.T) {
	tr := loadFixture(t)

	checked := 0
	for _, c := range CanonicalConfessions {
		if len(c.Scriptures) == 0 {
			t.Errorf("confession %q has no Scripture reference", c.Title)
		}
		for _, s := range c.Scriptures {
			// The version the corpus cites has to exist in the registry, or
			// the reader cannot open the verse the confession points at.
			v, ok := bible.VersionByID(strings.ToLower(strings.TrimSpace(s.Translation)))
			if !ok {
				t.Errorf("confession %q cites translation %q, which the registry does not ship",
					c.Title, s.Translation)
				continue
			}
			if !v.Default {
				t.Errorf("confession %q cites %s, but the fixture is generated from the default version %s",
					c.Title, v.ID, bible.DefaultVersion().ID)
			}

			ref, err := bible.NewReference(s.Book, s.Chapter, s.Verse)
			if err != nil {
				t.Errorf("confession %q cites %s %d:%s: %v", c.Title, s.Book, s.Chapter, s.Verse, err)
				continue
			}
			for _, n := range ref.Verses {
				if _, ok := tr.Text(ref.Book, ref.Chapter, n); !ok {
					t.Errorf("confession %q cites %s, absent from the %s fixture",
						c.Title, bible.DisplayRef(ref.Book, ref.Chapter, []int{n}), v.Abbrev)
				}
			}
			checked++
		}
	}
	if checked < 100 {
		t.Errorf("only %d citations checked; the corpus looks empty", checked)
	}
}

