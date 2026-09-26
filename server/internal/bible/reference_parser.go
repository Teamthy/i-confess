package bible

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PassageReference is a normalized scripture location. Book is the stable OSIS book
// identifier; StartVerse/EndVerse are zero only for a whole-chapter reference.
type PassageReference struct {
	Book       string `json:"book_id"`
	Chapter    int    `json:"chapter"`
	StartVerse int    `json:"start_verse,omitempty"`
	EndVerse   int    `json:"end_verse,omitempty"`
}

var referencePattern = regexp.MustCompile(`(?i)^(.+?)\s+(\d+)(?::(\d+)(?:\s*[-–—]\s*(\d+))?)?$`)

// Accept USFM codes and the longest canonical OSIS book IDs (e.g. 1Thess).
var canonicalVersePattern = regexp.MustCompile(`(?i)^([1-3]?[a-z]{2,5})\.(\d+)\.(\d+)$`)

// ParseReference supports full names, common abbreviations, and USFM IDs:
// "John 3:16", "JHN 3:16", the deep-link identity "JHN.3.16",
// "Romans 8", "Psalm 23", and single-chapter verse ranges.
func ParseReference(input string) (PassageReference, error) {
	input = strings.TrimSpace(input)
	if canonical := canonicalVersePattern.FindStringSubmatch(input); canonical != nil {
		input = canonical[1] + " " + canonical[2] + ":" + canonical[3]
	}
	match := referencePattern.FindStringSubmatch(input)
	if match == nil {
		return PassageReference{}, fmt.Errorf("invalid Bible reference")
	}
	bookID, ok := ParseBook(match[1])
	if !ok {
		return PassageReference{}, fmt.Errorf("unknown Bible book")
	}
	chapter, err := strconv.Atoi(match[2])
	if err != nil || chapter < 1 {
		return PassageReference{}, fmt.Errorf("invalid chapter")
	}
	book, _ := BookByID(bookID)
	if chapter > book.Chapters() {
		return PassageReference{}, fmt.Errorf("chapter is outside the canonical book")
	}
	ref := PassageReference{Book: bookID, Chapter: chapter}
	if match[3] == "" {
		return ref, nil
	}
	start, err := strconv.Atoi(match[3])
	if err != nil || start < 1 {
		return PassageReference{}, fmt.Errorf("invalid verse")
	}
	end := start
	if match[4] != "" {
		end, err = strconv.Atoi(match[4])
		if err != nil || end < start || end-start > 199 {
			return PassageReference{}, fmt.Errorf("invalid verse range")
		}
	}
	if start > book.Verses[chapter-1] || end > book.Verses[chapter-1] {
		return PassageReference{}, fmt.Errorf("verse is outside the canonical chapter")
	}
	ref.StartVerse, ref.EndVerse = start, end
	return ref, nil
}

// CanonicalVerseID returns a provider-independent ID like JHN.3.16.
func CanonicalVerseID(bookID string, chapter, verse int) string {
	code := ""
	for usfmCode, osisID := range usfm {
		if osisID == bookID {
			code = usfmCode
			break
		}
	}
	if code == "" {
		return ""
	}
	return fmt.Sprintf("%s.%d.%d", code, chapter, verse)
}
