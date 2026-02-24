# ITT Engine — GPU Acceleration (GoSL)

**Goal**: GPU-accelerated tensor computation via GoSL (Go → WGSL → WebGPU). Cross-platform, zero CGO.

**Date**: 2026-02-11
**Supersedes**: `2026-02-11-gpu-acceleration.md` (CUDA/CGO approach — deprecated)

---

## Executive Summary

### Why GoSL over CUDA?

| Criterion | CUDA (old plan) | GoSL (this plan) |
|---|---|---|
| **Platform** | NVIDIA only | Any GPU (NVIDIA, AMD, Intel, Apple Silicon) |
| **CGO** | Required (C++ interop) | Zero CGO (pure Go) |
| **Build** | NVCC toolchain, complex CI | `go build` — just works |
| **Kernel Language** | CUDA C (separate `.cu` files) | Go (GoSL transpiles to WGSL) |
| **Testability** | Hard (needs GPU for unit tests) | Easy (kernels are valid Go, CPU-testable) |
| **Backend** | CUDA Runtime | WebGPU (via wgpu-native) |
| **Maturity** | Most mature | Newer, but production-ready (Cogent Core) |
| **Debugging** | NVIDIA NSight | Go debugger works on kernel code |

**Decision**: GoSL provides 90% of CUDA's performance with 10% of the complexity. Cross-platform by default. No build toolchain pain.

### What is GoSL?

[GoSL](https://github.com/cogentcore/lab/tree/main/gosl) is a Go → GPU transpiler from the Cogent Core project:

1. You write **standard Go code** (with `//gosl:` annotations)
2. GoSL transpiles it to **WGSL** (WebGPU Shading Language)
3. At runtime, WGSL runs on any **WebGPU-compatible GPU**
4. Results are read back to Go via shared memory buffers

```
Go Code (jsd_kernel.go)
    ↓ GoSL transpiler
WGSL Shader (jsd_kernel.wgsl)
    ↓ WebGPU runtime
GPU Execution (any vendor)
    ↓ Buffer readback
Go Results (map[string]float64)
```

### Performance Targets

**Current CPU Performance** (Ryzen 5 3600, 12 cores):
```
Analyze 1k nodes:   277ms  (sequential), 153ms (parallel, Phase 1)
Analyze 25k nodes:  ~10s
Analyze 100k nodes: ~160s  (extrapolated)
```

**Expected GPU Performance**:
```
Analyze 1k nodes:   ~5ms     (55x speedup)
Analyze 10k nodes:  ~15ms    (186x speedup)
Analyze 100k nodes: ~80ms    (2000x speedup)
Analyze 1M nodes:   ~500ms   (9000x speedup)
```

**When GPU wins**: Graphs with > 1k nodes (below that, transfer overhead dominates)
**When CPU wins**: Small graphs, single-node analysis, already-cached results

---

## Architecture

### Hybrid CPU/GPU Pipeline

```
┌────────────────────────────────────────────────────────┐
│                 ITT Engine (CPU)                       │
│                                                        │
│   AddEvent() ──> MVCC Graph ──> Snapshot()            │
│                                      │                 │
│                              ┌───────▼────────┐       │
│                              │ computeTensions │       │
│                              │   (router)      │       │
│                              └───┬────────┬───┘       │
│                                  │        │            │
│               ┌──────────────────┘        └─────┐     │
│               ▼                                  ▼     │
│   ┌───────────────────┐           ┌──────────────────┐│
│   │    CPU Path       │           │    GPU Path      ││
│   │  (< threshold)    │           │  (>= threshold)  ││
│   │  Parallel JSD     │           │  GoSL dispatch   ││
│   └───────────────────┘           └────────┬─────────┘│
│                                            │          │
│              ┌─────────────────────────────┘          │
│              ▼                                         │
│   ┌───────────────────────────────────────────┐       │
│   │  Post-processing (CPU always)             │       │
│   │  - Curvature (per-edge, sparse)           │       │
│   │  - Concealment (BFS traversal)            │       │
│   │  - Temporal (history lookups)             │       │
│   │  - Anomaly detection                     │       │
│   └───────────────────────────────────────────┘       │
└────────────────────────────────────────────────────────┘
```

### Why Hybrid?

Not everything benefits from GPU:

| Operation | Best on | Why |
|---|---|---|
| **JSD Tensions** | GPU | Embarrassingly parallel, vector math |
| **Fiedler Value** | GPU | Dense matrix eigenvalue decomposition |
| **Curvature** | CPU | Sparse per-edge ops, random access patterns |
| **Concealment** | CPU | BFS graph traversal, irregular memory access |
| **Temporal** | CPU | History lookups, map access |
| **Anomaly Check** | CPU | Threshold comparison, callbacks |

**Strategy**: GPU computes the expensive tensor core (JSD + Fiedler), CPU handles everything else.

---

## Package Design

### New Package: `gpu/`

```
gpu/
├── backend.go          # ComputeBackend interface
├── serialize.go        # CSR + CSC graph serialization
├── serialize_test.go   # Roundtrip tests (pure Go, no GPU)
├── gosl_backend.go     # GoSL/WebGPU implementation
├── jsd_kernel.go       # JSD kernel in Go (GoSL-compatible)
└── gosl_backend_test.go # Parity test: CPU vs GPU
```

### 1. ComputeBackend Interface (`backend.go`)

Single abstraction over GPU computation. One implementation (GoSL) for now, extensible later.

```go
// File: gpu/backend.go
package gpu

import "github.com/MatheusGrego/itt-engine/analysis"

// DeviceInfo describes the GPU device.
type DeviceInfo struct {
    Name       string // e.g. "NVIDIA RTX 3090"
    Vendor     string // e.g. "NVIDIA", "AMD", "Intel", "Apple"
    Backend    string // "WebGPU (GoSL)"
    MemoryMB   int    // VRAM in MB
    MaxThreads int    // max compute threads
}

// ComputeBackend abstracts GPU computation.
// Single implementation (GoSL) for now, extensible later.
type ComputeBackend interface {
    // Name returns the backend identifier.
    Name() string

    // Available returns true if the GPU is usable.
    Available() bool

    // DeviceInfo returns GPU device details.
    DeviceInfo() DeviceInfo

    // AnalyzeTensions computes JSD-based tension for all nodes.
    // Returns map[nodeID]tension, matching TensionCalculator.CalculateAll() exactly.
    AnalyzeTensions(g analysis.GraphView) (map[string]float64, error)

    // FiedlerApprox computes the Fiedler value (algebraic connectivity).
    // Matches analysis.FiedlerApprox() semantics.
    FiedlerApprox(g analysis.GraphView, nodeIDs []string) (float64, error)

    // Close releases GPU resources.
    Close() error
}
```

**Why an interface?**
- Testable: inject mock backend in unit tests
- Extensible: add Vulkan/Metal backends later without changing engine
- Fallback-friendly: `Available()` check enables graceful CPU fallback

### 2. Graph Serialization (`serialize.go`)

GPU needs contiguous arrays. Convert graph to CSR (outgoing) + CSC (incoming) for bidirectional JSD.

```go
// File: gpu/serialize.go
package gpu

import "github.com/MatheusGrego/itt-engine/graph"

// CSRGraph is a Compressed Sparse Row representation (outgoing edges).
type CSRGraph struct {
    RowPtr   []int32   // length: numNodes + 1 (row i has edges [RowPtr[i], RowPtr[i+1]))
    ColIdx   []int32   // length: numEdges (column index of each edge)
    Values   []float64 // length: numEdges (edge weight)
    NumNodes int
    NumEdges int
    NodeIDs  []string  // index → nodeID mapping
}

// CSCGraph is a Compressed Sparse Column representation (incoming edges).
type CSCGraph struct {
    ColPtr   []int32   // length: numNodes + 1
    RowIdx   []int32   // length: numEdges
    Values   []float64 // length: numEdges
    NumNodes int
    NumEdges int
}

// SerializeCSR converts a GraphView to CSR format.
func SerializeCSR(g analysis.GraphView) *CSRGraph {
    n := g.NodeCount()
    nodeIDs := make([]string, 0, n)
    nodeIndex := make(map[string]int, n)

    // Assign stable index to each node
    idx := 0
    g.ForEachNode(func(node *graph.NodeData) bool {
        nodeIDs = append(nodeIDs, node.ID)
        nodeIndex[node.ID] = idx
        idx++
        return true
    })

    // Count edges
    numEdges := 0
    g.ForEachEdge(func(_ *graph.EdgeData) bool {
        numEdges++
        return true
    })

    // Build CSR
    rowPtr := make([]int32, n+1)
    colIdx := make([]int32, 0, numEdges)
    values := make([]float64, 0, numEdges)

    for i, nodeID := range nodeIDs {
        rowPtr[i] = int32(len(colIdx))
        neighbors := g.OutNeighbors(nodeID)
        for _, neighborID := range neighbors {
            edge, ok := g.GetEdge(nodeID, neighborID)
            if !ok {
                continue
            }
            colIdx = append(colIdx, int32(nodeIndex[neighborID]))
            values = append(values, edge.Weight)
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
    }
}

// SerializeCSC converts a GraphView to CSC format (incoming edges).
func SerializeCSC(g analysis.GraphView, nodeIndex map[string]int) *CSCGraph {
    n := g.NodeCount()
    numEdges := 0
    g.ForEachEdge(func(_ *graph.EdgeData) bool {
        numEdges++
        return true
    })

    // Build transposed edge list
    type edge struct {
        row int32
        val float64
    }
    cols := make([][]edge, n)

    g.ForEachEdge(func(e *graph.EdgeData) bool {
        fromIdx := int32(nodeIndex[e.From])
        toIdx := nodeIndex[e.To]
        cols[toIdx] = append(cols[toIdx], edge{row: fromIdx, val: e.Weight})
        return true
    })

    // Flatten to CSC
    colPtr := make([]int32, n+1)
    rowIdx := make([]int32, 0, numEdges)
    values := make([]float64, 0, numEdges)

    for i := 0; i < n; i++ {
        colPtr[i] = int32(len(rowIdx))
        for _, e := range cols[i] {
            rowIdx = append(rowIdx, e.row)
            values = append(values, e.val)
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
```

**Why CSR + CSC?**

JSD tension for node `i` requires:
- **Outgoing edges** (P distribution): CSR row `i`
- **Incoming edges** (context for Q distribution): CSC column `i`

Dual format avoids expensive transpose on GPU.

**Memory Layout Example**:
```
Graph: A→B(0.5), A→C(0.3), B→C(0.7), C→D(0.2), D→A(0.1)

CSR (outgoing):
  RowPtr: [0, 2, 3, 4, 5]    A:[0,2) B:[2,3) C:[3,4) D:[4,5)
  ColIdx: [1, 2, 2, 3, 0]    A→B,C  B→C    C→D    D→A
  Values: [0.5, 0.3, 0.7, 0.2, 0.1]

CSC (incoming):
  ColPtr: [0, 1, 2, 4, 5]    →A:[0,1) →B:[1,2) →C:[2,4) →D:[4,5)
  RowIdx: [3, 0, 0, 1, 2]    D→A    A→B    A→C,B→C  C→D
  Values: [0.1, 0.5, 0.3, 0.7, 0.2]
```

**Transfer Size** (PCIe 3.0 x16 = 16 GB/s):
```
10k nodes, 100k edges:
  CSR: ~1.2 MB → 0.075ms transfer
  CSC: ~1.2 MB → 0.075ms transfer
  Total: ~2.4 MB → 0.15ms ✅ negligible

100k nodes, 10M edges:
  CSR: ~120 MB → 7.5ms
  CSC: ~120 MB → 7.5ms
  Total: ~240 MB → 15ms ⚠️ acceptable (one-time cost)
```

### 3. JSD Kernel (`jsd_kernel.go`)

The kernel is **standard Go** with GoSL annotations. It matches `TensionCalculator.Calculate()` exactly.

```go
// File: gpu/jsd_kernel.go
package gpu

import "math"

//gosl:start

// ComputeNodeTension computes JSD-based tension for a single node.
// This function runs on the GPU — one invocation per node.
//
// Parameters are flat arrays (GPU-compatible):
//   - csrRowPtr, csrColIdx, csrValues: outgoing edges (CSR)
//   - cscColPtr, cscRowIdx, cscValues: incoming edges (CSC)
//   - tensions: output buffer, one float64 per node
//   - nodeIdx: which node this thread computes
//   - numNodes: total nodes
func ComputeNodeTension(
    csrRowPtr []int32, csrColIdx []int32, csrValues []float64,
    cscColPtr []int32, cscRowIdx []int32, cscValues []float64,
    tensions []float64,
    nodeIdx int32, numNodes int32,
) {
    if nodeIdx >= numNodes {
        return
    }

    // --- Build P distribution (this node's outgoing edge weights, normalized) ---
    outStart := csrRowPtr[nodeIdx]
    outEnd := csrRowPtr[nodeIdx+1]
    outDegree := outEnd - outStart

    if outDegree == 0 {
        tensions[nodeIdx] = 0.0
        return
    }

    // Sum outgoing weights
    sumP := 0.0
    for i := outStart; i < outEnd; i++ {
        sumP += csrValues[i]
    }
    if sumP == 0.0 {
        tensions[nodeIdx] = 0.0
        return
    }

    // --- Build Q distribution (average of neighbors' outgoing distributions) ---
    // For each neighbor, get their outgoing weight distribution and average them.

    // Step 1: Compute per-neighbor contribution to Q
    // Q[j] = mean of (neighbor_k's weight to target_j / neighbor_k's total weight)
    // over all neighbors k that connect to the same target j.
    //
    // Simplified: Q = uniform over outgoing targets (baseline expectation).
    // This matches the ITT paper's "reference distribution" definition.
    //
    // Advanced: Use neighbor distributions (requires gathering neighbor CSR rows).
    // For the GPU kernel, we use the perturbation approach:
    //   tension = sqrt(JSD(P, Q_ref))
    //   where Q_ref is constructed from incoming edge weight averages.

    // --- Incoming edges form the reference context ---
    inStart := cscColPtr[nodeIdx]
    inEnd := cscColPtr[nodeIdx+1]
    inDegree := inEnd - inStart

    // Reference distribution: weighted average of incoming weights
    sumIn := 0.0
    for i := inStart; i < inEnd; i++ {
        sumIn += cscValues[i]
    }

    // Combine degree information for tension
    totalDegree := float64(outDegree + inDegree)
    if totalDegree == 0 {
        tensions[nodeIdx] = 0.0
        return
    }

    // --- JSD Computation ---
    // P = normalized outgoing weights
    // Q = reference distribution (uniform over same dimension)
    //
    // JSD(P||Q) = 0.5 * KL(P||M) + 0.5 * KL(Q||M)
    // where M = 0.5 * (P + Q)

    jsd := 0.0
    for i := outStart; i < outEnd; i++ {
        p_i := csrValues[i] / sumP             // normalized outgoing weight
        q_i := 1.0 / float64(outDegree)        // uniform reference
        m_i := 0.5 * (p_i + q_i)

        if p_i > 0 && m_i > 0 {
            jsd += 0.5 * p_i * math.Log2(p_i/m_i)
        }
        if q_i > 0 && m_i > 0 {
            jsd += 0.5 * q_i * math.Log2(q_i/m_i)
        }
    }

    // Clamp to [0, 1] (JSD is bounded for probability distributions)
    if jsd < 0 {
        jsd = 0
    }

    tensions[nodeIdx] = math.Sqrt(jsd)
}

//gosl:end
```

**Key Design Decisions**:

1. **In-place from CSR**: No stack-allocated arrays (GPU has limited per-thread memory). Reads directly from CSR/CSC buffers.

2. **Float64 precision**: Matches CPU `TensionCalculator` exactly. No float32 truncation errors.

3. **GoSL annotations**: `//gosl:start` and `//gosl:end` mark the GPU-transpiled region. Everything between is valid Go AND valid WGSL (after transpilation).

4. **CPU-testable**: Since it's pure Go, we can call `ComputeNodeTension()` directly in unit tests to verify correctness without a GPU.

### 4. GoSL Backend (`gosl_backend.go`)

```go
// File: gpu/gosl_backend.go
package gpu

import (
    "fmt"

    "cogentcore.org/lab/gosl"
    "github.com/MatheusGrego/itt-engine/analysis"
    "github.com/MatheusGrego/itt-engine/graph"
)

// GoSLBackend implements ComputeBackend using GoSL (Go → WGSL → WebGPU).
type GoSLBackend struct {
    gpu  *gosl.GPU
    info DeviceInfo
}

// NewGoSLBackend initializes the GPU via WebGPU.
// Returns error if no GPU available (caller should fall back to CPU).
func NewGoSLBackend() (*GoSLBackend, error) {
    gpu := gosl.NewGPU()
    if err := gpu.Init(); err != nil {
        return nil, fmt.Errorf("gpu init failed: %w", err)
    }

    return &GoSLBackend{
        gpu: gpu,
        info: DeviceInfo{
            Name:    gpu.DeviceName(),
            Vendor:  gpu.Vendor(),
            Backend: "WebGPU (GoSL)",
        },
    }, nil
}

func (b *GoSLBackend) Name() string        { return "gosl" }
func (b *GoSLBackend) Available() bool      { return b.gpu != nil }
func (b *GoSLBackend) DeviceInfo() DeviceInfo { return b.info }

// AnalyzeTensions computes JSD tensions on GPU for all nodes.
func (b *GoSLBackend) AnalyzeTensions(g analysis.GraphView) (map[string]float64, error) {
    // 1. Serialize graph to CSR + CSC
    csr := SerializeCSR(g)
    nodeIndex := make(map[string]int, len(csr.NodeIDs))
    for i, id := range csr.NodeIDs {
        nodeIndex[id] = i
    }
    csc := SerializeCSC(g, nodeIndex)

    // 2. Allocate GPU buffers
    n := int32(csr.NumNodes)
    tensions := make([]float64, n)

    // 3. Configure compute dispatch
    //    Each workgroup processes 256 nodes (standard GPU workgroup size).
    //    Total workgroups = ceil(numNodes / 256).
    workgroupSize := 256
    numWorkgroups := (int(n) + workgroupSize - 1) / workgroupSize

    // 4. Upload buffers + dispatch kernel
    b.gpu.SetBufferData("csrRowPtr", csr.RowPtr)
    b.gpu.SetBufferData("csrColIdx", csr.ColIdx)
    b.gpu.SetBufferData("csrValues", csr.Values)
    b.gpu.SetBufferData("cscColPtr", csc.ColPtr)
    b.gpu.SetBufferData("cscRowIdx", csc.RowIdx)
    b.gpu.SetBufferData("cscValues", csc.Values)
    b.gpu.SetBufferData("tensions", tensions)

    // Dispatch: one thread per node
    b.gpu.Dispatch(numWorkgroups, 1, 1)

    // 5. Read back results
    b.gpu.ReadBufferData("tensions", tensions)

    // 6. Map indices back to node IDs
    result := make(map[string]float64, n)
    for i, id := range csr.NodeIDs {
        result[id] = tensions[i]
    }

    return result, nil
}

// FiedlerApprox computes algebraic connectivity on GPU.
func (b *GoSLBackend) FiedlerApprox(g analysis.GraphView, nodeIDs []string) (float64, error) {
    // TODO: Implement using inverse power iteration on GPU
    // For now, fall back to CPU
    return analysis.FiedlerApprox(g, nodeIDs), nil
}

func (b *GoSLBackend) Close() error {
    if b.gpu != nil {
        b.gpu.Release()
        b.gpu = nil
    }
    return nil
}
```

---

## Engine Integration

### 5. Builder Changes (`builder.go`)

```go
// File: builder.go (additions)

type Builder struct {
    // ... existing fields ...

    // GPU (Phase: GPU Acceleration)
    gpuBackend   gpu.ComputeBackend
    gpuThreshold int  // min nodes to route to GPU (default: 1000)
}

// WithGPU enables GPU acceleration with auto-detection.
// threshold = minimum node count to use GPU (default: 1000).
// If no GPU available, silently falls back to CPU.
func (b *Builder) WithGPU(threshold int) *Builder {
    backend, err := gpu.NewGoSLBackend()
    if err != nil {
        if b.logger != nil {
            b.logger.Warn("GPU not available, using CPU", "error", err)
        }
        return b // no GPU, just return
    }

    b.gpuBackend = backend
    b.gpuThreshold = threshold
    if b.logger != nil {
        info := backend.DeviceInfo()
        b.logger.Info("GPU acceleration enabled",
            "device", info.Name,
            "vendor", info.Vendor,
            "backend", info.Backend,
        )
    }
    return b
}

// WithGPUBackend injects a custom ComputeBackend (for testing).
func (b *Builder) WithGPUBackend(backend gpu.ComputeBackend) *Builder {
    b.gpuBackend = backend
    if b.gpuThreshold == 0 {
        b.gpuThreshold = 1000 // default
    }
    return b
}
```

### 6. Snapshot Routing (`snapshot.go`)

```go
// File: snapshot.go (modified Analyze method)

// computeTensions routes between CPU and GPU based on graph size.
func (s *Snapshot) computeTensions(tc *analysis.TensionCalculator, gv analysis.GraphView, workers int) map[string]float64 {
    nodeCount := gv.NodeCount()

    // GPU path: graph large enough AND backend available
    if s.config.gpuBackend != nil && s.config.gpuBackend.Available() && nodeCount >= s.config.gpuThreshold {
        tensions, err := s.config.gpuBackend.AnalyzeTensions(gv)
        if err != nil {
            // GPU failed — fall back to CPU silently
            if s.config.logger != nil {
                s.config.logger.Warn("GPU analysis failed, falling back to CPU", "error", err)
            }
            return analysis.CalculateAllParallel(tc, gv, workers)
        }
        return tensions
    }

    // CPU path: small graph or no GPU
    return analysis.CalculateAllParallel(tc, gv, workers)
}
```

Replace the direct call in `Analyze()`:

```diff
 // In Analyze(), around line ~264:
-tensions := analysis.CalculateAllParallel(tc, gv, workers)
+tensions := s.computeTensions(tc, gv, workers)
```

### 7. Engine Cleanup (`engine.go`)

```go
// In Engine.Stop():
func (e *Engine) Stop() {
    // ... existing cleanup ...

    // GPU cleanup
    if e.config.gpuBackend != nil {
        e.config.gpuBackend.Close()
    }
}
```

---

## Testing Strategy

### Serialization Tests (`gpu/serialize_test.go`)

**Pure Go, no GPU required. Runs everywhere.**

```go
func TestSerializeCSR_Roundtrip(t *testing.T) {
    // Build graph: A→B(0.5), A→C(0.3), B→C(0.7)
    g := graph.New()
    g.AddEdge("A", "B", 0.5, "test", time.Now())
    g.AddEdge("A", "C", 0.3, "test", time.Now())
    g.AddEdge("B", "C", 0.7, "test", time.Now())
    ig := g.Freeze()

    csr := SerializeCSR(ig)

    // Verify structure
    assert(t, csr.NumNodes == 3)
    assert(t, csr.NumEdges == 3)
    assert(t, len(csr.RowPtr) == 4)     // numNodes + 1
    assert(t, len(csr.ColIdx) == 3)     // numEdges
    assert(t, len(csr.Values) == 3)     // numEdges

    // Verify A has 2 outgoing edges, B has 1, C has 0
    aIdx := findNodeIdx(csr.NodeIDs, "A")
    bIdx := findNodeIdx(csr.NodeIDs, "B")
    cIdx := findNodeIdx(csr.NodeIDs, "C")

    assertDegree(t, csr, aIdx, 2)
    assertDegree(t, csr, bIdx, 1)
    assertDegree(t, csr, cIdx, 0)
}

func TestSerializeCSC_Roundtrip(t *testing.T) {
    // Same graph, verify incoming edges
    // B has 1 incoming (from A), C has 2 incoming (from A, B)
    ...
}

func TestSerialize_EmptyGraph(t *testing.T) {
    // Edge case: no nodes, no edges
    ...
}

func TestSerialize_SelfLoop(t *testing.T) {
    // A→A: self-loop handling
    ...
}

func TestSerialize_LargeGraph(t *testing.T) {
    // 10k nodes, 100k edges: verify memory layout is correct
    ...
}
```

### Parity Tests (`gpu/gosl_backend_test.go`)

**Critical: GPU results must match CPU results within epsilon.**

```go
func TestGPU_ParityCPU(t *testing.T) {
    if !gpuAvailable() {
        t.Skip("no GPU available")
    }

    // Build random graph
    g := buildRandomGraph(1000, 10000)

    // CPU: analysis.TensionCalculator
    tc := analysis.NewTensionCalculator(analysis.JSD{})
    cpuTensions := tc.CalculateAll(g)

    // GPU: GoSLBackend
    backend, _ := gpu.NewGoSLBackend()
    defer backend.Close()
    gpuTensions, err := backend.AnalyzeTensions(g)
    if err != nil {
        t.Fatalf("GPU analysis failed: %v", err)
    }

    // Assert parity: every node must match within ε = 1e-10
    epsilon := 1e-10
    for nodeID, cpuT := range cpuTensions {
        gpuT, ok := gpuTensions[nodeID]
        if !ok {
            t.Fatalf("node %s missing from GPU results", nodeID)
        }
        if math.Abs(cpuT-gpuT) > epsilon {
            t.Errorf("node %s: CPU=%.15f GPU=%.15f diff=%.2e",
                nodeID, cpuT, gpuT, math.Abs(cpuT-gpuT))
        }
    }
}
```

### Integration Test

```go
func TestEngine_GPUFallback(t *testing.T) {
    // Engine with GPU enabled but threshold=1M (so GPU is never used)
    // Verify CPU path still works
    ...
}

func TestEngine_GPUDisabled(t *testing.T) {
    // Engine without WithGPU() — GPU code is never invoked
    // Verify no regressions
    ...
}
```

---

## Milestones

### Milestone 1: Foundation -- DONE
- [x] Create branch `feature/gpu-acceleration`
- [x] Create `gpu/backend.go` — `ComputeBackend` interface (design change: `gpu.GraphView` re-declared, see below)
- [x] Create `gpu/serialize.go` — CSR + CSC serialization (design change: deterministic sorted ordering)
- [x] Create `gpu/serialize_test.go` — 12 tests + 3 benchmarks
- [ ] ~~Add `cogentcore/lab/gosl` dependency~~ (deferred to Milestone 5, see design change 3)

**Deliverable**: Pure-Go serialization passing tests. No GPU needed yet.

### Milestone 2: GoSL Kernel + Backend -- DONE
- [x] Create `gpu/jsd_kernel.go` — JSD tension kernel in Go (design change: exact perturbation algorithm, not simplified)
- [x] Create `gpu/jsd_kernel_test.go` — 12 parity tests + 2 benchmarks (10 graph topologies, ε=1e-10)
- [x] Create `gpu/gosl_backend.go` — GoSLBackend (CPU-reference mode, thread-safe)
- [x] Create `gpu/gosl_backend_test.go` — 9 tests + 2 benchmarks
- [x] Parity verified: CPU vs GPU tensions across 10 graph shapes (ε = 1e-10, zero mismatches)

**Deliverable**: Kernel computes same tensions as CPU for all tested graph topologies (up to 500 nodes).

### Milestone 3: Engine Integration -- DONE
- [x] Modify `builder.go` — `WithGPU(threshold)`, `WithGPUBackend(backend, threshold)` (design change: explicit threshold)
- [x] Modify `snapshot.go` — GPU routing in `Analyze()` via `computeTensions()` (design change: Snapshot method)
- [x] Modify `engine.go` — GPU cleanup on shutdown (`Stop()` calls `gpuBackend.Close()`)
- [x] Fallback: GPU error → CPU silently (logged if logger configured)
- [x] 9 integration tests covering routing, parity, fallback, lifecycle

**Deliverable**: `itt.NewBuilder().WithGPU(1000).Build()` works end-to-end. All 284 tests pass.

### Milestone 4: Tests & Benchmarks -- PENDING
- [x] Full test suite passes (`go test ./...`) — 284 tests across 8 packages
- [ ] Benchmark CPU vs GPU routing (100, 500, 1k nodes)
- [ ] Update this document with actual benchmark numbers
- [ ] Add GPU section to README.md

**Deliverable**: Benchmark numbers, documentation updated.

### Milestone 5: Wire GoSL (future)
- [ ] Add `cogentcore.org/lab/gosl` to `go.mod`
- [ ] Replace CPU loop in `GoSLBackend.AnalyzeTensions()` with `gpu.Dispatch()`
- [ ] Add `//gosl:start` / `//gosl:end` annotations to `jsd_kernel.go`
- [ ] Benchmark on actual GPU (RTX 2060)
- [ ] Implement `FiedlerApprox` on GPU

**Deliverable**: Actual GPU execution with measured speedup numbers.

---

## Performance Expectations

### Transfer Overhead

| Graph Size | CSR+CSC Size | Upload Time | Compute Time | Download Time | Total |
|---|---|---|---|---|---|
| 1k nodes, 10k edges | 240 KB | 0.015ms | 1ms | 0.01ms | ~1ms |
| 10k nodes, 100k edges | 2.4 MB | 0.15ms | 5ms | 0.1ms | ~5ms |
| 100k nodes, 1M edges | 24 MB | 1.5ms | 30ms | 1ms | ~33ms |
| 100k nodes, 10M edges | 240 MB | 15ms | 50ms | 10ms | ~75ms |

### Speedup Table

| Graph Size | CPU Sequential | CPU Parallel (12c) | GPU (GoSL) | Speedup vs CPU |
|---|---|---|---|---|
| 100 nodes | 312μs | 174μs | N/A (below threshold) | - |
| 1k nodes | 964μs | 537μs | ~1ms | ~0.5x (overhead dominates) |
| 10k nodes | ~28ms | ~15ms | ~5ms | 3-6x |
| 100k nodes | ~2.8s | ~1.5s | ~75ms | 20-37x |
| 1M nodes | ~280s | ~150s | ~500ms | 300-560x |

**Crossover point**: ~2-5k nodes (below this, CPU parallel is faster due to zero transfer overhead).

### Combined with Cache (Phase 2)

```
Best-case pipeline for 100k node graph:
1. Cache hit:     <1μs (instant) ← most requests
2. Cache miss:    75ms (GPU) ← first request per version
3. CPU fallback:  1.5s (if GPU fails) ← rare

Effective throughput: 10k-20k req/s (read-heavy)
Cold start:          ~75ms (vs 1.5s CPU, vs 10s sequential)
```

---

## Risks & Mitigations

### Risk 1: GoSL Maturity
**Problem**: GoSL is newer than CUDA. May have edge cases.
**Mitigation**: Parity tests. Every GPU result is validated against CPU. Automatic CPU fallback.

### Risk 2: WebGPU Driver Support
**Problem**: WebGPU is still maturing on some platforms.
**Mitigation**: GoSL uses wgpu-native which supports Windows, Linux, macOS. No browser needed.

### Risk 3: Float64 Precision
**Problem**: Some GPUs handle float64 slower than float32.
**Mitigation**: Accept the performance hit. Correctness > speed. JSD requires precision to avoid negative values from rounding.

### Risk 4: GoSL API Stability
**Problem**: GoSL API may change between versions.
**Mitigation**: Pin `cogentcore/lab` version in `go.mod`. Interface abstraction isolates changes.

### Risk 5: Large Graph Memory
**Problem**: 1M-node graph needs ~240 MB GPU RAM.
**Mitigation**: Check available VRAM before upload. Fallback to CPU if insufficient. Log warning.

---

## Comparison: Old Plan (CUDA) vs New Plan (GoSL)

| Aspect | CUDA Plan (deprecated) | GoSL Plan (current) |
|---|---|---|
| **Platform** | NVIDIA only | Cross-platform (NVIDIA, AMD, Intel, Apple) |
| **Build** | NVCC + CGO + complex CI | `go build` — standard Go toolchain |
| **Kernel Language** | CUDA C (.cu files) | Go (GoSL transpiles to WGSL) |
| **Testing** | Needs GPU for kernel tests | Kernels are valid Go — CPU-testable |
| **Performance** | ~100% of hardware potential | ~85-90% (WebGPU overhead) |
| **Debugging** | NVIDIA NSight (separate tool) | Go debugger (dlv) works on kernels |
| **Dependencies** | CUDA toolkit (2+ GB) | gosl module (~5 MB) |
| **New Packages** | gpu/ + C wrappers + CUDA headers | gpu/ only (pure Go) |
| **LOC Estimate** | ~2000 (Go+C+CUDA) | ~800 (Go only) |
| **Risk** | High (CGO, platform lock-in) | Medium (newer but simpler) |

**Bottom line**: GoSL trades ~10-15% peak performance for massive developer experience improvements. For ITT Engine's use case (not HPC/deep learning), this is the right trade-off.

---

**Status**: Milestones 1-3 Complete — CPU-reference mode. Milestone 4 (benchmarks + docs) pending.

---

## Implementation Status

### Milestone Progress

| Milestone | Status | Commit | Notes |
|---|---|---|---|
| **1: Foundation** | **Done** | `5c91aa3` | ComputeBackend interface, CSR/CSC serialization, 12 tests + 3 benchmarks |
| **2: Kernel + Backend** | **Done** | `ecfaacc` | JSD kernel with exact CPU parity (10 graph shapes, ε=1e-10), GoSL backend (CPU-reference), 33 GPU tests total |
| **3: Engine Integration** | **Done** | `b662979` | WithGPU/WithGPUBackend, computeTensions router, GPU cleanup on Stop, graceful fallback, 9 integration tests |
| **4: Tests & Benchmarks** | **Pending** | — | Final benchmarks, docs update, README GPU section |

### Test Inventory

| File | Tests | Benchmarks | Coverage |
|---|---|---|---|
| `gpu/serialize_test.go` | 12 | 3 | CSR/CSC: empty, triangle, weights, self-loop, disconnected, bidirectional, deterministic ordering, 1k nodes |
| `gpu/jsd_kernel_test.go` | 12 | 2 | Parity: triangle, bidirectional, star, chain, fully-connected, unequal weights, self-loop, disconnected, single-edge, 200-node random. Unit: isolated node, out-of-range |
| `gpu/gosl_backend_test.go` | 9 | 2 | Interface compliance, init/deviceInfo, parity (4-node + 500-node), empty graph, close idempotent, analyze-after-close, concurrent (10 goroutines), FiedlerApprox not-implemented |
| `gpu_integration_test.go` | 9 | 0 | Builder: WithGPU, zero-threshold, injection, negative-threshold. Routing: above/below threshold, CPU/GPU parity. Lifecycle: Stop closes backend. Fallback: GPU error → CPU. AnalyzeNode stays CPU |
| **Total** | **42** | **7** | |

### What Was Delivered

```
gpu/
├── backend.go           # ComputeBackend interface, GraphView, DeviceInfo, sentinel errors
├── serialize.go         # SerializeCSR + SerializeCSC (deterministic, sorted)
├── serialize_test.go    # 12 tests + 3 benchmarks
├── jsd_kernel.go        # ComputeNodeTension, ComputeAllTensions, JSD/KL helpers
├── jsd_kernel_test.go   # 12 parity tests + 2 benchmarks
├── gosl_backend.go      # GoSLBackend (CPU-reference mode)
├── gosl_backend_test.go # 9 tests + 2 benchmarks
└── helpers_test.go      # buildGraph, buildLargeGraph, nodeID

gpu_integration_test.go  # 9 engine-level GPU routing tests

builder.go               # +gpuBackend, +gpuThreshold, +WithGPU(), +WithGPUBackend()
snapshot.go              # +computeTensions() router, +gpu import
engine.go                # +gpuBackend.Close() in Stop()
```

### Current Mode: CPU-Reference

The entire GPU pipeline is wired and tested, but runs on CPU. The `GoSLBackend` executes the JSD kernel as pure Go — the same flat-array code that GoSL will transpile to WGSL.

```
Current flow:
  Analyze() → computeTensions() → GoSLBackend.AnalyzeTensions()
    → SerializeCSR/CSC → ComputeAllTensions (CPU loop) → map back to node IDs

Future flow (GoSL wired):
  Analyze() → computeTensions() → GoSLBackend.AnalyzeTensions()
    → SerializeCSR/CSC → gpu.Upload() → gpu.Dispatch() → gpu.ReadBack() → map back
```

Only `gosl_backend.go` changes when the GoSL dependency is added. Everything else (kernel, serialization, engine integration, tests) stays identical.

---

## Design Changes from Original Plan

### Change 1: `gpu.GraphView` re-declared instead of importing `analysis.GraphView`

**Plan**: `gpu/backend.go` imports `analysis` and uses `analysis.GraphView` directly.

**Implementation**: `gpu.GraphView` is re-declared in `gpu/backend.go` using `*graph.NodeData` / `*graph.EdgeData`.

**Why**: Avoids `gpu → analysis` dependency. The `gpu` package depends only on `graph` (leaf types). Go structural typing guarantees that `*graph.ImmutableGraph` and `*graph.UnifiedView` satisfy both `analysis.GraphView` and `gpu.GraphView` without any adapter. Cleaner dependency graph:

```
Before (planned):     gpu → analysis → graph
After (implemented):  gpu → graph
                      analysis → graph
```

### Change 2: JSD kernel uses exact perturbation algorithm (not simplified P/Q)

**Plan**: Kernel used a simplified approach — P = normalized outgoing weights, Q = uniform reference. Computed `JSD(P, Q_uniform)`.

**Implementation**: Kernel exactly replicates `analysis.TensionCalculator.Calculate()` — the **neighbor-perturbation** algorithm:
1. Collect all neighbors (in + out union, deduplicated)
2. For each neighbor, build its outgoing weight distribution
3. Zero out the edge to the target node, re-normalize
4. Compute JSD(original, perturbed)
5. Return mean divergence across all contributing neighbors

**Why**: The simplified version would produce **different results** from the CPU TensionCalculator. The parity requirement (ε = 1e-10) demands identical algorithms. The perturbation approach is the actual ITT definition of informational tension. Verified across 10 graph topologies with zero mismatches.

### Change 3: GoSL dependency not added yet (CPU-reference mode)

**Plan**: Milestone 2 adds `cogentcore.org/lab/gosl` and runs on GPU.

**Implementation**: GoSLBackend runs the kernel on CPU. No external dependency added.

**Why**: This is deliberate phasing. The architecture (interface, serialization, kernel, integration) is fully proven and tested without the GoSL dependency. Adding GoSL is a localized change to `gosl_backend.go` only — swap the `ComputeAllTensions` CPU loop for `gpu.Dispatch()`. This avoids coupling GoSL API stability risks to architectural decisions.

### Change 4: Deterministic serialization with sorted ordering

**Plan**: CSR used `ForEachNode` iteration order (non-deterministic, depends on map iteration).

**Implementation**: Node IDs are sorted alphabetically before indexing. Edges within each CSR row and CSC column are also sorted by target index.

**Why**: Deterministic ordering is required for: (a) reproducible test results, (b) coalesced GPU memory access patterns (sorted column indices improve cache hit rates on GPU), (c) consistent cache keys if CSR data is ever cached.

### Change 5: `WithGPUBackend` takes explicit threshold

**Plan**: `WithGPUBackend(backend)` with auto-default threshold of 1000.

**Implementation**: `WithGPUBackend(backend, threshold)` — explicit second parameter.

**Why**: Hidden defaults are surprising. The caller should decide the routing threshold based on their workload characteristics. `WithGPU(threshold)` handles the common case (auto-detect + explicit threshold). `WithGPUBackend(backend, threshold)` is for test injection and alternative backends — these callers always know what threshold they want.

### Change 6: `computeTensions` is a Snapshot method, not a free function

**Plan**: `computeTensions(tc, gv, workers)` as a function receiving all parameters.

**Implementation**: `computeTensions(gv, tc)` as a method on `*Snapshot`. Workers are read from `s.config.parallelWorkers` internally.

**Why**: As a method, it has direct access to `s.config` (gpuBackend, gpuThreshold, parallelWorkers, logger). No need to pass all configuration as parameters. The GPU type assertion `gv.(gpu.GraphView)` uses comma-ok pattern for safety.

### Change 7: Thread-safe GoSLBackend with mutex

**Plan**: No mention of concurrency in GoSLBackend.

**Implementation**: `sync.Mutex` protects all mutable state in GoSLBackend. `closed` and `available` flags are guarded.

**Why**: Multiple snapshots may call `AnalyzeTensions` concurrently from different goroutines. WebGPU command queues are not thread-safe — the mutex ensures serialized dispatch. Also prevents use-after-close races.

### Change 8: GPU routing only for `Analyze()`, not `AnalyzeNode()` / `AnalyzeRegion()`

**Plan**: Not specified — implied all analysis methods.

**Implementation**: Only `Analyze()` (full-graph) routes to GPU. `AnalyzeNode()` and `AnalyzeRegion()` always use CPU.

**Why**: GPU overhead (serialize + upload + dispatch + readback) is ~1-5ms minimum. Single-node analysis on CPU takes ~1.2μs. Region analysis typically covers a small subset. The crossover point is ~2-5k nodes — only full-graph `Analyze()` benefits.

### Change 9: `CSRGraph.NodeIdx` field added

**Plan**: CSR had `NodeIDs []string` only. CSC construction required a separate `nodeIndex map[string]int`.

**Implementation**: `CSRGraph` includes `NodeIdx map[string]int` alongside `NodeIDs []string`.

**Why**: Avoids rebuilding the index map every time CSC is constructed. The map is already computed during CSR serialization — exposing it saves an O(n) allocation. Used by both `SerializeCSC` and the parity test helpers.

---

## Next Steps

### Milestone 4: Tests & Benchmarks (immediate)

- [ ] Add GPU vs CPU benchmarks in `gpu_integration_test.go` (100, 500, 1k nodes)
- [ ] Add benchmark comparing `Analyze()` with GPU routing vs CPU-only
- [ ] Update this document with actual benchmark numbers
- [ ] Add GPU section to `README.md` with usage examples

### Milestone 5: Wire GoSL (when dependency is added)

- [ ] Add `cogentcore.org/lab/gosl` to `go.mod`
- [ ] Replace CPU loop in `GoSLBackend.AnalyzeTensions()` with `gpu.Dispatch()`
- [ ] Add `//gosl:start` / `//gosl:end` annotations to `jsd_kernel.go`
- [ ] Run parity tests on actual GPU (RTX 2060)
- [ ] Benchmark real GPU vs CPU and update performance tables
- [ ] Implement `FiedlerApprox` on GPU (inverse power iteration kernel)

### Future Optimizations

- [ ] **Batch serialization cache**: Cache CSR/CSC between consecutive `Analyze()` calls on the same version (avoids re-serializing unchanged graphs)
- [ ] **Pinned memory**: Use pinned (page-locked) host memory for faster PCIe transfers
- [ ] **Async dispatch**: Overlap GPU compute with CPU post-processing (curvature, concealment)
- [ ] **Multi-GPU**: Partition large graphs across multiple GPUs
- [ ] **Float32 fast path**: Optional float32 mode for graphs where precision can be relaxed (non-anomaly workloads)
