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

const parityEpsilon = 1e-10

// runParityCheck serializes a graph, runs the kernel on CPU, and compares
// against analysis.TensionCalculator.CalculateAll().
func runParityCheck(t *testing.T, ig *graph.ImmutableGraph) {
	t.Helper()

	// CPU reference: analysis.TensionCalculator
	tc := analysis.NewTensionCalculator(analysis.JSD{})
	cpuTensions := tc.CalculateAll(ig)

	// GPU kernel path: serialize → compute → map back
	csr := gpu.SerializeCSR(ig)
	csc := gpu.SerializeCSC(ig, csr.NodeIdx)

	gpuTensions := gpu.ComputeAllTensions(
		csr.RowPtr, csr.ColIdx, csr.Values,
		csc.ColPtr, csc.RowIdx,
		int32(csr.NumNodes),
	)

	// Verify every node matches
	for _, nodeID := range csr.NodeIDs {
		cpuT := cpuTensions[nodeID]
		gpuT := gpuTensions[csr.NodeIdx[nodeID]]
		diff := math.Abs(cpuT - gpuT)

		if diff > parityEpsilon {
			t.Errorf("PARITY FAIL node %q: cpu=%.15f gpu=%.15f diff=%.2e",
				nodeID, cpuT, gpuT, diff)
		}
	}

	// Verify all nodes are present
	if len(cpuTensions) != csr.NumNodes {
		t.Errorf("node count mismatch: cpu=%d gpu=%d", len(cpuTensions), csr.NumNodes)
	}
}

// --- Parity Tests ---

func TestParity_Triangle(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"A", "B", 0.5},
		{"A", "C", 0.3},
		{"B", "C", 0.7},
	})
	runParityCheck(t, ig)
}

func TestParity_Bidirectional(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"A", "B", 1.0},
		{"B", "A", 2.0},
	})
	runParityCheck(t, ig)
}

func TestParity_Star(t *testing.T) {
	// Hub H connected to 5 spokes
	edges := [][3]interface{}{}
	for i := 0; i < 5; i++ {
		spoke := fmt.Sprintf("S%d", i)
		edges = append(edges, [3]interface{}{"H", spoke, float64(i+1) * 0.1})
		edges = append(edges, [3]interface{}{spoke, "H", float64(i+1) * 0.2})
	}
	ig := buildGraph(edges)
	runParityCheck(t, ig)
}

func TestParity_Chain(t *testing.T) {
	// A→B→C→D→E
	edges := [][3]interface{}{
		{"A", "B", 0.5},
		{"B", "C", 0.6},
		{"C", "D", 0.7},
		{"D", "E", 0.8},
	}
	ig := buildGraph(edges)
	runParityCheck(t, ig)
}

func TestParity_FullyConnected(t *testing.T) {
	// 4-node complete directed graph
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
	runParityCheck(t, ig)
}

func TestParity_UnequalWeights(t *testing.T) {
	// Node with highly skewed weights → produces measurable tension
	ig := buildGraph([][3]interface{}{
		{"A", "B", 100.0},
		{"A", "C", 0.001},
		{"B", "A", 50.0},
		{"C", "A", 50.0},
	})
	runParityCheck(t, ig)
}

func TestParity_SelfLoop(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"A", "A", 1.0},
		{"A", "B", 2.0},
		{"B", "A", 3.0},
	})
	runParityCheck(t, ig)
}

func TestParity_DisconnectedComponents(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"A", "B", 1.0},
		{"B", "A", 1.0},
		{"C", "D", 2.0},
		{"D", "C", 2.0},
	})
	runParityCheck(t, ig)
}

func TestParity_SingleEdge(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"X", "Y", 0.42},
	})
	runParityCheck(t, ig)
}

func TestParity_LargeRandom(t *testing.T) {
	// 200 nodes, deterministic pseudo-random edges
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
	runParityCheck(t, ig)
}

// --- Kernel-specific unit tests ---

func TestKernel_IsolatedNode(t *testing.T) {
	// Node with no edges → tension = 0
	// Build manually: 2 nodes (A, B) with edge A→B. Check B has no outgoing.
	ig := buildGraph([][3]interface{}{
		{"A", "B", 1.0},
	})
	csr := gpu.SerializeCSR(ig)
	csc := gpu.SerializeCSC(ig, csr.NodeIdx)

	// B has no outgoing edges in CSR but has incoming.
	// The tension algorithm considers all neighbors (including incoming).
	// B's neighbor is A. A has outgoing edges. So B should get a tension value.
	tensions := gpu.ComputeAllTensions(
		csr.RowPtr, csr.ColIdx, csr.Values,
		csc.ColPtr, csc.RowIdx,
		int32(csr.NumNodes),
	)

	// Just verify no panic and values are finite
	for i, t_val := range tensions {
		if math.IsNaN(t_val) || math.IsInf(t_val, 0) {
			t.Errorf("node %d: tension is %f (not finite)", i, t_val)
		}
	}
}

func TestKernel_OutOfRange(t *testing.T) {
	// nodeIdx >= numNodes should return 0
	result := gpu.ComputeNodeTension(
		[]int32{0}, nil, nil,
		[]int32{0}, nil,
		5, 1,
	)
	if result != 0 {
		t.Errorf("out-of-range node: want 0, got %f", result)
	}
}

func BenchmarkKernel_100Nodes(b *testing.B) {
	ig := buildLargeGraph(100, 10)
	csr := gpu.SerializeCSR(ig)
	csc := gpu.SerializeCSC(ig, csr.NodeIdx)
	numNodes := int32(csr.NumNodes)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		gpu.ComputeAllTensions(
			csr.RowPtr, csr.ColIdx, csr.Values,
			csc.ColPtr, csc.RowIdx,
			numNodes,
		)
	}
}

func BenchmarkKernel_1kNodes(b *testing.B) {
	ig := buildLargeGraph(1000, 10)
	csr := gpu.SerializeCSR(ig)
	csc := gpu.SerializeCSC(ig, csr.NodeIdx)
	numNodes := int32(csr.NumNodes)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		gpu.ComputeAllTensions(
			csr.RowPtr, csr.ColIdx, csr.Values,
			csc.ColPtr, csc.RowIdx,
			numNodes,
		)
	}
}
