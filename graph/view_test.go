package graph

import (
	"sort"
	"testing"
	"time"
)

// helper to build an ImmutableGraph from nodes and edges.
func buildImmutable(nodes []*NodeData, edges []EdgeData) *ImmutableGraph {
	g := New()
	for _, n := range nodes {
		g.AddNode(n)
	}
	for _, e := range edges {
		g.AddEdge(e.From, e.To, e.Weight, e.Type, e.FirstSeen)
	}
	return NewImmutable(g)
}

func TestUnifiedView_NodeOnlyInBase(t *testing.T) {
	ts := time.Now()
	base := buildImmutable(
		[]*NodeData{{ID: "a", Type: "base", FirstSeen: ts, LastSeen: ts}},
		nil,
	)
	overlay := NewImmutableEmpty()
	v := NewUnifiedView(base, overlay)

	n, ok := v.GetNode("a")
	if !ok {
		t.Fatal("expected node a from base")
	}
	if n.Type != "base" {
		t.Fatalf("expected type 'base', got %q", n.Type)
	}
}

func TestUnifiedView_NodeOnlyInOverlay(t *testing.T) {
	ts := time.Now()
	base := NewImmutableEmpty()
	overlay := buildImmutable(
		[]*NodeData{{ID: "b", Type: "overlay", FirstSeen: ts, LastSeen: ts}},
		nil,
	)
	v := NewUnifiedView(base, overlay)

	n, ok := v.GetNode("b")
	if !ok {
		t.Fatal("expected node b from overlay")
	}
	if n.Type != "overlay" {
		t.Fatalf("expected type 'overlay', got %q", n.Type)
	}
}

func TestUnifiedView_NodeInBoth_OverlayWins(t *testing.T) {
	ts := time.Now()
	base := buildImmutable(
		[]*NodeData{{ID: "a", Type: "base-type", FirstSeen: ts, LastSeen: ts}},
		nil,
	)
	overlay := buildImmutable(
		[]*NodeData{{ID: "a", Type: "overlay-type", FirstSeen: ts, LastSeen: ts}},
		nil,
	)
	v := NewUnifiedView(base, overlay)

	n, ok := v.GetNode("a")
	if !ok {
		t.Fatal("expected node a")
	}
	if n.Type != "overlay-type" {
		t.Fatalf("expected overlay type, got %q", n.Type)
	}
}

func TestUnifiedView_EdgeInBoth_WeightsSummed(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	baseG := New()
	baseG.AddNode(&NodeData{ID: "a", FirstSeen: t1, LastSeen: t1})
	baseG.AddNode(&NodeData{ID: "b", FirstSeen: t1, LastSeen: t1})
	baseG.AddEdge("a", "b", 2.0, "tx", t1)
	base := NewImmutable(baseG)

	overlayG := New()
	overlayG.AddNode(&NodeData{ID: "a", FirstSeen: t2, LastSeen: t2})
	overlayG.AddNode(&NodeData{ID: "b", FirstSeen: t2, LastSeen: t2})
	overlayG.AddEdge("a", "b", 3.0, "tx-overlay", t2)
	overlay := NewImmutable(overlayG)

	v := NewUnifiedView(base, overlay)

	e, ok := v.GetEdge("a", "b")
	if !ok {
		t.Fatal("expected edge a->b")
	}
	if e.Weight != 5.0 {
		t.Fatalf("expected weight 5.0, got %f", e.Weight)
	}
	if e.Count != 2 {
		t.Fatalf("expected count 2, got %d", e.Count)
	}
	if e.Type != "tx-overlay" {
		t.Fatalf("expected overlay type, got %q", e.Type)
	}
	if !e.FirstSeen.Equal(t1) {
		t.Fatalf("expected FirstSeen %v, got %v", t1, e.FirstSeen)
	}
	if !e.LastSeen.Equal(t2) {
		t.Fatalf("expected LastSeen %v, got %v", t2, e.LastSeen)
	}
}

func TestUnifiedView_NeighborsDeduplicated(t *testing.T) {
	ts := time.Now()
	// base: a->b, a->c
	baseG := New()
	baseG.AddEdge("a", "b", 1, "tx", ts)
	baseG.AddEdge("a", "c", 1, "tx", ts)
	base := NewImmutable(baseG)

	// overlay: a->b, a->d
	overlayG := New()
	overlayG.AddEdge("a", "b", 1, "tx", ts)
	overlayG.AddEdge("a", "d", 1, "tx", ts)
	overlay := NewImmutable(overlayG)

	v := NewUnifiedView(base, overlay)

	out := v.OutNeighbors("a")
	sort.Strings(out)
	if len(out) != 3 {
		t.Fatalf("expected 3 out-neighbors, got %d: %v", len(out), out)
	}
	expected := []string{"b", "c", "d"}
	for i, id := range expected {
		if out[i] != id {
			t.Fatalf("expected out[%d]=%q, got %q", i, id, out[i])
		}
	}

	// in-neighbors of b: should be just "a" deduplicated
	in := v.InNeighbors("b")
	if len(in) != 1 || in[0] != "a" {
		t.Fatalf("expected in-neighbors [a], got %v", in)
	}

	// all neighbors of a: b, c, d
	all := v.Neighbors("a")
	sort.Strings(all)
	if len(all) != 3 {
		t.Fatalf("expected 3 neighbors, got %d: %v", len(all), all)
	}
}

func TestUnifiedView_ForEachNode_NoDuplicates(t *testing.T) {
	ts := time.Now()
	base := buildImmutable(
		[]*NodeData{
			{ID: "a", Type: "base", FirstSeen: ts, LastSeen: ts},
			{ID: "b", Type: "base", FirstSeen: ts, LastSeen: ts},
		},
		nil,
	)
	overlay := buildImmutable(
		[]*NodeData{
			{ID: "a", Type: "overlay", FirstSeen: ts, LastSeen: ts},
			{ID: "c", Type: "overlay", FirstSeen: ts, LastSeen: ts},
		},
		nil,
	)
	v := NewUnifiedView(base, overlay)

	seen := make(map[string]string) // id -> type
	v.ForEachNode(func(n *NodeData) bool {
		if _, exists := seen[n.ID]; exists {
			t.Fatalf("duplicate node %q", n.ID)
		}
		seen[n.ID] = n.Type
		return true
	})

	if len(seen) != 3 {
		t.Fatalf("expected 3 unique nodes, got %d", len(seen))
	}
	if seen["a"] != "overlay" {
		t.Fatalf("expected node a type 'overlay', got %q", seen["a"])
	}
	if seen["b"] != "base" {
		t.Fatalf("expected node b type 'base', got %q", seen["b"])
	}
	if seen["c"] != "overlay" {
		t.Fatalf("expected node c type 'overlay', got %q", seen["c"])
	}
}

func TestUnifiedView_ForEachEdge_MergedWeights(t *testing.T) {
	ts := time.Now()
	baseG := New()
	baseG.AddEdge("a", "b", 2.0, "tx", ts)
	baseG.AddEdge("a", "c", 1.0, "tx", ts)
	base := NewImmutable(baseG)

	overlayG := New()
	overlayG.AddEdge("a", "b", 3.0, "tx", ts)
	overlayG.AddEdge("b", "c", 4.0, "tx", ts)
	overlay := NewImmutable(overlayG)

	v := NewUnifiedView(base, overlay)

	edges := make(map[string]float64)
	v.ForEachEdge(func(e *EdgeData) bool {
		key := e.From + "->" + e.To
		edges[key] = e.Weight
		return true
	})

	if len(edges) != 3 {
		t.Fatalf("expected 3 edges, got %d: %v", len(edges), edges)
	}
	if edges["a->b"] != 5.0 {
		t.Fatalf("expected a->b weight 5.0, got %f", edges["a->b"])
	}
	if edges["a->c"] != 1.0 {
		t.Fatalf("expected a->c weight 1.0, got %f", edges["a->c"])
	}
	if edges["b->c"] != 4.0 {
		t.Fatalf("expected b->c weight 4.0, got %f", edges["b->c"])
	}
}

func TestUnifiedView_NodeCount_EdgeCount(t *testing.T) {
	ts := time.Now()
	baseG := New()
	baseG.AddEdge("a", "b", 1, "tx", ts)
	baseG.AddEdge("b", "c", 1, "tx", ts)
	base := NewImmutable(baseG)

	overlayG := New()
	overlayG.AddEdge("a", "b", 2, "tx", ts)
	overlayG.AddEdge("c", "d", 1, "tx", ts)
	overlay := NewImmutable(overlayG)

	v := NewUnifiedView(base, overlay)

	if v.NodeCount() != 4 {
		t.Fatalf("expected 4 nodes, got %d", v.NodeCount())
	}
	// edges: a->b (merged), b->c (base only), c->d (overlay only) = 3
	if v.EdgeCount() != 3 {
		t.Fatalf("expected 3 edges, got %d", v.EdgeCount())
	}
}

func TestUnifiedView_EmptyOverlay(t *testing.T) {
	ts := time.Now()
	baseG := New()
	baseG.AddEdge("a", "b", 1.0, "tx", ts)
	base := NewImmutable(baseG)
	overlay := NewImmutableEmpty()

	v := NewUnifiedView(base, overlay)

	if v.NodeCount() != 2 {
		t.Fatalf("expected 2 nodes, got %d", v.NodeCount())
	}
	if v.EdgeCount() != 1 {
		t.Fatalf("expected 1 edge, got %d", v.EdgeCount())
	}

	e, ok := v.GetEdge("a", "b")
	if !ok {
		t.Fatal("expected edge a->b")
	}
	if e.Weight != 1.0 {
		t.Fatalf("expected weight 1.0, got %f", e.Weight)
	}
}

func TestUnifiedView_EmptyBase(t *testing.T) {
	ts := time.Now()
	base := NewImmutableEmpty()
	overlayG := New()
	overlayG.AddEdge("x", "y", 5.0, "tx", ts)
	overlay := NewImmutable(overlayG)

	v := NewUnifiedView(base, overlay)

	if v.NodeCount() != 2 {
		t.Fatalf("expected 2 nodes, got %d", v.NodeCount())
	}
	if v.EdgeCount() != 1 {
		t.Fatalf("expected 1 edge, got %d", v.EdgeCount())
	}

	e, ok := v.GetEdge("x", "y")
	if !ok {
		t.Fatal("expected edge x->y")
	}
	if e.Weight != 5.0 {
		t.Fatalf("expected weight 5.0, got %f", e.Weight)
	}
}
