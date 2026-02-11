package compact

import (
	"testing"
	"time"

	"github.com/MatheusGrego/itt-engine/graph"
)

func buildImmutable(nodes []*graph.NodeData, edges []graph.EdgeData) *graph.ImmutableGraph {
	g := graph.New()
	for _, n := range nodes {
		g.AddNode(n)
	}
	for _, e := range edges {
		g.AddEdge(e.From, e.To, e.Weight, e.Type, e.FirstSeen)
		// Fix up count and LastSeen to match what was specified.
		if ge, ok := g.GetEdge(e.From, e.To); ok {
			ge.Count = e.Count
			ge.LastSeen = e.LastSeen
		}
	}
	return graph.NewImmutable(g)
}

func TestCompact_MergesOverlayIntoBase(t *testing.T) {
	ts := time.Now()
	base := buildImmutable(
		[]*graph.NodeData{{ID: "a", Type: "user", FirstSeen: ts, LastSeen: ts}},
		[]graph.EdgeData{{From: "a", To: "b", Weight: 1.0, Type: "tx", Count: 1, FirstSeen: ts, LastSeen: ts}},
	)
	overlay := buildImmutable(
		[]*graph.NodeData{{ID: "c", Type: "service", FirstSeen: ts, LastSeen: ts}},
		[]graph.EdgeData{{From: "c", To: "a", Weight: 2.0, Type: "call", Count: 1, FirstSeen: ts, LastSeen: ts}},
	)

	merged, _ := Compact(base, overlay)

	if merged.NodeCount() < 3 {
		t.Fatalf("expected at least 3 nodes, got %d", merged.NodeCount())
	}
	if merged.EdgeCount() != 2 {
		t.Fatalf("expected 2 edges, got %d", merged.EdgeCount())
	}

	e1, ok := merged.GetEdge("a", "b")
	if !ok || e1.Weight != 1.0 {
		t.Fatalf("expected base edge a->b with weight 1.0, got %v", e1)
	}
	e2, ok := merged.GetEdge("c", "a")
	if !ok || e2.Weight != 2.0 {
		t.Fatalf("expected overlay edge c->a with weight 2.0, got %v", e2)
	}
}

func TestCompact_OverlayNodeOverridesBase(t *testing.T) {
	ts := time.Now()
	base := buildImmutable(
		[]*graph.NodeData{{ID: "a", Type: "old-type", FirstSeen: ts, LastSeen: ts}},
		nil,
	)
	overlay := buildImmutable(
		[]*graph.NodeData{{ID: "a", Type: "new-type", FirstSeen: ts, LastSeen: ts}},
		nil,
	)

	merged, stats := Compact(base, overlay)

	n, ok := merged.GetNode("a")
	if !ok {
		t.Fatal("expected node a")
	}
	if n.Type != "new-type" {
		t.Fatalf("expected type 'new-type', got %q", n.Type)
	}
	if stats.NodesMerged != 1 {
		t.Fatalf("expected 1 node merged, got %d", stats.NodesMerged)
	}
}

func TestCompact_EdgeWeightsAccumulate(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	base := buildImmutable(
		[]*graph.NodeData{
			{ID: "a", FirstSeen: t1, LastSeen: t1},
			{ID: "b", FirstSeen: t1, LastSeen: t1},
		},
		[]graph.EdgeData{{From: "a", To: "b", Weight: 2.0, Type: "tx", Count: 3, FirstSeen: t1, LastSeen: t1}},
	)
	overlay := buildImmutable(
		[]*graph.NodeData{
			{ID: "a", FirstSeen: t2, LastSeen: t2},
			{ID: "b", FirstSeen: t2, LastSeen: t2},
		},
		[]graph.EdgeData{{From: "a", To: "b", Weight: 5.0, Type: "tx", Count: 2, FirstSeen: t2, LastSeen: t2}},
	)

	merged, stats := Compact(base, overlay)

	e, ok := merged.GetEdge("a", "b")
	if !ok {
		t.Fatal("expected edge a->b")
	}
	if e.Weight != 7.0 {
		t.Fatalf("expected accumulated weight 7.0, got %f", e.Weight)
	}
	if e.Count != 5 {
		t.Fatalf("expected accumulated count 5, got %d", e.Count)
	}
	if !e.FirstSeen.Equal(t1) {
		t.Fatalf("expected FirstSeen %v, got %v", t1, e.FirstSeen)
	}
	if !e.LastSeen.Equal(t2) {
		t.Fatalf("expected LastSeen %v, got %v", t2, e.LastSeen)
	}
	if stats.EdgesMerged != 1 {
		t.Fatalf("expected 1 edge merged, got %d", stats.EdgesMerged)
	}
}

func TestCompact_EmptyOverlay(t *testing.T) {
	ts := time.Now()
	base := buildImmutable(
		[]*graph.NodeData{{ID: "a", FirstSeen: ts, LastSeen: ts}},
		[]graph.EdgeData{{From: "a", To: "b", Weight: 1.0, Type: "tx", Count: 1, FirstSeen: ts, LastSeen: ts}},
	)
	overlay := graph.NewImmutableEmpty()

	merged, stats := Compact(base, overlay)

	if merged.NodeCount() != 2 {
		t.Fatalf("expected 2 nodes, got %d", merged.NodeCount())
	}
	if merged.EdgeCount() != 1 {
		t.Fatalf("expected 1 edge, got %d", merged.EdgeCount())
	}
	if stats.NodesMerged != 0 {
		t.Fatalf("expected 0 nodes merged, got %d", stats.NodesMerged)
	}
	if stats.EdgesMerged != 0 {
		t.Fatalf("expected 0 edges merged, got %d", stats.EdgesMerged)
	}
}

func TestCompact_EmptyBase(t *testing.T) {
	ts := time.Now()
	base := graph.NewImmutableEmpty()
	overlay := buildImmutable(
		[]*graph.NodeData{{ID: "x", Type: "svc", FirstSeen: ts, LastSeen: ts}},
		[]graph.EdgeData{{From: "x", To: "y", Weight: 3.0, Type: "call", Count: 1, FirstSeen: ts, LastSeen: ts}},
	)

	merged, _ := Compact(base, overlay)

	if merged.NodeCount() != 2 {
		t.Fatalf("expected 2 nodes, got %d", merged.NodeCount())
	}
	if merged.EdgeCount() != 1 {
		t.Fatalf("expected 1 edge, got %d", merged.EdgeCount())
	}
	e, ok := merged.GetEdge("x", "y")
	if !ok {
		t.Fatal("expected edge x->y")
	}
	if e.Weight != 3.0 {
		t.Fatalf("expected weight 3.0, got %f", e.Weight)
	}
}

func TestCompact_Stats(t *testing.T) {
	ts := time.Now()
	base := buildImmutable(
		[]*graph.NodeData{
			{ID: "a", Type: "base", FirstSeen: ts, LastSeen: ts},
			{ID: "b", Type: "base", FirstSeen: ts, LastSeen: ts},
		},
		[]graph.EdgeData{{From: "a", To: "b", Weight: 1.0, Type: "tx", Count: 1, FirstSeen: ts, LastSeen: ts}},
	)
	// Overlay shares node "a" with base and adds a new node "c" with
	// a new edge c->a. This avoids auto-creating "b" in the overlay,
	// so only "a" counts as a merged node.
	overlay := buildImmutable(
		[]*graph.NodeData{
			{ID: "a", Type: "overlay", FirstSeen: ts, LastSeen: ts},
			{ID: "c", Type: "overlay", FirstSeen: ts, LastSeen: ts},
		},
		[]graph.EdgeData{
			{From: "c", To: "a", Weight: 1.0, Type: "tx", Count: 1, FirstSeen: ts, LastSeen: ts},
		},
	)

	_, stats := Compact(base, overlay)

	// "a" is the only node that exists in both base and overlay.
	if stats.NodesMerged != 1 {
		t.Fatalf("expected 1 node merged (a), got %d", stats.NodesMerged)
	}
	// No edges overlap between base (a->b) and overlay (c->a).
	if stats.EdgesMerged != 0 {
		t.Fatalf("expected 0 edges merged, got %d", stats.EdgesMerged)
	}
	if stats.OverlayAfter != 0 {
		t.Fatalf("expected OverlayAfter 0, got %d", stats.OverlayAfter)
	}
	if stats.Duration < 0 {
		t.Fatal("expected non-negative duration")
	}
	if stats.Timestamp.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}
}

func TestShouldCompact_ByVolume(t *testing.T) {
	last := time.Now().Add(-time.Minute)

	if !shouldCompact(ByVolume, 100, 100, last, time.Hour) {
		t.Fatal("expected true when overlayEvents == threshold")
	}
	if !shouldCompact(ByVolume, 150, 100, last, time.Hour) {
		t.Fatal("expected true when overlayEvents > threshold")
	}
	if shouldCompact(ByVolume, 50, 100, last, time.Hour) {
		t.Fatal("expected false when overlayEvents < threshold")
	}
}

func TestShouldCompact_ByTime(t *testing.T) {
	if !shouldCompact(ByTime, 0, 0, time.Now().Add(-2*time.Hour), time.Hour) {
		t.Fatal("expected true when interval exceeded")
	}
	if shouldCompact(ByTime, 0, 0, time.Now().Add(-30*time.Minute), time.Hour) {
		t.Fatal("expected false when interval not reached")
	}
}

func TestShouldCompact_Manual(t *testing.T) {
	if shouldCompact(Manual, 1000, 1, time.Time{}, time.Nanosecond) {
		t.Fatal("Manual should always return false")
	}
}
