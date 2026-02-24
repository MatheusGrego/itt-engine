package gpu

import (
	"fmt"
	"sync"
)

// GoSLBackend implements ComputeBackend using GoSL (Go → WGSL → WebGPU).
//
// Uses float32 arithmetic throughout, matching WGSL's f32 type.
// Precision: ε ≈ 1e-5 relative to the CPU float64 reference.
//
// When built with CGO and a GPU is available, dispatches the JSD kernel
// on the GPU via WebGPU compute shaders. Otherwise, falls back to the
// CPU float32 kernel (same algorithm, same results).
//
// Thread-safe: protected by a mutex for concurrent Snapshot.Analyze() calls.
type GoSLBackend struct {
	mu        sync.Mutex
	pipeline  *gpuPipeline // nil = CPU fallback mode
	info      DeviceInfo
	available bool
	closed    bool
	useGPU    bool

	// Serialization cache: avoids re-serializing the same graph on repeated calls.
	// Invalidated when graph fingerprint (node+edge count) changes.
	cachedCSR      *CSRGraphF32
	cachedCSC      *CSCGraph
	cacheNodeCount int
	cacheEdgeCount int
}

// NewGoSLBackend initializes the GPU backend.
//
// Attempts GPU initialization via WebGPU. If GPU is not available (no CGO,
// no driver, no compatible device), falls back to CPU-reference mode
// with the float32 kernel.
func NewGoSLBackend() (*GoSLBackend, error) {
	b := &GoSLBackend{
		available: true,
	}

	// Try GPU initialization (no-op if CGO not available)
	pipeline, err := initGPUPipeline()
	if err != nil || pipeline == nil {
		// CPU fallback mode
		b.info = DeviceInfo{
			Name:    "CPU Reference (f32)",
			Vendor:  "none",
			Backend: "GoSL (CPU fallback, float32)",
		}
		b.useGPU = false
		return b, nil
	}

	b.pipeline = pipeline
	b.info = pipeline.info
	b.useGPU = true
	return b, nil
}

func (b *GoSLBackend) Name() string { return "gosl" }

func (b *GoSLBackend) Available() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.available && !b.closed
}

func (b *GoSLBackend) DeviceInfo() DeviceInfo {
	return b.info
}

// AnalyzeTensions computes JSD tensions for all nodes via the float32 kernel pipeline.
//
// Pipeline:
//  1. Serialize graph to CSR (float32 weights) + CSC (indices only).
//     Uses cached serialization if the graph fingerprint matches.
//  2. Dispatch float32 JSD kernel (GPU if available, otherwise CPU).
//  3. Upcast float32 tensions to float64 and map indices back to node IDs.
//
// Precision: results match CPU float64 reference within ε ≈ 1e-5.
func (b *GoSLBackend) AnalyzeTensions(g GraphView) (map[string]float64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, fmt.Errorf("%w: backend closed", ErrGPUNotAvailable)
	}

	// 1. Serialize (with cache)
	nc, ec := g.NodeCount(), g.EdgeCount()
	csr, csc := b.cachedCSR, b.cachedCSC
	if csr == nil || b.cacheNodeCount != nc || b.cacheEdgeCount != ec {
		// Cache miss — serialize and store
		csr = SerializeCSRF32(g)
		csc = SerializeCSC(g, csr.NodeIdx)
		b.cachedCSR = csr
		b.cachedCSC = csc
		b.cacheNodeCount = nc
		b.cacheEdgeCount = ec
	}

	if csr.NumNodes == 0 {
		return make(map[string]float64), nil
	}

	// 2. Dispatch kernel
	var tensionsF32 []float32
	var err error

	if b.useGPU && b.pipeline != nil {
		// GPU path: WebGPU compute dispatch
		tensionsF32, err = b.pipeline.dispatch(
			csr.RowPtr, csr.ColIdx, csr.Values,
			csc.ColPtr, csc.RowIdx,
			csr.NumNodes,
		)
		if err != nil {
			// GPU dispatch failed — fall back to CPU for this call
			tensionsF32 = ComputeAllTensionsF32(
				csr.RowPtr, csr.ColIdx, csr.Values,
				csc.ColPtr, csc.RowIdx,
				int32(csr.NumNodes),
			)
		}
	} else {
		// CPU path: float32 kernel on CPU
		tensionsF32 = ComputeAllTensionsF32(
			csr.RowPtr, csr.ColIdx, csr.Values,
			csc.ColPtr, csc.RowIdx,
			int32(csr.NumNodes),
		)
	}

	// 3. Map back to node IDs (upcast float32 → float64 for API compatibility)
	result := make(map[string]float64, csr.NumNodes)
	for i, id := range csr.NodeIDs {
		result[id] = float64(tensionsF32[i])
	}

	return result, nil
}

// FiedlerApprox computes the Fiedler value on GPU.
//
// TODO: Implement GPU-accelerated inverse power iteration.
// For now, returns an error to signal the engine should use the CPU path.
func (b *GoSLBackend) FiedlerApprox(g GraphView, nodeIDs []string) (float64, error) {
	return 0, fmt.Errorf("%w: FiedlerApprox not yet implemented on GPU", ErrComputeFailed)
}

// Close releases GPU resources. Idempotent.
func (b *GoSLBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	if b.pipeline != nil {
		b.pipeline.release()
		b.pipeline = nil
	}

	b.closed = true
	b.available = false
	b.useGPU = false
	b.cachedCSR = nil
	b.cachedCSC = nil
	return nil
}
