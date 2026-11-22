package api

import (
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Request latency (§7.2, PHASE 7 performance budgets: backend p50/p95/p99).
//
// A fixed-bucket histogram rather than a list of samples: recording is a few
// atomic adds with no lock and no allocation, memory is constant, and the
// buckets are exactly what a Prometheus histogram exposes, so a scraper can
// compute its own quantiles across replicas. The in-process p50/p95/p99 are
// estimated from the buckets the same way PromQL's histogram_quantile does,
// which bounds their error by the bucket width.

// latencyBucketsMs are the upper bounds of each bucket, in milliseconds. The
// final, implicit bucket is +Inf.
var latencyBucketsMs = []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

// RequestLatency is a lock-free latency histogram.
type RequestLatency struct {
	buckets  []atomic.Int64 // len(latencyBucketsMs)+1; last is +Inf
	count    atomic.Int64
	sumMicro atomic.Int64 // total duration in microseconds
	errors   atomic.Int64 // responses with status >= 500
}

// NewRequestLatency returns an empty histogram.
func NewRequestLatency() *RequestLatency {
	return &RequestLatency{buckets: make([]atomic.Int64, len(latencyBucketsMs)+1)}
}

// Observe records one request.
func (l *RequestLatency) Observe(d time.Duration, status int) {
	if l == nil {
		return
	}
	ms := float64(d) / float64(time.Millisecond)
	i := len(latencyBucketsMs) // +Inf
	for j, ub := range latencyBucketsMs {
		if ms <= ub {
			i = j
			break
		}
	}
	l.buckets[i].Add(1)
	l.count.Add(1)
	l.sumMicro.Add(d.Microseconds())
	if status >= 500 {
		l.errors.Add(1)
	}
}

// LatencySnapshot is a point-in-time copy of the histogram.
type LatencySnapshot struct {
	Count    int64   `json:"count"`
	Errors5x int64   `json:"errors_5xx"`
	MeanMs   float64 `json:"mean_ms"`
	P50Ms    float64 `json:"p50_ms"`
	P95Ms    float64 `json:"p95_ms"`
	P99Ms    float64 `json:"p99_ms"`
	// Buckets are cumulative counts keyed by upper bound in ms ("+Inf" last),
	// the shape a Prometheus histogram uses.
	Buckets []LatencyBucket `json:"buckets"`

	sumSeconds float64
}

// LatencyBucket is one cumulative histogram bucket.
type LatencyBucket struct {
	LeMs  string `json:"le_ms"`
	Count int64  `json:"count"`
}

// Snapshot copies the histogram and estimates its quantiles. Counters are
// read one at a time, so a snapshot taken under load can be off by the few
// requests that finished while it was read; it is never inconsistent enough to
// produce a quantile outside the bucket range.
func (l *RequestLatency) Snapshot() LatencySnapshot {
	if l == nil {
		return LatencySnapshot{}
	}
	raw := make([]int64, len(l.buckets))
	var total int64
	for i := range l.buckets {
		raw[i] = l.buckets[i].Load()
		total += raw[i]
	}
	s := LatencySnapshot{
		Count:      total,
		Errors5x:   l.errors.Load(),
		sumSeconds: float64(l.sumMicro.Load()) / 1e6,
	}
	if total > 0 {
		s.MeanMs = round2(s.sumSeconds * 1000 / float64(total))
	}
	var cum int64
	for i, n := range raw {
		cum += n
		le := "+Inf"
		if i < len(latencyBucketsMs) {
			le = strconv.FormatFloat(latencyBucketsMs[i], 'f', -1, 64)
		}
		s.Buckets = append(s.Buckets, LatencyBucket{LeMs: le, Count: cum})
	}
	s.P50Ms = quantile(0.50, raw, total)
	s.P95Ms = quantile(0.95, raw, total)
	s.P99Ms = quantile(0.99, raw, total)
	return s
}

// quantile estimates the q-th quantile by linear interpolation inside the
// bucket that contains it. A quantile that lands in the +Inf bucket reports
// the largest finite bound: the histogram cannot say how far beyond it lies,
// and inventing a number would be worse than saying "at least this".
func quantile(q float64, raw []int64, total int64) float64 {
	if total == 0 {
		return 0
	}
	rank := q * float64(total)
	var cum int64
	for i, n := range raw {
		if float64(cum+n) >= rank && n > 0 {
			if i == len(latencyBucketsMs) {
				return latencyBucketsMs[len(latencyBucketsMs)-1]
			}
			lower := 0.0
			if i > 0 {
				lower = latencyBucketsMs[i-1]
			}
			upper := latencyBucketsMs[i]
			frac := (rank - float64(cum)) / float64(n)
			return round2(lower + (upper-lower)*frac)
		}
		cum += n
	}
	return latencyBucketsMs[len(latencyBucketsMs)-1]
}

// WriteProm appends the histogram in Prometheus text exposition format.
func (s LatencySnapshot) WriteProm(b *strings.Builder) {
	b.WriteString("# HELP http_request_duration_seconds API request latency, excluding health, metrics and streaming routes.\n")
	b.WriteString("# TYPE http_request_duration_seconds histogram\n")
	for _, bk := range s.Buckets {
		le := bk.LeMs
		if le != "+Inf" {
			ms, _ := strconv.ParseFloat(le, 64)
			le = strconv.FormatFloat(ms/1000, 'f', -1, 64)
		}
		b.WriteString(`http_request_duration_seconds_bucket{le="` + le + `"} ` + itoa64(bk.Count) + "\n")
	}
	b.WriteString("http_request_duration_seconds_sum " + strconv.FormatFloat(s.sumSeconds, 'f', 6, 64) + "\n")
	b.WriteString("http_request_duration_seconds_count " + itoa64(s.Count) + "\n")
	b.WriteString("# TYPE http_requests_5xx_total counter\n")
	b.WriteString("http_requests_5xx_total " + itoa64(s.Errors5x) + "\n")
}

// latencyExcluded reports requests that would distort the API's latency:
// probes and scrapes (frequent, trivially fast), media delivery (duration is
// the size of the file and the listener's bandwidth) and live voice streams
// (open for the length of the render).
func latencyExcluded(r *http.Request) bool {
	p := r.URL.Path
	switch {
	case p == "/healthz", strings.HasPrefix(p, "/health/"),
		p == "/metrics", p == "/v1/metrics",
		strings.HasPrefix(p, "/media/"),
		strings.HasSuffix(p, "/voices/stream"):
		return true
	}
	return false
}
