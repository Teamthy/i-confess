package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/audio"
	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/rights"
	"github.com/Teamthy/i-confess/internal/voice"
	"github.com/Teamthy/i-confess/internal/workers"
)

// Voice-rights and audio-generation admin endpoints (§13, §14, §30, §61).
//
// These are the highest-consequence writes in the product: a mistake here can
// mean synthesizing a person's voice without permission. They are restricted to
// VOICE_MANAGER, validated strictly, and audited.

// SetPipeline installs the voice-generation pipeline.
func (h *Handler) SetPipeline(p *voice.Pipeline) { h.pipeline = p }

// rightsFor loads a voice's stored rights and normalises them for evaluation.
// A missing record yields nil, which denies every use.
func (h *Handler) rightsFor(ctx context.Context, voiceID string) (*rights.License, *models.VoiceRights, error) {
	vr, err := h.vrights.ByVoiceID(ctx, voiceID)
	if err != nil {
		// Absent record is a meaningful state, not a failure.
		return nil, nil, nil
	}
	return rights.FromSource(&rights.Source{
		VoiceID:                vr.VoiceID,
		RightsHolder:           vr.RightsHolder,
		Provider:               vr.Provider,
		AuthorizationReference: vr.AuthorizationReference,
		AllowedUse:             vr.AllowedUse,
		Territories:            vr.Territories,
		Status:                 vr.Status,
		LicenseStatus:          vr.LicenseStatus,
		StartDate:              vr.StartDate,
		ExpiryDate:             vr.ExpiryDate,
		ExpirationDate:         vr.ExpirationDate,
		CommercialUse:          vr.CommercialUse,
		AIGenerationAllowed:    vr.AIGenerationAllowed,
		MarketingAllowed:       vr.MarketingAllowed,
		ProviderVoiceID:        vr.ProviderVoiceID,
		RevocationTerms:        vr.RevocationTerms,
	}), vr, nil
}

// adminGetVoiceRights returns a voice's rights with a live evaluation attached,
// so an admin sees what the licence actually permits today rather than having
// to interpret raw fields.
func (h *Handler) adminGetVoiceRights(w http.ResponseWriter, r *http.Request) {
	voiceID := r.PathValue("id")

	lic, stored, _ := h.rightsFor(r.Context(), voiceID)
	now := time.Now().UTC()

	if lic == nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"voice_id": voiceID,
			"rights":   nil,
			"evaluation": map[string]any{
				"playback":     evaluate(nil, rights.UsePlayback, now),
				"synthesis":    evaluate(nil, rights.UseSynthesis, now),
				"marketing":    evaluate(nil, rights.UseMarketing, now),
				"user_content": evaluate(nil, rights.UseUserContent, now),
			},
			"warning": "No rights record exists. This voice cannot be used for any purpose.",
		})
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"voice_id": voiceID,
		"rights":   stored,
		"evaluation": map[string]any{
			"playback":     evaluate(lic, rights.UsePlayback, now),
			"synthesis":    evaluate(lic, rights.UseSynthesis, now),
			"marketing":    evaluate(lic, rights.UseMarketing, now),
			"user_content": evaluate(lic, rights.UseUserContent, now),
		},
		"expiring_soon": lic.ExpiringWithin(30*24*time.Hour, now),
	})
}

func evaluate(lic *rights.License, use rights.Use, at time.Time) map[string]any {
	d := rights.Evaluate(lic, rights.Request{Use: use, At: at})
	return map[string]any{"allowed": d.Allowed, "reason": string(d.Reason), "detail": d.Detail}
}

// adminUpsertVoiceRights records or updates a voice's rights.
//
// Every grant must be sent explicitly. The request uses plain bools rather than
// pointers so a partial payload can never silently preserve a permission the
// admin did not restate.
func (h *Handler) adminUpsertVoiceRights(w http.ResponseWriter, r *http.Request) {
	voiceID := r.PathValue("id")

	if _, err := h.audio.VoiceByID(r.Context(), voiceID); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "voice not found")
		return
	}

	var req struct {
		RightsHolder           string `json:"rights_holder"`
		AuthorizationReference string `json:"authorization_reference"`
		Provider               string `json:"provider"`
		AllowedUse             string `json:"allowed_use"`
		Territories            string `json:"territories"`
		Status                 string `json:"status"`
		StartDate              string `json:"start_date"`
		ExpiryDate             string `json:"expiry_date"`
		CommercialUse          bool   `json:"commercial_use"`
		AIGenerationAllowed    bool   `json:"ai_generation_allowed"`
		MarketingAllowed       bool   `json:"marketing_allowed"`
		ProviderVoiceID        string `json:"provider_voice_id"`
		RevocationTerms        string `json:"revocation_terms"`
		// RightsAttestation is a deliberate speed bump: the caller must state
		// the signed agreement backing an AI-generation grant. It is recorded,
		// not merely checked.
		RightsAttestation string `json:"rights_attestation"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !validRightsStatus(req.Status) {
		httpx.WriteError(w, http.StatusBadRequest,
			"status must be one of: pending, active, expired, revoked")
		return
	}
	if err := validateTime(req.StartDate); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "start_date must be RFC3339 or YYYY-MM-DD")
		return
	}
	if err := validateTime(req.ExpiryDate); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "expiry_date must be RFC3339 or YYYY-MM-DD")
		return
	}

	// Contradictory states are refused rather than stored and discovered later
	// at dispatch time.
	if req.AIGenerationAllowed && strings.TrimSpace(req.ProviderVoiceID) == "" {
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"provider_voice_id is required when ai_generation_allowed is true")
		return
	}
	// Granting synthesis demands an explicit attestation: holding recordings is
	// not consent (§13), so this makes authorising it a deliberate act.
	if req.AIGenerationAllowed && strings.TrimSpace(req.RightsAttestation) == "" {
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"rights_attestation is required to grant AI voice generation: state the signed agreement authorising synthesis")
		return
	}

	if req.AllowedUse == "" {
		req.AllowedUse = "recording"
	}
	if req.Territories == "" {
		req.Territories = "GLOBAL"
	}

	vr := &models.VoiceRights{
		VoiceID: voiceID, RightsHolder: req.RightsHolder,
		AuthorizationReference: req.AuthorizationReference,
		AllowedUse:             req.AllowedUse, Territories: req.Territories,
		Status: req.Status, LicenseStatus: req.Status,
		StartDate: req.StartDate, ExpiryDate: req.ExpiryDate,
		CommercialUse:       req.CommercialUse,
		AIGenerationAllowed: req.AIGenerationAllowed,
		MarketingAllowed:    req.MarketingAllowed,
		Provider:            req.Provider,
		ProviderVoiceID:     req.ProviderVoiceID,
		RevocationTerms:     req.RevocationTerms,
	}

	// Upsert: update when a record already exists, otherwise create.
	if existing, err := h.vrights.ByVoiceID(r.Context(), voiceID); err == nil && existing != nil {
		vr.ID = existing.ID
		if err := h.vrights.Update(r.Context(), vr); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "failed to update voice rights")
			return
		}
	} else if err := h.vrights.Create(r.Context(), vr); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save voice rights")
		return
	}

	h.auditRights(r, voiceID, req.AIGenerationAllowed, req.RightsAttestation)
	httpx.WriteJSON(w, http.StatusOK, vr)
}

// adminRevokeVoiceRights withdraws every permission for a voice immediately.
//
// Revocation is its own endpoint rather than an update because it must be fast
// and unambiguous: a minister withdrawing consent is a moment where clarity
// matters more than flexibility.
func (h *Handler) adminRevokeVoiceRights(w http.ResponseWriter, r *http.Request) {
	voiceID := r.PathValue("id")

	existing, err := h.vrights.ByVoiceID(r.Context(), voiceID)
	if err != nil || existing == nil {
		httpx.WriteError(w, http.StatusNotFound, "no rights record exists for this voice")
		return
	}

	existing.Status = "revoked"
	existing.LicenseStatus = "revoked"
	existing.CommercialUse = false
	existing.AIGenerationAllowed = false
	existing.MarketingAllowed = false

	if err := h.vrights.Update(r.Context(), existing); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to revoke voice rights")
		return
	}

	h.auditRights(r, voiceID, false, "REVOKED")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"voice_id": voiceID,
		"status":   "revoked",
		"note":     "All uses are now denied. Published audio is unaffected and must be withdrawn separately if required.",
	})
}

// adminGenerateAudio renders a confession in a voice, subject to rights.
//
// The handler does not evaluate rights itself: it loads the licence and hands
// it to the pipeline, which is the single chokepoint where the gate lives.
func (h *Handler) adminGenerateAudio(w http.ResponseWriter, r *http.Request) {
	if h.pipeline == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable,
			"voice synthesis is not configured on this server")
		return
	}

	var req struct {
		ConfessionID string `json:"confession_id"`
		VariantID    string `json:"variant_id"`
		VoiceID      string `json:"voice_id"`
		Language     string `json:"language"`
		Territory    string `json:"territory"`
		Force        bool   `json:"force"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ConfessionID == "" || req.VoiceID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "confession_id and voice_id are required")
		return
	}
	if req.Language == "" {
		req.Language = "en"
	}

	conf, err := h.cont.ConfessionByID(r.Context(), req.ConfessionID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "confession not found")
		return
	}

	// Editorial gate (§62): only approved content may be voiced. Generating
	// audio for a draft would let unreviewed theology reach listeners.
	switch conf.Status {
	case "approved", "published", "ready":
	default:
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"confession must be approved by an editor before audio can be generated")
		return
	}

	text := textForVariant(conf, req.VariantID)
	if strings.TrimSpace(text) == "" {
		httpx.WriteError(w, http.StatusUnprocessableEntity, "confession has no text for this variant")
		return
	}

	lic, _, _ := h.rightsFor(r.Context(), req.VoiceID)

	actor := ""
	if c := auth.FromContext(r); c != nil {
		actor = c.Sub
	}

	// Snapshot the exact text before synthesizing. This is what makes the QA
	// review meaningful: a reviewer approves the render against the words that
	// were spoken, not against whatever the confession says by the time they
	// look. It is also what audio_generation_jobs.content_version_id requires.
	version, err := h.cont.EnsureVersion(r.Context(), conf.ID, conf.Title,
		conf.ShortText, conf.MediumText, conf.LongText, conf.Language, actor)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not snapshot the confession text")
		return
	}

	// Record the request before doing the work. Until now a generation left no
	// trace beyond an audit line, so there was no status to poll and nothing to
	// retry, and a double-submitted form billed the provider twice.
	job, created, err := h.audio.CreateJob(r.Context(), &models.AudioJob{
		ConfessionID: req.ConfessionID, VariantID: req.VariantID, VoiceID: req.VoiceID,
		ContentVersionID: version.ID, RequestedBy: actor,
		Provider:       providerName(h.pipeline),
		Format:         "m4a",
		IdempotencyKey: generationKey(req.ConfessionID, req.VariantID, req.VoiceID, req.Language, version.ID, req.Force),
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not record the generation request")
		return
	}
	if !created {
		// This exact render was already requested.
		switch audio.JobStatus(job.Status) {
		case audio.JobSucceeded, audio.JobProcessing, audio.JobQueued:
			httpx.WriteJSON(w, http.StatusOK, map[string]any{
				"job":    job,
				"reused": true,
				"note":   "this render was already requested; see the job for its status",
			})
			return
		case audio.JobFailed:
			if job.AttemptCount >= job.MaxAttempts {
				httpx.WriteError(w, http.StatusConflict, fmt.Sprintf(
					"this render already failed %d of %d attempts; pass force to start a new job",
					job.AttemptCount, job.MaxAttempts))
				return
			}
			// The lifecycle does not permit failed -> processing directly, so a
			// retry is requeued first. That keeps the attempt visible in the
			// job's history instead of happening silently inside the runner.
			if job, err = h.audio.RequeueJob(r.Context(), job.ID); err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "could not requeue the generation job")
				return
			}
		}
	}

	// Detach from the request context, but keep its values.
	//
	// Generation calls a paid external provider and then writes to object
	// storage. Bound to r.Context(), a client disconnect - a closed laptop, a
	// proxy timeout on a long render - cancelled both, so the provider was
	// billed and the audio was thrown away with no record of either. The job
	// record makes the loss visible; this stops it happening.
	genCtx := context.WithoutCancel(r.Context())

	if _, err := h.audio.StartJob(genCtx, job.ID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not start the generation job")
		return
	}

	res, err := h.pipeline.Generate(genCtx, lic, voice.GenerateRequest{
		ConfessionID: req.ConfessionID, VariantID: req.VariantID, VoiceID: req.VoiceID,
		Language: req.Language, Text: text, Use: rights.UseSynthesis,
		Territory: req.Territory, RequestedBy: actor, Force: req.Force,
	})

	var denied *voice.ErrRightsDenied
	if errors.As(err, &denied) {
		// 451 is the honest status: the request is well-formed and the content
		// exists, but it may not be produced for legal reasons.
		code := string(denied.Decision.Reason)
		if _, ferr := h.audio.FailJob(genCtx, job.ID, code, denied.Decision.Detail); ferr != nil {
			log.Printf("audio: could not record rights refusal on job %s: %v", job.ID, ferr)
		}
		httpx.WriteJSON(w, http.StatusUnavailableForLegalReasons, map[string]any{
			"error":  "voice rights do not permit this generation",
			"reason": code,
			"detail": denied.Decision.Detail,
			"job_id": job.ID,
		})
		return
	}
	if err != nil {
		code := "provider_error"
		status := http.StatusBadGateway
		retryable := voice.IsRetryable(err)
		if !retryable {
			code = "provider_rejected"
			status = http.StatusUnprocessableEntity
		}
		if _, ferr := h.audio.FailJob(genCtx, job.ID, code, err.Error()); ferr != nil {
			log.Printf("audio: could not record failure on job %s: %v", job.ID, ferr)
		}
		if retryable {
			h.enqueueGenerationRetry(genCtx, req.ConfessionID, req.VariantID, req.VoiceID,
				req.Language, version.ID, actor)
		}
		httpx.WriteError(w, status, "audio generation failed: "+err.Error())
		return
	}

	asset := &models.AudioAsset{
		ConfessionID: req.ConfessionID, VariantID: req.VariantID, VoiceID: req.VoiceID,
		URL: res.Key, SizeBytes: res.SizeBytes, DurationSeconds: res.DurationSeconds,
		// The exact text this render speaks, so a later edit cannot silently
		// change what an approved asset means.
		ContentVersionID: version.ID,
		// New audio enters QA rather than going straight to listeners (§31).
		Status: string(audio.StatusProcessing),
	}
	if err := h.audio.UpsertAsset(genCtx, asset); err != nil {
		// The render exists and was paid for. Losing the reason would make this
		// undiagnosable: the operator sees a 500 and the audio team has a
		// provider invoice for audio the catalogue does not know about.
		log.Printf("audio: job %s rendered %s but the asset could not be recorded: %v", job.ID, res.Key, err)
		if _, ferr := h.audio.FailJob(genCtx, job.ID, "asset_record_failed", err.Error()); ferr != nil {
			log.Printf("audio: could not record the asset failure on job %s: %v", job.ID, ferr)
		}
		httpx.WriteError(w, http.StatusInternalServerError, "audio was generated but could not be recorded")
		return
	}
	job, err = h.audio.CompleteJob(genCtx, job.ID, asset.ID)
	if err != nil {
		log.Printf("audio: job %s produced an asset but could not be completed: %v", job.ID, err)
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"job":             job,
		"asset":           asset,
		"reused":          res.Reused,
		"checksum":        res.Checksum,
		"content_version": version.VersionNumber,
		"note":            "Asset is in QA. Approve it to make it available to listeners.",
	})
}

// generationKey identifies one render request so a retry cannot bill the
// provider twice. Force produces a distinct key on purpose: it is an explicit
// request to render again.
func generationKey(confessionID, variantID, voiceID, language, versionID string, force bool) string {
	material := strings.Join([]string{confessionID, variantID, voiceID, language, versionID}, "\x00")
	if force {
		material += "\x00force\x00" + time.Now().UTC().Format(time.RFC3339Nano)
	}
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:])
}

// providerName reports which adapter will do the work, for the job record.
func providerName(p *voice.Pipeline) string {
	if p == nil || p.Provider == nil {
		return ""
	}
	return p.Provider.Name()
}

// textForVariant picks the script matching a variant's length.
func textForVariant(c *models.Confession, variantID string) string {
	for _, v := range c.Variants {
		if v.ID != variantID {
			continue
		}
		switch {
		case v.DurationSeconds <= 45:
			return c.ShortText
		case v.DurationSeconds <= 120:
			return c.MediumText
		default:
			return c.LongText
		}
	}
	if c.MediumText != "" {
		return c.MediumText
	}
	return c.LongText
}

func validRightsStatus(s string) bool {
	switch s {
	case "pending", "active", "expired", "revoked":
		return true
	}
	return false
}

func validateTime(s string) error {
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if _, err := time.Parse(layout, s); err == nil {
			return nil
		}
	}
	return errors.New("unparseable time")
}

// enqueueGenerationRetry hands a render the provider could not finish to the
// background queue.
//
// A retryable provider fault is precisely the case a queue exists for: the
// request was well-formed, the rights were in order, the operator did everything
// right, and the only problem is that the provider was briefly unavailable.
// Without this the render stays failed until somebody notices and resubmits it
// by hand, and the confession is left without audio in the meantime.
//
// The idempotency key is derived from the render, so a second failure of the
// same render does not queue a second job behind the first.
func (h *Handler) enqueueGenerationRetry(ctx context.Context,
	confessionID, variantID, voiceID, language, versionID, actor string) {
	if h.queue == nil {
		return
	}
	id, err := h.queue.Enqueue(ctx, jobs.Job{
		Type: workers.TypeAudioGenerate,
		Payload: map[string]any{
			"confession_id": confessionID,
			"variant_id":    variantID,
			"voice_id":      voiceID,
			"language":      language,
			"actor":         actor,
		},
		IdempotencyKey: "retry:" + generationKey(confessionID, variantID, voiceID, language, versionID, false),
		MaxAttempts:    jobs.DefaultMaxAttempts,
	})
	switch {
	case errors.Is(err, jobs.ErrDuplicateJob):
		log.Printf("audio: a retry for this render is already queued (%s)", id)
	case err != nil:
		log.Printf("audio: could not queue a retry for %s/%s: %v", confessionID, voiceID, err)
	default:
		log.Printf("audio: queued retry %s for %s/%s after a retryable provider fault",
			id, confessionID, voiceID)
	}
}
