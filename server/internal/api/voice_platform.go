package api

// Licensed minister voice platform endpoints (Voice Platform §31, §46, §53,
// §67, §70).
//
// Request flow for POST /voices/generate:
//
//	auth → rate limit → content safety → live rights check + plan (no GPU)
//	     → cache lookup by content hash → HIT: signed URL
//	                                    → MISS: durable job on tts.{high,normal,low}
//
// The worker re-authorizes before each provider receives text, before a
// private upload, and in the transaction that publishes its result.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/ratelimit"
	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/voiceengine"
	"github.com/Teamthy/i-confess/internal/voiceeval"
	"github.com/Teamthy/i-confess/internal/voicegov"
	"github.com/google/uuid"
)

// JobVoiceGenerate is the queue job type for synthetic minister renders.
const JobVoiceGenerate = "voice.generate"

// VoiceGenerationRule bounds per-user synthesis. Generation costs GPU time and
// is the abuse surface for impersonation, so it is much tighter than reads.
var VoiceGenerationRule = ratelimit.Rule{Burst: 20, Window: time.Hour}

// SetVoiceOrchestrator installs the multi-engine voice orchestrator. Without
// it, generation endpoints report 503 and read endpoints still work.
func (h *Handler) SetVoiceOrchestrator(o *voiceengine.Orchestrator) { h.vorch = o }

// SetVoiceThresholds installs the empirically-set promotion thresholds.
func (h *Handler) SetVoiceThresholds(t voiceeval.Thresholds) { h.vthresholds = t }

// RegisterVoiceJobs registers the generation worker on the current queue. Call
// after SetQueue.
func (h *Handler) RegisterVoiceJobs() {
	h.queue.Register(JobVoiceGenerate, h.runVoiceGeneration)
}

func actorOf(r *http.Request) (id, label string) {
	if c := auth.FromContext(r); c != nil {
		return c.Sub, c.Email
	}
	return "", "system"
}

func isVoiceAdmin(r *http.Request) bool {
	c := auth.FromContext(r)
	if c == nil {
		return false
	}
	switch c.Role {
	case auth.RoleSuperAdmin, auth.RoleVoiceManager, auth.RoleAudioProducer, auth.RoleContentAdmin:
		return true
	}
	return false
}

// ---------------------------------------------------------------- public

// listMinisterVoices is the Voice Library (§45). Legal detail is never
// exposed here; only voices currently licensed for generation are listed.
func (h *Handler) listMinisterVoices(w http.ResponseWriter, r *http.Request) {
	voices, err := h.vplat.ListMinisterVoices(r.Context(), true)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to list voices")
		return
	}
	q := r.URL.Query()
	out := make([]store.MinisterVoice, 0, len(voices))
	for _, v := range voices {
		if l := q.Get("language"); l != "" && !voicegov.MatchesLanguage([]string{l}, v.Locale) && !strings.EqualFold(l, v.Language) {
			continue
		}
		if a := q.Get("accent"); a != "" && !strings.EqualFold(a, v.Accent) {
			continue
		}
		if g := q.Get("gender"); g != "" && !strings.EqualFold(g, v.GenderPresentation) {
			continue
		}
		out = append(out, v)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"voices":     out,
		"disclosure": "Minister voices are AI-generated using an authorized synthetic voice.",
	})
}

func (h *Handler) getMinisterVoice(w http.ResponseWriter, r *http.Request) {
	v, err := h.vplat.MinisterVoiceByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrVoiceNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "voice not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load voice")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"voice": v,
		"disclosure": "AI-generated using an authorized synthetic voice. Not a recording of the minister speaking these words."})
}

// getMinisterVoiceStyles lists styles that have a rights-cleared reference,
// i.e. that can actually be rendered.
func (h *Handler) getMinisterVoiceStyles(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := h.vplat.MinisterVoiceByID(r.Context(), id); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "voice not found")
		return
	}
	refs, err := h.vplat.References(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load styles")
		return
	}
	seen := map[string]bool{}
	styles := []string{}
	for _, ref := range refs {
		if ref.RightsOK && !seen[ref.Style] {
			seen[ref.Style] = true
			styles = append(styles, ref.Style)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"voiceId": id, "styles": styles})
}

type generateVoiceRequest struct {
	VoiceID   string  `json:"voiceId"`
	Text      string  `json:"text"`
	Language  string  `json:"language"`
	Locale    string  `json:"locale"`
	Style     string  `json:"style"`
	Purpose   string  `json:"purpose"`
	Speed     float64 `json:"speed"`
	Pitch     float64 `json:"pitch"`
	Priority  string  `json:"priority"`
	Territory string  `json:"territory"`
}

// generateMinisterVoice queues (or returns cached) synthetic speech.
func (h *Handler) generateMinisterVoice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if h.vorch == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "voice engine is not configured on this server")
		return
	}
	userID, email := actorOf(r)
	if h.limiter != nil {
		if ok, retry := h.limiter.Allow("voicegen:"+userID, VoiceGenerationRule); !ok {
			ratelimit.TooManyRequests(w, retry)
			return
		}
	}
	req, purpose, ok := h.decodeVoiceRequest(w, r, email)
	if !ok {
		return
	}
	// Scheduling privileges are independent of ownership.
	var priority voiceengine.Priority
	if isVoiceAdmin(r) {
		priority = voiceengine.Priority(req.Priority)
	}
	gen, plan, err := h.queueVoiceGeneration(r.Context(), voiceQueueInput{
		Req: req, Purpose: purpose, UserID: userID, Actor: email, Remote: clientIP(r), Priority: priority,
	})
	if err != nil {
		if plan == nil {
			h.writeVoicePlanError(w, r, req.VoiceID, email, err)
			return
		}
		httpx.WriteError(w, http.StatusServiceUnavailable, "could not queue generation")
		return
	}
	if gen.Status == string(voiceengine.GenCompleted) {
		h.writeGeneration(w, r, gen, http.StatusOK)
		return
	}
	h.writeGeneration(w, r, gen, http.StatusAccepted)
}

// voiceQueueInput is everything needed to plan and queue one render. It is
// shared by single generation, audio sessions and admin batches, so every
// path runs the same rights check, cache and dedup.
type voiceQueueInput struct {
	Req       generateVoiceRequest
	Purpose   voicegov.ContentPurpose
	UserID    string
	Actor     string
	Remote    string
	Editorial bool // set only by the role-gated admin batch route; personal is the default
	// Only staff may override interactive priority; batches force batch.
	Priority voiceengine.Priority
}

// queueVoiceGeneration resolves the plan (rights, model, reference), then
// creates or reuses the generation row and enqueues a worker job on a miss.
// A nil plan with an error means planning failed (use writeVoicePlanError).
func (h *Handler) queueVoiceGeneration(ctx context.Context, in voiceQueueInput) (*store.Generation, *voiceengine.Plan, error) {
	req := in.Req
	// Scope before hashing, both at submission and when a worker verifies the
	// queued plan. A private submission without an owner is never a catalog hit.
	visibility, owner := "catalog", ""
	userText := !in.Editorial
	if userText || in.Purpose == voicegov.PurposeConfession {
		visibility, owner = "private", in.UserID
		if owner == "" {
			return nil, nil, errors.New("private voice generation requires an owner")
		}
	}
	plan, err := h.resolveVoicePlan(ctx, voiceengine.Request{
		VoiceID: req.VoiceID, Markup: req.Text, Language: req.Language, Locale: req.Locale, Style: req.Style,
		Purpose: in.Purpose, UserText: userText, Territory: req.Territory, Speed: req.Speed, Pitch: req.Pitch, CacheOwnerID: owner,
	})
	if err != nil {
		return nil, nil, err
	}
	prio := in.Priority
	if prio == "" {
		prio = voiceengine.PriorityInteractive
	}
	textSum := sha256.Sum256([]byte(voiceengine.PlainText(mustParse(req.Text))))
	gen, created, err := h.vplat.CreateOrGetGeneration(ctx, &store.Generation{
		ContentHash: plan.ContentHash, VoiceID: req.VoiceID, ModelID: plan.Model.ID, Engine: string(plan.Model.Engine),
		EngineVersion: plan.Model.EngineVersion, Purpose: string(in.Purpose), Style: req.Style, Language: req.Language,
		Locale: req.Locale, TextSHA256: hex.EncodeToString(textSum[:]), GrantVersion: plan.Decision.GrantVersion,
		Queue: voiceengine.QueueFor(prio), OwnerUserID: owner, Visibility: visibility, RequestedBy: in.Actor,
	})
	if err != nil {
		return nil, plan, err
	}
	voiceMetrics.cache(created)
	if created {
		_, err := h.queue.Enqueue(ctx, jobs.Job{
			ID: gen.JobID, Type: JobVoiceGenerate, IdempotencyKey: JobVoiceGenerate + ":" + gen.JobID, MaxAttempts: 4,
			Payload: map[string]any{
				"generation_id": gen.ID, "voice_id": req.VoiceID, "text": req.Text, "language": req.Language,
				"locale": req.Locale, "style": req.Style, "purpose": string(in.Purpose), "user_text": userText,
				"territory": req.Territory, "speed": req.Speed, "pitch": req.Pitch, "queue": gen.Queue,
			},
		})
		if err != nil && !errors.Is(err, jobs.ErrDuplicateJob) {
			stateErr := h.vplat.FailGeneration(ctx, gen.ID, gen.JobID, string(voiceengine.ClassTransient), "enqueue failed")
			return nil, plan, errors.Join(err, stateErr)
		}
		_ = h.vplat.AppendRightsAudit(ctx, store.RightsAuditEntry{VoiceID: req.VoiceID, Actor: in.Actor, Action: "VOICE_GENERATION_QUEUED",
			ModelID: plan.Model.ID, GenerationID: gen.ID, GrantVersion: plan.Decision.GrantVersion, Decision: "allowed", RemoteAddr: in.Remote})
	}
	return gen, plan, nil
}

// decodeVoiceRequest parses and validates a generate/stream body, including
// the script safety floor. It writes the error response itself.
func (h *Handler) decodeVoiceRequest(w http.ResponseWriter, r *http.Request, email string) (generateVoiceRequest, voicegov.ContentPurpose, bool) {
	var req generateVoiceRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return req, "", false
	}
	if req.Language == "" {
		req.Language = "en"
	}
	if req.Style == "" {
		req.Style = "reflection"
	}
	purpose := voicegov.ContentPurpose(req.Purpose)
	if !voicegov.ValidPurpose(purpose) || purpose == voicegov.PurposeMarketing || purpose == voicegov.PurposeResearch {
		httpx.WriteError(w, http.StatusBadRequest, "purpose must be one of confession, prayer, reflection, devotional, bible")
		return req, "", false
	}
	if !voiceengine.ValidStyle(req.Style) {
		httpx.WriteError(w, http.StatusBadRequest, "unknown style")
		return req, "", false
	}
	if req.Speed != 0 && (req.Speed < 0.5 || req.Speed > 1.5) {
		httpx.WriteError(w, http.StatusBadRequest, "speed must be between 0.5 and 1.5")
		return req, "", false
	}
	if req.Pitch < -6 || req.Pitch > 6 {
		httpx.WriteError(w, http.StatusBadRequest, "pitch must be between -6 and 6 semitones")
		return req, "", false
	}
	// Text typed on a listener route is user-submitted, including when that
	// listener also works on the editorial team. Roles are not content provenance.
	if err := voiceengine.ValidateScript(req.Text, true); err != nil {
		var v *voiceengine.SafetyViolation
		if errors.As(err, &v) {
			voiceMetrics.refusedContent()
			_ = h.vplat.AppendRightsAudit(r.Context(), store.RightsAuditEntry{VoiceID: req.VoiceID, Actor: email,
				Action: "GENERATION_REFUSED_CONTENT", Decision: "denied", Reason: v.Code, RemoteAddr: clientIP(r)})
			httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": v.Detail, "code": v.Code})
			return req, "", false
		}
	}

	return req, purpose, true
}

func mustParse(text string) []voiceengine.Segment {
	segs, _ := voiceengine.ParseMarkup(text)
	return segs
}

func (h *Handler) resolveVoicePlan(ctx context.Context, req voiceengine.Request) (*voiceengine.Plan, error) {
	if _, err := h.vplat.MinisterVoiceByID(ctx, req.VoiceID); err != nil {
		return nil, err
	}
	grant, err := h.vplat.Grant(ctx, req.VoiceID)
	if err != nil {
		return nil, err
	}
	models, err := h.vplat.Models(ctx, req.VoiceID)
	if err != nil {
		return nil, err
	}
	refs, err := h.vplat.References(ctx, req.VoiceID)
	if err != nil {
		return nil, err
	}
	orch, err := h.requestOrchestrator(ctx)
	if err != nil {
		return nil, err
	}
	return orch.Resolve(ctx, grant, models, refs, req)
}

// requestOrchestrator installs the live grant check and dictionary on a copy.
// Concurrent requests never mutate the shared orchestrator.
func (h *Handler) requestOrchestrator(ctx context.Context) (*voiceengine.Orchestrator, error) {
	entries, ver, err := h.vplat.Pronunciations(ctx)
	if err != nil {
		return nil, err
	}
	o := *h.vorch
	o.ConfirmGrant = h.vplat.CheckRenderGrant
	o.Dict, o.DictVer = voiceengine.NewDictionary(entries), ver
	return &o, nil
}

func (h *Handler) writeVoicePlanError(w http.ResponseWriter, r *http.Request, voiceID, actor string, err error) {
	var rd *voiceengine.ErrRightsDenied
	var workerErr *voiceengine.Error
	switch {
	case errors.Is(err, store.ErrVoiceNotFound):
		httpx.WriteError(w, http.StatusNotFound, "voice not found")
	case errors.As(err, &rd):
		voiceMetrics.refusedRights()
		_ = h.vplat.AppendRightsAudit(r.Context(), store.RightsAuditEntry{VoiceID: voiceID, Actor: actor, Action: "GENERATION_REFUSED_RIGHTS",
			GrantVersion: rd.Decision.GrantVersion, Decision: "denied", Reason: string(rd.Decision.Reason), Detail: rd.Decision.Detail, RemoteAddr: clientIP(r)})
		// Ordinary users learn that the voice is unavailable, not the licence terms.
		body := map[string]any{"error": "this voice is not available for that use", "code": "voice_unavailable"}
		if isVoiceAdmin(r) {
			body["reason"], body["detail"], body["missing"] = rd.Decision.Reason, rd.Decision.Detail, rd.Decision.Missing
		}
		httpx.WriteJSON(w, http.StatusForbidden, body)
	case voiceengine.Classify(err) == voiceengine.ClassContent:
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "code": "invalid_script"})
	case voiceengine.Classify(err) == voiceengine.ClassModel || voiceengine.Classify(err) == voiceengine.ClassIdentity:
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "this voice is not ready for generation", "code": "voice_not_ready"})
	case errors.As(err, &workerErr) && workerErr.Class.Retryable():
		// A live preview has no queue to wait in. Report worker unavailability
		// without exposing its private endpoint or internal error message.
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "voice synthesis is temporarily unavailable", "code": "voice_temporarily_unavailable"})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "failed to plan generation")
	}
}

func (h *Handler) writeGeneration(w http.ResponseWriter, r *http.Request, g *store.Generation, status int) {
	w.Header().Set("Cache-Control", "private, no-store") // response may contain signed URLs
	uid, _ := actorOf(r)
	if !g.ReadableBy(uid) {
		httpx.WriteError(w, http.StatusNotFound, "job not found")
		return
	}
	body := map[string]any{"generation": g, "synthetic": true,
		"disclosure": "AI-generated using an authorized synthetic voice."}
	servable := true
	if g.Status == string(voiceengine.GenCompleted) {
		servable, _ = h.assetServable(r.Context(), g.VoiceID)
		if !servable {
			// The licence no longer authorizes delivery (revoked, expired,
			// suspended). Never hand out a URL, and say so rather than
			// pretending the render is still processing.
			body["available"], body["unavailableReason"] = false, "voice_rights_ended"
		} else if !publishedMaster(g) {
			body["available"], body["unavailableReason"] = false, "audio_unavailable"
		}
	}
	if servable && publishedMaster(g) && h.renderStore != nil {
		ttl := 4 * time.Hour
		if g.Visibility == "private" {
			ttl = 15 * time.Minute
		}
		if url, err := h.renderStore.GenerateSignedURL(r.Context(), g.StorageKey, ttl); err == nil {
			body["audioUrl"], body["expiresAt"] = url, time.Now().Add(ttl).UTC().Format(time.RFC3339)
		}
		if vs := h.signVariants(r.Context(), g, ttl); len(vs) > 0 {
			body["variants"] = vs
		}
	}
	httpx.WriteJSON(w, status, body)
}

func (h *Handler) getAudioJob(w http.ResponseWriter, r *http.Request) {
	uid, _ := actorOf(r)
	g, err := h.vplat.GenerationForActor(r.Context(), r.PathValue("id"), uid, true)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load job")
		return
	}
	h.writeGeneration(w, r, g, http.StatusOK)
}

func (h *Handler) cancelAudioJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	id := r.PathValue("id")
	uid, _ := actorOf(r)
	manageCatalog := isVoiceAdmin(r)
	ok, err := h.vplat.CancelGeneration(r.Context(), id, uid, manageCatalog)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to cancel")
		return
	}
	if !ok {
		g, err := h.vplat.GenerationForActor(r.Context(), id, uid, manageCatalog)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "failed to load job")
			return
		}
		if g == nil {
			httpx.WriteError(w, http.StatusNotFound, "job not found")
			return
		}
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{"error": "job can no longer be cancelled", "status": g.Status})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"generationId": id, "status": voiceengine.GenCancelled})
}

// ---------------------------------------------------------------- worker

// runVoiceGeneration executes one queued render. Errors are classified so the
// queue retries infrastructure faults and parks everything else (§71).
func (h *Handler) runVoiceGeneration(ctx context.Context, job *jobs.Job) error {
	p := job.Payload
	str := func(k string) string { s, _ := p[k].(string); return s }
	num := func(k string) float64 { f, _ := p[k].(float64); return f }
	genID := str("generation_id")
	if genID == "" {
		return jobs.Permanent(errors.New("generation_id is required"))
	}
	fail := func(class voiceengine.ErrorClass, cause error) error {
		if errors.Is(cause, store.ErrGenerationInactive) {
			return nil // cancellation or a newer submission won the conditional update
		}
		voiceMetrics.failure(string(class))
		var stateErr error
		if class.Retryable() && job.Attempts < job.MaxAttempts {
			// Fail on the queue will schedule backoff. The generation must
			// remain claimable and cancellable until the last attempt.
			stateErr = h.vplat.AdvanceGeneration(ctx, genID, job.ID, voiceengine.GenQueued)
		} else {
			// The generation is visible to the listener. Keep raw provider,
			// database and storage diagnostics on the queue job, not on it.
			msg := "voice generation could not be completed"
			if class.Retryable() {
				msg = fmt.Sprintf("voice generation could not be completed after %d attempts", job.Attempts)
			}
			var denied *voiceengine.ErrRightsDenied
			if errors.As(cause, &denied) {
				stateErr = h.vplat.RefuseGeneration(ctx, genID, job.ID, msg, denied.Decision)
			} else {
				stateErr = h.vplat.FailGeneration(ctx, genID, job.ID, string(class), msg)
			}
		}
		if errors.Is(stateErr, store.ErrGenerationInactive) {
			return nil
		}
		if stateErr != nil {
			return errors.Join(cause, fmt.Errorf("record voice generation failure: %w", stateErr))
		}
		if !class.Retryable() {
			return jobs.Permanent(cause)
		}
		return cause // the queue, not the worker, schedules retries or dead-letters exhaustion
	}
	g, err := h.vplat.GenerationByID(ctx, genID)
	if err != nil {
		return fail(voiceengine.ClassTransient, err)
	}
	if g == nil {
		return jobs.Permanent(errors.New("voice generation not found"))
	}
	if g.JobID != "" && g.JobID != job.ID {
		return nil // a new submission owns this generation ID
	}
	if voiceengine.GenerationStatus(g.Status).Terminal() {
		if g.Status == string(voiceengine.GenFailed) {
			return jobs.Permanent(fmt.Errorf("voice generation already failed (%s)", g.ErrorClass))
		}
		return nil // cancelled or already completed
	}
	if h.vorch == nil {
		return fail(voiceengine.ClassModel, errors.New("voice engine not configured"))
	}
	started := time.Now()
	if t, perr := time.Parse(time.RFC3339Nano, g.UpdatedAt); perr == nil {
		voiceMetrics.queued(started.Sub(t))
	}
	if err := h.vplat.AdvanceGeneration(ctx, genID, job.ID, voiceengine.GenProcessing); err != nil {
		return fail(voiceengine.ClassTransient, err)
	}
	userText, ok := p["user_text"].(bool)
	privateUse := userText || g.Purpose == string(voicegov.PurposeConfession)
	if !ok || !g.ValidAudience() || privateUse != (g.Visibility == "private") || g.VoiceID != str("voice_id") || g.Purpose != str("purpose") {
		return fail(voiceengine.ClassPermanent, errors.New("invalid queued render audience"))
	}
	req := voiceengine.Request{GenerationID: genID, CacheOwnerID: g.OwnerUserID, QueuedContentHash: g.ContentHash, VoiceID: str("voice_id"), Markup: str("text"), Language: str("language"),
		Locale: str("locale"), Style: str("style"), Purpose: voicegov.ContentPurpose(str("purpose")), UserText: userText,
		Territory: str("territory"), Speed: num("speed"), Pitch: num("pitch")}

	// Fresh rights, models and references: never the enqueue-time snapshot.
	grant, err := h.vplat.Grant(ctx, req.VoiceID)
	if err != nil {
		return fail(voiceengine.ClassTransient, err)
	}
	models, err := h.vplat.Models(ctx, req.VoiceID)
	if err != nil {
		return fail(voiceengine.ClassTransient, err)
	}
	refs, err := h.vplat.References(ctx, req.VoiceID)
	if err != nil {
		return fail(voiceengine.ClassTransient, err)
	}
	orch, err := h.requestOrchestrator(ctx)
	if err != nil {
		return fail(voiceengine.ClassTransient, err)
	}
	plan, res, err := orch.Generate(ctx, grant, models, refs, req)
	if err != nil {
		return fail(voiceengine.Classify(err), err)
	}
	// Use the decision for the actual renderer, not the enqueue-time grant.
	completed := &store.Generation{ID: genID, JobID: job.ID,
		ModelID: plan.Model.ID, Engine: string(res.Engine), EngineVersion: res.EngineVersion, DurationMS: res.DurationMS,
		FellBack: plan.FellBack, FallbackReason: plan.FallbackReason}
	sum := sha256.Sum256(res.Audio)
	audioHash := hex.EncodeToString(sum[:])
	renderHash := plan.ContentHash
	if plan.FellBack {
		// A substitution belongs to this job, not the primary cache. Isolate
		// its row and object even if the same fallback is later promoted.
		sum := sha256.Sum256([]byte("icf:substitution:v1\x00" + renderHash + "\x00" + genID))
		renderHash = hex.EncodeToString(sum[:])
	}
	// The content hash deduplicates requests in PostgreSQL. Each physical
	// attempt writes its own object so a losing/stale delivery can neither
	// overwrite nor delete a winner's master at the same hash.
	key := fmt.Sprintf(storage.VoiceRenderPrefix+"%s/%s/%s/%s/%s.wav", req.VoiceID, req.Purpose, plan.Model.ModelVersion, uuid.NewString(), renderHash)
	meta := map[string]string{
		"synthetic_audio": "true", "voice_id": req.VoiceID, "model_id": plan.Model.ID, "generation_id": genID,
		"engine": string(res.Engine), "engine_version": res.EngineVersion, "audio_sha256": audioHash,
		"style": req.Style, "language": req.Language, "fell_back": fmt.Sprint(plan.FellBack),
	}
	if err := h.vplat.BeginGenerationUpload(ctx, genID, job.ID, plan.Use); err != nil {
		return fail(voiceengine.Classify(err), err)
	}
	if h.renderStore == nil {
		return fail(voiceengine.ClassStorage, errors.New("no object storage configured"))
	}
	if err := h.renderStore.Upload(ctx, key, res.Audio, meta); err != nil {
		h.discardRenderObject(ctx, genID, key)
		return fail(voiceengine.ClassStorage, err)
	}
	completed.ContentHash, completed.AudioSHA256, completed.StorageKey = renderHash, audioHash, key
	h.recordRenderCost(completed, res, voiceCostFromEnv())
	if err := h.vplat.CompleteGeneration(ctx, completed, plan.Use); err != nil {
		if errors.Is(err, store.ErrRenderCommitUnknown) {
			log.Printf("voice: check publication for %s before cleaning up %s: %v", genID, key, err)
		} else {
			h.discardRenderObject(ctx, genID, key)
		}
		return fail(voiceengine.Classify(err), err)
	}
	voiceMetrics.generatedIn(string(res.Engine), time.Since(started), plan.FellBack)
	// Encodings remain best effort; the committed master and its audit are
	// not rolled back when a later encoding fails or its grant changes.
	h.encodeVariants(ctx, completed, plan.Use, res.Audio, meta)
	return nil
}

// Only for keys unique to this physical render and known not to be published.
// Do not delete on an ambiguous commit; that requires private-store reconciliation.
func (h *Handler) discardRenderObject(ctx context.Context, genID, key string) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := h.renderStore.Delete(cleanup, key); err != nil {
		log.Printf("voice: cleanup of unpublished object for %s failed (%s): %v", genID, key, err)
	}
}

// ---------------------------------------------------------------- admin

func (h *Handler) adminRegisterMinisterVoice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		store.MinisterVoice
		RightsHolder string `json:"rightsHolder"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.RightsHolder) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name and rightsHolder are required")
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Name
	}
	_, actor := actorOf(r)
	v := req.MinisterVoice
	if err := h.vplat.CreateMinisterVoice(r.Context(), &v, req.RightsHolder, actor); err != nil {
		httpx.WriteError(w, http.StatusConflict, "could not register voice")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"voice": v, "rightsStatus": voicegov.StatusPending,
		"note": "The voice authorizes nothing until its rights terms are recorded, a document is filed, and it is approved."})
}

func (h *Handler) adminGetVoiceGrant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	g, err := h.vplat.Grant(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load grant")
		return
	}
	if g == nil {
		httpx.WriteError(w, http.StatusNotFound, "no grant for this voice")
		return
	}
	now := time.Now().UTC()
	eval := map[string]any{}
	for _, a := range []voicegov.Action{voicegov.ActionIngestRecording, voicegov.ActionZeroShotClone, voicegov.ActionTrain, voicegov.ActionFineTune} {
		eval[string(a)] = voicegov.Authorize(g, voicegov.Request{Action: a, At: now})
	}
	for _, p := range []voicegov.ContentPurpose{voicegov.PurposeConfession, voicegov.PurposePrayer, voicegov.PurposeReflection,
		voicegov.PurposeDevotional, voicegov.PurposeBible, voicegov.PurposeMarketing} {
		eval["generate:"+string(p)] = voicegov.Authorize(g, voicegov.Request{Action: voicegov.ActionGenerate, Purpose: p, At: now})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"grant": g, "effectiveStatus": voicegov.EffectiveStatus(g, now),
		"existingAssetPolicy": voicegov.ExistingAssetPolicy(g, now), "evaluation": eval,
		"allCapabilities": voicegov.AllCapabilities,
	})
}

func (h *Handler) adminUpdateVoiceGrant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RightsHolder    string          `json:"rightsHolder"`
		EffectiveFrom   *time.Time      `json:"effectiveFrom"`
		ExpiresAt       *time.Time      `json:"expiresAt"`
		Capabilities    map[string]bool `json:"capabilities"`
		Restrictions    []string        `json:"restrictions"`
		Territories     []string        `json:"territories"`
		Languages       []string        `json:"languages"`
		PostTermination string          `json:"postTermination"`
		Notes           string          `json:"notes"`
		// Attestation must be true: the admin confirms the terms match the
		// signed document on file. Mirrors the legacy AI-grant attestation.
		Attestation bool `json:"attestation"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !req.Attestation {
		httpx.WriteError(w, http.StatusBadRequest, "attestation is required: confirm these terms match the signed rights document")
		return
	}
	t := store.GrantTerms{RightsHolder: req.RightsHolder, EffectiveFrom: req.EffectiveFrom, ExpiresAt: req.ExpiresAt,
		Capabilities: map[voicegov.Capability]bool{}, Territories: req.Territories, Languages: req.Languages,
		PostTermination: voicegov.AssetPolicy(req.PostTermination), Notes: req.Notes}
	for c, v := range req.Capabilities {
		t.Capabilities[voicegov.Capability(c)] = v
	}
	for _, p := range req.Restrictions {
		t.Restrictions = append(t.Restrictions, voicegov.ContentPurpose(p))
	}
	_, actor := actorOf(r)
	version, err := h.vplat.UpdateGrantTerms(r.Context(), r.PathValue("id"), t, actor, clientIP(r))
	switch {
	case errors.Is(err, store.ErrVoiceNotFound):
		httpx.WriteError(w, http.StatusNotFound, "voice not found")
	case errors.Is(err, store.ErrIllegalTransition):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case err != nil:
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"voiceId": r.PathValue("id"), "grantVersion": version})
	}
}

func (h *Handler) adminAddRightsDocument(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DocumentType string `json:"documentType"`
		Title        string `json:"title"`
		StorageKey   string `json:"storageKey"`
		SHA256       string `json:"sha256"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.DocumentType == "" || req.Title == "" {
		httpx.WriteError(w, http.StatusBadRequest, "documentType, title, storageKey and sha256 are required")
		return
	}
	if strings.HasPrefix(req.StorageKey, "http") {
		httpx.WriteError(w, http.StatusBadRequest, "storageKey must be a private bucket key, not a URL")
		return
	}
	_, actor := actorOf(r)
	id, err := h.vplat.AddRightsDocument(r.Context(), r.PathValue("id"), req.DocumentType, req.Title, req.StorageKey, req.SHA256, actor)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"documentId": id})
}

// adminTransitionVoice returns a handler for one lifecycle action.
func (h *Handler) adminTransitionVoice(to voicegov.Status) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Reason string `json:"reason"`
		}
		_ = httpx.DecodeJSON(r, &req)
		if to != voicegov.StatusApproved && to != voicegov.StatusUnderReview && strings.TrimSpace(req.Reason) == "" {
			httpx.WriteError(w, http.StatusBadRequest, "a reason is required")
			return
		}
		id := r.PathValue("id")
		_, actor := actorOf(r)
		err := h.vplat.TransitionGrant(r.Context(), id, to, actor, req.Reason, clientIP(r))
		switch {
		case errors.Is(err, store.ErrVoiceNotFound):
			httpx.WriteError(w, http.StatusNotFound, "voice not found")
			return
		case errors.Is(err, store.ErrIllegalTransition):
			httpx.WriteError(w, http.StatusConflict, err.Error())
			return
		case err != nil:
			httpx.WriteError(w, http.StatusInternalServerError, "failed to update rights status")
			return
		}
		// Revocation also withdraws the legacy coarse licence so the older
		// ElevenLabs pipeline cannot keep generating in this voice.
		if to == voicegov.StatusRevoked || to == voicegov.StatusSuspended {
			if vr, err := h.vrights.ByVoiceID(r.Context(), id); err == nil && vr != nil {
				vr.AIGenerationAllowed, vr.CommercialUse, vr.MarketingAllowed = false, false, false
				if to == voicegov.StatusRevoked {
					vr.Status, vr.LicenseStatus = "revoked", "revoked"
				}
				_ = h.vrights.Update(r.Context(), vr)
			}
		}
		g, _ := h.vplat.Grant(r.Context(), id)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"voiceId": id, "status": to,
			"existingAssetPolicy": voicegov.ExistingAssetPolicy(g, time.Now().UTC())})
	}
}

func (h *Handler) adminAddVoiceReference(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Style      string  `json:"style"`
		AudioKey   string  `json:"audioKey"`
		Transcript string  `json:"transcript"`
		DurationMS int     `json:"durationMs"`
		Quality    float64 `json:"quality"`
		RightsOK   bool    `json:"rightsOk"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.AudioKey == "" || strings.TrimSpace(req.Transcript) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "style, audioKey and a verified transcript are required")
		return
	}
	id := r.PathValue("id")
	g, err := h.vplat.Grant(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load grant")
		return
	}
	if d := voicegov.Authorize(g, voicegov.Request{Action: voicegov.ActionCreateReference}); !d.Allowed {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]any{"error": "rights do not permit creating references", "reason": d.Reason, "missing": d.Missing})
		return
	}
	_, actor := actorOf(r)
	refID, err := h.vplat.AddReference(r.Context(), voiceengine.Reference{VoiceID: id, Style: req.Style, URI: req.AudioKey,
		Transcript: req.Transcript, DurationMS: req.DurationMS, Quality: req.Quality, RightsOK: req.RightsOK}, actor)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"referenceId": refID})
}

func (h *Handler) adminListVoiceModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.vplat.Models(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to list models")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (h *Handler) adminRegisterVoiceModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Engine          string `json:"engine"`
		EngineVersion   string `json:"engineVersion"`
		ModelName       string `json:"modelName"`
		ModelVersion    string `json:"modelVersion"`
		Mode            string `json:"mode"`
		CheckpointKey   string `json:"checkpointKey"`
		DatasetVersion  string `json:"datasetVersion"`
		TrainingRunID   string `json:"trainingRunId"`
		Language        string `json:"language"`
		LicenseReviewed bool   `json:"licenseReviewed"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Engine == "" || req.ModelVersion == "" {
		httpx.WriteError(w, http.StatusBadRequest, "engine and modelVersion are required")
		return
	}
	id := r.PathValue("id")
	pin := voiceengine.WorkerIdentity{Engine: voiceengine.Engine(req.Engine).Canonical(), Revision: req.EngineVersion}
	if !pin.Valid() {
		httpx.WriteError(w, http.StatusBadRequest, "engineVersion must pin the reviewed worker checkpoint")
		return
	}
	g, _ := h.vplat.Grant(r.Context(), id)
	action := voicegov.ActionZeroShotClone
	if req.Mode == "fine_tuned" {
		action = voicegov.ActionFineTune
	}
	if d := voicegov.Authorize(g, voicegov.Request{Action: action}); !d.Allowed {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]any{"error": "rights do not permit this model", "reason": d.Reason, "missing": d.Missing})
		return
	}
	if req.Language == "" {
		req.Language = "en"
	}
	_, actor := actorOf(r)
	m := &voiceengine.Model{VoiceID: id, Engine: pin.Engine, EngineVersion: pin.Revision,
		ModelVersion: req.ModelVersion, Mode: req.Mode, CheckpointURI: req.CheckpointKey, DatasetVersion: req.DatasetVersion,
		TrainingRunID: req.TrainingRunID, LicenseReviewed: req.LicenseReviewed}
	if err := h.vplat.CreateModel(r.Context(), m, req.ModelName, req.Language, actor); err != nil {
		httpx.WriteError(w, http.StatusConflict, "model version already exists or is invalid; versions are immutable")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"model": m})
}

// adminEvaluateVoiceModel records measured metrics, computes ICF_VOICE_SCORE
// and runs the quality gate. It never promotes.
func (h *Handler) adminEvaluateVoiceModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ModelID            string             `json:"modelId"`
		GoldenSet          string             `json:"goldenSet"`
		Metrics            voiceeval.Metrics  `json:"metrics"`
		Weights            voiceeval.Weights  `json:"weights"`
		HumanApproved      bool               `json:"humanApproved"`
		BlindEvalCompleted bool               `json:"blindEvalCompleted"`
		BlindTestID        string             `json:"blindTestId"`
		FallbackApproved   bool               `json:"fallbackApproved"`
		Baseline           *voiceeval.Metrics `json:"baselineMetrics"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.ModelID == "" || req.GoldenSet == "" {
		httpx.WriteError(w, http.StatusBadRequest, "modelId, goldenSet and metrics are required")
		return
	}
	score, err := voiceeval.Compute(req.Metrics, req.Weights)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	id := r.PathValue("id")
	g, _ := h.vplat.Grant(r.Context(), id)
	rightsOK := voicegov.Authorize(g, voicegov.Request{Action: voicegov.ActionGenerate}).Allowed
	// Blind evaluation evidence: a closed blind test with enough evaluators
	// that actually rated this model. A bare attestation flag is accepted
	// outside production only (and recorded as such), so tests and early
	// benchmarks are not blocked, but production promotion needs real data.
	blindOK, blindBasis, blindNote := false, "none", ""
	switch {
	case req.BlindTestID != "":
		blindOK, blindNote = h.blindEvidence(r.Context(), req.BlindTestID, id, req.ModelID)
		if blindOK {
			blindBasis = "blind_test:" + req.BlindTestID
		}
	case req.BlindEvalCompleted && !voiceProduction():
		blindOK, blindBasis = true, "attested"
	case req.BlindEvalCompleted:
		blindNote = "production requires blindTestId; an attestation flag is not accepted"
	}
	in := voiceeval.GateInput{Candidate: score, RightsAllowed: rightsOK, HumanApproved: req.HumanApproved, BlindEvalCompleted: blindOK}
	if req.Baseline != nil {
		b, err := voiceeval.Compute(*req.Baseline, score.Weights)
		if err == nil {
			in.Baseline = &b
		}
	}
	gate := voiceeval.Gate(in, h.vthresholds)
	_, actor := actorOf(r)
	evalID, err := h.vplat.SaveEvaluation(r.Context(), id, req.ModelID, req.GoldenSet, score.Metrics, score.Weights, gate,
		score.ICFVoiceScore, score.Coverage, string(gate.Verdict), actor)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "failed to save evaluation (unknown model?)")
		return
	}
	if err := h.vplat.SetModelEvaluation(r.Context(), req.ModelID, score.ICFVoiceScore, string(gate.Verdict), req.FallbackApproved, actor); err != nil {
		httpx.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	_ = h.vplat.AppendRightsAudit(r.Context(), store.RightsAuditEntry{VoiceID: id, Actor: actor, Action: "MODEL_EVALUATED",
		ModelID: req.ModelID, Decision: string(gate.Verdict), Reason: "blind:" + blindBasis, Detail: blindNote, RemoteAddr: clientIP(r)})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"evaluationId": evalID, "score": score, "gate": gate,
		"blindEvaluation": map[string]any{"completed": blindOK, "basis": blindBasis, "note": blindNote}})
}

func (h *Handler) adminPromoteVoiceModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	models, err := h.vplat.Models(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load models")
		return
	}
	g, _ := h.vplat.Grant(r.Context(), id)
	if d := voicegov.Authorize(g, voicegov.Request{Action: voicegov.ActionGenerate}); !d.Allowed {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]any{"error": "rights verification failed", "reason": d.Reason})
		return
	}
	changes, err := voiceengine.Promote(models, r.PathValue("modelId"))
	if err != nil {
		httpx.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	_, actor := actorOf(r)
	if err := h.vplat.ApplyModelChanges(r.Context(), id, changes, actor, "MODEL_PROMOTED"); err != nil {
		httpx.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"changes": changes})
}

func (h *Handler) adminRollbackVoiceModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	models, err := h.vplat.Models(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load models")
		return
	}
	// A rollback makes a different model the production model, so it is a
	// promotion by another name and carries the same rights gate. Without it a
	// voice_manager could put a model live for a voice whose rights lapsed
	// since that model was last in production - which is exactly when an
	// operator rolls back and exactly when nobody is thinking about rights.
	g, _ := h.vplat.Grant(r.Context(), id)
	if d := voicegov.Authorize(g, voicegov.Request{Action: voicegov.ActionGenerate}); !d.Allowed {
		httpx.WriteJSON(w, http.StatusForbidden, map[string]any{"error": "rights verification failed", "reason": d.Reason})
		return
	}
	changes, err := voiceengine.Rollback(models, id)
	if err != nil {
		httpx.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	_, actor := actorOf(r)
	if err := h.vplat.ApplyModelChanges(r.Context(), id, changes, actor, "MODEL_ROLLED_BACK"); err != nil {
		httpx.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"changes": changes})
}

func (h *Handler) adminVoiceAudit(w http.ResponseWriter, r *http.Request) {
	entries, err := h.vplat.RightsAudit(r.Context(), r.PathValue("id"), 200)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load audit log")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (h *Handler) adminUpsertPronunciation(w http.ResponseWriter, r *http.Request) {
	var e voiceengine.PronunciationEntry
	if err := httpx.DecodeJSON(r, &e); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.vplat.UpsertPronunciation(r.Context(), e); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"term": e.Term, "locale": e.Locale})
}

// registerVoicePlatformRoutes adds every voice-platform route under both the
// bare and /v1 prefixes (TestRouteParityBetweenPrefixes).
func (h *Handler) registerVoicePlatformRoutes(mux *http.ServeMux, authed, voiceMgr, audioMgr func(http.Handler) http.Handler) {
	const vm = "voice_manager"
	const am = "audio_producer,voice_manager"
	for _, pfx := range []string{"", "/v1"} {
		h.route(mux, "GET "+pfx+"/minister-voices", "public", "voice", "Licensed minister voice library", nil, h.listMinisterVoices)
		h.route(mux, "GET "+pfx+"/voices/{id}", "public", "voice", "One minister voice (no legal detail)", nil, h.getMinisterVoice)
		h.route(mux, "GET "+pfx+"/voices/{id}/styles", "public", "voice", "Styles with a rights-cleared reference", nil, h.getMinisterVoiceStyles)
		// Dedup in CreateOrGetGeneration *after* live rights and checkpoint
		// checks. Replaying a saved HTTP body here could bypass both checks
		// and return a signed URL from a previous model or grant.
		h.route(mux, "POST "+pfx+"/voices/generate", "user", "voice", "Queue or fetch cached synthetic minister speech", authed, h.generateMinisterVoice)
		h.route(mux, "GET "+pfx+"/audio/jobs/{id}", "user", "voice", "Generation job status and signed audio", authed, h.getAudioJob)
		h.route(mux, "POST "+pfx+"/audio/jobs/{id}/cancel", "user", "voice", "Cancel a generation job", authed, h.cancelAudioJob)

		h.route(mux, "POST "+pfx+"/admin/minister-voices", vm, "admin-voice", "Register a minister voice (PENDING, authorizes nothing)", voiceMgr, h.adminRegisterMinisterVoice)
		h.route(mux, "GET "+pfx+"/admin/voices/{id}/grant", vm, "admin-voice", "Granular rights grant with live evaluation", voiceMgr, h.adminGetVoiceGrant)
		h.route(mux, "PUT "+pfx+"/admin/voices/{id}/grant", vm, "admin-voice", "Replace granular rights terms (attested)", voiceMgr, h.adminUpdateVoiceGrant)
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/rights-documents", vm, "admin-voice", "File a signed rights document", voiceMgr, h.adminAddRightsDocument)
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/review", vm, "admin-voice", "Move rights to UNDER_REVIEW", voiceMgr, h.adminTransitionVoice(voicegov.StatusUnderReview))
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/approve", vm, "admin-voice", "Approve voice rights", voiceMgr, h.adminTransitionVoice(voicegov.StatusApproved))
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/restrict", vm, "admin-voice", "Restrict voice rights", voiceMgr, h.adminTransitionVoice(voicegov.StatusRestricted))
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/suspend", vm, "admin-voice", "Suspend voice rights", voiceMgr, h.adminTransitionVoice(voicegov.StatusSuspended))
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/revoke", vm, "admin-voice", "Revoke voice rights (terminal)", voiceMgr, h.adminTransitionVoice(voicegov.StatusRevoked))
		h.route(mux, "GET "+pfx+"/admin/voices/{id}/audit", lvlVoiceAudit, "admin-voice", "Voice rights audit log", nil, h.adminVoiceAudit)
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/references", am, "admin-voice", "Add a reference clip", audioMgr, h.adminAddVoiceReference)
		h.route(mux, "GET "+pfx+"/admin/voices/{id}/models", lvlVoiceRead, "admin-voice", "List model versions", nil, h.adminListVoiceModels)
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/models", am, "admin-voice", "Register an immutable model version", audioMgr, h.adminRegisterVoiceModel)
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/evaluate", am, "admin-voice", "Record metrics, compute ICF_VOICE_SCORE, run the gate", audioMgr, h.adminEvaluateVoiceModel)
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/models/{modelId}/promote", vm, "admin-voice", "Promote an approved model to production", voiceMgr, h.adminPromoteVoiceModel)
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/rollback", vm, "admin-voice", "Roll back to the previous production model", voiceMgr, h.adminRollbackVoiceModel)
		h.route(mux, "PUT "+pfx+"/admin/pronunciations", am, "admin-voice", "Create or update a pronunciation entry", audioMgr, h.adminUpsertPronunciation)
	}
}

// deliveryFormats are the encodings derived from every WAV master (§26).
var deliveryFormats = []string{"aac", "opus", "mp3"}

// encodeVariants derives and stores delivery encodings for a completed
// render. Only the winning master starts encoding. Its siblings are private
// until the variant map commits under the same grant.
func (h *Handler) encodeVariants(ctx context.Context, g *store.Generation, use voicegov.RenderUse, master []byte, meta map[string]string) {
	if h.vworker == nil || h.renderStore == nil {
		return
	}
	current := func() bool {
		if err := h.vplat.CheckRenderGrant(ctx, use); err != nil {
			log.Printf("voice: not encoding or uploading more variants for %s: %v", g.ID, err)
			return false
		}
		return true
	}
	if !current() {
		return
	}
	enc, err := h.vworker.Encode(ctx, master, deliveryFormats)
	if err != nil {
		log.Printf("voice: encode %s failed (serving WAV only): %v", g.ID, err)
		return
	}
	out := make(map[string]store.AudioVariant, len(enc))
	discard := func() {
		for _, v := range out {
			h.discardRenderObject(ctx, g.ID, v.Key)
		}
	}
	for _, f := range deliveryFormats {
		v, ok := enc[f]
		if !ok {
			continue
		}
		// Only these sibling keys belong to this attempt. Never let a worker's
		// extension point a compensating delete at the master or another object.
		k, allowed := variantKey(g.StorageKey, f)
		if !allowed || v.Ext != path.Ext(k) {
			log.Printf("voice: unexpected %s variant extension for %s", f, g.ID)
			continue
		}
		if !current() {
			discard()
			return
		}
		vm := make(map[string]string, len(meta)+2)
		for mk, mv := range meta {
			vm[mk] = mv
		}
		vm["audio_sha256"], vm["master_sha256"] = v.SHA256, meta["audio_sha256"]
		if err := h.renderStore.Upload(ctx, k, v.Data, vm); err != nil {
			log.Printf("voice: upload %s variant %s failed: %v", g.ID, f, err)
			h.discardRenderObject(ctx, g.ID, k)
			continue
		}
		out[f] = store.AudioVariant{Key: k, SHA256: v.SHA256, Bytes: len(v.Data), ContentType: v.ContentType}
	}
	if len(out) > 0 {
		if err := h.vplat.SetGenerationVariants(ctx, g, use, out); err != nil {
			if !errors.Is(err, store.ErrRenderCommitUnknown) {
				discard()
			}
			log.Printf("voice: record variants for %s: %v", g.ID, err)
		}
	}
}

// signVariants returns {format: {url, contentType}} for a servable render.
// Callers must already have checked that the licence still permits delivery.
func (h *Handler) signVariants(ctx context.Context, g *store.Generation, ttl time.Duration) map[string]any {
	if h.renderStore == nil || len(g.Variants) == 0 {
		return nil
	}
	out := make(map[string]any, len(g.Variants))
	for f, v := range g.Variants {
		if !publishedVariant(g, f, v.Key) {
			continue
		}
		if u, err := h.renderStore.GenerateSignedURL(ctx, v.Key, ttl); err == nil {
			out[f] = map[string]any{"url": u, "contentType": v.ContentType, "bytes": v.Bytes}
		}
	}
	return out
}
