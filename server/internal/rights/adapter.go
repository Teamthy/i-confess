package rights

import (
	"strings"
	"time"
)

// Adapter from the persisted models.VoiceRights record to the License value
// this package evaluates.
//
// The stored record carries two overlapping generations of fields (`Status` and
// `LicenseStatus`, `ExpiryDate` and `ExpirationDate`). Rather than let that
// ambiguity leak into the rules, everything is normalised here — once — and the
// evaluator only ever sees a single unambiguous shape.
//
// The normalisation is deliberately pessimistic: where two fields disagree, the
// more restrictive reading wins. A rights record that is unclear must not be
// resolved in the platform's favour.

// Source is the subset of models.VoiceRights this package needs. Declaring it
// as an interface-free struct copy avoids importing models (which would create
// an import cycle) while keeping the mapping explicit and testable.
type Source struct {
	VoiceID                string
	RightsHolder           string
	Provider               string
	AuthorizationReference string

	AllowedUse  string // tts | recording | streaming | commercial
	Territories string // "GLOBAL" or comma-separated ISO codes
	Languages   string // comma-separated BCP-47 tags; empty = all

	Status         string // pending | active | expired | revoked
	LicenseStatus  string // legacy mirror of Status
	StartDate      string
	ExpiryDate     string
	ExpirationDate string // legacy mirror of ExpiryDate

	CommercialUse       bool
	AIGenerationAllowed bool
	MarketingAllowed    bool
	UserContentAllowed  bool
	ProviderVoiceID     string
	RevocationTerms     string
}

// FromSource normalises a persisted rights record into a License.
func FromSource(s *Source) *License {
	if s == nil {
		return nil
	}

	lic := &License{
		VoiceID:             s.VoiceID,
		Owner:               s.RightsHolder,
		Provider:            s.Provider,
		Status:              normaliseStatus(s.Status, s.LicenseStatus),
		CommercialUse:       s.CommercialUse,
		AIGenerationAllowed: s.AIGenerationAllowed,
		MarketingAllowed:    s.MarketingAllowed,
		UserContentAllowed:  s.UserContentAllowed,
		ProviderVoiceID:     s.ProviderVoiceID,
		RevocationTerms:     s.RevocationTerms,
		Territories:         parseTerritories(s.Territories),
		Languages:           splitCSV(s.Languages),
	}

	if t, ok := parseTime(s.StartDate); ok {
		lic.Start = &t
	}
	// Whichever expiry is sooner governs: if the two legacy fields disagree,
	// the platform must lose the argument, not the rights holder.
	if t, ok := earliest(s.ExpiryDate, s.ExpirationDate); ok {
		lic.Expiry = &t
	}

	// AllowedUse narrows synthesis independently of the boolean flag. A record
	// marked "recording" describes permission to play captured audio, which is
	// not permission to run a TTS model.
	switch strings.ToLower(strings.TrimSpace(s.AllowedUse)) {
	case "tts", "streaming", "commercial", "":
		// Synthesis remains governed by AIGenerationAllowed.
	default:
		lic.AIGenerationAllowed = false
		lic.UserContentAllowed = false
	}

	return lic
}

// normaliseStatus reconciles the two status fields. If either says the licence
// is revoked or expired, that wins; both must agree on "active" for the licence
// to be treated as live.
func normaliseStatus(status, legacy string) LicenseStatus {
	a := LicenseStatus(strings.ToLower(strings.TrimSpace(status)))
	b := LicenseStatus(strings.ToLower(strings.TrimSpace(legacy)))

	for _, s := range []LicenseStatus{a, b} {
		if s == StatusRevoked {
			return StatusRevoked
		}
	}
	for _, s := range []LicenseStatus{a, b} {
		if s == StatusExpired {
			return StatusExpired
		}
	}
	// Treat as active only when nothing contradicts it.
	if a == StatusActive && (b == StatusActive || b == "") {
		return StatusActive
	}
	if b == StatusActive && a == "" {
		return StatusActive
	}
	if a != "" {
		return a
	}
	if b != "" {
		return b
	}
	return StatusNone
}

// parseTerritories maps the sentinel "GLOBAL" to an empty list, which the
// evaluator reads as worldwide.
func parseTerritories(s string) []string {
	t := strings.TrimSpace(s)
	if t == "" || strings.EqualFold(t, "GLOBAL") {
		return nil
	}
	return splitCSV(t)
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// earliest returns the soonest of the supplied timestamps.
func earliest(values ...string) (time.Time, bool) {
	var best time.Time
	found := false
	for _, v := range values {
		if t, ok := parseTime(v); ok {
			if !found || t.Before(best) {
				best, found = t, true
			}
		}
	}
	return best, found
}
