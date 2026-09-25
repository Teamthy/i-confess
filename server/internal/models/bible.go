package models

// Scripture: versions, books, verses, and the reader's own marks on them.
//
// Two ideas hold these types together.
//
// The first is that a verse is identified by four coordinates - version, book,
// chapter, verse - and never by a display string. "Psalm 103:2", "Ps.103.2" and
// "PSA 103 2" are the same verse, and the only place that spelling is decided is
// internal/bible's normaliser. Everything here carries the canonical OSIS book
// id ("Ps") alongside the display name, so a client can render a reference and
// route to it without its own name table.
//
// The second is that a mark on a verse belongs to a translation. The KJV's
// words for John 3:16 and the Swahili New Testament's are different words, so a
// highlight is scoped to the version it was made in, not to the coordinates
// alone. A reader who switches translation keeps both, and each is shown
// against the text it was made against.

// BibleVersion is one translation, as the reader's picker needs it.
type BibleVersion struct {
	ID string `json:"id"`
	// Name and Abbrev are what the picker and the reference badges show.
	Name   string `json:"name"`
	Abbrev string `json:"abbrev"`
	// Language is ISO 639-1 and LanguageName is its display form.
	Language     string `json:"language"`
	LanguageName string `json:"language_name"`
	// Coverage is "full" or "new_testament"; CoverageLabel is the sentence a
	// client can display directly ("New Testament", "Old and New Testaments").
	Coverage      string `json:"coverage"`
	CoverageLabel string `json:"coverage_label"`
	// The licence travels with the version because it is a condition of
	// shipping the text at all, and a reader is entitled to see it.
	Licence     string `json:"licence"`
	LicenceURL  string `json:"licence_url,omitempty"`
	LicenceNote string `json:"licence_note,omitempty"`
	// Attribution names the upstream file the text came from.
	Attribution string `json:"attribution"`
	BlobURL     string `json:"blob_url,omitempty"`
	Year        string `json:"year,omitempty"`
	// BookCount, ChapterCount and VerseCount describe what is *loaded*, which
	// for a translation that has not been imported yet is zero.
	BookCount    int `json:"book_count"`
	ChapterCount int `json:"chapter_count"`
	VerseCount   int `json:"verse_count"`
	// Imported is false when the registry ships the version but no operator
	// has loaded the text yet. The picker shows it as unavailable rather than
	// opening an empty reader.
	Imported bool `json:"imported"`
	// OmittedChapters names chapters the source is missing, e.g. ["Matt.23"].
	OmittedChapters []string `json:"omitted_chapters,omitempty"`
	Default         bool     `json:"default"`
	SortOrder       int      `json:"sort_order"`
}

// BibleBook is one book of one translation.
type BibleBook struct {
	// ID is the canonical OSIS identifier, e.g. "1Pet".
	ID   string `json:"id"`
	Name string `json:"name"`
	// Testament is "old" or "new".
	Testament string `json:"testament"`
	// Order is the book's 1-based position in the canon.
	Order int `json:"order"`
	// Chapters is how many chapters this translation actually has, which is
	// less than CanonChapters when the source is missing one.
	Chapters      int `json:"chapters"`
	CanonChapters int `json:"canon_chapters"`
	// Verses is the translation's total for the book.
	Verses int `json:"verses"`
}

// BibleChapter is one chapter with its verses in order.
type BibleChapter struct {
	VersionID string       `json:"version_id"`
	Book      string       `json:"book"`
	BookName  string       `json:"book_name"`
	Chapter   int          `json:"chapter"`
	Verses    []BibleVerse `json:"verses"`
	// Prev and Next are the neighbouring chapters in this translation, so the
	// reader can page through Scripture without knowing the canon by heart.
	// Either is nil at the ends of the translation.
	Prev *BibleChapterRef `json:"prev,omitempty"`
	Next *BibleChapterRef `json:"next,omitempty"`
}

// BibleChapterRef points at a chapter without carrying its text.
type BibleChapterRef struct {
	Book     string `json:"book"`
	BookName string `json:"book_name"`
	Chapter  int    `json:"chapter"`
}

// BibleVerse is one verse of one translation, with the reader's own state on it
// when the request was authenticated.
type BibleVerse struct {
	VersionID string `json:"version_id"`
	Book      string `json:"book"`
	BookName  string `json:"book_name"`
	Chapter   int    `json:"chapter"`
	Verse     int    `json:"verse"`
	Text      string `json:"text"`
	// Reference is the display form, e.g. "1 Peter 2:24".
	Reference string `json:"reference"`
	// Highlight and Bookmarked describe this user's state. Absent for an
	// anonymous reader, who has none.
	Highlight  *VerseHighlight `json:"highlight,omitempty"`
	Bookmarked bool            `json:"bookmarked"`
	// Confessions are the confessions that cite this verse. It is embedded
	// rather than fetched separately because it is the point of the verse
	// page: a verse that has a confessional use must say so where the reader
	// is looking.
	Confessions []VerseConfession `json:"confessions,omitempty"`
}

// VerseConfession is a confession that cites a verse, with the category it
// belongs to - the link the reader taps to go from Scripture to a prayer.
type VerseConfession struct {
	ConfessionID string `json:"confession_id"`
	Title        string `json:"title"`
	CategoryID   string `json:"category_id,omitempty"`
	CategoryName string `json:"category_name,omitempty"`
	CategorySlug string `json:"category_slug,omitempty"`
	// Reference is how *this confession* cites the verse, which may be a range
	// ("Psalm 103:2-3") rather than the single verse being read.
	Reference     string `json:"reference"`
	IsDirectQuote bool   `json:"is_direct_quote"`
	// HasAudio reports whether the confession can be listened to, so the
	// client knows whether to offer playback before it asks for a stream.
	HasAudio bool `json:"has_audio"`
	// Speakable reports whether the confession can be spoken on demand by the
	// voice pipeline when it has no recording.
	Speakable bool `json:"speakable"`
}

// VerseHighlight is one saved mark.
type VerseHighlight struct {
	ID        string `json:"id"`
	VersionID string `json:"version_id"`
	Book      string `json:"book"`
	BookName  string `json:"book_name,omitempty"`
	Chapter   int    `json:"chapter"`
	Verse     int    `json:"verse"`
	// Color is one of the palette the schema constrains; empty means the
	// reader's default colour.
	Color     string `json:"color,omitempty"`
	Reference string `json:"reference,omitempty"`
	// Text is the verse text at the time it is listed, so a saved-mark list
	// reads as something other than coordinates.
	Text      string `json:"text,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// VerseBookmark is one saved verse with the reader's own label and note.
type VerseBookmark struct {
	ID        string `json:"id"`
	VersionID string `json:"version_id"`
	Book      string `json:"book"`
	BookName  string `json:"book_name,omitempty"`
	Chapter   int    `json:"chapter"`
	Verse     int    `json:"verse"`
	Label     string `json:"label,omitempty"`
	Note      string `json:"note,omitempty"`
	Reference string `json:"reference,omitempty"`
	Text      string `json:"text,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// HighlightColors is the palette the schema accepts. The list is duplicated in
// the database CHECK constraint deliberately: the constraint stops a bad value
// reaching storage, and this stops a bad value reaching the constraint, where
// the error would surface as a 500 instead of a 400.
var HighlightColors = []string{"yellow", "green", "blue", "pink", "purple"}

// ValidHighlightColor reports whether a colour is in the palette.
func ValidHighlightColor(c string) bool {
	for _, allowed := range HighlightColors {
		if c == allowed {
			return true
		}
	}
	return false
}
