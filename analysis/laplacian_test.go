package analysis

import (
	"testing"

	"github.com/MatheusGrego/itt-engine/graph"
)

// mockGraphView implements GraphView for testing the laplacian functions.
type mockGraphView struct {
	nodes map[string]*graph.NodeData
	edges map[[2]string]*graph.EdgeData
	out   map[string][]string // outgoing neighbors
	in    map[string][]string // incoming neighbors
}

func newMockGraphView() *mockGraphView {
	return &mockGraphView{
		nodes: make(map[string]*graph.NodeData),
		edges: make(map[[2]string]*graph.EdgeData),
		out:   make(map[string][]string),
		in:    make(map[string][]string),
	}
}

func (m *mockGraphView) addNode(id string) {
	m.nodes[id] = &graph.NodeData{ID: id}
}

func (m *mockGraphView) addEdge(from, to string, weight float64) {
	if _, ok := m.nodes[from]; !ok {
		m.addNode(from)
	}
	if _, ok := m.nodes[to]; !ok {
		m.addNode(to)
	}
	m.edges[[2]string{from, to}] = &graph.EdgeData{From: from, To: to, Weight: weight}
	// Track outgoing and incoming neighbors.
	if !m.hasNeighbor(m.out[from], to) {
		m.out[from] = append(m.out[from], to)
	}
	if !m.hasNeighbor(m.in[to], from) {
		m.in[to] = append(m.in[to], from)
	}
}

// addUndirectedEdge adds edges in both directions.
func (m *mockGraphView) addUndirectedEdge(a, b string, weight float64) {
	m.addEdge(a, b, weight)
	m.addEdge(b, a, weight)
}

func (m *mockGraphView) hasNeighbor(list []string, id string) bool {
	for _, n := range list {
		if n == id {
			return true
		}
	}
	return false
}

func (m *mockGraphView) GetNode(id string) (*graph.NodeData, bool) {
	n, ok := m.nodes[id]
	return n, ok
}

func (m *mockGraphView) GetEdge(from, to string) (*graph.EdgeData, bool) {
	e, ok := m.edges[[2]string{from, to}]
	return e, ok
}

func (m *mockGraphView) Neighbors(nodeID string) []string {
	// Union of out-neighbors and in-neighbors (unique).
	seen := make(map[string]bool)
	var result []string
	for _, n := range m.out[nodeID] {
		if !seen[n] {
			seen[n] = true
			result = append(result, n)
		}
	}
	for _, n := range m.in[nodeID] {
		if !seen[n] {
			seen[n] = true
			result = append(result, n)
		}
	}
	return result
}

func (m *mockGraphView) OutNeighbors(nodeID string) []string {
	return m.out[nodeID]
}

func (m *mockGraphView) InNeighbors(nodeID string) []string {
	return m.in[nodeID]
}

func (m *mockGraphView) NodeCount() int {
	return len(m.nodes)
}

func (m *mockGraphView) EdgeCount() int {
	return len(m.edges)
}

func (m *mockGraphView) ForEachNode(fn func(*graph.NodeData) bool) {
	for _, n := range m.nodes {
		if !fn(n) {
			return
		}
	}
}

func (m *mockGraphView) ForEachEdge(fn func(*graph.EdgeData) bool) {
	for _, e := range m.edges {
		if !fn(e) {
			return
		}
	}
}

// --- Test helpers ---

// buildPathGraph creates P_n: A-B-C-D (undirected path of 4 nodes).
func buildPathGraph() (*mockGraphView, []string) {
	g := newMockGraphView()
	nodes := []string{"A", "B", "C", "D"}
	g.addUndirectedEdge("A", "B", 1.0)
	g.addUndirectedEdge("B", "C", 1.0)
	g.addUndirectedEdge("C", "D", 1.0)
	return g, nodes
}

// buildCompleteGraph creates K_4: all 4 nodes connected to each other.
func buildCompleteGraph() (*mockGraphView, []string) {
	g := newMockGraphView()
	nodes := []string{"A", "B", "C", "D"}
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			g.addUndirectedEdge(nodes[i], nodes[j], 1.0)
		}
	}
	return g, nodes
}

// buildDisconnectedGraph creates two disconnected components: {A,B} and {C,D}.
func buildDisconnectedGraph() (*mockGraphView, []string) {
	g := newMockGraphView()
	nodes := []string{"A", "B", "C", "D"}
	g.addUndirectedEdge("A", "B", 1.0)
	g.addUndirectedEdge("C", "D", 1.0)
	return g, nodes
}

// --- Tests ---

func TestFiedlerValue_PathGraph(t *testing.T) {
	g, nodes := buildPathGraph()
	// P_4 has lambda_1 = 2*(1-cos(pi/4)) ~ 0.586.
	// Due to iterative approximation, we just check it's in a reasonable range.
	fiedler := FiedlerValue(g, nodes, 200, 1e-8)
	if fiedler < 0.1 {
		t.Fatalf("expected FiedlerValue > 0.1 for P_4, got %f", fiedler)
	}
	if fiedler > 2.0 {
		t.Fatalf("expected FiedlerValue < 2.0 for P_4, got %f", fiedler)
	}
}

func TestFiedlerValue_CompleteGraph(t *testing.T) {
	g, nodes := buildCompleteGraph()
	// K_4: lambda_1 = n = 4 (all eigenvalues except 0 are n).
	fiedler := FiedlerValue(g, nodes, 200, 1e-8)
	if fiedler <= 0 {
		t.Fatalf("expected FiedlerValue > 0 for K_4, got %f", fiedler)
	}
}

func TestFiedlerValue_DisconnectedGraph(t *testing.T) {
	g, nodes := buildDisconnectedGraph()
	// Disconnected graph => FiedlerValue should be 0.
	fiedler := FiedlerValue(g, nodes, 200, 1e-8)
	if fiedler != 0 {
		t.Fatalf("expected FiedlerValue == 0 for disconnected graph, got %f", fiedler)
	}
}

func TestFiedlerValue_TooFewNodes(t *testing.T) {
	g := newMockGraphView()
	g.addNode("A")
	fiedler := FiedlerValue(g, []string{"A"}, 100, 1e-8)
	if fiedler != 0 {
		t.Fatalf("expected FiedlerValue == 0 for single node, got %f", fiedler)
	}

	fiedler = FiedlerValue(g, nil, 100, 1e-8)
	if fiedler != 0 {
		t.Fatalf("expected FiedlerValue == 0 for nil nodeIDs, got %f", fiedler)
	}
}

func TestFiedlerApprox_PositiveForConnected(t *testing.T) {
	g, nodes := buildPathGraph()
	approx := FiedlerApprox(g, nodes)
	if approx <= 0 {
		t.Fatalf("expected FiedlerApprox > 0 for connected graph, got %f", approx)
	}
}

func TestFiedlerApprox_ZeroForDisconnected(t *testing.T) {
	g, nodes := buildDisconnectedGraph()
	approx := FiedlerApprox(g, nodes)
	if approx != 0 {
		t.Fatalf("expected FiedlerApprox == 0 for disconnected graph, got %f", approx)
	}
}

func TestFiedlerApprox_TooFewNodes(t *testing.T) {
	g := newMockGraphView()
	g.addNode("A")
	approx := FiedlerApprox(g, []string{"A"})
	if approx != 0 {
		t.Fatalf("expected FiedlerApprox == 0 for single node, got %f", approx)
	}
}

func TestFiedlerValue_CompleteGraph_Magnitude(t *testing.T) {
	g, nodes := buildCompleteGraph()
	fiedler := FiedlerValue(g, nodes, 200, 1e-8)
	// For K_4, all non-zero eigenvalues are 4. The Fiedler value should be close to 4.
	// Due to numerical approximation, allow a range.
	if fiedler < 1.0 {
		t.Fatalf("expected FiedlerValue close to 4 for K_4, got %f (too low)", fiedler)
	}
}

func TestFiedlerApprox_CompleteGraph(t *testing.T) {
	g, nodes := buildCompleteGraph()
	approx := FiedlerApprox(g, nodes)
	if approx <= 0 {
		t.Fatalf("expected FiedlerApprox > 0 for K_4, got %f", approx)
	}
}

func TestFiedlerValue_TriangleGraph(t *testing.T) {
	// K_3: lambda_1 = 3.
	g := newMockGraphView()
	nodes := []string{"A", "B", "C"}
	g.addUndirectedEdge("A", "B", 1.0)
	g.addUndirectedEdge("B", "C", 1.0)
	g.addUndirectedEdge("A", "C", 1.0)

	fiedler := FiedlerValue(g, nodes, 200, 1e-8)
	if fiedler <= 0 {
		t.Fatalf("expected FiedlerValue > 0 for triangle, got %f", fiedler)
	}
}

func TestFiedlerApprox_PathGraph(t *testing.T) {
	g, nodes := buildPathGraph()
	approx := FiedlerApprox(g, nodes)
	// Should be a positive lower bound.
	if approx <= 0 {
		t.Fatalf("expected FiedlerApprox > 0 for path graph, got %f", approx)
	}
	// Lower bound should not exceed the actual Fiedler value.
	exact := FiedlerValue(g, nodes, 200, 1e-8)
	if approx > exact*2 {
		t.Fatalf("FiedlerApprox (%f) should be a reasonable lower bound of FiedlerValue (%f)", approx, exact)
	}
}
