package api

import (
	"net/http"

	"github.com/Teamthy/i-confess/internal/ai"
	"github.com/Teamthy/i-confess/internal/httpx"
)

// aiParse §43 — AI orchestrates, never invents theology.
// NLU → {categories,duration} → Engine.Build trusted content.
func (h *Handler) aiParse(w http.ResponseWriter, r *http.Request) {
	var req ai.Request
	if err := httpx.DecodeJSON(r, &req); err != nil || (req.Utterance == "" && len(req.Categories) == 0) {
		httpx.WriteError(w, http.StatusBadRequest, "utterance or categories required")
		return
	}
	parser := ai.NewParserFromEnv()
	parsed, err := parser.Parse(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	engReq, err := parsed.ToEngineRequest()
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
