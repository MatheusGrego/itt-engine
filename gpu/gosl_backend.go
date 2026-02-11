package gpu

import (
	"fmt"
	"sync"
)

// GoSLBackend implements ComputeBackend using GoSL (Go → WGSL → WebGPU).
//
// Current implementation runs the JSD kernel on CPU as a reference/fallback.
// When GoSL is wired in, the ComputeAllTensions call will be replaced by
// a WebGPU compute dispatch, while the rest of the pipeline (serialize,
// deserialize, fallback) stays identical.
//
// Thread-safe: protected by a mutex for concurrent Snapshot.Analyze() calls.
type GoSLBackend struct {
	mu        sync.Mutex
	info      DeviceInfo
	available bool
	closed    bool
}

// NewGoSLBackend initializes the GPU backend.
//
// Currently operates in CPU-reference mode (runs kernel on CPU).
// TODO: Initialize WebGPU device via GoSL when dependency is added.
func NewGoSLBackend() (*GoSLBackend, error) {
	// TODO: Replace with actual GoSL GPU initialization:
	//   gpu := gosl.NewGPU()
	//   if err := gpu.Init(); err != nil { return nil, ... }
	//
	// For now, always succeed in CPU-reference mode.
	return &GoSLBackend{
		info: DeviceInfo{
			Name:    "CPU Reference",
			Vendor:  "none",
			Backend: "GoSL (CPU fallback)",
		},
		available: true,
	}, nil
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

// AnalyzeTensions computes JSD tensions for all nodes via the kernel pipeline.
//
// Pipeline:
//  1. Serialize graph to CSR + CSC (flat arrays).
//  2. Run JSD kernel (currently CPU; will be GPU dispatch via GoSL).
//  3. Map tensor indices back to node IDs.
func (b *GoSLBackend) AnalyzeTensions(g GraphView) (map[string]float64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, fmt.Errorf("%w: backend closed", ErrGPUNotAvailable)
	}

	// 1. Serialize
	csr := SerializeCSR(g)
	csc := SerializeCSC(g, csr.NodeIdx)

	if csr.NumNodes == 0 {
		return make(map[string]float64), nil
	}

	// 2. Compute (CPU reference — GoSL will replace this with GPU dispatch)
	//
	// When GoSL is wired in, this becomes:
	//   b.gpu.SetBufferData("csrRowPtr", csr.RowPtr)
	//   b.gpu.SetBufferData("csrColIdx", csr.ColIdx)
	//   ... (upload all buffers)
	//   b.gpu.Dispatch(numWorkgroups, 1, 1)
	//   b.gpu.ReadBufferData("tensions", tensions)
	//
	tensions := ComputeAllTensions(
		csr.RowPtr, csr.ColIdx, csr.Values,
		csc.ColPtr, csc.RowIdx,
		int32(csr.NumNodes),
	)

	// 3. Map back to node IDs
	result := make(map[string]float64, csr.NumNodes)
	for i, id := range csr.NodeIDs {
		result[id] = tensions[i]
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

	// TODO: Release GoSL GPU resources:
	//   b.gpu.Release()

	b.closed = true
	b.available = false
	return nil
}
