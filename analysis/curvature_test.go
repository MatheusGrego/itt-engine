package analysis

import (
	"math"
	"testing"
	"time"

	"github.com/mfreiregr/itt-engine/graph"
)

// buildImmutable is a helper that creates an ImmutableGraph from a mutable Graph.
func buildImmutable(g *graph.Graph) *graph.ImmutableGraph {
	return graph.NewImmutable(g)
}

// addBidirectional adds edges in both directions between two nodes.
func addBidirectional(g *graph.Graph, a, b string, w float64, ts time.Time) {
	g.AddEdge(a, b, w, "test", ts)
	g.AddEdge(b, a, w, "test", ts)
}

func TestCurvature_CompleteGraphK4(t *testing.T) {
	// In a complete graph K4, every pair of nodes is connected.
	// Edges in dense, clique-like structures should have positive
	// (or near-zero) curvature because the neighborhoods overlap heavily.
	g := graph.New()
	ts := time.Now()
	nodes := []string{"A", "B", "C", "D"}
	for i := 0; i < len(nodes); i++ {
		for j := 0; j < len(nodes); j++ {
			if i != j {
				g.AddEdge(nodes[i], nodes[j], 1.0, "test", ts)
			}
		}
	}
	ig := buildImmutable(g)
	cc := NewCurvatureCalculator(0.5)

	for i := 0; i < len(nodes); i++ {
		for j := 0; j < len(nodes); j++ {
			if i == j {
				continue
			}
			kappa := cc.Calculate(ig, nodes[i], nodes[j])
			// In a complete graph, curvature should be non-negative (positive).
			if kappa < -0.05 {
				t.Errorf("K4 edge (%s,%s): expected non-negative curvature, got %f",
					nodes[i], nodes[j], kappa)
			}
			// Log the value for inspection.
			t.Logf("K4 edge (%s,%s): curvature = %f", nodes[i], nodes[j], kappa)
		}
	}
}

func TestCurvature_BridgeEdge(t *testing.T) {
	// Two dense clusters connected by a single bridge edge.
	// The bridge edge should have negative curvature (bottleneck).
	//
	// Cluster 1: A-B, A-C, B-C (triangle)
	// Cluster 2: D-E, D-F, E-F (triangle)
	// Bridge: C-D
	g := graph.New()
	ts := time.Now()

	// Cluster 1 (fully connected triangle).
	addBidirectional(g, "A", "B", 1.0, ts)
	addBidirectional(g, "A", "C", 1.0, ts)
	addBidirectional(g, "B", "C", 1.0, ts)

	// Cluster 2 (fully connected triangle).
	addBidirectional(g, "D", "E", 1.0, ts)
	addBidirectional(g, "D", "F", 1.0, ts)
	addBidirectional(g, "E", "F", 1.0, ts)

	// Bridge edge.
	addBidirectional(g, "C", "D", 1.0, ts)

	ig := buildImmutable(g)
	cc := NewCurvatureCalculator(0.5)

	// The bridge edge C->D should have negative curvature.
	bridgeCurvature := cc.Calculate(ig, "C", "D")
	t.Logf("Bridge edge (C,D): curvature = %f", bridgeCurvature)
	if bridgeCurvature >= 0.0 {
		t.Errorf("Bridge edge (C,D): expected negative curvature, got %f", bridgeCurvature)
	}

	// An internal triangle edge (A->B) should have higher curvature than the bridge.
	internalCurvature := cc.Calculate(ig, "A", "B")
	t.Logf("Internal edge (A,B): curvature = %f", internalCurvature)
	if internalCurvature <= bridgeCurvature {
		t.Errorf("Internal edge should have higher curvature than bridge: internal=%f, bridge=%f",
			internalCurvature, bridgeCurvature)
	}
}

func TestCurvature_TreeEdge(t *testing.T) {
	// A tree (path graph): A-B-C-D-E.
	// Interior tree edges (where both endpoints have degree >= 2) should
	// have non-positive curvature because there are no cycles to create
	// neighborhood overlap beyond the lazy walk self-mass.
	//
	// Leaf edges (where one endpoint has degree 1) can have positive
	// curvature because the lazy walk forces mass overlap: the leaf's
	// measure concentrates on {self, parent} while the parent's measure
	// spreads to other neighbors but still shares mass on {parent, leaf}.
	g := graph.New()
	ts := time.Now()

	addBidirectional(g, "A", "B", 1.0, ts)
	addBidirectional(g, "B", "C", 1.0, ts)
	addBidirectional(g, "C", "D", 1.0, ts)
	addBidirectional(g, "D", "E", 1.0, ts)

	ig := buildImmutable(g)
	cc := NewCurvatureCalculator(0.5)

	// Interior edges (B-C, C-D) where both endpoints have degree >= 2
	// should have non-positive curvature (or very close to zero).
	interiorEdges := [][2]string{{"B", "C"}, {"C", "D"}}
	for _, e := range interiorEdges {
		kappa := cc.Calculate(ig, e[0], e[1])
		t.Logf("Interior tree edge (%s,%s): curvature = %f", e[0], e[1], kappa)
		if kappa > 0.05 {
			t.Errorf("Interior tree edge (%s,%s): expected non-positive curvature, got %f",
				e[0], e[1], kappa)
		}
	}

	// Leaf edges (A-B, D-E): curvature can be positive due to lazy walk
	// mass concentration; just verify they are finite and that interior
	// edges have strictly lower curvature than leaf edges.
	leafEdges := [][2]string{{"A", "B"}, {"D", "E"}}
	for _, e := range leafEdges {
		kappa := cc.Calculate(ig, e[0], e[1])
		t.Logf("Leaf tree edge (%s,%s): curvature = %f", e[0], e[1], kappa)
		if math.IsNaN(kappa) || math.IsInf(kappa, 0) {
			t.Errorf("Leaf tree edge (%s,%s): curvature is NaN/Inf", e[0], e[1])
		}
	}

	// Interior edges should have lower curvature than leaf edges.
	interiorK := cc.Calculate(ig, "B", "C")
	leafK := cc.Calculate(ig, "A", "B")
	if interiorK >= leafK {
		t.Errorf("Interior edge curvature (%f) should be less than leaf edge curvature (%f)",
			interiorK, leafK)
	}
}

func TestCurvature_SelfConsistency(t *testing.T) {
	// Verify that symmetric edges produce consistent curvature values.
	// For a bidirectional graph with uniform weights, kappa(x,y) and
	// kappa(y,x) should be very close (within tolerance).
	g := graph.New()
	ts := time.Now()

	addBidirectional(g, "A", "B", 1.0, ts)
	addBidirectional(g, "A", "C", 1.0, ts)
	addBidirectional(g, "B", "C", 1.0, ts)
	addBidirectional(g, "B", "D", 1.0, ts)

	ig := buildImmutable(g)
	cc := NewCurvatureCalculator(0.5)

	// For bidirectional edges, curvature should be symmetric.
	pairs := [][2]string{{"A", "B"}, {"A", "C"}, {"B", "C"}, {"B", "D"}}
	for _, p := range pairs {
		kFwd := cc.Calculate(ig, p[0], p[1])
		kRev := cc.Calculate(ig, p[1], p[0])
		t.Logf("Edge (%s,%s): forward=%f, reverse=%f", p[0], p[1], kFwd, kRev)
		if math.Abs(kFwd-kRev) > 1e-6 {
			t.Errorf("Edge (%s,%s): curvature asymmetry: forward=%f, reverse=%f",
				p[0], p[1], kFwd, kRev)
		}
	}
}

func TestCurvature_CalculateAll(t *testing.T) {
	// Verify that CalculateAll returns a value for every edge.
	g := graph.New()
	ts := time.Now()

	addBidirectional(g, "A", "B", 1.0, ts)
	addBidirectional(g, "B", "C", 1.0, ts)
	addBidirectional(g, "A", "C", 1.0, ts)

	ig := buildImmutable(g)
	cc := NewCurvatureCalculator(0.5)

	result := cc.CalculateAll(ig)

	// The graph has 6 directed edges (3 bidirectional pairs).
	expectedEdgeCount := ig.EdgeCount()
	if len(result) != expectedEdgeCount {
		t.Fatalf("CalculateAll returned %d entries, expected %d", len(result), expectedEdgeCount)
	}

	// Verify all entries are finite numbers.
	for key, val := range result {
		if math.IsNaN(val) || math.IsInf(val, 0) {
			t.Errorf("Edge (%s,%s): curvature is NaN/Inf", key[0], key[1])
		}
		t.Logf("Edge (%s,%s): curvature = %f", key[0], key[1], val)
	}
}

func TestCurvature_SingleEdge(t *testing.T) {
	// Two nodes connected by a single edge.
	// Neither node has other neighbors, so the lazy walk measures
	// place alpha mass on self and (1-alpha) on the single neighbor.
	// The measures should be "swapped" versions of each other.
	g := graph.New()
	ts := time.Now()

	addBidirectional(g, "X", "Y", 1.0, ts)

	ig := buildImmutable(g)
	cc := NewCurvatureCalculator(0.5)

	kappa := cc.Calculate(ig, "X", "Y")
	t.Logf("Single edge (X,Y): curvature = %f", kappa)

	// For two nodes with a single edge and alpha=0.5:
	// mu_X = {X: 0.5, Y: 0.5}
	// mu_Y = {X: 0.5, Y: 0.5}
	// The distributions are identical, so W1 = 0, and kappa = 1.
	// With Sinkhorn regularization there may be a small deviation.
	if math.Abs(kappa-1.0) > 0.05 {
		t.Errorf("Single edge (X,Y): expected curvature near 1.0, got %f", kappa)
	}

	// Should not return NaN or Inf.
	if math.IsNaN(kappa) || math.IsInf(kappa, 0) {
		t.Fatalf("Single edge (X,Y): curvature is NaN/Inf")
	}
}

func TestCurvature_NonExistentEdge(t *testing.T) {
	// Requesting curvature for a non-existent edge should return 0.
	g := graph.New()
	ts := time.Now()

	g.AddEdge("A", "B", 1.0, "test", ts)

	ig := buildImmutable(g)
	cc := NewCurvatureCalculator(0.5)

	kappa := cc.Calculate(ig, "A", "C")
	if kappa != 0 {
		t.Errorf("Non-existent edge: expected 0, got %f", kappa)
	}
}

func TestCurvature_DenseVsSparse(t *testing.T) {
	// Verify the qualitative property: edges in denser neighborhoods
	// should have higher curvature than edges in sparser neighborhoods.
	g := graph.New()
	ts := time.Now()

	// Dense cluster: 4-clique on {A, B, C, D}.
	clique := []string{"A", "B", "C", "D"}
	for i := 0; i < len(clique); i++ {
		for j := 0; j < len(clique); j++ {
			if i != j {
				g.AddEdge(clique[i], clique[j], 1.0, "test", ts)
			}
		}
	}

	// Sparse appendage: E connected only to D, F connected only to E.
	addBidirectional(g, "D", "E", 1.0, ts)
	addBidirectional(g, "E", "F", 1.0, ts)

	ig := buildImmutable(g)
	cc := NewCurvatureCalculator(0.5)

	// Dense internal edge.
	denseCurv := cc.Calculate(ig, "A", "B")

	// Sparse tree-like edge.
	sparseCurv := cc.Calculate(ig, "E", "F")

	t.Logf("Dense edge (A,B): curvature = %f", denseCurv)
	t.Logf("Sparse edge (E,F): curvature = %f", sparseCurv)

	if denseCurv <= sparseCurv {
		t.Errorf("Dense edge should have higher curvature than sparse: dense=%f, sparse=%f",
			denseCurv, sparseCurv)
	}
}

func TestSinkhorn_IdenticalDistributions(t *testing.T) {
	// When both distributions are identical, the Wasserstein distance should be ~0.
	mu := []float64{0.25, 0.25, 0.25, 0.25}
	nu := []float64{0.25, 0.25, 0.25, 0.25}
	cost := [][]float64{
		{0, 1, 1, 1},
		{1, 0, 1, 1},
		{1, 1, 0, 1},
		{1, 1, 1, 0},
	}

	w1 := sinkhorn(mu, nu, cost, 0.1, 100)
	// Sinkhorn regularization introduces a small bias; use a tolerance
	// that accommodates the entropic regularization error.
	if math.Abs(w1) > 0.01 {
		t.Errorf("Sinkhorn with identical distributions: expected ~0, got %f", w1)
	}
}

func TestSinkhorn_PointMasses(t *testing.T) {
	// Two point masses at different locations with distance 2.
	// W1 should be 2.
	mu := []float64{1.0, 0.0}
	nu := []float64{0.0, 1.0}
	cost := [][]float64{
		{0, 2},
		{2, 0},
	}

	w1 := sinkhorn(mu, nu, cost, 0.01, 200)
	if math.Abs(w1-2.0) > 0.1 {
		t.Errorf("Sinkhorn with point masses distance 2: expected ~2.0, got %f", w1)
	}
}

func TestBfsDistances(t *testing.T) {
	// Build a simple path graph: A - B - C - D.
	g := graph.New()
	ts := time.Now()

	addBidirectional(g, "A", "B", 1.0, ts)
	addBidirectional(g, "B", "C", 1.0, ts)
	addBidirectional(g, "C", "D", 1.0, ts)

	ig := buildImmutable(g)

	dist := bfsDistances(ig, "A", 4)

	if dist["A"] != 0 {
		t.Errorf("BFS distance A->A: expected 0, got %d", dist["A"])
	}
	if dist["B"] != 1 {
		t.Errorf("BFS distance A->B: expected 1, got %d", dist["B"])
	}
	if dist["C"] != 2 {
		t.Errorf("BFS distance A->C: expected 2, got %d", dist["C"])
	}
	if dist["D"] != 3 {
		t.Errorf("BFS distance A->D: expected 3, got %d", dist["D"])
	}
}

func TestBfsDistances_MaxDepth(t *testing.T) {
	// Verify BFS respects the depth limit.
	g := graph.New()
	ts := time.Now()

	addBidirectional(g, "A", "B", 1.0, ts)
	addBidirectional(g, "B", "C", 1.0, ts)
	addBidirectional(g, "C", "D", 1.0, ts)

	ig := buildImmutable(g)

	dist := bfsDistances(ig, "A", 2)

	if _, ok := dist["D"]; ok {
		t.Errorf("BFS with maxDepth=2 should not reach D (distance 3)")
	}
	if dist["C"] != 2 {
		t.Errorf("BFS distance A->C: expected 2, got %d", dist["C"])
	}
}
