package api

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Teamthy/i-confess/internal/engine"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/scheduler"
	"github.com/Teamthy/i-confess/internal/store"
)

func setNextScheduleRun(sc *models.Schedule, at time.Time) {
	next := scheduler.NextOccurrence(scheduler.Schedule{
		Time: sc.Time, DaysOfWeek: sc.DaysOfWeek, Timezone: sc.Timezone, Enabled: sc.Enabled,
	}, at)
	sc.NextRunAt = ""
	if !next.IsZero() {
		sc.NextRunAt = next.UTC().Format(time.RFC3339)
	}
}

func (h *Handler) listSchedules(w http.ResponseWriter, r *http.Request) {
	list, err := h.sched.ListByUser(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load schedules")
		return
	}
	at := time.Now()
	for i := range list {
		setNextScheduleRun(&list[i], at)
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

// validateSchedule is shared by create and patch. Validating only POST allowed
// PATCH to save impossible times/zones and bypass the session-length plan cap.
// Omitted days on creation retain the legacy daily default; an explicit empty
// array is rejected rather than silently turning "no days" into every day.
func (h *Handler) validateSchedule(w http.ResponseWriter, r *http.Request, sc *models.Schedule, checkPlan, checkContent bool) bool {
	sc.Label = strings.TrimSpace(sc.Label)
	if sc.Label == "" || utf8.RuneCountInString(sc.Label) > 120 {
		writeCode(w, http.StatusBadRequest, "SCHEDULE_INVALID", "label must be 1–120 characters")
		return false
	}
	if len(sc.Time) != 5 {
		writeCode(w, http.StatusBadRequest, "SCHEDULE_INVALID", "time must be HH:MM")
		return false
	}
	if _, err := time.Parse("15:04", sc.Time); err != nil {
		writeCode(w, http.StatusBadRequest, "SCHEDULE_INVALID", "time must be HH:MM")
		return false
	}
	sc.Timezone = strings.TrimSpace(sc.Timezone)
	if sc.Timezone == "" || sc.Timezone == "Local" {
		writeCode(w, http.StatusBadRequest, "SCHEDULE_INVALID", "timezone must be an IANA name, such as Africa/Lagos")
		return false
	}
	if _, err := time.LoadLocation(sc.Timezone); err != nil {
		writeCode(w, http.StatusBadRequest, "SCHEDULE_INVALID", "unknown timezone — use an IANA name, such as Africa/Lagos")
		return false
	}
	if len(sc.DaysOfWeek) == 0 {
		writeCode(w, http.StatusBadRequest, "SCHEDULE_INVALID", "choose at least one day (1=Monday through 7=Sunday)")
		return false
	}
	seen := map[int]bool{}
	days := make([]int, 0, 7)
	for _, day := range sc.DaysOfWeek {
		if day < 1 || day > 7 {
			writeCode(w, http.StatusBadRequest, "SCHEDULE_INVALID", "days must be between 1 (Monday) and 7 (Sunday)")
			return false
		}
		if !seen[day] {
			days = append(days, day)
			seen[day] = true
		}
	}
	sort.Ints(days)
	sc.DaysOfWeek = days
	if sc.DurationSeconds < engine.MinSessionSeconds || sc.DurationSeconds > engine.MaxSessionSeconds {
		writeCode(w, http.StatusBadRequest, "SCHEDULE_INVALID", "duration must be between 1 minute and 3 hours")
		return false
	}
	if checkPlan {
		ent := h.entitlementsFor(r.Context(), h.userID(r))
		if sc.DurationSeconds > ent.MaxSessionSeconds() {
			writePlanLimit(w, ent)
			return false
		}
	}
	if checkContent {
		if sc.VoiceID != "" {
			v, err := h.audio.VoiceByID(r.Context(), sc.VoiceID)
			if errors.Is(err, store.ErrNotFound) || (err == nil && (v.Status != "active" || !v.Playable)) {
				writeCode(w, http.StatusUnprocessableEntity, "VOICE_UNAVAILABLE", "choose an active voice with published audio")
				return false
			}
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "failed to validate voice")
				return false
			}
		}
		if len(sc.CategoryIDs) > 39 {
			writeCode(w, http.StatusBadRequest, "SCHEDULE_INVALID", "choose at most 39 categories")
			return false
		}
		cats := make([]string, 0, len(sc.CategoryIDs))
		seenCats := map[string]bool{}
		for _, id := range sc.CategoryIDs {
			c, err := h.cont.CategoryByID(r.Context(), id)
			if errors.Is(err, store.ErrNotFound) || (err == nil && c.Status != "published") {
				writeCode(w, http.StatusUnprocessableEntity, "CATEGORY_UNAVAILABLE", "choose published categories")
				return false
			}
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "failed to validate categories")
				return false
			}
			if !seenCats[id] {
				cats = append(cats, id)
				seenCats[id] = true
			}
		}
		sc.CategoryIDs = cats
	}
	return true
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
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	if req.DaysOfWeek == nil {
		req.DaysOfWeek = []int{1, 2, 3, 4, 5, 6, 7}
	}
	if req.DurationSeconds == 0 {
		req.DurationSeconds = min(1800, h.entitlementsFor(r.Context(), h.userID(r)).MaxSessionSeconds())
	}
	sc := &models.Schedule{
		UserID: h.userID(r), Label: req.Label, Time: req.Time,
		DaysOfWeek: req.DaysOfWeek, Timezone: req.Timezone,
		DurationSeconds: req.DurationSeconds, VoiceID: req.VoiceID,
		CategoryIDs: req.CategoryIDs, Enabled: req.Enabled == nil || *req.Enabled,
	}
	if !h.validateSchedule(w, r, sc, true, true) {
		return
	}
	if err := h.sched.Create(r.Context(), sc); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create schedule")
		return
	}
	setNextScheduleRun(sc, time.Now())
	httpx.WriteJSON(w, http.StatusCreated, sc)
}

func (h *Handler) updateSchedule(w http.ResponseWriter, r *http.Request) {
	sc, err := h.sched.ByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "schedule not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load schedule")
		return
	}
	if sc.UserID != h.userID(r) {
		httpx.WriteError(w, http.StatusNotFound, "schedule not found")
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
	// A downgraded user must still be able to pause/delete an old Premium
	// routine. Re-check the plan when changing its duration or re-enabling it.
	checkPlan := req.DurationSeconds != nil || (req.Enabled != nil && *req.Enabled)
	// Existing schedules may have been saved before catalogue/audio validation
	// was tightened. Re-validate their selected voice and categories on every
	// update so a label edit cannot preserve an impossible routine.
	checkContent := true
	if !h.validateSchedule(w, r, sc, checkPlan, checkContent) {
		return
	}
	if err := h.sched.Update(r.Context(), sc); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "schedule not found")
		} else {
			httpx.WriteError(w, http.StatusInternalServerError, "failed to update schedule")
		}
		return
	}
	setNextScheduleRun(sc, time.Now())
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
