package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
)

// libraryEntry is one published confession in the library index. The slug is
// the public, URL-safe name the website has always used for a confession; it
// is derived here so the server, not a bundled copy of the library, is the
// source of what exists and what it is called.
type libraryEntry struct {
	ID           string             `json:"id"`
	Slug         string             `json:"slug"`
	Title        string             `json:"title"`
	CategoryID   string             `json:"category_id"`
	CategorySlug string             `json:"category_slug"`
	Category     string             `json:"category"`
	ShortText    string             `json:"short_text"`
	MediumText   string             `json:"medium_text"`
	LongText     string             `json:"long_text"`
	Intensity    int                `json:"intensity"`
	Scriptures   []libraryScripture `json:"scriptures"`
}

type libraryScripture struct {
	Book        string `json:"book"`
	Chapter     int    `json:"chapter"`
	Verse       string `json:"verse"`
	Translation string `json:"translation"`
	Direct      bool   `json:"direct"`
}

var slugJunk = regexp.MustCompile(`[^a-z0-9]+`)

// confessionSlug matches web/lib/data.ts slugify: lower-case, runs of anything
// else become one hyphen, no hyphen at either end.
func confessionSlug(title string) string {
	return strings.Trim(slugJunk.ReplaceAllString(strings.ToLower(title), "-"), "-")
}

// libraryIndex lists every published confession in a published category.
func (h *Handler) libraryIndex(w http.ResponseWriter, r *http.Request) {
	cats, err := h.cont.ListCategories(r.Context(), false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load categories")
		return
	}
	type cat struct{ slug, name string }
	byID := make(map[string]cat, len(cats))
	for _, c := range cats {
		byID[c.ID] = cat{c.Slug, c.Name}
	}
	confs, err := h.cont.ListConfessions(r.Context(), true)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confessions")
		return
	}
	refs, err := h.cont.AllPublishedScriptures(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load scripture references")
		return
	}
	out := make([]libraryEntry, 0, len(confs))
	for _, c := range confs {
		k, ok := byID[c.CategoryID]
		if !ok { // the category is unpublished: its confessions are not public
			continue
		}
		out = append(out, libraryEntry{
			ID: c.ID, Slug: confessionSlug(c.Title), Title: c.Title,
			CategoryID: c.CategoryID, CategorySlug: k.slug, Category: k.name,
			ShortText: c.ShortText, MediumText: c.MediumText, LongText: c.LongText, Intensity: c.Intensity,
			Scriptures: scripturesOf(refs[c.ID]),
		})
	}
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"confessions": out})
}

func scripturesOf(refs []models.ScriptureRef) []libraryScripture {
	out := make([]libraryScripture, 0, len(refs))
	for _, r := range refs {
		out = append(out, libraryScripture{Book: r.Book, Chapter: r.Chapter, Verse: r.Verse, Translation: r.Translation, Direct: r.IsDirectQuote})
	}
	return out
}
