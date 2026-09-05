package api

import (
	"context"
	"net/http"
	"time"

	"github.com/Teamthy/i-confess/internal/cache"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
)

// content_cache.go — Phase 7 §7.1 Redis cache (stale-while-revalidate) on
// ListCategories / CategoryConfessions / ListVoices.
// In-memory TTL+SWR keeps workspace ~3M; swap for Redis adapter in prod.

var (
	catCache     = cache.New[[]models.Category](5*time.Minute, 10*time.Minute)
	catConfCache = cache.New[[]models.Confession](2*time.Minute, 5*time.Minute)
	voicesCache  = cache.New[[]models.Voice](5*time.Minute, 10*time.Minute)
	cacheMeter   = &cache.Meter{}
)

func (h *Handler) cachedListCategories(w http.ResponseWriter, r *http.Request) {
	if v, fresh, stale, ok := catCache.Get("categories:published"); ok {
		if fresh {
			cacheMeter.Hit()
		} else if stale {
			cacheMeter.Stale()
			go h.refreshCategories()
		}
		httpx.WriteJSON(w, http.StatusOK, v)
		return
	}
	cacheMeter.Miss()
	cats, err := h.cont.ListCategories(r.Context(), false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load categories")
		return
	}
	catCache.Set("categories:published", cats)
	httpx.WriteJSON(w, http.StatusOK, cats)
}

func (h *Handler) refreshCategories() {
	cats, err := h.cont.ListCategories(context.Background(), false)
	if err == nil {
		catCache.Set("categories:published", cats)
	}
}

func (h *Handler) cachedCategoryConfessions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	key := "catconf:" + id
	if v, fresh, stale, ok := catConfCache.Get(key); ok {
		if fresh {
			cacheMeter.Hit()
		} else if stale {
			cacheMeter.Stale()
			go h.refreshCategoryConfessions(id)
		}
		httpx.WriteJSON(w, http.StatusOK, v)
		return
	}
	cacheMeter.Miss()
	confs, err := h.cont.ConfessionsByCategory(r.Context(), id, true)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confessions")
		return
	}
	catConfCache.Set(key, confs)
	httpx.WriteJSON(w, http.StatusOK, confs)
}

func (h *Handler) refreshCategoryConfessions(id string) {
	confs, err := h.cont.ConfessionsByCategory(context.Background(), id, true)
	if err == nil {
		catConfCache.Set("catconf:"+id, confs)
	}
}

func (h *Handler) cachedListVoices(w http.ResponseWriter, r *http.Request) {
	if v, fresh, stale, ok := voicesCache.Get("voices:all"); ok {
		if fresh {
			cacheMeter.Hit()
		} else if stale {
			cacheMeter.Stale()
			go h.refreshVoices()
		}
		httpx.WriteJSON(w, http.StatusOK, v)
		return
	}
	cacheMeter.Miss()
	voices, err := h.audio.ListVoices(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load voices")
		return
	}
	voicesCache.Set("voices:all", voices)
	httpx.WriteJSON(w, http.StatusOK, voices)
}

func (h *Handler) refreshVoices() {
	voices, err := h.audio.ListVoices(context.Background())
	if err == nil {
		voicesCache.Set("voices:all", voices)
	}
}

// cacheStatsSnapshot exposes meter for /metrics
func cacheStatsSnapshot() CacheStats {
	s := cacheMeter.Snapshot()
	return CacheStats{Hits: s.Hits, Misses: s.Misses, Stale: s.Stale}
}
