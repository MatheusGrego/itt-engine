package graph

import "time"

// ImmutableGraph is a read-only, deep-copied snapshot of a Graph.
// Creating a new version returns a new ImmutableGraph without
// mutating the original (copy-on-write).
type ImmutableGraph struct {
	inner *Graph
}

// NewImmutable deep-copies a mutable Graph into an immutable one.
func NewImmutable(src *Graph) *ImmutableGraph {
	return &ImmutableGraph{inner: deepCopyGraph(src)}
}

// NewImmutableEmpty creates an empty immutable graph.
func NewImmutableEmpty() *ImmutableGraph {
	return &ImmutableGraph{inner: New()}
}

// WithEvent returns a NEW ImmutableGraph with the event applied.
// The receiver is not modified.
func (ig *ImmutableGraph) WithEvent(from, to string, weight float64, edgeType string, ts time.Time) *ImmutableGraph {
	cp := deepCopyGraph(ig.inner)
	cp.AddEdge(from, to, weight, edgeType, ts)

	if n, ok := cp.GetNode(from); ok {
		if ts.Before(n.FirstSeen) || n.FirstSeen.IsZero() {
			n.FirstSeen = ts
		}
		if ts.After(n.LastSeen) {
			n.LastSeen = ts
		}
	}
	if n, ok := cp.GetNode(to); ok {
		if ts.Before(n.FirstSeen) || n.FirstSeen.IsZero() {
			n.FirstSeen = ts
		}
		if ts.After(n.LastSeen) {
			n.LastSeen = ts
		}
	}

	return &ImmutableGraph{inner: cp}
}

// GetNode returns a node by ID.
func (ig *ImmutableGraph) GetNode(id string) (*NodeData, bool) {
	return ig.inner.GetNode(id)
}

// GetEdge returns an edge by endpoints.
func (ig *ImmutableGraph) GetEdge(from, to string) (*EdgeData, bool) {
	return ig.inner.GetEdge(from, to)
}

// Neighbors returns all neighbor IDs (in + out, deduplicated).
func (ig *ImmutableGraph) Neighbors(nodeID string) []string {
	return ig.inner.Neighbors(nodeID)
}

// OutNeighbors returns outgoing neighbor IDs.
func (ig *ImmutableGraph) OutNeighbors(nodeID string) []string {
	return ig.inner.OutNeighbors(nodeID)
}

// InNeighbors returns incoming neighbor IDs.
func (ig *ImmutableGraph) InNeighbors(nodeID string) []string {
	return ig.inner.InNeighbors(nodeID)
}

// NodeCount returns the number of nodes.
func (ig *ImmutableGraph) NodeCount() int { return ig.inner.NodeCount() }

// EdgeCount returns the number of edges.
func (ig *ImmutableGraph) EdgeCount() int { return ig.inner.EdgeCount() }

// ForEachNode iterates all nodes. Return false to stop.
func (ig *ImmutableGraph) ForEachNode(fn func(*NodeData) bool) { ig.inner.ForEachNode(fn) }

// ForEachEdge iterates all edges. Return false to stop.
func (ig *ImmutableGraph) ForEachEdge(fn func(*EdgeData) bool) { ig.inner.ForEachEdge(fn) }

// deepCopyGraph creates a fully independent copy of a Graph.
func deepCopyGraph(src *Graph) *Graph {
	dst := New()

	src.ForEachNode(func(n *NodeData) bool {
		cp := &NodeData{
			ID:        n.ID,
			Type:      n.Type,
			Degree:    n.Degree,
			InDegree:  n.InDegree,
			OutDegree: n.OutDegree,
			FirstSeen: n.FirstSeen,
			LastSeen:  n.LastSeen,
		}
		if n.Attributes != nil {
			cp.Attributes = make(map[string]float64, len(n.Attributes))
			for k, v := range n.Attributes {
				cp.Attributes[k] = v
			}
		}
		dst.nodes[cp.ID] = cp
		if dst.out[cp.ID] == nil {
			dst.out[cp.ID] = make(map[string]bool)
		}
		if dst.in[cp.ID] == nil {
			dst.in[cp.ID] = make(map[string]bool)
		}
		return true
	})

	src.ForEachEdge(func(e *EdgeData) bool {
		cp := &EdgeData{
			From:      e.From,
			To:        e.To,
			Weight:    e.Weight,
			Type:      e.Type,
			Count:     e.Count,
			FirstSeen: e.FirstSeen,
			LastSeen:  e.LastSeen,
		}
		dst.edges[edgeKey(cp.From, cp.To)] = cp
		dst.out[cp.From][cp.To] = true
		dst.in[cp.To][cp.From] = true
		return true
	})

	return dst
}
