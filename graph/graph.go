package graph

import "time"

// NodeData holds mutable node state inside the graph.
type NodeData struct {
	ID         string
	Type       string
	Degree     int
	InDegree   int
	OutDegree  int
	Attributes map[string]float64
	FirstSeen  time.Time
	LastSeen   time.Time
}

// EdgeData holds mutable edge state inside the graph.
type EdgeData struct {
	From      string
	To        string
	Weight    float64
	Type      string
	Count     int
	FirstSeen time.Time
	LastSeen  time.Time
}

// edgeKey returns a canonical key for a directed edge.
func edgeKey(from, to string) string {
	return from + "\x00" + to
}

// Graph is a mutable directed weighted graph using adjacency lists.
type Graph struct {
	nodes map[string]*NodeData
	edges map[string]*EdgeData
	out   map[string]map[string]bool
	in    map[string]map[string]bool
}

// New creates an empty graph.
func New() *Graph {
	return &Graph{
		nodes: make(map[string]*NodeData),
		edges: make(map[string]*EdgeData),
		out:   make(map[string]map[string]bool),
		in:    make(map[string]map[string]bool),
	}
}

// AddNode adds or updates a node.
func (g *Graph) AddNode(n *NodeData) {
	if existing, ok := g.nodes[n.ID]; ok {
		existing.Type = n.Type
		if !n.LastSeen.IsZero() {
			existing.LastSeen = n.LastSeen
		}
		return
	}
	g.nodes[n.ID] = n
	if g.out[n.ID] == nil {
		g.out[n.ID] = make(map[string]bool)
	}
	if g.in[n.ID] == nil {
		g.in[n.ID] = make(map[string]bool)
	}
}

// GetNode returns a node by ID.
func (g *Graph) GetNode(id string) (*NodeData, bool) {
	n, ok := g.nodes[id]
	return n, ok
}

// AddEdge adds or accumulates an edge. Creates nodes if missing.
func (g *Graph) AddEdge(from, to string, weight float64, edgeType string, ts time.Time) {
	if _, ok := g.nodes[from]; !ok {
		g.AddNode(&NodeData{ID: from, FirstSeen: ts, LastSeen: ts})
	}
	if _, ok := g.nodes[to]; !ok {
		g.AddNode(&NodeData{ID: to, FirstSeen: ts, LastSeen: ts})
	}

	key := edgeKey(from, to)
	if e, ok := g.edges[key]; ok {
		e.Weight += weight
		e.Count++
		if ts.After(e.LastSeen) {
			e.LastSeen = ts
		}
		if ts.Before(e.FirstSeen) {
			e.FirstSeen = ts
		}
		return
	}

	g.edges[key] = &EdgeData{
		From:      from,
		To:        to,
		Weight:    weight,
		Type:      edgeType,
		Count:     1,
		FirstSeen: ts,
		LastSeen:  ts,
	}

	g.out[from][to] = true
	g.in[to][from] = true

	g.nodes[from].OutDegree++
	g.nodes[from].Degree++
	g.nodes[to].InDegree++
	g.nodes[to].Degree++
}

// GetEdge returns an edge by endpoints.
func (g *Graph) GetEdge(from, to string) (*EdgeData, bool) {
	e, ok := g.edges[edgeKey(from, to)]
	return e, ok
}

// Neighbors returns all neighbors (in + out), deduplicated.
func (g *Graph) Neighbors(nodeID string) []string {
	seen := make(map[string]bool)
	var result []string
	for id := range g.out[nodeID] {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	for id := range g.in[nodeID] {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

// OutNeighbors returns nodes pointed to by nodeID.
func (g *Graph) OutNeighbors(nodeID string) []string {
	var result []string
	for id := range g.out[nodeID] {
		result = append(result, id)
	}
	return result
}

// InNeighbors returns nodes pointing to nodeID.
func (g *Graph) InNeighbors(nodeID string) []string {
	var result []string
	for id := range g.in[nodeID] {
		result = append(result, id)
	}
	return result
}

// NodeCount returns total number of nodes.
func (g *Graph) NodeCount() int { return len(g.nodes) }

// EdgeCount returns total number of unique edges.
func (g *Graph) EdgeCount() int { return len(g.edges) }

// ForEachNode iterates nodes. Return false to stop.
func (g *Graph) ForEachNode(fn func(*NodeData) bool) {
	for _, n := range g.nodes {
		if !fn(n) {
			return
		}
	}
}

// ForEachEdge iterates edges. Return false to stop.
func (g *Graph) ForEachEdge(fn func(*EdgeData) bool) {
	for _, e := range g.edges {
		if !fn(e) {
			return
		}
	}
}
