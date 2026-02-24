package gpu

import (
	"sort"

	"github.com/MatheusGrego/itt-engine/graph"
)

// CSRGraph is a Compressed Sparse Row representation of a directed graph.
//
// CSR stores outgoing edges efficiently: for node at index i, its outgoing
// edges occupy ColIdx[RowPtr[i]:RowPtr[i+1]] with weights in Values at the
// same range. This layout enables coalesced GPU memory reads when iterating
// a node's outgoing neighbors.
//
// CSR is the standard GPU-friendly sparse matrix format and is compatible with
// cuSPARSE, WGSL storage buffers, and WebGPU compute shaders.
type CSRGraph struct {
	RowPtr   []int32   // length: NumNodes + 1 — row i spans [RowPtr[i], RowPtr[i+1])
	ColIdx   []int32   // length: NumEdges — column index (target node) of each edge
	Values   []float64 // length: NumEdges — edge weight
	NumNodes int
	NumEdges int
	NodeIDs  []string       // index → nodeID mapping (stable, sorted)
	NodeIdx  map[string]int // nodeID → index mapping (inverse of NodeIDs)
}

// CSCGraph is a Compressed Sparse Column representation (incoming edges).
//
// CSC stores incoming edges efficiently: for node at index j, its incoming
// edges occupy RowIdx[ColPtr[j]:ColPtr[j+1]] with weights in Values at the
// same range. This is the transpose of CSR.
//
// JSD computation requires both outgoing (CSR) and incoming (CSC) edge access
// to build the perturbation distributions. Dual format avoids an expensive
// GPU-side transpose.
type CSCGraph struct {
	ColPtr   []int32   // length: NumNodes + 1 — column j spans [ColPtr[j], ColPtr[j+1])
	RowIdx   []int32   // length: NumEdges — row index (source node) of each edge
	Values   []float64 // length: NumEdges — edge weight
	NumNodes int
	NumEdges int
}

// SerializeCSR converts a GraphView to CSR format.
//
// Node ordering is deterministic (sorted by ID) to ensure reproducible
// GPU results across runs. The returned CSRGraph includes the NodeIdx
// mapping needed by [SerializeCSC].
//
// Optimized: uses two ForEachEdge passes (count degrees → fill arrays)
// instead of per-node OutNeighbors calls, reducing allocations from
// O(n) per-node slices to a fixed number of flat arrays.
func SerializeCSR(g GraphView) *CSRGraph {
	n := g.NodeCount()
	if n == 0 {
		return &CSRGraph{
			RowPtr:  []int32{0},
			ColIdx:  nil,
			Values:  nil,
			NodeIDs: nil,
			NodeIdx: make(map[string]int),
		}
	}

	// Collect and sort node IDs for deterministic ordering.
	nodeIDs := make([]string, 0, n)
	g.ForEachNode(func(nd *graph.NodeData) bool {
		nodeIDs = append(nodeIDs, nd.ID)
		return true
	})
	sort.Strings(nodeIDs)

	// Build inverse map: nodeID → index.
	nodeIdx := make(map[string]int, n)
	for i, id := range nodeIDs {
		nodeIdx[id] = i
	}

	numEdges := g.EdgeCount()

	// --- Pass 1: count out-degrees per node ---
	rowPtr := make([]int32, n+1)
	g.ForEachEdge(func(ed *graph.EdgeData) bool {
		fromIdx, ok := nodeIdx[ed.From]
		if ok {
			rowPtr[fromIdx+1]++
		}
		return true
	})

	// Prefix sum to build row pointers.
	for i := 1; i <= n; i++ {
		rowPtr[i] += rowPtr[i-1]
	}

	// --- Pass 2: fill colIdx + values using write cursors ---
	colIdx := make([]int32, numEdges)
	values := make([]float64, numEdges)
	cursor := make([]int32, n) // write position per row
	copy(cursor, rowPtr[:n])

	g.ForEachEdge(func(ed *graph.EdgeData) bool {
		fromIdx, fromOK := nodeIdx[ed.From]
		toIdx, toOK := nodeIdx[ed.To]
		if !fromOK || !toOK {
			return true
		}
		pos := cursor[fromIdx]
		cursor[fromIdx]++
		colIdx[pos] = int32(toIdx)
		values[pos] = ed.Weight
		return true
	})

	// --- Sort within each row by column index (deterministic ordering) ---
	for i := 0; i < n; i++ {
		lo := int(rowPtr[i])
		hi := int(rowPtr[i+1])
		sortCSRRow(colIdx[lo:hi], values[lo:hi])
	}

	return &CSRGraph{
		RowPtr:   rowPtr,
		ColIdx:   colIdx,
		Values:   values,
		NumNodes: n,
		NumEdges: numEdges,
		NodeIDs:  nodeIDs,
		NodeIdx:  nodeIdx,
	}
}

// sortCSRRow sorts colIdx and values in parallel by colIdx, using insertion
// sort (optimal for small slices typical of graph rows, and zero heap allocs).
func sortCSRRow(col []int32, val []float64) {
	for i := 1; i < len(col); i++ {
		c, v := col[i], val[i]
		j := i - 1
		for j >= 0 && col[j] > c {
			col[j+1] = col[j]
			val[j+1] = val[j]
			j--
		}
		col[j+1] = c
		val[j+1] = v
	}
}

// CSRGraphF32 is a float32 variant of [CSRGraph] for GPU kernels.
//
// Indices remain int32; only edge weights use float32. This is the natural
// format for WGSL compute shaders, which do not support float64.
type CSRGraphF32 struct {
	RowPtr   []int32   // length: NumNodes + 1
	ColIdx   []int32   // length: NumEdges
	Values   []float32 // length: NumEdges — edge weights as float32
	NumNodes int
	NumEdges int
	NodeIDs  []string       // index → nodeID mapping (stable, sorted)
	NodeIdx  map[string]int // nodeID → index mapping (inverse of NodeIDs)
}

// SerializeCSRF32 converts a GraphView to CSR format with float32 weights.
//
// This wraps [SerializeCSR] and converts edge weights from float64 to float32.
// The index arrays (RowPtr, ColIdx) and node mappings are shared with the
// underlying CSR, so this adds only O(NumEdges) allocation for the float32 values.
func SerializeCSRF32(g GraphView) *CSRGraphF32 {
	csr := SerializeCSR(g)

	values32 := make([]float32, len(csr.Values))
	for i, v := range csr.Values {
		values32[i] = float32(v)
	}

	return &CSRGraphF32{
		RowPtr:   csr.RowPtr,
		ColIdx:   csr.ColIdx,
		Values:   values32,
		NumNodes: csr.NumNodes,
		NumEdges: csr.NumEdges,
		NodeIDs:  csr.NodeIDs,
		NodeIdx:  csr.NodeIdx,
	}
}

// SerializeCSC converts a GraphView to CSC format (incoming edges).
//
// nodeIdx is the same mapping produced by [SerializeCSR].NodeIdx.
// Both CSR and CSC must use the same node ordering for kernel correctness.
//
// Optimized: uses two ForEachEdge passes (count in-degrees → fill arrays)
// instead of per-node slice appends, reducing allocations from O(n+E) to
// a fixed number of flat arrays.
func SerializeCSC(g GraphView, nodeIdx map[string]int) *CSCGraph {
	n := g.NodeCount()
	if n == 0 {
		return &CSCGraph{
			ColPtr: []int32{0},
		}
	}

	numEdges := g.EdgeCount()

	// --- Pass 1: count in-degrees per node ---
	colPtr := make([]int32, n+1)
	g.ForEachEdge(func(ed *graph.EdgeData) bool {
		toIdx, ok := nodeIdx[ed.To]
		if ok {
			colPtr[toIdx+1]++
		}
		return true
	})

	// Prefix sum to build column pointers.
	for j := 1; j <= n; j++ {
		colPtr[j] += colPtr[j-1]
	}

	// --- Pass 2: fill rowIdx + values using write cursors ---
	rowIdx := make([]int32, numEdges)
	values := make([]float64, numEdges)
	cursor := make([]int32, n)
	copy(cursor, colPtr[:n])

	g.ForEachEdge(func(ed *graph.EdgeData) bool {
		fromIdx, fromOK := nodeIdx[ed.From]
		toIdx, toOK := nodeIdx[ed.To]
		if !fromOK || !toOK {
			return true
		}
		pos := cursor[toIdx]
		cursor[toIdx]++
		rowIdx[pos] = int32(fromIdx)
		values[pos] = ed.Weight
		return true
	})

	// --- Sort within each column by row index (deterministic ordering) ---
	for j := 0; j < n; j++ {
		lo := int(colPtr[j])
		hi := int(colPtr[j+1])
		sortCSRRow(rowIdx[lo:hi], values[lo:hi]) // reuse same insertion sort
	}

	return &CSCGraph{
		ColPtr:   colPtr,
		RowIdx:   rowIdx,
		Values:   values,
		NumNodes: n,
		NumEdges: numEdges,
	}
}
