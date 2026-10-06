package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/voiceengine"
	"github.com/Teamthy/i-confess/internal/voiceeval"
	"github.com/Teamthy/i-confess/internal/voicegov"
)

// Role levels for the voice operations surface (§65). super_admin is always
// admitted by the middleware. Declared levels are enforced by routeAuth, so
// the OpenAPI contract and the real middleware cannot drift apart.
const (
	lvlVoiceRights = "voice_manager"
	lvlVoiceAudio  = "audio_producer,voice_manager"
	lvlVoiceML     = "ml_engineer,voice_manager"
	lvlVoiceRead   = "audio_producer,voice_manager,ml_engineer,auditor"
	lvlVoiceAudit  = "auditor,voice_manager"
	lvlVoiceBatch  = "audio_producer,voice_manager,content_admin"
	lvlVoiceRater  = "audio_producer,voice_manager,ml_engineer,auditor,content_admin"
)

// ---------------------------------------------------------------- asset policy (§67, §68)

// assetServable reports whether an existing render may still be delivered.
// It reads the grant fresh on every call: once rights stop authorizing, the
// licence's post-termination policy decides (retain / archive / unpublish /
// delete) and anything but "retain" withholds the signed URL.
func (h *Handler) assetServable(ctx context.Context, voiceID string) (bool, voicegov.AssetPolicy) {
	g, err := h.vplat.Grant(ctx, voiceID)
	if err != nil {
		return false, voicegov.AssetUnpublish // fail closed
	}
	p := voicegov.ExistingAssetPolicy(g, time.Now().UTC())
	return p == voicegov.AssetRetain, p
}

// SweepVoiceRights flips grants whose expiry passed to EXPIRED and applies a
// "delete" post-termination policy by purging stored renders. Idempotent;
// safe to run from any number of instances.
func (h *Handler) SweepVoiceRights(ctx context.Context) (expired, purged int, err error) {
	due, err := h.vplat.VoicesDueForExpiry(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, id := range due {
		if terr := h.vplat.TransitionGrant(ctx, id, voicegov.StatusExpired, "system", "licence expiry date passed", ""); terr == nil {
			expired++
		}
	}
	voices, err := h.vplat.ListMinisterVoices(ctx, false)
	if err != nil {
		return expired, 0, err
	}
	for _, v := range voices {
		ok, policy := h.assetServable(ctx, v.ID)
		if ok || policy != voicegov.AssetDelete || h.signer == nil {
			continue
		}
		gens, gerr := h.vplat.StoredGenerations(ctx, v.ID, 0)
		if gerr != nil {
			return expired, purged, gerr
		}
		for _, g := range gens {
			// Variants first: if the master delete then fails, the row stays
			// unpurged and the next sweep retries everything.
			variantFailed := false
			for _, v := range g.Variants {
				if derr := h.signer.Delete(ctx, v.Key); derr != nil {
					log.Printf("voice: purge %s variant failed: %v", g.ID, derr)
					variantFailed = true
				}
			}
			if variantFailed {
				continue
			}
			if derr := h.signer.Delete(ctx, g.StorageKey); derr != nil {
				log.Printf("voice: purge %s failed: %v", g.ID, derr)
				continue
			}
			_ = h.vplat.MarkGenerationPurged(ctx, g.ID)
			purged++
		}
		if len(gens) > 0 {
			_ = h.vplat.AppendRightsAudit(ctx, store.RightsAuditEntry{VoiceID: v.ID, Actor: "system", Action: "ASSETS_PURGED",
				Decision: "applied", Reason: string(policy), Detail: strconv.Itoa(len(gens)) + " stored renders deleted under post-termination policy"})
		}
	}
	return expired, purged, nil
}

// StartVoiceRightsSweeper runs SweepVoiceRights on an interval
// (VOICE_RIGHTS_SWEEP_INTERVAL, default 10m; "0" disables).
func (h *Handler) StartVoiceRightsSweeper(ctx context.Context) {
	every := 10 * time.Minute
	if v := os.Getenv("VOICE_RIGHTS_SWEEP_INTERVAL"); v != "" {
		if v == "0" {
			return
		}
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			every = d
		}
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			if e, p, err := h.SweepVoiceRights(ctx); err != nil {
				log.Printf("voice: rights sweep: %v", err)
			} else if e+p > 0 {
				log.Printf("voice: rights sweep expired %d grant(s), purged %d render(s)", e, p)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

func (h *Handler) adminSweepVoiceRights(w http.ResponseWriter, r *http.Request) {
	e, p, err := h.SweepVoiceRights(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "rights sweep failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"expired": e, "purged": p})
}

// ---------------------------------------------------------------- sessions (§33, §34)

// sessionSectionPurpose maps a section to the licence purpose it needs, so a
// scripture section is checked against can_use_in_bible_audio, a prayer
// against can_use_in_prayers, and so on.
func sessionSectionPurpose(section string, fallback voicegov.ContentPurpose) voicegov.ContentPurpose {
	switch section {
	case "SCRIPTURE":
		return voicegov.PurposeBible
	case "PRAYER":
		return voicegov.PurposePrayer
	case "CONFESSION":
		return voicegov.PurposeConfession
	case "REFLECTION":
		return voicegov.PurposeReflection
	}
	return fallback
}

// sessionDefaultStyle is the delivery style for a section when none is given.
func sessionDefaultStyle(section string) string {
	switch section {
	case "SCRIPTURE":
		return "scripture"
	case "PRAYER", "CONFESSION":
		return "prayer"
	case "TEACHING":
		return "teaching"
	case "ENCOURAGEMENT":
		return "encouragement"
	}
	return "reflection"
}

const maxSessionSections = 24

type sessionRequest struct {
	VoiceID  string  `json:"voiceId"`
	Title    string  `json:"title"`
	Purpose  string  `json:"purpose"`
	Language string  `json:"language"`
	Locale   string  `json:"locale"`
	Speed    float64 `json:"speed"`
	Sections []struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Style   string `json:"style"`
		PauseMS int    `json:"pauseMs"`
	} `json:"sections"`
}

func (h *Handler) createAudioSession(w http.ResponseWriter, r *http.Request) {
	if h.vorch == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "voice engine is not configured on this server")
		return
	}
	userID, email := actorOf(r)
	var req sessionRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Sections) == 0 || len(req.Sections) > maxSessionSections {
		httpx.WriteError(w, http.StatusBadRequest, "a session needs between 1 and 24 sections")
		return
	}
	purpose := voicegov.ContentPurpose(req.Purpose)
	if purpose == "" {
		purpose = voicegov.PurposeReflection
	}
	if !voicegov.ValidPurpose(purpose) || purpose == voicegov.PurposeMarketing || purpose == voicegov.PurposeResearch {
		httpx.WriteError(w, http.StatusBadRequest, "purpose must be one of confession, prayer, reflection, devotional, bible")
		return
	}
	if req.Language == "" {
		req.Language = "en"
	}
	if h.limiter != nil {
		if ok, retry := h.limiter.Allow("voicegen:"+userID, VoiceGenerationRule); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			httpx.WriteError(w, http.StatusTooManyRequests, "too many voice requests")
			return
		}
	}
	admin := isVoiceAdmin(r)
	userText := !admin
	sess := &store.AudioSession{OwnerUserID: userID, VoiceID: req.VoiceID, Title: req.Title, Purpose: string(purpose), Visibility: "private"}
	for i, sec := range req.Sections {
		typ := strings.ToUpper(strings.TrimSpace(sec.Type))
		if typ == "PAUSE" {
			if sec.PauseMS <= 0 || sec.PauseMS > 120000 {
				httpx.WriteError(w, http.StatusBadRequest, "a PAUSE section needs pauseMs between 1 and 120000")
				return
			}
			sess.Items = append(sess.Items, store.SessionItem{SectionType: typ, PauseMS: sec.PauseMS})
			continue
		}
		if !validSessionSection(typ) {
			httpx.WriteError(w, http.StatusBadRequest, "section "+strconv.Itoa(i)+": unknown type")
			return
		}
		style := sec.Style
		if style == "" {
			style = sessionDefaultStyle(typ)
		}
		if !voiceengine.ValidStyle(style) {
			httpx.WriteError(w, http.StatusBadRequest, "section "+strconv.Itoa(i)+": unknown style")
			return
		}
		if err := voiceengine.ValidateScript(sec.Text, userText); err != nil {
			var v *voiceengine.SafetyViolation
			if errors.As(err, &v) {
				voiceMetrics.refusedContent()
				_ = h.vplat.AppendRightsAudit(r.Context(), store.RightsAuditEntry{VoiceID: req.VoiceID, Actor: email,
					Action: "GENERATION_REFUSED_CONTENT", Decision: "denied", Reason: v.Code, RemoteAddr: clientIP(r)})
				httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": v.Detail, "code": v.Code, "section": i})
				return
			}
		}
		gen, plan, err := h.queueVoiceGeneration(r.Context(), voiceQueueInput{
			Req: generateVoiceRequest{VoiceID: req.VoiceID, Text: sec.Text, Language: req.Language, Locale: req.Locale,
				Style: style, Purpose: string(sessionSectionPurpose(typ, purpose)), Speed: req.Speed},
			Purpose: sessionSectionPurpose(typ, purpose), UserText: userText, UserID: userID, Actor: email,
			Remote: clientIP(r), Admin: admin,
		})
		if err != nil {
			if plan == nil {
				h.writeVoicePlanError(w, r, req.VoiceID, email, err)
				return
			}
			httpx.WriteError(w, http.StatusServiceUnavailable, "could not queue generation")
			return
		}
		sum := sha256.Sum256([]byte(sec.Text))
		sess.Items = append(sess.Items, store.SessionItem{SectionType: typ, Style: style, GenerationID: gen.ID,
			PauseMS: sec.PauseMS, TextSHA256: hex.EncodeToString(sum[:])})
	}
	if err := h.vplat.CreateAudioSession(r.Context(), sess); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save session")
		return
	}
	full, err := h.vplat.AudioSessionByID(r.Context(), sess.ID)
	if err != nil || full == nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load session")
		return
	}
	h.writeSession(w, r, full, http.StatusAccepted)
}

func validSessionSection(t string) bool {
	switch t {
	case "INTRO", "SCRIPTURE", "REFLECTION", "PRAYER", "CONFESSION", "ENCOURAGEMENT", "TEACHING", "CLOSING":
		return true
	}
	return false
}

func (h *Handler) getAudioSession(w http.ResponseWriter, r *http.Request) {
	sess, err := h.vplat.AudioSessionByID(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load session")
		return
	}
	uid, _ := actorOf(r)
	// Not-found and not-yours look the same so session ids cannot be probed.
	if sess == nil || (sess.OwnerUserID != uid && !isVoiceAdmin(r)) {
		httpx.WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	h.writeSession(w, r, sess, http.StatusOK)
}

// writeSession renders a session as a playlist: each voiced item carries its
// status and, once complete and still servable, a short-lived signed URL.
func (h *Handler) writeSession(w http.ResponseWriter, r *http.Request, s *store.AudioSession, status int) {
	servable, _ := h.assetServable(r.Context(), s.VoiceID)
	type item struct {
		Position int    `json:"position"`
		Type     string `json:"type"`
		Style    string `json:"style,omitempty"`
		PauseMS  int    `json:"pauseMs,omitempty"`
		Status   string `json:"status"`
		AudioURL string `json:"audioUrl,omitempty"`
		Duration int    `json:"durationMs,omitempty"`
		GenID    string `json:"generationId,omitempty"`
	}
	items := make([]item, 0, len(s.Items))
	done, total, totalMS := 0, 0, 0
	for _, it := range s.Items {
		out := item{Position: it.Position, Type: it.SectionType, Style: it.Style, PauseMS: it.PauseMS, GenID: it.GenerationID}
		if it.SectionType == "PAUSE" {
			out.Status, out.Duration = "READY", it.PauseMS
			totalMS += it.PauseMS
			items = append(items, out)
			continue
		}
		total++
		if g := it.Generation; g != nil {
			out.Status, out.Duration = g.Status, g.DurationMS
			if g.Status == string(voiceengine.GenCompleted) {
				done++
				totalMS += g.DurationMS + it.PauseMS
				if servable && g.StorageKey != "" && h.signer != nil {
					if u, err := h.signer.GenerateSignedURL(r.Context(), g.StorageKey, 15*time.Minute); err == nil {
						out.AudioURL = u
					}
				}
			}
		}
		items = append(items, out)
	}
	state := "PROCESSING"
	if done == total {
		state = "READY"
	}
	if !servable {
		state = "UNAVAILABLE"
	}
	httpx.WriteJSON(w, status, map[string]any{
		"session": map[string]any{"id": s.ID, "voiceId": s.VoiceID, "title": s.Title, "purpose": s.Purpose,
			"createdAt": s.CreatedAt, "status": state, "ready": done, "total": total, "durationMs": totalMS, "items": items},
		"synthetic": true, "disclosure": "AI-generated using an authorized synthetic voice.",
	})
}

// ---------------------------------------------------------------- batches (§83, §84)

const maxBatchItems = 10000

func (h *Handler) adminCreateBatch(w http.ResponseWriter, r *http.Request) {
	if h.vorch == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "voice engine is not configured on this server")
		return
	}
	userID, email := actorOf(r)
	voiceID := r.PathValue("id")
	var req struct {
		Title    string `json:"title"`
		Purpose  string `json:"purpose"`
		Style    string `json:"style"`
		Language string `json:"language"`
		Locale   string `json:"locale"`
		Kind     string `json:"kind"`
		Items    []struct {
			Label string `json:"label"`
			Text  string `json:"text"`
		} `json:"items"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Title) == "" || len(req.Items) == 0 || len(req.Items) > maxBatchItems {
		httpx.WriteError(w, http.StatusBadRequest, "a batch needs a title and between 1 and 10000 items")
		return
	}
	purpose := voicegov.ContentPurpose(req.Purpose)
	if !voicegov.ValidPurpose(purpose) || purpose == voicegov.PurposeResearch {
		httpx.WriteError(w, http.StatusBadRequest, "invalid purpose")
		return
	}
	if req.Style == "" {
		req.Style = sessionDefaultStyle(strings.ToUpper(string(purpose)))
	}
	if !voiceengine.ValidStyle(req.Style) {
		httpx.WriteError(w, http.StatusBadRequest, "unknown style")
		return
	}
	if req.Language == "" {
		req.Language = "en"
	}
	if req.Kind == "" {
		req.Kind = "batch"
	}
	if req.Kind != "batch" && req.Kind != "pregeneration" {
		httpx.WriteError(w, http.StatusBadRequest, "kind must be batch or pregeneration")
		return
	}
	// The GPU budget is consulted before a single render is queued (VE-017).
	// The order is the fix: this handler used to queue up to 10,000 renders and
	// record the batch afterwards, so a ceiling checked here would have been a
	// ceiling on money already spent.
	texts := make([]string, len(req.Items))
	for i, it := range req.Items {
		texts[i] = it.Text
	}
	verdict, err := h.checkBatchBudget(r.Context(), voiceID, texts)
	var be *budgetExceeded
	if errors.As(err, &be) {
		h.writeBudgetError(w, r, voiceID, email, be)
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not evaluate the GPU budget")
		return
	}
	b := &store.Batch{VoiceID: voiceID, Title: req.Title, Purpose: string(purpose), Style: req.Style, Kind: req.Kind, CreatedBy: email}
	queued := make([]string, 0, len(texts))
	for i, it := range req.Items {
		bi := store.BatchItem{Position: i, Label: it.Label}
		if err := voiceengine.ValidateScript(it.Text, false); err != nil {
			bi.Error = err.Error()
			b.Items = append(b.Items, bi)
			continue
		}
		gen, plan, err := h.queueVoiceGeneration(r.Context(), voiceQueueInput{
			Req: generateVoiceRequest{VoiceID: voiceID, Text: it.Text, Language: req.Language, Locale: req.Locale,
				Style: req.Style, Purpose: string(purpose)},
			Purpose: purpose, UserID: userID, Actor: email, Remote: clientIP(r), Admin: true, Priority: voiceengine.PriorityBatch,
		})
		if err != nil {
			// A rights refusal applies to every item: stop instead of recording
			// 10,000 identical refusals.
			var rd *voiceengine.ErrRightsDenied
			if plan == nil && (errors.As(err, &rd) || errors.Is(err, store.ErrVoiceNotFound)) {
				h.writeVoicePlanError(w, r, voiceID, email, err)
				return
			}
			bi.Error = err.Error()
			b.Items = append(b.Items, bi)
			continue
		}
		bi.GenerationID = gen.ID
		queued = append(queued, it.Text)
		b.Items = append(b.Items, bi)
	}
	// The recorded estimate covers what was actually queued. The ceiling above
	// was checked against everything the admin asked for - that is the request
	// that needed bounding - but a batch item refused by the safety floor never
	// reaches a GPU, and leaving it in the recorded plan would over-state what
	// this batch was expected to spend.
	est := estimateVoiceCost(queued, verdict.History, verdict.Config)
	if est.Items > 0 {
		b.EstInferenceSeconds, b.EstCostMicros, b.EstimateBasis = &est.InferenceSecs, est.CostMicros, est.Basis
	}
	if err := h.vplat.CreateBatch(r.Context(), b); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to record batch")
		return
	}
	_ = h.vplat.AppendRightsAudit(r.Context(), store.RightsAuditEntry{VoiceID: voiceID, Actor: email, Action: "VOICE_BATCH_QUEUED",
		Decision: "allowed", Detail: b.ID + ": " + strconv.Itoa(b.Total-b.Refused) + " queued, " + strconv.Itoa(b.Refused) +
			" refused; planned " + strconv.FormatFloat(est.InferenceSecs, 'f', 1, 64) + " GPU-seconds (" + est.Basis + ")",
		RemoteAddr: clientIP(r)})
	full, _ := h.vplat.BatchByID(r.Context(), b.ID)
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"batch": full})
}

func (h *Handler) adminListBatches(w http.ResponseWriter, r *http.Request) {
	bs, err := h.vplat.Batches(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to list batches")
		return
	}
	if bs == nil {
		bs = []store.Batch{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"batches": bs})
}

func (h *Handler) adminGetBatch(w http.ResponseWriter, r *http.Request) {
	b, err := h.vplat.BatchByID(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load batch")
		return
	}
	if b == nil {
		httpx.WriteError(w, http.StatusNotFound, "batch not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"batch": b})
}

// ---------------------------------------------------------------- blind evaluation (§50)

func blindMinEvaluators() int {
	if n, err := strconv.Atoi(os.Getenv("VOICE_BLIND_MIN_EVALUATORS")); err == nil && n > 0 {
		return n
	}
	return 3
}

func (h *Handler) adminCreateBlindTest(w http.ResponseWriter, r *http.Request) {
	_, email := actorOf(r)
	voiceID := r.PathValue("id")
	var req struct {
		Title string `json:"title"`
		Clips []struct {
			Source   string `json:"source"` // "REAL" or a model id
			AudioKey string `json:"audioKey"`
		} `json:"clips"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Title) == "" || len(req.Clips) < 2 || len(req.Clips) > 200 {
		httpx.WriteError(w, http.StatusBadRequest, "a blind test needs a title and 2-200 clips")
		return
	}
	if _, err := h.vplat.MinisterVoiceByID(r.Context(), voiceID); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "voice not found")
		return
	}
	models, err := h.vplat.Models(r.Context(), voiceID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load models")
		return
	}
	known := map[string]bool{"REAL": true}
	for _, m := range models {
		known[m.ID] = true
	}
	clips := make([]voiceeval.Clip, 0, len(req.Clips))
	keyBySrcIdx := make([]string, 0, len(req.Clips))
	for i, c := range req.Clips {
		if !known[c.Source] {
			httpx.WriteError(w, http.StatusBadRequest, "clip "+strconv.Itoa(i)+": source must be REAL or one of this voice's model ids")
			return
		}
		if !strings.HasPrefix(c.AudioKey, "voice-private/") && !strings.HasPrefix(c.AudioKey, "audio/") {
			httpx.WriteError(w, http.StatusBadRequest, "clip "+strconv.Itoa(i)+": audioKey must be a voice-private/ or audio/ object")
			return
		}
		// Carry the key through the shuffle in the (never serialised) ID.
		clips = append(clips, voiceeval.Clip{ID: strconv.Itoa(i), Source: c.Source})
		keyBySrcIdx = append(keyBySrcIdx, c.AudioKey)
	}
	blinded, err := voiceeval.Blind(clips)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to blind clips")
		return
	}
	t := &store.BlindTest{VoiceID: voiceID, Title: req.Title, CreatedBy: email, AudioKeys: map[string]string{}}
	for _, c := range blinded {
		idx, _ := strconv.Atoi(c.ID)
		t.AudioKeys[c.BlindLabel] = keyBySrcIdx[idx]
		t.Clips = append(t.Clips, voiceeval.Clip{Source: c.Source, BlindLabel: c.BlindLabel})
	}
	if err := h.vplat.CreateBlindTest(r.Context(), t); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create blind test")
		return
	}
	full, _ := h.vplat.BlindTestByID(r.Context(), t.ID)
	h.writeBlindTest(w, r, full, http.StatusCreated)
}

func (h *Handler) adminListBlindTests(w http.ResponseWriter, r *http.Request) {
	ts, err := h.vplat.BlindTests(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to list blind tests")
		return
	}
	if ts == nil {
		ts = []store.BlindTest{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tests": ts, "minEvaluators": blindMinEvaluators()})
}

func (h *Handler) adminGetBlindTest(w http.ResponseWriter, r *http.Request) {
	t, err := h.vplat.BlindTestByID(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load blind test")
		return
	}
	if t == nil {
		httpx.WriteError(w, http.StatusNotFound, "blind test not found")
		return
	}
	h.writeBlindTest(w, r, t, http.StatusOK)
}

// writeBlindTest shows evaluators only labels and audio. Sources, and this
// evaluator's own earlier ratings, are the only extras; sources appear once
// the test is closed.
func (h *Handler) writeBlindTest(w http.ResponseWriter, r *http.Request, t *store.BlindTest, status int) {
	uid, _ := actorOf(r)
	mine := map[string]map[string]float64{}
	if rs, err := h.vplat.BlindRatings(r.Context(), t.ID); err == nil {
		for _, x := range rs {
			if x.EvaluatorID == uid {
				if mine[x.ClipID] == nil {
					mine[x.ClipID] = map[string]float64{}
				}
				mine[x.ClipID][string(x.Dimension)] = x.Value
			}
		}
	}
	clips := make([]map[string]any, 0, len(t.Clips))
	for _, c := range t.Clips {
		m := map[string]any{"id": c.ID, "label": c.BlindLabel, "myRatings": mine[c.ID]}
		if key := t.AudioKeys[c.ID]; key != "" && h.signer != nil {
			if u, err := h.signer.GenerateSignedURL(r.Context(), key, 30*time.Minute); err == nil {
				m["audioUrl"] = u
			}
		}
		if t.Status == "closed" {
			m["source"] = c.Source
		}
		clips = append(clips, m)
	}
	httpx.WriteJSON(w, status, map[string]any{"test": map[string]any{
		"id": t.ID, "voiceId": t.VoiceID, "title": t.Title, "status": t.Status, "createdAt": t.CreatedAt,
		"closedAt": t.ClosedAt, "evaluators": t.Evaluators, "ratings": t.Ratings, "clips": clips,
		"dimensions": voiceeval.AllDimensions, "minEvaluators": blindMinEvaluators(),
	}})
}

func (h *Handler) adminRateBlindTest(w http.ResponseWriter, r *http.Request) {
	uid, _ := actorOf(r)
	testID := r.PathValue("id")
	var req struct {
		Ratings []struct {
			ClipID    string  `json:"clipId"`
			Dimension string  `json:"dimension"`
			Value     float64 `json:"value"`
			Comments  string  `json:"comments"`
		} `json:"ratings"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || len(req.Ratings) == 0 || len(req.Ratings) > 500 {
		httpx.WriteError(w, http.StatusBadRequest, "send 1-500 ratings")
		return
	}
	valid := map[string]bool{}
	for _, d := range voiceeval.AllDimensions {
		valid[string(d)] = true
	}
	for _, x := range req.Ratings {
		if !valid[x.Dimension] || x.Value < 1 || x.Value > 5 {
			httpx.WriteError(w, http.StatusBadRequest, "each rating needs a known dimension and a value from 1 to 5")
			return
		}
		err := h.vplat.RateBlindClip(r.Context(), testID, voiceeval.Rating{EvaluatorID: uid, ClipID: x.ClipID,
			Dimension: voiceeval.Dimension(x.Dimension), Value: x.Value, Comments: x.Comments})
		if errors.Is(err, store.ErrBlindTestClosed) {
			httpx.WriteError(w, http.StatusConflict, "this blind test is closed")
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"saved": len(req.Ratings)})
}

func (h *Handler) adminCloseBlindTest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := h.vplat.BlindTestByID(r.Context(), id)
	if err != nil || t == nil {
		httpx.WriteError(w, http.StatusNotFound, "blind test not found")
		return
	}
	if err := h.vplat.CloseBlindTest(r.Context(), id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to close test")
		return
	}
	_, email := actorOf(r)
	_ = h.vplat.AppendRightsAudit(r.Context(), store.RightsAuditEntry{VoiceID: t.VoiceID, Actor: email, Action: "BLIND_TEST_CLOSED",
		Decision: "recorded", Detail: id + ": " + strconv.Itoa(t.Evaluators) + " evaluators, " + strconv.Itoa(t.Ratings) + " ratings"})
	h.adminBlindResults(w, r)
}

// blindSummary unblinds a closed test.
func (h *Handler) blindSummary(ctx context.Context, id string) (*store.BlindTest, map[string]*voiceeval.SourceSummary, error) {
	t, err := h.vplat.BlindTestByID(ctx, id)
	if err != nil || t == nil {
		return t, nil, err
	}
	rs, err := h.vplat.BlindRatings(ctx, id)
	if err != nil {
		return t, nil, err
	}
	sum, err := voiceeval.Unblind(t.Clips, rs)
	return t, sum, err
}

func (h *Handler) adminBlindResults(w http.ResponseWriter, r *http.Request) {
	t, sum, err := h.blindSummary(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to summarise test")
		return
	}
	if t == nil {
		httpx.WriteError(w, http.StatusNotFound, "blind test not found")
		return
	}
	if t.Status != "closed" {
		httpx.WriteError(w, http.StatusConflict, "results are hidden until the test is closed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"testId": t.ID, "evaluators": t.Evaluators,
		"minEvaluators": blindMinEvaluators(), "sufficient": t.Evaluators >= blindMinEvaluators(), "bySource": sum})
}

// blindEvidence decides whether a closed blind test counts as completed
// blind evaluation for a model: closed, enough distinct evaluators, and the
// model was actually one of the clips.
func (h *Handler) blindEvidence(ctx context.Context, testID, voiceID, modelID string) (bool, string) {
	t, sum, err := h.blindSummary(ctx, testID)
	switch {
	case err != nil:
		return false, "blind test could not be read"
	case t == nil || t.VoiceID != voiceID:
		return false, "blind test not found for this voice"
	case t.Status != "closed":
		return false, "blind test is still open"
	case t.Evaluators < blindMinEvaluators():
		return false, "blind test has " + strconv.Itoa(t.Evaluators) + " evaluators; " + strconv.Itoa(blindMinEvaluators()) + " required"
	case sum[modelID] == nil:
		return false, "the model was not rated in this blind test"
	}
	return true, ""
}

// ---------------------------------------------------------------- routes

func (h *Handler) registerVoiceOpsRoutes(mux *http.ServeMux) {
	for _, pfx := range []string{"", "/v1"} {
		h.route(mux, "POST "+pfx+"/voice-sessions", AuthUser, "voice", "Create a multi-section audio session (each section rights-checked)", nil, h.createAudioSession)
		h.route(mux, "GET "+pfx+"/voice-sessions/{id}", AuthUser, "voice", "Audio session playlist with signed audio", nil, h.getAudioSession)

		h.route(mux, "POST "+pfx+"/admin/voices/{id}/batches", lvlVoiceBatch, "admin-voice", "Queue a batch or pregeneration run (up to 10,000 items)", nil, h.adminCreateBatch)
		h.route(mux, "GET "+pfx+"/admin/voices/{id}/batches", lvlVoiceBatch, "admin-voice", "List batches with progress", nil, h.adminListBatches)
		h.route(mux, "GET "+pfx+"/admin/batches/{id}", lvlVoiceBatch, "admin-voice", "One batch with per-item status", nil, h.adminGetBatch)

		h.route(mux, "POST "+pfx+"/admin/voices/{id}/blind-tests", lvlVoiceAudio, "admin-voice", "Create a blind listening test (sources hidden)", nil, h.adminCreateBlindTest)
		h.route(mux, "GET "+pfx+"/admin/voices/{id}/blind-tests", lvlVoiceRater, "admin-voice", "List blind tests", nil, h.adminListBlindTests)
		h.route(mux, "GET "+pfx+"/admin/blind-tests/{id}", lvlVoiceRater, "admin-voice", "Blind test clips for an evaluator", nil, h.adminGetBlindTest)
		h.route(mux, "POST "+pfx+"/admin/blind-tests/{id}/ratings", lvlVoiceRater, "admin-voice", "Submit blind ratings (1-5 per dimension)", nil, h.adminRateBlindTest)
		h.route(mux, "POST "+pfx+"/admin/blind-tests/{id}/close", lvlVoiceAudio, "admin-voice", "Close a blind test and reveal results", nil, h.adminCloseBlindTest)
		h.route(mux, "GET "+pfx+"/admin/blind-tests/{id}/results", lvlVoiceRead, "admin-voice", "Unblinded results of a closed test", nil, h.adminBlindResults)

		h.route(mux, "POST "+pfx+"/admin/voice-rights/sweep", lvlVoiceRights, "admin-voice", "Expire lapsed grants and apply post-termination policy now", nil, h.adminSweepVoiceRights)
		h.route(mux, "GET "+pfx+"/admin/voice-metrics", lvlVoiceRead, "admin-voice", "Voice generation metrics (JSON or Prometheus)", nil, h.adminVoiceMetrics)
	}
}

// voiceProduction mirrors config.IsProduction for the few voice rules that
// are stricter in production (e.g. blind evidence instead of attestation).
func voiceProduction() bool {
	e := strings.ToLower(strings.TrimSpace(os.Getenv("ENV")))
	return e == "production" || e == "prod"
}
