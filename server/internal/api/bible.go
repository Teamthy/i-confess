package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/httpx"
)

// SetBibleProvider installs the configured provider. Provider selection and all
// upstream response normalization remain server-side.
func (h *Handler) SetBibleProvider(provider bible.BibleProvider) {
	if provider != nil {
		h.bible = provider
	}
}

// SetBibleDiscoveryProvider installs a server-only provider catalogue source.
// It is used by the admin review workflow and is never exposed as application content.
func (h *Handler) SetBibleDiscoveryProvider(provider bible.BibleProvider) {
	if provider != nil {
		h.bibleDiscovery = provider
	}
}

func (h *Handler) readableBibleTranslation(ctx context.Context, id string) error {
	t, err := h.bible.GetTranslation(ctx, id)
	if err != nil {
		return err
	}
	if !strings.EqualFold(t.Status, "active") || !t.APIExposureAllowed {
		return bible.ErrNotFound
	}
	return nil
}

func (h *Handler) bibleUnavailable(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "30")
	httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
		"code":  "BIBLE_TEMPORARILY_UNAVAILABLE",
		"error": "Bible content is temporarily unavailable. Please try again.",
	})
}

func (h *Handler) bibleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, bible.ErrNotFound), errors.Is(err, bible.ErrRestricted):
		httpx.WriteJSON(w, http.StatusNotFound, map[string]string{
			"code": "BIBLE_NOT_FOUND", "error": "That Bible passage or translation is not available.",
		})
	default:
		h.bibleUnavailable(w)
	}
}

func (h *Handler) bibleLanguages(w http.ResponseWriter, r *http.Request) {
	items, err := h.bible.GetLanguages(r.Context())
	if err != nil {
		h.bibleError(w, err)
		return
	}
	translations, err := h.bible.GetTranslations(r.Context(), "", "")
	if err != nil {
		h.bibleError(w, err)
		return
	}
	available := map[string]bool{}
	for _, t := range translations {
		if strings.EqualFold(t.Status, "active") && t.APIExposureAllowed {
			available[t.Language] = true
		}
	}
	filtered := make([]bible.Language, 0, len(items))
	for _, language := range items {
		if available[language.ID] {
			filtered = append(filtered, language)
		}
	}
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=60")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"languages": filtered})
}

func (h *Handler) bibleTranslations(w http.ResponseWriter, r *http.Request) {
	language := strings.TrimSpace(r.URL.Query().Get("language"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 100 {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"code": "BIBLE_INVALID_QUERY", "error": "Search text is too long."})
		return
	}
	items, err := h.bible.GetTranslations(r.Context(), language, query)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	approved := make([]bible.Translation, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(item.Status, "active") && item.APIExposureAllowed {
			approved = append(approved, item)
		}
	}
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=60")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"translations": approved})
}

func (h *Handler) bibleTranslation(w http.ResponseWriter, r *http.Request) {
	t, err := h.bible.GetTranslation(r.Context(), r.PathValue("id"))
	if err != nil {
		h.bibleError(w, err)
		return
	}
	if !strings.EqualFold(t.Status, "active") || !t.APIExposureAllowed {
		h.bibleError(w, bible.ErrNotFound)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

func (h *Handler) bibleBooks(w http.ResponseWriter, r *http.Request) {
	translation := strings.TrimSpace(r.URL.Query().Get("translation"))
	if translation == "" {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"code": "BIBLE_TRANSLATION_REQUIRED", "error": "Choose a Bible translation."})
		return
	}
	if err := h.readableBibleTranslation(r.Context(), translation); err != nil {
		h.bibleError(w, err)
		return
	}
	books, err := h.bible.GetBooks(r.Context(), translation)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"books": books})
}

func (h *Handler) bibleBook(w http.ResponseWriter, r *http.Request) {
	translation := strings.TrimSpace(r.URL.Query().Get("translation"))
	if translation == "" {
		translation = strings.TrimSpace(r.PathValue("translation"))
	}
	if err := h.readableBibleTranslation(r.Context(), translation); err != nil {
		h.bibleError(w, err)
		return
	}
	book, err := h.bible.GetBook(r.Context(), translation, r.PathValue("id"))
	if err != nil {
		h.bibleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, book)
}

func (h *Handler) bibleBookChapters(w http.ResponseWriter, r *http.Request) {
	translation := strings.TrimSpace(r.URL.Query().Get("translation"))
	if translation == "" {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"code": "BIBLE_TRANSLATION_REQUIRED", "error": "Choose a Bible translation."})
		return
	}
	if err := h.readableBibleTranslation(r.Context(), translation); err != nil {
		h.bibleError(w, err)
		return
	}
	book, err := h.bible.GetBook(r.Context(), translation, r.PathValue("id"))
	if err != nil {
		h.bibleError(w, err)
		return
	}
	chapters := make([]int, book.ChapterCount)
	for i := range chapters {
		chapters[i] = i + 1
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"book": book, "chapters": chapters})
}

func (h *Handler) bibleChapter(w http.ResponseWriter, r *http.Request) {
	chapterNumber, err := strconv.Atoi(r.PathValue("chapter"))
	if err != nil || chapterNumber < 1 || chapterNumber > 200 {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"code": "BIBLE_INVALID_REFERENCE", "error": "Enter a valid Bible chapter."})
		return
	}
	translationID := r.PathValue("translation")
	if err := h.readableBibleTranslation(r.Context(), translationID); err != nil {
		h.bibleError(w, err)
		return
	}
	chapter, err := h.bible.GetChapter(r.Context(), translationID, r.PathValue("book"), chapterNumber)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	etag := `"` + chapter.Translation.ContentHash + ":" + chapter.Book.ID + ":" + strconv.Itoa(chapterNumber) + `"`
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=120")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, chapter)
}

func (h *Handler) bibleVerse(w http.ResponseWriter, r *http.Request) {
	chapter, chapterErr := strconv.Atoi(r.PathValue("chapter"))
	verse, verseErr := strconv.Atoi(r.PathValue("verse"))
	if chapterErr != nil || verseErr != nil || chapter < 1 || verse < 1 || chapter > 200 || verse > 300 {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"code": "BIBLE_INVALID_REFERENCE", "error": "Enter a valid Bible verse."})
		return
	}
	translationID := r.PathValue("translation")
	if err := h.readableBibleTranslation(r.Context(), translationID); err != nil {
		h.bibleError(w, err)
		return
	}
	value, err := h.bible.GetVerse(r.Context(), translationID, r.PathValue("book"), chapter, verse)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, value)
}

func (h *Handler) biblePassage(w http.ResponseWriter, r *http.Request) {
	reference := strings.TrimSpace(r.URL.Query().Get("reference"))
	translation := strings.TrimSpace(r.URL.Query().Get("translation"))
	if len(reference) > 120 || translation == "" || reference == "" {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"code": "BIBLE_INVALID_REFERENCE", "error": "Enter a passage and choose a translation."})
		return
	}
	if _, err := bible.ParseReference(reference); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"code": "BIBLE_INVALID_REFERENCE", "error": "We couldn't read that Bible reference. Check the book, chapter and verse."})
		return
	}
	if err := h.readableBibleTranslation(r.Context(), translation); err != nil {
		h.bibleError(w, err)
		return
	}
	value, err := h.bible.GetPassage(r.Context(), translation, reference)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, value)
}

func (h *Handler) bibleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	translation := strings.TrimSpace(r.URL.Query().Get("translation"))
	book := strings.TrimSpace(r.URL.Query().Get("book"))
	if query == "" || len(query) > 200 {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"code": "BIBLE_INVALID_QUERY", "error": "Enter a search phrase up to 200 characters."})
		return
	}
	if reference, err := bible.ParseReference(query); err == nil {
		_ = reference
		if translation == "" {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"code": "BIBLE_TRANSLATION_REQUIRED", "error": "Choose a Bible translation to open this reference."})
			return
		}
		if err := h.readableBibleTranslation(r.Context(), translation); err != nil {
			h.bibleError(w, err)
			return
		}
		passage, err := h.bible.GetPassage(r.Context(), translation, query)
		if err != nil {
			h.bibleError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"kind": "reference", "results": []any{passage}})
		return
	}
	if translation != "" {
		if err := h.readableBibleTranslation(r.Context(), translation); err != nil {
			h.bibleError(w, err)
			return
		}
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	results, err := h.bible.Search(r.Context(), query, translation, book, limit)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"kind": "text", "results": results})
}

func (h *Handler) bibleCrossReferences(w http.ResponseWriter, r *http.Request) {
	reference := strings.TrimSpace(r.URL.Query().Get("reference"))
	if _, err := bible.ParseReference(reference); err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{"code": "BIBLE_INVALID_REFERENCE", "error": "Enter a valid Bible reference."})
		return
	}
	references, err := h.bible.GetCrossReferences(r.Context(), reference)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"reference": reference, "references": references})
}
