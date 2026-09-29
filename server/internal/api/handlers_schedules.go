package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/Teamthy/i-confess/internal/engine"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

func (h *Handler) listSchedules(w http.ResponseWriter, r *http.Request) {
	list, err := h.sched.ListByUser(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load schedules")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) createSchedule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label           string   `json:"label"`
		Time            string   `json:"time"`
		DaysOfWeek      []int    `json:"days_of_week"`
		Timezone        string   `json:"timezone"`
		DurationSeconds int      `json:"duration_seconds"`
		VoiceID         string   `json:"voice_id"`
		CategoryIDs     []string `json:"category_ids"`
		Enabled         *bool    `json:"enabled"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Label == "" || req.Time == "" {
		httpx.WriteError(w, http.StatusBadRequest, "label and time are required")
		return
	}
	if _, err := time.Parse("15:04", req.Time); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "time must be HH:MM")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.DurationSeconds == 0 {
		req.DurationSeconds = 1800
	}
	// Checked at save time so the user hears "your plan allows 15 minutes"
	// now rather than discovering it when the schedule silently fails to fire.
	// The authoritative check is still the one in the build path, because the
	// plan can change between saving and firing.
	if req.DurationSeconds < engine.MinSessionSeconds || req.DurationSeconds > engine.MaxSessionSeconds {
		httpx.WriteError(w, http.StatusBadRequest, "duration must be between 1 minute and 3 hours")
		return
	}
	if ent := h.entitlementsFor(r.Context(), h.userID(r)); req.DurationSeconds > ent.MaxSessionSeconds() {
		writePlanLimit(w, ent)
		return
	}
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	sc := &models.Schedule{
		UserID:          h.userID(r),
		Label:           req.Label,
		Time:            req.Time,
		DaysOfWeek:      req.DaysOfWeek,
		Timezone:        req.Timezone,
		DurationSeconds: req.DurationSeconds,
		VoiceID:         req.VoiceID,
		CategoryIDs:     req.CategoryIDs,
		Enabled:         enabled,
	}
	if err := h.sched.Create(r.Context(), sc); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create schedule")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, sc)
}

func (h *Handler) updateSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sc, err := h.sched.ByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "schedule not found")
		return
	}
	if sc.UserID != h.userID(r) {
		httpx.WriteError(w, http.StatusForbidden, "not your schedule")
		return
	}
	var req struct {
		Label           *string  `json:"label"`
		Time            *string  `json:"time"`
		DaysOfWeek      []int    `json:"days_of_week"`
		Timezone        *string  `json:"timezone"`
		DurationSeconds *int     `json:"duration_seconds"`
		VoiceID         *string  `json:"voice_id"`
		CategoryIDs     []string `json:"category_ids"`
		Enabled         *bool    `json:"enabled"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Label != nil {
		sc.Label = *req.Label
	}
	if req.Time != nil {
		if _, err := time.Parse("15:04", *req.Time); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "time must be HH:MM")
			return
		}
		sc.Time = *req.Time
	}
	if req.DaysOfWeek != nil {
		sc.DaysOfWeek = req.DaysOfWeek
	}
	if req.Timezone != nil {
		sc.Timezone = *req.Timezone
	}
	if req.DurationSeconds != nil {
		sc.DurationSeconds = *req.DurationSeconds
	}
	if req.VoiceID != nil {
		sc.VoiceID = *req.VoiceID
	}
	if req.CategoryIDs != nil {
		sc.CategoryIDs = req.CategoryIDs
	}
	if req.Enabled != nil {
		sc.Enabled = *req.Enabled
	}
	if err := h.sched.Update(r.Context(), sc); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update schedule")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sc)
}

func (h *Handler) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	err := h.sched.Delete(r.Context(), r.PathValue("id"), h.userID(r))
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "schedule not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to delete schedule")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- Engagement ----------
