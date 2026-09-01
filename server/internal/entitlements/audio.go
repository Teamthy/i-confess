package entitlements

import (
	"fmt"
	"time"
)

// Audio access rules (§26, §27, §55).
//
// The governing principle: the backend is the sole authority on what a listener
// may hear. Nothing here reads a flag sent by a client. A session is composed
// from content the user may not (any longer) be entitled to — their plan can
// lapse between building a session and playing it — so entitlement is
// re-evaluated every time an audio URL is minted, not once at creation.

// AudioDecision is the outcome of an audio-access check. Like the rights
// evaluator, it carries a machine-readable reason so refusals can be counted,
// surfaced in support tooling, and turned into an accurate upgrade prompt.
type AudioDecision struct {
	Allowed bool
	Reason  AudioReason
	Detail  string
	// TTL is how long the resulting signed link may live. Free listeners get
	// shorter links so a leaked URL has a smaller blast radius, and so lapsed
	// entitlement takes effect sooner.
	TTL time.Duration
}

// AudioReason is a stable refusal code.
type AudioReason string

const (
	AudioOK               AudioReason = ""
	AudioPremiumVoice     AudioReason = "premium_voice_requires_subscription"
	AudioPremiumContent   AudioReason = "premium_content_requires_subscription"
	AudioDurationExceeded AudioReason = "session_duration_exceeds_plan_limit"
	AudioAssetNotReady    AudioReason = "audio_not_published"
)

// Signed-link lifetimes. Short enough that a leaked link expires quickly, long
// enough that a listener does not lose playback mid-track on a slow connection.
const (
	freePlaybackTTL    = 1 * time.Hour
	premiumPlaybackTTL = 6 * time.Hour
	// Downloads are entitled for far longer because the file is meant to
	// survive offline (§28), but never indefinitely: an expiring link is what
	// makes a cancelled subscription eventually stop working.
	premiumDownloadTTL = 24 * time.Hour
)

// MaxSessionSeconds is the longest session a plan may play.
//
// Free is genuinely useful (§25) — 15 minutes is a real morning devotional, not
// a teaser — but the long-form sessions that define the product are premium.
func (e Entitlements) MaxSessionSeconds() int {
	if e.Plan == PlanPremium {
		return 3 * 3600
	}
	return 15 * 60
}

// AudioRequest describes one audio-access check.
type AudioRequest struct {
	// VoicePremium is whether the voice is marked premium.
	VoicePremium bool
	// ContentPremium is whether the confession or its category is premium.
	ContentPremium bool
	// AssetStatus is the audio asset's publication status. Only "ready" audio
	// reaches listeners; anything still in QA must never be served (§31).
	AssetStatus string
	// ForDownload requests a longer-lived link for offline use.
	ForDownload bool
}

// CanPlayAudio decides whether a listener may be given a playable link.
func (e Entitlements) CanPlayAudio(req AudioRequest) AudioDecision {
	// Unpublished audio is refused for everyone, including premium listeners
	// and admins. QA state is not an entitlement question.
	if req.AssetStatus != "" && req.AssetStatus != "ready" {
		return AudioDecision{
			Reason: AudioAssetNotReady,
			Detail: fmt.Sprintf("audio status is %q, expected \"ready\"", req.AssetStatus),
		}
	}

	if req.VoicePremium && !e.CanAccessPremiumVoices {
		return AudioDecision{
			Reason: AudioPremiumVoice,
			Detail: "this voice is available on Premium",
		}
	}
	if req.ContentPremium && !e.CanAccessPremiumContent {
		return AudioDecision{
			Reason: AudioPremiumContent,
			Detail: "this confession is available on Premium",
		}
	}

	if req.ForDownload {
		if !e.CanDownload {
			return AudioDecision{
				Reason: AudioPremiumContent,
				Detail: "offline downloads are available on Premium",
			}
		}
		return AudioDecision{Allowed: true, TTL: premiumDownloadTTL}
	}

	return AudioDecision{Allowed: true, TTL: e.PlaybackTTL()}
}

// PlaybackTTL is the signed-link lifetime for this plan.
func (e Entitlements) PlaybackTTL() time.Duration {
	if e.Plan == PlanPremium {
		return premiumPlaybackTTL
	}
	return freePlaybackTTL
}
