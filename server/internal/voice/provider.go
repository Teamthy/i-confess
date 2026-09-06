// Package voice defines the speech-synthesis provider interface and its
// adapters (§14).
//
// Two rules shape this package:
//
//  1. Provider credentials are server-side only. No key, token or provider URL
//     is ever handed to a mobile or web client.
//  2. No adapter is reachable except through Pipeline.Generate, which evaluates
//     voice rights before it will dispatch. The gate lives above the adapter so
//     a new provider cannot accidentally bypass it.
package voice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Teamthy/i-confess/internal/backoff"
)

// Provider synthesizes speech. Implementations must be safe for concurrent use.
type Provider interface {
	// Name identifies the provider in logs, metrics and audit records.
	Name() string
	// Synthesize renders text with the given provider-side voice id.
	Synthesize(ctx context.Context, req SynthesisRequest) (*SynthesisResult, error)
}

// SynthesisRequest is a single render.
type SynthesisRequest struct {
	// ProviderVoiceID is the provider's own identifier, taken from the voice's
	// rights record — never from client input.
	ProviderVoiceID string
	Text            string
	Language        string
	// Format is the desired container/codec, e.g. "mp3_44100_128".
	Format string
	// Stability and Similarity are provider tuning knobs, surfaced so the
	// audio team can tune delivery without code changes.
	Stability  float64
	Similarity float64
}

// SynthesisResult is rendered audio plus the metadata the asset record needs.
type SynthesisResult struct {
	Audio       []byte
	ContentType string
	Codec       string
	BitrateKbps int
	SampleRate  int
	// DurationSeconds may be zero if the provider does not report it; the
	// pipeline then derives it during post-processing.
	DurationSeconds int
}

// Errors an adapter may return. The pipeline uses these to decide whether a
// job is worth retrying.
var (
	// ErrRetryable signals a transient fault: timeouts, 429s, 5xx.
	ErrRetryable = errors.New("retryable provider error")
	// ErrPermanent signals a fault that will recur: bad voice id, rejected
	// text, quota exhausted. Retrying only wastes money.
	ErrPermanent = errors.New("permanent provider error")
)

// RetryableError wraps a transient fault.
func RetryableError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrRetryable, fmt.Sprintf(format, args...))
}

// PermanentError wraps a fault that must not be retried.
func PermanentError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrPermanent, fmt.Sprintf(format, args...))
}

// IsRetryable reports whether the pipeline should schedule another attempt.
func IsRetryable(err error) bool {
	return errors.Is(err, ErrRetryable)
}

// Backoff returns the delay before attempt n (1-based), with exponential growth
// capped so a struggling provider is retried patiently rather than hammered.
// The schedule itself lives in internal/backoff, shared with the job queue and
// the mailer.
var generationBackoff = backoff.New(30*time.Second, 30*time.Minute)

func Backoff(attempt int) time.Duration { return generationBackoff.Next(attempt) }
