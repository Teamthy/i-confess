// Package voicegov is the granular rights model for licensed synthetic
// minister voices (Voice Platform §2, §4, §67, §68).
//
// internal/rights answers a coarse question - "may this voice be synthesized
// at all?" - with four booleans. That is not enough once recordings are used to
// build a cloned model: permission to *use* a voice is not permission to
// *train* on it, permission to train is not permission to distribute, and
// permission to run on in-house GPUs is not permission to send recordings to a
// third-party inference host. Each of those is a separate clause in a voice
// licence and each is a separate Capability here.
//
// Rules this package enforces:
//
//   - Zero values authorize nothing. A Grant with no capabilities set permits no
//     action, and an unknown status is treated as not approved.
//   - Only APPROVED and RESTRICTED grants authorize anything. RESTRICTED differs
//     from APPROVED in that the Restrictions list may carve out purposes.
//   - Expiry is evaluated at call time, not trusted from the stored status, so a
//     grant that lapsed overnight stops generation even if nobody updated it.
//   - Every Action maps to the full set of Capabilities it needs. Callers ask
//     "may I do X?" and never assemble the capability list themselves, so a new
//     call site cannot forget one.
//
// The package is pure: no database, no HTTP, no clock except the one passed in.
package voicegov

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Status is the lifecycle state of a voice rights grant.
type Status string

const (
	StatusPending     Status = "PENDING"
	StatusUnderReview Status = "UNDER_REVIEW"
	StatusApproved    Status = "APPROVED"
	StatusRestricted  Status = "RESTRICTED"
	StatusExpired     Status = "EXPIRED"
	StatusSuspended   Status = "SUSPENDED"
	StatusRevoked     Status = "REVOKED"
)

// AllStatuses lists every valid status, for validation and schema checks.
var AllStatuses = []Status{
	StatusPending, StatusUnderReview, StatusApproved, StatusRestricted,
	StatusExpired, StatusSuspended, StatusRevoked,
}

// Valid reports whether s is a recognised status.
func (s Status) Valid() bool {
	for _, v := range AllStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// Authorizing reports whether a grant in this status can authorize anything.
func (s Status) Authorizing() bool { return s == StatusApproved || s == StatusRestricted }

// Capability is one independently licensed right.
type Capability string

const (
	CanRecord                 Capability = "can_record"
	CanStoreRecordings        Capability = "can_store_recordings"
	CanProcessRecordings      Capability = "can_process_recordings"
	CanExtractVoice           Capability = "can_extract_voice"
	CanCreateEmbedding        Capability = "can_create_embedding"
	CanClone                  Capability = "can_clone"
	CanTrain                  Capability = "can_train"
	CanFineTune               Capability = "can_fine_tune"
	CanGenerate               Capability = "can_generate"
	CanCommercialize          Capability = "can_commercialize"
	CanDistribute             Capability = "can_distribute"
	CanStream                 Capability = "can_stream"
	CanDownload               Capability = "can_download"
	CanUseInBibleAudio        Capability = "can_use_in_bible_audio"
	CanUseInConfessions       Capability = "can_use_in_confessions"
	CanUseInPrayers           Capability = "can_use_in_prayers"
	CanUseInReflections       Capability = "can_use_in_reflections"
	CanUseInMarketing         Capability = "can_use_in_marketing"
	CanUseForResearch         Capability = "can_use_for_research"
	CanUseThirdPartyInfra     Capability = "can_use_third_party_infrastructure"
	CanStoreModelCheckpoints  Capability = "can_store_model_checkpoints"
	CanCreateDerivativeModels Capability = "can_create_derivative_models"
	CanUseUserSubmittedText   Capability = "can_use_user_submitted_text"
	CanRetainAfterTermination Capability = "can_retain_after_termination"
)

// AllCapabilities lists every capability in a stable order.
var AllCapabilities = []Capability{
	CanRecord, CanStoreRecordings, CanProcessRecordings, CanExtractVoice,
	CanCreateEmbedding, CanClone, CanTrain, CanFineTune, CanGenerate,
	CanCommercialize, CanDistribute, CanStream, CanDownload,
	CanUseInBibleAudio, CanUseInConfessions, CanUseInPrayers,
	CanUseInReflections, CanUseInMarketing, CanUseForResearch,
	CanUseThirdPartyInfra, CanStoreModelCheckpoints, CanCreateDerivativeModels,
	CanUseUserSubmittedText, CanRetainAfterTermination,
}

// ValidCapability reports whether c is a recognised capability.
func ValidCapability(c Capability) bool {
	for _, v := range AllCapabilities {
		if c == v {
			return true
		}
	}
	return false
}

// Action is something the platform wants to do with a voice.
type Action string

const (
	ActionIngestRecording Action = "ingest_recording"
	ActionPrepareDataset  Action = "prepare_dataset"
	ActionCreateReference Action = "create_reference"
	ActionZeroShotClone   Action = "zero_shot_clone"
	ActionTrain           Action = "train"
	ActionFineTune        Action = "fine_tune"
	ActionStoreCheckpoint Action = "store_checkpoint"
	ActionGenerate        Action = "generate"
	ActionStream          Action = "stream"
	ActionDownload        Action = "download"
	ActionEvaluate        Action = "evaluate"
)

// ContentPurpose is the product surface the generated audio is for.
type ContentPurpose string

const (
	PurposeConfession ContentPurpose = "confession"
	PurposePrayer     ContentPurpose = "prayer"
	PurposeReflection ContentPurpose = "reflection"
	PurposeBible      ContentPurpose = "bible"
	PurposeDevotional ContentPurpose = "devotional"
	PurposeMarketing  ContentPurpose = "marketing"
	PurposeResearch   ContentPurpose = "research"
)

// actionCaps are the capabilities every instance of an action needs.
var actionCaps = map[Action][]Capability{
	ActionIngestRecording: {CanStoreRecordings, CanProcessRecordings},
	ActionPrepareDataset:  {CanStoreRecordings, CanProcessRecordings, CanExtractVoice},
	ActionCreateReference: {CanStoreRecordings, CanProcessRecordings, CanExtractVoice, CanCreateEmbedding},
	ActionZeroShotClone:   {CanProcessRecordings, CanExtractVoice, CanCreateEmbedding, CanClone},
	ActionTrain:           {CanProcessRecordings, CanExtractVoice, CanClone, CanTrain, CanStoreModelCheckpoints},
	ActionFineTune:        {CanProcessRecordings, CanExtractVoice, CanClone, CanTrain, CanFineTune, CanStoreModelCheckpoints},
	ActionStoreCheckpoint: {CanStoreModelCheckpoints},
	// Generation requires the voice to have been legitimately cloned: a grant
	// that permits "generate" but not "clone" is internally inconsistent and
	// must not be read generously.
	ActionGenerate: {CanClone, CanGenerate},
	ActionStream:   {CanStream, CanDistribute},
	ActionDownload: {CanDownload, CanDistribute},
	// Evaluation renders audio but never distributes it.
	ActionEvaluate: {CanClone, CanGenerate},
}

// purposeCaps are added on top of the action's own set when audio is produced
// for, or delivered to, a product surface.
var purposeCaps = map[ContentPurpose][]Capability{
	PurposeConfession: {CanUseInConfessions, CanCommercialize},
	PurposePrayer:     {CanUseInPrayers, CanCommercialize},
	PurposeReflection: {CanUseInReflections, CanCommercialize},
	// Devotionals are reflection-shaped content and are licensed as such.
	PurposeDevotional: {CanUseInReflections, CanCommercialize},
	PurposeBible:      {CanUseInBibleAudio, CanCommercialize},
	PurposeMarketing:  {CanUseInMarketing, CanCommercialize},
	PurposeResearch:   {CanUseForResearch},
}

// ValidPurpose reports whether p is recognised.
func ValidPurpose(p ContentPurpose) bool { _, ok := purposeCaps[p]; return ok }

// Grant is the rights record for one voice.
type Grant struct {
	VoiceID       string              `json:"voiceId"`
	RightsHolder  string              `json:"rightsHolder"`
	Status        Status              `json:"status"`
	EffectiveFrom *time.Time          `json:"effectiveFrom,omitempty"`
	ExpiresAt     *time.Time          `json:"expiresAt,omitempty"`
	Capabilities  map[Capability]bool `json:"capabilities"`
	// Restrictions carve purposes out of a RESTRICTED grant, e.g. a licence
	// that allows everything except marketing during a dispute.
	Restrictions []ContentPurpose `json:"restrictions"`
	// Territories are ISO 3166-1 alpha-2 codes. Empty means worldwide.
	Territories []string `json:"territories"`
	// Languages are BCP-47 tags. Empty means all languages.
	Languages []string `json:"languages"`
	// PostTermination says what happens to already-generated assets once the
	// grant stops authorizing.
	PostTermination AssetPolicy `json:"postTermination"`
	// Version increments on every change so audit entries can cite exactly
	// which terms authorized an action.
	Version int `json:"version"`
}

// Has reports whether the grant carries capability c. Nil-safe.
func (g *Grant) Has(c Capability) bool { return g != nil && g.Capabilities[c] }

// Request describes an attempted use.
type Request struct {
	Action  Action
	Purpose ContentPurpose // optional for non-content actions
	// ThirdParty is true when the work runs on infrastructure the platform
	// does not operate (hosted inference APIs, rented GPUs under another
	// party's terms). It adds CanUseThirdPartyInfra.
	ThirdParty bool
	// UserSubmittedText is true when the words were written by an end user
	// rather than reviewed editorial content.
	UserSubmittedText bool
	Territory         string
	Language          string
	At                time.Time
}

// Reason is a stable, machine-readable refusal code.
type Reason string

const (
	ReasonOK                Reason = ""
	ReasonNoGrant           Reason = "no_grant"
	ReasonNotApproved       Reason = "grant_not_approved"
	ReasonSuspended         Reason = "grant_suspended"
	ReasonRevoked           Reason = "grant_revoked"
	ReasonExpired           Reason = "grant_expired"
	ReasonNotYetEffective   Reason = "grant_not_yet_effective"
	ReasonMissingCapability Reason = "capability_not_granted"
	ReasonRestricted        Reason = "purpose_restricted"
	ReasonTerritory         Reason = "territory_not_licensed"
	ReasonLanguage          Reason = "language_not_licensed"
	ReasonUnknownAction     Reason = "unknown_action"
	ReasonUnknownPurpose    Reason = "unknown_purpose"
)

// Decision is the outcome of Authorize.
type Decision struct {
	Allowed bool   `json:"allowed"`
	Reason  Reason `json:"reason,omitempty"`
	Detail  string `json:"detail,omitempty"`
	// Missing lists every capability the grant lacks for this request, so an
	// admin sees the whole gap at once instead of fixing one field at a time.
	Missing []Capability `json:"missing,omitempty"`
	// Required is what the request needed; recorded for the audit trail.
	Required []Capability `json:"required,omitempty"`
	// GrantVersion is the terms version the decision was made against.
	GrantVersion int `json:"grantVersion"`
}

func deny(r Reason, format string, args ...any) Decision {
	return Decision{Reason: r, Detail: fmt.Sprintf(format, args...)}
}

// RequiredCapabilities returns the de-duplicated, sorted capability set a
// request needs. Exported so admin UIs can show "what would this need?".
func RequiredCapabilities(req Request) ([]Capability, error) {
	base, ok := actionCaps[req.Action]
	if !ok {
		return nil, fmt.Errorf("unknown action %q", req.Action)
	}
	set := map[Capability]bool{}
	for _, c := range base {
		set[c] = true
	}
	if req.Purpose != "" {
		pc, ok := purposeCaps[req.Purpose]
		if !ok {
			return nil, fmt.Errorf("unknown purpose %q", req.Purpose)
		}
		for _, c := range pc {
			set[c] = true
		}
	}
	if req.ThirdParty {
		set[CanUseThirdPartyInfra] = true
	}
	if req.UserSubmittedText {
		set[CanUseUserSubmittedText] = true
	}
	out := make([]Capability, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// EffectiveStatus returns the status the grant is really in at time at. A grant
// stored as APPROVED whose expiry has passed is EXPIRED, whatever the column
// says - the stored status is a cache of the truth, not the truth.
func EffectiveStatus(g *Grant, at time.Time) Status {
	if g == nil {
		return ""
	}
	if g.Status.Authorizing() && g.ExpiresAt != nil && !at.Before(*g.ExpiresAt) {
		return StatusExpired
	}
	return g.Status
}

// Authorize decides whether req is permitted by g. It must be called before
// every generation, training run and distribution - never cached from when
// the voice was first uploaded (§67).
func Authorize(g *Grant, req Request) Decision {
	if g == nil {
		return deny(ReasonNoGrant, "no rights grant exists for this voice")
	}
	at := req.At
	if at.IsZero() {
		at = time.Now().UTC()
	}

	required, err := RequiredCapabilities(req)
	if err != nil {
		if _, ok := actionCaps[req.Action]; !ok {
			return deny(ReasonUnknownAction, "%v", err)
		}
		return deny(ReasonUnknownPurpose, "%v", err)
	}

	withVersion := func(d Decision) Decision {
		d.Required = required
		d.GrantVersion = g.Version
		return d
	}

	// ---- lifecycle ----
	switch EffectiveStatus(g, at) {
	case StatusApproved, StatusRestricted:
	case StatusRevoked:
		return withVersion(deny(ReasonRevoked, "voice rights were revoked"))
	case StatusSuspended:
		return withVersion(deny(ReasonSuspended, "voice rights are suspended pending review"))
	case StatusExpired:
		return withVersion(deny(ReasonExpired, "voice rights expired"))
	default:
		return withVersion(deny(ReasonNotApproved, "voice rights status is %q; APPROVED or RESTRICTED is required", g.Status))
	}
	if g.EffectiveFrom != nil && at.Before(*g.EffectiveFrom) {
		return withVersion(deny(ReasonNotYetEffective, "voice rights begin %s", g.EffectiveFrom.UTC().Format(time.RFC3339)))
	}

	// ---- restrictions ----
	if g.Status == StatusRestricted && req.Purpose != "" {
		for _, r := range g.Restrictions {
			if r == req.Purpose {
				return withVersion(deny(ReasonRestricted, "purpose %q is restricted under the current terms", req.Purpose))
			}
		}
	}

	// ---- scope ----
	if req.Territory != "" && len(g.Territories) > 0 && !containsFold(g.Territories, req.Territory) {
		return withVersion(deny(ReasonTerritory, "territory %q is not licensed", req.Territory))
	}
	if req.Language != "" && len(g.Languages) > 0 && !MatchesLanguage(g.Languages, req.Language) {
		return withVersion(deny(ReasonLanguage, "language %q is not licensed", req.Language))
	}

	// ---- capabilities ----
	var missing []Capability
	for _, c := range required {
		if !g.Has(c) {
			missing = append(missing, c)
		}
	}
	if len(missing) > 0 {
		d := withVersion(deny(ReasonMissingCapability, "grant lacks %s", joinCaps(missing)))
		d.Missing = missing
		return d
	}
	return withVersion(Decision{Allowed: true})
}

// AssetPolicy is what happens to already-generated audio when a grant stops
// authorizing (expiry, revocation). It is a licence term, so it is configured
// per grant rather than hardcoded (§68).
type AssetPolicy string

const (
	AssetRetain    AssetPolicy = "retain"    // keep serving existing assets
	AssetArchive   AssetPolicy = "archive"   // keep, stop serving
	AssetUnpublish AssetPolicy = "unpublish" // remove from catalogue, keep for audit
	AssetDelete    AssetPolicy = "delete"    // purge objects
)

// ValidAssetPolicy reports whether p is recognised.
func ValidAssetPolicy(p AssetPolicy) bool {
	switch p {
	case AssetRetain, AssetArchive, AssetUnpublish, AssetDelete:
		return true
	}
	return false
}

// ExistingAssetPolicy returns what to do with assets generated under g at time
// at. While the grant authorizes, assets are retained. Once it does not, the
// configured policy applies - defaulting to the conservative "unpublish" when
// the licence was silent, since serving audio after termination is the
// expensive mistake and restoring it is cheap.
func ExistingAssetPolicy(g *Grant, at time.Time) AssetPolicy {
	if g == nil {
		return AssetUnpublish
	}
	if EffectiveStatus(g, at).Authorizing() {
		return AssetRetain
	}
	if g.Status == StatusSuspended {
		// Suspension is temporary; hide but never delete.
		return AssetArchive
	}
	if g.PostTermination == AssetRetain && !g.Has(CanRetainAfterTermination) {
		// A retain policy the licence does not back with a post-termination
		// right is a data-entry error, not a permission.
		return AssetUnpublish
	}
	if ValidAssetPolicy(g.PostTermination) {
		return g.PostTermination
	}
	return AssetUnpublish
}

// transitions lists legal status changes. Anything not listed is refused so a
// revoked licence cannot be quietly flipped back to APPROVED by a PUT; that
// requires a new grant (and therefore a new document and review).
var transitions = map[Status][]Status{
	StatusPending:     {StatusUnderReview, StatusRevoked},
	StatusUnderReview: {StatusApproved, StatusRestricted, StatusPending, StatusRevoked},
	StatusApproved:    {StatusRestricted, StatusSuspended, StatusExpired, StatusRevoked},
	StatusRestricted:  {StatusApproved, StatusSuspended, StatusExpired, StatusRevoked},
	StatusSuspended:   {StatusApproved, StatusRestricted, StatusRevoked, StatusExpired},
	StatusExpired:     {StatusUnderReview, StatusRevoked},
	StatusRevoked:     {},
}

// CanTransition reports whether from -> to is a legal lifecycle change.
func CanTransition(from, to Status) bool {
	for _, s := range transitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// MatchesLanguage compares BCP-47 tags on their primary subtag, so a licence
// for "en" covers "en-NG" while one for "en-NG" does not cover "en-US".
func MatchesLanguage(licensed []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, l := range licensed {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == want || (!strings.Contains(l, "-") && strings.HasPrefix(want, l+"-")) {
			return true
		}
	}
	return false
}

func containsFold(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func joinCaps(cs []Capability) string {
	s := make([]string, len(cs))
	for i, c := range cs {
		s[i] = string(c)
	}
	return strings.Join(s, ", ")
}
