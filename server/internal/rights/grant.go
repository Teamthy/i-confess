package rights

import (
	"github.com/Teamthy/i-confess/internal/voicegov"
)

// ToGrant expresses a coarse License in the granular voicegov model.
//
// It exists because the platform carried two rights models at once (audit
// VE-001): the coarse four-boolean record the older admin endpoints write, and
// the granular capability grant the voice platform uses. Two models meant two
// answers: a capability revoked in voicegov did not affect the cloud-TTS path,
// which asked rights.Evaluate and heard nothing about cloning, embeddings or
// third-party infrastructure.
//
// From here on voicegov.Authorize is the only decision function on the
// generation path. A voice that has a granular grant is checked against that
// grant - in full, including the capabilities the coarse model never had. A
// voice that predates the granular model is projected into one here, so there
// is exactly one place where a render may be refused, one refusal vocabulary to
// audit, and no path that can quietly answer a different question.
//
// The projection is faithful, not generous. It carries what the coarse record
// already authorized and nothing it does not:
//
//   - Only an active, unexpired licence authorizes anything; the coarse
//     statuses map onto voicegov statuses one for one, and an unexpired
//     EffectiveFrom/Expiry is carried across rather than recomputed.
//   - AIGenerationAllowed becomes can_generate and can_clone - the coarse
//     model's entire opinion about synthesis - plus the content purposes the
//     platform dispatches, because the coarse model does not distinguish
//     purposes and its AI grant has always covered the listener-facing
//     products built on it.
//   - CommercialUse becomes can_commercialize, which the coarse evaluator
//     required for every listener-facing use.
//   - A configured provider voice id means synthesis runs on a hosted
//     provider, so can_use_third_party_infrastructure is carried only when the
//     record actually names a provider voice.
//   - Training, fine-tuning, checkpoint storage, derivative models, streaming,
//     download, distribution, research and post-termination retention are
//     never inferred. The coarse record has no opinion on them, and a
//     projection must not manufacture consent it cannot cite.
func (l *License) ToGrant() *voicegov.Grant {
	if l == nil {
		return nil
	}
	g := &voicegov.Grant{
		VoiceID:       l.VoiceID,
		RightsHolder:  l.Owner,
		Status:        grantStatus(l.Status),
		EffectiveFrom: l.Start,
		ExpiresAt:     l.Expiry,
		Territories:   append([]string(nil), l.Territories...),
		Languages:     append([]string(nil), l.Languages...),
		Capabilities:  map[voicegov.Capability]bool{},
	}
	if !g.Status.Authorizing() {
		// Pending, expired and revoked records authorize nothing. Returning
		// the grant with an empty capability set keeps Authorize's refusal
		// reason about the lifecycle ("grant_revoked") rather than about a
		// missing capability the voice would never have had.
		return g
	}

	if l.AIGenerationAllowed {
		for _, c := range []voicegov.Capability{
			voicegov.CanClone, voicegov.CanGenerate,
			voicegov.CanUseInConfessions, voicegov.CanUseInPrayers,
			voicegov.CanUseInReflections, voicegov.CanUseInBibleAudio,
		} {
			g.Capabilities[c] = true
		}
		if l.ProviderVoiceID != "" {
			g.Capabilities[voicegov.CanUseThirdPartyInfra] = true
		}
		if l.UserContentAllowed {
			g.Capabilities[voicegov.CanUseUserSubmittedText] = true
		}
	}
	if l.MarketingAllowed {
		g.Capabilities[voicegov.CanUseInMarketing] = true
	}
	if l.CommercialUse {
		g.Capabilities[voicegov.CanCommercialize] = true
	}
	return g
}

// grantStatus maps the coarse licence lifecycle onto the granular one.
// Anything unrecognised becomes PENDING, which authorizes nothing: an
// ambiguous record must not be read in the platform's favour.
func grantStatus(s LicenseStatus) voicegov.Status {
	switch s {
	case StatusActive:
		return voicegov.StatusApproved
	case StatusRevoked:
		return voicegov.StatusRevoked
	case StatusExpired:
		return voicegov.StatusExpired
	default:
		return voicegov.StatusPending
	}
}
