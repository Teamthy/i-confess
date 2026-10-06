// Package voiceengine is the engine-agnostic TTS orchestration layer
// (Voice Platform §7, §22, §29, §38-§40, §69, §71, §85).
//
// iCONFESS never talks to CosyVoice, GPT-SoVITS, VoxCPM or VoiceStudio
// directly. It talks to a VoiceProvider. Concrete engines run out of process
// on GPU workers (voice-engine/ in this repository) behind one small HTTP
// contract, so:
//
//   - no model inference ever runs inside the Go API process;
//   - no engine-specific markup, parameter name or error string leaks into
//     business logic; and
//   - no AGPL or model-licensed code is linked into the proprietary binary -
//     workers are separate programs reached over the network, and whether a
//     given engine may be deployed at all is a licence-manifest decision
//     (MODEL_LICENSES.md), not something this package assumes.
//
// This package does not evaluate rights. Callers (the orchestrator) must run
// voicegov.Authorize before every Generate; Orchestrator enforces that order.
package voiceengine

import (
	"context"
	"errors"
	"fmt"
)

// Engine names a TTS engine family.
type Engine string

const (
	EngineCosyVoice   Engine = "cosyvoice"
	EngineGPTSoVITS   Engine = "gptsovits"
	EngineVoxCPM      Engine = "voxcpm"
	EngineVoiceStudio Engine = "voicestudio"
	// EngineFake is the deterministic in-process test engine. It is refused
	// by NewRegistry unless explicitly allowed, so it cannot reach production.
	EngineFake Engine = "fake"
)

// VoiceProvider is the contract every engine adapter implements (§7).
type VoiceProvider interface {
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error)
	Clone(ctx context.Context, req CloneRequest) (*CloneResult, error)
	Health(ctx context.Context) error
	Capabilities(ctx context.Context) ProviderCapabilities
}

// Reference is a server-side handle to reference audio. URIs are private
// object-store keys resolved by the worker; they are never sent to clients.
type Reference struct {
	ID         string  `json:"id"`
	VoiceID    string  `json:"voice_id"`
	Style      string  `json:"style"`
	URI        string  `json:"uri"`
	Transcript string  `json:"transcript"`
	DurationMS int     `json:"duration_ms"`
	Quality    float64 `json:"quality"`
	// RightsOK is set by the caller from the reference's own rights record;
	// a reference cut from a recording that was not cleared is unusable even
	// when the voice as a whole is licensed.
	RightsOK bool `json:"-"`
}

// GenerateRequest is one render, already rights-checked and resolved to a
// concrete model. Text is delivered as rendered chunks so pauses and
// pronunciations survive engines with no markup support.
type GenerateRequest struct {
	GenerationID string `json:"generation_id"`
	VoiceID      string `json:"voice_id"`
	ModelID      string `json:"model_id"`
	ModelVersion string `json:"model_version"`
	// CheckpointURI is a private object-store key; only the worker resolves it.
	CheckpointURI string         `json:"checkpoint_uri,omitempty"`
	Chunks        []Chunk        `json:"chunks"`
	Language      string         `json:"language"`
	Locale        string         `json:"locale"`
	Style         string         `json:"style"`
	Reference     *Reference     `json:"reference,omitempty"`
	Prosody       ProsodyProfile `json:"prosody"`
	SampleRate    int            `json:"sample_rate"`
	// Params are engine-specific knobs produced by the adapter's MapParams,
	// never by callers.
	Params map[string]any `json:"params,omitempty"`
}

// GenerateResult is raw engine output (pre-mastering).
type GenerateResult struct {
	Audio         []byte
	ContentType   string
	SampleRate    int
	DurationMS    int
	Engine        Engine
	EngineVersion string
	ModelID       string
	// InferenceSeconds and WorkerSeconds are the worker's own measurement of
	// what this render cost (audit VE-017): time inside the model, and time in
	// the whole request. Zero means "not reported", which is recorded as NULL
	// rather than as a free render - a cloud adapter that times nothing must
	// not end up looking cheaper than one that times nothing but said so.
	InferenceSeconds float64
	WorkerSeconds    float64
}

// CloneRequest registers reference audio with an engine for zero-shot use.
type CloneRequest struct {
	VoiceID   string    `json:"voice_id"`
	Language  string    `json:"language"`
	Reference Reference `json:"reference"`
}

// CloneResult is an opaque worker-side speaker handle. Speaker embeddings stay
// on the worker; the API only ever stores this identifier (§64).
type CloneResult struct {
	SpeakerHandle string `json:"speaker_handle"`
}

// ProviderCapabilities describe what an engine can do.
type ProviderCapabilities struct {
	Engine        Engine   `json:"engine"`
	EngineVersion string   `json:"engine_version"`
	ZeroShot      bool     `json:"zero_shot"`
	FineTune      bool     `json:"fine_tune"`
	Streaming     bool     `json:"streaming"`
	Phonemes      bool     `json:"phonemes"`
	Emphasis      bool     `json:"emphasis"`
	Languages     []string `json:"languages"`
	MaxChunkChars int      `json:"max_chunk_chars"`
	// SelfHosted is false when inference leaves infrastructure the platform
	// operates, which requires can_use_third_party_infrastructure.
	SelfHosted bool   `json:"self_hosted"`
	License    string `json:"license"`
}

// SupportsLanguage reports whether the engine declares the language (primary
// subtag match).
func (c ProviderCapabilities) SupportsLanguage(lang string) bool {
	if len(c.Languages) == 0 {
		return false
	}
	p := primary(lang)
	for _, l := range c.Languages {
		if l == "*" || primary(l) == p {
			return true
		}
	}
	return false
}

// ErrorClass classifies failures for retry and dead-letter decisions (§71).
type ErrorClass string

const (
	ClassTransient ErrorClass = "transient"
	ClassPermanent ErrorClass = "permanent"
	ClassRights    ErrorClass = "rights"
	ClassContent   ErrorClass = "content"
	ClassModel     ErrorClass = "model"
	ClassGPU       ErrorClass = "gpu"
	ClassStorage   ErrorClass = "storage"
)

// Retryable reports whether another attempt could succeed. GPU faults (OOM,
// device lost) are retried because a different worker may take the job;
// rights, content and model faults never are.
func (c ErrorClass) Retryable() bool {
	switch c {
	case ClassTransient, ClassGPU, ClassStorage:
		return true
	}
	return false
}

// Error is a classified engine error.
type Error struct {
	Class  ErrorClass
	Engine Engine
	Msg    string
}

func (e *Error) Error() string { return fmt.Sprintf("%s %s error: %s", e.Engine, e.Class, e.Msg) }

// Classify extracts the class of err. Unclassified errors are treated as
// transient so a flaky network is retried - bounded by the queue's attempt
// cap, never indefinitely. Cancellation is permanent: the caller gave up.
func Classify(err error) ErrorClass {
	var e *Error
	if errors.As(err, &e) {
		return e.Class
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ClassTransient
	}
	if errors.Is(err, context.Canceled) {
		return ClassPermanent
	}
	return ClassTransient
}

func primary(tag string) string {
	for i, r := range tag {
		if r == '-' || r == '_' {
			return lower(tag[:i])
		}
	}
	return lower(tag)
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
