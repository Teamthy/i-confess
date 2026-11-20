package voiceengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/voicegov"
)

// ---------------------------------------------------------------------------
// Model registry semantics (§6, §20, §61)
// ---------------------------------------------------------------------------

// ModelStatus is a model version's lifecycle state.
type ModelStatus string

const (
	ModelCandidate  ModelStatus = "candidate"
	ModelEvaluating ModelStatus = "evaluating"
	ModelApproved   ModelStatus = "approved"   // passed the gate; eligible for promotion or fallback
	ModelProduction ModelStatus = "production" // the one serving model for its voice
	ModelRetired    ModelStatus = "retired"    // was production; kept for rollback
	ModelRejected   ModelStatus = "rejected"
)

// Model is one immutable, versioned production asset.
type Model struct {
	ID            string `json:"id"`
	VoiceID       string `json:"voice_id"`
	Engine        Engine `json:"engine"`
	EngineVersion string `json:"engine_version"`
	ModelVersion  string `json:"model_version"`
	// Mode is "zero_shot" (reference-conditioned base model) or "fine_tuned".
	Mode           string      `json:"mode"`
	DatasetVersion string      `json:"dataset_version,omitempty"`
	TrainingRunID  string      `json:"training_run_id,omitempty"`
	CheckpointURI  string      `json:"-"`
	Status         ModelStatus `json:"status"`
	ICFVoiceScore  float64     `json:"icf_voice_score"`
	// FallbackApproved must be set explicitly by a reviewer who listened to
	// this model against the production one. Being "approved" is not enough to
	// be substituted silently (§69).
	FallbackApproved bool `json:"fallback_approved"`
	// LicenseReviewed means the engine checkpoint's licence has been verified
	// in MODEL_LICENSES.md for this deployment.
	LicenseReviewed bool      `json:"license_reviewed"`
	PromotedAt      time.Time `json:"promoted_at,omitempty"`
}

// Promote returns the status changes that make target the production model
// for its voice. The previous production model is retired (never deleted or
// overwritten) so it remains available for rollback. Fine-tuned models must
// cite their exact dataset version (§62).
func Promote(models []Model, targetID string) (map[string]ModelStatus, error) {
	var target *Model
	for i := range models {
		if models[i].ID == targetID {
			target = &models[i]
		}
	}
	if target == nil {
		return nil, fmt.Errorf("model %s not found", targetID)
	}
	switch target.Status {
	case ModelApproved, ModelRetired:
	default:
		return nil, fmt.Errorf("model %s is %s; only approved or retired (rollback) models can be promoted", targetID, target.Status)
	}
	pin := WorkerIdentity{target.Engine, target.EngineVersion}
	if !pin.Valid() {
		return nil, fmt.Errorf("model %s: missing or invalid checkpoint pin", targetID)
	}
	if !target.LicenseReviewed {
		return nil, fmt.Errorf("model %s: engine licence has not been reviewed", targetID)
	}
	if target.Mode == "fine_tuned" && (target.DatasetVersion == "" || target.TrainingRunID == "") {
		return nil, fmt.Errorf("model %s: fine-tuned models must reference a dataset version and training run", targetID)
	}
	changes := map[string]ModelStatus{target.ID: ModelProduction}
	for _, m := range models {
		if m.VoiceID == target.VoiceID && m.Status == ModelProduction && m.ID != target.ID {
			changes[m.ID] = ModelRetired
		}
	}
	return changes, nil
}

// Rollback promotes the most recently retired model for voiceID.
func Rollback(models []Model, voiceID string) (map[string]ModelStatus, error) {
	var retired []Model
	for _, m := range models {
		if m.VoiceID == voiceID && m.Status == ModelRetired {
			retired = append(retired, m)
		}
	}
	if len(retired) == 0 {
		return nil, fmt.Errorf("voice %s has no retired model to roll back to", voiceID)
	}
	sort.Slice(retired, func(i, j int) bool { return retired[i].PromotedAt.After(retired[j].PromotedAt) })
	return Promote(models, retired[0].ID)
}

// ---------------------------------------------------------------------------
// Registry & fallback (§69)
// ---------------------------------------------------------------------------

// Registry maps engines to providers.
type Registry struct {
	providers  map[Engine]VoiceProvider
	production bool
}

// NewRegistry builds a registry. In production mode the fake engine and the
// VoiceStudio development engine are refused.
func NewRegistry(production bool) *Registry {
	return &Registry{providers: map[Engine]VoiceProvider{}, production: production}
}

// Register adds a provider.
func (r *Registry) Register(e Engine, p VoiceProvider) error {
	e = e.Canonical()
	if r.production && (e == EngineFake || e == EngineDevTone || e == EngineVoiceStudio) {
		return fmt.Errorf("engine %s is not permitted in production", e)
	}
	r.providers[e] = p
	return nil
}

// Provider returns the provider for e.
func (r *Registry) Provider(e Engine) (VoiceProvider, bool) {
	p, ok := r.providers[e.Canonical()]
	return p, ok
}

// FallbackPolicy bounds automatic substitution.
type FallbackPolicy struct {
	Enabled bool
	// MinScore is the lowest ICF_VOICE_SCORE a fallback model may have.
	MinScore float64
	// MaxScoreDrop is the largest drop from the production model's score.
	MaxScoreDrop float64
}

// Candidates returns production first, then eligible fallbacks in score
// order. A fallback must be: the same voice; approved and explicitly
// fallback-approved; licence-reviewed; within the quality bounds; and on an
// engine that declares the language. Different voices are never candidates.
func Candidates(models []Model, voiceID string, policy FallbackPolicy) (primaryModel *Model, fallbacks []Model, err error) {
	for i := range models {
		if models[i].VoiceID == voiceID && models[i].Status == ModelProduction {
			primaryModel = &models[i]
		}
	}
	if primaryModel == nil {
		return nil, nil, &Error{Class: ClassModel, Msg: "voice has no production model"}
	}
	if !policy.Enabled {
		return primaryModel, nil, nil
	}
	for _, m := range models {
		if m.VoiceID != voiceID || m.ID == primaryModel.ID || m.Status != ModelApproved ||
			!m.FallbackApproved || !m.LicenseReviewed || m.Engine.Canonical() == primaryModel.Engine.Canonical() {
			continue
		}
		if m.ICFVoiceScore < policy.MinScore {
			continue
		}
		if policy.MaxScoreDrop > 0 && primaryModel.ICFVoiceScore-m.ICFVoiceScore > policy.MaxScoreDrop {
			continue
		}
		fallbacks = append(fallbacks, m)
	}
	sort.SliceStable(fallbacks, func(i, j int) bool { return fallbacks[i].ICFVoiceScore > fallbacks[j].ICFVoiceScore })
	return primaryModel, fallbacks, nil
}

// ---------------------------------------------------------------------------
// Content hash & cache key (§29, §85)
// ---------------------------------------------------------------------------

// AudioPipelineVersion bumps whenever mastering changes audibly, so old
// cached assets are not served as if produced by the new chain.
const AudioPipelineVersion = "m1"

// HashInput is every field that changes the rendered audio.
type HashInput struct {
	// The same words in the same voice are not the same submission across
	// licensed uses. In particular, an editorial batch must never reuse a
	// staff member's personal confession (even with the same cache owner).
	Purpose       voicegov.ContentPurpose `json:"purpose"`
	UserText      bool                    `json:"user_text"`
	VoiceID       string                  `json:"voice_id"`
	ModelID       string                  `json:"model_id"`
	ModelVersion  string                  `json:"model_version"`
	Engine        Engine                  `json:"engine"`
	EngineVersion string                  `json:"engine_version"`
	Text          string                  `json:"text"` // canonical markup source
	// TextNormalised is what the normaliser made of the source, and
	// NormaliseVersion names the rule set that produced it. Both are hashed so
	// a change in the reading rules re-renders affected text instead of
	// serving audio that says something else (§VE-004).
	TextNormalised   string  `json:"text_normalised"`
	NormaliseVersion string  `json:"normalise_version"`
	Language         string  `json:"language"`
	Locale           string  `json:"locale"`
	Style            string  `json:"style"`
	Speed            float64 `json:"speed"`
	Pitch            float64 `json:"pitch"`
	ReferenceID      string  `json:"reference_id"`
	DictVersion      string  `json:"dict_version"`
	AudioVersion     string  `json:"audio_version"`
}

// ContentHash is SHA-256 over the canonical JSON of in. Text is
// whitespace-normalised so trivial reformatting does not bust the cache.
func ContentHash(in HashInput) string {
	in.Text = strings.Join(strings.Fields(in.Text), " ")
	in.Language = strings.ToLower(in.Language)
	if in.AudioVersion == "" {
		in.AudioVersion = AudioPipelineVersion
	}
	b, _ := json.Marshal(in) // struct field order is fixed, so this is canonical
	// Pre-verification cache rows may contain bytes from an unknown worker.
	// Never return one as a hit for a pinned request, even if its labels match.
	sum := sha256.Sum256(append([]byte("icf:verified-render:v1\x00"), b...))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Queues (§24)
// ---------------------------------------------------------------------------

// Queue names.
const (
	QueueTTSHigh       = "tts.high"
	QueueTTSNormal     = "tts.normal"
	QueueTTSLow        = "tts.low"
	QueueMastering     = "audio.mastering"
	QueueTranscription = "audio.transcription"
	QueueDataset       = "audio.dataset"
	QueueEvaluation    = "audio.evaluation"
)

// Priority is a request's urgency.
type Priority string

const (
	PriorityInteractive Priority = "interactive" // a user is waiting
	PriorityNormal      Priority = "normal"
	PriorityBatch       Priority = "batch" // bulk Bible, pre-generation
)

// QueueFor routes a TTS request.
func QueueFor(p Priority) string {
	switch p {
	case PriorityInteractive:
		return QueueTTSHigh
	case PriorityBatch:
		return QueueTTSLow
	}
	return QueueTTSNormal
}

// ---------------------------------------------------------------------------
// Orchestrator
// ---------------------------------------------------------------------------

// Request is an engine-neutral generation request after content validation.
type Request struct {
	// CacheOwnerID isolates private render keys. QueuedContentHash is the
	// persisted, owner-scoped primary plan; a delivery may not silently replan
	// after a promotion, dictionary edit, or change to the reference.
	CacheOwnerID      string
	QueuedContentHash string
	GenerationID      string
	VoiceID           string
	Markup            string
	Language          string
	Locale            string
	Style             string
	Purpose           voicegov.ContentPurpose
	UserText          bool
	Territory         string
	Speed             float64
	Pitch             float64
	At                time.Time
}

// Plan is the resolved, rights-checked generation.
type Plan struct {
	ContentHash string
	Model       Model
	Reference   *Reference
	Request     GenerateRequest
	Decision    voicegov.Decision // initial decision, not an authorization for later handoffs
	Use         voicegov.RenderUse
	FellBack    bool
	// FallbackReason records why the production model was not used; it is
	// written to the audit log and asset metadata so a substitution is never
	// silent.
	FallbackReason string
}

// ErrRightsDenied wraps a voicegov refusal.
type ErrRightsDenied struct{ Decision voicegov.Decision }

func (e *ErrRightsDenied) Error() string {
	return fmt.Sprintf("voice rights denied (%s): %s", e.Decision.Reason, e.Decision.Detail)
}

// Orchestrator resolves and executes generations. It holds no state beyond
// its dependencies; the caller supplies the grant, models and references
// fresh for every call so revocation takes effect immediately (§67).
type Orchestrator struct {
	// ConfirmGrant, installed by the governed API, checks that the decision
	// is still current immediately before a provider receives text. A fallback
	// after a slow primary is a new handoff, not covered by an earlier check.
	ConfirmGrant func(context.Context, voicegov.RenderUse) error
	Registry     *Registry
	Dict         *Dictionary
	DictVer      string
	Fallback     FallbackPolicy
}

func (o *Orchestrator) confirm(ctx context.Context, p *Plan) error {
	if o.ConfirmGrant == nil {
		return nil
	}
	return o.ConfirmGrant(ctx, p.Use)
}

// Resolve performs every check that does not need a GPU - markup parsing,
// production-model selection, the live rights decision, reference selection
// and the content hash - and returns the plan for the production model. The
// API uses it to answer "is this allowed, and is it already cached?" before
// enqueueing anything.
func (o *Orchestrator) Resolve(ctx context.Context, grant *voicegov.Grant, models []Model, refs []Reference, req Request) (*Plan, error) {
	segs, err := o.parse(&req)
	if err != nil {
		return nil, err
	}
	if err := preflightRights(grant, req); err != nil {
		return nil, err
	}
	primaryModel, _, err := Candidates(models, req.VoiceID, FallbackPolicy{})
	if err != nil {
		return nil, err
	}
	p, err := o.plan(ctx, grant, *primaryModel, refs, req, segs)
	if err != nil {
		return nil, err
	}
	if err := o.confirm(ctx, p.Plan); err != nil {
		return nil, err
	}
	return p.Plan, nil
}

// Generate runs: rights → model selection → reference → markup render →
// provider (with bounded, audited fallback). Rights are re-evaluated for each
// candidate because a third-party engine needs an extra capability.
func (o *Orchestrator) Generate(ctx context.Context, grant *voicegov.Grant, models []Model, refs []Reference, req Request) (*Plan, *GenerateResult, error) {
	segs, err := o.parse(&req)
	if err != nil {
		return nil, nil, err
	}
	if err := preflightRights(grant, req); err != nil {
		return nil, nil, err
	}
	primaryModel, fallbacks, err := Candidates(models, req.VoiceID, o.Fallback)
	if err != nil {
		return nil, nil, err
	}
	candidates := append([]Model{*primaryModel}, fallbacks...)

	// A busy primary may recover even if the last fallback lacks the
	// requested language or model. Preserve a retryable cause in that case;
	// explicit rights/content refusals still short-circuit below.
	var cause error
	remember := func(err error) {
		if cause == nil || !Classify(cause).Retryable() {
			cause = err
		}
	}
	var reasons []string
	for i, m := range candidates {
		rp, err := o.plan(ctx, grant, m, refs, req, segs)
		if err != nil {
			var rd *ErrRightsDenied
			if errors.As(err, &rd) && i == 0 {
				// The voice itself is not licensed for this; no engine
				// substitution can change that.
				return nil, nil, err
			}
			if errors.As(err, &rd) {
				reasons = append(reasons, fmt.Sprintf("%s: %s", m.Engine, rd.Decision.Reason))
				continue
			}
			class := Classify(err)
			if class == ClassContent || class == ClassRights || class == ClassPermanent || class == ClassIdentity {
				return nil, nil, err
			}
			reasons = append(reasons, fmt.Sprintf("%s: %s", m.Engine, class))
			remember(err)
			continue
		}
		plan := rp.Plan
		plan.FellBack = i > 0
		if i > 0 {
			plan.FallbackReason = strings.Join(reasons, "; ")
		}
		if err := o.confirm(ctx, plan); err != nil {
			return nil, nil, err
		}
		res, err := rp.provider.Generate(ctx, plan.Request)
		if err == nil {
			var actual WorkerIdentity
			if res != nil {
				actual = WorkerIdentity{res.Engine, res.EngineVersion}
			}
			if err := plan.Request.ExpectedWorker.verify(actual); err != nil {
				return nil, nil, err
			}
			res.Engine = actual.Engine.Canonical()
			return plan, res, nil
		}
		remember(err)
		class := Classify(err)
		reasons = append(reasons, fmt.Sprintf("%s: %s", m.Engine, class))
		// Only infrastructure faults justify trying another engine. Content
		// and rights faults would recur on every engine.
		if class == ClassContent || class == ClassRights || class == ClassPermanent || class == ClassIdentity {
			return plan, nil, err
		}
	}
	if cause == nil {
		cause = &Error{Class: ClassModel, Msg: "no eligible engine"}
	}
	return nil, nil, fmt.Errorf("all engines failed (%s): %w", strings.Join(reasons, "; "), cause)
}

func (o *Orchestrator) parse(req *Request) ([]Segment, error) {
	segs, err := ParseMarkup(req.Markup)
	if err != nil {
		return nil, &Error{Class: ClassContent, Msg: err.Error()}
	}
	// Normalisation runs here, once per request, so every path that parses -
	// resolve, generate and stream - reads a reference, a date, a price or a
	// year identically, and no engine's own frontend gets to decide (§VE-004).
	// It runs before the dictionary: a respelling is about one word and must
	// still win for the term it names.
	segs = NormaliseSegments(segs, req.Language, req.Locale)
	if PlainText(segs) == "" {
		return nil, &Error{Class: ClassContent, Msg: "no speakable text"}
	}
	if req.Style == "" {
		req.Style = "neutral"
	}
	return segs, nil
}

type resolvedPlan struct {
	*Plan
	provider  VoiceProvider
	streaming bool
}

func (o *Orchestrator) plan(ctx context.Context, grant *voicegov.Grant, m Model, refs []Reference, req Request, segs []Segment) (*resolvedPlan, error) {
	m.Engine = m.Engine.Canonical()
	pin := WorkerIdentity{m.Engine, m.EngineVersion}
	if !m.LicenseReviewed || !pin.Valid() {
		return nil, &Error{Class: ClassIdentity, Engine: m.Engine, Msg: "model lacks a reviewed, pinned revision"}
	}
	var ref *Reference
	var refID string
	if m.Mode != "fine_tuned" {
		var err error
		if ref, err = SelectReference(refs, req.VoiceID, req.Style); err != nil {
			if m.Status == ModelProduction && req.QueuedContentHash != "" {
				return nil, &Error{Class: ClassIdentity, Engine: m.Engine, Msg: "queued reference is no longer available"}
			}
			return nil, err
		}
		refID = ref.ID
	}
	prosody := ProsodyFor(req.Style, req.Speed)
	hash := ContentHash(HashInput{
		Purpose: req.Purpose, UserText: req.UserText,
		VoiceID: req.VoiceID, ModelID: m.ID, ModelVersion: m.ModelVersion, Engine: m.Engine,
		EngineVersion: m.EngineVersion, Text: req.Markup, Language: req.Language, Locale: req.Locale,
		Style: req.Style, Speed: prosody.Speed, Pitch: req.Pitch, ReferenceID: refID, DictVersion: o.DictVer,
		TextNormalised: PlainText(segs), NormaliseVersion: NormaliseVersion,
	})
	if req.CacheOwnerID != "" {
		sum := sha256.Sum256([]byte(hash + "|" + req.CacheOwnerID))
		hash = hex.EncodeToString(sum[:])
	}
	if m.Status == ModelProduction && req.QueuedContentHash != "" && hash != req.QueuedContentHash {
		return nil, &Error{Class: ClassIdentity, Engine: m.Engine, Msg: "queued primary render plan has changed; resubmit"}
	}
	p, ok := o.Registry.Provider(m.Engine)
	if !ok {
		return nil, &Error{Class: ClassModel, Engine: m.Engine, Msg: "no provider registered"}
	}
	caps, err := p.Capabilities(ctx)
	if err != nil {
		return nil, err
	}
	if err := pin.verify(WorkerIdentity{caps.Engine, caps.EngineVersion}); err != nil {
		return nil, err
	}
	if !caps.SupportsLanguage(req.Language) {
		return nil, &Error{Class: ClassModel, Engine: m.Engine, Msg: "language " + req.Language + " unsupported"}
	}
	scope := voicegov.Request{
		Action: voicegov.ActionGenerate, Purpose: req.Purpose, ThirdParty: !caps.SelfHosted,
		UserSubmittedText: req.UserText, Territory: req.Territory, Language: req.Language, At: req.At,
	}
	decision := voicegov.Authorize(grant, scope)
	if !decision.Allowed {
		return nil, &ErrRightsDenied{Decision: decision}
	}
	chunks := Render(o.Dict.Apply(segs, req.Locale, m.Engine, caps), caps, prosody)
	return &resolvedPlan{provider: p, streaming: caps.Streaming, Plan: &Plan{
		Model: m, Reference: ref, ContentHash: hash, Decision: decision,
		Use: voicegov.RenderUse{VoiceID: req.VoiceID, GrantVersion: decision.GrantVersion, Request: scope},
		Request: GenerateRequest{
			ExpectedWorker: pin, GenerationID: req.GenerationID, VoiceID: req.VoiceID, ModelID: m.ID, ModelVersion: m.ModelVersion,
			CheckpointURI: m.CheckpointURI, Chunks: chunks, Language: req.Language, Locale: req.Locale,
			Style: req.Style, Reference: ref, Prosody: prosody, SampleRate: 24000,
		},
	}}, nil
}

// preflightRights evaluates the grant before any model is looked at, so an
// unlicensed voice is refused as a rights matter (and audited as one) rather
// than revealing whether a model exists. Engine-specific capabilities (third-
// party infrastructure) are re-checked per candidate in plan.
func preflightRights(grant *voicegov.Grant, req Request) error {
	d := voicegov.Authorize(grant, voicegov.Request{
		Action: voicegov.ActionGenerate, Purpose: req.Purpose, UserSubmittedText: req.UserText,
		Territory: req.Territory, Language: req.Language, At: req.At,
	})
	if !d.Allowed {
		return &ErrRightsDenied{Decision: d}
	}
	return nil
}

// Stream plans a render with the production model only and opens a streamed
// synthesis. Streaming needs can_stream in addition to generation rights, and
// never falls back: switching engines mid-utterance would be an audible,
// silent voice substitution. The stream is ephemeral (not cached); the
// mastered, cached render still comes from Generate via the queue.
func (o *Orchestrator) Stream(ctx context.Context, grant *voicegov.Grant, models []Model, refs []Reference, req Request) (*Plan, *Stream, error) {
	segs, err := o.parse(&req)
	if err != nil {
		return nil, nil, err
	}
	if err := preflightRights(grant, req); err != nil {
		return nil, nil, err
	}
	if d := voicegov.Authorize(grant, voicegov.Request{Action: voicegov.ActionStream, Purpose: req.Purpose,
		UserSubmittedText: req.UserText, Territory: req.Territory, Language: req.Language, At: req.At}); !d.Allowed {
		return nil, nil, &ErrRightsDenied{Decision: d}
	}
	primary, _, err := Candidates(models, req.VoiceID, FallbackPolicy{})
	if err != nil {
		return nil, nil, err
	}
	rp, err := o.plan(ctx, grant, *primary, refs, req, segs)
	if err != nil {
		return nil, nil, err
	}
	sp, ok := rp.provider.(StreamingProvider)
	if !ok || !rp.streaming {
		return nil, nil, &Error{Class: ClassModel, Engine: primary.Engine, Msg: "engine does not support streaming"}
	}
	rp.Use.Request.Action = voicegov.ActionStream
	if err := o.confirm(ctx, rp.Plan); err != nil {
		return nil, nil, err
	}
	st, err := sp.GenerateStream(ctx, rp.Request)
	if err != nil {
		return nil, nil, err
	}
	var actual WorkerIdentity
	if st != nil {
		actual = WorkerIdentity{st.Engine, st.EngineVersion}
	}
	if err := rp.Request.ExpectedWorker.verify(actual); err != nil {
		if st != nil && st.Body != nil {
			st.Body.Close()
		}
		return nil, nil, err
	}
	st.Engine = actual.Engine.Canonical()
	st.Model = rp.Model
	return rp.Plan, st, nil
}
