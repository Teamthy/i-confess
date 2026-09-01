package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// Collector tracks application metrics for monitoring and alerting.
type Collector struct {
	mu sync.RWMutex

	// API metrics
	RequestsTotal      atomic.Int64 // total requests
	RequestsErrors     atomic.Int64 // failed requests
	RequestLatencyMs   atomic.Int64 // total latency (for averaging)
	RequestLatencyP99  atomic.Int64 // p99 latency

	// Job metrics
	JobsEnqueued       atomic.Int64 // total jobs created
	JobsProcessed      atomic.Int64 // total jobs completed
	JobsFailed         atomic.Int64 // total jobs failed
	JobsDeadLettered   atomic.Int64 // total jobs moved to dead-letter
	JobQueueDepth      atomic.Int64 // pending jobs

	// Database metrics
	DBConnections      atomic.Int64 // active connections
	DBQueryTimeMs      atomic.Int64 // total query time
	DBQueriesTotal     atomic.Int64 // total queries

	// Cache metrics (Redis)
	CacheHits          atomic.Int64 // cache hits
	CacheMisses        atomic.Int64 // cache misses
	CacheEvictions     atomic.Int64 // items evicted

	// Auth metrics
	LoginAttempts      atomic.Int64 // total login attempts
	LoginFailures      atomic.Int64 // failed logins

	// Background
	lastReportTime     time.Time
}

// New creates a new metrics collector.
func New() *Collector {
	return &Collector{
		lastReportTime: time.Now(),
	}
}

// RecordRequest records an API request.
func (c *Collector) RecordRequest(latencyMs int64, success bool) {
	c.RequestsTotal.Add(1)
	c.RequestLatencyMs.Add(latencyMs)
	if !success {
		c.RequestsErrors.Add(1)
	}
	// TODO: update p99 from histogram
}

// RecordJob records a background job.
func (c *Collector) RecordJob(latencyMs int64, success bool) {
	c.JobsProcessed.Add(1)
	if !success {
		c.JobsFailed.Add(1)
	}
}

// RecordJobEnqueued records a job being added to the queue.
func (c *Collector) RecordJobEnqueued(jobType string) {
	c.JobsEnqueued.Add(1)
	c.JobQueueDepth.Add(1)
}

// RecordJobDeadLettered records a job moved to dead-letter.
func (c *Collector) RecordJobDeadLettered() {
	c.JobsDeadLettered.Add(1)
	c.JobQueueDepth.Add(-1)
}

// RecordDatabaseQuery records a database operation.
func (c *Collector) RecordDatabaseQuery(latencyMs int64) {
	c.DBQueryTimeMs.Add(latencyMs)
	c.DBQueriesTotal.Add(1)
}

// RecordCacheHit records a cache hit.
func (c *Collector) RecordCacheHit() {
	c.CacheHits.Add(1)
}

// RecordCacheMiss records a cache miss.
func (c *Collector) RecordCacheMiss() {
	c.CacheMisses.Add(1)
}

// RecordLogin records a login attempt.
func (c *Collector) RecordLogin(success bool) {
	c.LoginAttempts.Add(1)
	if !success {
		c.LoginFailures.Add(1)
	}
}

// Snapshot captures current metrics.
func (c *Collector) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		RequestsTotal:     c.RequestsTotal.Load(),
		RequestsErrors:    c.RequestsErrors.Load(),
		JobsEnqueued:      c.JobsEnqueued.Load(),
		JobsProcessed:     c.JobsProcessed.Load(),
		JobsFailed:        c.JobsFailed.Load(),
		JobsDeadLettered:  c.JobsDeadLettered.Load(),
		JobQueueDepth:     c.JobQueueDepth.Load(),
		DBQueriesTotal:    c.DBQueriesTotal.Load(),
		CacheHits:         c.CacheHits.Load(),
		CacheMisses:       c.CacheMisses.Load(),
		LoginAttempts:     c.LoginAttempts.Load(),
		LoginFailures:     c.LoginFailures.Load(),
	}
}

// MetricsSnapshot represents a point-in-time snapshot of metrics.
type MetricsSnapshot struct {
	Timestamp        string `json:"timestamp"`
	RequestsTotal    int64  `json:"requests_total"`
	RequestsErrors   int64  `json:"requests_errors"`
	JobsEnqueued     int64  `json:"jobs_enqueued"`
	JobsProcessed    int64  `json:"jobs_processed"`
	JobsFailed       int64  `json:"jobs_failed"`
	JobsDeadLettered int64  `json:"jobs_dead_lettered"`
	JobQueueDepth    int64  `json:"job_queue_depth"`
	DBQueriesTotal   int64  `json:"db_queries_total"`
	CacheHits        int64  `json:"cache_hits"`
	CacheMisses      int64  `json:"cache_misses"`
	LoginAttempts    int64  `json:"login_attempts"`
	LoginFailures    int64  `json:"login_failures"`
}

// ErrorRatePercent calculates the error rate.
func (s *MetricsSnapshot) ErrorRatePercent() float64 {
	if s.RequestsTotal == 0 {
		return 0
	}
	return float64(s.RequestsErrors) / float64(s.RequestsTotal) * 100
}

// JobSuccessRatePercent calculates job success rate.
func (s *MetricsSnapshot) JobSuccessRatePercent() float64 {
	if s.JobsProcessed == 0 {
		return 100
	}
	return float64(s.JobsProcessed-s.JobsFailed) / float64(s.JobsProcessed) * 100
}

// CacheHitRatePercent calculates cache hit rate.
func (s *MetricsSnapshot) CacheHitRatePercent() float64 {
	total := s.CacheHits + s.CacheMisses
	if total == 0 {
		return 0
	}
	return float64(s.CacheHits) / float64(total) * 100
}

// LoginSuccessRatePercent calculates login success rate.
func (s *MetricsSnapshot) LoginSuccessRatePercent() float64 {
	if s.LoginAttempts == 0 {
		return 100
	}
	return float64(s.LoginAttempts-s.LoginFailures) / float64(s.LoginAttempts) * 100
}
