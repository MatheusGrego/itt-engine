package cache

import (
	"sync"
	"time"
)

// CacheKey uniquely identifies a cached result by MVCC version + query type
type CacheKey struct {
	VersionID uint64 // MVCC version ID (critical for snapshot isolation)
	QueryType string // "full_analysis" | "node_analysis" | "region_analysis"
	QueryArgs string // JSON-encoded args (e.g., nodeID for node_analysis)
}

// CachedEntry wraps any value with expiration metadata
// Generic cache entry - works with any type (use interface{})
type CachedEntry struct {
	Value      interface{} // Cached value (typically *Results)
	ComputedAt time.Time
	ExpiresAt  time.Time
}

// ResultsCache is a thread-safe, MVCC-aware cache for analysis results
// Key invariant: a cached result is valid ONLY for its specific MVCC version
//
// NOTE: Stores interface{} to avoid import cycle with parent package.
// Caller is responsible for type assertion.
type ResultsCache struct {
	mu   sync.RWMutex
	data map[CacheKey]CachedEntry
	ttl  time.Duration

	// Stats (for monitoring)
	hits   uint64
	misses uint64
}

// NewResultsCache creates a cache with the given TTL
// TTL acts as a safety fallback in case GC doesn't invalidate properly
func NewResultsCache(ttl time.Duration) *ResultsCache {
	if ttl <= 0 {
		ttl = 60 * time.Second // default: 1 minute
	}

	return &ResultsCache{
		data: make(map[CacheKey]CachedEntry),
		ttl:  ttl,
	}
}

// Get retrieves a cached value if valid (version match + not expired)
// Returns (value, true) on hit, (nil, false) on miss
// Caller must type assert the returned interface{}
func (c *ResultsCache) Get(key CacheKey) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, ok := c.data[key]
	if !ok {
		c.misses++
		return nil, false
	}

	// Check TTL expiration (safety mechanism)
	if time.Now().After(cached.ExpiresAt) {
		c.misses++
		return nil, false
	}

	c.hits++
	return cached.Value, true
}

// Set stores a value with MVCC version tag and TTL expiration
func (c *ResultsCache) Set(key CacheKey, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.data[key] = CachedEntry{
		Value:      value,
		ComputedAt: now,
		ExpiresAt:  now.Add(c.ttl),
	}
}

// InvalidateVersion removes ALL cache entries for a specific MVCC version
// Called by MVCC GC when a version is garbage collected
// Returns number of entries invalidated
func (c *ResultsCache) InvalidateVersion(versionID uint64) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for key := range c.data {
		if key.VersionID == versionID {
			delete(c.data, key)
			count++
		}
	}

	return count
}

// EvictExpired removes all TTL-expired entries
// Should be called periodically by a background goroutine
// Returns number of entries evicted
func (c *ResultsCache) EvictExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	count := 0

	for key, cached := range c.data {
		if now.After(cached.ExpiresAt) {
			delete(c.data, key)
			count++
		}
	}

	return count
}

// Clear removes all cache entries (used in testing)
func (c *ResultsCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[CacheKey]CachedEntry)
	c.hits = 0
	c.misses = 0
}

// CacheStats holds cache performance statistics
type CacheStats struct {
	Entries int
	Hits    uint64
	Misses  uint64
	HitRate float64 // hits / (hits + misses)
	SizeMB  float64 // approximate memory usage
}

// Stats returns cache statistics (hits, misses, size)
func (c *ResultsCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	// Rough estimate: 1 cache entry ≈ 5 MB (for full analysis of 25k nodes)
	sizeMB := float64(len(c.data)) * 5.0

	return CacheStats{
		Entries: len(c.data),
		Hits:    c.hits,
		Misses:  c.misses,
		HitRate: hitRate,
		SizeMB:  sizeMB,
	}
}
