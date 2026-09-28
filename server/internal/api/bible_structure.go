package api

import (
	"net/http"
	"strings"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/httpx"
)

// bible_structure.go — the canonical structure endpoint.
//
// Every other Bible read is translation-scoped: books, chapters and verses all
// answer "what does this edition carry?". A reader needs the question before
// that one — "what is the Bible shaped like?" — to draw a book list, a chapter
// grid and a verse picker, to validate a typed reference before spending a
// request on it, and to keep working when the catalogue is empty or a
// translation is still pending review.
//
// The answer is the canon, which does not change, so it is served from the
// in-process table with a long cache life and no database or provider call at
// all. With ?translation= the same structure is annotated with what that
// edition actually carries, which is how a client greys out the books a New
// Testament edition does not have instead of rendering an empty Genesis.

type bibleStructureAvailability struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Abbreviation     string   `json:"abbreviation"`
	Coverage         string   `json:"coverage,omitempty"`
	Direction        string   `json:"direction,omitempty"`
	LanguageCode     string   `json:"language_code,omitempty"`
	AvailableBookIDs []string `json:"available_book_ids"`
	MissingBookIDs   []string `json:"missing_book_ids"`
	ExtraBookIDs     []string `json:"extra_book_ids,omitempty"`
}

type bibleStructureResponse struct {
	bible.Structure
	Translation *bibleStructureAvailability `json:"translation,omitempty"`
}

func (h *Handler) bibleStructure(w http.ResponseWriter, r *http.Request) {
	structure := bible.CanonStructure()
	response := bibleStructureResponse{Structure: structure}

	translationID := strings.TrimSpace(r.URL.Query().Get("translation"))
	if translationID == "" {
		// The canon is immutable, so this response is safe to cache hard. A
		// client that holds it can navigate the whole Bible offline.
		w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
		httpx.WriteJSON(w, http.StatusOK, response)
		return
	}

	if err := h.readableBibleTranslation(r.Context(), translationID); err != nil {
		h.bibleError(w, err)
		return
	}
	translation, err := h.bible.GetTranslation(r.Context(), translationID)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	books, err := h.bible.GetBooks(r.Context(), translationID)
	if err != nil {
		h.bibleError(w, err)
		return
	}

	carried := make(map[string]bool, len(books))
	for _, book := range books {
		carried[book.ID] = true
	}
	availability := &bibleStructureAvailability{
		ID:               translation.ID,
		Name:             translation.Name,
		Abbreviation:     translation.Abbreviation,
		Coverage:         translation.Coverage,
		Direction:        translation.Direction,
		LanguageCode:     translation.Language,
		AvailableBookIDs: []string{},
		MissingBookIDs:   []string{},
	}
	canonical := make(map[string]bool, len(structure.Books))
	for _, book := range structure.Books {
		canonical[book.ID] = true
		if carried[book.ID] {
			availability.AvailableBookIDs = append(availability.AvailableBookIDs, book.ID)
			continue
		}
		availability.MissingBookIDs = append(availability.MissingBookIDs, book.ID)
	}
	// A book the edition carries that the Protestant canon does not (a
	// deuterocanonical book, say) is reported rather than hidden: the client
	// decides how to present it, and silently dropping it would misdescribe
	// the edition.
	for _, book := range books {
		if !canonical[book.ID] {
			availability.ExtraBookIDs = append(availability.ExtraBookIDs, book.ID)
		}
	}

	response.Translation = availability
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
	httpx.WriteJSON(w, http.StatusOK, response)
}
