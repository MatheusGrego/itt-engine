package analysis

import "math"

// ConcealmentCalculator computes the concealment cost for node sets.
// Concealment cost measures how "expensive" it is to conceal manipulation
// in a network neighborhood: Omega(Ns) = sum_k sum_vi tau(vi) * exp(-lambda*k).
type ConcealmentCalculator struct {
	lambda  float64            // exponential decay parameter
	tension *TensionCalculator // reuses tension computation
}

// NewConcealmentCalculator creates a ConcealmentCalculator.
// lambda controls exponential decay of distant contributions.
// tc is the tension calculator used to compute per-node tension values.
func NewConcealmentCalculator(lambda float64, tc *TensionCalculator) *ConcealmentCalculator {
	return &ConcealmentCalculator{
		lambda:  lambda,
		tension: tc,
	}
}

// Calculate computes the concealment cost for a set of nodes.
// It sums the concealment cost of each node in the set.
// maxHops limits the neighborhood depth.
func (cc *ConcealmentCalculator) Calculate(g GraphView, nodeIDs []string, maxHops int) float64 {
	total := 0.0
	for _, id := range nodeIDs {
		total += cc.CalculateNode(g, id, maxHops)
	}
	return total
}

// CalculateNode computes the concealment cost for a single node
// by walking its k-hop neighborhood via BFS.
//
// Algorithm:
//  1. Start BFS from nodeID.
//  2. For each node at hop distance k (0 to maxHops), compute tau(v) * exp(-lambda*k).
//  3. Sum all contributions.
//  4. Return the total.
func (cc *ConcealmentCalculator) CalculateNode(g GraphView, nodeID string, maxHops int) float64 {
	if _, ok := g.GetNode(nodeID); !ok {
		return 0
	}

	// BFS: track visited nodes and their hop distance.
	visited := make(map[string]int) // nodeID -> hop distance
	visited[nodeID] = 0

	// currentFrontier holds nodes at the current BFS level.
	currentFrontier := []string{nodeID}

	total := 0.0

	for k := 0; k <= maxHops; k++ {
		// Process all nodes at distance k.
		for _, vid := range currentFrontier {
			tau := cc.tension.Calculate(g, vid)
			total += tau * math.Exp(-cc.lambda*float64(k))
		}

		if k == maxHops {
			break
		}

		// Expand to the next frontier.
		var nextFrontier []string
		for _, vid := range currentFrontier {
			neighbors := g.Neighbors(vid)
			for _, nid := range neighbors {
				if _, seen := visited[nid]; !seen {
					visited[nid] = k + 1
					nextFrontier = append(nextFrontier, nid)
				}
			}
		}
		currentFrontier = nextFrontier
	}

	return total
}
