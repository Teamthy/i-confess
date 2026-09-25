package bible

// canon.go — the shape of the Bible the platform validates against.
//
// Book IDs are OSIS identifiers ("Gen", "1Cor", "Rev") because every source
// format the importer reads already carries them: OSIS uses them natively,
// USFX uses USFM codes that map one-to-one, and Zefania files identify books
// by number. A single canonical ID means a verse reference written by a
// confession, a highlight row and an API path all name the same thing.
//
// Chapter counts are canonical — every Protestant 66-book Bible has 1,189 of
// them, and a file that disagrees is wrong rather than different. Verse counts
// are NOT canonical: translations legitimately differ (the KJV's 31,102 verses
// against the ASV's 31,086), so Verses is a reference distribution taken from
// the KJV and the validator reports deviations instead of failing on them.
// That distinction is what the verification report in docs/51 records per
// version.

// Testament values.
const (
	Old = "old"
	New = "new"
)

// Book is one canonical book with its reference verse distribution.
type Book struct {
	ID        string
	Name      string
	Testament string
	// Verses holds the reference verse count per chapter, indexed from
	// chapter 1. Its length is the book's chapter count.
	Verses []int
}

// Chapters returns the number of chapters in the book.
func (b Book) Chapters() int { return len(b.Verses) }

// Order returns the book's 1-based position in the canon. It is the order
// everything displays and sorts by, so a translation's books, a reader's book
// list and a canonical-order column can never disagree about what comes first.
func (b Book) Order() int {
	for i := range Canon {
		if Canon[i].ID == b.ID {
			return i + 1
		}
	}
	return 0
}

// Canon is the 66 books in order, with KJV reference verse counts.
var Canon = []Book{
	{ID: "Gen", Name: "Genesis", Testament: Old, Chapters: {31, 25, 24, 26, 32, 22, 24, 22, 29, 32, 32, 20, 18, 24, 21, 16, 27, 33, 38, 18, 34, 24, 20, 67, 34, 35, 46, 22, 35, 43, 55, 32, 20, 31, 29, 43, 36, 30, 23, 23, 57, 38, 34, 34, 28, 34, 31, 22, 33, 26}},
	{ID: "Exod", Name: "Exodus", Testament: Old, Chapters: {22, 25, 22, 31, 23, 30, 25, 32, 35, 29, 10, 51, 22, 31, 27, 36, 16, 27, 25, 26, 36, 31, 33, 18, 40, 37, 21, 43, 46, 38, 18, 35, 23, 35, 35, 38, 29, 31, 43, 38}},
	{ID: "Lev", Name: "Leviticus", Testament: Old, Chapters: {17, 16, 17, 35, 19, 30, 38, 36, 24, 20, 47, 8, 59, 57, 33, 34, 16, 30, 37, 27, 24, 33, 44, 23, 55, 46, 34}},
	{ID: "Num", Name: "Numbers", Testament: Old, Chapters: {54, 34, 51, 49, 31, 27, 89, 26, 23, 36, 35, 16, 33, 45, 41, 50, 13, 32, 22, 29, 35, 41, 30, 25, 18, 65, 23, 31, 40, 16, 54, 42, 56, 29, 34, 13}},
	{ID: "Deut", Name: "Deuteronomy", Testament: Old, Chapters: {46, 37, 29, 49, 33, 25, 26, 20, 29, 22, 32, 32, 18, 29, 23, 22, 20, 22, 21, 20, 23, 30, 25, 22, 19, 19, 26, 68, 29, 20, 30, 52, 29, 12}},
	{ID: "Josh", Name: "Joshua", Testament: Old, Chapters: {18, 24, 17, 24, 15, 27, 26, 35, 27, 43, 23, 24, 33, 15, 63, 10, 18, 28, 51, 9, 45, 34, 16, 33}},
	{ID: "Judg", Name: "Judges", Testament: Old, Chapters: {36, 23, 31, 24, 31, 40, 25, 35, 57, 18, 40, 15, 25, 20, 20, 31, 13, 31, 30, 48, 25}},
	{ID: "Ruth", Name: "Ruth", Testament: Old, Chapters: {22, 23, 18, 22}},
	{ID: "1Sam", Name: "1 Samuel", Testament: Old, Chapters: {28, 36, 21, 22, 12, 21, 17, 22, 27, 27, 15, 25, 23, 52, 35, 23, 58, 30, 24, 42, 15, 23, 29, 22, 44, 25, 12, 25, 11, 31, 13}},
	{ID: "2Sam", Name: "2 Samuel", Testament: Old, Chapters: {27, 32, 39, 12, 25, 23, 29, 18, 13, 19, 27, 31, 39, 33, 37, 23, 29, 33, 43, 26, 22, 51, 39, 25}},
	{ID: "1Kgs", Name: "1 Kings", Testament: Old, Chapters: {53, 46, 28, 34, 18, 38, 51, 66, 28, 29, 43, 33, 34, 31, 34, 34, 24, 46, 21, 43, 29, 53}},
	{ID: "2Kgs", Name: "2 Kings", Testament: Old, Chapters: {18, 25, 27, 44, 27, 33, 20, 29, 37, 36, 21, 21, 25, 29, 38, 20, 41, 37, 37, 21, 26, 20, 37, 20, 30}},
	{ID: "1Chr", Name: "1 Chronicles", Testament: Old, Chapters: {54, 55, 24, 43, 26, 81, 40, 40, 44, 14, 47, 40, 14, 17, 29, 43, 27, 17, 19, 8, 30, 19, 32, 31, 31, 32, 34, 21, 30}},
	{ID: "2Chr", Name: "2 Chronicles", Testament: Old, Chapters: {17, 18, 17, 22, 14, 42, 22, 18, 31, 19, 23, 16, 22, 15, 19, 14, 19, 34, 11, 37, 20, 12, 21, 27, 28, 23, 9, 27, 36, 27, 21, 33, 25, 33, 27, 23}},
	{ID: "Ezra", Name: "Ezra", Testament: Old, Chapters: {11, 70, 13, 24, 17, 22, 28, 36, 15, 44}},
	{ID: "Neh", Name: "Nehemiah", Testament: Old, Chapters: {11, 20, 32, 23, 19, 19, 73, 18, 38, 39, 36, 47, 31}},
	{ID: "Esth", Name: "Esther", Testament: Old, Chapters: {22, 23, 15, 17, 14, 14, 10, 17, 32, 3}},
	{ID: "Job", Name: "Job", Testament: Old, Chapters: {22, 13, 26, 21, 27, 30, 21, 22, 35, 22, 20, 25, 28, 22, 35, 22, 16, 21, 29, 29, 34, 30, 17, 25, 6, 14, 23, 28, 25, 31, 40, 22, 33, 37, 16, 33, 24, 41, 30, 24, 34, 17}},
	{ID: "Ps", Name: "Psalms", Testament: Old, Chapters: {6, 12, 8, 8, 12, 10, 17, 9, 20, 18, 7, 8, 6, 7, 5, 11, 15, 50, 14, 9, 13, 31, 6, 10, 22, 12, 14, 9, 11, 12, 24, 11, 22, 22, 28, 12, 40, 22, 13, 17, 13, 11, 5, 26, 17, 11, 9, 14, 20, 23, 19, 9, 6, 7, 23, 13, 11, 11, 17, 12, 8, 12, 11, 10, 13, 20, 7, 35, 36, 5, 24, 20, 28, 23, 10, 12, 20, 72, 13, 19, 16, 8, 18, 12, 13, 17, 7, 18, 52, 17, 16, 15, 5, 23, 11, 13, 12, 9, 9, 5, 8, 28, 22, 35, 45, 48, 43, 13, 31, 7, 10, 10, 9, 8, 18, 19, 2, 29, 176, 7, 8, 9, 4, 8, 5, 6, 5, 6, 8, 8, 3, 18, 3, 3, 21, 26, 9, 8, 24, 13, 10, 7, 12, 15, 21, 10, 20, 14, 9, 6}},
	{ID: "Prov", Name: "Proverbs", Testament: Old, Chapters: {33, 22, 35, 27, 23, 35, 27, 36, 18, 32, 31, 28, 25, 35, 33, 33, 28, 24, 29, 30, 31, 29, 35, 34, 28, 28, 27, 28, 27, 33, 31}},
	{ID: "Eccl", Name: "Ecclesiastes", Testament: Old, Chapters: {18, 26, 22, 16, 20, 12, 29, 17, 18, 20, 10, 14}},
	{ID: "Song", Name: "Song of Solomon", Testament: Old, Chapters: {17, 17, 11, 16, 16, 13, 13, 14}},
	{ID: "Isa", Name: "Isaiah", Testament: Old, Chapters: {31, 22, 26, 6, 30, 13, 25, 22, 21, 34, 16, 6, 22, 32, 9, 14, 14, 7, 25, 6, 17, 25, 18, 23, 12, 21, 13, 29, 24, 33, 9, 20, 24, 17, 10, 22, 38, 22, 8, 31, 29, 25, 28, 28, 25, 13, 15, 22, 26, 11, 23, 15, 12, 17, 13, 12, 21, 14, 21, 22, 11, 12, 19, 12, 25, 24}},
	{ID: "Jer", Name: "Jeremiah", Testament: Old, Chapters: {19, 37, 25, 31, 31, 30, 34, 22, 26, 25, 23, 17, 27, 22, 21, 21, 27, 23, 15, 18, 14, 30, 40, 10, 38, 24, 22, 17, 32, 24, 40, 44, 26, 22, 19, 32, 21, 28, 18, 16, 18, 22, 13, 30, 5, 28, 7, 47, 39, 46, 64, 34}},
	{ID: "Lam", Name: "Lamentations", Testament: Old, Chapters: {22, 22, 66, 22, 22}},
	{ID: "Ezek", Name: "Ezekiel", Testament: Old, Chapters: {28, 10, 27, 17, 17, 14, 27, 18, 11, 22, 25, 28, 23, 23, 8, 63, 24, 32, 14, 49, 32, 31, 49, 27, 17, 21, 36, 26, 21, 26, 18, 32, 33, 31, 15, 38, 28, 23, 29, 49, 26, 20, 27, 31, 25, 24, 23, 35}},
	{ID: "Dan", Name: "Daniel", Testament: Old, Chapters: {21, 49, 30, 37, 31, 28, 28, 27, 27, 21, 45, 13}},
	{ID: "Hos", Name: "Hosea", Testament: Old, Chapters: {11, 23, 5, 19, 15, 11, 16, 14, 17, 15, 12, 14, 16, 9}},
	{ID: "Joel", Name: "Joel", Testament: Old, Chapters: {20, 32, 21}},
	{ID: "Amos", Name: "Amos", Testament: Old, Chapters: {15, 16, 15, 13, 27, 14, 17, 14, 15}},
	{ID: "Obad", Name: "Obadiah", Testament: Old, Chapters: {21}},
	{ID: "Jonah", Name: "Jonah", Testament: Old, Chapters: {17, 10, 10, 11}},
	{ID: "Mic", Name: "Micah", Testament: Old, Chapters: {16, 13, 12, 13, 15, 16, 20}},
	{ID: "Nah", Name: "Nahum", Testament: Old, Chapters: {15, 13, 19}},
	{ID: "Hab", Name: "Habakkuk", Testament: Old, Chapters: {17, 20, 19}},
	{ID: "Zeph", Name: "Zephaniah", Testament: Old, Chapters: {18, 15, 20}},
	{ID: "Hag", Name: "Haggai", Testament: Old, Chapters: {15, 23}},
	{ID: "Zech", Name: "Zechariah", Testament: Old, Chapters: {21, 13, 10, 14, 11, 15, 14, 23, 17, 12, 17, 14, 9, 21}},
	{ID: "Mal", Name: "Malachi", Testament: Old, Chapters: {14, 17, 18, 6}},
	{ID: "Matt", Name: "Matthew", Testament: New, Chapters: {25, 23, 17, 25, 48, 34, 29, 34, 38, 42, 30, 50, 58, 36, 39, 28, 27, 35, 30, 34, 46, 46, 39, 51, 46, 75, 66, 20}},
	{ID: "Mark", Name: "Mark", Testament: New, Chapters: {45, 28, 35, 41, 43, 56, 37, 38, 50, 52, 33, 44, 37, 72, 47, 20}},
	{ID: "Luke", Name: "Luke", Testament: New, Chapters: {80, 52, 38, 44, 39, 49, 50, 56, 62, 42, 54, 59, 35, 35, 32, 31, 37, 43, 48, 47, 38, 71, 56, 53}},
	{ID: "John", Name: "John", Testament: New, Chapters: {51, 25, 36, 54, 47, 71, 53, 59, 41, 42, 57, 50, 38, 31, 27, 33, 26, 40, 42, 31, 25}},
	{ID: "Acts", Name: "Acts", Testament: New, Chapters: {26, 47, 26, 37, 42, 15, 60, 40, 43, 48, 30, 25, 52, 28, 41, 40, 34, 28, 41, 38, 40, 30, 35, 27, 27, 32, 44, 31}},
	{ID: "Rom", Name: "Romans", Testament: New, Chapters: {32, 29, 31, 25, 21, 23, 25, 39, 33, 21, 36, 21, 14, 23, 33, 27}},
	{ID: "1Cor", Name: "1 Corinthians", Testament: New, Chapters: {31, 16, 23, 21, 13, 20, 40, 13, 27, 33, 34, 31, 13, 40, 58, 24}},
	{ID: "2Cor", Name: "2 Corinthians", Testament: New, Chapters: {24, 17, 18, 18, 21, 18, 16, 24, 15, 18, 33, 21, 14}},
	{ID: "Gal", Name: "Galatians", Testament: New, Chapters: {24, 21, 29, 31, 26, 18}},
	{ID: "Eph", Name: "Ephesians", Testament: New, Chapters: {23, 22, 21, 32, 33, 24}},
	{ID: "Phil", Name: "Philippians", Testament: New, Chapters: {30, 30, 21, 23}},
	{ID: "Col", Name: "Colossians", Testament: New, Chapters: {29, 23, 25, 18}},
	{ID: "1Thess", Name: "1 Thessalonians", Testament: New, Chapters: {10, 20, 13, 18, 28}},
	{ID: "2Thess", Name: "2 Thessalonians", Testament: New, Chapters: {12, 17, 18}},
	{ID: "1Tim", Name: "1 Timothy", Testament: New, Chapters: {20, 15, 16, 16, 25, 21}},
	{ID: "2Tim", Name: "2 Timothy", Testament: New, Chapters: {18, 26, 17, 22}},
	{ID: "Titus", Name: "Titus", Testament: New, Chapters: {16, 15, 15}},
	{ID: "Phlm", Name: "Philemon", Testament: New, Chapters: {25}},
	{ID: "Heb", Name: "Hebrews", Testament: New, Chapters: {14, 18, 19, 16, 14, 20, 28, 13, 28, 39, 40, 29, 25}},
	{ID: "Jas", Name: "James", Testament: New, Chapters: {27, 26, 18, 17, 20}},
	{ID: "1Pet", Name: "1 Peter", Testament: New, Chapters: {25, 25, 22, 19, 14}},
	{ID: "2Pet", Name: "2 Peter", Testament: New, Chapters: {21, 22, 18}},
	{ID: "1John", Name: "1 John", Testament: New, Chapters: {10, 29, 24, 21, 21}},
	{ID: "2John", Name: "2 John", Testament: New, Chapters: {13}},
	{ID: "3John", Name: "3 John", Testament: New, Chapters: {14}},
	{ID: "Jude", Name: "Jude", Testament: New, Chapters: {25}},
	{ID: "Rev", Name: "Revelation", Testament: New, Chapters: {20, 29, 22, 11, 14, 17, 17, 13, 21, 11, 19, 17, 18, 20, 8, 21, 18, 24, 21, 15, 27, 21}},
}

// canonIndex is built once so lookups do not rescan the table.
var canonIndex = func() map[string]*Book {
	m := make(map[string]*Book, len(Canon))
	for i := range Canon {
		m[Canon[i].ID] = &Canon[i]
	}
	return m
}()

// BookByID returns the canonical book for an OSIS identifier.
func BookByID(id string) (*Book, bool) {
	b, ok := canonIndex[id]
	return b, ok
}

// CanonicalChapterCount is the total number of chapters across the canon.
func CanonicalChapterCount() int {
	n := 0
	for _, b := range Canon {
		n += len(b.Verses)
	}
	return n
}

// ReferenceVerseCount is the total number of verses in the reference
// distribution, which is the KJV's 31,102.
func ReferenceVerseCount() int {
	n := 0
	for _, b := range Canon {
		for _, v := range b.Verses {
			n += v
		}
	}
	return n
}
