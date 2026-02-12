// Package gpu provides GPU-accelerated computation for the ITT Engine.
//
// The package follows the Strategy pattern via the [ComputeBackend] interface,
// allowing the engine to swap between GPU implementations (GoSL, future Vulkan, etc.)
// without changing analysis logic.
//
// Design principles:
//   - Interface Segregation: ComputeBackend exposes only what the engine needs.
//   - Dependency Inversion: Engine depends on the interface, not the concrete backend.
//   - Open/Closed: New backends (Vulkan, Metal) can be added without modifying engine code.
//   - Graceful Degradation: GPU failure always falls back to CPU transparently.
package gpu

import (
	"errors"
	"fmt"

	"github.com/MatheusGrego/itt-engine/graph"
)

// Sentinel errors for GPU operations.
var (
	ErrGPUInitFailed   = errors.New("gpu: initialization failed")
	ErrGPUNotAvailable = errors.New("gpu: device not available")
	ErrUploadFailed    = errors.New("gpu: buffer upload failed")
	ErrComputeFailed   = errors.New("gpu: compute dispatch failed")
	ErrDownloadFailed  = errors.New("gpu: buffer download failed")
)

// DeviceInfo describes the GPU device capabilities.
type DeviceInfo struct {
	Name       string // e.g. "NVIDIA RTX 3090", "Apple M2 GPU"
	Vendor     string // e.g. "NVIDIA", "AMD", "Intel", "Apple"
	Backend    string // e.g. "WebGPU (GoSL)"
	MemoryMB   int    // VRAM in MB (0 if unknown)
	MaxThreads int    // max compute threads per dispatch (0 if unknown)
}

// String returns a human-readable description of the device.
func (d DeviceInfo) String() string {
	if d.Name == "" {
		return fmt.Sprintf("%s (%s)", d.Backend, d.Vendor)
	}
	return fmt.Sprintf("%s — %s (%s)", d.Name, d.Backend, d.Vendor)
}

// GraphView is the read-only graph interface required by GPU backends.
//
// This is structurally identical to [analysis.GraphView]. We re-declare it
// here so that the gpu package depends only on graph (leaf types), not on the
// analysis package. Go structural typing ensures any value satisfying
// analysis.GraphView also satisfies gpu.GraphView.
type GraphView interface {
	GetNode(id string) (*graph.NodeData, bool)
	GetEdge(from, to string) (*graph.EdgeData, bool)
	Neighbors(nodeID string) []string
	OutNeighbors(nodeID string) []string
	InNeighbors(nodeID string) []string
	NodeCount() int
	EdgeCount() int
	ForEachNode(fn func(*graph.NodeData) bool)
	ForEachEdge(fn func(*graph.EdgeData) bool)
}

// ComputeBackend abstracts GPU-accelerated computation.
//
// Implementations must be safe for concurrent use from multiple goroutines.
// The engine holds a single backend instance and calls it from parallel snapshot analyses.
//
// Contract:
//   - [Available] must be callable at any time without side effects.
//   - [AnalyzeTensions] must return results consistent with [analysis.TensionCalculator.CalculateAll].
//     Float64 backends: ε = 1e-10. Float32 backends (e.g. GPU/WGSL): ε = 1e-5.
//   - [Close] must be idempotent — calling it multiple times is safe.
//   - If any method returns an error, the engine will fall back to CPU transparently.
type ComputeBackend interface {
	// Name returns the backend identifier (e.g. "gosl", "vulkan").
	Name() string

	// Available reports whether the GPU device is usable.
	// Must be safe to call at any time, even after Close.
	Available() bool

	// DeviceInfo returns GPU device details.
	DeviceInfo() DeviceInfo

	// AnalyzeTensions computes JSD-based tension for all nodes in the graph.
	//
	// The returned map must contain an entry for every node in the graph.
	// Values must match TensionCalculator.CalculateAll within the backend's
	// precision tolerance (float64: ε=1e-10, float32/GPU: ε=1e-5).
	//
	// Implementations should:
	//  1. Serialize the graph to GPU-friendly format (CSR/CSC).
	//  2. Upload buffers to GPU memory.
	//  3. Dispatch the JSD kernel (one thread per node).
	//  4. Download the tension results.
	//  5. Map indices back to node IDs.
	AnalyzeTensions(g GraphView) (map[string]float64, error)

	// FiedlerApprox computes the Fiedler value (algebraic connectivity) on GPU.
	//
	// The Fiedler value is the second-smallest eigenvalue of the graph Laplacian.
	// Must match analysis.FiedlerApprox within ε = 1e-6.
	FiedlerApprox(g GraphView, nodeIDs []string) (float64, error)

	// Close releases all GPU resources.
	// Must be idempotent — safe to call multiple times.
	Close() error
}
