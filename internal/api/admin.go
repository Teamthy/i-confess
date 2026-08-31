package api

import (
	"net/http"
	"strings"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
)

// ---------- Admin: categories ----------

func (h *Handler) adminCreateCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
		Premium     bool   `json:"premium"`
		Status      string `json:"status"`
		SortOrder   int    `json:"sort_order"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Slug == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name and slug are required")
		return
	}
	if req.Status == "" {
		req.Status = "draft"
	}
	c := &models.Category{
		Name: req.Name, Slug: req.Slug, Description: req.Description, Icon: req.Icon,
		Premium: req.Premium, Status: req.Status, SortOrder: req.SortOrder,
	}
	if err := h.cont.CreateCategory(r.Context(), c); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			httpx.WriteError(w, http.StatusConflict, "a category with this slug already exists")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create category")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (h *Handler) adminListCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.cont.ListCategories(r.Context(), true)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load categories")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cats)
}

// ---------- Admin: confessions ----------

func (h *Handler) adminCreateConfession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CategoryID string `json:"category_id"`
		Title      string `json:"title"`
		ShortText  string `json:"short_text"`
		MediumText string `json:"medium_text"`
		LongText   string `json:"long_text"`
		Description string `json:"description"`
		Tags       []string `json:"tags"`
		Intensity  int    `json:"intensity"`
		Language   string `json:"language"`
		Status     string `json:"status"`
		Author     string `json:"author"`
		Variants   []models.ConfessionVariant `json:"variants"`
		Scriptures []models.ScriptureRef      `json:"scriptures"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CategoryID == "" || req.Title == "" {
		httpx.WriteError(w, http.StatusBadRequest, "category_id and title are required")
		return
	}
	if _, err := h.cont.CategoryByID(r.Context(), req.CategoryID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "category not found")
		return
	}
	if req.Status == "" {
		req.Status = "draft"
	}
	if req.Language == "" {
		req.Language = "en"
	}
	if req.Intensity == 0 {
		req.Intensity = 1
	}
	c := &models.Confession{
		CategoryID: req.CategoryID, Title: req.Title, ShortText: req.ShortText,
		MediumText: req.MediumText, LongText: req.LongText, Description: req.Description,
		Tags: req.Tags, Intensity: req.Intensity, Language: req.Language, Status: req.Status,
		Author: req.Author, Variants: req.Variants, Scriptures: req.Scriptures,
	}
	if err := h.cont.CreateConfession(r.Context(), c); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create confession")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (h *Handler) adminListConfessions(w http.ResponseWriter, r *http.Request) {
	confs, err := h.cont.ListConfessions(r.Context(), false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confessions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, confs)
}

func (h *Handler) adminGetConfession(w http.ResponseWriter, r *http.Request) {
	c, err := h.cont.ConfessionByID(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "confession not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (h *Handler) adminUpdateConfessionStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || !validConfessionStatus(req.Status) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid status")
		return
	}
	if err := h.cont.UpdateConfessionStatus(r.Context(), r.PathValue("id"), req.Status); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update confession")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": req.Status})
}

func validConfessionStatus(s string) bool {
	switch s {
	case "draft", "content_review", "theological_review", "audio_production", "audio_qa", "approved", "published", "archived":
		return true
	}
	return false
}

// ---------- Admin: voices ----------

func (h *Handler) adminCreateVoice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Provider    string `json:"provider"`
		Gender      string `json:"gender"`
		Language    string `json:"language"`
		Premium     bool   `json:"premium"`
		Status      string `json:"status"`
		SampleURL   string `json:"sample_url"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Type == "" {
		req.Type = "professional"
	}
	if req.Language == "" {
		req.Language = "en"
	}
	if req.Status == "" {
		req.Status = "active"
	}
	v := &models.Voice{
		Name: req.Name, Description: req.Description, Type: req.Type, Provider: req.Provider,
		Gender: req.Gender, Language: req.Language, Premium: req.Premium, Status: req.Status, SampleURL: req.SampleURL,
	}
	if err := h.audio.CreateVoice(r.Context(), v); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create voice")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, v)
}

func (h *Handler) adminListVoices(w http.ResponseWriter, r *http.Request) {
	voices, err := h.audio.ListVoices(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load voices")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, voices)
}

// ---------- Admin: audio ----------

func (h *Handler) adminUpsertAudio(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConfessionID    string `json:"confession_id"`
		VariantID       string `json:"variant_id"`
		VoiceID         string `json:"voice_id"`
		URL             string `json:"url"`
		DurationSeconds int    `json:"duration_seconds"`
		SizeBytes       int64  `json:"size_bytes"`
		Status          string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ConfessionID == "" || req.VoiceID == "" || req.URL == "" {
		httpx.WriteError(w, http.StatusBadRequest, "confession_id, voice_id and url are required")
		return
	}
	if _, err := h.audio.VoiceByID(r.Context(), req.VoiceID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "voice not found")
		return
	}
	if _, err := h.cont.ConfessionByID(r.Context(), req.ConfessionID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "confession not found")
		return
	}
	a := &models.AudioAsset{
		ConfessionID: req.ConfessionID, VariantID: req.VariantID, VoiceID: req.VoiceID,
		URL: req.URL, DurationSeconds: req.DurationSeconds, SizeBytes: req.SizeBytes, Status: req.Status,
	}
	if err := h.audio.UpsertAsset(r.Context(), a); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save audio")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, a)
}

// ---------- Admin: users ----------

func (h *Handler) adminSetRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || !validRole(req.Role) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid role")
		return
	}
	if _, err := h.users.ByID(r.Context(), req.UserID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "user not found")
		return
	}
	if err := h.users.SetAdminRole(r.Context(), req.UserID, req.Role); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to set role")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"user_id": req.UserID, "role": req.Role})
}

func validRole(s string) bool {
	switch s {
	case "super_admin", "content_admin", "audio_producer", "theological_reviewer", "support_admin", "analytics_admin":
		return true
	}
	return false
}

func (h *Handler) adminSetSubscription(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Plan   string `json:"plan"`
		Status string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Plan != "free" && req.Plan != "premium" {
		httpx.WriteError(w, http.StatusBadRequest, "plan must be free or premium")
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if err := h.users.SetSubscription(r.Context(), req.UserID, req.Plan, req.Status); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to set subscription")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"user_id": req.UserID, "plan": req.Plan, "status": req.Status})
}

// ---------- Admin: dashboard summary ----------

func (h *Handler) adminStats(w http.ResponseWriter, r *http.Request) {
	confs, _ := h.cont.ListConfessions(r.Context(), false)
	cats, _ := h.cont.ListCategories(r.Context(), true)
	voices, _ := h.audio.ListVoices(r.Context())
	published := 0
	for _, c := range confs {
		if c.Status == "published" {
			published++
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"categories":          len(cats),
		"confessions":         len(confs),
		"published":           published,
		"voices":              len(voices),
		"confessions_by_status": statusCounts(confs),
	})
}

func statusCounts(confs []models.Confession) map[string]int {
	m := map[string]int{}
	for _, c := range confs {
		m[c.Status]++
	}
	return m
}
