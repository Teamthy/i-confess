package api

import (
	"context"
	"path"
	"strings"

	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/voiceengine"
)

// Both the legacy and attempt-specific master paths end in the row's unique
// content hash. Never hand out a different render via a stale object pointer.
func publishedMaster(g *store.Generation) bool {
	return g != nil && g.Status == string(voiceengine.GenCompleted) && g.VoiceID != "" && g.ContentHash != "" &&
		storage.VoiceRenderKey(g.StorageKey) && strings.HasPrefix(g.StorageKey, storage.VoiceRenderPrefix+g.VoiceID+"/") &&
		path.Base(g.StorageKey) == g.ContentHash+".wav"
}

func variantKey(master, format string) (string, bool) {
	if !storage.VoiceRenderKey(master) || !strings.HasSuffix(master, ".wav") {
		return "", false
	}
	var ext string
	switch format {
	case "aac":
		ext = ".m4a"
	case "mp3", "opus":
		ext = "." + format
	default:
		return "", false
	}
	return strings.TrimSuffix(master, ".wav") + ext, true
}

func publishedVariant(g *store.Generation, format, key string) bool {
	if !publishedMaster(g) {
		return false
	}
	expected, ok := variantKey(g.StorageKey, format)
	return ok && key == expected && g.Variants[format].Key == key
}

// A raw key supplied by a reviewer (or saved in an older blind test) grants
// no access to a governed render. A real editorial master/encoding remains
// usable when the source model, voice, published object and live rights match.
// Prepared evaluation recordings retain their existing studio workflow.
func (h *Handler) blindClipStore(ctx context.Context, voiceID, source, key string) (storage.ObjectStorage, bool) {
	if storage.VoiceSourceKey(key) {
		return h.signer, true
	}
	if !storage.VoiceRenderKey(key) {
		return nil, false
	}
	master := strings.TrimSuffix(key, path.Ext(key)) + ".wav"
	g, err := h.vplat.CatalogRenderForReview(ctx, voiceID, source, master)
	if err != nil || !publishedMaster(g) {
		return nil, false
	}
	if key != g.StorageKey {
		format := strings.TrimPrefix(path.Ext(key), ".")
		if format == "m4a" {
			format = "aac"
		}
		if !publishedVariant(g, format, key) {
			return nil, false
		}
	}
	if ok, _ := h.assetServable(ctx, voiceID); !ok {
		return nil, false
	}
	return h.renderStore, true
}
