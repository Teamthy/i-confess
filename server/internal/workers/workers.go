// Package workers holds the job handlers - the work the queue performs.
//
// It is separate from internal/jobs on purpose. internal/jobs is the mechanism:
// a durable queue, a worker pool, retries, backoff, dead letters. This package is
// the policy: what an "audio.generate" job actually does. Keeping them apart
// means the queue has no opinion about voices or notifications, and the handlers
// can depend on the store, the voice pipeline and the push transport without
// pulling any of that into the queue.
package workers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/push"
)

// Job type names. These are the values written to jobs.type.
const (
	// TypeAudioGenerate renders a confession variant in a voice.
	TypeAudioGenerate = "audio.generate"
	// TypeNotificationSend delivers one notification to one device.
	TypeNotificationSend = "notification.send"
	// TypeAudioProcess validates and measures an uploaded asset.
	TypeAudioProcess = "audio.process"
)

// AllTypes is every job type this package can perform, for the admin queue view.
func AllTypes() []string {
	return []string{TypeAudioGenerate, TypeNotificationSend, TypeAudioProcess}
}

// Generator renders audio for a confession variant.
type Generator interface {
	Generate(ctx context.Context, req GenerationRequest) error
}

// GenerationRequest is everything needed to render one variant.
type GenerationRequest struct {
	ConfessionID string
	VariantID    string
	VoiceID      string
	Language     string
	Actor        string
}

// Processor validates and measures an uploaded audio asset.
type Processor interface {
	Process(ctx context.Context, assetID string) error
}

// Services are the collaborators the handlers need.
//
// A collaborator left nil does not make its job type quietly succeed. That was
// the failure mode of the handlers this package replaces: they logged a line,
// returned nil, and the queue recorded a completed job for work that never
// happened. An unconfigured handler now parks the job with the reason attached.
type Services struct {
	Generate Generator
	Notify   push.Sender
	Process  Processor
}

// Register installs the handlers whose collaborator is configured, and returns
// the job types it installed.
//
// A type with no collaborator is deliberately left unregistered rather than
// wired to a function that can only fail. The queue then parks any such job with
// "no handler registered", which says what is actually wrong - this server is
// not configured to do that work - instead of a job that appears to be handled
// and is not.
func Register(q jobs.Queue, s Services) []string {
	var installed []string
	if s.Generate != nil {
		q.Register(TypeAudioGenerate, s.handleGenerate)
		installed = append(installed, TypeAudioGenerate)
	}
	if s.Notify != nil {
		q.Register(TypeNotificationSend, s.handleNotify)
		installed = append(installed, TypeNotificationSend)
	}
	if s.Process != nil {
		q.Register(TypeAudioProcess, s.handleProcess)
		installed = append(installed, TypeAudioProcess)
	}
	return installed
}

func (s Services) handleGenerate(ctx context.Context, p map[string]any) error {
	if s.Generate == nil {
		return jobs.Permanent(errors.New("no audio generator is configured on this server"))
	}
	confessionID, err := requireString(p, "confession_id")
	if err != nil {
		return jobs.Permanent(err)
	}
	voiceID, err := requireString(p, "voice_id")
	if err != nil {
		return jobs.Permanent(err)
	}
	variantID, _ := p["variant_id"].(string)
	language, _ := p["language"].(string)
	if language == "" {
		language = "en"
	}
	actor, _ := p["actor"].(string)

	return s.Generate.Generate(ctx, GenerationRequest{
		ConfessionID: confessionID,
		VariantID:    variantID,
		VoiceID:      voiceID,
		Language:     language,
		Actor:        actor,
	})
}

func (s Services) handleNotify(ctx context.Context, p map[string]any) error {
	if s.Notify == nil {
		return jobs.Permanent(errors.New("no push sender is configured on this server"))
	}
	token, err := requireString(p, "token")
	if err != nil {
		return jobs.Permanent(err)
	}
	title, err := requireString(p, "title")
	if err != nil {
		return jobs.Permanent(err)
	}
	body, _ := p["body"].(string)
	platform, _ := p["platform"].(string)

	n := push.Notification{
		Token:    token,
		Platform: push.Platform(platform),
		Title:    title,
		Body:     body,
	}
	if data, ok := p["data"].(map[string]any); ok {
		n.Data = make(map[string]string, len(data))
		for k, v := range data {
			if str, ok := v.(string); ok {
				n.Data[k] = str
			}
		}
	}

	err = s.Notify.Send(ctx, n)
	if err == nil {
		return nil
	}
	// A transport fault is worth another attempt; a rejected token is not. The
	// distinction is what keeps a permanently undeliverable notification from
	// occupying a retry slot for half an hour before it parks.
	if errors.Is(err, push.ErrRetryable) {
		return err
	}
	return jobs.Permanent(err)
}

func (s Services) handleProcess(ctx context.Context, p map[string]any) error {
	if s.Process == nil {
		return jobs.Permanent(errors.New("no audio processor is configured on this server"))
	}
	assetID, err := requireString(p, "audio_asset_id")
	if err != nil {
		return jobs.Permanent(err)
	}
	return s.Process.Process(ctx, assetID)
}

// requireString reads a mandatory string field. A missing field is permanent:
// no amount of retrying supplies it.
func requireString(p map[string]any, key string) (string, error) {
	v, ok := p[key].(string)
	if !ok || strings.TrimSpace(v) == "" {
		return "", fmt.Errorf("payload is missing %q", key)
	}
	return v, nil
}
