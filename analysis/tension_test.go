package analysis

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/MatheusGrego/itt-engine/graph"
)

// helper to build an immutable graph from edges.
func buildGraph(edges [][3]interface{}) *graph.ImmutableGraph {
	g := graph.New()
	ts := time.Now()
	for _, e := range edges {
		from := e[0].(string)
		to := e[1].(string)
		w := e[2].(float64)
		g.AddEdge(from, to, w, "test", ts)
	}
	return graph.NewImmutable(g)
}

func TestTension_IsolatedNode(t *testing.T) {
	// A node with no neighbors should have tension = 0.
	g := graph.New()
	g.AddNode(&graph.NodeData{ID: "A"})
	ig := graph.NewImmutable(g)

	tc := NewTensionCalculator(JSD{})
	tension := tc.Calculate(ig, "A")
	if tension != 0 {
		t.Fatalf("expected tension 0 for isolated node, got %f", tension)
	}
}

func TestTension_NodeNotFound(t *testing.T) {
	g := graph.New()
	g.AddNode(&graph.NodeData{ID: "A"})
	ig := graph.NewImmutable(g)

	tc := NewTensionCalculator(JSD{})
	tension := tc.Calculate(ig, "MISSING")
	if tension != 0 {
		t.Fatalf("expected tension 0 for missing node, got %f", tension)
	}
}

func TestTension_SingleEdge(t *testing.T) {
	// A->B with weight 1. B's only outgoing? No, B has no outgoing edges.
	// A has outgoing to B only. B is a neighbor of A.
	// For the tension of B:
	//   Neighbor A has out-neighbors = [B]. Original dist = [1.0].
	//   Perturbed: zero out B => [0.0], normalized => uniform [1.0].
	//   Divergence(JSD) of [1.0] vs [1.0] = 0.
	// Actually, with a single out-neighbor, removing the target leaves all zeros,
	// which Normalize converts to uniform. Both become [1.0]. JSD = 0.
	// B1: that was the bug. Calculate now returns the divergence maximum
	// (JSD = 1) for p = 1; see TestTension_B1_SoleTargetGivesMaxDivergence.
	//
	// Let's instead test tension of A in a graph A->B, A->C.
	// Neighbors of A: B and C (via out-edges).
	// B has no out-edges => skip. C has no out-edges => skip. Tension = 0.
	//
	// Better test: B->A and B->C. Tension of A:
	//   Neighbor B: out-neighbors = [A, C]. Original = normalize([1, 1]) = [0.5, 0.5].
	//   Perturbed: zero A => [0, 1] => normalize => [0, 1].
	//   JSD([0.5, 0.5], [0, 1]) > 0.
	ig := buildGraph([][3]interface{}{
		{"B", "A", 1.0},
		{"B", "C", 1.0},
	})

	tc := NewTensionCalculator(JSD{})
	tension := tc.Calculate(ig, "A")
	if tension <= 0 {
		t.Fatalf("expected positive tension for single-edge node, got %f", tension)
	}
}

func TestTension_HubHigherThanLeaf(t *testing.T) {
	// Hub H is connected to many leaves. Each leaf sends edges to H and
	// to one other leaf, giving them 2 out-edges each. H sends edges to
	// all leaves.
	//
	// Hub tension: each leaf has 2 out-edges (one to H, one to another leaf).
	//   Removing H zeroes out half the distribution => large divergence.
	//   5 such neighbors => high average divergence.
	//
	// Leaf (L1) tension: H has 5 out-edges, removing L1 zeroes 1/5 => smaller divergence.
	//   L5 has 2 out-edges (to H and L1), removing L1 zeroes 1/2 => moderate divergence.
	//   Average across these 2 neighbors is less than hub's average across 5 neighbors,
	//   because the hub's neighbors are more uniformly and severely disrupted.
	ig := buildGraph([][3]interface{}{
		{"H", "L1", 1.0},
		{"H", "L2", 1.0},
		{"H", "L3", 1.0},
		{"H", "L4", 1.0},
		{"H", "L5", 1.0},
		{"L1", "H", 1.0},
		{"L1", "L2", 1.0},
		{"L2", "H", 1.0},
		{"L2", "L3", 1.0},
		{"L3", "H", 1.0},
		{"L3", "L4", 1.0},
		{"L4", "H", 1.0},
		{"L4", "L5", 1.0},
		{"L5", "H", 1.0},
		{"L5", "L1", 1.0},
	})

	tc := NewTensionCalculator(JSD{})
	hubTension := tc.Calculate(ig, "H")
	leafTension := tc.Calculate(ig, "L1")

	if hubTension <= leafTension {
		t.Fatalf("expected hub tension (%f) > leaf tension (%f)", hubTension, leafTension)
	}
}

func TestTension_AlwaysNonNegative(t *testing.T) {
	// Test with several topologies.
	topologies := []struct {
		name  string
		edges [][3]interface{}
	}{
		{
			name: "triangle",
			edges: [][3]interface{}{
				{"A", "B", 1.0},
				{"B", "C", 2.0},
				{"C", "A", 0.5},
			},
		},
		{
			name: "chain",
			edges: [][3]interface{}{
				{"A", "B", 1.0},
				{"B", "C", 1.0},
				{"C", "D", 1.0},
			},
		},
		{
			name: "mixed-weights",
			edges: [][3]interface{}{
				{"A", "B", 10.0},
				{"A", "C", 0.1},
				{"B", "C", 5.0},
				{"C", "A", 3.0},
			},
		},
	}

	for _, div := range []DivergenceFunc{JSD{}, Hellinger{}} {
		tc := NewTensionCalculator(div)
		for _, topo := range topologies {
			ig := buildGraph(topo.edges)
			all := tc.CalculateAll(ig)
			for nodeID, val := range all {
				if val < -1e-10 {
					t.Fatalf("[%s/%s] tension of %s is negative: %f", div.Name(), topo.name, nodeID, val)
				}
			}
		}
	}
}

func TestTension_CalculateAllReturnsAllNodes(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"A", "B", 1.0},
		{"B", "C", 1.0},
		{"C", "A", 1.0},
	})

	tc := NewTensionCalculator(JSD{})
	all := tc.CalculateAll(ig)

	if len(all) != ig.NodeCount() {
		t.Fatalf("expected %d entries in CalculateAll, got %d", ig.NodeCount(), len(all))
	}

	for _, nodeID := range []string{"A", "B", "C"} {
		if _, ok := all[nodeID]; !ok {
			t.Fatalf("CalculateAll missing node %s", nodeID)
		}
	}
}

func TestTension_SymmetricGraph(t *testing.T) {
	// Complete graph with uniform weights: all tensions should be similar.
	// K4: every node connects to every other node with weight 1.
	nodes := []string{"A", "B", "C", "D"}
	var edges [][3]interface{}
	for _, from := range nodes {
		for _, to := range nodes {
			if from != to {
				edges = append(edges, [3]interface{}{from, to, 1.0})
			}
		}
	}
	ig := buildGraph(edges)

	tc := NewTensionCalculator(JSD{})
	all := tc.CalculateAll(ig)

	// All tensions should be equal (or very close).
	vals := make([]float64, 0, len(all))
	for _, v := range all {
		vals = append(vals, v)
	}

	for i := 1; i < len(vals); i++ {
		if math.Abs(vals[i]-vals[0]) > 1e-10 {
			t.Fatalf("expected all tensions equal in symmetric graph, got %v", all)
		}
	}
}

func TestTension_NeighborNoOutEdges(t *testing.T) {
	// A->B. B has no out-edges. Tension of A:
	// Neighbor B has no out-neighbors => skip.
	// No contributing neighbors => tension = 0.
	ig := buildGraph([][3]interface{}{
		{"A", "B", 1.0},
	})

	tc := NewTensionCalculator(JSD{})
	tension := tc.Calculate(ig, "A")
	if tension != 0 {
		t.Fatalf("expected tension 0 when neighbor has no out-edges, got %f", tension)
	}
}

func TestTension_DifferentDivergenceFunctions(t *testing.T) {
	ig := buildGraph([][3]interface{}{
		{"A", "B", 1.0},
		{"A", "C", 2.0},
		{"B", "A", 1.0},
		{"B", "C", 1.0},
		{"C", "A", 1.0},
	})

	for _, div := range []DivergenceFunc{JSD{}, KL{}, Hellinger{}} {
		tc := NewTensionCalculator(div)
		tension := tc.Calculate(ig, "A")
		if math.IsNaN(tension) || math.IsInf(tension, 0) {
			t.Fatalf("%s produced NaN/Inf tension", div.Name())
		}
		if tension < 0 {
			t.Fatalf("%s produced negative tension: %f", div.Name(), tension)
		}
	}
}

// jsdLeaveOneOut is the closed form of JSD(P, P∖v) when v holds a fraction p
// of P's mass (T2): it depends only on p, not on how the rest is split.
func jsdLeaveOneOut(p float64) float64 {
	if p >= 1 {
		return 1
	}
	return 0.5 * (p + (1-p)*math.Log2(2*(1-p)/(2-p)) + math.Log2(2/(2-p)))
}

func TestTension_B1_SoleTargetGivesMaxDivergence(t *testing.T) {
	// a->v is a's only out-edge (p = 1). Removing v leaves a's perturbed
	// distribution all zeros; the limit of the divergence as p -> 1 is its
	// maximum, not 0.
	ig := buildGraph([][3]interface{}{
		{"a", "v", 5.0},
	})

	cases := []struct {
		div  DivergenceFunc
		want float64
	}{
		{JSD{}, 1.0},
		{Hellinger{}, 1.0},
		{KL{}, math.Log2(1 / epsilon)},
	}
	for _, c := range cases {
		got := NewTensionCalculator(c.div).Calculate(ig, "v")
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: tension(v) = %.12f, want %.12f", c.div.Name(), got, c.want)
		}
	}
}

func TestTension_B1_HalfWeightMatchesClosedForm(t *testing.T) {
	// a->v (1), a->x (1): a sends p = 0.5 of its weight to v.
	ig := buildGraph([][3]interface{}{
		{"a", "v", 1.0},
		{"a", "x", 1.0},
	})

	got := NewTensionCalculator(JSD{}).Calculate(ig, "v")
	want := 0.311278
	if math.Abs(got-want) > 1e-5 {
		t.Fatalf("tension(v) = %.6f, want f(0.5) = %.6f", got, want)
	}
	if math.Abs(jsdLeaveOneOut(0.5)-want) > 1e-5 {
		t.Fatalf("closed form f(0.5) = %.6f, want %.6f", jsdLeaveOneOut(0.5), want)
	}
}

func TestJSD_LeaveOneOutDependsOnlyOnP(t *testing.T) {
	// T2: for any vector where v holds fraction p of the mass,
	// JSD(P, P∖v) == f(p), regardless of how the remaining mass is split.
	rng := rand.New(rand.NewPCG(1, 2))
	for _, p := range []float64{0.05, 0.3, 0.5, 0.8, 0.99} {
		want := jsdLeaveOneOut(p)
		for trial := 0; trial < 20; trial++ {
			k := 1 + rng.IntN(9) // number of other slots
			raw := make([]float64, k+1)
			rest := 0.0
			for i := 1; i <= k; i++ {
				raw[i] = 0.01 + rng.Float64()
				rest += raw[i]
			}
			// Scale so that v (slot 0) holds exactly fraction p.
			raw[0] = p / (1 - p) * rest
			scale := 1 + 100*rng.Float64()
			for i := range raw {
				raw[i] *= scale
			}

			perturbed := append([]float64(nil), raw...)
			perturbed[0] = 0

			got := JSD{}.Compute(Normalize(raw), Normalize(perturbed))
			if math.Abs(got-want) > 1e-9 {
				t.Fatalf("p=%.2f k=%d: JSD = %.12f, want f(p) = %.12f", p, k, got, want)
			}
		}
	}
}
