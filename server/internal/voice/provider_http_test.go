package voice

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// The shared plumbing carries the guarantees every adapter depends on, so it
// is tested directly rather than through one provider.

func TestReadProviderAudioBoundsThePayload(t *testing.T) {
	// One byte past the cap. The point is that the adapter never allocates
	// the whole thing: a hostile or broken endpoint cannot exhaust a worker.
	res := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(&prefixReader{n: maxAudioBytes + 1}),
	}
	got, err := readProviderAudio(res)
	if err == nil {
		t.Fatalf("expected an error, got %d bytes", len(got))
	}
	if !errors.Is(err, ErrPermanent) {
		t.Errorf("oversized body = %v, want permanent", err)
	}
}

// prefixReader yields n zero bytes without materialising them, so the bound
// can be tested for the size of a real cap rather than a token one.
type prefixReader struct {
	n    int
	done int
}

func (r *prefixReader) Read(p []byte) (int, error) {
	if r.done >= r.n {
		return 0, io.EOF
	}
	remaining := r.n - r.done
	if len(p) > remaining {
		p = p[:remaining]
	}
	for i := range p {
		p[i] = 0
	}
	r.done += len(p)
	return len(p), nil
}

func TestReadProviderAudioTreatsEmptyAsAFault(t *testing.T) {
	// A 200 with no body would otherwise become a zero-byte asset and a job
	// marked succeeded. Silence is representable; nothing at all is a failure.
	res := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
	}
	if _, err := readProviderAudio(res); !IsRetryable(err) {
		t.Errorf("empty body = %v, want retryable", err)
	}
}

func TestReadProviderAudioAcceptsASmallPayload(t *testing.T) {
	res := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("audio")),
	}
	got, err := readProviderAudio(res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "audio" {
		t.Errorf("body = %q, want %q", got, "audio")
	}
}

func TestClassifyProviderStatusSplitsOnRetryability(t *testing.T) {
	cases := []struct {
		status    int
		wantRetry bool
	}{
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusBadRequest, false},
		{http.StatusNotFound, false},
	}
	for _, c := range cases {
		res := &http.Response{
			StatusCode: c.status,
			Body:       io.NopCloser(strings.NewReader("detail")),
		}
		err := classifyProviderStatus(res)
		if IsRetryable(err) != c.wantRetry {
			t.Errorf("status %d: retryable = %v, want %v", c.status, IsRetryable(err), c.wantRetry)
		}
		if !strings.Contains(err.Error(), "detail") {
			t.Errorf("status %d: error lost the provider detail: %v", c.status, err)
		}
	}
}

func TestClassifyProviderStatusTruncatesAHugeErrorBody(t *testing.T) {
	// The diagnostic snippet is itself bounded: an endpoint that answers a
	// failure with gigabytes must not get them read into memory just to be
	// put in a log line.
	res := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(&prefixReader{n: 8 << 20}),
	}
	err := classifyProviderStatus(res)
	if !IsRetryable(err) {
		t.Errorf("500 = %v, want retryable", err)
	}
	if n := len(err.Error()); n > 4096 {
		t.Errorf("error message is %d bytes; the snippet should be truncated", n)
	}
}

func TestSynthesizeHTTPReportsATransportFailureAsRetryable(t *testing.T) {
	// A connection that cannot be opened is transient: there is no response
	// to inspect, so the only safe assumption is to try again.
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:1/nothing-listening", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if _, err := synthesizeHTTP(nil, req); !IsRetryable(err) {
		t.Errorf("transport failure = %v, want retryable", err)
	}
}

func TestRegionalLocaleAgreesAcrossProviders(t *testing.T) {
	// Two adapters resolving "yo" to different countries would render the
	// same confession in two accents depending on which provider served it.
	for _, code := range []string{"yo", "ig", "ha", "sw", "en"} {
		if _, ok := regionalLocale[code]; !ok {
			t.Errorf("regionalLocale is missing %q", code)
		}
	}
	if regionalLocale["yo"] != "yo-NG" {
		t.Errorf("yo = %q, want yo-NG: this is a Nigerian product", regionalLocale["yo"])
	}
}
