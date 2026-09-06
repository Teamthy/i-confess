package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/Teamthy/i-confess/internal/audio"
	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/rights"
	"github.com/Teamthy/i-confess/internal/voice"
	"github.com/Teamthy/i-confess/internal/workers"
)

// Generate performs one render on behalf of the job queue. It satisfies
// workers.Generator.
//
// It walks the same steps the synchronous admin endpoint does - editorial gate,
// text snapshot, job record, rights-gated synthesis, asset into QA - because the
// queue path must not be a cheaper version of the real thing. The difference is
// what happens on failure: instead of an HTTP status, a failure returns an error
// the queue turns into a retry with backoff, or into a parked job when retrying
// cannot help.
//
// The classification matters more than it looks. Marking everything retryable
// means a confession that was never approved is re-attempted for half an hour
// before anyone notices. Marking everything permanent means a provider that was
// briefly down never gets a second chance.
func (h *Handler) Generate(ctx context.Context, req workers.GenerationRequest) error {
	if h.pipeline == nil {
		return jobs.Permanent(errors.New("voice synthesis is not configured on this server"))
	}

	conf, err := h.cont.ConfessionByID(ctx, req.ConfessionID)
	if err != nil {
		return jobs.Permanent(fmt.Errorf("confession %s: %w", req.ConfessionID, err))
	}

	// Editorial gate (§62): only approved content may be voiced. Retrying does
	// not approve it.
	switch conf.Status {
	case "approved", "published", "ready":
	default:
		return jobs.Permanent(fmt.Errorf(
			"confession is %q; an editor must approve it before audio can be generated", conf.Status))
	}

	text := textForVariant(conf, req.VariantID)
	if strings.TrimSpace(text) == "" {
		return jobs.Permanent(errors.New("confession has no text for this variant"))
	}

	lic, _, err := h.rightsFor(ctx, req.VoiceID)
	if err != nil {
		return jobs.Permanent(fmt.Errorf("voice %s: %w", req.VoiceID, err))
	}

	// Snapshot the exact text before synthesizing, so a QA reviewer approves the
	// render against the words that were spoken.
	version, err := h.cont.EnsureVersion(ctx, conf.ID, conf.Title,
		conf.ShortText, conf.MediumText, conf.LongText, conf.Language, req.Actor)
	if err != nil {
		return fmt.Errorf("snapshot confession text: %w", err)
	}

	job, created, err := h.audio.CreateJob(ctx, &models.AudioJob{
		ConfessionID: req.ConfessionID, VariantID: req.VariantID, VoiceID: req.VoiceID,
		ContentVersionID: version.ID, RequestedBy: req.Actor,
		Provider:       providerName(h.pipeline),
		Format:         "m4a",
		IdempotencyKey: generationKey(req.ConfessionID, req.VariantID, req.VoiceID, req.Language, version.ID, false),
	})
	if err != nil {
		return fmt.Errorf("record generation request: %w", err)
	}
	if !created {
		switch audio.JobStatus(job.Status) {
		case audio.JobSucceeded:
			// Already rendered. Nothing to do, and nothing to bill.
			return nil
		case audio.JobProcessing, audio.JobQueued:
			return nil
		case audio.JobFailed:
			if job.AttemptCount >= job.MaxAttempts {
				return jobs.Permanent(fmt.Errorf(
					"this render already failed %d of %d attempts", job.AttemptCount, job.MaxAttempts))
			}
			// The lifecycle does not permit failed -> processing directly, so a
			// retry is requeued first and stays visible in the job's history.
			if job, err = h.audio.RequeueJob(ctx, job.ID); err != nil {
				return fmt.Errorf("requeue generation job: %w", err)
			}
		}
	}

	if _, err := h.audio.StartJob(ctx, job.ID); err != nil {
		return fmt.Errorf("start generation job: %w", err)
	}

	res, err := h.pipeline.Generate(ctx, lic, voice.GenerateRequest{
		ConfessionID: req.ConfessionID, VariantID: req.VariantID, VoiceID: req.VoiceID,
		Language: req.Language, Text: text, Use: rights.UseSynthesis,
		RequestedBy: req.Actor,
	})

	var denied *voice.ErrRightsDenied
	if errors.As(err, &denied) {
		code := string(denied.Decision.Reason)
		if _, ferr := h.audio.FailJob(ctx, job.ID, code, denied.Decision.Detail); ferr != nil {
			log.Printf("audio: could not record rights refusal on job %s: %v", job.ID, ferr)
		}
		// A rights refusal is a legal answer, not a transient fault. Retrying
		// would re-ask a question whose answer will not change.
		return jobs.Permanent(fmt.Errorf("voice rights do not permit this generation: %s", code))
	}
	if err != nil {
		code, permanent := "provider_error", false
		if !voice.IsRetryable(err) {
			code, permanent = "provider_rejected", true
		}
		if _, ferr := h.audio.FailJob(ctx, job.ID, code, err.Error()); ferr != nil {
			log.Printf("audio: could not record failure on job %s: %v", job.ID, ferr)
		}
		if permanent {
			return jobs.Permanent(err)
		}
		return err
	}

	asset := &models.AudioAsset{
		ConfessionID: req.ConfessionID, VariantID: req.VariantID, VoiceID: req.VoiceID,
		URL: res.Key, SizeBytes: res.SizeBytes, DurationSeconds: res.DurationSeconds,
		ContentVersionID: version.ID,
		// New audio enters QA rather than going straight to listeners (§31).
		Status: string(audio.StatusProcessing),
	}
	if err := h.audio.UpsertAsset(ctx, asset); err != nil {
		// The render exists and was paid for. Recording the reason is what keeps
		// a provider invoice reconcilable against the catalogue.
		log.Printf("audio: job %s rendered %s but the asset could not be recorded: %v", job.ID, res.Key, err)
		if _, ferr := h.audio.FailJob(ctx, job.ID, "asset_record_failed", err.Error()); ferr != nil {
			log.Printf("audio: could not record the asset failure on job %s: %v", job.ID, ferr)
		}
		// Worth another attempt: the audio is already paid for, and a transient
		// database fault should not turn it into a second bill.
		return fmt.Errorf("record generated asset: %w", err)
	}

	if _, err := h.audio.CompleteJob(ctx, job.ID, asset.ID); err != nil {
		log.Printf("audio: job %s produced an asset but could not be completed: %v", job.ID, err)
	}
	return nil
}
