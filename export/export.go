package export

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/MatheusGrego/itt-engine/graph"
)

// GraphView is a read-only graph interface for export.
type GraphView interface {
	NodeCount() int
	EdgeCount() int
	ForEachNode(fn func(*graph.NodeData) bool)
	ForEachEdge(fn func(*graph.EdgeData) bool)
}

// --- JSON Export ---

// jsonNode is the JSON-serializable node representation.
type jsonNode struct {
	ID         string             `json:"id"`
	Type       string             `json:"type,omitempty"`
	Degree     int                `json:"degree"`
	InDegree   int                `json:"in_degree"`
	OutDegree  int                `json:"out_degree"`
	Attributes map[string]float64 `json:"attributes,omitempty"`
}

// jsonEdge is the JSON-serializable edge representation.
type jsonEdge struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Weight float64 `json:"weight"`
	Type   string  `json:"type,omitempty"`
	Count  int     `json:"count"`
}

// jsonGraph is the top-level JSON structure.
type jsonGraph struct {
	Nodes []jsonNode `json:"nodes"`
	Edges []jsonEdge `json:"edges"`
}

// JSON writes the graph as JSON to the writer.
func JSON(w io.Writer, g GraphView) error {
	out := jsonGraph{}

	g.ForEachNode(func(n *graph.NodeData) bool {
		out.Nodes = append(out.Nodes, jsonNode{
			ID:         n.ID,
			Type:       n.Type,
			Degree:     n.Degree,
			InDegree:   n.InDegree,
			OutDegree:  n.OutDegree,
			Attributes: n.Attributes,
		})
		return true
	})

	g.ForEachEdge(func(e *graph.EdgeData) bool {
		out.Edges = append(out.Edges, jsonEdge{
			From:   e.From,
			To:     e.To,
			Weight: e.Weight,
			Type:   e.Type,
			Count:  e.Count,
		})
		return true
	})

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// --- DOT Export ---

// DOT writes the graph in Graphviz DOT format to the writer.
func DOT(w io.Writer, g GraphView) error {
	var b strings.Builder
	b.WriteString("digraph {\n")

	g.ForEachNode(func(n *graph.NodeData) bool {
		label := n.ID
		if n.Type != "" {
			label = fmt.Sprintf("%s\\n(%s)", n.ID, n.Type)
		}
		fmt.Fprintf(&b, "  %q [label=%q];\n", n.ID, label)
		return true
	})

	g.ForEachEdge(func(e *graph.EdgeData) bool {
		label := fmt.Sprintf("%.2f", e.Weight)
		if e.Type != "" {
			label = fmt.Sprintf("%s\\n%.2f", e.Type, e.Weight)
		}
		fmt.Fprintf(&b, "  %q -> %q [label=%q];\n", e.From, e.To, label)
		return true
	})

	b.WriteString("}\n")
	_, err := io.WriteString(w, b.String())
	return err
}
