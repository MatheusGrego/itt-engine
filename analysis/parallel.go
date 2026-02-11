package analysis

import (
	"runtime"
	"sync"

	"github.com/MatheusGrego/itt-engine/graph"
)

// CalculateAllParallel computes tension for all nodes using parallel workers.
// This is a drop-in replacement for TensionCalculator.CalculateAll() that uses
// multiple goroutines to speed up computation for large graphs.
//
// For graphs with < 100 nodes, falls back to sequential execution to avoid
// goroutine overhead.
func CalculateAllParallel(tc *TensionCalculator, g GraphView, workers int) map[string]float64 {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	// Collect all node IDs
	nodeIDs := make([]string, 0, g.NodeCount())
	g.ForEachNode(func(n *graph.NodeData) bool {
		nodeIDs = append(nodeIDs, n.ID)
		return true
	})

	// For very small graphs, parallel overhead > benefit
	if len(nodeIDs) < 100 {
		return tc.CalculateAll(g)
	}

	// Partition nodes into chunks (one per worker)
	chunks := partitionNodes(nodeIDs, workers)

	// Thread-safe result map (sync.Map for concurrent writes)
	var resultMap sync.Map

	// Launch worker pool
	var wg sync.WaitGroup
	for _, chunk := range chunks {
		wg.Add(1)
		go func(nodeIDs []string) {
			defer wg.Done()
			for _, id := range nodeIDs {
				tension := tc.Calculate(g, id)
				resultMap.Store(id, tension)
			}
		}(chunk)
	}

	// Wait for all workers
	wg.Wait()

	// Convert sync.Map to regular map
	results := make(map[string]float64, len(nodeIDs))
	resultMap.Range(func(key, value interface{}) bool {
		results[key.(string)] = value.(float64)
		return true
	})

	return results
}

// partitionNodes splits nodes into roughly equal chunks
func partitionNodes(nodes []string, workers int) [][]string {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	if len(nodes) == 0 {
		return nil
	}

	// Each chunk gets ceil(len(nodes) / workers) nodes
	chunkSize := (len(nodes) + workers - 1) / workers
	chunks := make([][]string, 0, workers)

	for i := 0; i < len(nodes); i += chunkSize {
		end := i + chunkSize
		if end > len(nodes) {
			end = len(nodes)
		}
		chunks = append(chunks, nodes[i:end])
	}

	return chunks
}
