package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLatencyQuantilesInterpolateWithinBuckets(t *testing.T) {
	l := NewRequestLatency()
	// 90 requests in (0,5ms], 9 in (100,250ms], 1 in (1000,2500ms].
	for i := 0; i < 90; i++ {
		l.Observe(2*time.Millisecond, 200)
	}
	for i := 0; i < 9; i++ {
		l.Observe(200*time.Millisecond, 200)
	}
	l.Observe(2*time.Second, 503)

	s := l.Snapshot()
	if s.Count != 100 || s.Errors5x != 1 {
		t.Fatalf("count/errors = %d/%d, want 100/1", s.Count, s.Errors5x)
	}
	// rank 50 of 90 in [0,5] -> 50/90*5
	if s.P50Ms != 2.78 {
		t.Errorf("p50 = %v, want 2.78", s.P50Ms)
	}
	// rank 95: 5 into the 9 in [100,250] -> 100 + 5/9*150
	if s.P95Ms != 183.33 {
		t.Errorf("p95 = %v, want 183.33", s.P95Ms)
	}
	// rank 99: exactly the top of the [100,250] bucket
	if s.P99Ms != 250 {
		t.Errorf("p99 = %v, want 250", s.P99Ms)
	}
	if last := s.Buckets[len(s.Buckets)-1]; last.LeMs != "+Inf" || last.Count != 100 {
		t.Errorf("+Inf bucket = %+v, want cumulative 100", last)
	}
	// mean = (90*2 + 9*200 + 2000) / 100
	if s.MeanMs != 39.8 {
		t.Errorf("mean = %v, want 39.8", s.MeanMs)
	}
}

func TestLatencyBeyondLastBucketReportsTheBound(t *testing.T) {
	l := NewRequestLatency()
	l.Observe(30*time.Second, 200)
	if got := l.Snapshot().P99Ms; got != 10000 {
		t.Fatalf("p99 = %v, want the largest finite bound 10000", got)
	}
	if got := NewRequestLatency().Snapshot(); got.Count != 0 || got.P99Ms != 0 {
		t.Fatalf("empty snapshot = %+v", got)
	}
}

func TestLatencyExcludesProbesScrapesAndStreams(t *testing.T) {
	for path, want := range map[string]bool{
		"/healthz": true, "/health/ready": true, "/metrics": true, "/v1/metrics": true,
		"/media/audio/a.mp3": true, "/v1/voices/stream": true,
		"/categories": false, "/v1/ai/parse": false, "/voices": false,
	} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if got := latencyExcluded(r); got != want {
			t.Errorf("latencyExcluded(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestMetricsReportRequestLatency drives real requests through the router and
// reads them back from /metrics in both formats. The old p99 was a TODO in a
// package nothing imported, so /metrics never carried latency at all.
func TestMetricsReportRequestLatency(t *testing.T) {
	_, srv, token := adminClient(t)

	for i := 0; i < 3; i++ {
		resp, err := http.Get(srv.URL + "/categories")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	get := func(path, accept string) string {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d %s", path, resp.StatusCode, body)
		}
		return string(body)
	}

	var out struct {
		Latency LatencySnapshot `json:"latency"`
	}
	if err := json.Unmarshal([]byte(get("/metrics", "")), &out); err != nil {
		t.Fatal(err)
	}
	// Registration and sign-in in adminClient also count; /metrics does not.
	if out.Latency.Count < 3 {
		t.Fatalf("latency count = %d, want at least the 3 catalogue reads", out.Latency.Count)
	}
	if out.Latency.P99Ms <= 0 || out.Latency.P50Ms > out.Latency.P99Ms {
		t.Fatalf("implausible quantiles: %+v", out.Latency)
	}

	before := out.Latency.Count
	prom := get("/v1/metrics", "text/plain")
	for _, want := range []string{
		"# TYPE http_request_duration_seconds histogram",
		`http_request_duration_seconds_bucket{le="0.005"} `,
		`http_request_duration_seconds_bucket{le="+Inf"} `,
		"http_request_duration_seconds_count ",
		"http_requests_5xx_total 0",
		"login_success_total ",
	} {
		if !strings.Contains(prom, want) {
			t.Errorf("prometheus output missing %q:\n%s", want, prom)
		}
	}
	// Scraping must not count itself.
	if err := json.Unmarshal([]byte(get("/metrics", "")), &out); err != nil {
		t.Fatal(err)
	}
	if out.Latency.Count != before {
		t.Errorf("scraping /metrics changed the request count: %d -> %d", before, out.Latency.Count)
	}
}
