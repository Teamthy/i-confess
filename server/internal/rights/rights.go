// Package rights implements voice-rights evaluation (§13).
//
// The governing principle: access to a minister's recordings is NOT consent to
// synthesize their voice. Every synthetic generation must be authorized by an
// explicit, unexpired, unrevoked licence that names the intended use. This
// package is deliberately pure domain logic with no I/O so the rules can be
// exhaustively tested and audited.
package rights

import (
	"fmt"
	"strings"
	"time"
)

// LicenseStatus is the lifecycle state of a voice licence.
type LicenseStatus string

const (
	StatusNone    LicenseStatus = "none"
	StatusPending LicenseStatus = "pending"
	StatusActive  LicenseStatus = "active"
	StatusExpired LicenseStatus = "expired"
	StatusRevoked LicenseStatus = "revoked"
)

// Use is the purpose a caller wants to put a voice to. Each use is authorized
// independently: a licence permitting playback does not imply permission to
// synthesize, and permission to synthesize does not imply marketing use.
type Use string

const (
	// UsePlayback is streaming already-produced audio to a listener.
	UsePlayback Use = "playback"
	// UseSynthesis is generating new audio with a synthetic model of the voice.
	UseSynthesis Use = "synthesis"
	// UseMarketing is promotional use (adverts, trailers, social clips).
	UseMarketing Use = "marketing"
	// UseUserContent is synthesizing a *user's* personal text in this voice,
	// which is the highest-risk use: the platform cannot vet the words in
	// advance, so it demands its own explicit grant.
	UseUserContent Use = "user_content"
)

// License is the rights record attached to a voice. Zero values are restrictive
// by design: an empty License authorizes nothing.
type License struct {
	VoiceID  string
	Owner    string
	Provider string
	Status   LicenseStatus

	Start  *time.Time
	Expiry *time.Time

	// CommercialUse gates any revenue-generating context. Because the product
	// is subscription-funded, essentially all listener playback is commercial.
	CommercialUse bool
	// AIGenerationAllowed gates UseSynthesis. This is the field that must never
	// be inferred from the mere existence of recordings.
	AIGenerationAllowed bool
	// MarketingAllowed gates UseMarketing.
	MarketingAllowed bool
	// UserContentAllowed gates UseUserContent.
	UserContentAllowed bool

	// Territories are ISO 3166-1 alpha-2 codes. Empty means worldwide.
	Territories []string
	// Languages are BCP-47 tags. Empty means all languages.
	Languages []string

	// ProviderVoiceID is the synthesis provider's identifier. Required before
	// any synthesis can be dispatched.
	ProviderVoiceID string

	RevocationTerms string
}

// Request describes the specific thing a caller wants to do.
type Request struct {
	Use       Use
	Territory string // ISO 3166-1 alpha-2; empty skips the territory check
	Language  string // BCP-47; empty skips the language check
	At        time.Time
}

// Decision is the outcome of an evaluation. It is deliberately explicit rather
// than a bare bool so refusals can be surfaced to admins and written to the
// audit log with a machine-readable reason.
type Decision struct {
	Allowed bool
	Reason  Reason
	Detail  string
}

// Reason is a stable, machine-readable refusal code.
type Reason string

const (
	ReasonOK              Reason = ""
	ReasonNoLicense       Reason = "no_license"
	ReasonNotActive       Reason = "license_not_active"
	ReasonNotYetEffective Reason = "license_not_yet_effective"
	ReasonExpired         Reason = "license_expired"
	ReasonRevoked         Reason = "license_revoked"
	ReasonNoCommercial    Reason = "commercial_use_not_granted"
	ReasonNoAIGeneration  Reason = "ai_generation_not_granted"
	ReasonNoMarketing     Reason = "marketing_use_not_granted"
	ReasonNoUserContent   Reason = "user_content_use_not_granted"
	ReasonTerritory       Reason = "territory_not_licensed"
	ReasonLanguage        Reason = "language_not_licensed"
	ReasonNoProviderVoice Reason = "no_provider_voice_id"
	ReasonUnknownUse      Reason = "unknown_use"
)

func deny(r Reason, format string, args ...any) Decision {
	return Decision{Allowed: false, Reason: r, Detail: fmt.Sprintf(format, args...)}
}

// Evaluate decides whether req is authorized by lic.
//
// Evaluation order is intentional: lifecycle validity is checked before
// use-specific grants, so an expired licence reports expiry rather than a
// confusing "not granted" for a permission it nominally carries.
func Evaluate(lic *License, req Request) Decision {
	if lic == nil {
		return deny(ReasonNoLicense, "no licence record exists for this voice")
	}
	at := req.At
	if at.IsZero() {
		at = time.Now().UTC()
	}

	// ---- lifecycle ----
	switch lic.Status {
	case StatusRevoked:
		return deny(ReasonRevoked, "licence was revoked")
	case StatusExpired:
		return deny(ReasonExpired, "licence is marked expired")
	case StatusActive:
		// continue
	case StatusNone, StatusPending, "":
		return deny(ReasonNotActive, "licence status is %q, expected %q", string(lic.Status), StatusActive)
	default:
		return deny(ReasonNotActive, "unrecognised licence status %q", string(lic.Status))
	}

	if lic.Start != nil && at.Before(*lic.Start) {
		return deny(ReasonNotYetEffective, "licence begins %s", lic.Start.UTC().Format(time.RFC3339))
	}
	// Expiry is exclusive: a licence expiring at T is invalid at T.
	if lic.Expiry != nil && !at.Before(*lic.Expiry) {
		return deny(ReasonExpired, "licence expired %s", lic.Expiry.UTC().Format(time.RFC3339))
	}

	// ---- scope ----
	if req.Territory != "" && len(lic.Territories) > 0 && !containsFold(lic.Territories, req.Territory) {
		return deny(ReasonTerritory, "territory %q is outside the licensed territories", req.Territory)
	}
	if req.Language != "" && len(lic.Languages) > 0 && !matchesLanguage(lic.Languages, req.Language) {
		return deny(ReasonLanguage, "language %q is outside the licensed languages", req.Language)
	}

	// ---- use-specific grants ----
	// The platform is subscription-funded, so every listener-facing use is a
	// commercial one and requires the commercial grant.
	if !lic.CommercialUse {
		return deny(ReasonNoCommercial, "licence does not grant commercial use")
	}

	switch req.Use {
	case UsePlayback:
		return Decision{Allowed: true}

	case UseSynthesis:
		if !lic.AIGenerationAllowed {
			return deny(ReasonNoAIGeneration,
				"licence does not permit AI voice generation; holding recordings is not consent to synthesize")
		}
		if strings.TrimSpace(lic.ProviderVoiceID) == "" {
			return deny(ReasonNoProviderVoice, "no provider voice id is configured for this voice")
		}
		return Decision{Allowed: true}

	case UseMarketing:
		if !lic.MarketingAllowed {
			return deny(ReasonNoMarketing, "licence does not permit marketing use")
		}
		return Decision{Allowed: true}

	case UseUserContent:
		// User content is synthesis plus an additional grant, because the
		// platform cannot review the words before they are spoken.
		if !lic.AIGenerationAllowed {
			return deny(ReasonNoAIGeneration, "licence does not permit AI voice generation")
		}
		if !lic.UserContentAllowed {
			return deny(ReasonNoUserContent,
				"licence does not permit speaking user-submitted text in this voice")
		}
		if strings.TrimSpace(lic.ProviderVoiceID) == "" {
			return deny(ReasonNoProviderVoice, "no provider voice id is configured for this voice")
		}
		return Decision{Allowed: true}

	default:
		return deny(ReasonUnknownUse, "unknown use %q", string(req.Use))
	}
}

// CanSynthesize is a convenience wrapper for the pipeline's gate.
func CanSynthesize(lic *License, language string, at time.Time) Decision {
	return Evaluate(lic, Request{Use: UseSynthesis, Language: language, At: at})
}

// ExpiringWithin reports licences that need renewal attention. Admin surfaces
// use this so rights lapse loudly rather than silently breaking generation.
func (l *License) ExpiringWithin(d time.Duration, at time.Time) bool {
	if l == nil || l.Expiry == nil || l.Status != StatusActive {
		return false
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return l.Expiry.After(at) && l.Expiry.Before(at.Add(d))
}

func containsFold(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

// matchesLanguage compares BCP-47 tags on their primary subtag, so a licence
// for "en" covers "en-GB" while a licence for "en-GB" does not cover "en-US".
func matchesLanguage(licensed []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, l := range licensed {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == want {
			return true
		}
		if !strings.Contains(l, "-") && strings.HasPrefix(want, l+"-") {
			return true
		}
	}
	return false
}
