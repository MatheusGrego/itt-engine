package gpu

import "math"

// This file contains the float32 JSD tension kernel designed for GPU execution.
//
// The float32 kernel is structurally compatible with GoSL transpilation to WGSL:
//   - No dynamic allocation (no map, make, or append)
//   - No multiple return values
//   - Fixed-size workspace buffers instead of slices
//   - float32 arithmetic throughout (WGSL does not support float64)
//
// The algorithm is identical to jsd_kernel.go (float64) but with relaxed precision.
// Expected parity with CPU reference: ε ≈ 1e-5 (vs 1e-10 for float64).
//
// For anomaly detection, float32 is more than sufficient: typical thresholds
// are 0.1–0.5 and JSD values lie in [0, 0.7]. Float32 provides ~7 significant
// digits, giving precision of ~1e-7 in this range.

const kernelEpsilonF32 float32 = 1e-7

// MaxNeighbors is the maximum number of neighbors per node for the GPU kernel.
// Nodes with degree exceeding this limit will have excess neighbors silently ignored.
const MaxNeighbors = 512

// MaxOutDegree is the maximum outgoing edge count per neighbor for the GPU kernel.
// Neighbors with more outgoing edges than this limit are skipped.
const MaxOutDegree = 512

// ComputeNodeTensionF32 computes the informational tension for a single node
// using float32 arithmetic. This is the GPU kernel entry point.
//
// Algorithm matches [ComputeNodeTension] (float64) exactly, with:
//   - float64 → float32 throughout
//   - map → linear search for neighbor deduplication
//   - make/append → fixed-size stack buffers
func ComputeNodeTensionF32(
	csrRowPtr []int32, csrColIdx []int32, csrValues []float32,
	cscColPtr []int32, cscRowIdx []int32,
	nodeIdx int32, numNodes int32,
) float32 {
	if nodeIdx >= numNodes {
		return 0
	}

	// Step 1: Collect all neighbors (no map, no append — GPU-compatible).
	var neighborBuf [MaxNeighbors]int32
	numNeighbors := collectNeighborsF32(
		csrRowPtr, csrColIdx, cscColPtr, cscRowIdx,
		nodeIdx, neighborBuf[:],
	)
	if numNeighbors == 0 {
		return 0
	}

	// Step 2: For each neighbor, compute JSD between original and perturbed distributions.
	var totalDiv float32
	count := int32(0)

	// Pre-allocated workspace buffers (no make — GPU-compatible).
	var original [MaxOutDegree]float32
	var perturbed [MaxOutDegree]float32
	var mBuf [MaxOutDegree]float32

	for ni := int32(0); ni < numNeighbors; ni++ {
		nIdx := neighborBuf[ni]
		outStart := csrRowPtr[nIdx]
		outEnd := csrRowPtr[nIdx+1]
		outDegree := outEnd - outStart

		if outDegree == 0 || outDegree > MaxOutDegree {
			continue
		}

		// Build original and perturbed distributions.
		for i := int32(0); i < outDegree; i++ {
			w := csrValues[outStart+i]
			target := csrColIdx[outStart+i]

			original[i] = w
			if target == nodeIdx {
				perturbed[i] = 0
			} else {
				perturbed[i] = w
			}
		}

		// Normalize in-place.
		normalizeInPlaceF32(original[:outDegree])
		normalizeInPlaceF32(perturbed[:outDegree])

		// JSD with pre-allocated M buffer.
		div := jsdF32(original[:outDegree], perturbed[:outDegree], mBuf[:outDegree])
		totalDiv += div
		count++
	}

	if count == 0 {
		return 0
	}
	return totalDiv / float32(count)
}

// collectNeighborsF32 writes the deduplicated union of outgoing and incoming
// neighbor indices into buf. Returns the number of neighbors found.
// Uses linear search for deduplication (GPU-compatible: no map).
func collectNeighborsF32(
	csrRowPtr []int32, csrColIdx []int32,
	cscColPtr []int32, cscRowIdx []int32,
	nodeIdx int32, buf []int32,
) int32 {
	count := int32(0)
	maxBuf := int32(len(buf))

	// Outgoing neighbors (CSR row).
	outStart := csrRowPtr[nodeIdx]
	outEnd := csrRowPtr[nodeIdx+1]
	for i := outStart; i < outEnd; i++ {
		target := csrColIdx[i]
		if !containsI32(buf[:count], target) && count < maxBuf {
			buf[count] = target
			count++
		}
	}

	// Incoming neighbors (CSC column).
	inStart := cscColPtr[nodeIdx]
	inEnd := cscColPtr[nodeIdx+1]
	for i := inStart; i < inEnd; i++ {
		source := cscRowIdx[i]
		if !containsI32(buf[:count], source) && count < maxBuf {
			buf[count] = source
			count++
		}
	}

	return count
}

// containsI32 checks if val exists in the slice using linear search.
// GPU-compatible alternative to map[int32]bool.
func containsI32(s []int32, val int32) bool {
	for _, v := range s {
		if v == val {
			return true
		}
	}
	return false
}

// normalizeInPlaceF32 normalizes dist in-place so values sum to 1.0.
// If sum is zero, sets uniform distribution. No allocation.
func normalizeInPlaceF32(dist []float32) {
	var total float32
	for _, v := range dist {
		total += v
	}

	n := len(dist)
	if total == 0 {
		uniform := float32(1.0) / float32(n)
		for i := range dist {
			dist[i] = uniform
		}
		return
	}

	for i := range dist {
		dist[i] /= total
	}
}

// jsdF32 computes Jensen-Shannon Divergence between p and q using float32.
// m is a pre-allocated buffer for the mixture distribution (must be len >= len(p)).
func jsdF32(p, q []float32, m []float32) float32 {
	for i := range p {
		m[i] = 0.5*p[i] + 0.5*q[i]
	}
	return 0.5*klDivF32(p, m) + 0.5*klDivF32(q, m)
}

// klDivF32 computes KL divergence from p to q using float32.
func klDivF32(p, q []float32) float32 {
	var sum float32
	for i := range p {
		pi := p[i] + kernelEpsilonF32
		qi := q[i] + kernelEpsilonF32
		if pi > kernelEpsilonF32 {
			sum += pi * log2f32(pi/qi)
		}
	}
	return sum
}

// log2f32 computes log2 in float32.
// On GPU (WGSL), this maps to the native log2() intrinsic.
func log2f32(x float32) float32 {
	return float32(math.Log2(float64(x)))
}

// ComputeAllTensionsF32 runs ComputeNodeTensionF32 for every node.
// This is the CPU fallback for the float32 kernel.
func ComputeAllTensionsF32(
	csrRowPtr []int32, csrColIdx []int32, csrValues []float32,
	cscColPtr []int32, cscRowIdx []int32,
	numNodes int32,
) []float32 {
	tensions := make([]float32, numNodes)
	for i := int32(0); i < numNodes; i++ {
		tensions[i] = ComputeNodeTensionF32(
			csrRowPtr, csrColIdx, csrValues,
			cscColPtr, cscRowIdx,
			i, numNodes,
		)
	}
	return tensions
}
