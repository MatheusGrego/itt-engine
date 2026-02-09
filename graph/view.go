package graph

// UnifiedView merges a base and overlay ImmutableGraph into a single
// read-only view. Overlay nodes override base nodes. Edge weights from
// both layers are summed.
type UnifiedView struct {
	base    *ImmutableGraph
	overlay *ImmutableGraph
}

// NewUnifiedView creates a merged view of base and overlay graphs.
func NewUnifiedView(base, overlay *ImmutableGraph) *UnifiedView {
	return &UnifiedView{base: base, overlay: overlay}
}

// GetNode returns a node. Overlay wins if present in both.
func (v *UnifiedView) GetNode(id string) (*NodeData, bool) {
	if n, ok := v.overlay.GetNode(id); ok {
		return n, true
	}
	return v.base.GetNode(id)
}

// GetEdge returns an edge. If present in both, weights are summed.
func (v *UnifiedView) GetEdge(from, to string) (*EdgeData, bool) {
	be, bok := v.base.GetEdge(from, to)
	oe, ook := v.overlay.GetEdge(from, to)

	if bok && ook {
		return mergeEdges(be, oe), true
	}
	if ook {
		return oe, true
	}
	if bok {
		return be, true
	}
	return nil, false
}

// Neighbors returns deduplicated neighbors from both layers.
func (v *UnifiedView) Neighbors(nodeID string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, id := range v.overlay.Neighbors(nodeID) {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	for _, id := range v.base.Neighbors(nodeID) {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

// OutNeighbors returns deduplicated out-neighbors from both layers.
func (v *UnifiedView) OutNeighbors(nodeID string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, id := range v.overlay.OutNeighbors(nodeID) {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	for _, id := range v.base.OutNeighbors(nodeID) {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

// InNeighbors returns deduplicated in-neighbors from both layers.
func (v *UnifiedView) InNeighbors(nodeID string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, id := range v.overlay.InNeighbors(nodeID) {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	for _, id := range v.base.InNeighbors(nodeID) {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

// NodeCount returns total unique nodes across both layers.
func (v *UnifiedView) NodeCount() int {
	seen := make(map[string]bool)
	v.overlay.ForEachNode(func(n *NodeData) bool {
		seen[n.ID] = true
		return true
	})
	v.base.ForEachNode(func(n *NodeData) bool {
		seen[n.ID] = true
		return true
	})
	return len(seen)
}

// EdgeCount returns total unique edges across both layers.
func (v *UnifiedView) EdgeCount() int {
	seen := make(map[string]bool)
	v.overlay.ForEachEdge(func(e *EdgeData) bool {
		seen[edgeKey(e.From, e.To)] = true
		return true
	})
	v.base.ForEachEdge(func(e *EdgeData) bool {
		seen[edgeKey(e.From, e.To)] = true
		return true
	})
	return len(seen)
}

// ForEachNode iterates all unique nodes (overlay wins on conflicts).
func (v *UnifiedView) ForEachNode(fn func(*NodeData) bool) {
	seen := make(map[string]bool)
	// Overlay first — overlay wins on conflicts.
	v.overlay.ForEachNode(func(n *NodeData) bool {
		seen[n.ID] = true
		return fn(n)
	})
	// Then base, skipping already-seen IDs.
	v.base.ForEachNode(func(n *NodeData) bool {
		if seen[n.ID] {
			return true
		}
		return fn(n)
	})
}

// ForEachEdge iterates all edges (merged weights for duplicates).
func (v *UnifiedView) ForEachEdge(fn func(*EdgeData) bool) {
	seen := make(map[string]bool)
	// Overlay first. For edges in both, emit merged version.
	v.overlay.ForEachEdge(func(oe *EdgeData) bool {
		key := edgeKey(oe.From, oe.To)
		seen[key] = true
		if be, ok := v.base.GetEdge(oe.From, oe.To); ok {
			return fn(mergeEdges(be, oe))
		}
		return fn(oe)
	})
	// Then base, skipping already-seen edge keys.
	v.base.ForEachEdge(func(be *EdgeData) bool {
		key := edgeKey(be.From, be.To)
		if seen[key] {
			return true
		}
		return fn(be)
	})
}

// mergeEdges creates a new EdgeData with combined metrics from base and overlay.
// The overlay's From, To, and Type are used. Weight and Count are summed.
// FirstSeen is the minimum and LastSeen is the maximum of both.
func mergeEdges(base, overlay *EdgeData) *EdgeData {
	merged := &EdgeData{
		From:   overlay.From,
		To:     overlay.To,
		Type:   overlay.Type,
		Weight: base.Weight + overlay.Weight,
		Count:  base.Count + overlay.Count,
	}
	if base.FirstSeen.Before(overlay.FirstSeen) {
		merged.FirstSeen = base.FirstSeen
	} else {
		merged.FirstSeen = overlay.FirstSeen
	}
	if base.LastSeen.After(overlay.LastSeen) {
		merged.LastSeen = base.LastSeen
	} else {
		merged.LastSeen = overlay.LastSeen
	}
	return merged
}
