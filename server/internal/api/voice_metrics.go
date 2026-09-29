package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Teamthy/i-confess/internal/httpx"
)

// voiceMetricsSet is the in-process voice telemetry (§58, §82). It is
// deliberately small: counters and latency summaries per engine that an
// operator can scrape (Prometheus text) or read as JSON in the admin UI.
// Every API/worker process keeps its own; aggregation is the scraper's job.
type voiceMetricsSet struct {
	mu sync.Mutex

	cacheHits, cacheMisses int64
	rightsRefusals         int64
	contentRefusals        int64
	fallbacks              int64
	streams                int64

	generated map[string]*latency // by engine: synthesis+upload time
	queueWait latency             // enqueue -> worker start
	firstByte latency             // stream request -> first audio byte
	failed    map[string]int64    // by error class
}

type latency struct {
	N         int64   `json:"n"`
	SumMS     float64 `json:"sumMs"`
	MaxMS     float64 `json:"maxMs"`
	bucketsMS [len(latencyBuckets) + 1]int64
}

// latencyBuckets are upper bounds in ms for the histogram.
var latencyBuckets = [...]float64{250, 500, 1000, 2500, 5000, 10000, 30000, 60000, 300000}

func (l *latency) observe(ms float64) {
	l.N++
	l.SumMS += ms
	if ms > l.MaxMS {
		l.MaxMS = ms
	}
	i := sort.SearchFloat64s(latencyBuckets[:], ms)
	l.bucketsMS[i]++
}

func (l *latency) meanMS() float64 {
	if l.N == 0 {
		return 0
	}
	return l.SumMS / float64(l.N)
}

var voiceMetrics = &voiceMetricsSet{generated: map[string]*latency{}, failed: map[string]int64{}}

func (m *voiceMetricsSet) cache(miss bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if miss {
		m.cacheMisses++
	} else {
		m.cacheHits++
	}
}

func (m *voiceMetricsSet) generatedIn(engine string, d time.Duration, fellBack bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.generated[engine]
	if l == nil {
		l = &latency{}
		m.generated[engine] = l
	}
	l.observe(float64(d.Milliseconds()))
	if fellBack {
		m.fallbacks++
	}
}

func (m *voiceMetricsSet) failure(class string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failed[class]++
	if class == "rights" {
		m.rightsRefusals++
	}
}

func (m *voiceMetricsSet) refusedContent() {
	m.mu.Lock()
	m.contentRefusals++
	m.mu.Unlock()
}

func (m *voiceMetricsSet) refusedRights() {
	m.mu.Lock()
	m.rightsRefusals++
	m.mu.Unlock()
}

func (m *voiceMetricsSet) queued(wait time.Duration) {
	m.mu.Lock()
	m.queueWait.observe(float64(wait.Milliseconds()))
	m.mu.Unlock()
}

func (m *voiceMetricsSet) streamStarted(ttfb time.Duration) {
	m.mu.Lock()
	m.streams++
	m.firstByte.observe(float64(ttfb.Milliseconds()))
	m.mu.Unlock()
}

// snapshot is the JSON view.
func (m *voiceMetricsSet) snapshot() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	engines := map[string]any{}
	var ok int64
	for e, l := range m.generated {
		ok += l.N
		engines[e] = map[string]any{"n": l.N, "meanMs": round2(l.meanMS()), "maxMs": l.MaxMS}
	}
	var failed int64
	fails := map[string]int64{}
	for c, n := range m.failed {
		failed += n
		fails[c] = n
	}
	hitRate, successRate := 0.0, 0.0
	if t := m.cacheHits + m.cacheMisses; t > 0 {
		hitRate = float64(m.cacheHits) / float64(t)
	}
	if t := ok + failed; t > 0 {
		successRate = float64(ok) / float64(t)
	}
	return map[string]any{
		"cacheHits": m.cacheHits, "cacheMisses": m.cacheMisses, "cacheHitRate": round2(hitRate),
		"generated": ok, "failed": failed, "failedByClass": fails, "successRate": round2(successRate),
		"byEngine": engines, "fallbacks": m.fallbacks,
		"rightsRefusals": m.rightsRefusals, "contentRefusals": m.contentRefusals,
		"queueWaitMeanMs": round2(m.queueWait.meanMS()), "queueWaitMaxMs": m.queueWait.MaxMS,
		"streams": m.streams, "streamFirstByteMeanMs": round2(m.firstByte.meanMS()),
	}
}

// prometheus renders the Prometheus text exposition format.
func (m *voiceMetricsSet) prometheus() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	counter := func(name, help string, v int64) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", name, help, name, name, v)
	}
	counter("icf_voice_cache_hits_total", "Generation requests served from the content-hash cache.", m.cacheHits)
	counter("icf_voice_cache_misses_total", "Generation requests that queued a render.", m.cacheMisses)
	counter("icf_voice_rights_refusals_total", "Requests or jobs refused by the rights check.", m.rightsRefusals)
	counter("icf_voice_content_refusals_total", "Requests refused by the script safety floor.", m.contentRefusals)
	counter("icf_voice_fallbacks_total", "Renders produced by an approved fallback model.", m.fallbacks)
	counter("icf_voice_streams_total", "Live preview streams started.", m.streams)

	b.WriteString("# HELP icf_voice_failures_total Failed renders by error class.\n# TYPE icf_voice_failures_total counter\n")
	classes := make([]string, 0, len(m.failed))
	for c := range m.failed {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	for _, c := range classes {
		fmt.Fprintf(&b, "icf_voice_failures_total{class=%q} %d\n", c, m.failed[c])
	}

	hist := func(name, help, labels string, l *latency) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s histogram\n", name, help, name)
		var cum int64
		for i, ub := range latencyBuckets {
			cum += l.bucketsMS[i]
			fmt.Fprintf(&b, "%s_bucket{%sle=\"%g\"} %d\n", name, labels, ub/1000, cum)
		}
		cum += l.bucketsMS[len(latencyBuckets)]
		fmt.Fprintf(&b, "%s_bucket{%sle=\"+Inf\"} %d\n", name, labels, cum)
		fmt.Fprintf(&b, "%s_sum{%s} %g\n%s_count{%s} %d\n", name, strings.TrimSuffix(labels, ","), l.SumMS/1000,
			name, strings.TrimSuffix(labels, ","), l.N)
	}
	engines := make([]string, 0, len(m.generated))
	for e := range m.generated {
		engines = append(engines, e)
	}
	sort.Strings(engines)
	for _, e := range engines {
		hist("icf_voice_generation_seconds", "Synthesis plus upload time per render.", fmt.Sprintf("engine=%q,", e), m.generated[e])
	}
	hist("icf_voice_queue_wait_seconds", "Time from enqueue to worker start.", "", &m.queueWait)
	hist("icf_voice_stream_first_byte_seconds", "Time to first audio byte on live preview.", "", &m.firstByte)
	return b.String()
}

func round2(f float64) float64 { return float64(int64(f*100+0.5)) / 100 }

func (h *Handler) adminVoiceMetrics(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("format") == "prometheus" {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(voiceMetrics.prometheus()))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"metrics": voiceMetrics.snapshot(),
		"note": "Per-process counters since start. Scrape each instance (format=prometheus) and aggregate."})
}
