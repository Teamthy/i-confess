package storage

import (
	"context"
	"errors"
	"strings"
	"time"
)

// VoiceRenderPrefix contains governed masters AND their delivery variants,
// including older keys. The key does not say who owns it or grant playback.
const VoiceRenderPrefix = "audio/voices/"

var errWrongMediaScope = errors.New("object is not in this media namespace")

// GeneralMediaKey is for sources outside the governed render pipeline. Use
// it at import as well as at delivery: previously saved references are not
// proof that somebody authorized re-use of a listener's recording.
func GeneralMediaKey(key string) bool {
	return ValidKey(key) && !strings.HasPrefix(key, VoiceRenderPrefix)
}

// VoiceSourceKey accepts worker-readable sources, never synthesized renders.
func VoiceSourceKey(key string) bool {
	return GeneralMediaKey(key) && (strings.HasPrefix(key, "voice-private/") || strings.HasPrefix(key, "audio/"))
}

// VoiceRenderKey identifies a governed object, not an access grant to it.
func VoiceRenderKey(key string) bool {
	return ValidKey(key) && strings.HasPrefix(key, VoiceRenderPrefix)
}

// GeneralMedia and VoiceRenders expose disjoint views of the same private
// bucket. Only code with generation ownership and rights checks receives the
// latter. Keeping the views at the storage boundary also protects future sign,
// inspect, export or cleanup callers; it does not replace bucket IAM.
func GeneralMedia(s ObjectStorage) ObjectStorage { return scopedObjects(s, false) }
func VoiceRenders(s ObjectStorage) ObjectStorage { return scopedObjects(s, true) }

func scopedObjects(s ObjectStorage, renders bool) ObjectStorage {
	if s == nil {
		return nil
	}
	return objectScope{store: s, renders: renders}
}

type objectScope struct {
	store   ObjectStorage
	renders bool
}

func (s objectScope) accepts(key string) bool {
	if s.renders {
		return VoiceRenderKey(key)
	}
	return GeneralMediaKey(key)
}

func (s objectScope) Upload(ctx context.Context, key string, data []byte, meta map[string]string) error {
	if !s.accepts(key) {
		return errWrongMediaScope
	}
	return s.store.Upload(ctx, key, data, meta)
}

func (s objectScope) Download(ctx context.Context, key string) ([]byte, error) {
	if !s.accepts(key) {
		return nil, errWrongMediaScope
	}
	return s.store.Download(ctx, key)
}

func (s objectScope) Delete(ctx context.Context, key string) error {
	if !s.accepts(key) {
		return errWrongMediaScope
	}
	return s.store.Delete(ctx, key)
}

func (s objectScope) GenerateSignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if !s.accepts(key) {
		return "", errWrongMediaScope
	}
	return s.store.GenerateSignedURL(ctx, key, ttl)
}

func (s objectScope) Exists(ctx context.Context, key string) (bool, error) {
	if !s.accepts(key) {
		return false, nil
	}
	return s.store.Exists(ctx, key)
}

func (s objectScope) GetSize(ctx context.Context, key string) (int64, error) {
	if !s.accepts(key) {
		return 0, errWrongMediaScope
	}
	return s.store.GetSize(ctx, key)
}

func (s objectScope) GetMetadata(ctx context.Context, key string) (map[string]string, error) {
	if !s.accepts(key) {
		return nil, errWrongMediaScope
	}
	return s.store.GetMetadata(ctx, key)
}

func (s objectScope) List(ctx context.Context, prefix string) ([]string, error) {
	keys, err := s.store.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	var visible []string
	for _, k := range keys {
		if s.accepts(k) {
			visible = append(visible, k)
		}
	}
	return visible, nil
}
