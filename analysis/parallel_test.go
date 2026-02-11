package analysis

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/MatheusGrego/itt-engine/graph"
)

// buildTestGraph creates a graph with N nodes forming a ring
func buildTestGraph(n int) *graph.Graph {
	g := graph.New()
	now := time.Now()

	for i := 0; i < n; i++ {
		from := nodeID(i)
		to := nodeID((i + 1) % n)

		g.AddEdge(from, to, rand.Float64()+0.1, "test", now)
	}
	return g
}

func nodeID(i int) string {
	return fmt.Sprintf("node-%d", i)
}

func TestCalculateAllParallel_Correctness(t *testing.T) {
	// Build graph with 1000 nodes
	g := buildTestGraph(1000)

	tc := NewTensionCalculator(JSD{})

	// Sequential
	seqResults := tc.CalculateAll(g)

	// Parallel (4 workers)
	parResults := CalculateAllParallel(tc, g, 4)

	// Results should be identical
	if len(seqResults) != len(parResults) {
		t.Fatalf("result count mismatch: seq=%d, par=%d", len(seqResults), len(parResults))
	}

	for nodeID, seqTension := range seqResults {
		parTension, ok := parResults[nodeID]
		if !ok {
			t.Errorf("node %s missing in parallel results", nodeID)
			continue
		}

		// Allow tiny floating-point error
		if math.Abs(seqTension-parTension) > 1e-10 {
			t.Errorf("tension mismatch for %s: seq=%.10f, par=%.10f",
				nodeID, seqTension, parTension)
		}
	}
}

func TestCalculateAllParallel_SmallGraphFallback(t *testing.T) {
	// Small graph (< 100 nodes) should use sequential path
	g := buildTestGraph(50)

	tc := NewTensionCalculator(JSD{})

	// This should automatically fall back to sequential
	results := CalculateAllParallel(tc, g, 4)

	if len(results) != 50 {
		t.Errorf("expected 50 results, got %d", len(results))
	}
}

func TestCalculateAllParallel_EmptyGraph(t *testing.T) {
	g := graph.New()
	tc := NewTensionCalculator(JSD{})

	results := CalculateAllParallel(tc, g, 4)

	if len(results) != 0 {
		t.Errorf("expected 0 results for empty graph, got %d", len(results))
	}
}

func TestCalculateAllParallel_AutoWorkers(t *testing.T) {
	g := buildTestGraph(500)
	tc := NewTensionCalculator(JSD{})

	// workers <= 0 should auto-detect
	results := CalculateAllParallel(tc, g, 0)

	if len(results) != 500 {
		t.Errorf("expected 500 results, got %d", len(results))
	}
}

func BenchmarkCalculateAll_Sequential(b *testing.B) {
	g := buildTestGraph(1000)
	tc := NewTensionCalculator(JSD{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc.CalculateAll(g)
	}
}

func BenchmarkCalculateAll_Parallel2(b *testing.B) {
	g := buildTestGraph(1000)
	tc := NewTensionCalculator(JSD{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateAllParallel(tc, g, 2)
	}
}

func BenchmarkCalculateAll_Parallel4(b *testing.B) {
	g := buildTestGraph(1000)
	tc := NewTensionCalculator(JSD{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateAllParallel(tc, g, 4)
	}
}

func BenchmarkCalculateAll_Parallel8(b *testing.B) {
	g := buildTestGraph(1000)
	tc := NewTensionCalculator(JSD{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateAllParallel(tc, g, 8)
	}
}

func BenchmarkCalculateAll_ParallelAuto(b *testing.B) {
	g := buildTestGraph(1000)
	tc := NewTensionCalculator(JSD{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateAllParallel(tc, g, 0) // auto-detect
	}
}
