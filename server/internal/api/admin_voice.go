package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/rights"
	"github.com/Teamthy/i-confess/internal/voice"
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

	res, err := h.pipeline.Generate(r.Context(), lic, voice.GenerateRequest{
		ConfessionID: req.ConfessionID, VariantID: req.VariantID, VoiceID: req.VoiceID,
		Language: req.Language, Text: text, Use: rights.UseSynthesis,
		Territory: req.Territory, RequestedBy: actor, Force: req.Force,
	})

	var denied *voice.ErrRightsDenied
	if errors.As(err, &denied) {
		// 451 is the honest status: the request is well-formed and the content
		// exists, but it may not be produced for legal reasons.
		httpx.WriteJSON(w, http.StatusUnavailableForLegalReasons, map[string]any{
			"error":  "voice rights do not permit this generation",
			"reason": string(denied.Decision.Reason),
			"detail": denied.Decision.Detail,
		})
		return
	}
	if err != nil {
		status := http.StatusBadGateway
		if !voice.IsRetryable(err) {
			status = http.StatusUnprocessableEntity
		}
		httpx.WriteError(w, status, "audio generation failed: "+err.Error())
		return
	}

	asset := &models.AudioAsset{
		ConfessionID: req.ConfessionID, VariantID: req.VariantID, VoiceID: req.VoiceID,
		URL: res.Key, SizeBytes: res.SizeBytes, DurationSeconds: res.DurationSeconds,
		// New audio enters QA rather than going straight to listeners (§31).
		Status: "processing",
	}
	if err := h.audio.UpsertAsset(r.Context(), asset); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "audio was generated but could not be recorded")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"asset":    asset,
		"reused":   res.Reused,
		"checksum": res.Checksum,
		"note":     "Asset is in QA. Publish it to make it available to listeners.",
	})
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
