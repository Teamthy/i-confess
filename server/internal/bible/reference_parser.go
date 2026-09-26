package bible

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Reference is a normalized scripture location. Book is the stable OSIS book
// identifier; StartVerse/EndVerse are zero only for a whole-chapter reference.
type Reference struct {
	Book      string `json:"book_id"`
	Chapter   int    `json:"chapter"`
	StartVerse int   `json:"start_verse,omitempty"`
	EndVerse   int   `json:"end_verse,omitempty"`
}

var referencePattern = regexp.MustCompile(`(?i)^(.+?)\s+(\d+)(?::(\d+)(?:\s*[-–—]\s*(\d+))?)?$`)

// ParseReference supports full names, common abbreviations and canonical/USFM
// IDs already understood by ParseBook: "John 3:16", "Jn 3:16", "JHN 3:16",
// "Romans 8", "Psalm 23", and single-chapter verse ranges.
func ParseReference(input string) (Reference, error) {
	match := referencePattern.FindStringSubmatch(strings.TrimSpace(input))
	if match == nil {
		return Reference{}, fmt.Errorf("invalid Bible reference")
	}
	bookID, ok := ParseBook(match[1])
	if !ok {
		return Reference{}, fmt.Errorf("unknown Bible book")
	}
	chapter, err := strconv.Atoi(match[2])
	if err != nil || chapter < 1 {
		return Reference{}, fmt.Errorf("invalid chapter")
	}
	book, _ := BookByID(bookID)
	if chapter > book.Chapters() {
		return Reference{}, fmt.Errorf("chapter is outside the canonical book")
	}
	ref := Reference{Book: bookID, Chapter: chapter}
	if match[3] == "" {
		return ref, nil
	}
	start, err := strconv.Atoi(match[3])
	if err != nil || start < 1 {
		return Reference{}, fmt.Errorf("invalid verse")
	}
	end := start
	if match[4] != "" {
		end, err = strconv.Atoi(match[4])
		if err != nil || end < start || end-start > 199 {
			return Reference{}, fmt.Errorf("invalid verse range")
		}
	}
	if start > book.Verses[chapter-1] || end > book.Verses[chapter-1] {
		return Reference{}, fmt.Errorf("verse is outside the canonical chapter")
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
