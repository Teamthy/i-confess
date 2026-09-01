package entitlements

import (
	"testing"
	"time"
)

// The core commercial rule: a free listener must not be handed a playable link
// to premium audio, no matter what the client asks for.
func TestFreePlanCannotPlayPremiumVoiceOrContent(t *testing.T) {
	free := FromPlan(PlanFree)

	if d := free.CanPlayAudio(AudioRequest{VoicePremium: true, AssetStatus: "ready"}); d.Allowed {
		t.Fatal("free plan was granted a premium voice")
	} else if d.Reason != AudioPremiumVoice {
		t.Fatalf("reason = %q, want %q", d.Reason, AudioPremiumVoice)
	}

	if d := free.CanPlayAudio(AudioRequest{ContentPremium: true, AssetStatus: "ready"}); d.Allowed {
		t.Fatal("free plan was granted premium content")
	} else if d.Reason != AudioPremiumContent {
		t.Fatalf("reason = %q, want %q", d.Reason, AudioPremiumContent)
	}
}

// Free must remain genuinely useful (§25): standard content still plays.
func TestFreePlanCanPlayStandardContent(t *testing.T) {
	free := FromPlan(PlanFree)
	d := free.CanPlayAudio(AudioRequest{AssetStatus: "ready"})
	if !d.Allowed {
		t.Fatalf("free plan refused standard content: %s", d.Reason)
	}
	if d.TTL != freePlaybackTTL {
		t.Fatalf("ttl = %v, want %v", d.TTL, freePlaybackTTL)
	}
}

func TestPremiumPlanCanPlayEverything(t *testing.T) {
	premium := FromPlan(PlanPremium)
	for _, req := range []AudioRequest{
		{AssetStatus: "ready"},
		{VoicePremium: true, AssetStatus: "ready"},
		{ContentPremium: true, AssetStatus: "ready"},
		{VoicePremium: true, ContentPremium: true, AssetStatus: "ready"},
	} {
		if d := premium.CanPlayAudio(req); !d.Allowed {
			t.Fatalf("premium refused %+v: %s", req, d.Reason)
		}
	}
}

// Unpublished audio must never be served — not to premium listeners, not to
// anyone. QA state is not an entitlement question.
func TestUnpublishedAudioIsRefusedForEveryone(t *testing.T) {
	for _, plan := range []string{PlanFree, PlanPremium} {
		e := FromPlan(plan)
		for _, status := range []string{"processing", "queued", "failed", "archived"} {
			d := e.CanPlayAudio(AudioRequest{AssetStatus: status})
			if d.Allowed {
				t.Fatalf("plan %s was served audio in status %q", plan, status)
			}
			if d.Reason != AudioAssetNotReady {
				t.Fatalf("reason = %q, want %q", d.Reason, AudioAssetNotReady)
			}
		}
	}
}

// A premium refusal must take precedence in a way that produces an accurate
// upgrade prompt, but an unpublished asset is refused before that is even
// considered — an admin must not be told to "upgrade" to hear QA audio.
func TestUnpublishedBeatsPremiumRefusal(t *testing.T) {
	free := FromPlan(PlanFree)
	d := free.CanPlayAudio(AudioRequest{VoicePremium: true, AssetStatus: "processing"})
	if d.Reason != AudioAssetNotReady {
		t.Fatalf("reason = %q, want the publication refusal to win", d.Reason)
	}
}

func TestDownloadRequiresPremiumAndGetsLongerTTL(t *testing.T) {
	free := FromPlan(PlanFree)
	if d := free.CanPlayAudio(AudioRequest{AssetStatus: "ready", ForDownload: true}); d.Allowed {
		t.Fatal("free plan was granted a download")
	}

	premium := FromPlan(PlanPremium)
	d := premium.CanPlayAudio(AudioRequest{AssetStatus: "ready", ForDownload: true})
	if !d.Allowed {
		t.Fatalf("premium refused a download: %s", d.Reason)
	}
	if d.TTL != premiumDownloadTTL {
		t.Fatalf("download ttl = %v, want %v", d.TTL, premiumDownloadTTL)
	}
	// A download link must outlive a playback link, or offline use breaks.
	if d.TTL <= premium.PlaybackTTL() {
		t.Fatal("download ttl should exceed playback ttl")
	}
}

// Free links expire sooner so lapsed entitlement takes effect quickly and a
// leaked URL has a smaller blast radius.
func TestFreeLinksAreShorterLivedThanPremium(t *testing.T) {
	if FromPlan(PlanFree).PlaybackTTL() >= FromPlan(PlanPremium).PlaybackTTL() {
		t.Fatal("free playback links should be shorter-lived than premium")
	}
	if FromPlan(PlanFree).PlaybackTTL() <= 0 {
		t.Fatal("free playback ttl must be positive")
	}
}

func TestMaxSessionSeconds(t *testing.T) {
	free := FromPlan(PlanFree).MaxSessionSeconds()
	premium := FromPlan(PlanPremium).MaxSessionSeconds()

	if free <= 0 || free >= premium {
		t.Fatalf("free=%d premium=%d: free must be positive and below premium", free, premium)
	}
	if premium != 3*3600 {
		t.Fatalf("premium max = %d, want 3h", premium)
	}
	// Free must be genuinely useful, not a teaser.
	if free < 10*60 {
		t.Fatalf("free max of %ds is too short to be useful", free)
	}
}

// An unknown or empty plan must be treated as free, never as premium. Failing
// open here would hand away the entire catalogue.
func TestUnknownPlanFailsClosedToFree(t *testing.T) {
	for _, plan := range []string{"", "enterprise", "PREMIUM", "trial"} {
		e := FromPlan(plan)
		if e.CanAccessPremiumVoices || e.CanAccessPremiumContent || e.CanDownload {
			t.Fatalf("plan %q was granted premium capabilities", plan)
		}
		if d := e.CanPlayAudio(AudioRequest{VoicePremium: true, AssetStatus: "ready"}); d.Allowed {
			t.Fatalf("plan %q was granted a premium voice", plan)
		}
	}
}

func TestTTLsAreSane(t *testing.T) {
	if freePlaybackTTL > time.Hour*2 {
		t.Fatal("free playback links live too long")
	}
	if premiumDownloadTTL > 48*time.Hour {
		t.Fatal("download links live too long")
	}
}
