package analysis

import (
	"math"
	"testing"
	"time"

	"github.com/MatheusGrego/itt-engine/graph"
)

func TestConcealmentCost_IsolatedNode(t *testing.T) {
	// A node with no neighbors has tension = 0.
	// Concealment cost = tau(node) * exp(-lambda*0) = 0 * 1 = 0.
	g := graph.New()
	g.AddNode(&graph.NodeData{ID: "A"})
	ig := graph.NewImmutable(g)

	tc := NewTensionCalculator(JSD{})
	cc := NewConcealmentCalculator(1.0, tc)

	cost := cc.CalculateNode(ig, "A", 3)
	if cost != 0 {
		t.Fatalf("expected concealment cost 0 for isolated node, got %f", cost)
	}
}

func TestConcealmentCost_HighDegreeNode(t *testing.T) {
	// Hub node H has many neighbors contributing tension at k=1.
	// A leaf node L1 has fewer neighbors.
	// Hub should have higher concealment cost than leaf.
	ts := time.Now()
	g := graph.New()

	// Hub H connected to 5 leaves with bidirectional edges.
	leaves := []string{"L1", "L2", "L3", "L4", "L5"}
	for _, leaf := range leaves {
		g.AddEdge("H", leaf, 1.0, "test", ts)
		g.AddEdge(leaf, "H", 1.0, "test", ts)
	}
	// Give leaves some cross-connections so they have nonzero tension.
	g.AddEdge("L1", "L2", 1.0, "test", ts)
	g.AddEdge("L2", "L3", 1.0, "test", ts)
	g.AddEdge("L3", "L4", 1.0, "test", ts)
	g.AddEdge("L4", "L5", 1.0, "test", ts)
	g.AddEdge("L5", "L1", 1.0, "test", ts)

	ig := graph.NewImmutable(g)

	tc := NewTensionCalculator(JSD{})
	cc := NewConcealmentCalculator(1.0, tc)

	hubCost := cc.CalculateNode(ig, "H", 2)
	leafCost := cc.CalculateNode(ig, "L1", 2)

	if hubCost <= leafCost {
		t.Fatalf("expected hub concealment cost (%f) > leaf concealment cost (%f)", hubCost, leafCost)
	}
}

func TestConcealmentCost_ExponentialDecay(t *testing.T) {
	// Build a chain: A -> B -> C -> D -> E (with bidirectional edges so
	// BFS can traverse and tension is nonzero due to multi-out-edge nodes).
	ts := time.Now()
	g := graph.New()

	// Bidirectional chain so that neighbors are reachable and tension > 0
	// for nodes with multiple out-edges.
	g.AddEdge("A", "B", 1.0, "test", ts)
	g.AddEdge("B", "A", 1.0, "test", ts)
	g.AddEdge("B", "C", 1.0, "test", ts)
	g.AddEdge("C", "B", 1.0, "test", ts)
	g.AddEdge("C", "D", 1.0, "test", ts)
	g.AddEdge("D", "C", 1.0, "test", ts)
	g.AddEdge("D", "E", 1.0, "test", ts)
	g.AddEdge("E", "D", 1.0, "test", ts)

	ig := graph.NewImmutable(g)

	tc := NewTensionCalculator(JSD{})
	cc := NewConcealmentCalculator(1.0, tc)

	// More hops should capture more distant neighbors, increasing cost.
	cost1 := cc.CalculateNode(ig, "A", 1)
	cost2 := cc.CalculateNode(ig, "A", 2)
	cost3 := cc.CalculateNode(ig, "A", 3)

	if cost1 <= 0 {
		t.Fatalf("expected positive cost at maxHops=1, got %f", cost1)
	}
	if cost2 <= cost1 {
		t.Fatalf("expected cost(maxHops=2)=%f > cost(maxHops=1)=%f", cost2, cost1)
	}
	if cost3 <= cost2 {
		t.Fatalf("expected cost(maxHops=3)=%f > cost(maxHops=2)=%f", cost3, cost2)
	}
}

func TestConcealmentCost_NodeSet(t *testing.T) {
	// Cost of set {A, B} >= cost of {A}.
	ts := time.Now()
	g := graph.New()

	g.AddEdge("A", "B", 1.0, "test", ts)
	g.AddEdge("B", "A", 1.0, "test", ts)
	g.AddEdge("B", "C", 1.0, "test", ts)
	g.AddEdge("C", "B", 1.0, "test", ts)
	g.AddEdge("C", "D", 1.0, "test", ts)
	g.AddEdge("D", "C", 1.0, "test", ts)

	ig := graph.NewImmutable(g)

	tc := NewTensionCalculator(JSD{})
	cc := NewConcealmentCalculator(1.0, tc)

	costA := cc.Calculate(ig, []string{"A"}, 2)
	costAB := cc.Calculate(ig, []string{"A", "B"}, 2)

	if costAB < costA {
		t.Fatalf("expected cost({A,B})=%f >= cost({A})=%f", costAB, costA)
	}
}

func TestConcealmentCost_MissingNode(t *testing.T) {
	// Missing node should return 0.
	g := graph.New()
	g.AddNode(&graph.NodeData{ID: "A"})
	ig := graph.NewImmutable(g)

	tc := NewTensionCalculator(JSD{})
	cc := NewConcealmentCalculator(1.0, tc)

	cost := cc.CalculateNode(ig, "MISSING", 3)
	if cost != 0 {
		t.Fatalf("expected concealment cost 0 for missing node, got %f", cost)
	}
}

func TestConcealmentCost_LambdaEffect(t *testing.T) {
	// Higher lambda should decay contributions faster, resulting in lower cost
	// for the same graph (when there are multi-hop contributions).
	ts := time.Now()
	g := graph.New()

	g.AddEdge("A", "B", 1.0, "test", ts)
	g.AddEdge("B", "A", 1.0, "test", ts)
	g.AddEdge("B", "C", 1.0, "test", ts)
	g.AddEdge("C", "B", 1.0, "test", ts)

	ig := graph.NewImmutable(g)

	tc := NewTensionCalculator(JSD{})
	ccLow := NewConcealmentCalculator(0.5, tc)
	ccHigh := NewConcealmentCalculator(2.0, tc)

	costLow := ccLow.CalculateNode(ig, "A", 2)
	costHigh := ccHigh.CalculateNode(ig, "A", 2)

	if costLow <= costHigh {
		t.Fatalf("expected lower lambda (%f) to give higher cost than higher lambda (%f)", costLow, costHigh)
	}

	// Both should be positive.
	if costLow <= 0 || costHigh <= 0 {
		t.Fatalf("expected positive costs, got low=%f, high=%f", costLow, costHigh)
	}
}

func TestConcealmentCost_NonNegative(t *testing.T) {
	// Concealment cost should never be negative.
	ig := buildGraph([][3]interface{}{
		{"A", "B", 1.0},
		{"B", "C", 2.0},
		{"C", "A", 0.5},
		{"A", "C", 3.0},
	})

	tc := NewTensionCalculator(JSD{})
	cc := NewConcealmentCalculator(1.0, tc)

	ig.ForEachNode(func(n *graph.NodeData) bool {
		cost := cc.CalculateNode(ig, n.ID, 3)
		if cost < -1e-10 {
			t.Fatalf("negative concealment cost for node %s: %f", n.ID, cost)
		}
		if math.IsNaN(cost) {
			t.Fatalf("NaN concealment cost for node %s", n.ID)
		}
		return true
	})
}
