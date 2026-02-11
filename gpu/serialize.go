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
	NodeIDs  []string        // index → nodeID mapping (stable, sorted)
	NodeIdx  map[string]int  // nodeID → index mapping (inverse of NodeIDs)
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
func SerializeCSR(g GraphView) *CSRGraph {
	n := g.NodeCount()
	if n == 0 {
		return &CSRGraph{
			RowPtr:   []int32{0},
			ColIdx:   nil,
			Values:   nil,
			NodeIDs:  nil,
			NodeIdx:  make(map[string]int),
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

	// Pre-count edges for capacity hints.
	numEdges := g.EdgeCount()

	// Build CSR arrays.
	rowPtr := make([]int32, n+1)
	colIdx := make([]int32, 0, numEdges)
	values := make([]float64, 0, numEdges)

	for i, nodeID := range nodeIDs {
		rowPtr[i] = int32(len(colIdx))

		// Collect outgoing edges and sort by target index for deterministic output.
		neighbors := g.OutNeighbors(nodeID)
		type outEdge struct {
			targetIdx int32
			weight    float64
		}
		edges := make([]outEdge, 0, len(neighbors))
		for _, neighborID := range neighbors {
			ed, ok := g.GetEdge(nodeID, neighborID)
			if !ok {
				continue
			}
			tIdx, exists := nodeIdx[neighborID]
			if !exists {
				continue
			}
			edges = append(edges, outEdge{targetIdx: int32(tIdx), weight: ed.Weight})
		}
		sort.Slice(edges, func(a, b int) bool {
			return edges[a].targetIdx < edges[b].targetIdx
		})

		for _, e := range edges {
			colIdx = append(colIdx, e.targetIdx)
			values = append(values, e.weight)
		}
	}
	rowPtr[n] = int32(len(colIdx))

	return &CSRGraph{
		RowPtr:   rowPtr,
		ColIdx:   colIdx,
		Values:   values,
		NumNodes: n,
		NumEdges: len(colIdx),
		NodeIDs:  nodeIDs,
		NodeIdx:  nodeIdx,
	}
}

// SerializeCSC converts a GraphView to CSC format (incoming edges).
//
// nodeIdx is the same mapping produced by [SerializeCSR].NodeIdx.
// Both CSR and CSC must use the same node ordering for kernel correctness.
func SerializeCSC(g GraphView, nodeIdx map[string]int) *CSCGraph {
	n := g.NodeCount()
	if n == 0 {
		return &CSCGraph{
			ColPtr: []int32{0},
		}
	}

	// Collect incoming edges per node.
	type inEdge struct {
		sourceIdx int32
		weight    float64
	}
	incoming := make([][]inEdge, n)

	g.ForEachEdge(func(ed *graph.EdgeData) bool {
		fromIdx, fromOK := nodeIdx[ed.From]
		toIdx, toOK := nodeIdx[ed.To]
		if !fromOK || !toOK {
			return true // skip edges with unknown nodes
		}
		incoming[toIdx] = append(incoming[toIdx], inEdge{
			sourceIdx: int32(fromIdx),
			weight:    ed.Weight,
		})
		return true
	})

	// Sort incoming edges per column by source index for deterministic output.
	for i := range incoming {
		sort.Slice(incoming[i], func(a, b int) bool {
			return incoming[i][a].sourceIdx < incoming[i][b].sourceIdx
		})
	}

	// Flatten into CSC arrays.
	numEdges := g.EdgeCount()
	colPtr := make([]int32, n+1)
	rowIdx := make([]int32, 0, numEdges)
	values := make([]float64, 0, numEdges)

	for j := 0; j < n; j++ {
		colPtr[j] = int32(len(rowIdx))
		for _, e := range incoming[j] {
			rowIdx = append(rowIdx, e.sourceIdx)
			values = append(values, e.weight)
		}
	}
	colPtr[n] = int32(len(rowIdx))

	return &CSCGraph{
		ColPtr:   colPtr,
		RowIdx:   rowIdx,
		Values:   values,
		NumNodes: n,
		NumEdges: len(rowIdx),
	}
}
