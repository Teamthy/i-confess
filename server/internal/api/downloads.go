package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

// Offline downloads (§28, §44).
//
// A download is a time-bounded licence, not a permanent copy. Three things are
// enforced here and nowhere else:
//
//  1. Downloads require entitlement, checked server-side at request time.
//  2. The plan's limit is enforced against live licences, not lifetime total.
//  3. The client receives a signed URL that expires; the storage key never
//     leaves the server, so a lapsed licence cannot be redeemed from a record
//     cached on the device.

// listDownloads returns the caller's offline library.
func (h *Handler) listDownloads(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)

	downloads, err := h.downloads.ListActive(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load downloads")
		return
	}
	ent := h.entitlementsFor(r.Context(), userID)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"downloads": downloads,
		"limit":     ent.DownloadsLimit,
		"used":      len(downloads),
		// Surfaced so the client can warn before content stops working rather
		// than letting it fail silently at 6 AM on a plane.
		"offline_hours_allowed": ent.OfflineHoursAllowed,
	})
}

// createDownload issues an offline licence and a signed URL to fetch the file.
func (h *Handler) createDownload(w http.ResponseWriter, r *http.Request) {
	if h.signer == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "downloads are not configured on this server")
		return
	}

	var req struct {
		ConfessionID string `json:"confession_id"`
		VariantID    string `json:"variant_id"`
		VoiceID      string `json:"voice_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.ConfessionID == "" {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "confession_id is required")
		return
	}

	userID := h.userID(r)
	ent := h.entitlementsFor(r.Context(), userID)

	// Entitlement first, before any lookup work: a free user must not be able
	// to probe which assets exist by watching error codes.
	if !ent.CanDownload {
		httpx.WriteJSON(w, http.StatusPaymentRequired, map[string]any{
			"error":  "offline downloads are available on Premium",
			"code":   "ENTITLEMENT_REQUIRED",
			"reason": "offline_downloads_require_subscription",
			"plan":   ent.Plan,
		})
		return
	}

	// The limit counts live licences, so removing a download frees a slot.
	// Counting lifetime downloads instead would punish someone for managing
	// their storage.
	used, err := h.downloads.ActiveCount(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to check download limit")
		return
	}
	if ent.DownloadsLimit > 0 && used >= ent.DownloadsLimit {
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error": "you have reached your download limit",
			"code":  "DOWNLOAD_LIMIT_REACHED",
			"limit": ent.DownloadsLimit, "used": used,
		})
		return
	}

	// Resolve the asset. The voice falls back to the user's default so a client
	// does not have to know which voices exist.
	voiceID := req.VoiceID
	if voiceID == "" {
		if prefs, perr := h.profiles.Preferences(r.Context(), userID); perr == nil {
			voiceID = prefs.DefaultVoiceID
		}
	}
	asset, err := h.findDownloadableAsset(r, req.ConfessionID, req.VariantID, voiceID)
	if err != nil {
		writeCode(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "no audio available for that confession")
		return
	}

	// Only published audio may be taken offline. QA material must not escape
	// onto a device where it cannot be recalled.
	if asset.Status != "ready" {
		writeCode(w, http.StatusUnprocessableEntity, "AUDIO_NOT_PUBLISHED",
			"that audio is not available for download yet")
		return
	}

	// A premium voice needs the premium entitlement, same as streaming.
	if v, verr := h.audio.VoiceByID(r.Context(), asset.VoiceID); verr == nil && v.Premium {
		if !ent.CanAccessPremiumVoices {
			httpx.WriteJSON(w, http.StatusPaymentRequired, map[string]any{
				"error": "that voice is available on Premium", "code": "ENTITLEMENT_REQUIRED",
			})
			return
		}
	}

	ttl := time.Duration(ent.OfflineHoursAllowed) * time.Hour
	if ttl <= 0 {
		ttl = 168 * time.Hour
	}

	d := &models.Download{
		UserID: userID, AudioAssetID: asset.ID,
		ConfessionID: asset.ConfessionID, VoiceID: asset.VoiceID,
		StorageKey: asset.URL, DurationSeconds: asset.DurationSeconds,
		SizeBytes: asset.SizeBytes, Checksum: asset.Checksum,
	}
	if err := h.downloads.Create(r.Context(), d, ttl); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to record download")
		return
	}

	// The fetch URL is short-lived and separate from the licence: the licence
	// says how long the file may be kept, this link says how long the client
	// has to actually pull the bytes.
	url, err := h.signer.GenerateSignedURL(r.Context(), asset.URL, 1*time.Hour)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to prepare download")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"download":     d,
		"download_url": url,
		"expires_at":   d.ExpiresAt,
		"note":         "This file may be kept offline until the licence expires. Re-open the app before then to renew it.",
	})
}

// refreshDownload re-issues a fetch URL and renews the licence.
//
// Renewal re-checks entitlement, which is how a cancelled subscription
// eventually stops working offline: the licence simply stops being renewed and
// lapses on its own.
func (h *Handler) refreshDownload(w http.ResponseWriter, r *http.Request) {
	if h.signer == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "downloads are not configured")
		return
	}
	userID := h.userID(r)

	d, err := h.downloads.ByID(r.Context(), userID, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "download not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load download")
		return
	}

	ent := h.entitlementsFor(r.Context(), userID)
	if !ent.CanDownload {
		httpx.WriteJSON(w, http.StatusPaymentRequired, map[string]any{
			"error": "offline downloads are available on Premium",
			"code":  "ENTITLEMENT_REQUIRED",
			"note":  "This download will stop working when its licence expires.",
		})
		return
	}

	ttl := time.Duration(ent.OfflineHoursAllowed) * time.Hour
	if ttl <= 0 {
		ttl = 168 * time.Hour
	}
	renewed := &models.Download{
		ID: d.ID, UserID: userID, AudioAssetID: d.AudioAssetID,
		ConfessionID: d.ConfessionID, VoiceID: d.VoiceID,
		StorageKey: d.StorageKey, DurationSeconds: d.DurationSeconds,
		SizeBytes: d.SizeBytes, Checksum: d.Checksum,
	}
	if err := h.downloads.Create(r.Context(), renewed, ttl); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to renew download")
		return
	}

	url, err := h.signer.GenerateSignedURL(r.Context(), d.StorageKey, 1*time.Hour)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to prepare download")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"download": renewed, "download_url": url, "expires_at": renewed.ExpiresAt,
	})
}

// deleteDownload releases a licence, freeing a slot.
func (h *Handler) deleteDownload(w http.ResponseWriter, r *http.Request) {
	err := h.downloads.Remove(r.Context(), h.userID(r), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "download not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to remove download")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "download removed",
		"note":    "Delete the local file on this device to reclaim storage.",
	})
}

// findDownloadableAsset picks the audio for a confession, preferring the
// requested voice and variant.
func (h *Handler) findDownloadableAsset(r *http.Request, confessionID, variantID, voiceID string) (*models.AudioAsset, error) {
	assets, err := h.audio.AssetsFor(r.Context(), confessionID, voiceID)
	if err != nil {
		return nil, err
	}
	if len(assets) == 0 && voiceID != "" {
		// Fall back to any voice rather than refusing: the user asked for the
		// confession, and a different available voice is better than nothing.
		assets, err = h.audio.AssetsFor(r.Context(), confessionID, "")
		if err != nil {
			return nil, err
		}
	}
	if len(assets) == 0 {
		return nil, store.ErrNotFound
	}
	if variantID != "" {
		for i := range assets {
			if assets[i].VariantID == variantID {
				return &assets[i], nil
			}
		}
	}
	return &assets[0], nil
}
