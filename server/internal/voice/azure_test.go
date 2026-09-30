package voice

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestAzure(handler http.HandlerFunc) (*Azure, *httptest.Server) {
	srv := httptest.NewServer(handler)
	a := NewAzure("test-key", "westeurope")
	// Point the region-derived host at the test server instead.
	a.Client = &http.Client{Transport: rewriteHost{srv: srv}}
	return a, srv
}

// rewriteHost sends every request to the test server regardless of the host
// the adapter built from its region, so the region logic stays under test
// rather than being stubbed out.
type rewriteHost struct{ srv *httptest.Server }

func (t rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(t.srv.URL, "http://")
	return http.DefaultTransport.RoundTrip(clone)
}

func TestAzureSynthesizeReturnsRawAudioWithTheRightType(t *testing.T) {
	want := []byte("ID3fake-mp3-bytes")

	var gotPath, gotKey, gotFormat string
	a, srv := newTestAzure(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("Ocp-Apim-Subscription-Key")
		gotFormat = r.Header.Get("X-Microsoft-OutputFormat")
		_, _ = w.Write(want)
	})
	defer srv.Close()

	got, err := a.Synthesize(context.Background(), SynthesisRequest{
		ProviderVoiceID: "en-US-AvaNeural", Text: "hello", Language: "en",
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
	if gotPath != "/cognitiveservices/v1" {
		t.Errorf("path = %q, want /cognitiveservices/v1", gotPath)
	}
	if gotKey != "test-key" {
		t.Errorf("subscription key header = %q", gotKey)
	}
	if gotFormat != "audio-24khz-96kbitrate-mono-mp3" {
		t.Errorf("output format = %q", gotFormat)
	}
}

func TestAzureSynthesizeEscapesTextBeforeBuildingSSML(t *testing.T) {
	// Confession text is user content and the body is XML. Without escaping,
	// this text would close the <voice> element and inject its own, changing
	// how the render sounds or worse.
	const hostile = `</voice><voice name='en-US-GuyNeural'>injected`
	const alsoHostile = `<script>alert(1)</script> & "quotes" 'single'`

	var body string
	a, srv := newTestAzure(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		_, _ = w.Write([]byte("audio"))
	})
	defer srv.Close()

	if _, err := a.Synthesize(context.Background(), SynthesisRequest{
		ProviderVoiceID: "en-US-AvaNeural", Text: hostile + alsoHostile, Language: "en",
	}); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	if !strings.HasPrefix(body, "<speak") || !strings.HasSuffix(body, "</speak>") {
		t.Fatalf("body is not a complete SSML document: %s", body)
	}
	if strings.Contains(body, "<script>") {
		t.Errorf("unescaped markup reached the SSML: %s", body)
	}
	if strings.Count(body, "<voice") != 1 {
		t.Errorf("expected exactly one <voice> element, got: %s", body)
	}
	for _, esc := range []string{"&lt;", "&gt;", "&amp;", "&quot;", "&apos;"} {
		if !strings.Contains(body, esc) {
			t.Errorf("expected %s in the escaped body: %s", esc, body)
		}
	}
}

func TestEscapeSSMLHandlesControlCharactersAndKeepsWhitespace(t *testing.T) {
	// XML 1.0 forbids most control characters outright; escaping them would
	// produce an invalid document, so they are dropped.
	if got := escapeSSML("a\x00b\x7fc"); got != "abc" {
		t.Errorf("control characters = %q, want %q", got, "abc")
	}
	// Newlines and tabs are legal and must survive: SSML uses them for pauses.
	if got := escapeSSML("a\nb\tc"); got != "a\nb\tc" {
		t.Errorf("whitespace = %q, want it preserved", got)
	}
	if got := escapeSSML(`&<>"'`); got != "&amp;&lt;&gt;&quot;&apos;" {
		t.Errorf("escaping = %q", got)
	}
}

func TestAzureRejectsIncompleteConfiguration(t *testing.T) {
	var called bool
	a, srv := newTestAzure(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte("audio"))
	})
	defer srv.Close()

	noKey := *a
	noKey.SubscriptionKey = ""
	if _, err := noKey.Synthesize(context.Background(), SynthesisRequest{
		ProviderVoiceID: "v", Text: "hi",
	}); !errors.Is(err, ErrPermanent) {
		t.Errorf("missing key = %v, want permanent", err)
	}

	noRegion := *a
	noRegion.Region = ""
	if _, err := noRegion.Synthesize(context.Background(), SynthesisRequest{
		ProviderVoiceID: "v", Text: "hi",
	}); !errors.Is(err, ErrPermanent) {
		t.Errorf("missing region = %v, want permanent", err)
	}

	// A blank region is the same as a missing one: it would build the host
	// "https://.tts.speech.microsoft.com" and fail as a DNS error, which the
	// queue would retry for an hour.
	blankRegion := *a
	blankRegion.Region = "   "
	if _, err := blankRegion.Synthesize(context.Background(), SynthesisRequest{
		ProviderVoiceID: "v", Text: "hi",
	}); !errors.Is(err, ErrPermanent) {
		t.Errorf("blank region = %v, want permanent", err)
	}

	if called {
		t.Error("the provider must not be called with incomplete configuration")
	}
}

func TestAzureClassifiesProviderFaults(t *testing.T) {
	cases := []struct {
		status    int
		wantRetry bool
	}{
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		// Azure answers an unsupported voice with 400; retrying cannot help.
		{http.StatusBadRequest, false},
	}
	for _, c := range cases {
		a, srv := newTestAzure(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte("azure said no"))
		})
		defer srv.Close()

		_, err := a.Synthesize(context.Background(), SynthesisRequest{
			ProviderVoiceID: "v", Text: "hi",
		})
		if err == nil {
			t.Fatalf("status %d: expected an error", c.status)
		}
		if IsRetryable(err) != c.wantRetry {
			t.Errorf("status %d: retryable = %v, want %v", c.status, IsRetryable(err), c.wantRetry)
		}
	}
}

func TestAzureLanguageResolutionPrefersTheVoiceLocale(t *testing.T) {
	cases := []struct{ lang, voice, want string }{
		{"", "en-GB-SoniaNeural", "en-GB"},
		{"en-GB", "en-US-AvaNeural", "en-GB"},
		{"ha", "some-voice", "ha-NG"},
		{"", "some-voice", "en-US"},
	}
	for _, c := range cases {
		if got := azureLanguage(c.lang, c.voice, "en-US"); got != c.want {
			t.Errorf("azureLanguage(%q, %q) = %q, want %q", c.lang, c.voice, got, c.want)
		}
	}
}

func TestAzureCodecMapping(t *testing.T) {
	cases := map[string][2]string{
		"audio-24khz-96kbitrate-mono-mp3": {"mp3", "audio/mpeg"},
		"audio-16khz-32kbitrate-mono-mp3": {"mp3", "audio/mpeg"},
		"riff-24khz-16bit-mono-pcm":       {"pcm_s16le", "audio/wav"},
		"ogg-24khz-16bit-mono-opus":       {"opus", "audio/ogg"},
	}
	for in, want := range cases {
		codec, ct := azureCodec(in)
		if codec != want[0] || ct != want[1] {
			t.Errorf("azureCodec(%q) = %q/%q, want %q/%q", in, codec, ct, want[0], want[1])
		}
	}
	if codec, _ := azureCodec("sil-16khz-16bit-mono-siren"); codec != "" {
		t.Errorf("an unservable format = %q, want empty", codec)
	}
}
