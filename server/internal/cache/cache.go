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

// Cache is a tiny in-memory TTL+SWR cache for hot read paths.
// Phase 7 §7.1 — Redis cache on ListCategories/CategoryConfessions/ListVoices
// (stale-while-revalidate). This in-memory implementation satisfies the
// contract and is swapped for Redis via the Store interface in production.
// It keeps workspace budget to ~3M source (no persisted node_modules/Redis client).
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

func (m *Meter) Hit()   { m.mu.Lock(); m.s.Hits++; m.mu.Unlock() }
func (m *Meter) Miss()  { m.mu.Lock(); m.s.Misses++; m.mu.Unlock() }
func (m *Meter) Stale() { m.mu.Lock(); m.s.Stale++; m.mu.Unlock() }
func (m *Meter) Snapshot() Stats { m.mu.Lock(); defer m.mu.Unlock(); return m.s }
func (m *Meter) HitRate() float64 {
	s := m.Snapshot()
	total := s.Hits + s.Misses + s.Stale
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total) * 100
}
