package gpu_test

import (
	"testing"
	"time"

	"github.com/MatheusGrego/itt-engine/gpu"
	"github.com/MatheusGrego/itt-engine/graph"
)

// --- helpers ---

// buildGraph creates a mutable graph, adds edges, and returns an ImmutableGraph.
func buildGraph(edges [][3]interface{}) *graph.ImmutableGraph {
	g := graph.New()
	now := time.Now()
	for _, e := range edges {
		from := e[0].(string)
		to := e[1].(string)
		w := e[2].(float64)
		g.AddEdge(from, to, w, "test", now)
	}
	return graph.NewImmutable(g)
}

func assertCSRInvariant(t *testing.T, csr *gpu.CSRGraph) {
	t.Helper()

	// RowPtr length = NumNodes + 1
	if len(csr.RowPtr) != csr.NumNodes+1 {
		t.Fatalf("RowPtr length: want %d, got %d", csr.NumNodes+1, len(csr.RowPtr))
	}
	// ColIdx and Values same length = NumEdges
	if len(csr.ColIdx) != csr.NumEdges {
		t.Fatalf("ColIdx length: want %d, got %d", csr.NumEdges, len(csr.ColIdx))
	}
	if len(csr.Values) != csr.NumEdges {
		t.Fatalf("Values length: want %d, got %d", csr.NumEdges, len(csr.Values))
	}
	// RowPtr is monotonically non-decreasing
	for i := 1; i < len(csr.RowPtr); i++ {
		if csr.RowPtr[i] < csr.RowPtr[i-1] {
			t.Fatalf("RowPtr not monotonic at %d: %d < %d", i, csr.RowPtr[i], csr.RowPtr[i-1])
		}
	}
	// Last RowPtr = NumEdges
	if csr.RowPtr[csr.NumNodes] != int32(csr.NumEdges) {
		t.Fatalf("RowPtr[last]: want %d, got %d", csr.NumEdges, csr.RowPtr[csr.NumNodes])
	}
	// ColIdx values in [0, NumNodes)
	for i, col := range csr.ColIdx {
		if col < 0 || int(col) >= csr.NumNodes {
			t.Fatalf("ColIdx[%d]=%d out of range [0, %d)", i, col, csr.NumNodes)
		}
	}
	// NodeIDs length matches NumNodes
	if len(csr.NodeIDs) != csr.NumNodes {
		t.Fatalf("NodeIDs length: want %d, got %d", csr.NumNodes, len(csr.NodeIDs))
	}
	// NodeIdx is inverse of NodeIDs
	if len(csr.NodeIdx) != csr.NumNodes {
		t.Fatalf("NodeIdx length: want %d, got %d", csr.NumNodes, len(csr.NodeIdx))
	}
	for i, id := range csr.NodeIDs {
		if idx, ok := csr.NodeIdx[id]; !ok || idx != i {
			t.Fatalf("NodeIdx[%q]: want %d, got %d (ok=%v)", id, i, idx, ok)
		}
	}
}

func assertCSCInvariant(t *testing.T, csc *gpu.CSCGraph) {
	t.Helper()

	if len(csc.ColPtr) != csc.NumNodes+1 {
		t.Fatalf("ColPtr length: want %d, got %d", csc.NumNodes+1, len(csc.ColPtr))
	}
	if len(csc.RowIdx) != csc.NumEdges {
		t.Fatalf("RowIdx length: want %d, got %d", csc.NumEdges, len(csc.RowIdx))
	}
	if len(csc.Values) != csc.NumEdges {
		t.Fatalf("Values length: want %d, got %d", csc.NumEdges, len(csc.Values))
	}
	for i := 1; i < len(csc.ColPtr); i++ {
		if csc.ColPtr[i] < csc.ColPtr[i-1] {
			t.Fatalf("ColPtr not monotonic at %d: %d < %d", i, csc.ColPtr[i], csc.ColPtr[i-1])
		}
	}
	if csc.ColPtr[csc.NumNodes] != int32(csc.NumEdges) {
		t.Fatalf("ColPtr[last]: want %d, got %d", csc.NumEdges, csc.ColPtr[csc.NumNodes])
	}
	for i, row := range csc.RowIdx {
		if row < 0 || int(row) >= csc.NumNodes {
			t.Fatalf("RowIdx[%d]=%d out of range [0, %d)", i, row, csc.NumNodes)
		}
	}
}

// outDegree returns the CSR out-degree for node at index i.
func outDegree(csr *gpu.CSRGraph, i int) int {
	return int(csr.RowPtr[i+1] - csr.RowPtr[i])
}

// inDegree returns the CSC in-degree for node at index j.
func inDegree(csc *gpu.CSCGraph, j int) int {
	return int(csc.ColPtr[j+1] - csc.ColPtr[j])
}

// --- tests ---

func TestSerializeCSR_EmptyGraph(t *testing.T) {
	ig := graph.NewImmutableEmpty()
	csr := gpu.SerializeCSR(ig)

	if csr.NumNodes != 0 {
		t.Fatalf("NumNodes: want 0, got %d", csr.NumNodes)
	}
	if csr.NumEdges != 0 {
		t.Fatalf("NumEdges: want 0, got %d", csr.NumEdges)
	}
	// RowPtr should have exactly 1 element (the sentinel 0)
	if len(csr.RowPtr) != 1 || csr.RowPtr[0] != 0 {
		t.Fatalf("RowPtr for empty graph: want [0], got %v", csr.RowPtr)
	}
}

func TestSerializeCSC_EmptyGraph(t *testing.T) {
	ig := graph.NewImmutableEmpty()
	csc := gpu.SerializeCSC(ig, make(map[string]int))

	if csc.NumNodes != 0 {
		t.Fatalf("NumNodes: want 0, got %d", csc.NumNodes)
	}
	if len(csc.ColPtr) != 1 || csc.ColPtr[0] != 0 {
		t.Fatalf("ColPtr for empty graph: want [0], got %v", csc.ColPtr)
	}
}

func TestSerializeCSR_SimpleTriangle(t *testing.T) {
	// A→B(0.5), A→C(0.3), B→C(0.7)
	ig := buildGraph([][3]interface{}{
		{"A", "B", 0.5},
		{"A", "C", 0.3},
		{"B", "C", 0.7},
	})

	csr := gpu.SerializeCSR(ig)

	assertCSRInvariant(t, csr)

	if csr.NumNodes != 3 {
		t.Fatalf("NumNodes: want 3, got %d", csr.NumNodes)
	}
	if csr.NumEdges != 3 {
		t.Fatalf("NumEdges: want 3, got %d", csr.NumEdges)
	}

	// NodeIDs are sorted: ["A", "B", "C"]
	want := []string{"A", "B", "C"}
	for i, id := range csr.NodeIDs {
		if id != want[i] {
			t.Fatalf("NodeIDs[%d]: want %q, got %q", i, want[i], id)
		}
	}

	// A has 2 outgoing edges (→B, →C)
	aIdx := csr.NodeIdx["A"]
	if outDegree(csr, aIdx) != 2 {
		t.Fatalf("A out-degree: want 2, got %d", outDegree(csr, aIdx))
	}

	// B has 1 outgoing edge (→C)
	bIdx := csr.NodeIdx["B"]
	if outDegree(csr, bIdx) != 1 {
		t.Fatalf("B out-degree: want 1, got %d", outDegree(csr, bIdx))
	}

	// C has 0 outgoing edges
	cIdx := csr.NodeIdx["C"]
	if outDegree(csr, cIdx) != 0 {
		t.Fatalf("C out-degree: want 0, got %d", outDegree(csr, cIdx))
	}
}

func TestSerializeCSC_SimpleTriangle(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"A", "B", 0.5},
		{"A", "C", 0.3},
		{"B", "C", 0.7},
	})

	csr := gpu.SerializeCSR(ig)
	csc := gpu.SerializeCSC(ig, csr.NodeIdx)

	assertCSCInvariant(t, csc)

	if csc.NumNodes != 3 {
		t.Fatalf("NumNodes: want 3, got %d", csc.NumNodes)
	}
	if csc.NumEdges != 3 {
		t.Fatalf("NumEdges: want 3, got %d", csc.NumEdges)
	}

	// A has 0 incoming edges
	aIdx := csr.NodeIdx["A"]
	if inDegree(csc, aIdx) != 0 {
		t.Fatalf("A in-degree: want 0, got %d", inDegree(csc, aIdx))
	}

	// B has 1 incoming edge (from A)
	bIdx := csr.NodeIdx["B"]
	if inDegree(csc, bIdx) != 1 {
		t.Fatalf("B in-degree: want 1, got %d", inDegree(csc, bIdx))
	}

	// C has 2 incoming edges (from A, from B)
	cIdx := csr.NodeIdx["C"]
	if inDegree(csc, cIdx) != 2 {
		t.Fatalf("C in-degree: want 2, got %d", inDegree(csc, cIdx))
	}
}

func TestSerializeCSR_WeightsPreserved(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"X", "Y", 3.14},
		{"X", "Z", 2.71},
	})

	csr := gpu.SerializeCSR(ig)
	assertCSRInvariant(t, csr)

	xIdx := csr.NodeIdx["X"]
	start := csr.RowPtr[xIdx]
	end := csr.RowPtr[xIdx+1]

	// X has 2 outgoing edges; collect weights
	weights := make(map[int32]float64)
	for i := start; i < end; i++ {
		weights[csr.ColIdx[i]] = csr.Values[i]
	}

	yIdx := int32(csr.NodeIdx["Y"])
	zIdx := int32(csr.NodeIdx["Z"])

	if w, ok := weights[yIdx]; !ok || w != 3.14 {
		t.Fatalf("X→Y weight: want 3.14, got %v (ok=%v)", w, ok)
	}
	if w, ok := weights[zIdx]; !ok || w != 2.71 {
		t.Fatalf("X→Z weight: want 2.71, got %v (ok=%v)", w, ok)
	}
}

func TestSerializeCSC_WeightsPreserved(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"X", "Z", 3.14},
		{"Y", "Z", 2.71},
	})

	csr := gpu.SerializeCSR(ig)
	csc := gpu.SerializeCSC(ig, csr.NodeIdx)
	assertCSCInvariant(t, csc)

	zIdx := csr.NodeIdx["Z"]
	start := csc.ColPtr[zIdx]
	end := csc.ColPtr[zIdx+1]

	// Z has 2 incoming edges; collect weights
	weights := make(map[int32]float64)
	for i := start; i < end; i++ {
		weights[csc.RowIdx[i]] = csc.Values[i]
	}

	xIdx := int32(csr.NodeIdx["X"])
	yIdx := int32(csr.NodeIdx["Y"])

	if w, ok := weights[xIdx]; !ok || w != 3.14 {
		t.Fatalf("X→Z weight: want 3.14, got %v (ok=%v)", w, ok)
	}
	if w, ok := weights[yIdx]; !ok || w != 2.71 {
		t.Fatalf("Y→Z weight: want 2.71, got %v (ok=%v)", w, ok)
	}
}

func TestSerialize_SingleNode(t *testing.T) {
	// A single node with no edges (isolated node added via self-referencing edge)
	g := graph.New()
	now := time.Now()
	g.AddEdge("solo", "solo", 1.0, "self", now)
	ig := graph.NewImmutable(g)

	csr := gpu.SerializeCSR(ig)
	assertCSRInvariant(t, csr)

	if csr.NumNodes != 1 {
		t.Fatalf("NumNodes: want 1, got %d", csr.NumNodes)
	}
	if csr.NumEdges != 1 {
		t.Fatalf("NumEdges: want 1, got %d (self-loop)", csr.NumEdges)
	}

	// Self-loop: ColIdx should point to self
	if csr.ColIdx[0] != 0 {
		t.Fatalf("Self-loop ColIdx: want 0, got %d", csr.ColIdx[0])
	}

	csc := gpu.SerializeCSC(ig, csr.NodeIdx)
	assertCSCInvariant(t, csc)

	if csc.NumEdges != 1 {
		t.Fatalf("CSC NumEdges: want 1, got %d", csc.NumEdges)
	}
}

func TestSerialize_DisconnectedComponents(t *testing.T) {
	// Two disconnected pairs: A→B, C→D
	ig := buildGraph([][3]interface{}{
		{"A", "B", 1.0},
		{"C", "D", 2.0},
	})

	csr := gpu.SerializeCSR(ig)
	assertCSRInvariant(t, csr)

	if csr.NumNodes != 4 {
		t.Fatalf("NumNodes: want 4, got %d", csr.NumNodes)
	}
	if csr.NumEdges != 2 {
		t.Fatalf("NumEdges: want 2, got %d", csr.NumEdges)
	}

	// A and C have 1 outgoing; B and D have 0 outgoing
	if outDegree(csr, csr.NodeIdx["A"]) != 1 {
		t.Fatal("A should have 1 outgoing edge")
	}
	if outDegree(csr, csr.NodeIdx["B"]) != 0 {
		t.Fatal("B should have 0 outgoing edges")
	}
	if outDegree(csr, csr.NodeIdx["C"]) != 1 {
		t.Fatal("C should have 1 outgoing edge")
	}
	if outDegree(csr, csr.NodeIdx["D"]) != 0 {
		t.Fatal("D should have 0 outgoing edges")
	}
}

func TestSerialize_Bidirectional(t *testing.T) {
	// A→B(1.0) and B→A(2.0) — two separate directed edges
	ig := buildGraph([][3]interface{}{
		{"A", "B", 1.0},
		{"B", "A", 2.0},
	})

	csr := gpu.SerializeCSR(ig)
	assertCSRInvariant(t, csr)

	if csr.NumEdges != 2 {
		t.Fatalf("NumEdges: want 2, got %d", csr.NumEdges)
	}

	// Both nodes should have 1 outgoing edge
	if outDegree(csr, csr.NodeIdx["A"]) != 1 {
		t.Fatal("A should have 1 outgoing edge")
	}
	if outDegree(csr, csr.NodeIdx["B"]) != 1 {
		t.Fatal("B should have 1 outgoing edge")
	}

	csc := gpu.SerializeCSC(ig, csr.NodeIdx)
	assertCSCInvariant(t, csc)

	// Both nodes should have 1 incoming edge
	if inDegree(csc, csr.NodeIdx["A"]) != 1 {
		t.Fatal("A should have 1 incoming edge")
	}
	if inDegree(csc, csr.NodeIdx["B"]) != 1 {
		t.Fatal("B should have 1 incoming edge")
	}
}

func TestSerialize_DeterministicOrdering(t *testing.T) {
	// Run serialization multiple times — node order must be identical
	ig := buildGraph([][3]interface{}{
		{"Z", "A", 1.0},
		{"M", "Z", 2.0},
		{"A", "M", 3.0},
	})

	first := gpu.SerializeCSR(ig)

	for run := 0; run < 10; run++ {
		csr := gpu.SerializeCSR(ig)
		for i, id := range csr.NodeIDs {
			if id != first.NodeIDs[i] {
				t.Fatalf("run %d: NodeIDs[%d] = %q, want %q", run, i, id, first.NodeIDs[i])
			}
		}
		for i := range csr.RowPtr {
			if csr.RowPtr[i] != first.RowPtr[i] {
				t.Fatalf("run %d: RowPtr[%d] = %d, want %d", run, i, csr.RowPtr[i], first.RowPtr[i])
			}
		}
		for i := range csr.ColIdx {
			if csr.ColIdx[i] != first.ColIdx[i] {
				t.Fatalf("run %d: ColIdx[%d] = %d, want %d", run, i, csr.ColIdx[i], first.ColIdx[i])
			}
		}
		for i := range csr.Values {
			if csr.Values[i] != first.Values[i] {
				t.Fatalf("run %d: Values[%d] = %f, want %f", run, i, csr.Values[i], first.Values[i])
			}
		}
	}
}

func TestSerialize_EdgeCountParity(t *testing.T) {
	// CSR and CSC must report the same edge count
	ig := buildGraph([][3]interface{}{
		{"A", "B", 0.5},
		{"B", "C", 0.7},
		{"C", "A", 0.3},
		{"A", "C", 0.2},
	})

	csr := gpu.SerializeCSR(ig)
	csc := gpu.SerializeCSC(ig, csr.NodeIdx)

	if csr.NumEdges != csc.NumEdges {
		t.Fatalf("Edge count mismatch: CSR=%d, CSC=%d", csr.NumEdges, csc.NumEdges)
	}
}

func TestSerialize_LargeGraph(t *testing.T) {
	// 1000 nodes, ~9000 edges (chain with extra random connections)
	g := graph.New()
	now := time.Now()

	n := 1000
	for i := 0; i < n-1; i++ {
		from := nodeID(i)
		to := nodeID(i + 1)
		g.AddEdge(from, to, 0.5, "chain", now)
	}
	// Add cross-links to create a denser graph
	for i := 0; i < n; i++ {
		target := (i*7 + 13) % n // deterministic pseudo-random target
		if target != i {
			g.AddEdge(nodeID(i), nodeID(target), 0.3, "cross", now)
		}
	}

	ig := graph.NewImmutable(g)
	csr := gpu.SerializeCSR(ig)
	assertCSRInvariant(t, csr)

	if csr.NumNodes != n {
		t.Fatalf("NumNodes: want %d, got %d", n, csr.NumNodes)
	}

	csc := gpu.SerializeCSC(ig, csr.NodeIdx)
	assertCSCInvariant(t, csc)

	if csr.NumEdges != csc.NumEdges {
		t.Fatalf("Edge parity: CSR=%d, CSC=%d", csr.NumEdges, csc.NumEdges)
	}

	t.Logf("Large graph: %d nodes, %d edges", csr.NumNodes, csr.NumEdges)
}

func nodeID(i int) string {
	return "node" + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	b := make([]byte, 0, 10)
	for i > 0 {
		b = append(b, byte('0'+i%10))
		i /= 10
	}
	// reverse
	for l, r := 0, len(b)-1; l < r; l, r = l+1, r-1 {
		b[l], b[r] = b[r], b[l]
	}
	return string(b)
}

func BenchmarkSerializeCSR_1k(b *testing.B) {
	ig := buildLargeGraph(1000, 10)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		gpu.SerializeCSR(ig)
	}
}

func BenchmarkSerializeCSR_10k(b *testing.B) {
	ig := buildLargeGraph(10000, 10)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		gpu.SerializeCSR(ig)
	}
}

func BenchmarkSerializeCSC_1k(b *testing.B) {
	ig := buildLargeGraph(1000, 10)
	csr := gpu.SerializeCSR(ig)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		gpu.SerializeCSC(ig, csr.NodeIdx)
	}
}

func buildLargeGraph(n, avgDegree int) *graph.ImmutableGraph {
	g := graph.New()
	now := time.Now()
	for i := 0; i < n; i++ {
		for d := 1; d <= avgDegree; d++ {
			target := (i + d) % n
			g.AddEdge(nodeID(i), nodeID(target), 0.5, "test", now)
		}
	}
	return graph.NewImmutable(g)
}
