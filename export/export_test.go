package export

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MatheusGrego/itt-engine/graph"
)

// helper builds a small graph and returns an immutable snapshot.
func buildTestGraph(t *testing.T) *graph.ImmutableGraph {
	t.Helper()
	g := graph.New()
	now := time.Now()
	g.AddNode(&graph.NodeData{ID: "alice", Type: "user"})
	g.AddNode(&graph.NodeData{ID: "bob", Type: "user"})
	g.AddEdge("alice", "bob", 1.5, "transfer", now)
	return graph.NewImmutable(g)
}

// --- JSON Tests ---

func TestJSON_BasicGraph(t *testing.T) {
	ig := buildTestGraph(t)

	var buf bytes.Buffer
	if err := JSON(&buf, ig); err != nil {
		t.Fatalf("JSON export failed: %v", err)
	}

	// Must be valid JSON.
	var parsed jsonGraph
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}

	if len(parsed.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(parsed.Nodes))
	}
	if len(parsed.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(parsed.Edges))
	}

	// Verify the edge values.
	e := parsed.Edges[0]
	if e.From != "alice" || e.To != "bob" {
		t.Fatalf("unexpected edge endpoints: %s -> %s", e.From, e.To)
	}
	if e.Weight != 1.5 {
		t.Fatalf("expected weight 1.5, got %f", e.Weight)
	}
	if e.Type != "transfer" {
		t.Fatalf("expected type transfer, got %s", e.Type)
	}
	if e.Count != 1 {
		t.Fatalf("expected count 1, got %d", e.Count)
	}
}

func TestJSON_EmptyGraph(t *testing.T) {
	ig := graph.NewImmutableEmpty()

	var buf bytes.Buffer
	if err := JSON(&buf, ig); err != nil {
		t.Fatalf("JSON export failed: %v", err)
	}

	// Must be valid JSON.
	var parsed jsonGraph
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}

	// Nodes and Edges should be nil or empty.
	if len(parsed.Nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(parsed.Nodes))
	}
	if len(parsed.Edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(parsed.Edges))
	}
}

func TestJSON_NodeAttributes(t *testing.T) {
	g := graph.New()
	g.AddNode(&graph.NodeData{
		ID:   "x",
		Type: "sensor",
		Attributes: map[string]float64{
			"risk":  0.85,
			"score": 42.0,
		},
	})
	ig := graph.NewImmutable(g)

	var buf bytes.Buffer
	if err := JSON(&buf, ig); err != nil {
		t.Fatalf("JSON export failed: %v", err)
	}

	var parsed jsonGraph
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}

	if len(parsed.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(parsed.Nodes))
	}

	n := parsed.Nodes[0]
	if n.ID != "x" {
		t.Fatalf("expected id 'x', got '%s'", n.ID)
	}
	if n.Type != "sensor" {
		t.Fatalf("expected type 'sensor', got '%s'", n.Type)
	}
	if n.Attributes == nil {
		t.Fatal("expected attributes map, got nil")
	}
	if n.Attributes["risk"] != 0.85 {
		t.Fatalf("expected risk 0.85, got %f", n.Attributes["risk"])
	}
	if n.Attributes["score"] != 42.0 {
		t.Fatalf("expected score 42.0, got %f", n.Attributes["score"])
	}
}

func TestJSON_NodeDegrees(t *testing.T) {
	ig := buildTestGraph(t)

	var buf bytes.Buffer
	if err := JSON(&buf, ig); err != nil {
		t.Fatalf("JSON export failed: %v", err)
	}

	var parsed jsonGraph
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Find alice and bob.
	nodeMap := make(map[string]jsonNode)
	for _, n := range parsed.Nodes {
		nodeMap[n.ID] = n
	}

	alice := nodeMap["alice"]
	if alice.OutDegree != 1 {
		t.Fatalf("alice out_degree: expected 1, got %d", alice.OutDegree)
	}
	if alice.InDegree != 0 {
		t.Fatalf("alice in_degree: expected 0, got %d", alice.InDegree)
	}
	if alice.Degree != 1 {
		t.Fatalf("alice degree: expected 1, got %d", alice.Degree)
	}

	bob := nodeMap["bob"]
	if bob.InDegree != 1 {
		t.Fatalf("bob in_degree: expected 1, got %d", bob.InDegree)
	}
	if bob.OutDegree != 0 {
		t.Fatalf("bob out_degree: expected 0, got %d", bob.OutDegree)
	}
}

// --- DOT Tests ---

func TestDOT_BasicGraph(t *testing.T) {
	ig := buildTestGraph(t)

	var buf bytes.Buffer
	if err := DOT(&buf, ig); err != nil {
		t.Fatalf("DOT export failed: %v", err)
	}

	out := buf.String()

	if !strings.HasPrefix(out, "digraph {\n") {
		t.Fatalf("output should start with 'digraph {\\n', got: %q", out[:min(len(out), 30)])
	}
	if !strings.HasSuffix(out, "}\n") {
		t.Fatalf("output should end with '}\\n'")
	}

	// Nodes must appear.
	if !strings.Contains(out, `"alice"`) {
		t.Fatal("DOT output missing node alice")
	}
	if !strings.Contains(out, `"bob"`) {
		t.Fatal("DOT output missing node bob")
	}

	// Edge must appear.
	if !strings.Contains(out, `"alice" -> "bob"`) {
		t.Fatal("DOT output missing edge alice -> bob")
	}
}

func TestDOT_EmptyGraph(t *testing.T) {
	ig := graph.NewImmutableEmpty()

	var buf bytes.Buffer
	if err := DOT(&buf, ig); err != nil {
		t.Fatalf("DOT export failed: %v", err)
	}

	expected := "digraph {\n}\n"
	if buf.String() != expected {
		t.Fatalf("expected %q, got %q", expected, buf.String())
	}
}

func TestDOT_EdgeLabels(t *testing.T) {
	ig := buildTestGraph(t)

	var buf bytes.Buffer
	if err := DOT(&buf, ig); err != nil {
		t.Fatalf("DOT export failed: %v", err)
	}

	out := buf.String()

	// Edge has type "transfer" so label should include type and weight.
	if !strings.Contains(out, "transfer") {
		t.Fatal("DOT edge label should include edge type 'transfer'")
	}
	if !strings.Contains(out, "1.50") {
		t.Fatal("DOT edge label should include weight '1.50'")
	}
}

func TestDOT_NodeWithType(t *testing.T) {
	ig := buildTestGraph(t)

	var buf bytes.Buffer
	if err := DOT(&buf, ig); err != nil {
		t.Fatalf("DOT export failed: %v", err)
	}

	out := buf.String()

	// Nodes have type "user", so labels should include the type.
	if !strings.Contains(out, "(user)") {
		t.Fatal("DOT node label should include node type '(user)'")
	}
}

func TestDOT_NodeWithoutType(t *testing.T) {
	g := graph.New()
	g.AddNode(&graph.NodeData{ID: "plain"})
	ig := graph.NewImmutable(g)

	var buf bytes.Buffer
	if err := DOT(&buf, ig); err != nil {
		t.Fatalf("DOT export failed: %v", err)
	}

	out := buf.String()

	// Node without type should use just the ID as label.
	if !strings.Contains(out, `[label="plain"]`) {
		t.Fatalf("expected label=\"plain\" for node without type, got: %s", out)
	}
}

func TestDOT_EdgeWithoutType(t *testing.T) {
	g := graph.New()
	g.AddEdge("a", "b", 2.0, "", time.Now())
	ig := graph.NewImmutable(g)

	var buf bytes.Buffer
	if err := DOT(&buf, ig); err != nil {
		t.Fatalf("DOT export failed: %v", err)
	}

	out := buf.String()

	// Edge without type should just show weight.
	if !strings.Contains(out, `[label="2.00"]`) {
		t.Fatalf("expected label=\"2.00\" for edge without type, got: %s", out)
	}
}
