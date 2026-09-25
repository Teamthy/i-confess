package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Teamthy/i-confess/internal/cache"
	"github.com/Teamthy/i-confess/internal/log"
)

// cache_invalidation.go — making the read caches correct when the data changes.
//
// Before this file the three content caches had no writer at all: cache.Cache
// has Invalidate and InvalidatePrefix, and no call site used either. The
// practical effect was not only the cross-instance delay G-10 described, it was
// that an admin who published a confession and then reloaded the catalogue
// still saw the old list for up to ttl+swr — fifteen minutes on categories and
// voices — on the very instance that performed the write.
//
// So an invalidation now happens in two places, in this order:
//
//  1. Locally, synchronously, on the instance handling the write. This is the
//     part that fixes what a human notices.
//  2. On the bus, for the other instances, best-effort. A publish failure is
//     logged, never returned: the write has committed, and the fallback is the
//     TTL, which is what the whole system relied on before.
//
// Nothing here caches less or more than it did. The keys, and therefore the
// blast radius of each invalidation, are the ones PHASE 07 defined.

// Cache keys. They are built here rather than as literals in content_cache.go
// so that the readers and the invalidators cannot drift: a test that publishes
// through these constants and then asserts a fresh read is asserting the same
// strings the read path uses.
const (
	cacheKeyCategories = "categories:published"
	// cacheKeyCategoryConfessionsPrefix covers one key per category
	// ("catconf:<id>"). A confession write invalidates the whole prefix: the
	// message does not carry which category the confession belongs to, and
	// the prefix is at most one entry per category.
	cacheKeyCategoryConfessionsPrefix = "catconf:"
	cacheKeyVoices                    = "voices:all"
)

// SetCacheBus installs the transport that carries invalidations to the other
// API instances.
//
// Without it the Handler is a correct single-instance deployment: its own
// writes still invalidate its own caches. With it, a write on any instance
// invalidates every instance's copy. Production passes a RedisBus when
// REDIS_ADDR is configured; tests pass a MemoryBus to stand up two Handlers
// over one database.
//
// An error means the subscription could not be established, so this instance
// will still serve content another instance has replaced, for up to ttl+swr.
// The caller decides whether that is fatal; main logs it and keeps serving.
func (h *Handler) SetCacheBus(bus cache.Bus) error {
	if bus == nil {
		return errors.New("api: nil cache bus")
	}
	if h.cacheOwner == "" {
		// Identifies this instance's own messages so it does not re-apply
		// them. Random rather than hostname+pid: a container's hostname is
		// reused after a restart, and treating a dead instance's message as
		// our own would silently skip a real invalidation.
		h.cacheOwner = uuid.NewString()
	}
	if err := bus.Subscribe(context.Background(), h.applyInvalidation); err != nil {
		return fmt.Errorf("api: cache invalidation subscription: %w", err)
	}
	h.cacheBus = bus
	return nil
}

// CloseCacheBus stops the subscription and releases the bus. It is safe to call
// more than once and on a Handler that never had a bus.
func (h *Handler) CloseCacheBus() {
	if h.cacheBus == nil {
		return
	}
	if err := h.cacheBus.Close(); err != nil {
		log.Warn("api: closing cache invalidation bus: " + err.Error())
	}
	h.cacheBus = nil
}

// invalidateCache makes a set of cache entries untrue everywhere.
//
// Call it after the write commits, never before: a published message that
// travels faster than the transaction would send other instances to re-read
// the database and cache the value they are about to invalidate.
func (h *Handler) invalidateCache(msgs ...cache.Message) {
	if len(msgs) == 0 {
		return
	}
	for _, msg := range msgs {
		h.applyInvalidation(msg)
	}
	if h.cacheBus == nil {
		return
	}
	for _, msg := range msgs {
		msg.Origin = h.cacheOwner
		msg.Sent = time.Now().UTC()
		if err := h.cacheBus.Publish(context.Background(), msg); err != nil {
			// Degraded, not failed. The write is committed and this instance
			// is already correct; the other instances will fall back to their
			// TTL, which is exactly the behaviour before this existed.
			log.Warn("api: cache invalidation publish failed (other instances will keep serving stale content until their TTL expires): " + err.Error())
		}
	}
}

// applyInvalidation drops one message's keys from this instance's caches. It is
// the Bus subscription handler, so it may be called from another goroutine and
// must not block: every branch is a map delete under a short lock.
func (h *Handler) applyInvalidation(msg cache.Message) {
	if msg.Origin != "" && msg.Origin == h.cacheOwner {
		// Our own write: already applied synchronously by invalidateCache.
		return
	}
	switch {
	case strings.HasPrefix(msg.Key, cacheKeyCategoryConfessionsPrefix):
		if msg.Prefix {
			h.catConfCache.InvalidatePrefix(cacheKeyCategoryConfessionsPrefix)
		} else {
			h.catConfCache.Invalidate(msg.Key)
		}
		h.cacheMeter.Invalidated()
	case msg.Key == cacheKeyCategories:
		h.catCache.Invalidate(cacheKeyCategories)
		h.cacheMeter.Invalidated()
	case msg.Key == cacheKeyVoices:
		h.voicesCache.Invalidate(cacheKeyVoices)
		h.cacheMeter.Invalidated()
	}
}

// invalidationsForCategoryWrite returns the messages a category change needs.
func invalidationsForCategoryWrite() []cache.Message {
	return []cache.Message{{Key: cacheKeyCategories}}
}

// invalidationsForConfessionWrite returns the messages a confession change
// needs. A confession is served inside its category's list, and the message
// does not name the category, so the whole prefix goes.
func invalidationsForConfessionWrite() []cache.Message {
	return []cache.Message{{Key: cacheKeyCategoryConfessionsPrefix, Prefix: true}}
}

// invalidationsForVoiceWrite returns the messages a voice change needs.
func invalidationsForVoiceWrite() []cache.Message {
	return []cache.Message{{Key: cacheKeyVoices}}
}
