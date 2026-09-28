package api

import (
	"database/sql"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/httpx"
)

// bible_topics.go — topics and the random reviewed passage.
//
// The brief asks for a topical system that is data-driven rather than a set of
// hard-coded pages, and for a random endpoint that never invents a reference.
// Both are answered from content this platform has already reviewed: the
// published confession corpus cites Scripture, each confession sits in a
// published category, and every citation was canon-validated when it was
// seeded. So a topic here is not an editorial guess layered on top of the
// Bible — it is the set of passages the reviewed corpus actually stands on for
// that part of life, ranked by how many confessions cite them.
//
// That has three consequences worth stating, because they are the reason this
// is safe to serve:
//
//   - Nothing is fabricated. A citation that no longer parses against the
//     canon is skipped, not guessed at, and a topic with no valid citation is
//     not published as an empty page.
//   - No provider call is made. Topics are references, not text; the client
//     reads the text through the normal rights-gated endpoints, so a topic
//     page cannot become a way around a translation's licence.
//   - The topical system grows with the corpus. Adding a reviewed confession
//     with a citation extends its topic automatically.

type topicPassage struct {
	Reference       string `json:"reference"`
	BookID          string `json:"book_id"`
	BookName        string `json:"book_name"`
	Chapter         int    `json:"chapter"`
	Verses          string `json:"verses,omitempty"`
	CanonicalID     string `json:"canonical_id,omitempty"`
	ConfessionCount int    `json:"confession_count"`
}

type bibleTopic struct {
	ID           string         `json:"id"`
	Slug         string         `json:"slug"`
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	PassageCount int            `json:"passage_count"`
	Passages     []topicPassage `json:"passages,omitempty"`
}

// topicRow is one reviewed citation joined to the category it belongs to.
type topicRow struct {
	categoryID  string
	slug        string
	name        string
	description string
	sortOrder   int
	book        string
	chapter     int
	verse       string
}

// loadTopicCitations reads every canonical citation the published corpus makes,
// grouped by published category. It is the single query both handlers use.
func (h *Handler) loadTopicCitations(r *http.Request, slug string) ([]topicRow, error) {
	query := `SELECT c.id,c.slug,c.name,COALESCE(c.description,''),c.sort_order,
	                 s.book,COALESCE(s.chapter,0),COALESCE(s.verse,'')
	          FROM scripture_references s
	          JOIN confessions f ON f.id = s.confession_id
	          JOIN categories c ON c.id = f.category_id
	          WHERE f.status='published' AND c.status='published'`
	args := []any{}
	if slug != "" {
		query += ` AND c.slug = ?`
		args = append(args, slug)
	}
	query += ` ORDER BY c.sort_order, c.name, s.sort_order`

	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []topicRow{}
	for rows.Next() {
		var row topicRow
		if err := rows.Scan(&row.categoryID, &row.slug, &row.name, &row.description, &row.sortOrder,
			&row.book, &row.chapter, &row.verse); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// normalizeCitation resolves a stored citation against the canon. A citation
// that does not resolve is dropped: a topic page that links to a verse which
// is not in any Bible is worse than a shorter topic page.
func normalizeCitation(row topicRow) (topicPassage, bool) {
	if row.chapter < 1 {
		return topicPassage{}, false
	}
	ref, err := bible.NewReference(row.book, row.chapter, firstNonEmpty(row.verse, "1"))
	if err != nil {
		return topicPassage{}, false
	}
	passage := topicPassage{
		Reference: ref.Display,
		BookID:    ref.Book,
		BookName:  ref.Name,
		Chapter:   ref.Chapter,
	}
	if len(ref.Verses) > 0 {
		passage.Verses = strings.Join(itoaAll(ref.Verses), ",")
		passage.CanonicalID = bible.CanonicalVerseID(ref.Book, ref.Chapter, ref.Verses[0])
	}
	return passage, true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func itoaAll(numbers []int) []string {
	out := make([]string, 0, len(numbers))
	for _, n := range numbers {
		out = append(out, strconv.Itoa(n))
	}
	return out
}

// groupTopics turns citation rows into topics with de-duplicated, ranked
// passages. Ordering is deterministic so the same corpus always produces the
// same page.
func groupTopics(rows []topicRow) []bibleTopic {
	type acc struct {
		topic    bibleTopic
		order    int
		passages map[string]*topicPassage
		sequence []string
	}
	byTopic := map[string]*acc{}
	order := []string{}

	for _, row := range rows {
		passage, ok := normalizeCitation(row)
		if !ok {
			continue
		}
		entry := byTopic[row.slug]
		if entry == nil {
			entry = &acc{
				topic:    bibleTopic{ID: row.categoryID, Slug: row.slug, Name: row.name, Description: row.description},
				order:    row.sortOrder,
				passages: map[string]*topicPassage{},
			}
			byTopic[row.slug] = entry
			order = append(order, row.slug)
		}
		key := passage.Reference
		if existing, seen := entry.passages[key]; seen {
			existing.ConfessionCount++
			continue
		}
		passage.ConfessionCount = 1
		entry.passages[key] = &passage
		entry.sequence = append(entry.sequence, key)
	}

	topics := make([]bibleTopic, 0, len(order))
	for _, slug := range order {
		entry := byTopic[slug]
		passages := make([]topicPassage, 0, len(entry.sequence))
		for _, key := range entry.sequence {
			passages = append(passages, *entry.passages[key])
		}
		// Most-cited first, then canonical order for a stable page.
		sort.SliceStable(passages, func(i, j int) bool {
			if passages[i].ConfessionCount != passages[j].ConfessionCount {
				return passages[i].ConfessionCount > passages[j].ConfessionCount
			}
			return passages[i].Reference < passages[j].Reference
		})
		entry.topic.Passages = passages
		entry.topic.PassageCount = len(passages)
		topics = append(topics, entry.topic)
	}
	sort.SliceStable(topics, func(i, j int) bool { return byTopic[topics[i].Slug].order < byTopic[topics[j].Slug].order })
	return topics
}

// bibleTopics lists every topic that has at least one canonical passage.
func (h *Handler) bibleTopics(w http.ResponseWriter, r *http.Request) {
	rows, err := h.loadTopicCitations(r, "")
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load Bible topics.")
		return
	}
	topics := groupTopics(rows)
	// The list is a directory: passages belong to the detail page.
	summaries := make([]bibleTopic, 0, len(topics))
	for _, topic := range topics {
		if topic.PassageCount == 0 {
			continue
		}
		topic.Passages = nil
		summaries = append(summaries, topic)
	}
	w.Header().Set("Cache-Control", "public, max-age=120, stale-while-revalidate=600")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"topics": summaries})
}

// bibleTopic returns one topic and the passages the reviewed corpus cites for
// it. References only: the text is read through the rights-gated endpoints.
func (h *Handler) bibleTopic(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.PathValue("slug"))
	if slug == "" {
		httpx.WriteError(w, http.StatusBadRequest, "Choose a topic.")
		return
	}
	rows, err := h.loadTopicCitations(r, slug)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load the topic.")
		return
	}
	topics := groupTopics(rows)
	if len(topics) == 0 || topics[0].PassageCount == 0 {
		httpx.WriteJSON(w, http.StatusNotFound, map[string]string{
			"code":  "BIBLE_NOT_FOUND",
			"error": "That topic has no reviewed passages yet.",
		})
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=120, stale-while-revalidate=600")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"topic": topics[0]})
}

// bibleRandom returns one passage drawn from the reviewed corpus's citations,
// read in the requested translation.
//
// "Random" is the one place a Bible app is most tempted to reach outside what
// it has reviewed. It does not here: the pool is the same canon-validated
// citation set the topics are built from, the translation is rights-checked
// exactly as any other read, and if the pool is empty the endpoint says so
// rather than reaching for an arbitrary verse.
func (h *Handler) bibleRandom(w http.ResponseWriter, r *http.Request) {
	translationID := strings.TrimSpace(r.URL.Query().Get("translation"))
	if translationID == "" {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]string{
			"code":  "BIBLE_TRANSLATION_REQUIRED",
			"error": "Choose a Bible translation.",
		})
		return
	}
	if err := h.readableBibleTranslation(r.Context(), translationID); err != nil {
		h.bibleError(w, err)
		return
	}

	var book, verse, topicSlug, topicName string
	var chapter int
	err := h.db.QueryRowContext(r.Context(), `SELECT s.book,COALESCE(s.chapter,0),COALESCE(s.verse,''),c.slug,c.name
	          FROM scripture_references s
	          JOIN confessions f ON f.id = s.confession_id
	          JOIN categories c ON c.id = f.category_id
	          WHERE f.status='published' AND c.status='published' AND s.chapter IS NOT NULL
	          ORDER BY random() LIMIT 1`).Scan(&book, &chapter, &verse, &topicSlug, &topicName)
	if err == sql.ErrNoRows {
		httpx.WriteJSON(w, http.StatusNotFound, map[string]string{
			"code":  "BIBLE_NOT_FOUND",
			"error": "No reviewed passage is available yet.",
		})
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to select a passage.")
		return
	}

	passage, ok := normalizeCitation(topicRow{book: book, chapter: chapter, verse: verse})
	if !ok {
		httpx.WriteJSON(w, http.StatusNotFound, map[string]string{
			"code":  "BIBLE_NOT_FOUND",
			"error": "No reviewed passage is available yet.",
		})
		return
	}

	read, err := h.bible.GetPassage(r.Context(), translationID, passage.Reference)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	// A random passage must not be cached: the next reader asks for a new one.
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"reference": passage.Reference,
		"passage":   read,
		"topic":     map[string]string{"slug": topicSlug, "name": topicName},
	})
}
