package api

import (
	"context"
	"net/http"

	"github.com/Teamthy/i-confess/internal/httpx"
)

// content_cache.go — Phase 7 §7.1 Redis cache (stale-while-revalidate) on
// ListCategories / CategoryConfessions / ListVoices.
// In-memory TTL+SWR keeps workspace ~3M; swap for Redis adapter in prod.

// The content caches live on the Handler, not at package scope.
//
// They were package-level, which meant every Handler in the process shared one
// cache keyed by constants like "categories:published". Two Handlers bound to
// different databases therefore served each other's content, and in tests an
// earlier case warmed the cache with an empty list that a later case then read
// back instead of its own freshly created category. Scoping the caches to the
// Handler makes a Handler serve only the content it is connected to.

func (h *Handler) cachedListCategories(w http.ResponseWriter, r *http.Request) {
	if v, fresh, stale, ok := h.catCache.Get("categories:published"); ok {
		if fresh {
			h.cacheMeter.Hit()
		} else if stale {
			h.cacheMeter.Stale()
			go h.refreshCategories()
		}
		httpx.WriteJSON(w, http.StatusOK, v)
		return
	}
	h.cacheMeter.Miss()
	cats, err := h.cont.ListCategories(r.Context(), false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load categories")
		return
	}
	h.catCache.Set("categories:published", cats)
	httpx.WriteJSON(w, http.StatusOK, cats)
}

func (h *Handler) refreshCategories() {
	cats, err := h.cont.ListCategories(context.Background(), false)
	if err == nil {
		h.catCache.Set("categories:published", cats)
	}
}

func (h *Handler) cachedCategoryConfessions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	key := "catconf:" + id
	if v, fresh, stale, ok := h.catConfCache.Get(key); ok {
		if fresh {
			h.cacheMeter.Hit()
		} else if stale {
			h.cacheMeter.Stale()
			go h.refreshCategoryConfessions(id)
		}
		httpx.WriteJSON(w, http.StatusOK, v)
		return
	}
	h.cacheMeter.Miss()
	confs, err := h.cont.ConfessionsByCategory(r.Context(), id, true)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confessions")
		return
	}
	h.catConfCache.Set(key, confs)
	httpx.WriteJSON(w, http.StatusOK, confs)
}

func (h *Handler) refreshCategoryConfessions(id string) {
	confs, err := h.cont.ConfessionsByCategory(context.Background(), id, true)
	if err == nil {
		h.catConfCache.Set("catconf:"+id, confs)
	}
}

func (h *Handler) cachedListVoices(w http.ResponseWriter, r *http.Request) {
	if v, fresh, stale, ok := h.voicesCache.Get("voices:all"); ok {
		if fresh {
			h.cacheMeter.Hit()
		} else if stale {
			h.cacheMeter.Stale()
			go h.refreshVoices()
		}
		httpx.WriteJSON(w, http.StatusOK, v)
		return
	}
	h.cacheMeter.Miss()
	voices, err := h.audio.ListVoices(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load voices")
		return
	}
	h.voicesCache.Set("voices:all", voices)
	httpx.WriteJSON(w, http.StatusOK, voices)
}

func (h *Handler) refreshVoices() {
	voices, err := h.audio.ListVoices(context.Background())
	if err == nil {
		h.voicesCache.Set("voices:all", voices)
	}
}

// cacheStatsSnapshot exposes this Handler's cache meter for /metrics.
func (h *Handler) cacheStatsSnapshot() CacheStats {
	s := h.cacheMeter.Snapshot()
	return CacheStats{Hits: s.Hits, Misses: s.Misses, Stale: s.Stale}
}
