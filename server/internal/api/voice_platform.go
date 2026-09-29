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
// The worker (runVoiceGeneration) re-reads the grant and re-authorizes before
// it touches a GPU, so a revocation between enqueue and execution is honoured.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/ratelimit"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/voiceengine"
	"github.com/Teamthy/i-confess/internal/voiceeval"
	"github.com/Teamthy/i-confess/internal/voicegov"
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
	req, purpose, userText, ok := h.decodeVoiceRequest(w, r, email)
	if !ok {
		return
	}
	ctx := r.Context()
	plan, err := h.resolveVoicePlan(ctx, voiceengine.Request{
		VoiceID: req.VoiceID, Markup: req.Text, Language: req.Language, Locale: req.Locale, Style: req.Style,
		Purpose: purpose, UserText: userText, Territory: req.Territory, Speed: req.Speed, Pitch: req.Pitch,
	})
	if err != nil {
		h.writeVoicePlanError(w, r, req.VoiceID, email, err)
		return
	}

	// Private purposes (a personal confession) are cached per owner so two
	// users never share - or discover - each other's renders.
	visibility, owner, hash := "catalog", "", plan.ContentHash
	if userText || purpose == voicegov.PurposeConfession {
		visibility, owner = "private", userID
		sum := sha256.Sum256([]byte(plan.ContentHash + "|" + userID))
		hash = hex.EncodeToString(sum[:])
	}
	prio := voiceengine.Priority(req.Priority)
	if !isVoiceAdmin(r) || prio == "" {
		prio = voiceengine.PriorityInteractive
	}
	textSum := sha256.Sum256([]byte(voiceengine.PlainText(mustParse(req.Text))))
	gen, created, err := h.vplat.CreateOrGetGeneration(ctx, &store.Generation{
		ContentHash: hash, VoiceID: req.VoiceID, ModelID: plan.Model.ID, Engine: string(plan.Model.Engine),
		EngineVersion: plan.Model.EngineVersion, Purpose: string(purpose), Style: req.Style, Language: req.Language,
		Locale: req.Locale, TextSHA256: hex.EncodeToString(textSum[:]), GrantVersion: plan.Decision.GrantVersion,
		Queue: voiceengine.QueueFor(prio), OwnerUserID: owner, Visibility: visibility, RequestedBy: email,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to record generation")
		return
	}
	if gen.Status == string(voiceengine.GenCompleted) {
		h.writeGeneration(w, r, gen, http.StatusOK)
		return
	}
	if created {
		jobID, err := h.queue.Enqueue(ctx, jobs.Job{
			Type: JobVoiceGenerate, IdempotencyKey: gen.ID + ":" + gen.UpdatedAt, MaxAttempts: 4,
			Payload: map[string]any{
				"generation_id": gen.ID, "voice_id": req.VoiceID, "text": req.Text, "language": req.Language,
				"locale": req.Locale, "style": req.Style, "purpose": string(purpose), "user_text": userText,
				"territory": req.Territory, "speed": req.Speed, "pitch": req.Pitch, "queue": gen.Queue,
			},
		})
		if err != nil && !errors.Is(err, jobs.ErrDuplicateJob) {
			_ = h.vplat.FailGeneration(ctx, gen.ID, string(voiceengine.ClassTransient), "enqueue failed")
			httpx.WriteError(w, http.StatusServiceUnavailable, "could not queue generation")
			return
		}
		_ = h.vplat.SetGenerationJob(ctx, gen.ID, jobID)
		gen.JobID = jobID
		_ = h.vplat.AppendRightsAudit(ctx, store.RightsAuditEntry{VoiceID: req.VoiceID, Actor: email, Action: "VOICE_GENERATION_QUEUED",
			ModelID: plan.Model.ID, GenerationID: gen.ID, GrantVersion: plan.Decision.GrantVersion, Decision: "allowed", RemoteAddr: clientIP(r)})
	}
	h.writeGeneration(w, r, gen, http.StatusAccepted)
}

// decodeVoiceRequest parses and validates a generate/stream body, including
// the script safety floor. It writes the error response itself.
func (h *Handler) decodeVoiceRequest(w http.ResponseWriter, r *http.Request, email string) (generateVoiceRequest, voicegov.ContentPurpose, bool, bool) {
	var req generateVoiceRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return req, "", false, false
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
		return req, "", false, false
	}
	if !voiceengine.ValidStyle(req.Style) {
		httpx.WriteError(w, http.StatusBadRequest, "unknown style")
		return req, "", false, false
	}
	if req.Speed != 0 && (req.Speed < 0.5 || req.Speed > 1.5) {
		httpx.WriteError(w, http.StatusBadRequest, "speed must be between 0.5 and 1.5")
		return req, "", false, false
	}
	if req.Pitch < -6 || req.Pitch > 6 {
		httpx.WriteError(w, http.StatusBadRequest, "pitch must be between -6 and 6 semitones")
		return req, "", false, false
	}
	// Anyone who is not a voice/content admin is supplying their own words.
	// That needs its own licence capability and the stricter safety floor.
	userText := !isVoiceAdmin(r)
	if err := voiceengine.ValidateScript(req.Text, userText); err != nil {
		var v *voiceengine.SafetyViolation
		if errors.As(err, &v) {
			_ = h.vplat.AppendRightsAudit(r.Context(), store.RightsAuditEntry{VoiceID: req.VoiceID, Actor: email,
				Action: "GENERATION_REFUSED_CONTENT", Decision: "denied", Reason: v.Code, RemoteAddr: clientIP(r)})
			httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": v.Detail, "code": v.Code})
			return req, "", false, false
		}
	}

	return req, purpose, userText, true
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
	orch, err := h.orchestratorWithDictionary(ctx)
	if err != nil {
		return nil, err
	}
	return orch.Resolve(ctx, grant, models, refs, req)
}

// orchestratorWithDictionary returns a per-call copy of the orchestrator with
// the current pronunciation dictionary. The shared orchestrator is never
// mutated, so concurrent requests and workers cannot race on it, and a
// dictionary edit takes effect on the very next render.
func (h *Handler) orchestratorWithDictionary(ctx context.Context) (*voiceengine.Orchestrator, error) {
	entries, ver, err := h.vplat.Pronunciations(ctx)
	if err != nil {
		return nil, err
	}
	o := *h.vorch
	o.Dict, o.DictVer = voiceengine.NewDictionary(entries), ver
	return &o, nil
}

func (h *Handler) writeVoicePlanError(w http.ResponseWriter, r *http.Request, voiceID, actor string, err error) {
	var rd *voiceengine.ErrRightsDenied
	switch {
	case errors.Is(err, store.ErrVoiceNotFound):
		httpx.WriteError(w, http.StatusNotFound, "voice not found")
	case errors.As(err, &rd):
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
	case voiceengine.Classify(err) == voiceengine.ClassModel:
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "this voice is not ready for generation", "code": "voice_not_ready"})
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "failed to plan generation")
	}
}

func (h *Handler) writeGeneration(w http.ResponseWriter, r *http.Request, g *store.Generation, status int) {
	body := map[string]any{"generation": g, "synthetic": true,
		"disclosure": "AI-generated using an authorized synthetic voice."}
	if g.Status == string(voiceengine.GenCompleted) && g.StorageKey != "" && h.signer != nil {
		ttl := 4 * time.Hour
		if g.Visibility == "private" {
			ttl = 15 * time.Minute
		}
		if url, err := h.signer.GenerateSignedURL(r.Context(), g.StorageKey, ttl); err == nil {
			body["audioUrl"], body["expiresAt"] = url, time.Now().Add(ttl).UTC().Format(time.RFC3339)
		}
	}
	httpx.WriteJSON(w, status, body)
}

// canSeeGeneration enforces private-by-default (§35): private renders are
// visible only to their owner and voice admins.
func canSeeGeneration(r *http.Request, g *store.Generation) bool {
	if g.Visibility != "private" {
		return true
	}
	uid, _ := actorOf(r)
	return (uid != "" && uid == g.OwnerUserID) || isVoiceAdmin(r)
}

func (h *Handler) getAudioJob(w http.ResponseWriter, r *http.Request) {
	g, err := h.vplat.GenerationByID(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load job")
		return
	}
	// Not-found and not-yours are indistinguishable so ids cannot be probed.
	if g == nil || !canSeeGeneration(r, g) {
		httpx.WriteError(w, http.StatusNotFound, "job not found")
		return
	}
	h.writeGeneration(w, r, g, http.StatusOK)
}

func (h *Handler) cancelAudioJob(w http.ResponseWriter, r *http.Request) {
	g, err := h.vplat.GenerationByID(r.Context(), r.PathValue("id"))
	if err != nil || g == nil || !canSeeGeneration(r, g) {
		httpx.WriteError(w, http.StatusNotFound, "job not found")
		return
	}
	ok, err := h.vplat.CancelGeneration(r.Context(), g.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to cancel")
		return
	}
	if !ok {
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{"error": "job can no longer be cancelled", "status": g.Status})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"generationId": g.ID, "status": voiceengine.GenCancelled})
}

// ---------------------------------------------------------------- worker

// runVoiceGeneration executes one queued render. Errors are classified so the
// queue retries infrastructure faults and parks everything else (§71).
func (h *Handler) runVoiceGeneration(ctx context.Context, p map[string]any) error {
	str := func(k string) string { s, _ := p[k].(string); return s }
	num := func(k string) float64 { f, _ := p[k].(float64); return f }
	genID := str("generation_id")
	g, err := h.vplat.GenerationByID(ctx, genID)
	if err != nil {
		return err
	}
	if g == nil || voiceengine.GenerationStatus(g.Status).Terminal() {
		return nil // cancelled or already done; nothing to do
	}
	if h.vorch == nil {
		return jobs.Permanent(errors.New("voice engine not configured"))
	}
	fail := func(class voiceengine.ErrorClass, err error) error {
		_ = h.vplat.FailGeneration(ctx, genID, string(class), err.Error())
		if class.Retryable() {
			return err
		}
		return jobs.Permanent(err)
	}
	if err := h.vplat.AdvanceGeneration(ctx, genID, voiceengine.GenProcessing); err != nil {
		return nil // cancelled between claim and start
	}
	userText, _ := p["user_text"].(bool)
	req := voiceengine.Request{GenerationID: genID, VoiceID: str("voice_id"), Markup: str("text"), Language: str("language"),
		Locale: str("locale"), Style: str("style"), Purpose: voicegov.ContentPurpose(str("purpose")), UserText: userText,
		Territory: str("territory"), Speed: num("speed"), Pitch: num("pitch")}

	// Fresh rights, models and references: never the enqueue-time snapshot.
	grant, err := h.vplat.Grant(ctx, req.VoiceID)
	if err != nil {
		return err
	}
	models, err := h.vplat.Models(ctx, req.VoiceID)
	if err != nil {
		return err
	}
	refs, err := h.vplat.References(ctx, req.VoiceID)
	if err != nil {
		return err
	}
	orch, err := h.orchestratorWithDictionary(ctx)
	if err != nil {
		return err
	}
	plan, res, err := orch.Generate(ctx, grant, models, refs, req)
	if err != nil {
		var rd *voiceengine.ErrRightsDenied
		if errors.As(err, &rd) {
			_ = h.vplat.AppendRightsAudit(ctx, store.RightsAuditEntry{VoiceID: req.VoiceID, Actor: "worker", Action: "GENERATION_REFUSED_RIGHTS",
				GenerationID: genID, GrantVersion: rd.Decision.GrantVersion, Decision: "denied", Reason: string(rd.Decision.Reason)})
			return fail(voiceengine.ClassRights, err)
		}
		return fail(voiceengine.Classify(err), err)
	}
	// Cancelled while the GPU was busy: discard the result rather than
	// uploading an asset nobody wants.
	if cur, _ := h.vplat.GenerationByID(ctx, genID); cur != nil && cur.Status == string(voiceengine.GenCancelled) {
		return nil
	}
	_ = h.vplat.AdvanceGeneration(ctx, genID, voiceengine.GenUploading)
	if h.signer == nil {
		return fail(voiceengine.ClassStorage, errors.New("no object storage configured"))
	}
	sum := sha256.Sum256(res.Audio)
	audioHash := hex.EncodeToString(sum[:])
	key := fmt.Sprintf("audio/voices/%s/%s/%s/%s.wav", req.VoiceID, req.Purpose, plan.Model.ModelVersion, g.ContentHash)
	meta := map[string]string{
		"synthetic_audio": "true", "voice_id": req.VoiceID, "model_id": plan.Model.ID, "generation_id": genID,
		"engine": string(res.Engine), "engine_version": res.EngineVersion, "audio_sha256": audioHash,
		"style": req.Style, "language": req.Language, "fell_back": fmt.Sprint(plan.FellBack),
	}
	if err := h.signer.Upload(ctx, key, res.Audio, meta); err != nil {
		return fail(voiceengine.ClassStorage, err)
	}
	if err := h.vplat.CompleteGeneration(ctx, &store.Generation{ID: genID, ModelID: plan.Model.ID, Engine: string(res.Engine),
		EngineVersion: res.EngineVersion, AudioSHA256: audioHash, StorageKey: key, DurationMS: res.DurationMS,
		FellBack: plan.FellBack, FallbackReason: plan.FallbackReason, GrantVersion: plan.Decision.GrantVersion}); err != nil {
		return err
	}
	action := "VOICE_GENERATED"
	if plan.FellBack {
		action = "VOICE_GENERATED_FALLBACK"
	}
	return h.vplat.AppendRightsAudit(ctx, store.RightsAuditEntry{VoiceID: req.VoiceID, Actor: "worker", Action: action,
		ModelID: plan.Model.ID, GenerationID: genID, GrantVersion: plan.Decision.GrantVersion, Decision: "allowed", Detail: plan.FallbackReason})
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
	m := &voiceengine.Model{VoiceID: id, Engine: voiceengine.Engine(req.Engine), EngineVersion: req.EngineVersion,
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
	in := voiceeval.GateInput{Candidate: score, RightsAllowed: rightsOK, HumanApproved: req.HumanApproved, BlindEvalCompleted: req.BlindEvalCompleted}
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
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"evaluationId": evalID, "score": score, "gate": gate})
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
		h.route(mux, "POST "+pfx+"/voices/generate", "user", "voice", "Queue or fetch cached synthetic minister speech", authed,
			func(w http.ResponseWriter, r *http.Request) {
				h.idempotencyMiddleware(http.HandlerFunc(h.generateMinisterVoice)).ServeHTTP(w, r)
			})
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
		h.route(mux, "GET "+pfx+"/admin/voices/{id}/audit", vm, "admin-voice", "Voice rights audit log", voiceMgr, h.adminVoiceAudit)
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/references", am, "admin-voice", "Add a reference clip", audioMgr, h.adminAddVoiceReference)
		h.route(mux, "GET "+pfx+"/admin/voices/{id}/models", am, "admin-voice", "List model versions", audioMgr, h.adminListVoiceModels)
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/models", am, "admin-voice", "Register an immutable model version", audioMgr, h.adminRegisterVoiceModel)
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/evaluate", am, "admin-voice", "Record metrics, compute ICF_VOICE_SCORE, run the gate", audioMgr, h.adminEvaluateVoiceModel)
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/models/{modelId}/promote", vm, "admin-voice", "Promote an approved model to production", voiceMgr, h.adminPromoteVoiceModel)
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/rollback", vm, "admin-voice", "Roll back to the previous production model", voiceMgr, h.adminRollbackVoiceModel)
		h.route(mux, "PUT "+pfx+"/admin/pronunciations", am, "admin-voice", "Create or update a pronunciation entry", audioMgr, h.adminUpsertPronunciation)
	}
}
