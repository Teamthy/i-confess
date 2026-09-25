package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/models"
)

// BibleStore owns the reader: the imported translations, the verses, and the
// two kinds of mark a listener can leave on one.
//
// The registry in internal/bible is the source of truth for *what ships* -
// names, licences, coverage, digests - and this store is the source of truth
// for *what is loaded*. Merging them in one place is deliberate: a version is
// offered in the picker when the registry ships it, and is readable only when
// the importer has loaded it, so a client never opens a translation that has
// no text in it.
//
// Every read of a mark is scoped by user_id in the query itself, not filtered
// afterwards, so a handler that forgets a check returns nothing rather than
// someone else's reading history.
type BibleStore struct{ db *db.DB }

func NewBibleStore(db *db.DB) *BibleStore { return &BibleStore{db: db} }

// versionColumns is the projection every version query returns, in the order
// scanVersion reads. Kept as one constant so the list and the scanner cannot
// drift apart silently.
const versionColumns = `id, name, abbrev, language, language_name, coverage, year,
	licence, licence_url, licence_note, attribution, blob_url,
	book_count, chapter_count, verse_count, is_default, sort_order`

// Versions lists every translation the registry ships, with the counts of what
// is actually loaded.
//
// A registry entry with no rows behind it is still returned, marked
// Imported: false and carrying the licence and coverage it will have once an
// operator loads it. Hiding it would make "which translations does this
// platform offer" depend on an import having been run, and showing it as
// readable would open an empty reader - so the flag is the difference, and the
// client decides how to present it.
func (s *BibleStore) Versions(ctx context.Context) ([]models.BibleVersion, error) {
	loaded := map[string]models.BibleVersion{}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+versionColumns+` FROM bible_versions WHERE deleted_at IS NULL ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		loaded[v.ID] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]models.BibleVersion, 0, len(bible.Versions))
	for _, rv := range bible.Versions {
		v, ok := loaded[rv.ID]
		if !ok {
			v = models.BibleVersion{ID: rv.ID, BookCount: 0, ChapterCount: 0, VerseCount: 0}
		}
		applyRegistry(&v, rv)
		out = append(out, v)
	}
	// A version in the database but not in the registry means someone loaded a
	// source the code no longer ships. Return it rather than dropping it
	// silently: the reader can still open text that is in the database, and an
	// operator needs to see the orphan to decide whether to remove it.
	for id, v := range loaded {
		if _, ok := bible.VersionByID(id); ok {
			continue
		}
		v.Imported = true
		out = append(out, v)
	}
	return out, nil
}

// Version returns one translation, or ErrNotFound when the registry does not
// ship it.
func (s *BibleStore) Version(ctx context.Context, id string) (models.BibleVersion, error) {
	v, ok := bible.VersionByID(strings.ToLower(strings.TrimSpace(id)))
	if !ok {
		return models.BibleVersion{}, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+versionColumns+` FROM bible_versions WHERE id=? AND deleted_at IS NULL`, v.ID)
	loaded, err := scanVersion(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Registered but not imported: everything except the counts is known.
	case err != nil:
		return models.BibleVersion{}, err
	default:
		loaded.ID = v.ID
	}
	applyRegistry(&loaded, v)
	return loaded, nil
}

// applyRegistry fills in the fields that come from code rather than data, and
// marks whether any text is loaded.
func applyRegistry(v *models.BibleVersion, rv bible.Version) {
	v.ID = rv.ID
	v.Name = rv.Name
	v.Abbrev = rv.Abbrev
	v.Language = rv.Language
	v.LanguageName = rv.LanguageName
	v.Coverage = rv.Coverage
	v.CoverageLabel = rv.CoverageLabel()
	v.Licence = rv.Licence
	v.LicenceURL = rv.LicenceURL
	v.LicenceNote = rv.LicenceNote
	v.Attribution = rv.Attribution
	v.BlobURL = rv.BlobURL()
	v.Year = rv.Year
	v.OmittedChapters = rv.OmittedChapters
	v.Default = rv.Default
	v.SortOrder = rv.SortOrder
	v.Imported = v.BookCount > 0 && v.VerseCount > 0
}

type rowScanner interface{ Scan(dest ...any) error }

func scanVersion(sc rowScanner) (models.BibleVersion, error) {
	var v models.BibleVersion
	var year, licenceURL, licenceNote, blobURL sql.NullString
	var isDefault int
	err := sc.Scan(&v.ID, &v.Name, &v.Abbrev, &v.Language, &v.LanguageName, &v.Coverage,
		&year, &v.Licence, &licenceURL, &licenceNote, &v.Attribution, &blobURL,
		&v.BookCount, &v.ChapterCount, &v.VerseCount, &isDefault, &v.SortOrder)
	if err != nil {
		return models.BibleVersion{}, err
	}
	v.Year = year.String
	v.LicenceURL = licenceURL.String
	v.LicenceNote = licenceNote.String
	v.BlobURL = blobURL.String
	v.Default = isDefault != 0
	return v, nil
}

// Books lists the books a translation actually contains, in canonical order.
//
// The counts come from the verses rather than from the registry: the registry
// says what the source file promised, and this says what arrived, which is what
// the reader has to be shown.
func (s *BibleStore) Books(ctx context.Context, versionID string) ([]models.BibleBook, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT book_id, book_name, testament, canonical_order,
		        COUNT(DISTINCT chapter) AS chapters, COUNT(*) AS verses
		   FROM bible_verses
		  WHERE version_id=? AND deleted_at IS NULL
		  GROUP BY book_id, book_name, testament, canonical_order
		  ORDER BY canonical_order`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.BibleBook
	for rows.Next() {
		var b models.BibleBook
		if err := rows.Scan(&b.ID, &b.Name, &b.Testament, &b.Order, &b.Chapters, &b.Verses); err != nil {
			return nil, err
		}
		if cb, ok := bible.BookByID(b.ID); ok {
			b.CanonChapters = cb.Chapters()
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Chapter reads one chapter in order.
func (s *BibleStore) Chapter(ctx context.Context, versionID, book string, chapter int) (models.BibleChapter, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT book_id, book_name, chapter, verse, text
		   FROM bible_verses
		  WHERE version_id=? AND book_id=? AND chapter=? AND deleted_at IS NULL
		  ORDER BY verse`, versionID, book, chapter)
	if err != nil {
		return models.BibleChapter{}, err
	}
	defer rows.Close()

	ch := models.BibleChapter{VersionID: versionID, Book: book, Chapter: chapter}
	for rows.Next() {
		var v models.BibleVerse
		if err := rows.Scan(&v.Book, &v.BookName, &v.Chapter, &v.Verse, &v.Text); err != nil {
			return models.BibleChapter{}, err
		}
		v.VersionID = versionID
		v.Reference = bible.DisplayRef(v.Book, v.Chapter, []int{v.Verse})
		ch.BookName = v.BookName
		ch.Verses = append(ch.Verses, v)
	}
	if err := rows.Err(); err != nil {
		return models.BibleChapter{}, err
	}
	if len(ch.Verses) == 0 {
		return models.BibleChapter{}, ErrNotFound
	}
	prev, next, err := s.neighbours(ctx, versionID, book, chapter)
	if err != nil {
		return models.BibleChapter{}, err
	}
	ch.Prev, ch.Next = prev, next
	return ch, nil
}

// neighbours finds the chapters either side of this one within the translation.
//
// It looks in the book first, which is the common case and one indexed query,
// and only then reaches for the adjacent book. The point of the lookup is that
// the reader can move through Scripture without knowing where a book ends: at
// the last chapter of Malachi, "next" is Matthew.
func (s *BibleStore) neighbours(ctx context.Context, versionID, book string, chapter int) (*models.BibleChapterRef, *models.BibleChapterRef, error) {
	prev, err := s.chapterAt(ctx, versionID, book, chapter, false)
	if err != nil {
		return nil, nil, err
	}
	next, err := s.chapterAt(ctx, versionID, book, chapter, true)
	if err != nil {
		return nil, nil, err
	}
	return prev, next, nil
}

// chapterAt returns the chapter immediately before or after one, spilling into
// the neighbouring book when the current book has none.
func (s *BibleStore) chapterAt(ctx context.Context, versionID, book string, chapter int, forward bool) (*models.BibleChapterRef, error) {
	var order int
	if err := s.db.QueryRowContext(ctx,
		`SELECT canonical_order FROM bible_verses WHERE version_id=? AND book_id=? LIMIT 1`,
		versionID, book).Scan(&order); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	cmp, ord := "<", "DESC"
	if forward {
		cmp, ord = ">", "ASC"
	}
	var ref models.BibleChapterRef
	err := s.db.QueryRowContext(ctx,
		`SELECT book_id, book_name, chapter FROM bible_verses
		  WHERE version_id=? AND book_id=? AND chapter `+cmp+` ? AND deleted_at IS NULL
		  ORDER BY chapter `+ord+` LIMIT 1`, versionID, book, chapter).
		Scan(&ref.Book, &ref.BookName, &ref.Chapter)
	if err == nil {
		return &ref, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// No neighbour inside this book: the next one along is the first chapter of
	// the adjacent book, in either direction.
	bord, border := "<", "DESC"
	if forward {
		bord, border = ">", "ASC"
	}
	var other struct {
		id, name string
		order    int
	}
	err = s.db.QueryRowContext(ctx,
		`SELECT book_id, book_name, canonical_order FROM bible_verses
		  WHERE version_id=? AND canonical_order `+bord+` ? AND deleted_at IS NULL
		  GROUP BY book_id, book_name, canonical_order
		  ORDER BY canonical_order `+border+` LIMIT 1`, versionID, order).
		Scan(&other.id, &other.name, &other.order)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	chapterSQL := `SELECT MIN(chapter) FROM bible_verses`
	if !forward {
		chapterSQL = `SELECT MAX(chapter) FROM bible_verses`
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		chapterSQL+` WHERE version_id=? AND book_id=? AND deleted_at IS NULL`,
		versionID, other.id).Scan(&n); err != nil {
		return nil, err
	}
	return &models.BibleChapterRef{Book: other.id, BookName: other.name, Chapter: n}, nil
}

// Verse reads a single verse.
func (s *BibleStore) Verse(ctx context.Context, versionID, book string, chapter, verse int) (models.BibleVerse, error) {
	var v models.BibleVerse
	err := s.db.QueryRowContext(ctx,
		`SELECT book_id, book_name, chapter, verse, text FROM bible_verses
		  WHERE version_id=? AND book_id=? AND chapter=? AND verse=? AND deleted_at IS NULL`,
		versionID, book, chapter, verse).
		Scan(&v.Book, &v.BookName, &v.Chapter, &v.Verse, &v.Text)
	if errors.Is(err, sql.ErrNoRows) {
		return models.BibleVerse{}, ErrNotFound
	}
	if err != nil {
		return models.BibleVerse{}, err
	}
	v.VersionID = versionID
	v.Reference = bible.DisplayRef(v.Book, v.Chapter, []int{v.Verse})
	return v, nil
}

// VerseExists reports whether a translation actually has a coordinate, which is
// what stops a highlight being saved against a verse no translation contains.
func (s *BibleStore) VerseExists(ctx context.Context, versionID, book string, chapter, verse int) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM bible_verses
		  WHERE version_id=? AND book_id=? AND chapter=? AND verse=? AND deleted_at IS NULL LIMIT 1`,
		versionID, book, chapter, verse).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ConfessionsForVerse returns the confessions that cite a verse.
//
// This is one direction of the link between Scripture and the library: the
// reader is looking at a verse and wants the prayer that quotes it. The
// citation is a range in the corpus ("22-24"), so the test is interval
// containment rather than equality - asking only for verse_start = verse would
// hide exactly the confessions that quote a passage.
func (s *BibleStore) ConfessionsForVerse(ctx context.Context, book string, chapter, verse int) ([]models.VerseConfession, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.title, c.category_id,
		        COALESCE(cat.name,''), COALESCE(cat.slug,''),
		        sr.book, sr.chapter, sr.verse, sr.is_direct_quote,
		        EXISTS (SELECT 1 FROM audio_assets a
		                 WHERE a.content_id = c.id AND a.status = 'published'
		                   AND a.deleted_at IS NULL) AS has_audio
		   FROM scripture_references sr
		   JOIN confessions c ON c.id = sr.confession_id AND c.deleted_at IS NULL
		   LEFT JOIN categories cat ON cat.id = c.category_id
		  WHERE sr.book_id = ? AND sr.chapter = ?
		    AND COALESCE(sr.verse_start,0) <= ? AND COALESCE(sr.verse_end, sr.verse_start) >= ?
		    AND c.status = 'published'
		  ORDER BY cat.sort_order, cat.name, c.title`, book, chapter, verse, verse)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.VerseConfession
	for rows.Next() {
		var vc models.VerseConfession
		var refBook, refVerse string
		var refChapter int
		var direct int
		if err := rows.Scan(&vc.ConfessionID, &vc.Title, &vc.CategoryID, &vc.CategoryName,
			&vc.CategorySlug, &refBook, &refChapter, &refVerse, &direct, &vc.HasAudio); err != nil {
			return nil, err
		}
		vc.IsDirectQuote = direct != 0
		vc.Speakable = !vc.HasAudio
		// The reference is rendered through the same normaliser the deep link
		// uses, so the label and the destination cannot disagree.
		if ref, err := bible.NewReference(refBook, refChapter, refVerse); err == nil {
			vc.Reference = ref.Display
		} else {
			vc.Reference = fmt.Sprintf("%s %d:%s", refBook, refChapter, refVerse)
		}
		out = append(out, vc)
	}
	return out, rows.Err()
}

// ConfessionsInChapter returns every published confession that cites anything
// in a chapter, keyed by verse number.
//
// The chapter view uses it to mark the verses that have a confessional use
// before the reader taps one. A citation of a range is expanded to the verses
// it covers, so a confession citing Psalm 103:2-3 marks both verses.
func (s *BibleStore) ConfessionsInChapter(ctx context.Context, book string, chapter int) (map[int][]models.VerseConfession, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.title, c.category_id,
		        COALESCE(cat.name,''), COALESCE(cat.slug,''),
		        sr.book, sr.chapter, sr.verse, sr.is_direct_quote,
		        COALESCE(sr.verse_start, 0), COALESCE(sr.verse_end, sr.verse_start),
		        EXISTS (SELECT 1 FROM audio_assets a
		                 WHERE a.content_id = c.id AND a.status = 'published'
		                   AND a.deleted_at IS NULL) AS has_audio
		   FROM scripture_references sr
		   JOIN confessions c ON c.id = sr.confession_id AND c.deleted_at IS NULL
		   LEFT JOIN categories cat ON cat.id = c.category_id
		  WHERE sr.book_id = ? AND sr.chapter = ? AND c.status = 'published'
		  ORDER BY COALESCE(sr.verse_start,0), c.title`, book, chapter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int][]models.VerseConfession{}
	for rows.Next() {
		var vc models.VerseConfession
		var refBook, refVerse string
		var refChapter, start, end int
		var direct int
		if err := rows.Scan(&vc.ConfessionID, &vc.Title, &vc.CategoryID, &vc.CategoryName,
			&vc.CategorySlug, &refBook, &refChapter, &refVerse, &direct, &start, &end, &vc.HasAudio); err != nil {
			return nil, err
		}
		vc.IsDirectQuote = direct != 0
		vc.Speakable = !vc.HasAudio
		if ref, err := bible.NewReference(refBook, refChapter, refVerse); err == nil {
			vc.Reference = ref.Display
		}
		// A range is capped so a malformed citation cannot expand into
		// thousands of map entries.
		if end < start {
			end = start
		}
		if end-start > 200 {
			end = start + 200
		}
		for v := start; v <= end; v++ {
			if v < 1 {
				continue
			}
			out[v] = append(out[v], vc)
		}
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Highlights
// ---------------------------------------------------------------------------

// UpsertHighlight saves a highlight, replacing the colour of an existing one.
//
// Idempotent on the natural key: marking a verse twice with the same colour is
// one highlight, and marking it with a second colour changes the first rather
// than stacking a row the reader cannot see. The partial unique index in the
// schema enforces the same rule, so two concurrent taps cannot both insert.
func (s *BibleStore) UpsertHighlight(ctx context.Context, userID string, h *models.VerseHighlight) error {
	if h.ID == "" {
		h.ID = newID()
	}
	h.CreatedAt, h.UpdatedAt = now(), now()
	return s.db.QueryRowContext(ctx,
		`INSERT INTO verse_highlights
		    (id,user_id,version_id,book_id,chapter,verse,color,created_at,updated_at,row_version)
		 VALUES (?,?,?,?,?,?,?,?,?,1)
		 ON CONFLICT (user_id,version_id,book_id,chapter,verse) WHERE deleted_at IS NULL
		 DO UPDATE SET color = EXCLUDED.color, updated_at = EXCLUDED.updated_at,
		               row_version = verse_highlights.row_version + 1
		 RETURNING id, created_at, updated_at`,
		h.ID, userID, h.VersionID, h.Book, h.Chapter, h.Verse, nullIfEmpty(h.Color), h.CreatedAt, h.UpdatedAt).
		Scan(&h.ID, &h.CreatedAt, &h.UpdatedAt)
}

// DeleteHighlight removes a highlight. Deleting one that is not there is not an
// error: the gesture is "this verse is not highlighted", and that is already
// true.
func (s *BibleStore) DeleteHighlight(ctx context.Context, userID, versionID, book string, chapter, verse int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE verse_highlights SET deleted_at=?, updated_at=?, row_version=row_version+1
		  WHERE user_id=? AND version_id=? AND book_id=? AND chapter=? AND verse=? AND deleted_at IS NULL`,
		now(), now(), userID, versionID, book, chapter, verse)
	return err
}

// ListHighlights returns a user's highlights, newest first, optionally scoped
// to one version, book or chapter.
func (s *BibleStore) ListHighlights(ctx context.Context, userID, versionID, book string, chapter int) ([]models.VerseHighlight, error) {
	q := `SELECT h.id, h.version_id, h.book_id, COALESCE(v.book_name, h.book_id),
	             h.chapter, h.verse, COALESCE(h.color,''), COALESCE(v.text,''),
	             h.created_at, h.updated_at
	        FROM verse_highlights h
	        LEFT JOIN bible_verses v
	               ON v.version_id = h.version_id AND v.book_id = h.book_id
	              AND v.chapter = h.chapter AND v.verse = h.verse AND v.deleted_at IS NULL
	       WHERE h.user_id = ? AND h.deleted_at IS NULL`
	args := []any{userID}
	if versionID != "" {
		q += ` AND h.version_id = ?`
		args = append(args, versionID)
	}
	if book != "" {
		q += ` AND h.book_id = ?`
		args = append(args, book)
	}
	if chapter > 0 {
		q += ` AND h.chapter = ?`
		args = append(args, chapter)
	}
	q += ` ORDER BY h.created_at DESC, h.id DESC`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.VerseHighlight
	for rows.Next() {
		var h models.VerseHighlight
		if err := rows.Scan(&h.ID, &h.VersionID, &h.Book, &h.BookName, &h.Chapter,
			&h.Verse, &h.Color, &h.Text, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, err
		}
		h.Reference = bible.DisplayRef(h.Book, h.Chapter, []int{h.Verse})
		out = append(out, h)
	}
	return out, rows.Err()
}

// HighlightsInChapter returns the highlighted verse numbers of one chapter,
// keyed by verse, for the reader to paint as it renders.
func (s *BibleStore) HighlightsInChapter(ctx context.Context, userID, versionID, book string, chapter int) (map[int]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT verse, COALESCE(color,'') FROM verse_highlights
		  WHERE user_id=? AND version_id=? AND book_id=? AND chapter=? AND deleted_at IS NULL`,
		userID, versionID, book, chapter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int]string{}
	for rows.Next() {
		var verse int
		var color string
		if err := rows.Scan(&verse, &color); err != nil {
			return nil, err
		}
		out[verse] = color
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Bookmarks
// ---------------------------------------------------------------------------

// UpsertBookmark saves a bookmark, updating the label and note of an existing
// one. A second tap on the same verse is an edit, not a duplicate.
func (s *BibleStore) UpsertBookmark(ctx context.Context, userID string, b *models.VerseBookmark) error {
	if b.ID == "" {
		b.ID = newID()
	}
	b.CreatedAt, b.UpdatedAt = now(), now()
	return s.db.QueryRowContext(ctx,
		`INSERT INTO verse_bookmarks
		    (id,user_id,version_id,book_id,chapter,verse,label,note,created_at,updated_at,row_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,1)
		 ON CONFLICT (user_id,version_id,book_id,chapter,verse) WHERE deleted_at IS NULL
		 DO UPDATE SET label = EXCLUDED.label, note = EXCLUDED.note,
		               updated_at = EXCLUDED.updated_at,
		               row_version = verse_bookmarks.row_version + 1
		 RETURNING id, created_at, updated_at`,
		b.ID, userID, b.VersionID, b.Book, b.Chapter, b.Verse,
		nullIfEmpty(b.Label), nullIfEmpty(b.Note), b.CreatedAt, b.UpdatedAt).
		Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
}

// DeleteBookmark removes a bookmark, idempotently for the same reason a
// highlight delete is.
func (s *BibleStore) DeleteBookmark(ctx context.Context, userID, versionID, book string, chapter, verse int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE verse_bookmarks SET deleted_at=?, updated_at=?, row_version=row_version+1
		  WHERE user_id=? AND version_id=? AND book_id=? AND chapter=? AND verse=? AND deleted_at IS NULL`,
		now(), now(), userID, versionID, book, chapter, verse)
	return err
}

// ListBookmarks returns a user's bookmarks with the verse text resolved, in
// canonical order rather than insertion order: a list of saved verses reads
// like Scripture, not like a log of taps.
func (s *BibleStore) ListBookmarks(ctx context.Context, userID, versionID, book string, chapter, limit int) ([]models.VerseBookmark, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	q := `SELECT b.id, b.version_id, b.book_id, COALESCE(v.book_name, b.book_id),
	             b.chapter, b.verse, COALESCE(b.label,''), COALESCE(b.note,''),
	             COALESCE(v.text,''), b.created_at, b.updated_at,
	             COALESCE(v.canonical_order, 999)
	        FROM verse_bookmarks b
	        LEFT JOIN bible_verses v
	               ON v.version_id = b.version_id AND v.book_id = b.book_id
	              AND v.chapter = b.chapter AND v.verse = b.verse AND v.deleted_at IS NULL
	       WHERE b.user_id = ? AND b.deleted_at IS NULL`
	args := []any{userID}
	if versionID != "" {
		q += ` AND b.version_id = ?`
		args = append(args, versionID)
	}
	if book != "" {
		q += ` AND b.book_id = ?`
		args = append(args, book)
	}
	if chapter > 0 {
		q += ` AND b.chapter = ?`
		args = append(args, chapter)
	}
	q += ` ORDER BY COALESCE(v.canonical_order, 999), b.chapter, b.verse LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.VerseBookmark
	for rows.Next() {
		var b models.VerseBookmark
		var order int
		if err := rows.Scan(&b.ID, &b.VersionID, &b.Book, &b.BookName, &b.Chapter,
			&b.Verse, &b.Label, &b.Note, &b.Text, &b.CreatedAt, &b.UpdatedAt, &order); err != nil {
			return nil, err
		}
		b.Reference = bible.DisplayRef(b.Book, b.Chapter, []int{b.Verse})
		out = append(out, b)
	}
	return out, rows.Err()
}

// BookmarksInChapter returns the bookmarked verse numbers of one chapter.
func (s *BibleStore) BookmarksInChapter(ctx context.Context, userID, versionID, book string, chapter int) (map[int]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT verse FROM verse_bookmarks
		  WHERE user_id=? AND version_id=? AND book_id=? AND chapter=? AND deleted_at IS NULL`,
		userID, versionID, book, chapter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int]bool{}
	for rows.Next() {
		var verse int
		if err := rows.Scan(&verse); err != nil {
			return nil, err
		}
		out[verse] = true
	}
	return out, rows.Err()
}
