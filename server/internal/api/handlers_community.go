package api

import (
	"net/http"
	"strconv"

	"github.com/Teamthy/i-confess/internal/community"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/store"
)

func (h *Handler) createCommunityPost(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Body       string `json:"body"`
		Visibility string `json:"visibility"`
	}
	// support both keys
	var raw map[string]any
	if err := httpx.DecodeJSON(r, &raw); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body required")
		return
	}
	if v, ok := raw["body"].(string); ok {
		req.Body = v
	}
	if v, ok := raw["visibility"].(string); ok {
		req.Visibility = v
	}
	if req.Body == "" || len(req.Body) > 2000 {
		httpx.WriteError(w, http.StatusBadRequest, "body 1..2000 chars")
		return
	}
	if req.Visibility == "" {
		req.Visibility = community.VisibilityPrivate
	}
	if !community.IsValidVisibility(req.Visibility) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid visibility")
		return
	}

	// For shared or public posts, gate on email verification to prevent abuse
	if req.Visibility != community.VisibilityPrivate {
		if u, err := h.users.ByID(r.Context(), userID); err == nil && !u.EmailVerified {
			httpx.WriteError(w, http.StatusForbidden, "email verification is required before sharing community posts")
			return
		}
	}

	store := community.NewStore(h.db)
	p, err := store.Create(r.Context(), userID, req.Body, req.Visibility)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create post")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, p)
}

func (h *Handler) feedCommunity(w http.ResponseWriter, r *http.Request) {
	store := community.NewStore(h.db)
	posts, err := store.Feed(r.Context(), 20)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load feed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"posts": posts})
}

func (h *Handler) reactCommunity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Reaction string `json:"reaction"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Reaction == "" {
		httpx.WriteError(w, http.StatusBadRequest, "reaction required")
		return
	}
	if req.Reaction != "amen" && req.Reaction != "heart" && req.Reaction != "pray" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid reaction")
		return
	}
	cStore := community.NewStore(h.db)
	if err := cStore.React(r.Context(), id, h.userID(r), community.Reaction(req.Reaction)); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to react")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// feedUserConfessions is the public UGC reader that closes G-40. It returns
// only confessions that are both public in intent (visibility=public) and
// published by a moderator (status=published), ordered newest first. Author
// identity is never returned — the projection omits user_id, reviewed_by and
// any other account-linked field. Shared/private or non-published rows are
// excluded by the store query, not by post-filtering, so a regression that
// widens the query is caught by the database-backed tests.
func (h *Handler) feedUserConfessions(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	eng := store.NewEngagementStore(h.db)
	list, err := eng.ListPublishedUserConfessions(r.Context(), limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confessions")
		return
	}
	// Project to anonymous public shape.
	type publicUC struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Text        string `json:"text"`
		CategoryID  string `json:"category_id,omitempty"`
		Visibility  string `json:"visibility"`
		Status      string `json:"status"`
		PublishedAt string `json:"published_at,omitempty"`
		CreatedAt   string `json:"created_at"`
	}
	out := make([]publicUC, 0, len(list))
	for _, uc := range list {
		out = append(out, publicUC{
			ID:          uc.ID,
			Title:       uc.Title,
			Text:        uc.Text,
			CategoryID:  uc.CategoryID,
			Visibility:  uc.Visibility,
			Status:      uc.Status,
			PublishedAt: uc.PublishedAt,
			CreatedAt:   uc.CreatedAt,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"confessions": out})
}
