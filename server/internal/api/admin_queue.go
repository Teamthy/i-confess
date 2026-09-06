package api

import (
	"net/http"
	"strconv"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/workers"
)

// adminQueueStats reports what the background queue is holding.
//
// A queue with no window into it is a queue nobody operates. The number that
// matters is dead_letter: those are jobs that gave up, and until somebody looks,
// the work they represent is simply not happening.
func (h *Handler) adminQueueStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.queue.Stats(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not read queue statistics")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"stats":   stats,
		"types":   workers.AllTypes(),
		"durable": h.queueDurable,
	})
}

// adminQueueRequeue puts parked jobs back in the queue.
//
// This is the operational half of a dead-letter strategy. Parking a job rather
// than deleting it is only useful if a human can release it once the cause - a
// provider outage, a missing configuration - has been fixed.
func (h *Handler) adminQueueRequeue(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 1000 {
			httpx.WriteError(w, http.StatusBadRequest, "limit must be between 1 and 1000")
			return
		}
		limit = n
	}

	n, err := h.queue.RequeueDead(r.Context(), limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not requeue dead-lettered jobs")
		return
	}
	stats, _ := h.queue.Stats(r.Context())
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"requeued": n,
		"stats":    stats,
	})
}
