package voice

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

// Shared HTTP plumbing for provider adapters.
//
// Every adapter faces the same three hazards, and each one has to be handled
// identically or the queue's retry policy becomes meaningless:
//
//   - an unbounded response body, which lets a hostile or broken endpoint
//     exhaust the worker's memory;
//   - a non-200 status whose class decides whether retrying can possibly help;
//   - a 200 with no audio, which is a fault and not a success.
//
// The rules live here so a new adapter inherits them rather than re-deriving
// them, and so "retryable" means the same thing across providers.

// defaultProviderTimeout bounds one provider call. A render is a paid,
// user-visible operation; without a ceiling a hung socket pins a queue worker
// for as long as the OS keeps the connection alive.
const defaultProviderTimeout = 120 * time.Second

// newProviderClient builds the HTTP client an adapter should use by default.
func newProviderClient() *http.Client {
	return &http.Client{Timeout: defaultProviderTimeout}
}

// synthesizeHTTP performs a prepared request and returns the audio payload.
//
// The caller owns building the request, because the shape differs per provider;
// this owns everything that must not differ: status classification, the read
// bound, and the distinction between "no audio" and "empty but valid".
func synthesizeHTTP(client *http.Client, req *http.Request) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}

	res, err := client.Do(req)
	if err != nil {
		// A transport failure - DNS, TLS, timeout, reset - is transient by
		// nature. There is no request we can inspect to prove otherwise.
		return nil, RetryableError("provider request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, classifyProviderStatus(res)
	}

	return readProviderAudio(res)
}

// readProviderAudio reads the response body under a hard bound.
func readProviderAudio(res *http.Response) ([]byte, error) {
	audio, err := io.ReadAll(io.LimitReader(res.Body, maxAudioBytes+1))
	if err != nil {
		return nil, RetryableError("read audio: %v", err)
	}
	if len(audio) > maxAudioBytes {
		// Re-reading will not make the payload smaller.
		return nil, PermanentError("provider returned more than %d bytes", maxAudioBytes)
	}
	if len(audio) == 0 {
		// A 200 with no body is a broken render, not an empty one. Retrying is
		// cheap and is the only way this ever succeeds.
		return nil, RetryableError("provider returned no audio")
	}
	return audio, nil
}

// classifyProviderStatus maps a non-200 response onto the retry policy.
func classifyProviderStatus(res *http.Response) error {
	// Read a bounded snippet for diagnostics; the body may be huge or hostile.
	snippet, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	msg := strings.TrimSpace(string(snippet))

	switch {
	case res.StatusCode == http.StatusTooManyRequests, res.StatusCode >= 500:
		// Rate limits and server faults pass. Both are the queue's reason to
		// exist.
		return RetryableError("provider returned %d: %s", res.StatusCode, msg)
	case res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden:
		// Credentials are wrong or withdrawn. Retrying burns attempts and
		// hides the real cause behind a generic failure.
		return PermanentError("provider rejected credentials (%d): %s", res.StatusCode, msg)
	default:
		// 400-class: the request itself is wrong, so resending it unchanged
		// produces the same answer.
		return PermanentError("provider returned %d: %s", res.StatusCode, msg)
	}
}

// providerRequest builds a POST with a context, so a cancelled job stops the
// in-flight call instead of finishing a render nobody wants.
func providerRequest(ctx context.Context, url string, body io.Reader, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, PermanentError("build request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

// regionalLocale expands bare ISO-639-1 codes to full BCP-47 tags.
//
// Shared by every adapter that needs a locale: two providers resolving "sw" to
// different countries would render the same confession in two accents.
//
// The list is short on purpose: these are the languages this product ships
// content in. Anything else is left to the caller to name in full rather than
// guessed at, because a wrong region is a wrong accent.
var regionalLocale = map[string]string{
	"en": "en-US",
	"es": "es-ES",
	"fr": "fr-FR",
	"de": "de-DE",
	"it": "it-IT",
	"pt": "pt-BR",
	"nl": "nl-NL",
	"pl": "pl-PL",
	"ru": "ru-RU",
	"tr": "tr-TR",
	"ar": "ar-XA",
	"hi": "hi-IN",
	"id": "id-ID",
	"ja": "ja-JP",
	"ko": "ko-KR",
	"vi": "vi-VN",
	"zh": "cmn-CN",
	"af": "af-ZA",
	"sw": "sw-KE",
	"am": "am-ET",
	// Nigerian languages are first-class here: this is a Nigerian product and
	// the default region for these codes is not the US.
	"yo": "yo-NG",
	"ig": "ig-NG",
	"ha": "ha-NG",
}
