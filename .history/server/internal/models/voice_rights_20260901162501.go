package models

import "time"

// VoiceRights manages authorization and legal metadata for a voice.
// Tracks rights holders, allowed use cases, territorial restrictions, and expiration.
type VoiceRights struct {
	ID                      string `json:"id"`
	VoiceID                 string `json:"voice_id"`
	
	RightsHolder            string `json:"rights_holder"`
	AuthorizationReference  string `json:"authorization_reference,omitempty"`
	
	// Use restrictions
	AllowedUse              string `json:"allowed_use"` // tts | recording | streaming | commercial
	Territories             string `json:"territories"` // GLOBAL or comma-separated ISO codes
	
	// Timeline
	StartDate               string `json:"start_date,omitempty"`
	ExpiryDate              string `json:"expiry_date,omitempty"`
	Status                  string `json:"status"` // pending | active | expired | revoked
	
	// Legacy fields (for backward compatibility with existing code)
	LicenseStatus           string `json:"license_status,omitempty"` // active | pending | expired | revoked
	CommercialUse           bool   `json:"commercial_use"`
	AIGenerationAllowed     bool   `json:"ai_generation_allowed"`
	MarketingAllowed        bool   `json:"marketing_allowed"`
	ExpirationDate          string `json:"expiration_date,omitempty"` // RFC3339
	RevocationTerms         string `json:"revocation_terms,omitempty"`
	Provider                string `json:"provider,omitempty"` // google_cloud_tts, aws_polly, etc
	ProviderVoiceID         string `json:"provider_voice_id,omitempty"`
	Notes                   string `json:"notes,omitempty"`
	
	Metadata                string `json:"metadata,omitempty"`
	
	CreatedAt               string `json:"created_at"`
	UpdatedAt               string `json:"updated_at"`
}

// IsActive checks if voice rights are currently active.
func (vr *VoiceRights) IsActive() bool {
	if vr.Status != "active" && vr.LicenseStatus != "active" {
		return false
	}
	
	// Check expiration date if present
	if vr.ExpiryDate != "" {
		expiryTime, err := time.Parse(time.RFC3339, vr.ExpiryDate)
		if err == nil && time.Now().After(expiryTime) {
			return false
		}
	}
	
	if vr.ExpirationDate != "" {
		expiryTime, err := time.Parse(time.RFC3339, vr.ExpirationDate)
		if err == nil && time.Now().After(expiryTime) {
			return false
		}
	}
	
	return true
}

// CanGenerateAI checks if AI voice generation is permitted.
func (vr *VoiceRights) CanGenerateAI() bool {
	return vr.IsActive() && vr.AIGenerationAllowed && (vr.AllowedUse == "tts" || vr.AllowedUse == "streaming")
}

// CanUseCommercially checks if commercial use is permitted.
func (vr *VoiceRights) CanUseCommercially() bool {
	return vr.IsActive() && (vr.CommercialUse || vr.AllowedUse == "commercial")
}

// CanUseInTerritory checks if the voice can be used in a specific territory.
func (vr *VoiceRights) CanUseInTerritory(countryCode string) bool {
	if !vr.IsActive() {
		return false
	}
	
	territories := vr.Territories
	if territories == "" {
		territories = vr.Territories // fallback
	}
	
	if territories == "GLOBAL" {
		return true
	}
	
	// Parse comma-separated territories
	for _, t := range parseCommaSeparatedList(territories) {
		if t == countryCode {
			return true
		}
	}
	
	return false
}

// Helper function
func parseCommaSeparatedList(s string) []string {
	var result []string
	if s == "" {
		return result
	}
	for _, item := range splitString(s, ',') {
		if trimmed := trimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitString(s string, sep byte) []string {
	var result []string
	var current string
	for _, r := range s {
		if byte(r) == sep {
			result = append(result, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
