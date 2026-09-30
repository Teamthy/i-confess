package voice

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The adapters are tested against an httptest server rather than the real
// provider: these are contracts about how the adapter behaves, and they have
// to hold without a network, a key or a billing account.

func newTestGoogle(handler http.HandlerFunc) (*Google, *httptest.Server) {
	srv := httptest.NewServer(handler)
	g := NewGoogle("test-key")
	g.BaseURL = srv.URL
	return g, srv
}

func TestGoogleSynthesizeDecodesBase64Audio(t *testing.T) {
	want := []byte("RIFFfakeaudio-bytes")

	g, srv := newTestGoogle(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1/text:synthesize") {
			t.Errorf("path = %q, want /v1/text:synthesize", r.URL.Path)
		}
		// The key travels as a query parameter, because that is the only
		// credential this API accepts without a service-account file.
		if got := r.URL.Query().Get("key"); got != "test-key" {
			t.Errorf("key param = %q, want test-key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"audioContent":"` + base64.StdEncoding.EncodeToString(want) + `"}`))
	})
	defer srv.Close()

	got, err := g.Synthesize(context.Background(), SynthesisRequest{
		ProviderVoiceID: "en-US-Standard-A",
		Text:            "hello",
		Language:        "en",
	})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(got.Audio) != string(want) {
		t.Errorf("audio = %q, want %q", got.Audio, want)
	}
	if got.Codec != "mp3" {
		t.Errorf("codec = %q, want mp3", got.Codec)
	}
	if got.ContentType != "audio/mpeg" {
		t.Errorf("content type = %q, want audio/mpeg", got.ContentType)
	}
	// The pipeline measures duration from the container, so nothing is
	// estimated here and a stale guess cannot reach the asset row.
	if got.DurationSeconds != 0 {
		t.Errorf("duration = %d, want 0 (measured downstream)", got.DurationSeconds)
	}
}

func TestGoogleSynthesizeSendsTheVoiceAndLanguage(t *testing.T) {
	var body string
	g, srv := newTestGoogle(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		_, _ = w.Write([]byte(`{"audioContent":"` +
			base64.StdEncoding.EncodeToString([]byte("audio")) + `"}`))
	})
	defer srv.Close()

	if _, err := g.Synthesize(context.Background(), SynthesisRequest{
		ProviderVoiceID: "en-GB-Standard-B",
		Text:            "text",
		Language:        "en-GB",
	}); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	for _, want := range []string{`"name":"en-GB-Standard-B"`, `"languageCode":"en-GB"`, `"text":"text"`} {
		if !strings.Contains(body, want) {
			t.Errorf("request body missing %s: %s", want, body)
		}
	}
}

func TestGoogleRejectsBadInputBeforeCallingOut(t *testing.T) {
	g, srv := newTestGoogle(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the provider must not be called for an invalid request")
	})
	defer srv.Close()

	cases := map[string]SynthesisRequest{
		"no voice": {Text: "hi", ProviderVoiceID: ""},
		"no text":  {Text: "", ProviderVoiceID: "voice"},
		"blank":    {Text: "   ", ProviderVoiceID: "voice"},
	}
	for name, req := range cases {
		if _, err := g.Synthesize(context.Background(), req); !errors.Is(err, ErrPermanent) {
			t.Errorf("%s: err = %v, want a permanent error", name, err)
		}
	}

	// A missing key is permanent too: no request will succeed until an
	// operator sets one.
	noKey := NewGoogle("")
	noKey.BaseURL = srv.URL
	if _, err := noKey.Synthesize(context.Background(), SynthesisRequest{
		ProviderVoiceID: "voice", Text: "hi",
	}); !errors.Is(err, ErrPermanent) {
		t.Errorf("missing key: err = %v, want a permanent error", err)
	}
}

func TestGoogleClassifiesProviderFaults(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		wantRetry bool
	}{
		{"rate limited", http.StatusTooManyRequests, true},
		{"server error", http.StatusInternalServerError, true},
		{"bad gateway", http.StatusBadGateway, true},
		{"unauthorised", http.StatusUnauthorized, false},
		{"forbidden", http.StatusForbidden, false},
		{"bad request", http.StatusBadRequest, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, srv := newTestGoogle(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte("provider said no"))
			})
			defer srv.Close()

			_, err := g.Synthesize(context.Background(), SynthesisRequest{
				ProviderVoiceID: "voice", Text: "hi",
			})
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := IsRetryable(err); got != c.wantRetry {
				t.Errorf("IsRetryable = %v, want %v (%v)", got, c.wantRetry, err)
			}
			// The provider's own message has to survive: an operator cannot
			// diagnose a render from "permanent provider error" alone.
			if !strings.Contains(err.Error(), "provider said no") {
				t.Errorf("error lost the provider's message: %v", err)
			}
		})
	}
}

func TestGoogleTreatsAnEmptyEnvelopeAsAFault(t *testing.T) {
	// A 200 carrying no audioContent is a broken render. Returning an empty
	// result here would store a zero-byte asset and mark the job succeeded.
	g, srv := newTestGoogle(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"audioContent":""}`))
	})
	defer srv.Close()

	_, err := g.Synthesize(context.Background(), SynthesisRequest{
		ProviderVoiceID: "voice", Text: "hi",
	})
	if !IsRetryable(err) {
		t.Errorf("empty audioContent = %v, want retryable", err)
	}
}

func TestGoogleLanguageResolution(t *testing.T) {
	cases := []struct{ lang, voice, want string }{
		{"en", "en-US-Standard-A", "en-US"},
		{"en-GB", "en-US-Standard-A", "en-GB"},   // explicit tag wins over the voice
		{"", "en-GB-Standard-B", "en-GB"},        // voice's own locale
		{"yo", "some-voice", "yo-NG"},            // Nigerian languages map to NG
		{"", "some-voice", "en-US"},              // falls back
		{"xx", "some-voice", "en-US"},            // unknown code falls back
	}
	for _, c := range cases {
		if got := googleLanguage(c.lang, c.voice, "en-US"); got != c.want {
			t.Errorf("googleLanguage(%q, %q) = %q, want %q", c.lang, c.voice, got, c.want)
		}
	}
}

func TestGoogleCodecMapping(t *testing.T) {
	if codec, ct := googleCodec("MP3"); codec != "mp3" || ct != "audio/mpeg" {
		t.Errorf("MP3 -> %q/%q", codec, ct)
	}
	if codec, ct := googleCodec("LINEAR16"); codec != "pcm_s16le" || ct != "audio/wav" {
		t.Errorf("LINEAR16 -> %q/%q", codec, ct)
	}
	// An encoding the player cannot serve must not be recorded as mp3.
	if codec, _ := googleCodec("MULAW"); codec != "" {
		t.Errorf("MULAW -> %q, want empty", codec)
	}
}
