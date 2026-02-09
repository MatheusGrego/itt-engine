package analysis

import (
	"math"

	"github.com/MatheusGrego/itt-engine/graph"
)

// CurvatureCalculator computes Ollivier-Ricci curvature for edges in a graph.
//
// Ollivier-Ricci curvature for an edge (x, y) is defined as:
//
//	k(x,y) = 1 - W1(mu_x, mu_y) / d(x,y)
//
// where W1 is the Wasserstein-1 (Earth Mover's) distance between
// lazy random walk measures mu_x and mu_y, and d(x,y) is the
// shortest-path distance between x and y (1 for directly connected nodes).
//
// The lazy random walk measure at node x assigns weight alpha to x itself
// and distributes (1 - alpha) uniformly among the neighbors of x.
type CurvatureCalculator struct {
	alpha   float64 // lazy random walk parameter (weight on self)
	reg     float64 // Sinkhorn regularization parameter
	maxIter int     // maximum Sinkhorn iterations
}

// NewCurvatureCalculator creates a CurvatureCalculator with the given lazy
// random walk parameter alpha. Typical value is 0.5, meaning half the
// probability mass stays at the node and half is distributed to neighbors.
func NewCurvatureCalculator(alpha float64) *CurvatureCalculator {
	return &CurvatureCalculator{
		alpha:   alpha,
		reg:     0.1,
		maxIter: 100,
	}
}

// Calculate computes the Ollivier-Ricci curvature of a single edge (from, to).
// Returns 0 if the edge does not exist.
//
// For directly connected nodes d(from, to) = 1, so:
//
//	k(from, to) = 1 - W1(mu_from, mu_to)
func (cc *CurvatureCalculator) Calculate(g GraphView, from, to string) float64 {
	if _, ok := g.GetEdge(from, to); !ok {
		return 0
	}

	// Build lazy random walk measures.
	muFrom := cc.buildMeasure(g, from)
	muTo := cc.buildMeasure(g, to)

	// Collect the union of support node IDs.
	supportSet := make(map[string]bool)
	for id := range muFrom {
		supportSet[id] = true
	}
	for id := range muTo {
		supportSet[id] = true
	}

	// Order the support nodes deterministically.
	support := make([]string, 0, len(supportSet))
	for id := range supportSet {
		support = append(support, id)
	}
	sortStrings(support)

	n := len(support)

	// Build distribution vectors aligned to the support.
	mu := make([]float64, n)
	nu := make([]float64, n)
	for i, id := range support {
		mu[i] = muFrom[id]
		nu[i] = muTo[id]
	}

	// Build the cost matrix using BFS shortest-path distances.
	cost := cc.buildCostMatrix(g, support)

	// Compute Wasserstein-1 distance via the Sinkhorn algorithm.
	w1 := sinkhorn(mu, nu, cost, cc.reg, cc.maxIter)

	// d(from, to) = 1 for a directly connected edge.
	return 1.0 - w1
}

// CalculateAll computes the Ollivier-Ricci curvature for every edge in the
// graph. Returns a map keyed by [from, to] pairs.
func (cc *CurvatureCalculator) CalculateAll(g GraphView) map[[2]string]float64 {
	result := make(map[[2]string]float64)
	g.ForEachEdge(func(e *graph.EdgeData) bool {
		key := [2]string{e.From, e.To}
		result[key] = cc.Calculate(g, e.From, e.To)
		return true
	})
	return result
}

// buildMeasure builds the lazy random walk probability measure for a node.
// mu_x(x) = alpha, and mu_x(neighbor) = (1 - alpha) / deg(x) for each
// neighbor of x. If the node has no neighbors, all mass stays on x.
func (cc *CurvatureCalculator) buildMeasure(g GraphView, nodeID string) map[string]float64 {
	neighbors := g.Neighbors(nodeID)
	measure := make(map[string]float64)

	if len(neighbors) == 0 {
		measure[nodeID] = 1.0
		return measure
	}

	measure[nodeID] = cc.alpha
	share := (1.0 - cc.alpha) / float64(len(neighbors))
	for _, nID := range neighbors {
		measure[nID] += share
	}

	return measure
}

// buildCostMatrix computes pairwise shortest-path distances among the support
// nodes using BFS from each support node, limited to a maximum depth of 4.
func (cc *CurvatureCalculator) buildCostMatrix(g GraphView, support []string) [][]float64 {
	n := len(support)

	cost := make([][]float64, n)
	for i := range cost {
		cost[i] = make([]float64, n)
		for j := range cost[i] {
			if i != j {
				// Default to a large distance if unreachable within BFS depth.
				cost[i][j] = 4.0
			}
		}
	}

	// BFS from each support node.
	const maxDepth = 4
	for i, src := range support {
		dist := bfsDistances(g, src, maxDepth)
		for j, dst := range support {
			if i == j {
				continue
			}
			if d, ok := dist[dst]; ok {
				cost[i][j] = float64(d)
			}
		}
	}

	return cost
}

// bfsDistances performs a breadth-first search from src up to maxDepth hops,
// returning a map of node ID to distance.
func bfsDistances(g GraphView, src string, maxDepth int) map[string]int {
	dist := map[string]int{src: 0}
	queue := []string{src}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		d := dist[current]
		if d >= maxDepth {
			continue
		}

		for _, neighbor := range g.Neighbors(current) {
			if _, visited := dist[neighbor]; !visited {
				dist[neighbor] = d + 1
				queue = append(queue, neighbor)
			}
		}
	}

	return dist
}

// sinkhorn computes an approximate Wasserstein-1 distance between two discrete
// distributions mu and nu using the Sinkhorn-Knopp algorithm for regularized
// optimal transport.
//
// Parameters:
//   - mu, nu: probability distributions (must sum to 1)
//   - cost: pairwise cost matrix (cost[i][j] = distance from i to j)
//   - reg: regularization parameter (smaller = more accurate, less stable)
//   - maxIter: maximum number of Sinkhorn iterations
func sinkhorn(mu, nu []float64, cost [][]float64, reg float64, maxIter int) float64 {
	n := len(mu)
	m := len(nu)

	if n == 0 || m == 0 {
		return 0
	}

	// Build the Gibbs kernel: K[i][j] = exp(-cost[i][j] / reg).
	K := make([][]float64, n)
	for i := range K {
		K[i] = make([]float64, m)
		for j := range K[i] {
			K[i][j] = math.Exp(-cost[i][j] / reg)
		}
	}

	// Initialize scaling vectors.
	u := make([]float64, n)
	v := make([]float64, m)
	for i := range u {
		u[i] = 1.0
	}
	for j := range v {
		v[j] = 1.0
	}

	for iter := 0; iter < maxIter; iter++ {
		// v = nu / (K^T u)
		for j := 0; j < m; j++ {
			sum := 0.0
			for i := 0; i < n; i++ {
				sum += K[i][j] * u[i]
			}
			if sum > 1e-300 {
				v[j] = nu[j] / sum
			}
		}

		// u = mu / (K v)
		for i := 0; i < n; i++ {
			sum := 0.0
			for j := 0; j < m; j++ {
				sum += K[i][j] * v[j]
			}
			if sum > 1e-300 {
				u[i] = mu[i] / sum
			}
		}
	}

	// Compute transport cost: sum_{i,j} u[i] * K[i][j] * v[j] * cost[i][j].
	total := 0.0
	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			gamma := u[i] * K[i][j] * v[j]
			total += gamma * cost[i][j]
		}
	}

	return total
}

// sortStrings sorts a slice of strings in place using insertion sort.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		key := s[i]
		j := i - 1
		for j >= 0 && s[j] > key {
			s[j+1] = s[j]
			j--
		}
		s[j+1] = key
	}
}
