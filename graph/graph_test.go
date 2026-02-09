package graph

import (
	"testing"
	"time"
)

func TestGraph_AddNodeAndGet(t *testing.T) {
	g := New()
	n := &NodeData{ID: "a", Type: "test"}
	g.AddNode(n)

	got, ok := g.GetNode("a")
	if !ok {
		t.Fatal("expected node found")
	}
	if got.ID != "a" {
		t.Fatalf("expected id 'a', got %q", got.ID)
	}
}

func TestGraph_GetNode_NotFound(t *testing.T) {
	g := New()
	_, ok := g.GetNode("x")
	if ok {
		t.Fatal("expected node not found")
	}
}

func TestGraph_AddEdge(t *testing.T) {
	g := New()
	g.AddNode(&NodeData{ID: "a"})
	g.AddNode(&NodeData{ID: "b"})

	ts := time.Now()
	g.AddEdge("a", "b", 1.5, "tx", ts)

	e, ok := g.GetEdge("a", "b")
	if !ok {
		t.Fatal("expected edge found")
	}
	if e.Weight != 1.5 {
		t.Fatalf("expected weight 1.5, got %f", e.Weight)
	}
	if e.Count != 1 {
		t.Fatalf("expected count 1, got %d", e.Count)
	}
}

func TestGraph_AddEdge_Accumulates(t *testing.T) {
	g := New()
	g.AddNode(&NodeData{ID: "a"})
	g.AddNode(&NodeData{ID: "b"})

	ts := time.Now()
	g.AddEdge("a", "b", 1.0, "tx", ts)
	g.AddEdge("a", "b", 2.0, "tx", ts)

	e, _ := g.GetEdge("a", "b")
	if e.Weight != 3.0 {
		t.Fatalf("expected accumulated weight 3.0, got %f", e.Weight)
	}
	if e.Count != 2 {
		t.Fatalf("expected count 2, got %d", e.Count)
	}
}

func TestGraph_Neighbors(t *testing.T) {
	g := New()
	g.AddNode(&NodeData{ID: "a"})
	g.AddNode(&NodeData{ID: "b"})
	g.AddNode(&NodeData{ID: "c"})

	ts := time.Now()
	g.AddEdge("a", "b", 1, "", ts)
	g.AddEdge("c", "a", 1, "", ts)

	out := g.OutNeighbors("a")
	if len(out) != 1 || out[0] != "b" {
		t.Fatalf("expected out [b], got %v", out)
	}

	in := g.InNeighbors("a")
	if len(in) != 1 || in[0] != "c" {
		t.Fatalf("expected in [c], got %v", in)
	}

	all := g.Neighbors("a")
	if len(all) != 2 {
		t.Fatalf("expected 2 neighbors, got %d", len(all))
	}
}

func TestGraph_NodeCount_EdgeCount(t *testing.T) {
	g := New()
	g.AddNode(&NodeData{ID: "a"})
	g.AddNode(&NodeData{ID: "b"})

	ts := time.Now()
	g.AddEdge("a", "b", 1, "", ts)

	if g.NodeCount() != 2 {
		t.Fatalf("expected 2 nodes, got %d", g.NodeCount())
	}
	if g.EdgeCount() != 1 {
		t.Fatalf("expected 1 edge, got %d", g.EdgeCount())
	}
}

func TestGraph_Degree(t *testing.T) {
	g := New()
	g.AddNode(&NodeData{ID: "a"})
	g.AddNode(&NodeData{ID: "b"})
	g.AddNode(&NodeData{ID: "c"})

	ts := time.Now()
	g.AddEdge("a", "b", 1, "", ts)
	g.AddEdge("a", "c", 1, "", ts)
	g.AddEdge("c", "a", 1, "", ts)

	n, _ := g.GetNode("a")
	if n.OutDegree != 2 {
		t.Fatalf("expected outDegree 2, got %d", n.OutDegree)
	}
	if n.InDegree != 1 {
		t.Fatalf("expected inDegree 1, got %d", n.InDegree)
	}
	if n.Degree != 3 {
		t.Fatalf("expected degree 3, got %d", n.Degree)
	}
}

func TestGraph_AddEdge_CreatesNodes(t *testing.T) {
	g := New()
	ts := time.Now()
	g.AddEdge("x", "y", 1.0, "auto", ts)

	if g.NodeCount() != 2 {
		t.Fatalf("expected 2 auto-created nodes, got %d", g.NodeCount())
	}
	nx, ok := g.GetNode("x")
	if !ok {
		t.Fatal("expected node x")
	}
	if nx.FirstSeen != ts {
		t.Fatal("expected FirstSeen set")
	}
}

func TestGraph_SelfLoop(t *testing.T) {
	g := New()
	ts := time.Now()
	g.AddEdge("a", "a", 1.0, "self", ts)

	n, _ := g.GetNode("a")
	if n.InDegree != 1 || n.OutDegree != 1 || n.Degree != 2 {
		t.Fatalf("self-loop degree wrong: in=%d out=%d total=%d", n.InDegree, n.OutDegree, n.Degree)
	}
}
