package cache

import (
	"sync"
	"time"
)

// Item holds a cached value with expiry and stale window.
type Item[V any] struct {
	Value      V
	ExpiresAt  time.Time // fresh until
	StaleUntil time.Time // serve stale while revalidating
}

// Cache is a tiny in-memory TTL+SWR cache for hot read paths:
// ListCategories, CategoryConfessions and ListVoices.
//
// It is per-process. There is no Redis-backed implementation and no Store
// interface to swap one in, despite an earlier revision of this comment
// claiming otherwise — the only Redis in the system is the rate limiter in
// internal/ratelimit. That matters once more than one API instance runs: each
// holds its own copy, so an admin edit to a category is invisible to the other
// instances until their TTL expires, for up to ttl+swr (15 minutes on the
// category cache today).
//
// At one instance that is correct and cheap, which is why it is still the right
// choice now. It stops being correct at two, and the fix is either a shared
// cache or explicit invalidation on write. Tracked as G-10 in
// docs/03-TECHNOLOGY-DECISIONS.md.
//
// The earlier comment also justified this implementation by "workspace budget",
// which is a constraint of the tooling that generated the file, not a property
// of the product. Infrastructure choices need product reasons.
type Cache[V any] struct {
	mu   sync.RWMutex
	data map[string]Item[V]
	ttl  time.Duration // fresh
	swr  time.Duration // stale window after ttl
}

func New[V any](ttl, swr time.Duration) *Cache[V] {
	return &Cache[V]{data: make(map[string]Item[V]), ttl: ttl, swr: swr}
}

// Get returns value and whether it was fresh, stale, or miss.
func (c *Cache[V]) Get(key string) (v V, fresh bool, stale bool, ok bool) {
	c.mu.RLock()
	it, ok := c.data[key]
	c.mu.RUnlock()
	if !ok {
		var zero V
		return zero, false, false, false
	}
	now := time.Now()
	if now.Before(it.ExpiresAt) {
		return it.Value, true, false, true
	}
	if now.Before(it.StaleUntil) {
		return it.Value, false, true, true
	}
	var zero V
	return zero, false, false, false
}

func (c *Cache[V]) Set(key string, v V) {
	now := time.Now()
	c.mu.Lock()
	c.data[key] = Item[V]{Value: v, ExpiresAt: now.Add(c.ttl), StaleUntil: now.Add(c.ttl + c.swr)}
	c.mu.Unlock()
}

func (c *Cache[V]) Invalidate(key string) {
	c.mu.Lock()
	delete(c.data, key)
	c.mu.Unlock()
}

func (c *Cache[V]) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	for k := range c.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.data, k)
		}
	}
	c.mu.Unlock()
}

// Stats for dashboard hit-rate.
type Stats struct {
	Hits, Misses, Stale int64
}

type Meter struct {
	mu sync.Mutex
	s  Stats
}

func (m *Meter) Hit()            { m.mu.Lock(); m.s.Hits++; m.mu.Unlock() }
func (m *Meter) Miss()           { m.mu.Lock(); m.s.Misses++; m.mu.Unlock() }
func (m *Meter) Stale()          { m.mu.Lock(); m.s.Stale++; m.mu.Unlock() }
func (m *Meter) Snapshot() Stats { m.mu.Lock(); defer m.mu.Unlock(); return m.s }
func (m *Meter) HitRate() float64 {
	s := m.Snapshot()
	total := s.Hits + s.Misses + s.Stale
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total) * 100
}
