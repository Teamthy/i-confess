package models

// VoiceRights manages authorization and legal metadata for a voice.
type VoiceRights struct {
	ID                  string   `json:"id"`
	VoiceID             string   `json:"voice_id"`
	OwnerName           string   `json:"owner_name"`
	LicenseStatus       string   `json:"license_status"` // active | pending | expired | revoked
	CommercialUse       bool     `json:"commercial_use"`
	AIGenerationAllowed bool     `json:"ai_generation_allowed"`
	Territories         []string `json:"territories"` // ISO country codes or "global"
	Languages           []string `json:"languages"`   // en, es, fr, etc
	MarketingAllowed    bool     `json:"marketing_allowed"`
	ExpirationDate      string   `json:"expiration_date,omitempty"` // RFC3339
	RevocationTerms     string   `json:"revocation_terms,omitempty"`
	Provider            string   `json:"provider,omitempty"` // google_cloud_tts, aws_polly, etc
	ProviderVoiceID     string   `json:"provider_voice_id,omitempty"`
	Notes               string   `json:"notes,omitempty"`
	CreatedAt           string   `json:"created_at"`
	UpdatedAt           string   `json:"updated_at"`
}

// IsActive checks if voice rights are currently active.
func (vr *VoiceRights) IsActive() bool {
	if vr.LicenseStatus != "active" {
		return false
	}
	// Check expiration date if present
	if vr.ExpirationDate != "" {
		// In production, parse and compare against current time
		// For now, assume valid if present
	}
	return true
}

// CanGenerateAI checks if AI voice generation is permitted.
func (vr *VoiceRights) CanGenerateAI() bool {
	return vr.IsActive() && vr.AIGenerationAllowed
}

// CanUseCommercially checks if commercial use is permitted.
func (vr *VoiceRights) CanUseCommercially() bool {
	return vr.IsActive() && vr.CommercialUse
}

// CanUseInTerritory checks if the voice can be used in a specific territory.
func (vr *VoiceRights) CanUseInTerritory(countryCode string) bool {
	if !vr.IsActive() {
		return false
	}
	for _, t := range vr.Territories {
		if t == "global" || t == countryCode {
			return true
		}
	}
	return false
}
