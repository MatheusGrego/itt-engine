package analysis

import (
	"math"
	"math/rand"
)

// FiedlerValue computes an approximation of the algebraic connectivity
// (second-smallest eigenvalue of the graph Laplacian, also known as lambda_1)
// using inverse power iteration with a Jacobi iterative solver.
//
// The algorithm:
//  1. Build the graph Laplacian L = D - A for the specified nodes.
//  2. Use inverse iteration: repeatedly solve L * x = v for x using Jacobi iteration.
//  3. After each solve, orthogonalize x against the constant vector (null space of L)
//     so that convergence targets the eigenvector for lambda_1 rather than lambda_0 = 0.
//  4. Compute the Rayleigh quotient to estimate lambda_1.
//
// Returns 0 if the graph has fewer than 2 nodes or is disconnected.
// nodeIDs specifies which nodes to include in the subgraph.
// maxIter controls the number of outer inverse iteration steps.
// tol is the convergence tolerance on the eigenvalue estimate.
func FiedlerValue(g GraphView, nodeIDs []string, maxIter int, tol float64) float64 {
	n := len(nodeIDs)
	if n < 2 {
		return 0
	}

	// Build index map.
	idx := make(map[string]int, n)
	for i, id := range nodeIDs {
		idx[id] = i
	}

	// Build Laplacian in sparse form: diagonal (degree) + adjacency weights.
	degree := make([]float64, n)
	// adj stores off-diagonal entries: adj[i] maps j -> accumulated weight.
	adj := make([]map[int]float64, n)
	for i := range adj {
		adj[i] = make(map[int]float64)
	}

	for i, id := range nodeIDs {
		for _, neighbor := range g.Neighbors(id) {
			j, ok := idx[neighbor]
			if !ok || i == j {
				continue
			}
			w := 1.0
			if e, ok := g.GetEdge(id, neighbor); ok {
				w = e.Weight
				if w <= 0 {
					w = 1.0
				}
			}
			adj[i][j] += w
			degree[i] += w
		}
	}

	// Check that all nodes have nonzero degree (basic connectivity check).
	for i := 0; i < n; i++ {
		if degree[i] == 0 {
			return 0 // isolated node means graph is disconnected
		}
	}

	// Initialize a random starting vector orthogonal to the constant vector.
	rng := rand.New(rand.NewSource(42))
	v := make([]float64, n)
	for i := range v {
		v[i] = rng.Float64() - 0.5
	}
	laplacianOrthogonalize(v)
	laplacianNormalize(v)

	// Inverse iteration: solve L * x = v using Jacobi iteration,
	// then set v = x / ||x||. This converges to the eigenvector of the
	// smallest eigenvalue of L. Since we orthogonalize against the null
	// space (constant vector), we skip lambda_0 = 0 and converge to lambda_1.
	var lambda float64
	jacobiIter := 50 // inner iterations for Jacobi solver

	for iter := 0; iter < maxIter; iter++ {
		// Solve L * x = v using Jacobi iteration.
		x := jacobiSolveLaplacian(degree, adj, v, n, jacobiIter)

		// Orthogonalize against constant vector.
		laplacianOrthogonalize(x)

		norm := laplacianNormalize(x)
		if norm < 1e-15 {
			return 0 // degenerate case
		}

		// Compute Rayleigh quotient: lambda = v^T L x / (x^T x).
		// Since x is normalized, x^T x = 1. And v = L * x_prev (approximately),
		// so we compute x^T L x directly.
		lx := laplacianMultiply(degree, adj, x, n)
		newLambda := laplacianDot(x, lx)

		// The Rayleigh quotient of the inverse-iteration vector gives 1/lambda_1,
		// but since we compute x^T L x (not x^T L^{-1} x), we get lambda directly.
		// Actually, inverse iteration: if x converges to the eigenvector of the
		// smallest eigenvalue, then x^T L x -> lambda_1.

		if iter > 0 && math.Abs(newLambda-lambda) < tol {
			lambda = newLambda
			break
		}
		lambda = newLambda

		// Update v for next iteration.
		copy(v, x)
	}

	if lambda < 1e-12 {
		return 0 // effectively disconnected
	}
	return lambda
}

// jacobiSolveLaplacian approximately solves L * x = b using Jacobi iteration,
// where L is the graph Laplacian with diagonal = degree and off-diagonal = -adj.
func jacobiSolveLaplacian(degree []float64, adj []map[int]float64, b []float64, n int, maxIter int) []float64 {
	x := make([]float64, n)
	xNew := make([]float64, n)

	// Initialize x = b / diag(L) as a starting point.
	for i := 0; i < n; i++ {
		if degree[i] > 0 {
			x[i] = b[i] / degree[i]
		}
	}

	for iter := 0; iter < maxIter; iter++ {
		for i := 0; i < n; i++ {
			if degree[i] == 0 {
				xNew[i] = 0
				continue
			}
			// L[i][i] = degree[i], L[i][j] = -adj[i][j]
			// Jacobi: x_new[i] = (b[i] - sum_{j!=i} L[i][j] * x[j]) / L[i][i]
			//        = (b[i] + sum_j adj[i][j] * x[j]) / degree[i]
			sum := 0.0
			for j, w := range adj[i] {
				sum += w * x[j]
			}
			xNew[i] = (b[i] + sum) / degree[i]
		}
		x, xNew = xNew, x
	}

	return x
}

// laplacianMultiply computes y = L * x where L is the graph Laplacian.
func laplacianMultiply(degree []float64, adj []map[int]float64, x []float64, n int) []float64 {
	y := make([]float64, n)
	for i := 0; i < n; i++ {
		y[i] = degree[i] * x[i]
		for j, w := range adj[i] {
			y[i] -= w * x[j]
		}
	}
	return y
}

// FiedlerApprox returns a fast lower-bound approximation of algebraic connectivity
// using the Cheeger inequality: lambda_1 >= h^2 / (2 * d_max).
//
// h (the Cheeger constant) is estimated by computing edge expansion for a
// BFS-based bisection of the graph. This is much faster than eigendecomposition
// but provides only a lower bound.
//
// Returns 0 for disconnected graphs or graphs with fewer than 2 nodes.
func FiedlerApprox(g GraphView, nodeIDs []string) float64 {
	n := len(nodeIDs)
	if n < 2 {
		return 0
	}

	// Build adjacency info.
	nodeSet := make(map[string]bool, n)
	for _, id := range nodeIDs {
		nodeSet[id] = true
	}

	// Find max degree within the node set.
	dMax := 0.0
	for _, id := range nodeIDs {
		d := 0.0
		for _, neighbor := range g.Neighbors(id) {
			if nodeSet[neighbor] {
				d++
			}
		}
		if d > dMax {
			dMax = d
		}
	}
	if dMax == 0 {
		return 0
	}

	// Pick node with the highest degree as the BFS start node.
	startNode := nodeIDs[0]
	maxDeg := 0
	for _, id := range nodeIDs {
		d := 0
		for _, neighbor := range g.Neighbors(id) {
			if nodeSet[neighbor] {
				d++
			}
		}
		if d > maxDeg {
			maxDeg = d
			startNode = id
		}
	}

	// BFS ordering from start node.
	visited := make(map[string]bool, n)
	queue := []string{startNode}
	visited[startNode] = true
	order := make([]string, 0, n)

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		order = append(order, curr)
		for _, neighbor := range g.Neighbors(curr) {
			if nodeSet[neighbor] && !visited[neighbor] {
				visited[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}

	// Check connectivity: if BFS didn't reach all nodes, graph is disconnected.
	if len(order) < n {
		return 0
	}

	// Try partition at n/2.
	half := n / 2
	if half == 0 {
		half = 1
	}
	setS := make(map[string]bool, half)
	for i := 0; i < half; i++ {
		setS[order[i]] = true
	}

	// Count cut edges (edges crossing from S to complement).
	cut := 0.0
	for id := range setS {
		for _, neighbor := range g.Neighbors(id) {
			if nodeSet[neighbor] && !setS[neighbor] {
				cut++
			}
		}
	}

	// Volume of S = sum of degrees of nodes in S (counting only edges within nodeSet).
	volS := 0.0
	for id := range setS {
		for _, neighbor := range g.Neighbors(id) {
			if nodeSet[neighbor] {
				volS++
			}
		}
	}

	// Volume of complement.
	volComplement := 0.0
	for _, id := range nodeIDs {
		if !setS[id] {
			for _, neighbor := range g.Neighbors(id) {
				if nodeSet[neighbor] {
					volComplement++
				}
			}
		}
	}

	minVol := volS
	if volComplement < minVol {
		minVol = volComplement
	}
	if minVol == 0 {
		return 0
	}

	// Cheeger constant estimate.
	h := cut / minVol

	// Cheeger inequality: lambda_1 >= h^2 / (2 * d_max).
	return (h * h) / (2 * dMax)
}

// laplacianOrthogonalize removes the projection of v onto the constant vector,
// ensuring it lies in the orthogonal complement of the null space of the Laplacian.
func laplacianOrthogonalize(v []float64) {
	n := len(v)
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	mean := sum / float64(n)
	for i := range v {
		v[i] -= mean
	}
}

// laplacianNormalize scales v to unit length, returning the original norm.
func laplacianNormalize(v []float64) float64 {
	norm := 0.0
	for _, x := range v {
		norm += x * x
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return 0
	}
	for i := range v {
		v[i] /= norm
	}
	return norm
}

// laplacianDot computes the dot product of vectors a and b.
func laplacianDot(a, b []float64) float64 {
	sum := 0.0
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}
