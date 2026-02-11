package gpu

import "math"

// This file contains the JSD tension kernel as a pure Go function.
//
// The function signature is designed for GPU execution (flat arrays, index-based),
// but is valid Go that can run on CPU for testing. GoSL will transpile this to WGSL
// for WebGPU dispatch.
//
// The algorithm exactly matches analysis.TensionCalculator.Calculate():
//  1. Find all neighbors of node i (union of in + out, deduplicated).
//  2. For each neighbor n, build n's outgoing weight distribution.
//  3. Simulate removal of node i: zero out edge n→i, re-normalize.
//  4. Compute JSD between original and perturbed distributions.
//  5. Return mean divergence across all contributing neighbors.

const kernelEpsilon = 1e-12

// ComputeNodeTension computes the informational tension for a single node.
//
// This is the GPU kernel entry point: one invocation per node.
// All data is passed as flat arrays matching CSR/CSC layout.
//
// Parameters:
//   - csrRowPtr, csrColIdx, csrValues: outgoing edges (CSR format)
//   - cscColPtr, cscRowIdx: incoming edges (CSC format, values not needed)
//   - nodeIdx: which node this invocation computes
//   - numNodes: total node count
//
// Returns the tension value for the node.
func ComputeNodeTension(
	csrRowPtr []int32, csrColIdx []int32, csrValues []float64,
	cscColPtr []int32, cscRowIdx []int32,
	nodeIdx int32, numNodes int32,
) float64 {
	if nodeIdx >= numNodes {
		return 0
	}

	// Step 1: Collect all neighbors of nodeIdx (union of out + in, deduplicated).
	neighbors := collectNeighbors(csrRowPtr, csrColIdx, cscColPtr, cscRowIdx, nodeIdx)
	if len(neighbors) == 0 {
		return 0
	}

	// Step 2: For each neighbor, compute JSD between original and perturbed distributions.
	totalDiv := 0.0
	count := 0

	for _, nIdx := range neighbors {
		// Get neighbor's outgoing edges: CSR row nIdx.
		outStart := csrRowPtr[nIdx]
		outEnd := csrRowPtr[nIdx+1]
		outDegree := int(outEnd - outStart)

		if outDegree == 0 {
			continue
		}

		// Build original and perturbed distributions.
		original := make([]float64, outDegree)
		perturbed := make([]float64, outDegree)

		for i := 0; i < outDegree; i++ {
			w := csrValues[outStart+int32(i)]
			target := csrColIdx[outStart+int32(i)]

			original[i] = w
			if target == nodeIdx {
				perturbed[i] = 0 // zero out edge to the removed node
			} else {
				perturbed[i] = w
			}
		}

		origDist := normalize(original)
		pertDist := normalize(perturbed)

		div := jsd(origDist, pertDist)
		totalDiv += div
		count++
	}

	if count == 0 {
		return 0
	}
	return totalDiv / float64(count)
}

// collectNeighbors returns the deduplicated union of outgoing and incoming
// neighbor indices for the given node.
func collectNeighbors(
	csrRowPtr []int32, csrColIdx []int32,
	cscColPtr []int32, cscRowIdx []int32,
	nodeIdx int32,
) []int32 {
	seen := make(map[int32]bool)
	var result []int32

	// Outgoing neighbors (CSR row).
	outStart := csrRowPtr[nodeIdx]
	outEnd := csrRowPtr[nodeIdx+1]
	for i := outStart; i < outEnd; i++ {
		target := csrColIdx[i]
		if !seen[target] {
			seen[target] = true
			result = append(result, target)
		}
	}

	// Incoming neighbors (CSC column).
	inStart := cscColPtr[nodeIdx]
	inEnd := cscColPtr[nodeIdx+1]
	for i := inStart; i < inEnd; i++ {
		source := cscRowIdx[i]
		if !seen[source] {
			seen[source] = true
			result = append(result, source)
		}
	}

	return result
}

// normalize returns a copy of dist where all values sum to 1.0.
// If sum is zero, returns a uniform distribution.
// Matches analysis.Normalize exactly.
func normalize(dist []float64) []float64 {
	total := 0.0
	for _, v := range dist {
		total += v
	}

	n := len(dist)
	result := make([]float64, n)

	if total == 0 {
		uniform := 1.0 / float64(n)
		for i := range result {
			result[i] = uniform
		}
		return result
	}

	for i, v := range dist {
		result[i] = v / total
	}
	return result
}

// jsd computes the Jensen-Shannon Divergence between distributions p and q.
// Matches analysis.JSD.Compute exactly, including epsilon smoothing.
func jsd(p, q []float64) float64 {
	m := make([]float64, len(p))
	for i := range p {
		m[i] = 0.5*p[i] + 0.5*q[i]
	}
	return 0.5*klDiv(p, m) + 0.5*klDiv(q, m)
}

// klDiv computes the KL divergence from p to q.
// Matches analysis.klDiv exactly, including epsilon smoothing.
func klDiv(p, q []float64) float64 {
	sum := 0.0
	for i := range p {
		pi := p[i] + kernelEpsilon
		qi := q[i] + kernelEpsilon
		if pi > kernelEpsilon {
			sum += pi * math.Log2(pi/qi)
		}
	}
	return sum
}

// ComputeAllTensions runs ComputeNodeTension for every node.
// This is the CPU fallback / reference implementation that the GPU kernel replaces.
func ComputeAllTensions(
	csrRowPtr []int32, csrColIdx []int32, csrValues []float64,
	cscColPtr []int32, cscRowIdx []int32,
	numNodes int32,
) []float64 {
	tensions := make([]float64, numNodes)
	for i := int32(0); i < numNodes; i++ {
		tensions[i] = ComputeNodeTension(
			csrRowPtr, csrColIdx, csrValues,
			cscColPtr, cscRowIdx,
			i, numNodes,
		)
	}
	return tensions
}
