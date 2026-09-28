package bible

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// The structure is navigation data three clients draw from, so it is tested as
// data: a book that loses its section, an abbreviation that goes missing or a
// section table that drifts out of step with the canon would each show up as a
// hole in a book list rather than as a failing request.

func TestStructureCoversTheCanon(t *testing.T) {
	s := CanonStructure()

	if s.BookCount != 66 || len(s.Books) != 66 {
		t.Fatalf("structure has %d books (BookCount %d), want 66", len(s.Books), s.BookCount)
	}
	if s.ChapterCount != 1189 {
		t.Errorf("structure has %d chapters, want 1189", s.ChapterCount)
	}
	if s.VerseCount != 31102 {
		t.Errorf("structure has %d reference verses, want the KJV's 31102", s.VerseCount)
	}

	chapters, verses := 0, 0
	for i, b := range s.Books {
		if b.CanonicalOrder != i+1 {
			t.Errorf("%s is at index %d but reports order %d", b.ID, i, b.CanonicalOrder)
		}
		if b.Section == "" {
			t.Errorf("%s has no section", b.ID)
		}
		if b.Abbreviation == "" {
			t.Errorf("%s has no abbreviation", b.ID)
		}
		if b.USFM == "" {
			t.Errorf("%s has no USFM code", b.ID)
		}
		if b.ChapterCount != len(b.Chapters) {
			t.Errorf("%s reports %d chapters but carries %d verse counts", b.ID, b.ChapterCount, len(b.Chapters))
		}
		sum := 0
		for _, n := range b.Chapters {
			sum += n
		}
		if sum != b.VerseCount {
			t.Errorf("%s verse count %d does not match its chapters (%d)", b.ID, b.VerseCount, sum)
		}
		// Every book has to be openable by the name a reader types.
		if id, ok := ParseBook(b.Name); !ok || id != b.ID {
			t.Errorf("%s is not resolvable by its own name %q", b.ID, b.Name)
		}
		chapters += b.ChapterCount
		verses += b.VerseCount
	}
	if chapters != s.ChapterCount || verses != s.VerseCount {
		t.Errorf("book totals (%d chapters, %d verses) disagree with the structure header (%d, %d)",
			chapters, verses, s.ChapterCount, s.VerseCount)
	}
}

func TestStructureTestamentsAndSectionsPartitionTheCanon(t *testing.T) {
	s := CanonStructure()

	if len(s.Testaments) != 2 {
		t.Fatalf("structure has %d testaments, want 2", len(s.Testaments))
	}
	seen := map[string]int{}
	books, chapters := 0, 0
	for _, testament := range s.Testaments {
		if testament.BookCount == 0 || testament.ChapterCount == 0 {
			t.Errorf("%s is empty", testament.ID)
		}
		sectionBooks := 0
		for _, section := range testament.Sections {
			if section.Testament != testament.ID {
				t.Errorf("section %s sits under %s but claims %s", section.ID, testament.ID, section.Testament)
			}
			if section.BookCount != len(section.BookIDs) {
				t.Errorf("section %s reports %d books but lists %d", section.ID, section.BookCount, len(section.BookIDs))
			}
			for _, id := range section.BookIDs {
				seen[id]++
			}
			sectionBooks += section.BookCount
		}
		if sectionBooks != testament.BookCount {
			t.Errorf("%s sections hold %d books, testament reports %d", testament.ID, sectionBooks, testament.BookCount)
		}
		books += testament.BookCount
		chapters += testament.ChapterCount
	}
	if books != 66 {
		t.Errorf("testaments hold %d books, want 66", books)
	}
	if chapters != 1189 {
		t.Errorf("testaments hold %d chapters, want 1189", chapters)
	}
	// Exactly one section per book: no book missing, none listed twice.
	for _, b := range s.Books {
		if seen[b.ID] != 1 {
			t.Errorf("%s appears in %d sections, want exactly 1", b.ID, seen[b.ID])
		}
	}
	if s.Testaments[0].BookCount != 39 || s.Testaments[1].BookCount != 27 {
		t.Errorf("testament split is %d/%d, want 39/27", s.Testaments[0].BookCount, s.Testaments[1].BookCount)
	}
}

func TestStructureAliasesResolveLikeTheParser(t *testing.T) {
	s := CanonStructure()
	if len(s.Aliases) < len(s.Books)*2 {
		t.Fatalf("alias table has only %d entries for %d books", len(s.Aliases), len(s.Books))
	}
	// The published table is what a client-side parser keys on, so every
	// entry in it must resolve to the same book the server's parser picks.
	for key, id := range s.Aliases {
		got, ok := ParseBook(key)
		if !ok || got != id {
			t.Errorf("alias %q published as %s but ParseBook returned %q (ok=%v)", key, id, got, ok)
		}
	}
	// The spellings a reader actually types, normalised the way a client must.
	for _, c := range []struct{ input, want string }{
		{"John", "John"}, {"jn", "John"}, {"JHN", "John"},
		{"1 Corinthians", "1Cor"}, {"1 Cor", "1Cor"}, {"1cor", "1Cor"},
		{"Psalm", "Ps"}, {"Psalms", "Ps"}, {"ps", "Ps"},
		{"Song of Solomon", "Song"}, {"II Timothy", "2Tim"},
	} {
		if id, ok := s.Aliases[NormalizeBookKey(c.input)]; !ok || id != c.want {
			t.Errorf("%q normalises to %q which maps to %q (ok=%v), want %s",
				c.input, NormalizeBookKey(c.input), id, ok, c.want)
		}
	}
}

func TestStructureVerseBoundsMatchTheReferenceDistribution(t *testing.T) {
	// The bounds a client validates a typed reference against.
	for _, c := range []struct {
		book    string
		chapter int
		verses  int
	}{
		{"Ps", 119, 176}, {"John", 3, 36}, {"Gen", 1, 31}, {"Rev", 22, 21}, {"3John", 1, 14},
	} {
		got, ok := ReferenceVerseCountFor(c.book, c.chapter)
		if !ok || got != c.verses {
			t.Errorf("%s %d has %d reference verses (ok=%v), want %d", c.book, c.chapter, got, ok, c.verses)
		}
	}
	if _, ok := ReferenceVerseCountFor("John", 22); ok {
		t.Error("John 22 resolved, but John has 21 chapters")
	}
	if _, ok := ReferenceVerseCountFor("Nope", 1); ok {
		t.Error("an unknown book resolved")
	}
}

func TestUSFMCodesRoundTripToCanonicalVerseIDs(t *testing.T) {
	for _, b := range CanonStructure().Books {
		code, ok := USFMCode(b.ID)
		if !ok || code != b.USFM {
			t.Errorf("%s publishes USFM %q but USFMCode returned %q (ok=%v)", b.ID, b.USFM, code, ok)
		}
		if id := CanonicalVerseID(b.ID, 1, 1); id != code+".1.1" {
			t.Errorf("%s canonical verse ID is %q, want %q", b.ID, id, code+".1.1")
		}
	}
	if got := CanonicalVerseID("John", 3, 16); got != "JHN.3.16" {
		t.Errorf("John 3:16 canonical ID is %q, want JHN.3.16", got)
	}
}

// The web app reads the canon from a generated file rather than the API so the
// reader can draw a complete Bible before its first request resolves. That
// file is only trustworthy if it cannot drift, so it is compared here.
func TestWebCanonJSONIsCurrent(t *testing.T) {
	const path = "../../../web/lib/canon.json"
	checkedIn, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	generated, err := StructureJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, checkedIn) {
		t.Fatalf("%s is stale; regenerate it with:\n\tcd server && go run ./cmd/bible-structure ../web/lib/canon.json", path)
	}
}

// Every registry entry must stand on its own: a reader seeing a version
// listed but not imported here needs the full SHA-256 of the source file it
// was reviewed from, the file itself, the attribution and the licence —
// provenance is the whole point of publishing the registry.
func TestRegistryEntriesCarryFullProvenance(t *testing.T) {
	reg := PublishedRegistry()
	if reg.VersionCount != len(reg.Versions) {
		t.Errorf("VersionCount is %d but the document lists %d versions", reg.VersionCount, len(reg.Versions))
	}
	if reg.LanguageCount != len(Languages()) {
		t.Errorf("LanguageCount is %d but Languages() reports %d", reg.LanguageCount, len(Languages()))
	}
	if reg.VersionCount == 0 {
		t.Fatal("published registry is empty")
	}
	for _, e := range reg.Versions {
		if len(e.SHA256) != 64 {
			t.Errorf("%s: sha256 is %d characters, want 64", e.ID, len(e.SHA256))
		}
		if e.SourceFile == "" {
			t.Errorf("%s: missing source file", e.ID)
		}
		if e.Attribution == "" {
			t.Errorf("%s: missing attribution", e.ID)
		}
		if e.Licence == "" {
			t.Errorf("%s: missing licence", e.ID)
		}
	}
}

// web/lib/versions.json feeds the translations page's "reviewed registry"
// section. Two generators legitimately produce it — Go's RegistryJSON and
// scripts/gen-bible-registry.py, which parses registry.go directly so the web
// app can regenerate it without a Go toolchain — so the file is compared by
// decoded content, not by bytes.
func TestWebRegistryJSONIsCurrent(t *testing.T) {
	const path = "../../../web/lib/versions.json"
	checkedIn, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var document RegistryDocument
	if err := json.Unmarshal(checkedIn, &document); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if !reflect.DeepEqual(document, PublishedRegistry()) {
		t.Fatalf("%s no longer matches PublishedRegistry(); regenerate it with:\n\tmake bible-registry\nor:\n\tcd server && go run ./cmd/bible-registry ../web/lib/versions.json", path)
	}
}
