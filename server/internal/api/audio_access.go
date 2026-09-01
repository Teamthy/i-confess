package api

import (
	"context"
	"log"
	"net/http"

	"github.com/Teamthy/i-confess/internal/entitlements"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/storage"
)

// Entitlement-gated audio delivery (§11, §26, §27, §55).
//
// Two rules are enforced here and nowhere else, so there is exactly one place
// to audit:
//
//  1. Audio bytes never pass through this API. The client receives a
//     short-lived signed URL; the CDN serves the bytes.
//  2. Entitlement is evaluated at URL-minting time, on every read. A plan can
//     lapse between building a session and playing it, so checking once at
//     creation would leave a cancelled subscriber with working links.
//
// The client is never trusted: nothing here reads an `isPremium` flag from the
// request. The plan is loaded from the database for the authenticated user.

// SetSigner installs the object store used to mint audio URLs.
func (h *Handler) SetSigner(s storage.ObjectStorage) { h.signer = s }

// SetMediaHandler installs the development audio origin. Production leaves this
// nil and serves audio from the CDN instead.
func (h *Handler) SetMediaHandler(handler http.Handler) { h.mediaHandler = handler }

// entitlementsFor loads the authenticated user's plan and resolves it to
// capabilities. An unreadable plan degrades to Free rather than to Premium:
// a database hiccup must never hand away the paid catalogue.
func (h *Handler) entitlementsFor(ctx context.Context, userID string) entitlements.Entitlements {
	if userID == "" {
		return entitlements.FromPlan(entitlements.PlanFree)
	}
	plan, err := h.users.Subscription(ctx, userID)
	if err != nil {
		log.Printf("entitlements: failed to load plan for user %s, defaulting to free: %v", userID, err)
		return entitlements.FromPlan(entitlements.PlanFree)
	}
	return entitlements.FromPlan(plan)
}

// audioContext carries the per-item facts an entitlement decision needs.
type audioContext struct {
	voicePremium   bool
	contentPremium bool
	assetStatus    string
}

// signSessionAudio replaces each item's stored storage key with a freshly
// signed, expiring URL — or blanks it when the listener is not entitled.
//
// Blanking rather than erroring is deliberate: a free listener whose session
// contains one premium item should still hear the rest. The player treats an
// item with no URL as unplayable and skips it, and `locked` tells the UI to
// show an upgrade affordance instead of a silent gap.
func (h *Handler) signSessionAudio(ctx context.Context, sess *models.Session, ent entitlements.Entitlements) {
	if sess == nil || h.signer == nil {
		return
	}

	// Voice premium-ness is per-voice, not per-item; cache lookups so a
	// 3-hour session does not issue hundreds of identical queries.
	voiceCache := map[string]bool{}
	isVoicePremium := func(voiceID string) bool {
		if voiceID == "" {
			return false
		}
		if p, ok := voiceCache[voiceID]; ok {
			return p
		}
		premium := false
		if v, err := h.audio.VoiceByID(ctx, voiceID); err == nil {
			premium = v.Premium
		}
		voiceCache[voiceID] = premium
		return premium
	}

	for i := range sess.Items {
		item := &sess.Items[i]
		key := item.AudioURL
		if key == "" {
			continue
		}

		decision := ent.CanPlayAudio(entitlements.AudioRequest{
			VoicePremium: isVoicePremium(item.VoiceID),
			// Asset status defaults to ready here: the session engine only
			// selects published assets. The check still runs so that a future
			// engine change cannot silently start serving QA audio.
			AssetStatus: "ready",
		})
		if !decision.Allowed {
			item.AudioURL = ""
			item.Locked = true
			item.LockReason = string(decision.Reason)
			continue
		}

		signed, err := h.signer.GenerateSignedURL(ctx, key, decision.TTL)
		if err != nil {
			// Blank the URL rather than leak a raw storage key.
			log.Printf("audio: failed to sign key for session %s item %d: %v", sess.ID, i, err)
			item.AudioURL = ""
			continue
		}
		item.AudioURL = signed
	}
}
