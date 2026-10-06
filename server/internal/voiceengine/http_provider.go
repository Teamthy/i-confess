package voiceengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Worker HTTP contract (implemented by voice-engine/icf_worker/server.py):
//
//	GET  /v1/health        -> 200 {"status":"ok"} | 503
//	GET  /v1/capabilities  -> ProviderCapabilities JSON
//	POST /v1/synthesize    <- GenerateRequest JSON
//	                       -> 200 audio/wav body, headers X-Sample-Rate,
//	                          X-Duration-Ms, X-Engine-Version, and the measured
//	                          cost headers X-Inference-Seconds /
//	                          X-Worker-Seconds (VE-017; absent means "not
//	                          reported", never "cost nothing")
//	                       -> 4xx/5xx {"error":{"class":"gpu","message":"..."}}
//	POST /v1/clone         <- CloneRequest JSON -> CloneResult JSON
//
// Requests carry a bearer token shared between API and workers. Workers must
// not be reachable from the public internet.

// ParamMapper converts the engine-neutral prosody profile into the engine's
// own parameter names. This is the only place engine-specific tuning lives.
type ParamMapper func(p ProsodyProfile, style string) map[string]any

// HTTPProvider is a VoiceProvider backed by a GPU worker.
type HTTPProvider struct {
	engine  Engine
	baseURL string
	token   string
	client  *http.Client
	// streamClient has no overall timeout; the request context bounds it.
	streamClient *http.Client
	mapper       ParamMapper
	// fallbackCaps are used when the worker cannot be reached, so a down
	// worker is reported as unhealthy rather than as "supports nothing".
	fallbackCaps ProviderCapabilities

	mu        sync.Mutex
	caps      *ProviderCapabilities
	capsUntil time.Time
}

// HTTPProviderConfig configures an HTTPProvider.
type HTTPProviderConfig struct {
	Engine  Engine
	BaseURL string
	Token   string
	Timeout time.Duration
	Mapper  ParamMapper
	Caps    ProviderCapabilities
}

// NewHTTPProvider builds an adapter for one worker pool.
func NewHTTPProvider(c HTTPProviderConfig) *HTTPProvider {
	if c.Timeout == 0 {
		c.Timeout = 5 * time.Minute
	}
	if c.Mapper == nil {
		c.Mapper = func(ProsodyProfile, string) map[string]any { return nil }
	}
	c.Caps.Engine = c.Engine
	return &HTTPProvider{
		engine: c.Engine, baseURL: strings.TrimRight(c.BaseURL, "/"), token: c.Token,
		client: &http.Client{Timeout: c.Timeout}, streamClient: &http.Client{}, mapper: c.Mapper, fallbackCaps: c.Caps,
	}
}

// Generate implements VoiceProvider.
func (p *HTTPProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	req.Params = p.mapper(req.Prosody, req.Style)
	resp, err := p.do(ctx, http.MethodPost, "/v1/synthesize", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, p.decodeError(resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
	if err != nil {
		return nil, &Error{Class: ClassTransient, Engine: p.engine, Msg: "read audio: " + err.Error()}
	}
	if len(body) == 0 {
		return nil, &Error{Class: ClassModel, Engine: p.engine, Msg: "worker returned empty audio"}
	}
	sr, _ := strconv.Atoi(resp.Header.Get("X-Sample-Rate"))
	dur, _ := strconv.Atoi(resp.Header.Get("X-Duration-Ms"))
	return &GenerateResult{
		Audio: body, ContentType: resp.Header.Get("Content-Type"), SampleRate: sr, DurationMS: dur,
		Engine: p.engine, EngineVersion: resp.Header.Get("X-Engine-Version"), ModelID: req.ModelID,
		InferenceSeconds: headerSeconds(resp.Header, "X-Inference-Seconds"),
		WorkerSeconds:    headerSeconds(resp.Header, "X-Worker-Seconds"),
	}, nil
}

// headerSeconds reads a measured duration header. Anything unreadable, empty
// or negative becomes 0, which the caller records as NULL: an unreported cost
// must never be mistaken for a render that cost nothing.
func headerSeconds(h http.Header, key string) float64 {
	v := strings.TrimSpace(h.Get(key))
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0
	}
	return f
}

// Clone implements VoiceProvider.
func (p *HTTPProvider) Clone(ctx context.Context, req CloneRequest) (*CloneResult, error) {
	resp, err := p.do(ctx, http.MethodPost, "/v1/clone", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, p.decodeError(resp)
	}
	var out CloneResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.SpeakerHandle == "" {
		return nil, &Error{Class: ClassModel, Engine: p.engine, Msg: "worker returned no speaker handle"}
	}
	return &out, nil
}

// Health implements VoiceProvider.
func (p *HTTPProvider) Health(ctx context.Context) error {
	resp, err := p.do(ctx, http.MethodGet, "/v1/health", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return p.decodeError(resp)
	}
	return nil
}

// Capabilities implements VoiceProvider. Results are cached for a minute.
func (p *HTTPProvider) Capabilities(ctx context.Context) ProviderCapabilities {
	p.mu.Lock()
	if p.caps != nil && time.Now().Before(p.capsUntil) {
		c := *p.caps
		p.mu.Unlock()
		return c
	}
	p.mu.Unlock()

	resp, err := p.do(ctx, http.MethodGet, "/v1/capabilities", nil)
	if err != nil {
		return p.fallbackCaps
	}
	defer resp.Body.Close()
	var c ProviderCapabilities
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&c) != nil {
		return p.fallbackCaps
	}
	// The worker cannot rename itself into a different engine.
	c.Engine = p.engine
	p.mu.Lock()
	p.caps, p.capsUntil = &c, time.Now().Add(time.Minute)
	p.mu.Unlock()
	return c
}

func (p *HTTPProvider) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	return p.doWith(ctx, p.client, method, path, body)
}

func (p *HTTPProvider) doWith(ctx context.Context, client *http.Client, method, path string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, &Error{Class: ClassPermanent, Engine: p.engine, Msg: err.Error()}
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, rdr)
	if err != nil {
		return nil, &Error{Class: ClassPermanent, Engine: p.engine, Msg: err.Error()}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return nil, &Error{Class: ClassPermanent, Engine: p.engine, Msg: "cancelled"}
		}
		return nil, &Error{Class: ClassTransient, Engine: p.engine, Msg: err.Error()}
	}
	return resp, nil
}

func (p *HTTPProvider) decodeError(resp *http.Response) error {
	var env struct {
		Error struct {
			Class   ErrorClass `json:"class"`
			Message string     `json:"message"`
		} `json:"error"`
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	_ = json.Unmarshal(b, &env)
	class := env.Error.Class
	switch class {
	case ClassTransient, ClassPermanent, ClassRights, ClassContent, ClassModel, ClassGPU, ClassStorage:
	default:
		switch {
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			class = ClassTransient
		default:
			class = ClassPermanent
		}
	}
	msg := env.Error.Message
	if msg == "" {
		msg = fmt.Sprintf("worker returned HTTP %d", resp.StatusCode)
	}
	return &Error{Class: class, Engine: p.engine, Msg: msg}
}

// ---------------------------------------------------------------------------
// Engine adapters. Each is an HTTPProvider with its own parameter mapping and
// declared capabilities. Capabilities here are conservative defaults; the
// worker's /v1/capabilities response is authoritative once reachable.
// Licence strings are deliberately "see MODEL_LICENSES.md" - they are verified
// per checkpoint there, not asserted in code.
// ---------------------------------------------------------------------------

// NewCosyVoiceProvider returns the CosyVoice adapter.
//
// `instruct` is the natural-language style instruction that CosyVoice's
// inference_instruct2 consumes; `instruct_style` stays as the provenance name of
// the style that produced it, so a render can be explained after the fact even
// if the instruction texts change. A worker whose CosyVoice build has no
// instruct2 renders zero-shot from the same parameters and reports
// style_instruction=false in /v1/capabilities, so the difference is visible
// rather than silent (VE-006).
func NewCosyVoiceProvider(baseURL, token string) *HTTPProvider {
	return NewHTTPProvider(HTTPProviderConfig{
		Engine: EngineCosyVoice, BaseURL: baseURL, Token: token,
		Caps: ProviderCapabilities{EngineVersion: "3", ZeroShot: true, FineTune: true, Streaming: true,
			Languages: []string{"en", "zh"}, MaxChunkChars: 300, SelfHosted: true, License: "see MODEL_LICENSES.md"},
		Mapper: func(p ProsodyProfile, style string) map[string]any {
			params := map[string]any{"speed": p.Speed, "instruct_style": style}
			if instruction := InstructionFor(style, p); instruction != "" {
				params["instruct"] = instruction
			}
			return params
		},
	})
}

// NewGPTSoVITSProvider returns the GPT-SoVITS adapter. GPT-SoVITS runs as its
// own API server; the worker proxies to it.
func NewGPTSoVITSProvider(baseURL, token string) *HTTPProvider {
	return NewHTTPProvider(HTTPProviderConfig{
		Engine: EngineGPTSoVITS, BaseURL: baseURL, Token: token,
		Caps: ProviderCapabilities{EngineVersion: "v2", ZeroShot: true, FineTune: true,
			Languages: []string{"en", "zh", "ja", "ko"}, MaxChunkChars: 200, SelfHosted: true, License: "see MODEL_LICENSES.md"},
		Mapper: func(p ProsodyProfile, _ string) map[string]any {
			// GPT-SoVITS exposes sampling temperature rather than an energy
			// knob; pitch variation maps onto it loosely and is clamped.
			temp := 0.6 + 0.6*clamp01(p.PitchVariation)
			return map[string]any{"speed_factor": p.Speed, "temperature": round2(temp)}
		},
	})
}

// NewVoxCPMProvider returns the VoxCPM adapter.
func NewVoxCPMProvider(baseURL, token string) *HTTPProvider {
	return NewHTTPProvider(HTTPProviderConfig{
		Engine: EngineVoxCPM, BaseURL: baseURL, Token: token,
		Caps: ProviderCapabilities{EngineVersion: "2", ZeroShot: true,
			Languages: []string{"en", "zh"}, MaxChunkChars: 300, SelfHosted: true, License: "see MODEL_LICENSES.md"},
		Mapper: func(p ProsodyProfile, _ string) map[string]any {
			return map[string]any{"cfg_value": round2(1.5 + clamp01(p.Energy)), "speed": p.Speed}
		},
	})
}

// NewVoiceStudioProvider targets a local VoiceStudio instance. It is intended
// for development and benchmarking only (§9); the registry refuses it in
// production mode because VoiceStudio is AGPL-3.0 and its network-service
// obligations have not been cleared (see VOICESTUDIO_LICENSE.md).
func NewVoiceStudioProvider(baseURL, token string) *HTTPProvider {
	return NewHTTPProvider(HTTPProviderConfig{
		Engine: EngineVoiceStudio, BaseURL: baseURL, Token: token,
		Caps: ProviderCapabilities{ZeroShot: true, Languages: []string{"*"}, MaxChunkChars: 300,
			SelfHosted: true, License: "AGPL-3.0 (application); models separately licensed"},
	})
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }
