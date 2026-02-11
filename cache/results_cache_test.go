package cache

import (
	"testing"
	"time"
)

func TestResultsCache_SetGet(t *testing.T) {
	cache := NewResultsCache(1 * time.Minute)

	key := CacheKey{
		VersionID: 1,
		QueryType: "full_analysis",
		QueryArgs: "",
	}

	// Miss initially
	_, ok := cache.Get(key)
	if ok {
		t.Fatal("expected cache miss on first access")
	}

	// Set value
	value := "test_result"
	cache.Set(key, value)

	// Hit after set
	cached, ok := cache.Get(key)
	if !ok {
		t.Fatal("expected cache hit after set")
	}
	if cached.(string) != value {
		t.Fatalf("expected %q, got %q", value, cached)
	}

	stats := cache.Stats()
	if stats.Hits != 1 || stats.Misses != 1 {
		t.Fatalf("expected 1 hit and 1 miss, got hits=%d misses=%d", stats.Hits, stats.Misses)
	}
}

func TestResultsCache_TTLExpiration(t *testing.T) {
	cache := NewResultsCache(50 * time.Millisecond)

	key := CacheKey{
		VersionID: 1,
		QueryType: "node_analysis",
		QueryArgs: "node1",
	}

	cache.Set(key, "value")

	// Hit immediately
	_, ok := cache.Get(key)
	if !ok {
		t.Fatal("expected cache hit immediately after set")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Miss after expiration
	_, ok = cache.Get(key)
	if ok {
		t.Fatal("expected cache miss after TTL expiration")
	}
}

func TestResultsCache_InvalidateVersion(t *testing.T) {
	cache := NewResultsCache(1 * time.Minute)

	// Add entries for multiple versions
	for v := uint64(1); v <= 5; v++ {
		for q := 0; q < 3; q++ {
			key := CacheKey{
				VersionID: v,
				QueryType: "test",
				QueryArgs: string(rune('A' + q)), // unique args per query
			}
			cache.Set(key, v)
		}
	}

	stats := cache.Stats()
	if stats.Entries != 15 {
		t.Fatalf("expected 15 entries, got %d", stats.Entries)
	}

	// Invalidate version 3
	removed := cache.InvalidateVersion(3)
	if removed != 3 {
		t.Fatalf("expected 3 entries removed, got %d", removed)
	}

	stats = cache.Stats()
	if stats.Entries != 12 {
		t.Fatalf("expected 12 entries after invalidation, got %d", stats.Entries)
	}

	// Verify version 3 is gone
	key := CacheKey{VersionID: 3, QueryType: "test", QueryArgs: "A"}
	_, ok := cache.Get(key)
	if ok {
		t.Fatal("expected miss for invalidated version")
	}

	// Verify other versions still exist
	key2 := CacheKey{VersionID: 2, QueryType: "test", QueryArgs: "A"}
	_, ok = cache.Get(key2)
	if !ok {
		t.Fatal("expected hit for non-invalidated version")
	}
}

func TestResultsCache_EvictExpired(t *testing.T) {
	cache := NewResultsCache(30 * time.Millisecond)

	// Add entries
	for i := 0; i < 10; i++ {
		key := CacheKey{
			VersionID: uint64(i),
			QueryType: "test",
		}
		cache.Set(key, i)
	}

	stats := cache.Stats()
	if stats.Entries != 10 {
		t.Fatalf("expected 10 entries, got %d", stats.Entries)
	}

	// Wait for expiration
	time.Sleep(50 * time.Millisecond)

	// Evict expired
	evicted := cache.EvictExpired()
	if evicted != 10 {
		t.Fatalf("expected 10 evicted, got %d", evicted)
	}

	stats = cache.Stats()
	if stats.Entries != 0 {
		t.Fatalf("expected 0 entries after eviction, got %d", stats.Entries)
	}
}

func TestResultsCache_Clear(t *testing.T) {
	cache := NewResultsCache(1 * time.Minute)

	// Add entries
	for i := 0; i < 5; i++ {
		key := CacheKey{
			VersionID: uint64(i),
			QueryType: "test",
		}
		cache.Set(key, i)
		cache.Get(key) // generate hit
	}

	stats := cache.Stats()
	if stats.Entries != 5 {
		t.Fatalf("expected 5 entries, got %d", stats.Entries)
	}
	if stats.Hits != 5 {
		t.Fatalf("expected 5 hits, got %d", stats.Hits)
	}

	// Clear
	cache.Clear()

	stats = cache.Stats()
	if stats.Entries != 0 {
		t.Fatalf("expected 0 entries after clear, got %d", stats.Entries)
	}
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Fatalf("expected stats reset after clear, got hits=%d misses=%d", stats.Hits, stats.Misses)
	}
}

func TestResultsCache_Stats_HitRate(t *testing.T) {
	cache := NewResultsCache(1 * time.Minute)

	key := CacheKey{VersionID: 1, QueryType: "test"}

	// 1 miss (initial)
	cache.Get(key)

	// Set
	cache.Set(key, "value")

	// 9 hits
	for i := 0; i < 9; i++ {
		cache.Get(key)
	}

	stats := cache.Stats()
	if stats.Hits != 9 || stats.Misses != 1 {
		t.Fatalf("expected 9 hits and 1 miss, got hits=%d misses=%d", stats.Hits, stats.Misses)
	}

	// Expected hit rate: 9 / (9 + 1) = 0.9
	if stats.HitRate < 0.89 || stats.HitRate > 0.91 {
		t.Fatalf("expected hit rate ~0.9, got %f", stats.HitRate)
	}
}

func TestResultsCache_ConcurrentAccess(t *testing.T) {
	cache := NewResultsCache(1 * time.Minute)

	// Concurrent writes
	done := make(chan bool)
	for g := 0; g < 10; g++ {
		go func(id int) {
			for i := 0; i < 100; i++ {
				key := CacheKey{
					VersionID: uint64(id),
					QueryType: "test",
					QueryArgs: "",
				}
				cache.Set(key, id*100+i)
			}
			done <- true
		}(g)
	}

	// Wait for writes
	for g := 0; g < 10; g++ {
		<-done
	}

	// Concurrent reads
	for g := 0; g < 10; g++ {
		go func(id int) {
			for i := 0; i < 100; i++ {
				key := CacheKey{
					VersionID: uint64(id),
					QueryType: "test",
				}
				cache.Get(key)
			}
			done <- true
		}(g)
	}

	// Wait for reads
	for g := 0; g < 10; g++ {
		<-done
	}

	stats := cache.Stats()
	if stats.Entries != 10 {
		t.Fatalf("expected 10 entries after concurrent access, got %d", stats.Entries)
	}
}
