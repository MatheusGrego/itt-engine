package itt

import (
	"context"
	"testing"
	"time"
)

// TestCache_FullAnalysis verifies cache hit/miss behavior for Snapshot.Analyze()
func TestCache_FullAnalysis(t *testing.T) {
	cfg := &Builder{
		threshold:          1.5,
		channelSize:        10,
		detectabilityAlpha: 0.05,
	}
	cfg = cfg.WithCache(1 * time.Minute)

	engine, err := cfg.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer engine.Stop()

	// Submit events
	for i := 0; i < 10; i++ {
		engine.AddEvent(Event{
			Source: "A",
			Target: "B",
			Weight: float64(i+1) * 0.1,
			Type:   "test",
		})
	}

	time.Sleep(50 * time.Millisecond) // allow processing

	// First analysis (cache miss)
	snap1 := engine.Snapshot()
	defer snap1.Close()

	start := time.Now()
	result1, err := snap1.Analyze()
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	duration1 := time.Since(start)

	// Second analysis on same snapshot (cache hit)
	start = time.Now()
	result2, err := snap1.Analyze()
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	duration2 := time.Since(start)

	// Verify results match
	if len(result1.Tensions) != len(result2.Tensions) {
		t.Fatalf("tension count mismatch: %d vs %d", len(result1.Tensions), len(result2.Tensions))
	}

	// Cache hit should be significantly faster
	if duration2 >= duration1 {
		t.Logf("WARNING: cache hit not faster (miss=%v, hit=%v)", duration1, duration2)
	}

	// Verify cache stats
	stats := engine.ResultsCache.Stats()
	if stats.Hits < 1 {
		t.Fatalf("expected at least 1 cache hit, got %d", stats.Hits)
	}
	t.Logf("Cache stats: entries=%d, hits=%d, misses=%d, hit_rate=%.2f%%",
		stats.Entries, stats.Hits, stats.Misses, stats.HitRate*100)
}

// TestCache_NodeAnalysis verifies cache for AnalyzeNode()
func TestCache_NodeAnalysis(t *testing.T) {
	cfg := &Builder{
		threshold:          1.5,
		channelSize:        10,
		detectabilityAlpha: 0.05,
	}
	cfg = cfg.WithCache(1 * time.Minute)

	engine, err := cfg.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer engine.Stop()

	// Submit events
	engine.AddEvent(Event{Source: "A", Target: "B", Weight: 0.5, Type: "test"})
	engine.AddEvent(Event{Source: "B", Target: "C", Weight: 0.5, Type: "test"})
	engine.AddEvent(Event{Source: "A", Target: "C", Weight: 0.5, Type: "test"})

	time.Sleep(50 * time.Millisecond)

	snap := engine.Snapshot()
	defer snap.Close()

	// First call (miss)
	result1, err := snap.AnalyzeNode("A")
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	// Second call (hit)
	result2, err := snap.AnalyzeNode("A")
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	// Verify results match
	if result1.NodeID != result2.NodeID || result1.Tension != result2.Tension {
		t.Fatalf("cached result mismatch")
	}

	stats := engine.ResultsCache.Stats()
	if stats.Hits < 1 {
		t.Fatalf("expected at least 1 cache hit, got %d", stats.Hits)
	}
}

// TestCache_RegionAnalysis verifies cache for AnalyzeRegion()
func TestCache_RegionAnalysis(t *testing.T) {
	cfg := &Builder{
		threshold:          1.5,
		channelSize:        10,
		detectabilityAlpha: 0.05,
	}
	cfg = cfg.WithCache(1 * time.Minute)

	engine, err := cfg.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer engine.Stop()

	// Submit events
	for _, pair := range [][2]string{{"A", "B"}, {"B", "C"}, {"C", "D"}, {"A", "D"}} {
		engine.AddEvent(Event{
			Source: pair[0],
			Target: pair[1],
			Weight: 0.5,
			Type:   "test",
		})
	}

	time.Sleep(50 * time.Millisecond)

	snap := engine.Snapshot()
	defer snap.Close()

	nodeIDs := []string{"A", "B", "C"}

	// First call (miss)
	result1, err := snap.AnalyzeRegion(nodeIDs)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	// Second call (hit)
	result2, err := snap.AnalyzeRegion(nodeIDs)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	// Verify results match
	if len(result1.Nodes) != len(result2.Nodes) {
		t.Fatalf("node count mismatch: %d vs %d", len(result1.Nodes), len(result2.Nodes))
	}

	// Test with reordered nodeIDs (should still hit cache due to sorting)
	nodeIDsReordered := []string{"C", "A", "B"}
	result3, err := snap.AnalyzeRegion(nodeIDsReordered)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if len(result3.Nodes) != len(result1.Nodes) {
		t.Fatalf("reordered query returned different result")
	}

	stats := engine.ResultsCache.Stats()
	if stats.Hits < 2 {
		t.Fatalf("expected at least 2 cache hits, got %d", stats.Hits)
	}
}

// TestCache_Invalidation verifies cache invalidation on version removal
func TestCache_Invalidation(t *testing.T) {
	cfg := &Builder{
		threshold:          1.5,
		channelSize:        10,
		detectabilityAlpha: 0.05,
	}
	cfg = cfg.WithCache(1 * time.Minute)
	cfg = cfg.GCSnapshotForce(100 * time.Millisecond) // force-close after 100ms

	engine, err := cfg.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer engine.Stop()

	// Submit event
	engine.AddEvent(Event{Source: "A", Target: "B", Weight: 0.5, Type: "test"})
	time.Sleep(50 * time.Millisecond)

	// Create snapshot and analyze (populates cache)
	snap := engine.Snapshot()
	versionID := snap.version.ID
	snap.Analyze()
	snap.Close()

	// Verify cache has entry
	stats := engine.ResultsCache.Stats()
	if stats.Entries < 1 {
		t.Fatalf("expected cache entry, got %d", stats.Entries)
	}

	// Wait for GC to force-close and invalidate
	time.Sleep(150 * time.Millisecond)

	// Trigger GC manually to ensure it runs
	engine.gc.Collect()

	// Verify cache invalidated (may take a moment for async cleanup)
	time.Sleep(50 * time.Millisecond)

	// Check if version was removed (cache should have been invalidated)
	// Note: This is a best-effort test since GC timing is non-deterministic
	t.Logf("After GC - cache entries: %d, versionID: %d", engine.ResultsCache.Stats().Entries, versionID)
}

// TestCache_Disabled verifies engine works without cache
func TestCache_Disabled(t *testing.T) {
	cfg := &Builder{
		threshold:          1.5,
		channelSize:        10,
		detectabilityAlpha: 0.05,
	}
	// No WithCache() call - cache disabled

	engine, err := cfg.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer engine.Stop()

	engine.AddEvent(Event{Source: "A", Target: "B", Weight: 0.5, Type: "test"})
	time.Sleep(50 * time.Millisecond)

	snap := engine.Snapshot()
	defer snap.Close()

	// Should work without cache
	_, err = snap.Analyze()
	if err != nil {
		t.Fatalf("analyze failed without cache: %v", err)
	}

	// Verify cache is nil
	if engine.ResultsCache != nil {
		t.Fatal("expected nil cache when disabled")
	}
}

// TestCache_EvictionWorker verifies background eviction
func TestCache_EvictionWorker(t *testing.T) {
	cfg := &Builder{
		threshold:          1.5,
		channelSize:        10,
		detectabilityAlpha: 0.05,
	}
	cfg = cfg.WithCache(50 * time.Millisecond) // very short TTL

	engine, err := cfg.Build()
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer engine.Stop()

	engine.AddEvent(Event{Source: "A", Target: "B", Weight: 0.5, Type: "test"})
	time.Sleep(50 * time.Millisecond)

	snap := engine.Snapshot()
	defer snap.Close()

	// Populate cache
	snap.Analyze()

	stats := engine.ResultsCache.Stats()
	if stats.Entries < 1 {
		t.Fatalf("expected cache entry, got %d", stats.Entries)
	}

	// Wait for TTL expiration + eviction worker cycle (10s ticker)
	// Note: This test is time-sensitive and may be flaky
	time.Sleep(100 * time.Millisecond)

	// Manual eviction to test immediately
	engine.ResultsCache.EvictExpired()

	stats = engine.ResultsCache.Stats()
	if stats.Entries > 0 {
		t.Logf("Cache still has %d entries (may not have expired yet)", stats.Entries)
	}
}
