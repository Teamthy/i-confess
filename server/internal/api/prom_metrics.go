package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/tracing"
)

// promMetrics exposes Prometheus-style metrics + JSON snapshot.
// Phase 7 §7.2 observability — hit rate dashboard >85% for cache (§7.1).
func (h *Handler) promMetrics(w http.ResponseWriter, r *http.Request) {
	// Content-negotiate: Accept: text/plain -> Prometheus exposition; else JSON
	accept := r.Header.Get("Accept")
	isProm := accept == "text/plain" || r.URL.Query().Get("format") == "prom"

	if isProm {
		// lightweight Prom text for Grafana agent
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		var b strings.Builder
		if h.metrics != nil {
			// AuthMetrics counters, sorted so scrapes diff cleanly.
			snap := h.metrics.Snapshot()
			names := make([]string, 0, len(snap))
			for k := range snap {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, k := range names {
				b.WriteString(k + " " + itoa64(snap[k]) + "\n")
			}
		}
		h.latency.Snapshot().WriteProm(&b)
		_, _ = w.Write([]byte(b.String()))
		return
	}
	// JSON snapshot — includes cache hit rate if wired
	var cache map[string]any
	if h.cacheStats != nil {
		cs := h.cacheStats()
		cache = map[string]any{
			"hits":     cs.Hits,
			"misses":   cs.Misses,
			"stale":    cs.Stale,
			"hit_rate": cs.HitRate(),
			// Writes that dropped a cached entry here. A hit rate alone cannot
			// distinguish "the catalogue never changes" from "the catalogue
			// changed and nothing noticed".
			"invalidations": cs.Invalidations,
		}
	}
	traceID := tracing.TraceIDFromContext(r.Context())
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"trace_id": traceID,
		"counters": func() map[string]int64 {
			if h.metrics != nil {
				return h.metrics.Snapshot()
			}
			return map[string]int64{}
		}(),
		"cache": cache,
		// Request latency (p50/p95/p99 estimated from the histogram buckets).
		"latency": h.latency.Snapshot(),
	})
}

func itoa64(n int64) string {
	// avoid fmt for hot path
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
