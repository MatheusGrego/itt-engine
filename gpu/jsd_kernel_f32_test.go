package gpu_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/MatheusGrego/itt-engine/analysis"
	"github.com/MatheusGrego/itt-engine/gpu"
	"github.com/MatheusGrego/itt-engine/graph"
)

const parityEpsilonF32 = 1e-5

// runParityCheckF32 serializes a graph, runs the float32 kernel on CPU,
// and compares against analysis.TensionCalculator.CalculateAll() (float64).
func runParityCheckF32(t *testing.T, ig *graph.ImmutableGraph) {
	t.Helper()

	// CPU reference: analysis.TensionCalculator (float64)
	tc := analysis.NewTensionCalculator(analysis.JSD{})
	cpuTensions := tc.CalculateAll(ig)

	// GPU f32 kernel path: serialize → compute → map back
	csr := gpu.SerializeCSRF32(ig)
	csc := gpu.SerializeCSC(ig, csr.NodeIdx)

	gpuTensions := gpu.ComputeAllTensionsF32(
		csr.RowPtr, csr.ColIdx, csr.Values,
		csc.ColPtr, csc.RowIdx,
		int32(csr.NumNodes),
	)

	// Track max diff for logging
	maxDiff := 0.0

	// Verify every node matches within float32 tolerance
	for _, nodeID := range csr.NodeIDs {
		cpuT := cpuTensions[nodeID]
		gpuT := float64(gpuTensions[csr.NodeIdx[nodeID]])
		diff := math.Abs(cpuT - gpuT)

		if diff > maxDiff {
			maxDiff = diff
		}

		if diff > parityEpsilonF32 {
			t.Errorf("PARITY FAIL node %q: cpu=%.15f gpu_f32=%.15f diff=%.2e",
				nodeID, cpuT, gpuT, diff)
		}
	}

	// Verify all nodes are present
	if len(cpuTensions) != csr.NumNodes {
		t.Errorf("node count mismatch: cpu=%d gpu=%d", len(cpuTensions), csr.NumNodes)
	}

	t.Logf("max diff: %.2e (tolerance: %.2e)", maxDiff, parityEpsilonF32)
}

// --- Float32 Parity Tests (same shapes as float64) ---

func TestParityF32_Triangle(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"A", "B", 0.5},
		{"A", "C", 0.3},
		{"B", "C", 0.7},
	})
	runParityCheckF32(t, ig)
}

func TestParityF32_Bidirectional(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"A", "B", 1.0},
		{"B", "A", 2.0},
	})
	runParityCheckF32(t, ig)
}

func TestParityF32_Star(t *testing.T) {
	edges := [][3]interface{}{}
	for i := 0; i < 5; i++ {
		spoke := fmt.Sprintf("S%d", i)
		edges = append(edges, [3]interface{}{"H", spoke, float64(i+1) * 0.1})
		edges = append(edges, [3]interface{}{spoke, "H", float64(i+1) * 0.2})
	}
	ig := buildGraph(edges)
	runParityCheckF32(t, ig)
}

func TestParityF32_Chain(t *testing.T) {
	edges := [][3]interface{}{
		{"A", "B", 0.5},
		{"B", "C", 0.6},
		{"C", "D", 0.7},
		{"D", "E", 0.8},
	}
	ig := buildGraph(edges)
	runParityCheckF32(t, ig)
}

func TestParityF32_FullyConnected(t *testing.T) {
	nodes := []string{"A", "B", "C", "D"}
	edges := [][3]interface{}{}
	w := 0.1
	for _, from := range nodes {
		for _, to := range nodes {
			if from != to {
				edges = append(edges, [3]interface{}{from, to, w})
				w += 0.05
			}
		}
	}
	ig := buildGraph(edges)
	runParityCheckF32(t, ig)
}

func TestParityF32_UnequalWeights(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"A", "B", 100.0},
		{"A", "C", 0.001},
		{"B", "A", 50.0},
		{"C", "A", 50.0},
	})
	runParityCheckF32(t, ig)
}

func TestParityF32_SelfLoop(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"A", "A", 1.0},
		{"A", "B", 2.0},
		{"B", "A", 3.0},
	})
	runParityCheckF32(t, ig)
}

func TestParityF32_DisconnectedComponents(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"A", "B", 1.0},
		{"B", "A", 1.0},
		{"C", "D", 2.0},
		{"D", "C", 2.0},
	})
	runParityCheckF32(t, ig)
}

func TestParityF32_SingleEdge(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"X", "Y", 0.42},
	})
	runParityCheckF32(t, ig)
}

func TestParityF32_LargeRandom(t *testing.T) {
	g := graph.New()
	now := time.Now()
	n := 200
	for i := 0; i < n; i++ {
		for d := 1; d <= 5; d++ {
			target := (i*7 + d*13 + 3) % n
			if target != i {
				w := float64((i*11+d*3)%100+1) / 100.0
				g.AddEdge(nodeID(i), nodeID(target), w, "test", now)
			}
		}
	}
	ig := graph.NewImmutable(g)
	runParityCheckF32(t, ig)
}

// --- Float32-specific unit tests ---

func TestKernelF32_IsolatedNode(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"A", "B", 1.0},
	})
	csr := gpu.SerializeCSRF32(ig)
	csc := gpu.SerializeCSC(ig, csr.NodeIdx)

	tensions := gpu.ComputeAllTensionsF32(
		csr.RowPtr, csr.ColIdx, csr.Values,
		csc.ColPtr, csc.RowIdx,
		int32(csr.NumNodes),
	)

	for i, t_val := range tensions {
		if math.IsNaN(float64(t_val)) || math.IsInf(float64(t_val), 0) {
			t.Errorf("node %d: tension is %f (not finite)", i, t_val)
		}
	}
}

func TestKernelF32_OutOfRange(t *testing.T) {
	result := gpu.ComputeNodeTensionF32(
		[]int32{0}, nil, nil,
		[]int32{0}, nil,
		5, 1,
	)
	if result != 0 {
		t.Errorf("out-of-range node: want 0, got %f", result)
	}
}

// TestKernelF32_AnomalyDecisionParity verifies that anomaly decisions
// (tension > threshold) are identical between float32 and float64 kernels
// for thresholds >= 0.01.
func TestKernelF32_AnomalyDecisionParity(t *testing.T) {
	ig := buildLargeGraph(200, 8)

	// Float64 reference
	csr64 := gpu.SerializeCSR(ig)
	csc := gpu.SerializeCSC(ig, csr64.NodeIdx)
	tensions64 := gpu.ComputeAllTensions(
		csr64.RowPtr, csr64.ColIdx, csr64.Values,
		csc.ColPtr, csc.RowIdx,
		int32(csr64.NumNodes),
	)

	// Float32
	csr32 := gpu.SerializeCSRF32(ig)
	tensions32 := gpu.ComputeAllTensionsF32(
		csr32.RowPtr, csr32.ColIdx, csr32.Values,
		csc.ColPtr, csc.RowIdx,
		int32(csr32.NumNodes),
	)

	// Test multiple thresholds
	thresholds := []float64{0.01, 0.05, 0.1, 0.2, 0.3, 0.5}

	for _, thresh := range thresholds {
		mismatches := 0
		for i := 0; i < int(csr64.NumNodes); i++ {
			anomaly64 := tensions64[i] > thresh
			anomaly32 := float64(tensions32[i]) > thresh

			if anomaly64 != anomaly32 {
				mismatches++
				// Only report if not a boundary case (within 1e-3 of threshold)
				margin := math.Abs(tensions64[i] - thresh)
				if margin > 1e-3 {
					t.Errorf("threshold=%.2f node %d: f64=%v (%.6f) f32=%v (%.6f)",
						thresh, i, anomaly64, tensions64[i], anomaly32, float64(tensions32[i]))
				}
			}
		}
		if mismatches > 0 {
			t.Logf("threshold=%.2f: %d boundary mismatches out of %d nodes (expected for values near threshold)",
				thresh, mismatches, csr64.NumNodes)
		}
	}
}

// TestSerializeCSRF32_WeightsConversion verifies float64 → float32 conversion.
func TestSerializeCSRF32_WeightsConversion(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"X", "Y", 3.14},
		{"X", "Z", 2.71},
	})

	csr64 := gpu.SerializeCSR(ig)
	csr32 := gpu.SerializeCSRF32(ig)

	if csr32.NumNodes != csr64.NumNodes {
		t.Fatalf("NumNodes mismatch: f64=%d f32=%d", csr64.NumNodes, csr32.NumNodes)
	}
	if csr32.NumEdges != csr64.NumEdges {
		t.Fatalf("NumEdges mismatch: f64=%d f32=%d", csr64.NumEdges, csr32.NumEdges)
	}

	// Indices must be identical
	for i := range csr64.RowPtr {
		if csr32.RowPtr[i] != csr64.RowPtr[i] {
			t.Errorf("RowPtr[%d]: f64=%d f32=%d", i, csr64.RowPtr[i], csr32.RowPtr[i])
		}
	}
	for i := range csr64.ColIdx {
		if csr32.ColIdx[i] != csr64.ColIdx[i] {
			t.Errorf("ColIdx[%d]: f64=%d f32=%d", i, csr64.ColIdx[i], csr32.ColIdx[i])
		}
	}

	// Values must be float32 conversion of float64
	for i := range csr64.Values {
		want := float32(csr64.Values[i])
		got := csr32.Values[i]
		if want != got {
			t.Errorf("Values[%d]: want %f got %f", i, want, got)
		}
	}
}

// --- Benchmarks ---

func BenchmarkKernelF32_100Nodes(b *testing.B) {
	ig := buildLargeGraph(100, 10)
	csr := gpu.SerializeCSRF32(ig)
	csc := gpu.SerializeCSC(ig, csr.NodeIdx)
	numNodes := int32(csr.NumNodes)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		gpu.ComputeAllTensionsF32(
			csr.RowPtr, csr.ColIdx, csr.Values,
			csc.ColPtr, csc.RowIdx,
			numNodes,
		)
	}
}

func BenchmarkKernelF32_1kNodes(b *testing.B) {
	ig := buildLargeGraph(1000, 10)
	csr := gpu.SerializeCSRF32(ig)
	csc := gpu.SerializeCSC(ig, csr.NodeIdx)
	numNodes := int32(csr.NumNodes)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		gpu.ComputeAllTensionsF32(
			csr.RowPtr, csr.ColIdx, csr.Values,
			csc.ColPtr, csc.RowIdx,
			numNodes,
		)
	}
}

func BenchmarkSerializeCSRF32_1k(b *testing.B) {
	ig := buildLargeGraph(1000, 10)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		gpu.SerializeCSRF32(ig)
	}
}
