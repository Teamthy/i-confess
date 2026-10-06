package voice

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Azure adapts Azure AI Speech (the /cognitiveservices/v1 text-to-speech API).
//
// Two things make this adapter different from the other two, and both are
// security properties rather than convenience:
//
//  1. The request body is SSML, an XML document. Confession text is user
//     content, so it is escaped before interpolation - otherwise a confession
//     containing markup could close the <voice> element and inject elements of
//     its own, including ones that change how the render sounds.
//  2. The response is raw audio rather than JSON, so there is no envelope to
//     validate. The read is still bounded, and an empty body is still a fault.
type Azure struct {
	// SubscriptionKey is the Speech resource key. Server-side only.
	SubscriptionKey string
	// Region is the Speech resource region, e.g. "westeurope". It forms the
	// endpoint host, so a wrong value is a DNS failure rather than a 401.
	Region string
	// OutputFormat is the X-Microsoft-OutputFormat header. The default is the
	// MP3 the pipeline inspects and the player serves.
	OutputFormat string
	// DefaultLanguage is the xml:lang used when a request names no language.
	DefaultLanguage string
	Client          *http.Client
}

// NewAzure builds an adapter with production-sane defaults.
func NewAzure(subscriptionKey, region string) *Azure {
	return &Azure{
		SubscriptionKey: subscriptionKey,
		Region:          region,
		OutputFormat:    "audio-24khz-96kbitrate-mono-mp3",
		DefaultLanguage: "en-US",
		Client:          newProviderClient(),
	}
}

// Name implements Provider.
func (a *Azure) Name() string { return "azure" }

// SelfHosted implements Provider. Azure Speech is a hosted third-party API, so
// synthesis in a minister's cloned voice needs
// can_use_third_party_infrastructure on the grant.
func (a *Azure) SelfHosted() bool { return false }

// Synthesize implements Provider.
func (a *Azure) Synthesize(ctx context.Context, req SynthesisRequest) (*SynthesisResult, error) {
	if a.SubscriptionKey == "" {
		return nil, PermanentError("azure speech key is not configured")
	}
	if strings.TrimSpace(a.Region) == "" {
		return nil, PermanentError("azure speech region is not configured")
	}
	if strings.TrimSpace(req.ProviderVoiceID) == "" {
		return nil, PermanentError("no provider voice id supplied")
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, PermanentError("refusing to synthesize empty text")
	}

	lang := azureLanguage(req.Language, req.ProviderVoiceID, a.DefaultLanguage)
	outputFormat := a.OutputFormat
	if outputFormat == "" {
		outputFormat = "audio-24khz-96kbitrate-mono-mp3"
	}

	ssml := buildSSML(lang, req.ProviderVoiceID, req.Text)

	url := fmt.Sprintf("https://%s.tts.speech.microsoft.com/cognitiveservices/v1",
		strings.ToLower(strings.TrimSpace(a.Region)))

	httpReq, err := providerRequest(ctx, url, bytes.NewReader([]byte(ssml)), map[string]string{
		"Ocp-Apim-Subscription-Key": a.SubscriptionKey,
		"Content-Type":              "application/ssml+xml",
		"X-Microsoft-OutputFormat":  outputFormat,
		"Accept":                    "*/*",
	})
	if err != nil {
		return nil, err
	}

	audio, err := synthesizeHTTP(a.Client, httpReq)
	if err != nil {
		return nil, err
	}

	codec, contentType := azureCodec(outputFormat)
	return &SynthesisResult{
		Audio:       audio,
		ContentType: contentType,
		Codec:       codec,
		// Duration is measured by the pipeline from the returned container.
		// Azure reports no duration of its own, and guessing one from the
		// nominal bitrate would put a wrong number in the asset row.
		DurationSeconds: 0,
	}, nil
}

// buildSSML wraps text in a minimal, well-formed SSML document.
func buildSSML(lang, voice, text string) string {
	var b strings.Builder
	b.WriteString("<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='")
	b.WriteString(escapeSSML(lang))
	b.WriteString("'><voice xml:lang='")
	b.WriteString(escapeSSML(lang))
	b.WriteString("' name='")
	b.WriteString(escapeSSML(voice))
	b.WriteString("'>")
	b.WriteString(escapeSSML(text))
	b.WriteString("</voice></speak>")
	return b.String()
}

// escapeSSML escapes text for inclusion in an XML element or attribute.
//
// Every value interpolated into the document goes through here, including the
// voice name and language: they come from the voice rights record rather than
// from a user, but treating them as trusted is the kind of assumption that
// survives until the day the rights record is editable by someone else.
//
// Control characters are dropped rather than escaped: XML 1.0 forbids most of
// them outright, so an escaped form would still be invalid.
func escapeSSML(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			if r == '\t' || r == '\n' || r == '\r' {
				b.WriteRune(r)
				continue
			}
			if r < 0x20 || r == 0x7F {
				// Forbidden in XML 1.0.
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// azureLanguage resolves the xml:lang tag.
//
// A voice name such as "en-US-AvaNeural" carries its own locale and is
// authoritative; a bare code is expanded through the shared table so the two
// adapters agree on what "sw" means.
func azureLanguage(language, voiceID, fallback string) string {
	if lang := strings.TrimSpace(language); strings.Contains(lang, "-") {
		return lang
	}
	if parts := strings.Split(voiceID, "-"); len(parts) >= 2 {
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

// azureCodec maps an Azure output format onto the codec and content type the
// asset record stores.
func azureCodec(outputFormat string) (codec, contentType string) {
	f := strings.ToLower(outputFormat)
	switch {
	case strings.Contains(f, "-mp3"), strings.Contains(f, "mpeg"):
		return "mp3", "audio/mpeg"
	case strings.Contains(f, "opus"):
		return "opus", "audio/ogg"
	case strings.Contains(f, "flac"):
		return "flac", "audio/flac"
	case strings.Contains(f, "pcm"), strings.Contains(f, "wav"):
		return "pcm_s16le", "audio/wav"
	default:
		return "", ""
	}
}

// Compile-time assertion that the adapter satisfies the interface the pipeline
// depends on.
var _ Provider = (*Azure)(nil)
