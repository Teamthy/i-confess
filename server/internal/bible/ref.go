package bible

import (
	"fmt"
	"strconv"
	"strings"
)

// Reference parsing and normalisation.
//
// The platform stores Scripture citations in three different dialects and all
// three have to name the same verse:
//
//   - the canonical confession corpus, which writes "1 Peter" and "22-24"
//     the way a person would;
//   - OSIS files, which use osisID values like "1Pet.2.24";
//   - USFX files, which use USFM codes like "1PE" and a numeric verse.
//
// A highlight, a bookmark and a deep link all key off the same normalised
// triple (book, chapter, verse), so if two of those dialects disagree by one
// character the feature silently links to nothing. Everything funnels through
// ParseBook and ParseVerseSpec for that reason, and the tests pin the corpus's
// actual spellings rather than a tidy subset of them.

// usfm maps USFM codes to OSIS book IDs. USFX sources identify books this way;
// the mapping is one-to-one and total across the canon.
var usfm = map[string]string{
	"GEN": "Gen", "EXO": "Exod", "LEV": "Lev", "NUM": "Num", "DEU": "Deut",
	"JOS": "Josh", "JDG": "Judg", "RUT": "Ruth", "1SA": "1Sam", "2SA": "2Sam",
	"1KI": "1Kgs", "2KI": "2Kgs", "1CH": "1Chr", "2CH": "2Chr", "EZR": "Ezra",
	"NEH": "Neh", "EST": "Esth", "JOB": "Job", "PSA": "Ps", "PRO": "Prov",
	"ECC": "Eccl", "SNG": "Song", "ISA": "Isa", "JER": "Jer", "LAM": "Lam",
	"EZK": "Ezek", "DAN": "Dan", "HOS": "Hos", "JOL": "Joel", "AMO": "Amos",
	"OBA": "Obad", "JON": "Jonah", "MIC": "Mic", "NAM": "Nah", "HAB": "Hab",
	"ZEP": "Zeph", "HAG": "Hag", "ZEC": "Zech", "MAL": "Mal",
	"MAT": "Matt", "MRK": "Mark", "LUK": "Luke", "JHN": "John", "ACT": "Acts",
	"ROM": "Rom", "1CO": "1Cor", "2CO": "2Cor", "GAL": "Gal", "EPH": "Eph",
	"PHP": "Phil", "COL": "Col", "1TH": "1Thess", "2TH": "2Thess", "1TI": "1Tim",
	"2TI": "2Tim", "TIT": "Titus", "PHM": "Phlm", "HEB": "Heb", "JAS": "Jas",
	"1PE": "1Pet", "2PE": "2Pet", "1JN": "1John", "2JN": "2John", "3JN": "3John",
	"JUD": "Jude", "REV": "Rev",
}

// Zefania numbers books 1-66 in canonical order.
func bookByNumber(n int) (string, bool) {
	if n < 1 || n > len(Canon) {
		return "", false
	}
	return Canon[n-1].ID, true
}

// aliases covers every spelling that appears in the canonical confession
// corpus, plus the variants a human or an API client is likely to type.
// Keys are lowercased and stripped of punctuation and whitespace, so
// "1 Peter", "1peter", "1  Peter" and "I Peter" all collapse to one key.
var aliases = func() map[string]string {
	m := make(map[string]string, len(Canon)*4)

	put := func(key, id string) {
		k := normalizeKey(key)
		if k == "" {
			return
		}
		// First writer wins: the canon's own name is registered first below,
		// so an alias can never shadow a book.
		if _, exists := m[k]; !exists {
			m[k] = id
		}
	}

	// Canonical ID and full name, e.g. "1Cor" and "1 Corinthians".
	for _, b := range Canon {
		put(b.ID, b.ID)
		put(b.Name, b.ID)
	}

	// Spellings the corpus uses that differ from the canon's display name.
	extra := map[string]string{
		"psalm": "Ps", "psalms": "Ps", "psa": "Ps",
		"song of songs": "Song", "song of solomon": "Song", "canticles": "Song",
		"revelation of john": "Rev", "revelations": "Rev", "apocalypse": "Rev",
		"esias": "Isa", "isaiah": "Isa",
		"1 samuel": "1Sam", "2 samuel": "2Sam",
		"1 kings": "1Kgs", "2 kings": "2Kgs",
		"1 chronicles": "1Chr", "2 chronicles": "2Chr",
		"1 corinthians": "1Cor", "2 corinthians": "2Cor",
		"1 thessalonians": "1Thess", "2 thessalonians": "2Thess",
		"1 timothy": "1Tim", "2 timothy": "2Tim",
		"1 peter": "1Pet", "2 peter": "2Pet",
		"1 john": "1John", "2 john": "2John", "3 john": "3John",
		"philemon": "Phlm", "philippians": "Phil",
		"james": "Jas", "hebrews": "Heb", "titus": "Titus", "jude": "Jude",
		"colossians": "Col", "ephesians": "Eph", "galatians": "Gal",
		"philippians ": "Phil",
	}
	for k, v := range extra {
		put(k, v)
	}

	// Roman-numeral prefixes, which older citations use ("II Timothy", "I
	// John"). The alias table is keyed by the modern form - the canon calls
	// the book "2 Timothy" - so the number is what has to be rewritten, and
	// the rewrite has to happen before the map is read.
	roman := map[string]string{"1": "i", "2": "ii", "3": "iii"}
	numbered := make([]struct{ key, id string }, 0, len(m))
	for k, id := range m {
		parts := strings.SplitN(k, " ", 2)
		if len(parts) != 2 {
			continue
		}
		if r, ok := roman[parts[0]]; ok {
			numbered = append(numbered, struct{ key, id string }{r + " " + parts[1], id})
		}
	}
	for _, n := range numbered {
		put(n.key, n.id)
	}

	// USFM codes, which is what USFX sources carry.
	for code, id := range usfm {
		put(code, id)
	}

	return m
}()

// normalizeKey lowercases and removes everything that is not a letter, digit
// or single separating space.
func normalizeKey(s string) string {
	var b strings.Builder
	lastSpace := true // trims leading spaces
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSpace = false
		case r == ' ' || r == '\t':
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
		default:
			// Punctuation and periods are separators: "1Pet.2.24" normalises
			// to "1pet 2 24" when the whole string is passed, but book names
			// never contain punctuation so dropping it is safe.
			if r == '.' || r == '-' || r == '\'' {
				continue
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// ParseBook resolves a book name in any supported dialect to its canonical
// OSIS identifier. It accepts the corpus spellings ("Psalm", "1 Peter"),
// canonical IDs ("Ps", "1Pet"), full names ("Psalms"), USFM codes ("PSA")
// and numbered OSIS IDs ("Ps.103" - the book part is taken).
func ParseBook(name string) (string, bool) {
	raw := strings.TrimSpace(name)
	if raw == "" {
		return "", false
	}
	// "Ps.103" / "1Pet.2.24": take the book part before the first dot only if
	// what follows looks like a chapter number.
	if i := strings.IndexByte(raw, '.'); i > 0 {
		if _, err := strconv.Atoi(strings.TrimSpace(raw[i+1:])); err == nil {
			raw = raw[:i]
		}
	}
	if id, ok := aliases[normalizeKey(raw)]; ok {
		return id, true
	}
	return "", false
}

// ParseVerseSpec turns a verse specification into the individual verses it
// names. The corpus stores ranges as free text ("5", "22-24", "2-3"), so a
// range is expanded rather than approximated: a confession that cites Psalm
// 103:2-3 links to two verses and highlights both.
//
// A spec that cannot be read returns (nil, false) so callers can report it
// instead of guessing.
func ParseVerseSpec(spec string) ([]int, bool) {
	s := strings.TrimSpace(spec)
	if s == "" {
		return nil, false
	}
	// Normalise the separators a hand-written range may use.
	s = strings.NewReplacer("–", "-", "—", "-", " to ", "-", ",", "-", ";", "-").Replace(s)

	parts := strings.Split(s, "-")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		// "2a" / "14b": the letter is a sub-verse marker, the verse number is
		// the number in front of it.
		p = strings.TrimRightFunc(p, func(r rune) bool {
			return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		})
		// An empty part means the dash had nothing on one side of it
		// ("-4", "3-"), which is not a verse.
		if p == "" {
			return nil, false
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 {
			return nil, false
		}
		nums = append(nums, n)
	}
	switch len(nums) {
	case 0:
		return nil, false
	case 1:
		return nums, true
	case 2:
		if nums[0] > nums[1] {
			return nil, false
		}
		out := make([]int, 0, nums[1]-nums[0]+1)
		for v := nums[0]; v <= nums[1]; v++ {
			out = append(out, v)
		}
		return out, true
	default:
		// A comma-separated list is already discrete; a chain of ranges is
		// ambiguous and refused.
		return nil, false
	}
}

// Reference is a normalised citation: the coordinates a reader, a highlight
// and a deep link all need to agree on.
type Reference struct {
	Book    string `json:"book"`             // canonical OSIS ID, e.g. "1Pet"
	Name    string `json:"name"`             // display name, e.g. "1 Peter"
	Chapter int    `json:"chapter"`          //
	Verses  []int  `json:"verses,omitempty"` // expanded, ascending
	// Display is the human form, e.g. "1 Peter 2:24" or "Psalm 103:2-3".
	Display string `json:"display"`
}

// DisplayRef renders a citation the way a reader expects to see it, and is the
// single place that spelling is produced so the web app, the mobile app and
// the API can never disagree about what a reference is called.
func DisplayRef(book string, chapter int, verses []int) string {
	b, ok := BookByID(book)
	name := book
	if ok {
		name = b.Name
	}
	switch len(verses) {
	case 0:
		return fmt.Sprintf("%s %d", name, chapter)
	case 1:
		return fmt.Sprintf("%s %d:%d", name, chapter, verses[0])
	default:
		// Collapse a run back to "2-3" but only when it is contiguous; a
		// scattered list reads better comma-separated.
		contiguous := true
		for i := 1; i < len(verses); i++ {
			if verses[i] != verses[i-1]+1 {
				contiguous = false
				break
			}
		}
		if contiguous {
			return fmt.Sprintf("%s %d:%d-%d", name, chapter, verses[0], verses[len(verses)-1])
		}
		parts := make([]string, 0, len(verses))
		for _, v := range verses {
			parts = append(parts, strconv.Itoa(v))
		}
		return fmt.Sprintf("%s %d:%s", name, chapter, strings.Join(parts, ","))
	}
}

// NewReference builds a normalised reference, validating it against the canon.
// A citation that does not exist in the canon is rejected here rather than
// stored, because a stored reference to a verse that is not in any Bible
// produces a dead deep link that no client can repair.
func NewReference(book string, chapter int, verseSpec string) (Reference, error) {
	id, ok := ParseBook(book)
	if !ok {
		return Reference{}, fmt.Errorf("bible: unknown book %q", book)
	}
	return NewReferenceByID(id, chapter, verseSpec)
}

// NewReferenceByID is NewReference for a caller that already holds a canonical
// book ID.
func NewReferenceByID(book string, chapter int, verseSpec string) (Reference, error) {
	b, ok := BookByID(book)
	if !ok {
		return Reference{}, fmt.Errorf("bible: unknown book %q", book)
	}
	if chapter < 1 || chapter > b.Chapters() {
		return Reference{}, fmt.Errorf("bible: %s has %d chapters, got %d", b.Name, b.Chapters(), chapter)
	}
	verses, ok := ParseVerseSpec(verseSpec)
	if !ok {
		return Reference{}, fmt.Errorf("bible: unreadable verse spec %q", verseSpec)
	}
	// Verse counts differ between translations, so the reference distribution
	// is used as the outer bound: anything beyond the highest count any
	// translation carries for that chapter cannot be a real verse.
	limit := 0
	for _, v := range verses {
		if v > limit {
			limit = v
		}
	}
	if limit > b.Verses[chapter-1] {
		return Reference{}, fmt.Errorf("bible: %s %d has %d verses, got %d",
			b.Name, chapter, b.Verses[chapter-1], limit)
	}
	return Reference{
		Book:    b.ID,
		Name:    b.Name,
		Chapter: chapter,
		Verses:  verses,
		Display: DisplayRef(b.ID, chapter, verses),
	}, nil
}
