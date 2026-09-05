package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Teamthy/i-confess/internal/engine"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/store"
)

// Templates — Custom sessions + shareable URLs (§5.4)

func (h *Handler) createTemplate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		CategoryIDs []string          `json:"category_ids"`
		Weights     map[string]float64 `json:"weights"`
		VoiceID     string            `json:"voice_id"`
		VoiceRules  map[string]any    `json:"voice_rules"`
		Ordering    map[string]any    `json:"ordering"`
		IsPublic    bool              `json:"is_public"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || len(req.CategoryIDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "name and at least one category are required")
		return
	}

	// Serialize maps to JSON strings for store (keep simple)
	weightsJSON := ""
	if req.Weights != nil {
		if b, err := json.Marshal(req.Weights); err == nil {
			weightsJSON = string(b)
		}
	}
	voiceRulesJSON := ""
	if req.VoiceRules != nil {
		if b, err := json.Marshal(req.VoiceRules); err == nil {
			voiceRulesJSON = string(b)
		}
	}
	orderingJSON := ""
	if req.Ordering != nil {
		if b, err := json.Marshal(req.Ordering); err == nil {
			orderingJSON = string(b)
		}
	}

	t := &store.Template{
		UserID:      h.userID(r),
		Name:        req.Name,
		Description: req.Description,
		CategoryIDs: req.CategoryIDs,
		Weights:     weightsJSON,
		VoiceID:     req.VoiceID,
		VoiceRules:  voiceRulesJSON,
		Ordering:    orderingJSON,
		IsPublic:    req.IsPublic,
	}
	if err := h.templates.Create(r.Context(), t); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create template")
		return
	}
	// Shareable URL
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"template":   t,
		"share_url":  "https://iconfess.app/t/" + t.ShareToken,
		"deeplink":   "iconfess://t/" + t.ShareToken,
	})
}

func (h *Handler) listTemplates(w http.ResponseWriter, r *http.Request) {
	list, err := h.templates.ListByUser(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load templates")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) getTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := h.templates.ByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "template not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load template")
		return
	}
	if t.UserID != h.userID(r) && !t.IsPublic {
		httpx.WriteError(w, http.StatusForbidden, "not your template")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

func (h *Handler) getTemplateByShareToken(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	t, err := h.templates.ByShareToken(r.Context(), token)
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "template not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load template")
		return
	}
	if !t.IsPublic {
		// Private templates are not shareable via token
		httpx.WriteError(w, http.StatusForbidden, "template is private")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

func (h *Handler) updateTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := h.templates.ByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "template not found")
		return
	}
	if t.UserID != h.userID(r) {
		httpx.WriteError(w, http.StatusForbidden, "not your template")
		return
	}
	var req struct {
		Name        *string           `json:"name"`
		Description *string           `json:"description"`
		CategoryIDs []string          `json:"category_ids"`
		Weights     map[string]float64 `json:"weights"`
		VoiceID     *string           `json:"voice_id"`
		IsPublic    *bool             `json:"is_public"`
		Ordering    map[string]any    `json:"ordering"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name != nil {
		t.Name = *req.Name
	}
	if req.Description != nil {
		t.Description = *req.Description
	}
	if req.CategoryIDs != nil {
		t.CategoryIDs = req.CategoryIDs
	}
	if req.Weights != nil {
		if b, err := json.Marshal(req.Weights); err == nil {
			t.Weights = string(b)
		}
	}
	if req.VoiceID != nil {
		t.VoiceID = *req.VoiceID
	}
	if req.IsPublic != nil {
		t.IsPublic = *req.IsPublic
	}
	if req.Ordering != nil {
		if b, err := json.Marshal(req.Ordering); err == nil {
			t.Ordering = string(b)
		}
	}
	if err := h.templates.Update(r.Context(), t); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update template")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

func (h *Handler) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.templates.Delete(r.Context(), id, h.userID(r)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "template not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to delete template")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// startTemplate creates a session from a template (POST /v1/templates/{id}/start)
func (h *Handler) startTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := h.templates.ByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "template not found")
		return
	}
	if t.UserID != h.userID(r) && !t.IsPublic {
		httpx.WriteError(w, http.StatusForbidden, "not your template")
		return
	}
	// Build from template — decode weights/ordering if present
	var weights map[string]float64
	if t.Weights != "" {
		_ = json.Unmarshal([]byte(t.Weights), &weights)
	}
	strategy := "BALANCED"
	if t.Ordering != "" {
		var ord map[string]string
		if err := json.Unmarshal([]byte(t.Ordering), &ord); err == nil {
			if s, ok := ord["strategy"]; ok && s != "" {
				strategy = s
			}
		}
	}
	// Default duration 30m if not in ordering; could be stored as duration param in template later
	duration := 1800
	// Reuse createSession logic: just call engine.Build via trigger
	// For now, delegate to a helper that builds and persists
	h.createSessionFromTemplate(w, r, t, weights, strategy, duration)
}

// createSessionFromTemplate helper — builds a session from template weights/voice
func (h *Handler) createSessionFromTemplate(w http.ResponseWriter, r *http.Request, t *store.Template, weights map[string]float64, strategy string, duration int) {
	ent := h.entitlementsFor(r.Context(), h.userID(r))
	if max := ent.MaxSessionSeconds(); duration > max {
		httpx.WriteJSON(w, http.StatusPaymentRequired, map[string]any{
			"error":       "duration exceeds plan limit",
			"max_seconds": max,
		})
		return
	}
	favs, _ := h.eng.ListFavorites(r.Context(), h.userID(r), "confession")
	favMap := map[string]bool{}
	for _, f := range favs {
		favMap[f.EntityID] = true
	}
	history, _ := h.eng.History(r.Context(), h.userID(r), 20)
	recentMap := map[string]bool{}
	for i, rec := range history {
		if i >= 10 {
			break
		}
		if rec.ConfessionID != "" {
			recentMap[rec.ConfessionID] = true
		}
	}
	sess, err := h.engn.Build(r.Context(), engine.Request{
		UserID:          h.userID(r),
		CategoryIDs:     t.CategoryIDs,
		CategoryWeights: weights,
		DurationSeconds: duration,
		Strategy:        engine.Strategy(strategy),
		VoiceID:         t.VoiceID,
		FavoriteIDs:     favMap,
		RecentIDs:       recentMap,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusUnprocessableEntity, "no content for this template")
		return
	}
	sess.Title = t.Name
	if err := h.sess.Create(r.Context(), sess); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save session")
		return
	}
	h.signSessionAudio(r.Context(), sess, ent)
	httpx.WriteJSON(w, http.StatusCreated, sess)
}
