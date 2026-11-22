package api

import (
	"log"
	"net/http"
	"time"
)

// logRequests is a minimal structured access-log middleware. It also feeds
// the request-latency histogram, so every request is timed exactly once.
func logRequests(next http.Handler, latency *RequestLatency) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		elapsed := time.Since(start)
		if !latencyExcluded(r) {
			latency.Observe(elapsed, rec.status)
		}
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, elapsed)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
