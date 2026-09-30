package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Google adapts Google Cloud Text-to-Speech.
//
// It is the fallback provider for voices ElevenLabs does not carry, and the
// cheapest one for high-volume renders, so it has to behave like the
// ElevenLabs adapter from the queue's point of view: the same read bound, the
// same retry classification, and no credential ever leaving the server.
//
// Authentication is an API key in the query string rather than an OAuth token.
// A key is the only credential that works without a service-account file on
// disk, which keeps deployment to a single environment variable.
type Google struct {
	// APIKey is the Cloud Text-to-Speech API key. Server-side only.
	APIKey string
	// BaseURL overrides the API origin, for testing and for regional endpoints.
	BaseURL string
	// DefaultLanguage is used when a request names no language. It is a full
	// BCP-47 tag because Google has no notion of a bare "en".
	DefaultLanguage string
	// AudioEncoding defaults to MP3, the container the pipeline inspects.
	AudioEncoding string
	// SpeakingRate and Pitch are Google's own prosody controls.
	//
	// They are deliberately NOT read from SynthesisRequest.Stability and
	// .Similarity. Those are ElevenLabs' knobs and mean something else, and
	// reusing them would make the same job sound different depending on
	// which provider happened to serve it. Zero means "Google's default",
	// which keeps the field out of the request entirely.
	SpeakingRate float64
	Pitch        float64
	Client       *http.Client
}

// NewGoogle builds an adapter with production-sane defaults.
func NewGoogle(apiKey string) *Google {
	return &Google{
		APIKey:          apiKey,
		BaseURL:         "https://texttospeech.googleapis.com",
		DefaultLanguage: "en-US",
		AudioEncoding:   "MP3",
		Client:          newProviderClient(),
	}
}

// Name implements Provider.
func (g *Google) Name() string { return "google" }

type googleRequest struct {
	Input       googleInput       `json:"input"`
	Voice       googleVoice       `json:"voice"`
	AudioConfig googleAudioConfig `json:"audioConfig"`
}

type googleInput struct {
	Text string `json:"text"`
}

type googleVoice struct {
	LanguageCode string `json:"languageCode"`
	Name         string `json:"name,omitempty"`
}

type googleAudioConfig struct {
	AudioEncoding string  `json:"audioEncoding"`
	SpeakingRate  float64 `json:"speakingRate,omitempty"`
	Pitch         float64 `json:"pitch,omitempty"`
}

type googleResponse struct {
	AudioContent string `json:"audioContent"`
}

// Synthesize implements Provider.
func (g *Google) Synthesize(ctx context.Context, req SynthesisRequest) (*SynthesisResult, error) {
	if g.APIKey == "" {
		return nil, PermanentError("google tts api key is not configured")
	}
	if strings.TrimSpace(req.ProviderVoiceID) == "" {
		return nil, PermanentError("no provider voice id supplied")
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, PermanentError("refusing to synthesize empty text")
	}

	encoding := g.AudioEncoding
	if encoding == "" {
		encoding = "MP3"
	}

	body, err := json.Marshal(googleRequest{
		Input: googleInput{Text: req.Text},
		Voice: googleVoice{
			LanguageCode: googleLanguage(req.Language, req.ProviderVoiceID, g.DefaultLanguage),
			Name:         req.ProviderVoiceID,
		},
		AudioConfig: googleAudioConfig{
			AudioEncoding: encoding,
			// Omitted when unset so Google applies its own defaults. A zero
			// here would otherwise be sent as an explicit "rate 0", which is
			// outside the documented range and a 400.
			SpeakingRate: g.SpeakingRate,
			Pitch:        g.Pitch,
		},
	})
	if err != nil {
		return nil, PermanentError("encode request: %v", err)
	}

	url := fmt.Sprintf("%s/v1/text:synthesize?key=%s",
		strings.TrimRight(g.BaseURL, "/"), url.QueryEscape(g.APIKey))

	httpReq, err := providerRequest(ctx, url, bytes.NewReader(body), map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	})
	if err != nil {
		return nil, err
	}

	raw, err := synthesizeHTTP(g.Client, httpReq)
	if err != nil {
		return nil, err
	}

	var parsed googleResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		// The body was inside the read bound but is not the JSON we asked for.
		// Resending the same request gets the same answer.
		return nil, PermanentError("decode response: %v", err)
	}
	if strings.TrimSpace(parsed.AudioContent) == "" {
		return nil, RetryableError("provider returned no audio content")
	}

	audio, err := base64.StdEncoding.DecodeString(parsed.AudioContent)
	if err != nil {
		return nil, PermanentError("decode audio content: %v", err)
	}
	if len(audio) == 0 {
		return nil, RetryableError("provider returned empty audio content")
	}
	if len(audio) > maxAudioBytes {
		return nil, PermanentError("provider returned more than %d bytes", maxAudioBytes)
	}

	codec, contentType := googleCodec(encoding)
	return &SynthesisResult{
		Audio:       audio,
		ContentType: contentType,
		Codec:       codec,
		// Bitrate and sample rate are whatever the encoding implies; the
		// pipeline measures duration from the container rather than trusting
		// a provider's arithmetic, so nothing is estimated here.
		DurationSeconds: 0,
	}, nil
}

// googleLanguage resolves the languageCode Google requires.
//
// Google rejects a bare "en": the field wants a full BCP-47 tag. A voice name
// such as "en-US-Standard-A" already carries one, so it wins. Otherwise a bare
// code is expanded through a table; anything unrecognised is passed through
// unchanged so the provider's own error names the problem.
func googleLanguage(language, voiceID, fallback string) string {
	if lang := strings.TrimSpace(language); strings.Contains(lang, "-") {
		return lang
	}
	if parts := strings.Split(voiceID, "-"); len(parts) >= 2 {
		// "en-US-Standard-A" -> "en-US".
		if len(parts[0]) == 2 && len(parts[1]) >= 2 {
			return parts[0] + "-" + parts[1]
		}
	}
	lang := strings.TrimSpace(language)
	if lang == "" {
		return fallback
	}
	if mapped, ok := regionalLocale[strings.ToLower(lang)]; ok {
		return mapped
	}
	if fallback != "" {
		return fallback
	}
	return lang
}

// googleCodec maps a Google audioEncoding onto the codec and content type the
// asset record stores.
func googleCodec(encoding string) (codec, contentType string) {
	switch strings.ToUpper(encoding) {
	case "MP3":
		return "mp3", "audio/mpeg"
	case "LINEAR16":
		return "pcm_s16le", "audio/wav"
	case "OGG_OPUS":
		return "opus", "audio/ogg"
	case "FLAC":
		return "flac", "audio/flac"
	default:
		// AALAW/MULAW and the rest are telephony codecs the player does not
		// serve; naming mp3 would record a lie in the asset row.
		return "", ""
	}
}

// Compile-time assertion that the adapter satisfies the interface the pipeline
// depends on. A signature drift fails the build instead of the first render.
var _ Provider = (*Google)(nil)
