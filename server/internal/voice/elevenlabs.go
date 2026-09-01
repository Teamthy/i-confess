package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ElevenLabs adapts the ElevenLabs text-to-speech API.
//
// The API key is read from the server environment and never leaves the
// backend (§14). Requests carry a short timeout and a bounded read so a slow
// or hostile response cannot pin a worker or exhaust memory.
type ElevenLabs struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

// NewElevenLabs builds an adapter with production-sane defaults.
func NewElevenLabs(apiKey string) *ElevenLabs {
	return &ElevenLabs{
		APIKey:  apiKey,
		BaseURL: "https://api.elevenlabs.io",
		Model:   "eleven_multilingual_v2",
		Client:  &http.Client{Timeout: 120 * time.Second},
	}
}

// Name implements Provider.
func (e *ElevenLabs) Name() string { return "elevenlabs" }

// maxAudioBytes caps a single render at roughly an hour of 128 kbps audio.
const maxAudioBytes = 64 << 20

type elevenRequest struct {
	Text          string         `json:"text"`
	ModelID       string         `json:"model_id"`
	LanguageCode  string         `json:"language_code,omitempty"`
	VoiceSettings elevenSettings `json:"voice_settings"`
}

type elevenSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
}

// Synthesize implements Provider.
func (e *ElevenLabs) Synthesize(ctx context.Context, req SynthesisRequest) (*SynthesisResult, error) {
	if e.APIKey == "" {
		return nil, PermanentError("elevenlabs api key is not configured")
	}
	if strings.TrimSpace(req.ProviderVoiceID) == "" {
		return nil, PermanentError("no provider voice id supplied")
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, PermanentError("refusing to synthesize empty text")
	}

	format := req.Format
	if format == "" {
		format = "mp3_44100_128"
	}
	stability, similarity := req.Stability, req.Similarity
	if stability == 0 {
		stability = 0.5
	}
	if similarity == 0 {
		similarity = 0.75
	}

	body, err := json.Marshal(elevenRequest{
		Text:         req.Text,
		ModelID:      e.Model,
		LanguageCode: req.Language,
		VoiceSettings: elevenSettings{
			Stability: stability, SimilarityBoost: similarity,
		},
	})
	if err != nil {
		return nil, PermanentError("encode request: %v", err)
	}

	url := fmt.Sprintf("%s/v1/text-to-speech/%s?output_format=%s",
		strings.TrimRight(e.BaseURL, "/"), req.ProviderVoiceID, format)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, PermanentError("build request: %v", err)
	}
	httpReq.Header.Set("xi-api-key", e.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "audio/mpeg")

	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(httpReq)
	if err != nil {
		// Network faults and timeouts are transient by nature.
		return nil, RetryableError("provider request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// Read a bounded snippet for diagnostics; the body may be huge or hostile.
		snippet, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		msg := strings.TrimSpace(string(snippet))
		switch {
		case res.StatusCode == http.StatusTooManyRequests,
			res.StatusCode >= 500:
			return nil, RetryableError("provider returned %d: %s", res.StatusCode, msg)
		case res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden:
			// Credential problems are permanent until an operator intervenes;
			// retrying would burn attempts and mask the real cause.
			return nil, PermanentError("provider rejected credentials (%d): %s", res.StatusCode, msg)
		default:
			return nil, PermanentError("provider returned %d: %s", res.StatusCode, msg)
		}
	}

	audio, err := io.ReadAll(io.LimitReader(res.Body, maxAudioBytes+1))
	if err != nil {
		return nil, RetryableError("read audio: %v", err)
	}
	if len(audio) > maxAudioBytes {
		return nil, PermanentError("provider returned more than %d bytes", maxAudioBytes)
	}
	if len(audio) == 0 {
		return nil, RetryableError("provider returned no audio")
	}

	bitrate, sampleRate := parseFormat(format)
	return &SynthesisResult{
		Audio:       audio,
		ContentType: "audio/mpeg",
		Codec:       "mp3",
		BitrateKbps: bitrate,
		SampleRate:  sampleRate,
		// Derive duration from CBR size; post-processing refines it later.
		DurationSeconds: estimateDuration(len(audio), bitrate),
	}, nil
}

// parseFormat reads "mp3_44100_128" into its sample rate and bitrate.
func parseFormat(format string) (bitrateKbps, sampleRateHz int) {
	parts := strings.Split(format, "_")
	if len(parts) != 3 {
		return 128, 44100
	}
	fmt.Sscanf(parts[1], "%d", &sampleRateHz)
	fmt.Sscanf(parts[2], "%d", &bitrateKbps)
	if sampleRateHz == 0 {
		sampleRateHz = 44100
	}
	if bitrateKbps == 0 {
		bitrateKbps = 128
	}
	return bitrateKbps, sampleRateHz
}

func estimateDuration(sizeBytes, bitrateKbps int) int {
	if bitrateKbps <= 0 {
		return 0
	}
	bytesPerSecond := (bitrateKbps * 1000) / 8
	if bytesPerSecond == 0 {
		return 0
	}
	return sizeBytes / bytesPerSecond
}
