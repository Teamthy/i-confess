package bible

import "testing"

func TestParseReferenceNormalizesCommonForms(t *testing.T) {
	tests := []struct {
		input, book         string
		chapter, start, end int
	}{
		{"John 3:16", "John", 3, 16, 16},
		{"Jn 3:16", "John", 3, 16, 16},
		{"JHN 3:16", "John", 3, 16, 16},
		{"JHN.3.16", "John", 3, 16, 16},
		{"PSA.23.1", "Ps", 23, 1, 1},
		{"1CO.13.4", "1Cor", 13, 4, 4},
		{"1Thess.5.11", "1Thess", 5, 11, 11},
		{"1TH.5.11", "1Thess", 5, 11, 11},
		{"2thess.1.3", "2Thess", 1, 3, 3},
		{"3John.1.4", "3John", 1, 4, 4},
		{"Romans 8", "Rom", 8, 0, 0},
		{"Rom 8:28", "Rom", 8, 28, 28},
		{"Psalm 23", "Ps", 23, 0, 0},
		{"Ps 23:1", "Ps", 23, 1, 1},
		{"1 Corinthians 13:4-7", "1Cor", 13, 4, 7},
		{"1 Cor 13:4–7", "1Cor", 13, 4, 7},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseReference(tt.input)
			if err != nil {
				t.Fatalf("ParseReference() error = %v", err)
			}
			if got.Book != tt.book || got.Chapter != tt.chapter || got.StartVerse != tt.start || got.EndVerse != tt.end {
				t.Fatalf("ParseReference() = %#v", got)
			}
		})
	}
}

func TestParseReferenceRejectsInvalidLocations(t *testing.T) {
	for _, input := range []string{"", "Yoruba 3:2", "John 0:1", "John 3:16-15", "John 4:999", "Revelation 23", "JHN.3.999", "JHN.3.16-17", "1Thess.5.99"} {
		if _, err := ParseReference(input); err == nil {
			t.Errorf("ParseReference(%q) unexpectedly succeeded", input)
		}
	}
}

func TestCanonicalVerseIDUsesStableUSFMIdentity(t *testing.T) {
	if got := CanonicalVerseID("John", 3, 16); got != "JHN.3.16" {
		t.Fatalf("got %q", got)
	}
	if got := CanonicalVerseID("Ps", 23, 1); got != "PSA.23.1" {
		t.Fatalf("got %q", got)
	}
	if got := CanonicalVerseID("unknown", 1, 1); got != "" {
		t.Fatalf("unknown book returned %q", got)
	}
}
