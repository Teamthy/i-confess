package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/store"
)

// Private study data: the journal and Scripture memory. Both used to live
// only in a browser's localStorage.

const (
	maxJournalTitle = 120
	maxJournalBody  = 20000
	maxMood         = 32
)

// listJournal returns one page of entries, newest first. `next_cursor` is
// set while older entries remain; pass it back as `?cursor=` for the next
// page. Without paging, entries past the first page could not be seen or
// deleted from any client.
func (h *Handler) listJournal(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeCode(w, http.StatusBadRequest, "JOURNAL_INVALID", "limit must be between 1 and 200")
			return
		}
		limit = n
	}
	list, next, err := h.study.ListJournalPage(r.Context(), h.userID(r), limit, r.URL.Query().Get("cursor"))
	if errors.Is(err, store.ErrBadCursor) {
		writeCode(w, http.StatusBadRequest, "JOURNAL_INVALID", "invalid cursor")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load your journal")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": list, "next_cursor": next})
}

type journalInput struct {
	Title      string `json:"title"`
	Body       string `json:"body"`
	Mood       string `json:"mood"`
	RowVersion int    `json:"row_version"`
}

func (in *journalInput) valid(w http.ResponseWriter) bool {
	in.Title, in.Body, in.Mood = strings.TrimSpace(in.Title), strings.TrimSpace(in.Body), strings.TrimSpace(in.Mood)
	switch {
	case in.Body == "" || utf8.RuneCountInString(in.Body) > maxJournalBody:
		writeCode(w, http.StatusBadRequest, "JOURNAL_INVALID", "write an entry (up to 20,000 characters)")
	case utf8.RuneCountInString(in.Title) > maxJournalTitle:
		writeCode(w, http.StatusBadRequest, "JOURNAL_INVALID", "keep the title to 120 characters")
	case utf8.RuneCountInString(in.Mood) > maxMood:
		writeCode(w, http.StatusBadRequest, "JOURNAL_INVALID", "keep the mood short")
	default:
		if in.Title == "" {
			in.Title = "Entry — " + time.Now().UTC().Format("2 January 2006")
		}
		return true
	}
	return false
}

func (h *Handler) createJournal(w http.ResponseWriter, r *http.Request) {
	var in journalInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		writeCode(w, http.StatusBadRequest, "JOURNAL_INVALID", "invalid request body")
		return
	}
	if !in.valid(w) {
		return
	}
	e := &store.JournalEntry{Title: in.Title, Body: in.Body, Mood: in.Mood}
	if err := h.study.CreateJournal(r.Context(), h.userID(r), e); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save the entry")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, e)
}

func (h *Handler) updateJournal(w http.ResponseWriter, r *http.Request) {
	var in journalInput
	if err := httpx.DecodeJSON(r, &in); err != nil || in.RowVersion < 1 {
		writeCode(w, http.StatusBadRequest, "JOURNAL_INVALID", "row_version is required")
		return
	}
	if !in.valid(w) {
		return
	}
	e := &store.JournalEntry{ID: r.PathValue("id"), Title: in.Title, Body: in.Body, Mood: in.Mood}
	switch err := h.study.UpdateJournal(r.Context(), h.userID(r), e, in.RowVersion); {
	case errors.Is(err, store.ErrNotFound):
		writeCode(w, http.StatusNotFound, "NOT_FOUND", "entry not found")
	case errors.Is(err, store.ErrStudyConflict):
		writeCode(w, http.StatusConflict, "VERSION_CONFLICT", "this entry changed on another device; reload it")
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save the entry")
	default:
		httpx.WriteJSON(w, http.StatusOK, e)
	}
}

func (h *Handler) deleteJournal(w http.ResponseWriter, r *http.Request) {
	switch err := h.study.DeleteJournal(r.Context(), h.userID(r), r.PathValue("id")); {
	case errors.Is(err, store.ErrNotFound):
		writeCode(w, http.StatusNotFound, "NOT_FOUND", "entry not found")
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "failed to delete the entry")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) listMemory(w http.ResponseWriter, r *http.Request) {
	list, err := h.study.ListMemory(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load your verses")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"verses": list})
}

func (h *Handler) addMemory(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reference string `json:"reference"`
		Text      string `json:"text"`
		Title     string `json:"title"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		writeCode(w, http.StatusBadRequest, "MEMORY_INVALID", "invalid request body")
		return
	}
	in.Reference, in.Text, in.Title = strings.TrimSpace(in.Reference), strings.TrimSpace(in.Text), strings.TrimSpace(in.Title)
	if in.Reference == "" || len(in.Reference) > 80 || in.Text == "" || utf8.RuneCountInString(in.Text) > 2000 || len(in.Title) > 200 {
		writeCode(w, http.StatusBadRequest, "MEMORY_INVALID", "a reference and its text are required")
		return
	}
	m := &store.MemoryVerse{Reference: in.Reference, Text: in.Text, Title: in.Title}
	created, err := h.study.AddMemory(r.Context(), h.userID(r), m, time.Now())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to add the verse")
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, map[string]any{"verse": m, "created": created})
}

func (h *Handler) gradeMemory(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Remembered *bool `json:"remembered"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil || in.Remembered == nil {
		writeCode(w, http.StatusBadRequest, "MEMORY_INVALID", "remembered (true or false) is required")
		return
	}
	m, err := h.study.GradeMemory(r.Context(), h.userID(r), r.PathValue("id"), *in.Remembered, time.Now())
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeCode(w, http.StatusNotFound, "NOT_FOUND", "verse not found")
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "failed to record the review")
	default:
		httpx.WriteJSON(w, http.StatusOK, m)
	}
}

func (h *Handler) deleteMemory(w http.ResponseWriter, r *http.Request) {
	switch err := h.study.DeleteMemory(r.Context(), h.userID(r), r.PathValue("id")); {
	case errors.Is(err, store.ErrNotFound):
		writeCode(w, http.StatusNotFound, "NOT_FOUND", "verse not found")
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "failed to remove the verse")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
