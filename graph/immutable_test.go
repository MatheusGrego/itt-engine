package graph

import (
	"testing"
	"time"
)

func TestImmutableGraph_FromGraph(t *testing.T) {
	g := New()
	g.AddNode(&NodeData{ID: "a"})
	g.AddNode(&NodeData{ID: "b"})
	g.AddEdge("a", "b", 1.0, "tx", time.Now())

	ig := NewImmutable(g)
	if ig.NodeCount() != 2 {
		t.Fatalf("expected 2 nodes, got %d", ig.NodeCount())
	}
	if ig.EdgeCount() != 1 {
		t.Fatalf("expected 1 edge, got %d", ig.EdgeCount())
	}
}

func TestImmutableGraph_DeepCopy(t *testing.T) {
	g := New()
	g.AddNode(&NodeData{ID: "a", Attributes: map[string]float64{"x": 1.0}})

	ig := NewImmutable(g)

	// Mutate original — must not affect immutable
	g.AddNode(&NodeData{ID: "c"})
	n, _ := g.GetNode("a")
	n.Attributes["x"] = 999

	if ig.NodeCount() != 1 {
		t.Fatalf("immutable should have 1 node, got %d", ig.NodeCount())
	}
	igNode, _ := ig.GetNode("a")
	if igNode.Attributes["x"] != 1.0 {
		t.Fatalf("immutable attribute mutated: got %f", igNode.Attributes["x"])
	}
}

func TestImmutableGraph_WithEvent(t *testing.T) {
	g := New()
	g.AddNode(&NodeData{ID: "a"})
	g.AddNode(&NodeData{ID: "b"})
	g.AddEdge("a", "b", 1.0, "tx", time.Now())

	ig := NewImmutable(g)
	ts := time.Now()
	ig2 := ig.WithEvent("b", "c", 2.0, "tx", ts)

	// Original unchanged
	if ig.NodeCount() != 2 {
		t.Fatalf("original should have 2 nodes, got %d", ig.NodeCount())
	}
	_, ok := ig.GetNode("c")
	if ok {
		t.Fatal("original should not have node c")
	}

	// New version has the update
	if ig2.NodeCount() != 3 {
		t.Fatalf("new should have 3 nodes, got %d", ig2.NodeCount())
	}
	_, ok = ig2.GetNode("c")
	if !ok {
		t.Fatal("new should have node c")
	}
}

func TestImmutableGraph_GraphView(t *testing.T) {
	g := New()
	g.AddNode(&NodeData{ID: "a"})
	g.AddNode(&NodeData{ID: "b"})
	g.AddEdge("a", "b", 1.0, "tx", time.Now())

	ig := NewImmutable(g)

	n, ok := ig.GetNode("a")
	if !ok || n.ID != "a" {
		t.Fatal("GetNode failed")
	}
	e, ok := ig.GetEdge("a", "b")
	if !ok || e.Weight != 1.0 {
		t.Fatal("GetEdge failed")
	}
	neighbors := ig.Neighbors("a")
	if len(neighbors) != 1 {
		t.Fatalf("expected 1 neighbor, got %d", len(neighbors))
	}
}

func TestImmutableGraph_WithEvent_PreservesOriginalEdges(t *testing.T) {
	g := New()
	g.AddEdge("a", "b", 1.0, "tx", time.Now())

	ig := NewImmutable(g)
	ig2 := ig.WithEvent("a", "b", 3.0, "tx", time.Now())

	// Original edge weight unchanged
	e1, _ := ig.GetEdge("a", "b")
	if e1.Weight != 1.0 {
		t.Fatalf("original edge weight mutated: got %f", e1.Weight)
	}

	// New version has accumulated weight
	e2, _ := ig2.GetEdge("a", "b")
	if e2.Weight != 4.0 {
		t.Fatalf("expected accumulated weight 4.0, got %f", e2.Weight)
	}
}

func TestImmutableGraph_Empty(t *testing.T) {
	ig := NewImmutableEmpty()
	if ig.NodeCount() != 0 {
		t.Fatalf("expected 0 nodes, got %d", ig.NodeCount())
	}
	if ig.EdgeCount() != 0 {
		t.Fatalf("expected 0 edges, got %d", ig.EdgeCount())
	}
}
