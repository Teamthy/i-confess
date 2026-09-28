package bible

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
)

// structure.go — the canonical shape of the Bible, as data every surface reads.
//
// canon.go already holds the 66 books and their reference verse distribution.
// What was missing is the *navigable* structure: the grouping a reader
// recognises (Law, History, Wisdom, the Prophets, the Gospels, the Epistles),
// the short forms a reference list prints, the USFM code a deep link uses and
// the alias table a reference parser needs. Those were spread between ref.go's
// unexported alias map, the display names in canon.go and nothing at all for
// sections, which meant the web app and the mobile app each had to invent
// their own copy of the same table — and a book grouped differently in two
// clients is a bug the API can never see.
//
// So the structure is computed here, once, from the canon, and published:
//
//   - GET /v1/bible/structure serves it to any client.
//   - cmd/bible-structure writes web/lib/canon.json from it, and a test in
//     this package fails when that file drifts from this table.
//
// Nothing here is translation data. A translation says which of these books it
// actually carries and how many verses its edition has in each chapter; the
// structure says what the canon is. Keeping the two apart is what lets the
// reader render a complete book list before any translation has loaded, and
// what lets a version legitimately differ from the reference distribution
// without the navigation changing shape.

// Section identifiers. They are the groupings the reader displays, not a
// doctrinal claim: Protestant Bibles are conventionally read in these blocks
// and every book belongs to exactly one.
const (
	SectionLaw            = "law"
	SectionHistory        = "history"
	SectionWisdom         = "wisdom"
	SectionMajorProphets  = "major_prophets"
	SectionMinorProphets  = "minor_prophets"
	SectionGospels        = "gospels"
	SectionActs           = "acts"
	SectionPaulineLetters = "pauline_letters"
	SectionGeneralLetters = "general_letters"
	SectionApocalypse     = "apocalypse"
)

// sectionOrder is the canonical section sequence with its display names. The
// first book of each section is recorded so the table cannot silently drift
// out of step with Canon: the builder walks the canon in order and starts a
// new section when it reaches that book, and a mismatch fails the test.
var sectionOrder = []struct {
	ID        string
	Name      string
	Testament string
	FirstBook string
}{
	{SectionLaw, "Law", Old, "Gen"},
	{SectionHistory, "History", Old, "Josh"},
	{SectionWisdom, "Wisdom & Poetry", Old, "Job"},
	{SectionMajorProphets, "Major Prophets", Old, "Isa"},
	{SectionMinorProphets, "Minor Prophets", Old, "Hos"},
	{SectionGospels, "Gospels", New, "Matt"},
	{SectionActs, "History", New, "Acts"},
	{SectionPaulineLetters, "Letters of Paul", New, "Rom"},
	{SectionGeneralLetters, "General Letters", New, "Heb"},
	{SectionApocalypse, "Apocalypse", New, "Rev"},
}

// abbreviations are the short forms shown in reference lists, chips and
// breadcrumbs. They are the conventional English abbreviations rather than the
// OSIS IDs, because "Song" and "Phlm" read as identifiers while "Song" and
// "Phm" read as a Bible.
var abbreviations = map[string]string{
	"Gen": "Gen", "Exod": "Ex", "Lev": "Lev", "Num": "Num", "Deut": "Deut",
	"Josh": "Josh", "Judg": "Judg", "Ruth": "Ruth", "1Sam": "1 Sam", "2Sam": "2 Sam",
	"1Kgs": "1 Kgs", "2Kgs": "2 Kgs", "1Chr": "1 Chr", "2Chr": "2 Chr", "Ezra": "Ezra",
	"Neh": "Neh", "Esth": "Esth", "Job": "Job", "Ps": "Ps", "Prov": "Prov",
	"Eccl": "Eccl", "Song": "Song", "Isa": "Isa", "Jer": "Jer", "Lam": "Lam",
	"Ezek": "Ezek", "Dan": "Dan", "Hos": "Hos", "Joel": "Joel", "Amos": "Amos",
	"Obad": "Obad", "Jonah": "Jonah", "Mic": "Mic", "Nah": "Nah", "Hab": "Hab",
	"Zeph": "Zeph", "Hag": "Hag", "Zech": "Zech", "Mal": "Mal",
	"Matt": "Matt", "Mark": "Mark", "Luke": "Luke", "John": "John", "Acts": "Acts",
	"Rom": "Rom", "1Cor": "1 Cor", "2Cor": "2 Cor", "Gal": "Gal", "Eph": "Eph",
	"Phil": "Phil", "Col": "Col", "1Thess": "1 Thess", "2Thess": "2 Thess",
	"1Tim": "1 Tim", "2Tim": "2 Tim", "Titus": "Titus", "Phlm": "Phm",
	"Heb": "Heb", "Jas": "Jas", "1Pet": "1 Pet", "2Pet": "2 Pet",
	"1John": "1 Jn", "2John": "2 Jn", "3John": "3 Jn", "Jude": "Jude", "Rev": "Rev",
}

// osisToUSFM inverts the usfm table in ref.go so a book can name its USFM code
// without every caller rescanning the map.
var osisToUSFM = func() map[string]string {
	m := make(map[string]string, len(usfm))
	for code, id := range usfm {
		m[id] = code
	}
	return m
}()

// USFMCode returns the three-letter USFM code for a canonical book ID.
// It is the code canonical verse IDs are built from ("JHN" -> JHN.3.16).
func USFMCode(bookID string) (string, bool) {
	code, ok := osisToUSFM[bookID]
	return code, ok
}

// StructureBook is one book as the navigation needs it: identity, placement
// and size. Chapters carries the reference verse count per chapter, so a
// client can render a chapter grid, validate a typed reference and size a
// verse picker without fetching any translation.
type StructureBook struct {
	ID             string   `json:"id"`
	USFM           string   `json:"usfm"`
	Name           string   `json:"name"`
	Abbreviation   string   `json:"abbreviation"`
	Testament      string   `json:"testament"`
	Section        string   `json:"section"`
	CanonicalOrder int      `json:"canonical_order"`
	ChapterCount   int      `json:"chapter_count"`
	VerseCount     int      `json:"verse_count"`
	Chapters       []int    `json:"chapters"`
	Aliases        []string `json:"aliases,omitempty"`
}

// StructureSection is a run of consecutive books inside one testament.
type StructureSection struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Testament    string   `json:"testament"`
	BookIDs      []string `json:"book_ids"`
	BookCount    int      `json:"book_count"`
	ChapterCount int      `json:"chapter_count"`
}

// StructureTestament is one of the two testaments with its sections.
type StructureTestament struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	BookCount    int                `json:"book_count"`
	ChapterCount int                `json:"chapter_count"`
	VerseCount   int                `json:"verse_count"`
	Sections     []StructureSection `json:"sections"`
}

// Structure is the whole canonical shape: what the Bible is, before any
// translation says what it carries.
type Structure struct {
	Canon        string               `json:"canon"`
	BookCount    int                  `json:"book_count"`
	ChapterCount int                  `json:"chapter_count"`
	VerseCount   int                  `json:"verse_count"`
	Testaments   []StructureTestament `json:"testaments"`
	Books        []StructureBook      `json:"books"`
	// Aliases maps a normalised book key to its canonical ID. It is the same
	// table ParseBook resolves against, published so a client-side reference
	// parser produces identical answers to the server's.
	Aliases map[string]string `json:"aliases"`
}

var (
	structureOnce  sync.Once
	structureValue Structure
)

// CanonStructure returns the canonical structure. It is computed once from
// Canon and is safe to share: callers must treat the result as read-only.
func CanonStructure() Structure {
	structureOnce.Do(buildStructure)
	return structureValue
}

// aliasesByBook groups the normalised alias keys under the book they resolve
// to, so each book can publish the spellings that open it.
func aliasesByBook() map[string][]string {
	out := make(map[string][]string, len(Canon))
	for key, id := range aliases {
		out[id] = append(out[id], key)
	}
	for id := range out {
		sort.Strings(out[id])
	}
	return out
}

// AliasIndex returns a copy of the normalised alias table: key -> book ID.
// Keys are produced by NormalizeBookKey, which a client must apply to user
// input before looking a book up.
func AliasIndex() map[string]string {
	out := make(map[string]string, len(aliases))
	for k, v := range aliases {
		out[k] = v
	}
	return out
}

// NormalizeBookKey lowercases a book name and strips the punctuation and
// repeated spacing that distinguish "1 Peter", "1peter" and "1  Peter". It is
// the exact transformation the alias table is keyed by.
func NormalizeBookKey(s string) string { return normalizeKey(s) }

func buildStructure() {
	byAlias := aliasesByBook()

	books := make([]StructureBook, 0, len(Canon))
	sectionOf := make(map[string]string, len(Canon))
	current := 0

	for i := range Canon {
		b := Canon[i]
		// Advance to the section this book opens, if it opens one. The table
		// is ordered, so a book that starts a section always arrives exactly
		// when the next entry expects it.
		if current+1 < len(sectionOrder) && sectionOrder[current+1].FirstBook == b.ID {
			current++
		}
		section := sectionOrder[current]
		sectionOf[b.ID] = section.ID

		verses := 0
		for _, v := range b.Verses {
			verses += v
		}
		chapters := make([]int, len(b.Verses))
		copy(chapters, b.Verses)

		books = append(books, StructureBook{
			ID:             b.ID,
			USFM:           osisToUSFM[b.ID],
			Name:           b.Name,
			Abbreviation:   abbreviations[b.ID],
			Testament:      b.Testament,
			Section:        section.ID,
			CanonicalOrder: i + 1,
			ChapterCount:   len(b.Verses),
			VerseCount:     verses,
			Chapters:       chapters,
			Aliases:        byAlias[b.ID],
		})
	}

	index := make(map[string]StructureBook, len(books))
	for _, b := range books {
		index[b.ID] = b
	}

	testaments := make([]StructureTestament, 0, 2)
	for _, testament := range []struct{ id, name string }{{Old, "Old Testament"}, {New, "New Testament"}} {
		entry := StructureTestament{ID: testament.id, Name: testament.name}
		for _, section := range sectionOrder {
			if section.Testament != testament.id {
				continue
			}
			s := StructureSection{ID: section.ID, Name: section.Name, Testament: section.Testament}
			for _, b := range books {
				if sectionOf[b.ID] != section.ID {
					continue
				}
				s.BookIDs = append(s.BookIDs, b.ID)
				s.ChapterCount += b.ChapterCount
			}
			s.BookCount = len(s.BookIDs)
			entry.Sections = append(entry.Sections, s)
		}
		for _, b := range books {
			if b.Testament != testament.id {
				continue
			}
			entry.BookCount++
			entry.ChapterCount += b.ChapterCount
			entry.VerseCount += b.VerseCount
		}
		testaments = append(testaments, entry)
	}

	structureValue = Structure{
		Canon:        "protestant",
		BookCount:    len(books),
		ChapterCount: CanonicalChapterCount(),
		VerseCount:   ReferenceVerseCount(),
		Testaments:   testaments,
		Books:        books,
		Aliases:      AliasIndex(),
	}
}

// StructureJSON renders the canonical structure the way the checked-in
// web/lib/canon.json holds it: indented, key-stable and newline-terminated, so
// a regeneration produces a byte-identical file when nothing changed and an
// obvious diff when something did.
func StructureJSON() ([]byte, error) {
	document, err := json.MarshalIndent(CanonStructure(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(document, '\n'), nil
}

// RegistryEntry is one reviewed version's provenance: identity, coverage and
// the exact file its text came from — never the text itself. It is the
// honest list a reader-facing surface can show about versions that are
// reviewed but not imported: what the edition is, what licence it carries,
// and which source file was verified against which digest.
type RegistryEntry struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Abbreviation    string   `json:"abbreviation"`
	Language        string   `json:"language"`
	LanguageName    string   `json:"language_name"`
	Coverage        string   `json:"coverage"`
	Year            string   `json:"year,omitempty"`
	Licence         string   `json:"licence"`
	LicenceURL      string   `json:"licence_url,omitempty"`
	LicenceNote     string   `json:"licence_note,omitempty"`
	OmittedBooks    []string `json:"omitted_books,omitempty"`
	OmittedChapters []string `json:"omitted_chapters,omitempty"`
	Attribution     string   `json:"attribution"`
	SourceFile      string   `json:"source_file"`
	SHA256          string   `json:"sha256"`
	Bytes           int64    `json:"bytes"`
	Format          string   `json:"format"`
	SortOrder       int      `json:"sort_order"`
	Default         bool     `json:"default,omitempty"`
	Status          string   `json:"status,omitempty"`
}

// RegistryDocument is the whole reviewed registry as one JSON document — the
// shape scripts/gen-bible-registry.py writes to web/lib/versions.json and the
// shape TestWebRegistryJSONIsCurrent decodes it into.
type RegistryDocument struct {
	VersionCount  int             `json:"version_count"`
	LanguageCount int             `json:"language_count"`
	Versions      []RegistryEntry `json:"versions"`
}

// PublishedRegistry projects Versions into provenance-only entries: twelve
// versions, seven languages, sorted by sort order, carrying no scripture
// text. The web app renders this as its "reviewed registry" so a reader can
// see every edition the platform has reviewed, imported here or not.
func PublishedRegistry() RegistryDocument {
	entries := make([]RegistryEntry, 0, len(Versions))
	for _, v := range Versions {
		entries = append(entries, RegistryEntry{
			ID:              v.ID,
			Name:            v.Name,
			Abbreviation:    v.Abbrev,
			Language:        v.Language,
			LanguageName:    v.LanguageName,
			Coverage:        v.Coverage,
			Year:            v.Year,
			Licence:         v.Licence,
			LicenceURL:      v.LicenceURL,
			LicenceNote:     v.LicenceNote,
			OmittedBooks:    v.OmittedBooks,
			OmittedChapters: v.OmittedChapters,
			Attribution:     v.Attribution,
			SourceFile:      v.SourceFile,
			SHA256:          v.SHA256,
			Bytes:           v.Bytes,
			Format:          v.Format,
			SortOrder:       v.SortOrder,
			Default:         v.Default,
			Status:          v.Status,
		})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].SortOrder < entries[j].SortOrder })
	return RegistryDocument{
		VersionCount:  len(entries),
		LanguageCount: len(Languages()),
		Versions:      entries,
	}
}

// RegistryJSON renders the published registry the way the checked-in
// web/lib/versions.json holds it: indented and newline-terminated. The Go
// test compares that file by decoded content rather than bytes, so a second
// generator (scripts/gen-bible-registry.py, which parses registry.go) may
// format it differently as long as the content agrees.
func RegistryJSON() ([]byte, error) {
	document, err := json.MarshalIndent(PublishedRegistry(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(document, '\n'), nil
}

// StructureBookByID returns one book's structure entry.
func StructureBookByID(id string) (StructureBook, bool) {
	for _, b := range CanonStructure().Books {
		if strings.EqualFold(b.ID, id) {
			return b, true
		}
	}
	return StructureBook{}, false
}

// ReferenceVerseCountFor returns the reference verse count for one chapter,
// which is the upper bound a client should allow when validating a typed
// reference. It is the KJV distribution, not a promise about any translation.
func ReferenceVerseCountFor(bookID string, chapter int) (int, bool) {
	b, ok := BookByID(bookID)
	if !ok || chapter < 1 || chapter > b.Chapters() {
		return 0, false
	}
	return b.Verses[chapter-1], true
}
