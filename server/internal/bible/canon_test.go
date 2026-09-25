package bible

import (
	"testing"
)

// The canon is the reference everything else is checked against, so it is
// tested as data: if a chapter count is edited by hand, this fails before the
// importer starts rejecting translations that were right.

func TestCanonShape(t *testing.T) {
	if len(Canon) != 66 {
		t.Fatalf("canon has %d books, want 66", len(Canon))
	}
	if got := CanonicalChapterCount(); got != 1189 {
		t.Errorf("canon has %d chapters, want 1189", got)
	}
	if got := ReferenceVerseCount(); got != 31102 {
		t.Errorf("reference distribution has %d verses, want the KJV's 31102", got)
	}

	var old, new int
	seenID := map[string]bool{}
	seenName := map[string]bool{}
	for i, b := range Canon {
		if b.ID == "" || b.Name == "" {
			t.Fatalf("book %d is missing an ID or a name", i)
		}
		if seenID[b.ID] {
			t.Errorf("duplicate book ID %q", b.ID)
		}
		if seenName[b.Name] {
			t.Errorf("duplicate book name %q", b.Name)
		}
		seenID[b.ID] = true
		seenName[b.Name] = true
		if len(b.Verses) == 0 {
			t.Errorf("%s has no chapters", b.ID)
		}
		for c, n := range b.Verses {
			if n <= 0 {
				t.Errorf("%s chapter %d has %d verses", b.ID, c+1, n)
			}
		}
		switch b.Testament {
		case Old:
			old++
		case New:
			new++
		default:
			t.Errorf("%s has unknown testament %q", b.ID, b.Testament)
		}
	}
	if old != 39 || new != 27 {
		t.Errorf("testament split is %d old / %d new, want 39 / 27", old, new)
	}
}

// TestCanonSpotChecks pins the chapters that transcription errors hit first:
// the longest chapter, the shortest book, and both ends of the canon.
func TestCanonSpotChecks(t *testing.T) {
	cases := []struct {
		book     string
		chapters int
	}{
		{"Gen", 50}, {"Ps", 150}, {"Isa", 66}, {"Obad", 1},
		{"3John", 1}, {"Jude", 1}, {"Rev", 22}, {"Matt", 28},
	}
	for _, c := range cases {
		b, ok := BookByID(c.book)
		if !ok {
			t.Fatalf("%s is not in the canon", c.book)
		}
		if b.Chapters() != c.chapters {
			t.Errorf("%s has %d chapters, want %d", c.book, b.Chapters(), c.chapters)
		}
	}
	ps, _ := BookByID("Ps")
	if got := ps.Verses[118]; got != 176 {
		t.Errorf("Psalm 119 has %d verses, want 176", got)
	}
	if got := ps.Verses[116]; got != 2 {
		t.Errorf("Psalm 117 has %d verses, want 2", got)
	}
	obad, _ := BookByID("Obad")
	if got := obad.Verses[0]; got != 21 {
		t.Errorf("Obadiah has %d verses, want 21", got)
	}
}

func TestBookOrderIsCanonical(t *testing.T) {
	for i, b := range Canon {
		if got := b.Order(); got != i+1 {
			t.Errorf("%s reports order %d, want %d", b.ID, got, i+1)
		}
	}
	if b, _ := BookByID("Gen"); b.Order() != 1 {
		t.Error("Genesis is not first")
	}
	if b, _ := BookByID("Rev"); b.Order() != 66 {
		t.Error("Revelation is not last")
	}
}

func TestParseBook(t *testing.T) {
	cases := map[string]string{
		// The spellings the canonical confession corpus actually uses.
		"Isaiah": "Isa", "Psalm": "Ps", "1 Peter": "1Pet", "2 Corinthians": "2Cor",
		"1 Thessalonians": "1Thess", "3 John": "3John", "1 Chronicles": "1Chr",
		"Song of Solomon": "Song", "Proverbs": "Prov", "Deuteronomy": "Deut",
		// Canonical IDs and full names.
		"Ps": "Ps", "1Cor": "1Cor", "Revelation": "Rev", "Psalms": "Ps",
		// USFM codes, which USFX sources carry.
		"PSA": "Ps", "JHN": "John", "1PE": "1Pet", "REV": "Rev", "SNG": "Song",
		// OSIS references with the chapter attached.
		"Ps.103": "Ps", "1Pet.2": "1Pet",
		// Case and spacing should not matter.
		"  isaiah ": "Isa", "1  peter": "1Pet",
		// Roman numerals, which older citations use.
		"II Timothy": "2Tim", "I John": "1John",
	}
	for in, want := range cases {
		got, ok := ParseBook(in)
		if !ok {
			t.Errorf("ParseBook(%q) failed", in)
			continue
		}
		if got != want {
			t.Errorf("ParseBook(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "Hezekiah", "NIV", "Gospel of Thomas"} {
		if got, ok := ParseBook(bad); ok {
			t.Errorf("ParseBook(%q) = %q, want failure", bad, got)
		}
	}
}

func TestParseVerseSpec(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"5", []int{5}},
		{" 24 ", []int{24}},
		{"2-3", []int{2, 3}},
		{"22-24", []int{22, 23, 24}},
		{"3-6", []int{3, 4, 5, 6}},
		{"9-10", []int{9, 10}},
		{"14b", []int{14}}, // sub-verse marker
		{"1-4", []int{1, 2, 3, 4}},
		{"17", []int{17}},
	}
	for _, c := range cases {
		got, ok := ParseVerseSpec(c.in)
		if !ok {
			t.Errorf("ParseVerseSpec(%q) failed", c.in)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("ParseVerseSpec(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("ParseVerseSpec(%q) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
	for _, bad := range []string{"", "  ", "abc", "3-1", "1-2-3", "-4"} {
		if got, ok := ParseVerseSpec(bad); ok {
			t.Errorf("ParseVerseSpec(%q) = %v, want failure", bad, got)
		}
	}
}

func TestNewReference(t *testing.T) {
	ref, err := NewReference("1 Peter", 2, "24")
	if err != nil {
		t.Fatalf("NewReference: %v", err)
	}
	if ref.Book != "1Pet" || ref.Chapter != 2 || len(ref.Verses) != 1 || ref.Verses[0] != 24 {
		t.Errorf("unexpected reference %+v", ref)
	}
	if ref.Display != "1 Peter 2:24" {
		t.Errorf("Display = %q, want %q", ref.Display, "1 Peter 2:24")
	}

	ranged, err := NewReference("Psalm", 103, "2-3")
	if err != nil {
		t.Fatalf("NewReference: %v", err)
	}
	if ranged.Display != "Psalms 103:2-3" {
		t.Errorf("Display = %q, want %q", ranged.Display, "Psalms 103:2-3")
	}

	// A reference outside the canon must be refused rather than stored: it
	// would produce a deep link that no client can repair.
	bad := []struct {
		book    string
		chapter int
		verse   string
	}{
		{"Genesis", 51, "1"},   // Genesis has 50 chapters
		{"Jude", 1, "26"},      // Jude has 25 verses
		{"Psalms", 119, "177"}, // Psalm 119 has 176
		{"Hezekiah", 1, "1"},   // not a book
		{"Genesis", 1, ""},     // no verse
	}
	for _, b := range bad {
		if _, err := NewReference(b.book, b.chapter, b.verse); err == nil {
			t.Errorf("NewReference(%s %d:%s) succeeded, want an error", b.book, b.chapter, b.verse)
		}
	}
}

func TestDisplayRefCollapsesOnlyContiguousRuns(t *testing.T) {
	if got := DisplayRef("Ps", 103, []int{2, 3}); got != "Psalms 103:2-3" {
		t.Errorf("contiguous run rendered %q", got)
	}
	if got := DisplayRef("Ps", 103, []int{2, 5}); got != "Psalms 103:2,5" {
		t.Errorf("scattered verses rendered %q", got)
	}
	if got := DisplayRef("Ps", 103, nil); got != "Psalms 103" {
		t.Errorf("chapter-only reference rendered %q", got)
	}
	if got := DisplayRef("nope", 1, []int{1}); got != "nope 1:1" {
		t.Errorf("unknown book rendered %q", got)
	}
}
