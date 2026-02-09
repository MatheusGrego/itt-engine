package compact

import (
	"time"

	"github.com/mfreiregr/itt-engine/graph"
)

// Strategy enumerates compaction trigger types.
type Strategy int

const (
	ByVolume Strategy = iota // trigger when overlay exceeds threshold
	ByTime                   // trigger after interval
	Manual                   // manual trigger only
)

// Stats holds compaction metrics.
type Stats struct {
	NodesMerged   int
	EdgesMerged   int
	OverlayBefore int
	OverlayAfter  int
	Duration      time.Duration
	Timestamp     time.Time
}

// Compact merges overlay into base, returning a new merged ImmutableGraph
// and compaction statistics. The original graphs are not modified.
func Compact(base, overlay *graph.ImmutableGraph) (*graph.ImmutableGraph, Stats) {
	start := time.Now()

	overlayNodes := 0
	overlayEdges := 0
	overlay.ForEachNode(func(n *graph.NodeData) bool {
		overlayNodes++
		return true
	})
	overlay.ForEachEdge(func(e *graph.EdgeData) bool {
		overlayEdges++
		return true
	})

	g := graph.New()

	// Step 1: Copy all nodes from base.
	base.ForEachNode(func(n *graph.NodeData) bool {
		g.AddNode(&graph.NodeData{
			ID:        n.ID,
			Type:      n.Type,
			Degree:    n.Degree,
			InDegree:  n.InDegree,
			OutDegree: n.OutDegree,
			FirstSeen: n.FirstSeen,
			LastSeen:  n.LastSeen,
		})
		return true
	})

	// Step 2: Copy all edges from base.
	base.ForEachEdge(func(e *graph.EdgeData) bool {
		g.AddEdge(e.From, e.To, e.Weight, e.Type, e.FirstSeen)
		// AddEdge sets Count to 1, but we need to preserve original count.
		// Also AddEdge uses FirstSeen as both FirstSeen and LastSeen initially.
		// We must fix up the edge to match base exactly.
		if ge, ok := g.GetEdge(e.From, e.To); ok {
			ge.Count = e.Count
			ge.LastSeen = e.LastSeen
		}
		return true
	})

	// Step 3: Copy all nodes from overlay (overlay wins for existing nodes).
	nodesMerged := 0
	overlay.ForEachNode(func(n *graph.NodeData) bool {
		if _, exists := g.GetNode(n.ID); exists {
			nodesMerged++
		}
		g.AddNode(&graph.NodeData{
			ID:        n.ID,
			Type:      n.Type,
			FirstSeen: n.FirstSeen,
			LastSeen:  n.LastSeen,
		})
		return true
	})

	// Step 4: Copy all edges from overlay (accumulates weight/count).
	edgesMerged := 0
	overlay.ForEachEdge(func(e *graph.EdgeData) bool {
		if _, exists := g.GetEdge(e.From, e.To); exists {
			edgesMerged++
		}
		g.AddEdge(e.From, e.To, e.Weight, e.Type, e.FirstSeen)
		// Fix up: AddEdge increments count by 1, but we want to add overlay's count.
		// AddEdge also only considers the single timestamp. We need to ensure
		// the overlay's count and LastSeen are properly merged.
		if ge, ok := g.GetEdge(e.From, e.To); ok {
			// For a new edge, AddEdge sets Count=1. We want Count = e.Count.
			// For an existing edge, AddEdge increments Count by 1. We want
			// the base count + overlay count.
			// After step 2, the edge has base.Count. AddEdge added 1.
			// So we need to adjust: subtract 1, add e.Count.
			ge.Count += e.Count - 1
			if e.LastSeen.After(ge.LastSeen) {
				ge.LastSeen = e.LastSeen
			}
		}
		return true
	})

	result := graph.NewImmutable(g)

	return result, Stats{
		NodesMerged:   nodesMerged,
		EdgesMerged:   edgesMerged,
		OverlayBefore: overlayNodes + overlayEdges,
		OverlayAfter:  0,
		Duration:      time.Since(start),
		Timestamp:     start,
	}
}

// ShouldCompact determines if compaction should trigger based on strategy.
func ShouldCompact(strategy Strategy, overlayEvents int, threshold int, lastCompaction time.Time, interval time.Duration) bool {
	switch strategy {
	case ByVolume:
		return overlayEvents >= threshold
	case ByTime:
		return time.Since(lastCompaction) >= interval
	case Manual:
		return false
	default:
		return false
	}
}
