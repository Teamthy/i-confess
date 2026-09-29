package api

import (
	"errors"
	"net/http"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/store"
)

func (h *Handler) listCollections(w http.ResponseWriter, r *http.Request) {
	cols, err := h.cont.ListCollections(r.Context(), false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load collections")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cols)
}

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.cont.ListCategories(r.Context(), false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load categories")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cats)
}

func (h *Handler) categoryConfessions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	confs, err := h.cont.ConfessionsByCategory(r.Context(), id, true)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confessions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, confs)
}

func (h *Handler) getConfession(w http.ResponseWriter, r *http.Request) {
	c, err := h.cont.ConfessionByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "confession not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confession")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (h *Handler) listVoices(w http.ResponseWriter, r *http.Request) {
	voices, err := h.audio.ListVoices(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load voices")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, voices)
}

// ---------- Sessions ----------
