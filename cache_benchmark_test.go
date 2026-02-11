package itt

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// BenchmarkAnalyze_NoCache measures baseline performance without cache
func BenchmarkAnalyze_NoCache(b *testing.B) {
	sizes := []int{100, 1000, 5000}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("nodes=%d", n), func(b *testing.B) {
			cfg := &Builder{
				threshold:          1.5,
				channelSize:        1000,
				detectabilityAlpha: 0.05,
			}
			// No cache

			engine, _ := cfg.Build()
			engine.Start(context.Background())
			defer engine.Stop()

			// Build graph
			for i := 0; i < n; i++ {
				for j := i + 1; j < minInt(i+10, n); j++ {
					engine.AddEvent(Event{
						Source: fmt.Sprintf("node%d", i),
						Target: fmt.Sprintf("node%d", j),
						Weight: 0.5,
						Type:   "test",
					})
				}
			}
			time.Sleep(100 * time.Millisecond) // allow processing

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				snap := engine.Snapshot()
				snap.Analyze()
				snap.Close()
			}
		})
	}
}

// BenchmarkAnalyze_CacheMiss measures performance on cache miss (first call)
func BenchmarkAnalyze_CacheMiss(b *testing.B) {
	sizes := []int{100, 1000, 5000}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("nodes=%d", n), func(b *testing.B) {
			cfg := &Builder{
				threshold:          1.5,
				channelSize:        1000,
				detectabilityAlpha: 0.05,
			}
			cfg = cfg.WithCache(1 * time.Minute)

			engine, _ := cfg.Build()
			engine.Start(context.Background())
			defer engine.Stop()

			// Build graph
			for i := 0; i < n; i++ {
				for j := i + 1; j < minInt(i+10, n); j++ {
					engine.AddEvent(Event{
						Source: fmt.Sprintf("node%d", i),
						Target: fmt.Sprintf("node%d", j),
						Weight: 0.5,
						Type:   "test",
					})
				}
			}
			time.Sleep(100 * time.Millisecond) // allow processing

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				snap := engine.Snapshot()
				snap.Analyze()
				snap.Close()

				// Clear cache for next iteration to simulate miss
				engine.ResultsCache.Clear()
			}
		})
	}
}

// BenchmarkAnalyze_CacheHit measures performance on cache hit (subsequent calls)
func BenchmarkAnalyze_CacheHit(b *testing.B) {
	sizes := []int{100, 1000, 5000}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("nodes=%d", n), func(b *testing.B) {
			cfg := &Builder{
				threshold:          1.5,
				channelSize:        1000,
				detectabilityAlpha: 0.05,
			}
			cfg = cfg.WithCache(1 * time.Minute)

			engine, _ := cfg.Build()
			engine.Start(context.Background())
			defer engine.Stop()

			// Build graph
			for i := 0; i < n; i++ {
				for j := i + 1; j < minInt(i+10, n); j++ {
					engine.AddEvent(Event{
						Source: fmt.Sprintf("node%d", i),
						Target: fmt.Sprintf("node%d", j),
						Weight: 0.5,
						Type:   "test",
					})
				}
			}
			time.Sleep(100 * time.Millisecond) // allow processing

			// Pre-populate cache
			snap := engine.Snapshot()
			snap.Analyze()

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				snap.Analyze() // Cache hit
			}

			snap.Close()
		})
	}
}

// BenchmarkAnalyzeNode_CacheHit measures single node analysis cache performance
func BenchmarkAnalyzeNode_CacheHit(b *testing.B) {
	cfg := &Builder{
		threshold:          1.5,
		channelSize:        1000,
		detectabilityAlpha: 0.05,
	}
	cfg = cfg.WithCache(1 * time.Minute)

	engine, _ := cfg.Build()
	engine.Start(context.Background())
	defer engine.Stop()

	// Build graph
	n := 1000
	for i := 0; i < n; i++ {
		for j := i + 1; j < minInt(i+10, n); j++ {
			engine.AddEvent(Event{
				Source: fmt.Sprintf("node%d", i),
				Target: fmt.Sprintf("node%d", j),
				Weight: 0.5,
				Type:   "test",
			})
		}
	}
	time.Sleep(100 * time.Millisecond)

	snap := engine.Snapshot()
	defer snap.Close()

	// Pre-populate cache
	snap.AnalyzeNode("node100")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		snap.AnalyzeNode("node100") // Cache hit
	}
}

// BenchmarkAnalyzeRegion_CacheHit measures region analysis cache performance
func BenchmarkAnalyzeRegion_CacheHit(b *testing.B) {
	cfg := &Builder{
		threshold:          1.5,
		channelSize:        1000,
		detectabilityAlpha: 0.05,
	}
	cfg = cfg.WithCache(1 * time.Minute)

	engine, _ := cfg.Build()
	engine.Start(context.Background())
	defer engine.Stop()

	// Build graph
	n := 1000
	for i := 0; i < n; i++ {
		for j := i + 1; j < minInt(i+10, n); j++ {
			engine.AddEvent(Event{
				Source: fmt.Sprintf("node%d", i),
				Target: fmt.Sprintf("node%d", j),
				Weight: 0.5,
				Type:   "test",
			})
		}
	}
	time.Sleep(100 * time.Millisecond)

	snap := engine.Snapshot()
	defer snap.Close()

	// Region: 100 nodes
	nodeIDs := make([]string, 100)
	for i := 0; i < 100; i++ {
		nodeIDs[i] = fmt.Sprintf("node%d", i)
	}

	// Pre-populate cache
	snap.AnalyzeRegion(nodeIDs)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		snap.AnalyzeRegion(nodeIDs) // Cache hit
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
