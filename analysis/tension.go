package analysis

import "github.com/MatheusGrego/itt-engine/graph"

// GraphView is what the tension calculator needs from a graph.
type GraphView interface {
	GetNode(id string) (*graph.NodeData, bool)
	GetEdge(from, to string) (*graph.EdgeData, bool)
	Neighbors(nodeID string) []string
	OutNeighbors(nodeID string) []string
	InNeighbors(nodeID string) []string
	NodeCount() int
	EdgeCount() int
	ForEachNode(fn func(*graph.NodeData) bool)
	ForEachEdge(fn func(*graph.EdgeData) bool)
}

// TensionCalculator computes informational tension for nodes in a graph.
// Tension measures how much a node's removal would perturb the weight
// distributions of its neighbors.
type TensionCalculator struct {
	divergence DivergenceFunc
}

// NewTensionCalculator creates a TensionCalculator with the given divergence function.
func NewTensionCalculator(div DivergenceFunc) *TensionCalculator {
	return &TensionCalculator{divergence: div}
}

// Calculate computes the informational tension of a single node.
//
// Algorithm:
//  1. For each neighbor n of nodeID, build the outgoing weight distribution of n.
//  2. Simulate removal of nodeID: zero out edges from n to nodeID, re-normalize.
//  3. Compute divergence between original and perturbed distributions.
//  4. Return the mean divergence across all contributing neighbors.
func (tc *TensionCalculator) Calculate(g GraphView, nodeID string) float64 {
	if _, ok := g.GetNode(nodeID); !ok {
		return 0
	}

	neighbors := g.Neighbors(nodeID)
	if len(neighbors) == 0 {
		return 0
	}

	totalDiv := 0.0
	count := 0

	for _, nID := range neighbors {
		outNeighbors := g.OutNeighbors(nID)
		if len(outNeighbors) == 0 {
			continue
		}

		// Build original outgoing weight distribution for neighbor nID.
		// Each slot corresponds to an out-neighbor of nID.
		original := make([]float64, len(outNeighbors))
		perturbedRaw := make([]float64, len(outNeighbors))

		for i, target := range outNeighbors {
			w := 0.0
			if e, ok := g.GetEdge(nID, target); ok {
				w = e.Weight
			}
			original[i] = w
			if target == nodeID {
				perturbedRaw[i] = 0 // zero out edges to the removed node
			} else {
				perturbedRaw[i] = w
			}
		}

		origDist := Normalize(original)
		pertDist := Normalize(perturbedRaw)

		div := tc.divergence.Compute(origDist, pertDist)
		totalDiv += div
		count++
	}

	if count == 0 {
		return 0
	}
	return totalDiv / float64(count)
}

// CalculateAll computes informational tension for every node in the graph.
func (tc *TensionCalculator) CalculateAll(g GraphView) map[string]float64 {
	result := make(map[string]float64)
	g.ForEachNode(func(n *graph.NodeData) bool {
		result[n.ID] = tc.Calculate(g, n.ID)
		return true
	})
	return result
}
