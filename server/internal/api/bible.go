package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

// The reader's HTTP surface.
//
// Two rules shaped it.
//
// The first is that a reference is a *coordinate*, never a string to be parsed
// by a client. Every route takes the canonical OSIS book id and integer chapter
// and verse, and the version id it belongs to, so "Psalm 103:2" reaches the
// server as Ps / 103 / 2 and comes back with the display form already rendered.
// A client that built its own reference strings would need its own book table,
// its own abbreviation rules and its own idea of what "Song of Solomon" is
// called; the three would drift, and the drift would show up as deep links that
// open the wrong verse.
//
// The second is that the public read path carries no personal data at all. A
// chapter is the same for every reader, so it is served without authentication
// and cached as such. The reader's own highlights and bookmarks are a separate,
// session-validated surface under /me/bible/*, which is why a revoked session
// cannot read a highlight even though it could still read the verse it was made
// against.

// registerBibleRoutes wires the reader under both the unprefixed and the /v1
// prefix. The parity test in this package fails the build if only one is
// registered.
func (h *Handler) registerBibleRoutes(mux *http.ServeMux, authed func(http.Handler) http.Handler) {
	for _, p := range []string{"", "/v1"} {
		// Public reading.
		h.route(mux, "GET "+p+"/bible/versions", "public", "bible",
			"Translations available to the reader, with licence and coverage", nil, h.listBibleVersions)
		h.route(mux, "GET "+p+"/bible/versions/{id}", "public", "bible",
			"One translation", nil, h.getBibleVersion)
		h.route(mux, "GET "+p+"/bible/versions/{id}/books", "public", "bible",
			"Books a translation contains, in canonical order", nil, h.listBibleBooks)
		h.route(mux, "GET "+p+"/bible/versions/{id}/books/{book}/chapters/{chapter}", "public", "bible",
			"One chapter with its verses and confessional cross-links", nil, h.getBibleChapter)
		h.route(mux, "GET "+p+"/bible/versions/{id}/passages/{ref}", "public", "bible",
			"A passage by reference, e.g. 1Pet.2.24 or Ps.103.2-3", nil, h.getBiblePassage)
		h.route(mux, "GET "+p+"/bible/verses/{book}/{chapter}/{verse}/confessions", "public", "bible",
			"Confessions that cite a verse, with their categories", nil, h.listVerseConfessions)

		// The listener's own marks. Session-validated: highlights and
		// bookmarks are personal data, and a stale token must not read them.
		h.route(mux, "GET "+p+"/me/bible/highlights", "user", "bible",
			"List highlights, optionally for one version, book or chapter", authed, h.listHighlights)
		h.route(mux, "PUT "+p+"/me/bible/highlights/{version}/{book}/{chapter}/{verse}", "user", "bible",
			"Highlight a verse, or change its colour", authed, h.putHighlight)
		h.route(mux, "DELETE "+p+"/me/bible/highlights/{version}/{book}/{chapter}/{verse}", "user", "bible",
			"Clear a highlight", authed, h.deleteHighlight)
		h.route(mux, "GET "+p+"/me/bible/bookmarks", "user", "bible",
			"List bookmarks in canonical order", authed, h.listBookmarks)
		h.route(mux, "PUT "+p+"/me/bible/bookmarks/{version}/{book}/{chapter}/{verse}", "user", "bible",
			"Bookmark a verse with an optional label and note", authed, h.putBookmark)
		h.route(mux, "DELETE "+p+"/me/bible/bookmarks/{version}/{book}/{chapter}/{verse}", "user", "bible",
			"Remove a bookmark", authed, h.deleteBookmark)
	}
}

// ---------------------------------------------------------------------------
// Versions and books
// ---------------------------------------------------------------------------

func (h *Handler) listBibleVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := h.bible.Versions(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load translations")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"versions":  versions,
		"default":   bible.DefaultVersion().ID,
		"languages": bible.Languages(),
	})
}

func (h *Handler) getBibleVersion(w http.ResponseWriter, r *http.Request) {
	v, err := h.bible.Version(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "BIBLE_VERSION_UNKNOWN", "no such translation")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load the translation")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

func (h *Handler) listBibleBooks(w http.ResponseWriter, r *http.Request) {
	version, ok := h.bibleVersionOr404(w, r)
	if !ok {
		return
	}
	books, err := h.bible.Books(r.Context(), version.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load books")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"version": version,
		"books":   books,
	})
}

// ---------------------------------------------------------------------------
// Reading
// ---------------------------------------------------------------------------

func (h *Handler) getBibleChapter(w http.ResponseWriter, r *http.Request) {
	version, ok := h.bibleVersionOr404(w, r)
	if !ok {
		return
	}
	book, ok := h.bibleBookOr400(w, r.PathValue("book"))
	if !ok {
		return
	}
	chapter, err := strconv.Atoi(r.PathValue("chapter"))
	if err != nil || chapter < 1 || chapter > 150 {
		writeCode(w, http.StatusBadRequest, "BIBLE_REFERENCE_INVALID", "chapter must be a number between 1 and 150")
		return
	}

	ch, err := h.bible.Chapter(r.Context(), version.ID, book, chapter)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "BIBLE_CHAPTER_UNAVAILABLE",
			"this translation does not contain that chapter")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load the chapter")
		return
	}

	// Which verses of this chapter have a confessional use. One query for the
	// chapter rather than one per verse: the badge has to be on the verse
	// before the reader taps it, and a per-verse lookup would be a query per
	// verse on the most-read endpoint in the feature.
	links, err := h.bible.ConfessionsInChapter(r.Context(), book, chapter)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load cross-links")
		return
	}
	for i := range ch.Verses {
		if cs, ok := links[ch.Verses[i].Verse]; ok {
			ch.Verses[i].Confessions = cs
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"version": version,
		"chapter": ch,
	})
}

// getBiblePassage resolves a reference like "1Pet.2.24", "Gen.1" or
// "Ps.103.2-3" into verses, with the confessions that cite them.
//
// The reference is expanded through the same normaliser a confession's own
// citation goes through, so a deep link from the library and a request typed by
// hand cannot resolve to different verses.
func (h *Handler) getBiblePassage(w http.ResponseWriter, r *http.Request) {
	version, ok := h.bibleVersionOr404(w, r)
	if !ok {
		return
	}
	ref := strings.TrimSpace(r.PathValue("ref"))
	parts := strings.Split(ref, ".")
	if len(parts) < 2 {
		writeCode(w, http.StatusBadRequest, "BIBLE_REFERENCE_INVALID", "use Book.Chapter or Book.Chapter.Verse")
		return
	}
	book, ok := h.bibleBookOr400(w, parts[0])
	if !ok {
		return
	}
	chapter, err := strconv.Atoi(parts[1])
	if err != nil || chapter < 1 {
		writeCode(w, http.StatusBadRequest, "BIBLE_REFERENCE_INVALID", "chapter must be a positive number")
		return
	}

	// Book.Chapter is a whole chapter; Book.Chapter.Verse[-Verse] is a passage.
	if len(parts) == 2 {
		h.writeChapterAsPassage(w, r, version, book, chapter)
		return
	}

	nums, ok := bible.ParseVerseSpec(parts[2])
	if !ok {
		writeCode(w, http.StatusBadRequest, "BIBLE_REFERENCE_INVALID", "unreadable verse reference: "+parts[2])
		return
	}

	verses := make([]models.BibleVerse, 0, len(nums))
	for _, n := range nums {
		v, err := h.bible.Verse(r.Context(), version.ID, book, chapter, n)
		if errors.Is(err, store.ErrNotFound) {
			// A range may legitimately span a verse a translation does not
			// have - the World English Bible omits Luke 17:36 - so a missing
			// verse inside a range is skipped rather than turning the whole
			// range into a 404.
			continue
		}
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "failed to load the passage")
			return
		}
		links, err := h.bible.ConfessionsForVerse(r.Context(), book, chapter, n)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "failed to load cross-links")
			return
		}
		v.Confessions = links
		verses = append(verses, v)
	}
	if len(verses) == 0 {
		writeCode(w, http.StatusNotFound, "BIBLE_VERSE_UNAVAILABLE",
			"this translation does not contain that verse")
		return
	}
	numbers := make([]int, 0, len(verses))
	for _, v := range verses {
		numbers = append(numbers, v.Verse)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"version":   version,
		"book":      book,
		"book_name": verses[0].BookName,
		"chapter":   chapter,
		"reference": bible.DisplayRef(book, chapter, numbers),
		"verses":    verses,
	})
}

// writeChapterAsPassage answers a chapter-shaped passage with the chapter
// payload, so a client that deep-links to "Ps.103" gets the same shape as one
// that asked for the chapter directly.
func (h *Handler) writeChapterAsPassage(w http.ResponseWriter, r *http.Request, version models.BibleVersion, book string, chapter int) {
	ch, err := h.bible.Chapter(r.Context(), version.ID, book, chapter)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "BIBLE_CHAPTER_UNAVAILABLE", "this translation does not contain that chapter")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load the chapter")
		return
	}
	links, err := h.bible.ConfessionsInChapter(r.Context(), book, chapter)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load cross-links")
		return
	}
	for i := range ch.Verses {
		if cs, ok := links[ch.Verses[i].Verse]; ok {
			ch.Verses[i].Confessions = cs
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"version": version,
		"chapter": ch,
	})
}

// listVerseConfessions is the verse-first direction of the cross-link: given a
// verse, the confessions that cite it. It is version-independent, because a
// confession cites a reference, not a translation.
func (h *Handler) listVerseConfessions(w http.ResponseWriter, r *http.Request) {
	book, ok := h.bibleBookOr400(w, r.PathValue("book"))
	if !ok {
		return
	}
	chapter, err := strconv.Atoi(r.PathValue("chapter"))
	if err != nil || chapter < 1 {
		writeCode(w, http.StatusBadRequest, "BIBLE_REFERENCE_INVALID", "chapter must be a positive number")
		return
	}
	verse, err := strconv.Atoi(r.PathValue("verse"))
	if err != nil || verse < 1 {
		writeCode(w, http.StatusBadRequest, "BIBLE_REFERENCE_INVALID", "verse must be a positive number")
		return
	}
	links, err := h.bible.ConfessionsForVerse(r.Context(), book, chapter, verse)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confessions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"book":        book,
		"chapter":     chapter,
		"verse":       verse,
		"reference":   bible.DisplayRef(book, chapter, []int{verse}),
		"confessions": links,
	})
}

// ---------------------------------------------------------------------------
// Highlights
// ---------------------------------------------------------------------------

func (h *Handler) listHighlights(w http.ResponseWriter, r *http.Request) {
	version := strings.TrimSpace(r.URL.Query().Get("version_id"))
	book := ""
	if raw := strings.TrimSpace(r.URL.Query().Get("book")); raw != "" {
		parsed, ok := h.bibleBookOr400(w, raw)
		if !ok {
			return
		}
		book = parsed
	}
	chapter := atoiQuery(r, "chapter")
	highlights, err := h.bible.ListHighlights(r.Context(), h.userID(r), version, book, chapter)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load highlights")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"highlights": highlights,
		"colors":     models.HighlightColors,
	})
}

func (h *Handler) putHighlight(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)
	version, book, chapter, verse, ok := h.bibleCoordinates(w, r)
	if !ok {
		return
	}
	var req struct {
		Color string `json:"color"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeCode(w, http.StatusBadRequest, "BIBLE_HIGHLIGHT_INVALID", "invalid request body")
		return
	}
	color := strings.ToLower(strings.TrimSpace(req.Color))
	if color == "" {
		// A tap with no colour chosen means the reader's default rather than
		// an invisible mark.
		color = "yellow"
	}
	if !models.ValidHighlightColor(color) {
		writeCode(w, http.StatusBadRequest, "BIBLE_HIGHLIGHT_INVALID",
			"color must be one of "+strings.Join(models.HighlightColors, ", "))
		return
	}
	if exists, err := h.bible.VerseExists(r.Context(), version, book, chapter, verse); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save the highlight")
		return
	} else if !exists {
		writeCode(w, http.StatusNotFound, "BIBLE_VERSE_UNAVAILABLE", "this translation does not contain that verse")
		return
	}

	highlight := &models.VerseHighlight{
		VersionID: version, Book: book, Chapter: chapter, Verse: verse, Color: color,
	}
	if err := h.bible.UpsertHighlight(r.Context(), userID, highlight); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save the highlight")
		return
	}
	highlight.Reference = bible.DisplayRef(book, chapter, []int{verse})
	httpx.WriteJSON(w, http.StatusOK, highlight)
}

func (h *Handler) deleteHighlight(w http.ResponseWriter, r *http.Request) {
	version, book, chapter, verse, ok := h.bibleCoordinates(w, r)
	if !ok {
		return
	}
	if err := h.bible.DeleteHighlight(r.Context(), h.userID(r), version, book, chapter, verse); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to clear the highlight")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Bookmarks
// ---------------------------------------------------------------------------

func (h *Handler) listBookmarks(w http.ResponseWriter, r *http.Request) {
	version := strings.TrimSpace(r.URL.Query().Get("version_id"))
	book := ""
	if raw := strings.TrimSpace(r.URL.Query().Get("book")); raw != "" {
		parsed, ok := h.bibleBookOr400(w, raw)
		if !ok {
			return
		}
		book = parsed
	}
	bookmarks, err := h.bible.ListBookmarks(r.Context(), h.userID(r), version, book,
		atoiQuery(r, "chapter"), atoiQuery(r, "limit"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load bookmarks")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"bookmarks": bookmarks,
	})
}

func (h *Handler) putBookmark(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)
	version, book, chapter, verse, ok := h.bibleCoordinates(w, r)
	if !ok {
		return
	}
	var req struct {
		Label string `json:"label"`
		Note  string `json:"note"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeCode(w, http.StatusBadRequest, "BIBLE_BOOKMARK_INVALID", "invalid request body")
		return
	}
	label := strings.TrimSpace(req.Label)
	note := strings.TrimSpace(req.Note)
	if utf8.RuneCountInString(label) > 80 {
		writeCode(w, http.StatusBadRequest, "BIBLE_BOOKMARK_INVALID", "label must be 80 characters or fewer")
		return
	}
	if utf8.RuneCountInString(note) > 500 {
		writeCode(w, http.StatusBadRequest, "BIBLE_BOOKMARK_INVALID", "note must be 500 characters or fewer")
		return
	}
	if exists, err := h.bible.VerseExists(r.Context(), version, book, chapter, verse); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save the bookmark")
		return
	} else if !exists {
		writeCode(w, http.StatusNotFound, "BIBLE_VERSE_UNAVAILABLE", "this translation does not contain that verse")
		return
	}

	bookmark := &models.VerseBookmark{
		VersionID: version, Book: book, Chapter: chapter, Verse: verse, Label: label, Note: note,
	}
	if err := h.bible.UpsertBookmark(r.Context(), userID, bookmark); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save the bookmark")
		return
	}
	bookmark.Reference = bible.DisplayRef(book, chapter, []int{verse})
	httpx.WriteJSON(w, http.StatusOK, bookmark)
}

func (h *Handler) deleteBookmark(w http.ResponseWriter, r *http.Request) {
	version, book, chapter, verse, ok := h.bibleCoordinates(w, r)
	if !ok {
		return
	}
	if err := h.bible.DeleteBookmark(r.Context(), h.userID(r), version, book, chapter, verse); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to remove the bookmark")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Shared validation
// ---------------------------------------------------------------------------

// bibleVersionOr404 resolves the {id} path value to a translation that has text
// loaded, writing the response and returning false when it cannot.
//
// A registered-but-empty translation answers 404 rather than an empty chapter
// list: the client's next request would be for a chapter, and "the translation
// is installed but has no text" is a state the reader must not be sent into.
func (h *Handler) bibleVersionOr404(w http.ResponseWriter, r *http.Request) (models.BibleVersion, bool) {
	id := strings.ToLower(strings.TrimSpace(r.PathValue("id")))
	v, err := h.bible.Version(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "BIBLE_VERSION_UNKNOWN", "no such translation")
		return models.BibleVersion{}, false
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load the translation")
		return models.BibleVersion{}, false
	}
	if !v.Imported {
		writeCode(w, http.StatusNotFound, "BIBLE_VERSION_UNAVAILABLE",
			"this translation is registered but its text has not been imported")
		return models.BibleVersion{}, false
	}
	return v, true
}

// bibleBookOr400 resolves a book name in any supported dialect to its canonical
// OSIS id.
func (h *Handler) bibleBookOr400(w http.ResponseWriter, raw string) (string, bool) {
	id, ok := bible.ParseBook(raw)
	if !ok {
		writeCode(w, http.StatusBadRequest, "BIBLE_BOOK_UNKNOWN", "unrecognised book: "+strings.TrimSpace(raw))
		return "", false
	}
	return id, true
}

// bibleCoordinates reads the {version}/{book}/{chapter}/{verse} path values a
// highlight or bookmark route carries.
func (h *Handler) bibleCoordinates(w http.ResponseWriter, r *http.Request) (string, string, int, int, bool) {
	version := strings.ToLower(strings.TrimSpace(r.PathValue("version")))
	if _, ok := bible.VersionByID(version); !ok {
		writeCode(w, http.StatusNotFound, "BIBLE_VERSION_UNKNOWN", "no such translation")
		return "", "", 0, 0, false
	}
	book, ok := h.bibleBookOr400(w, r.PathValue("book"))
	if !ok {
		return "", "", 0, 0, false
	}
	chapter, err1 := strconv.Atoi(r.PathValue("chapter"))
	verse, err2 := strconv.Atoi(r.PathValue("verse"))
	if err1 != nil || err2 != nil || chapter < 1 || verse < 1 {
		writeCode(w, http.StatusBadRequest, "BIBLE_REFERENCE_INVALID", "chapter and verse must be positive numbers")
		return "", "", 0, 0, false
	}
	// The canon is the outer bound: a coordinate outside it cannot exist in any
	// translation, so it is refused before the database is asked.
	if _, err := bible.NewReferenceByID(book, chapter, strconv.Itoa(verse)); err != nil {
		writeCode(w, http.StatusBadRequest, "BIBLE_REFERENCE_INVALID", err.Error())
		return "", "", 0, 0, false
	}
	return version, book, chapter, verse, true
}

func atoiQuery(r *http.Request, name string) int {
	n, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(name)))
	if err != nil || n < 0 {
		return 0
	}
	return n
}
