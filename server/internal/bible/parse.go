package bible

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Source parsing.
//
// The importer reads three XML dialects, because between them they are what
// public-domain publishers actually ship:
//
//   - OSIS (KJV, Swahili, Open English Bible). Verses arrive either as
//     empty "milestone" elements (<verse sID="Gen.1.1"/>text<verse eID="..."/>)
//     or as containers (<verse osisID="Gen.1.1">text</verse>), and one file
//     can use both. Book divisions also include front matter and apocrypha
//     that are not in the canon, so div counts must never be trusted.
//   - USFX (WEB, BBE, BSB, WEBBE). <book id="JHN"><c id="3"><v id="16"/>text<ve/>,
//     with section headings, poetry lines and footnotes interleaved with the
//     verse text.
//   - Zefania (ASV, Darby, DRA, YLT). <BIBLEBOOK bnumber="45"><CHAPTER cnumber="1">
//     <VERS vnumber="1">text</VERS>, plain and already numeric.
//
// All three feed one collector, so the rules that matter - what counts as
// verse text, what gets discarded, and how whitespace is folded - are decided
// once. A parser per format with its own text handling would drift, and the
// difference would show up as a stray footnote in a verse a user is reading
// aloud.

// Source formats.
const (
	FormatOSIS    = "osis"
	FormatUSFX    = "usfx"
	FormatZefania = "zefania"
)

// ParsedTranslation is a parsed Bible: its books in canonical order, with the verse
// text keyed by chapter and verse number.
type ParsedTranslation struct {
	// ID is the version identifier the API and the database use, e.g. "kjv".
	ID string
	// Books is in canonical order - the order the reader displays - not the
	// order the file happens to list them in.
	Books []BookText
	// Skipped records book divisions the file contained that are not in the
	// canon (front matter, apocrypha, glossaries). The importer reports these
	// rather than silently dropping or silently importing them.
	Skipped []string
	// Containers records how many verses arrived in each XML style. It is a
	// parser health signal: a file that is entirely milestones or entirely
	// containers parses fine, but one where the count is close to zero in both
	// means the parser did not recognise the dialect and produced a Bible with
	// almost no text in it.
	Milestones int
	Containers int
	// EmptyVerses counts verse elements that carried no text. They are pruned
	// rather than stored: several editions keep a numbered placeholder for a
	// verse they do not have (the World English Bible against Luke 17:36) and
	// a reader must not be shown an empty verse as though it were Scripture.
	EmptyVerses int
	// EmptySamples names a few pruned verses, for the verification report.
	EmptySamples []string

	byBook map[string]*BookText
}

// BookText is one book of a translation.
type BookText struct {
	ID       string
	Chapters []ChapterText
}

// ChapterText is one chapter: a chapter number and its verses.
type ChapterText struct {
	Number int
	Verses []ParsedVerse
}

// ParsedVerse is a single verse of a translation.
type ParsedVerse struct {
	Number int
	Text   string
}

// VerseCount is the total number of verses parsed.
func (t *ParsedTranslation) VerseCount() int {
	n := 0
	for i := range t.Books {
		for j := range t.Books[i].Chapters {
			n += len(t.Books[i].Chapters[j].Verses)
		}
	}
	return n
}

// Book returns the named book, or nil.
func (t *ParsedTranslation) Book(id string) *BookText {
	if t.byBook == nil {
		return nil
	}
	return t.byBook[id]
}

// Chapter returns the named chapter, or nil.
func (t *ParsedTranslation) Chapter(book string, chapter int) *ChapterText {
	b := t.Book(book)
	if b == nil {
		return nil
	}
	i := sort.Search(len(b.Chapters), func(i int) bool { return b.Chapters[i].Number >= chapter })
	if i < len(b.Chapters) && b.Chapters[i].Number == chapter {
		return &b.Chapters[i]
	}
	return nil
}

// Text returns the text of one verse, and whether the translation carries it.
// A translation that does not cover a book (the Swahili file is New Testament
// only) misses cleanly instead of returning an empty string that a caller
// might render as a blank verse.
func (t *ParsedTranslation) Text(book string, chapter, verse int) (string, bool) {
	ch := t.Chapter(book, chapter)
	if ch == nil {
		return "", false
	}
	i := sort.Search(len(ch.Verses), func(i int) bool { return ch.Verses[i].Number >= verse })
	if i < len(ch.Verses) && ch.Verses[i].Number == verse {
		return ch.Verses[i].Text, true
	}
	return "", false
}

// Parse reads one source file in the named format.
//
// id is the version identifier the result is filed under; format is one of
// FormatOSIS, FormatUSFX or FormatZefania.
func Parse(r io.Reader, format, id string) (*ParsedTranslation, error) {
	t := &ParsedTranslation{ID: id, byBook: map[string]*BookText{}}
	c := &collector{t: t}

	dec := xml.NewDecoder(noBOM(r))
	// The KJV OSIS file declares its own encoding and is published as UTF-8
	// with characters well outside Latin-1; leaving the decoder's charset
	// handling on its default would mangle them.
	dec.Strict = false

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("bible: parse %s (%s): %w", id, format, err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			if err := c.start(el, format); err != nil {
				return nil, err
			}
		case xml.EndElement:
			c.end(el.Name.Local)
		case xml.CharData:
			c.chars(string(el))
		}
	}
	c.finish()

	if len(t.Books) == 0 {
		return nil, fmt.Errorf("bible: parse %s: no canonical books found in a %s file", id, format)
	}
	t.sortBooks()
	return t, nil
}

// noBOM strips a UTF-8 byte-order mark. Several Zefania files begin with one,
// which precedes the XML declaration and makes a strict decoder fail on the
// first byte.
func noBOM(r io.Reader) io.Reader {
	br := bufio.NewReaderSize(r, 64*1024)
	if b, err := br.Peek(3); err == nil && len(b) == 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		_, _ = br.Discard(3)
	}
	return br
}

// skipSubtrees are elements whose text is not Scripture: footnotes, cross
// references, section headings, running heads and front-matter scaffolding.
// Importing any of it would put words in a verse that the translation does not
// have there, which is worse than a missing verse because nothing downstream
// can tell it apart from the real text.
var skipSubtrees = map[string]bool{
	// Shared
	"note": true, "figure": true, "table": true, "header": true,
	// USFX
	"f": true, "fe": true, "x": true, "d": true, "s": true, "s1": true,
	"s2": true, "s3": true, "s4": true, "ms": true, "ms1": true, "ms2": true,
	"h": true, "toc": true, "id": true, "ide": true, "rem": true, "rq": true,
	"tr": true, "cp": true, "cl": true, "ca": true, "va": true, "vp": true,
	// Headings and speakers are editorial furniture. A <title> inside a
	// chapter sits between two verses, so importing it would append a heading
	// to the verse before it.
	"title": true, "speaker": true, "revisiondesc": true, "work": true,
}

type collector struct {
	t *ParsedTranslation

	curBook    *BookText
	curChapter *ChapterText
	curVerse   *ParsedVerse

	buf     strings.Builder
	skip    int
	sawBook string

	// verseKind tracks which style opened the verse currently being filled, so
	// the matching end event flushes it.
	open      bool
	milestone bool
}

// start handles one StartElement.
func (c *collector) start(el xml.StartElement, format string) error {
	name := el.Name.Local

	if c.skip > 0 {
		if skipSubtrees[strings.ToLower(name)] {
			c.skip++
		}
		return nil
	}
	if skipSubtrees[strings.ToLower(name)] {
		c.skip = 1
		return nil
	}

	switch strings.ToLower(name) {
	case "book", "biblebook":
		return c.startBook(el, format)
	case "div":
		// OSIS wraps books in <div type="book">; anything else is a section.
		if attribute(el, "type") == "book" {
			return c.startBook(el, FormatOSIS)
		}
	case "chapter":
		c.startChapterFromOSIS(el)
	case "c":
		if format == FormatUSFX {
			if n, err := strconv.Atoi(strings.TrimSpace(attribute(el, "id"))); err == nil {
				c.startChapter(n)
			}
		}
	case "vers", "verse":
		return c.startVerse(el, format)
	case "v", "ve":
		if format == FormatUSFX {
			if n, err := strconv.Atoi(strings.TrimSpace(attribute(el, "id"))); err == nil {
				// USFX verses are milestones: the text follows as siblings and
				// the verse closes at <ve/>, not at the end of the element.
				c.milestone = true
				c.startVerseNumber(n)
			}
			// <ve/> and <v id=".."/> ending immediately both close a verse.
			if strings.EqualFold(name, "ve") || attribute(el, "id") == "" {
				c.flush()
			}
		}
	}
	return nil
}

// end handles one EndElement.
func (c *collector) end(name string) {
	if c.skip > 0 {
		if skipSubtrees[strings.ToLower(name)] {
			c.skip--
		}
		return
	}
	switch strings.ToLower(name) {
	case "verse", "vers":
		if !c.milestone {
			c.flush()
		}
	}
}

// chars appends character data to the verse currently being read.
func (c *collector) chars(s string) {
	if c.skip > 0 || c.curVerse == nil || !c.open {
		return
	}
	c.buf.WriteString(s)
}

func (c *collector) startBook(el xml.StartElement, format string) error {
	c.flush()
	c.curBook, c.curChapter, c.sawBook = nil, nil, ""

	var id string
	switch format {
	case FormatZefania:
		n, err := strconv.Atoi(strings.TrimSpace(attribute(el, "bnumber")))
		if err != nil {
			// Not every Zefania file is numbered. Fall back to the name.
			id, _ = ParseBook(attribute(el, "bname"))
		} else if got, ok := bookByNumber(n); ok {
			id = got
		} else {
			// bnumber 67+ is an apocryphal book in some files; record it and
			// read on.
			if name := strings.TrimSpace(attribute(el, "bname")); name != "" {
				c.t.Skipped = append(c.t.Skipped, fmt.Sprintf("%s (bnumber %d)", name, n))
			}
			return nil
		}
	default:
		raw := attribute(el, "osisID")
		if raw == "" {
			raw = attribute(el, "id")
		}
		// USFX and OSIS both allow "Gen" or "GEN"; a trailing book number
		// ("GEN.1") is not a thing, but a leading number in Zefania-style
		// names is handled by ParseBook.
		if got, ok := ParseBook(raw); ok {
			id = got
		} else if osis, ok := usfm[strings.ToUpper(strings.TrimSpace(raw))]; ok {
			id = osis
		} else if osis, ok := bookByNumber(atoiOrZero(raw)); ok {
			id = osis
		}
	}

	if id == "" {
		if raw := strings.TrimSpace(firstNonEmpty(attribute(el, "osisID"), attribute(el, "id"), attribute(el, "bname"))); raw != "" {
			c.t.Skipped = append(c.t.Skipped, raw)
		}
		return nil
	}

	c.sawBook = id
	if c.t.byBook[id] != nil {
		// Two divisions claiming the same canonical book: keep the first and
		// say so instead of overwriting verses that are already correct.
		c.t.Skipped = append(c.t.Skipped, id+" (duplicate division)")
		c.curBook = nil
		return nil
	}
	b := &BookText{ID: id}
	c.t.byBook[id] = b
	c.t.Books = append(c.t.Books, *b)
	c.curBook = &c.t.Books[len(c.t.Books)-1]
	c.t.byBook[id] = c.curBook
	return nil
}

func (c *collector) startChapterFromOSIS(el xml.StartElement) {
	ref := firstNonEmpty(attribute(el, "osisID"), attribute(el, "sID"))
	if ref == "" {
		return
	}
	// "Gen.1" or "Gen.1.seID.00001"
	parts := strings.Split(ref, ".")
	if len(parts) < 2 {
		return
	}
	if n, err := strconv.Atoi(parts[1]); err == nil {
		c.startChapter(n)
	}
}

func (c *collector) startChapter(n int) {
	c.flush()
	c.curChapter = nil
	if c.curBook == nil || n < 1 {
		return
	}
	if last := len(c.curBook.Chapters) - 1; last >= 0 && c.curBook.Chapters[last].Number == n {
		c.curChapter = &c.curBook.Chapters[last]
		return
	}
	c.curBook.Chapters = append(c.curBook.Chapters, ChapterText{Number: n})
	c.curChapter = &c.curBook.Chapters[len(c.curBook.Chapters)-1]
}

// startVerse handles a verse element in either dialect.
func (c *collector) startVerse(el xml.StartElement, format string) error {
	if format == FormatZefania {
		n, err := strconv.Atoi(strings.TrimSpace(attribute(el, "vnumber")))
		if err != nil {
			return nil
		}
		c.milestone = false
		c.startVerseNumber(n)
		return nil
	}
	ref := firstNonEmpty(attribute(el, "osisID"), attribute(el, "sID"))

	// An element carrying only eID closes the verse that sID opened.
	if ref == "" && attribute(el, "eID") != "" {
		c.flush()
		return nil
	}

	parts := strings.Split(ref, ".")
	num := 0
	if n, err := strconv.Atoi(strings.TrimSpace(attribute(el, "n"))); err == nil {
		num = n
	} else if len(parts) == 3 {
		if n, err := strconv.Atoi(parts[2]); err == nil {
			num = n
		}
	}
	// "<verse osisID='Gen.1.1' sID='....seID.00002' n='1'/>" is a milestone:
	// the text follows as siblings, not as children, and the element's own end
	// event arrives immediately after this one. Marking it as a milestone is
	// what stops that end event from flushing the verse before its text has
	// been read - the single mistake that turns a milestone file into a Bible
	// with one word per verse.
	c.milestone = attribute(el, "sID") != ""
	// The chapter in the verse's own osisID is authoritative: it is the source
	// telling us where this verse lives, which is more reliable than inferring
	// it from the surrounding elements.
	if len(parts) == 3 {
		if n, err := strconv.Atoi(parts[1]); err == nil {
			c.startChapter(n)
		}
	}
	c.startVerseNumber(num)
	return nil
}

func (c *collector) startVerseNumber(n int) {
	// A continuation of the verse already open: the source split one verse
	// into several milestone elements. Keep accumulating into it instead of
	// starting a second verse with the same number, which would produce a
	// duplicate and lose the text between the two.
	if c.open && c.curVerse != nil && c.curVerse.Number == n {
		return
	}
	c.flush()
	if c.curChapter == nil || n < 0 {
		return
	}
	c.curChapter.Verses = append(c.curChapter.Verses, ParsedVerse{Number: n})
	c.curVerse = &c.curChapter.Verses[len(c.curChapter.Verses)-1]
	c.open = true
	if c.milestone {
		c.t.Milestones++
	} else {
		c.t.Containers++
	}
	c.buf.Reset()
}

// flush writes the accumulated buffer into the current verse.
//
// The text is merged with whatever the verse already holds rather than
// replacing it. A single verse is frequently split across several elements in
// the source - the King James OSIS file breaks a verse into one milestone per
// poetry line or quotation - so replacing on each segment would leave the
// verse holding only its last fragment.
func (c *collector) flush() {
	if !c.open || c.curVerse == nil {
		c.buf.Reset()
		return
	}
	text := foldSpace(c.buf.String())
	if text != "" {
		if c.curVerse.Text == "" {
			c.curVerse.Text = text
		} else {
			c.curVerse.Text = foldSpace(c.curVerse.Text + " " + text)
		}
	}
	c.buf.Reset()
	c.open = false
	c.curVerse = nil
}

func (c *collector) finish() {
	c.flush()
	for i := range c.t.Books {
		b := &c.t.Books[i]
		for j := range b.Chapters {
			// Prune placeholder verses before sorting and indexing.
			kept := b.Chapters[j].Verses[:0]
			for _, v := range b.Chapters[j].Verses {
				if strings.TrimSpace(v.Text) == "" {
					c.t.EmptyVerses++
					if len(c.t.EmptySamples) < 5 {
						c.t.EmptySamples = append(c.t.EmptySamples,
							fmt.Sprintf("%s.%d.%d", b.ID, b.Chapters[j].Number, v.Number))
					}
					continue
				}
				kept = append(kept, v)
			}
			b.Chapters[j].Verses = kept
			verses := b.Chapters[j].Verses
			sort.SliceStable(verses, func(x, y int) bool {
				return verses[x].Number < verses[y].Number
			})
		}
		chapters := b.Chapters
		sort.SliceStable(chapters, func(x, y int) bool {
			return chapters[x].Number < chapters[y].Number
		})
		if len(b.Chapters) > 0 {
			c.t.byBook[b.ID] = b
		}
	}
}

// sortBooks orders the translation canonically, so a reader and a diff of two
// translations agree on what comes first.
func (t *ParsedTranslation) sortBooks() {
	pos := make(map[string]int, len(Canon))
	for i, b := range Canon {
		pos[b.ID] = i
	}
	sort.SliceStable(t.Books, func(i, j int) bool {
		return pos[t.Books[i].ID] < pos[t.Books[j].ID]
	})
	t.byBook = make(map[string]*BookText, len(t.Books))
	for i := range t.Books {
		t.byBook[t.Books[i].ID] = &t.Books[i]
	}
}

// foldSpace collapses every run of whitespace to a single space and trims the
// result. Source files wrap lines wherever they like - often inside a phrase -
// so the whitespace in the file is layout, not text.
func foldSpace(s string) string {
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func attribute(el xml.StartElement, name string) string {
	for _, a := range el.Attr {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func atoiOrZero(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
