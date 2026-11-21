package api

import (
	"errors"
	"net/http"

	"github.com/Teamthy/i-confess/internal/ai"
	"github.com/Teamthy/i-confess/internal/httpx"
)

// SetAIParser installs the builder's NLU parser. NewHandler defaults to the
// keyword parser.
func (h *Handler) SetAIParser(p ai.Parser) {
	if p != nil {
		h.aiParser = p
	}
}

// aiParse §43 — AI orchestrates, never invents theology.
// NLU → {categories,duration} → Engine.Build trusted content.
//
// The parser chooses among the published categories in the database, and the
// result is translated from slugs to the category ids the engine selects by.
// Explicit categories may be given as slugs or ids; unknown ones are rejected.
func (h *Handler) aiParse(w http.ResponseWriter, r *http.Request) {
	var req ai.Request
	if err := httpx.DecodeJSON(r, &req); err != nil || (req.Utterance == "" && len(req.Categories) == 0) {
		httpx.WriteError(w, http.StatusBadRequest, "utterance or categories required")
		return
	}
	if len(req.Utterance) > 2000 {
		httpx.WriteError(w, http.StatusBadRequest, "utterance must be 2000 characters or fewer")
		return
	}

	cats, err := h.cont.ListCategories(r.Context(), false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not load categories")
		return
	}
	catalog := make([]ai.Category, 0, len(cats))
	slugToID := make(map[string]string, len(cats))
	idToSlug := make(map[string]string, len(cats))
	for _, c := range cats {
		catalog = append(catalog, ai.Category{Slug: c.Slug, Name: c.Name, Description: c.Description})
		slugToID[c.Slug] = c.ID
		idToSlug[c.ID] = c.Slug
	}

	// Normalise explicit categories to slugs so both parsers and the engine
	// mapping see one form.
	for i, c := range req.Categories {
		if _, ok := slugToID[c]; ok {
			continue
		}
		slug, ok := idToSlug[c]
		if !ok {
			httpx.WriteError(w, http.StatusBadRequest, "unknown category: "+c)
			return
		}
		req.Categories[i] = slug
	}

	parsed, err := h.aiParser.Parse(r.Context(), req, catalog)
	if err != nil {
		if errors.Is(err, ai.ErrNoInput) {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.WriteError(w, http.StatusServiceUnavailable, "could not interpret the request")
		return
	}
	engReq, err := parsed.ToEngineRequest(slugToID)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"parsed":         parsed,
		"engine_request": engReq,
		"guardrail":      "AI maps to category/duration only — Engine selects Scripture-rooted confessions",
	})
}
