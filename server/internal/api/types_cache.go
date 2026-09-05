package api

// CacheStats is the snapshot exposed at /metrics for dashboard §7.1.
type CacheStats struct {
	Hits, Misses, Stale int64
}

func (c CacheStats) HitRate() float64 {
	total := c.Hits + c.Misses + c.Stale
	if total == 0 {
		return 0
	}
	return float64(c.Hits) / float64(total) * 100
}
