package api

// CacheStats is the snapshot exposed at /metrics for dashboard §7.1.
type CacheStats struct {
	Hits, Misses, Stale int64
	// Invalidations is how many entries were dropped because the content
	// changed, on this instance. Zero here on a database that has been edited
	// since boot means writes are not reaching the invalidation path.
	Invalidations int64
}

func (c CacheStats) HitRate() float64 {
	total := c.Hits + c.Misses + c.Stale
	if total == 0 {
		return 0
	}
	return float64(c.Hits) / float64(total) * 100
}
