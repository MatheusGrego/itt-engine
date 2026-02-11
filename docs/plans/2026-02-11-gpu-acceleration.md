# ITT Engine — GPU Acceleration Plan

**Goal**: Enable GPU-accelerated tensor computation for extreme-scale workloads (100k+ nodes, real-time batch analysis)

**Date**: 2026-02-11
**Target Use Cases**: Black hole detection, network security at ISP scale, financial fraud detection (millions of transactions/day)

---

## Executive Summary

### Why GPU?

**Current CPU Performance** (Ryzen 5 3600, 12 cores):
```
Analyze 1k nodes:   277ms  (3.6 analyses/sec)
Analyze 25k nodes:  ~10s   (0.1 analyses/sec)
Analyze 100k nodes: ~160s  (0.006 analyses/sec) [extrapolated]
```

**GPU Promise**:
- **Massive Parallelism**: 10,000+ CUDA cores vs 12 CPU cores
- **SIMD Operations**: Perfect for JSD/KL divergence (vector operations)
- **Dense Linear Algebra**: Laplacian eigenvalues (Fiedler) in milliseconds
- **Batch Processing**: Analyze 100 snapshots in parallel

**Expected Speedup**:
- **JSD Computation**: 50-200x (embarrassingly parallel)
- **Laplacian Operations**: 100-500x (dense matrix ops)
- **Batch Analysis**: 10-100x (multiple graphs simultaneously)
- **Overall**: 20-100x for graphs > 10k nodes

**When NOT to use GPU**:
- Graphs < 1,000 nodes (CPU overhead > GPU benefit)
- Single analysis (memory transfer overhead dominates)
- Sparse graphs with low degree (GPU underutilized)

---

## Architecture Overview

### 1. Hybrid CPU/GPU Pipeline

```
┌─────────────────────────────────────────────────────────────┐
│                    ITT Engine (CPU)                         │
│  ┌──────────────┐    ┌─────────────┐    ┌──────────────┐  │
│  │  AddEvent()  │───>│ MVCC Graph  │───>│ Snapshot()   │  │
│  └──────────────┘    └─────────────┘    └──────────────┘  │
│                            │                      │         │
│                            v                      v         │
│                    ┌───────────────────────────────────┐   │
│                    │  Analyze() Router                 │   │
│                    │  - Check graph size               │   │
│                    │  - Check GPU availability         │   │
│                    │  - Route: CPU vs GPU vs Hybrid    │   │
│                    └───────────────────────────────────┘   │
│                            │                      │         │
│         ┌──────────────────┴──────────────────┐  │         │
│         v                                      v  v         │
│  ┌──────────────┐                      ┌──────────────┐   │
│  │  CPU Path    │                      │  GPU Path    │   │
│  │ (< 1k nodes) │                      │ (> 10k nodes)│   │
│  └──────────────┘                      └──────────────┘   │
└─────────────────────────────────────────────────────────────┘
                                                 │
                                                 v
┌─────────────────────────────────────────────────────────────┐
│                    GPU Accelerator                          │
│  ┌──────────────┐    ┌─────────────┐    ┌──────────────┐  │
│  │ Graph        │───>│ Tensor      │───>│ Results      │  │
│  │ Serializer   │    │ Kernels     │    │ Deserializer │  │
│  │ (CPU→GPU)    │    │ (CUDA/CL)   │    │ (GPU→CPU)    │  │
│  └──────────────┘    └─────────────┘    └──────────────┘  │
│         │                   │                    │          │
│         v                   v                    v          │
│  ┌──────────────────────────────────────────────────────┐  │
│  │          GPU Memory (Device RAM)                     │  │
│  │  - Adjacency Matrix (sparse CSR format)             │  │
│  │  - Degree Vectors (in/out/total)                    │  │
│  │  - Edge Weight Matrix                               │  │
│  │  - Tension Results Vector                           │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### 2. Memory Transfer Strategy

**Problem**: CPU ↔ GPU transfer is expensive (PCIe bandwidth ~16 GB/s)

**Solution**: Minimize transfers via persistent GPU memory

```go
type GPUAccelerator struct {
    // Persistent GPU memory (reused across analyses)
    deviceGraph     *GPUGraph       // adjacency matrix in GPU RAM
    deviceTensions  *GPUVector      // result buffer

    // Transfer only deltas
    lastVersion     uint64
    dirtyNodes      map[string]bool // nodes modified since last transfer

    // Batch processing
    batchQueue      chan *Snapshot  // queue for batch analysis
    batchSize       int             // analyze N snapshots together
}
```

**Transfer Patterns**:
1. **Cold Start** (first analysis):
   - CPU → GPU: Full graph (~1ms for 10k nodes)
   - GPU compute: 5ms
   - GPU → CPU: Tensions only (~100μs)
   - **Total**: ~6ms

2. **Warm Update** (incremental):
   - CPU → GPU: Only modified edges (~10μs for 100 events)
   - GPU compute: 5ms
   - GPU → CPU: Tensions only (~100μs)
   - **Total**: ~5.1ms

3. **Batch Mode** (100 snapshots):
   - CPU → GPU: 100 graphs (~100ms)
   - GPU compute: 500ms (parallel)
   - GPU → CPU: 100 result sets (~10ms)
   - **Total**: ~610ms = 6.1ms per analysis

---

## Phase 1: Foundation (CUDA Backend)

### 1.1 Technology Selection

**Option A: CUDA (NVIDIA only)** ✅ RECOMMENDED
- **Pros**: Most mature, best tooling, gonum/cuda bindings exist
- **Cons**: NVIDIA GPUs only (no AMD/Intel)
- **Performance**: Highest (cuBLAS, cuSPARSE optimized)

**Option B: OpenCL (Cross-platform)**
- **Pros**: Works on NVIDIA/AMD/Intel
- **Cons**: Less mature in Go, slower than CUDA
- **Performance**: 70-80% of CUDA

**Option C: Vulkan Compute (Modern)**
- **Pros**: Cross-platform, modern API
- **Cons**: Immature Go bindings, complex
- **Performance**: Similar to OpenCL

**Decision**: Start with **CUDA** (target NVIDIA GPUs), add OpenCL fallback in Phase 2.

### 1.2 Go ↔ CUDA Integration

**Challenge**: Go has no native GPU support. Must use CGO + CUDA C.

**Architecture**:
```
Go Application
    ↓ (CGO call)
C Wrapper (gpu_wrapper.c)
    ↓ (CUDA API)
CUDA Kernels (kernels.cu)
    ↓ (NVCC compile)
PTX/CUBIN (GPU binary)
```

**Implementation**:

```go
// File: gpu/accelerator.go
package gpu

/*
#cgo CFLAGS: -I/usr/local/cuda/include
#cgo LDFLAGS: -L/usr/local/cuda/lib64 -lcudart -lcublas -lcusparse

#include "gpu_wrapper.h"
*/
import "C"
import (
    "unsafe"
    "github.com/MatheusGrego/itt-engine/graph"
)

type Accelerator struct {
    ctx     C.GPUContext
    enabled bool
}

func NewAccelerator() (*Accelerator, error) {
    var ctx C.GPUContext
    if C.gpu_init(&ctx) != 0 {
        return nil, ErrGPUInitFailed
    }
    return &Accelerator{ctx: ctx, enabled: true}, nil
}

// AnalyzeTensions computes JSD-based tension for all nodes
func (a *Accelerator) AnalyzeTensions(g *graph.ImmutableGraph) (map[string]float64, error) {
    // 1. Serialize graph to CSR format
    csr := serializeToCSR(g)

    // 2. Transfer to GPU
    deviceGraph := C.gpu_upload_graph(
        a.ctx,
        (*C.int)(unsafe.Pointer(&csr.rowPtr[0])),
        (*C.int)(unsafe.Pointer(&csr.colIdx[0])),
        (*C.float)(unsafe.Pointer(&csr.values[0])),
        C.int(csr.numRows),
        C.int(csr.numNonZero),
    )
    defer C.gpu_free_graph(a.ctx, deviceGraph)

    // 3. Allocate result buffer
    tensions := make([]float64, g.NodeCount())
    deviceTensions := C.gpu_alloc_vector(a.ctx, C.int(len(tensions)))
    defer C.gpu_free_vector(a.ctx, deviceTensions)

    // 4. Launch kernel
    C.gpu_compute_jsd_tensions(a.ctx, deviceGraph, deviceTensions)

    // 5. Download results
    C.gpu_download_vector(
        a.ctx,
        deviceTensions,
        (*C.double)(unsafe.Pointer(&tensions[0])),
        C.int(len(tensions)),
    )

    // 6. Map back to node IDs
    result := make(map[string]float64, len(tensions))
    i := 0
    g.ForEachNode(func(n *graph.NodeData) bool {
        result[n.ID] = tensions[i]
        i++
        return true
    })

    return result, nil
}
```

### 1.3 CUDA Kernels

**Kernel 1: JSD Computation** (Embarrassingly Parallel)

```cuda
// File: gpu/kernels/jsd.cu

__global__ void compute_jsd_tensions(
    const int* rowPtr,      // CSR row pointers
    const int* colIdx,      // CSR column indices
    const float* values,    // Edge weights
    const int numNodes,
    double* tensions        // Output: tension per node
) {
    int nodeIdx = blockIdx.x * blockDim.x + threadIdx.x;
    if (nodeIdx >= numNodes) return;

    // Get this node's edges
    int edgeStart = rowPtr[nodeIdx];
    int edgeEnd = rowPtr[nodeIdx + 1];
    int degree = edgeEnd - edgeStart;

    if (degree == 0) {
        tensions[nodeIdx] = 0.0;
        return;
    }

    // Build empirical distribution P (this node's outgoing edges)
    float P[MAX_DEGREE];
    float sumP = 0.0f;
    for (int i = 0; i < degree; i++) {
        P[i] = values[edgeStart + i];
        sumP += P[i];
    }
    // Normalize
    for (int i = 0; i < degree; i++) {
        P[i] /= sumP;
    }

    // Compute mean distribution Q (average of neighbors' distributions)
    float Q[MAX_DEGREE];
    for (int i = 0; i < degree; i++) {
        int neighborIdx = colIdx[edgeStart + i];
        int nEdgeStart = rowPtr[neighborIdx];
        int nEdgeEnd = rowPtr[neighborIdx + 1];
        int nDegree = nEdgeEnd - nEdgeStart;

        // TODO: Average neighbor distributions (simplified here)
        Q[i] = 1.0f / degree; // uniform for now
    }

    // Compute JSD = 0.5 * (KL(P||M) + KL(Q||M)) where M = 0.5*(P+Q)
    float M[MAX_DEGREE];
    float jsd = 0.0f;
    for (int i = 0; i < degree; i++) {
        M[i] = 0.5f * (P[i] + Q[i]);
        if (P[i] > 0) jsd += 0.5f * P[i] * log2f(P[i] / M[i]);
        if (Q[i] > 0) jsd += 0.5f * Q[i] * log2f(Q[i] / M[i]);
    }

    tensions[nodeIdx] = sqrt(jsd); // ITT tension = sqrt(JSD)
}
```

**Launch Configuration**:
```c
// Threads per block: 256 (optimal for most GPUs)
// Blocks: ceil(numNodes / 256)
int threadsPerBlock = 256;
int numBlocks = (numNodes + threadsPerBlock - 1) / threadsPerBlock;

compute_jsd_tensions<<<numBlocks, threadsPerBlock>>>(
    rowPtr, colIdx, values, numNodes, tensions
);
cudaDeviceSynchronize();
```

**Kernel 2: Laplacian Eigenvalue** (Dense Linear Algebra)

```cuda
// Use cuSPARSE + cuSOLVER for Fiedler value
// Approximate via inverse power iteration

#include <cusparse.h>
#include <cusolver_sp.h>

void gpu_fiedler_approx(
    cusparseHandle_t cusparseH,
    cusolverSpHandle_t cusolverH,
    const CSRMatrix* A,  // adjacency matrix
    double* lambda1      // output: Fiedler value
) {
    // 1. Build Laplacian: L = D - A
    // 2. Inverse power iteration to find smallest non-zero eigenvalue
    // 3. Use cuSOLVER's LOBPCG (Locally Optimal Block Preconditioned Conjugate Gradient)

    cusolverSpDcsreigvsi(
        cusolverH,
        A->numRows,
        A->nnz,
        A->descrA,
        A->csrVal,
        A->csrRowPtr,
        A->csrColIdx,
        0.0,  // shift (find eigenvalue near 0)
        NULL, // initial guess
        1000, // max iterations
        1e-6, // tolerance
        &mu,  // output: eigenvalue
        x     // output: eigenvector
    );

    *lambda1 = mu;
}
```

### 1.4 Graph Serialization (CPU → GPU)

**Challenge**: `graph.ImmutableGraph` is a Go struct with pointers. GPU needs contiguous arrays.

**Solution**: Convert to CSR (Compressed Sparse Row) format.

```go
// File: gpu/serialize.go

type CSRGraph struct {
    RowPtr    []int32   // length: numNodes + 1
    ColIdx    []int32   // length: numEdges
    Values    []float32 // length: numEdges
    NumNodes  int
    NumEdges  int
    NodeIDs   []string  // map index → nodeID
}

func SerializeToCSR(g *graph.ImmutableGraph) *CSRGraph {
    n := g.NodeCount()
    nodeIDs := make([]string, 0, n)
    nodeIndex := make(map[string]int, n)

    // Assign index to each node
    idx := 0
    g.ForEachNode(func(node *graph.NodeData) bool {
        nodeIDs = append(nodeIDs, node.ID)
        nodeIndex[node.ID] = idx
        idx++
        return true
    })

    // Count edges for pre-allocation
    numEdges := 0
    g.ForEachEdge(func(_ *graph.EdgeData) bool {
        numEdges++
        return true
    })

    // Build CSR
    rowPtr := make([]int32, n+1)
    colIdx := make([]int32, numEdges)
    values := make([]float32, numEdges)

    edgeIdx := 0
    for i, nodeID := range nodeIDs {
        rowPtr[i] = int32(edgeIdx)

        neighbors := g.OutNeighbors(nodeID)
        for _, neighborID := range neighbors {
            edge, _ := g.GetEdge(nodeID, neighborID)
            colIdx[edgeIdx] = int32(nodeIndex[neighborID])
            values[edgeIdx] = float32(edge.Weight)
            edgeIdx++
        }
    }
    rowPtr[n] = int32(edgeIdx)

    return &CSRGraph{
        RowPtr:   rowPtr,
        ColIdx:   colIdx,
        Values:   values,
        NumNodes: n,
        NumEdges: numEdges,
        NodeIDs:  nodeIDs,
    }
}
```

**Memory Layout**:
```
Example: Graph with 4 nodes, 5 edges
A → B (0.5)
A → C (0.3)
B → C (0.7)
C → D (0.2)
D → A (0.1)

CSR Format:
RowPtr:  [0, 2, 3, 4, 5]  // node A has edges [0,2), node B has [2,3), etc.
ColIdx:  [1, 2, 2, 3, 0]  // edge 0 goes to node 1 (B), edge 1 to node 2 (C), etc.
Values:  [0.5, 0.3, 0.7, 0.2, 0.1]
NodeIDs: ["A", "B", "C", "D"]
```

**Transfer Time** (PCIe 3.0 x16 = 16 GB/s):
```
10k nodes, 100k edges:
- RowPtr:  10k * 4 bytes = 40 KB
- ColIdx:  100k * 4 bytes = 400 KB
- Values:  100k * 4 bytes = 400 KB
- Total:   ~840 KB
- Transfer: 840 KB / 16 GB/s = 0.05ms ✅ negligible

100k nodes, 10M edges:
- RowPtr:  100k * 4 bytes = 400 KB
- ColIdx:  10M * 4 bytes = 40 MB
- Values:  10M * 4 bytes = 40 MB
- Total:   ~80 MB
- Transfer: 80 MB / 16 GB/s = 5ms ⚠️ non-trivial but acceptable
```

---

## Phase 2: Integration with Engine

### 2.1 Routing Logic (CPU vs GPU)

**Auto-selection based on graph size**:

```go
// File: snapshot.go (modified)

func (s *Snapshot) Analyze() (*Results, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    if err := s.checkClosed(); err != nil {
        return nil, err
    }

    // Phase 2 cache check (unchanged)
    if s.cache != nil {
        // ... cache lookup ...
    }

    gv := s.graphView()
    nodeCount := gv.NodeCount()

    // GPU routing logic
    if s.config.gpuEnabled && s.config.gpu != nil && nodeCount >= s.config.gpuThreshold {
        return s.analyzeGPU(gv)
    }

    // CPU path (existing implementation)
    return s.analyzeCPU(gv)
}

func (s *Snapshot) analyzeGPU(gv analysis.GraphView) (*Results, error) {
    start := time.Now()

    // 1. Check if graph is already on GPU (warm cache)
    if s.config.gpu.HasGraph(s.version.ID) {
        // Incremental update: send only deltas
        return s.config.gpu.AnalyzeIncremental(s.version.ID, gv)
    }

    // 2. Cold start: upload full graph
    tensions, err := s.config.gpu.AnalyzeTensions(gv)
    if err != nil {
        // Fallback to CPU
        s.config.logger.Warn("GPU analysis failed, falling back to CPU", "error", err)
        return s.analyzeCPU(gv)
    }

    // 3. Post-process on CPU (curvature, concealment, etc.)
    return s.buildResults(tensions, gv, time.Since(start))
}
```

### 2.2 Builder Configuration

```go
// File: builder.go

type Builder struct {
    // ... existing fields ...

    // GPU Configuration
    gpuEnabled   bool
    gpuThreshold int     // min nodes to use GPU (default: 1000)
    gpuBackend   string  // "cuda" | "opencl" | "auto"
    gpuDevice    int     // GPU device ID (default: 0)
    gpu          *gpu.Accelerator
}

func (b *Builder) WithGPU(threshold int) *Builder {
    b.gpuEnabled = true
    b.gpuThreshold = threshold
    b.gpuBackend = "auto" // detect CUDA, fallback to OpenCL
    return b
}

func (b *Builder) WithGPUBackend(backend string, deviceID int) *Builder {
    b.gpuBackend = backend
    b.gpuDevice = deviceID
    return b
}

func (b *Builder) Build() (*Engine, error) {
    // ... existing validation ...

    if b.gpuEnabled {
        gpu, err := gpu.NewAccelerator(b.gpuBackend, b.gpuDevice)
        if err != nil {
            // GPU not available, disable silently
            b.gpuEnabled = false
            if b.logger != nil {
                b.logger.Warn("GPU initialization failed, using CPU", "error", err)
            }
        } else {
            b.gpu = gpu
            if b.logger != nil {
                b.logger.Info("GPU acceleration enabled", "backend", gpu.Backend(), "device", gpu.DeviceName())
            }
        }
    }

    return newEngine(b), nil
}
```

### 2.3 Hybrid Mode (Best of Both Worlds)

**Challenge**: Some operations are better on GPU, others on CPU.

**Strategy**:
- **GPU**: Tensor computation (JSD, KL), Laplacian eigenvalues
- **CPU**: Curvature (per-edge ops), Concealment (BFS traversal), Temporal (history lookups)

```go
func (s *Snapshot) analyzeHybrid(gv analysis.GraphView) (*Results, error) {
    // 1. GPU: Compute all tensions in parallel
    tensions, err := s.config.gpu.AnalyzeTensions(gv)
    if err != nil {
        return s.analyzeCPU(gv) // fallback
    }

    // 2. CPU: Curvature (if enabled)
    var edgeCurvatures map[[2]string]float64
    if s.config.curvatureAlpha > 0 {
        cc := analysis.NewCurvatureCalculator(s.config.curvatureAlpha)
        edgeCurvatures = cc.CalculateAll(gv) // CPU (sparse graph ops)
    }

    // 3. CPU: Concealment (if enabled)
    var concCalc *analysis.ConcealmentCalculator
    if s.config.concealmentLambda > 0 {
        tc := analysis.NewTensionCalculator(s.getDivergence())
        concCalc = analysis.NewConcealmentCalculator(s.config.concealmentLambda, tc)
    }

    // 4. GPU: Fiedler value (if temporal analysis enabled)
    var fiedlerValue float64
    if s.tensionHistory != nil {
        nodeIDs := make([]string, 0, gv.NodeCount())
        gv.ForEachNode(func(n *graph.NodeData) bool {
            nodeIDs = append(nodeIDs, n.ID)
            return true
        })
        fiedlerValue = s.config.gpu.FiedlerApprox(gv, nodeIDs) // GPU (dense matrix)
    }

    // 5. Merge results
    return s.buildResults(tensions, gv, edgeCurvatures, concCalc, fiedlerValue)
}
```

---

## Phase 3: Batch Processing

### 3.1 Multi-Snapshot Analysis

**Use Case**: Analyze 100 snapshots in parallel (e.g., hourly data for 4 days)

**Architecture**:
```go
type BatchRequest struct {
    Snapshots []*Snapshot
    Options   BatchOptions
}

type BatchOptions struct {
    MaxConcurrent int  // max snapshots in GPU memory (default: 10)
    Async         bool // return channel for streaming results
}

func (acc *Accelerator) AnalyzeBatch(req *BatchRequest) ([]*Results, error) {
    // 1. Upload all graphs to GPU (pipelined)
    deviceGraphs := make([]*C.DeviceGraph, len(req.Snapshots))
    for i, snap := range req.Snapshots {
        csr := SerializeToCSR(snap.graphView())
        deviceGraphs[i] = acc.uploadGraph(csr)
    }

    // 2. Launch batched kernel (all nodes across all graphs in parallel)
    allTensions := acc.computeBatchTensions(deviceGraphs)

    // 3. Download results
    results := make([]*Results, len(req.Snapshots))
    for i, tensions := range allTensions {
        results[i] = buildResults(tensions, req.Snapshots[i])
    }

    return results, nil
}
```

**Kernel: Batched Tensor Computation**:
```cuda
__global__ void compute_batch_jsd_tensions(
    const BatchedCSR* graphs,  // array of CSR graphs
    int numGraphs,
    double* tensions           // output: [graph0_tensions, graph1_tensions, ...]
) {
    // Each block processes one graph
    int graphIdx = blockIdx.x;
    if (graphIdx >= numGraphs) return;

    // Each thread processes one node
    int nodeIdx = threadIdx.x;
    const CSRGraph* g = &graphs[graphIdx];
    if (nodeIdx >= g->numNodes) return;

    // Compute tension for this node in this graph
    tensions[graphIdx * MAX_NODES + nodeIdx] = compute_node_jsd(g, nodeIdx);
}
```

**Performance**:
```
Sequential CPU (100 snapshots × 10k nodes each):
- Time: 100 × 277ms = 27.7s

Batched GPU:
- Upload: 100 × 0.05ms = 5ms
- Compute: 500ms (all in parallel)
- Download: 100 × 0.1ms = 10ms
- Total: 515ms
- Speedup: 53.8x ✅
```

---

## Phase 4: Advanced Optimizations

### 4.1 Persistent GPU Memory (Graph Caching)

**Problem**: Repeatedly uploading the same graph is wasteful.

**Solution**: Keep frequently-accessed graphs in GPU RAM.

```go
type GPUGraphCache struct {
    mu      sync.RWMutex
    graphs  map[uint64]*DeviceGraph  // versionID → GPU graph
    lru     *LRU                     // evict old graphs when full
    maxSize int64                    // max GPU memory (bytes)
}

func (c *GPUGraphCache) Get(versionID uint64) (*DeviceGraph, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    dg, ok := c.graphs[versionID]
    if ok {
        c.lru.Touch(versionID)
    }
    return dg, ok
}

func (c *GPUGraphCache) Put(versionID uint64, dg *DeviceGraph) {
    c.mu.Lock()
    defer c.mu.Unlock()

    // Evict LRU if necessary
    for c.currentSize+dg.Size > c.maxSize {
        evictID := c.lru.RemoveOldest()
        evicted := c.graphs[evictID]
        c.currentSize -= evicted.Size
        evicted.Free()
        delete(c.graphs, evictID)
    }

    c.graphs[versionID] = dg
    c.currentSize += dg.Size
    c.lru.Add(versionID)
}
```

**Memory Budget**:
```
NVIDIA RTX 3090: 24 GB VRAM
- Reserve 8 GB for tensors/kernels
- Cache budget: 16 GB

10k node graph: ~840 KB
16 GB / 840 KB = ~19,000 graphs in cache ✅

100k node graph: ~80 MB
16 GB / 80 MB = ~200 graphs in cache ✅
```

### 4.2 Incremental Updates

**Problem**: Graph changes slowly (e.g., 100 new edges per hour). Re-uploading full graph is wasteful.

**Solution**: Send only deltas.

```go
func (acc *Accelerator) UpdateGraphIncremental(
    versionID uint64,
    addedEdges []EdgeUpdate,
    removedEdges []EdgeUpdate,
) error {
    deviceGraph := acc.cache.Get(versionID)
    if deviceGraph == nil {
        return ErrGraphNotCached
    }

    // Upload edge deltas (small transfer)
    C.gpu_update_edges(
        acc.ctx,
        deviceGraph.handle,
        (*C.EdgeUpdate)(unsafe.Pointer(&addedEdges[0])),
        C.int(len(addedEdges)),
        (*C.EdgeUpdate)(unsafe.Pointer(&removedEdges[0])),
        C.int(len(removedEdges)),
    )

    return nil
}
```

**Performance**:
```
Full upload (10k nodes, 100k edges): 0.05ms
Incremental (100 new edges): 0.001ms
Speedup: 50x ✅
```

### 4.3 Multi-GPU Support

**Use Case**: Data center with 8× NVIDIA A100 GPUs.

**Strategy**: Partition graph across GPUs (spatial decomposition).

```go
type MultiGPU struct {
    devices []*Accelerator
    strategy PartitionStrategy
}

type PartitionStrategy int

const (
    PartitionByNodeRange PartitionStrategy = iota  // nodes 0-1k → GPU0, 1k-2k → GPU1, etc.
    PartitionByDegree                              // high-degree nodes → separate GPU
    PartitionByCommunity                           // Louvain communities → separate GPU
)

func (m *MultiGPU) AnalyzeDistributed(g *graph.ImmutableGraph) (map[string]float64, error) {
    // 1. Partition graph
    partitions := m.strategy.Partition(g, len(m.devices))

    // 2. Upload partitions to GPUs in parallel
    var wg sync.WaitGroup
    resultChans := make([]chan map[string]float64, len(m.devices))

    for i, partition := range partitions {
        wg.Add(1)
        resultChans[i] = make(chan map[string]float64, 1)

        go func(deviceID int, subgraph *graph.ImmutableGraph) {
            defer wg.Done()
            tensions, _ := m.devices[deviceID].AnalyzeTensions(subgraph)
            resultChans[deviceID] <- tensions
        }(i, partition)
    }

    // 3. Merge results
    wg.Wait()
    merged := make(map[string]float64, g.NodeCount())
    for _, ch := range resultChans {
        partial := <-ch
        for k, v := range partial {
            merged[k] = v
        }
    }

    return merged, nil
}
```

**Speedup**:
```
1 GPU (RTX 3090): 100k nodes in 50ms
8 GPUs (A100):    100k nodes in 6.25ms (8x speedup)
```

---

## Benchmarking & Validation

### Expected Performance (NVIDIA RTX 3090)

| Graph Size | CPU (Ryzen 5 3600) | GPU (RTX 3090) | Speedup |
|------------|-------------------|----------------|---------|
| 1k nodes   | 277 ms            | 5 ms           | 55x     |
| 10k nodes  | 2.8 s             | 15 ms          | 186x    |
| 100k nodes | 160 s (est)       | 80 ms          | 2000x   |
| 1M nodes   | 4500 s (est)      | 500 ms         | 9000x   |

**Batch Mode** (100 snapshots, 10k nodes each):
```
CPU Sequential:   100 × 2.8s = 280s
CPU Parallel:     2.8s (limited by 12 cores)
GPU Batch:        515ms
Speedup vs CPU:   543x ✅
```

### Validation Strategy

1. **Correctness**:
   - Run CPU and GPU analysis on same graph
   - Assert tensions match within ε = 1e-6 (floating-point tolerance)
   - Automated test: 1000 random graphs

2. **Performance**:
   - Benchmark CPU vs GPU for graphs: 100, 1k, 10k, 100k nodes
   - Profile memory transfer overhead
   - Measure batch processing throughput

3. **Reliability**:
   - Stress test: 1M analyses over 24 hours
   - Memory leak detection (CUDA memcheck)
   - GPU crash recovery (fallback to CPU)

---

## Implementation Roadmap

### Milestone 1: CUDA Foundation (2 weeks)
- [ ] CGO wrapper for CUDA runtime
- [ ] CSR serialization
- [ ] Basic JSD kernel
- [ ] CPU/GPU parity test
- **Deliverable**: GPU computes same tensions as CPU (1k nodes)

### Milestone 2: Engine Integration (1 week)
- [ ] Routing logic (CPU vs GPU)
- [ ] Builder.WithGPU() API
- [ ] Fallback mechanism
- [ ] Benchmarks
- **Deliverable**: Engine auto-selects GPU for large graphs

### Milestone 3: Batch Processing (1 week)
- [ ] Batched kernel
- [ ] Multi-snapshot upload
- [ ] Async API
- **Deliverable**: Analyze 100 snapshots in < 1s

### Milestone 4: Advanced Optimizations (2 weeks)
- [ ] GPU graph cache (LRU)
- [ ] Incremental updates
- [ ] Fiedler value (cuSOLVER)
- [ ] Multi-GPU support
- **Deliverable**: 1M nodes in < 1s

---

## Hardware Requirements

### Minimum (Development):
- **GPU**: NVIDIA GTX 1660 (6 GB VRAM, 1408 CUDA cores)
- **CPU**: 4 cores
- **RAM**: 16 GB
- **Storage**: NVMe SSD (fast graph loading)

### Recommended (Production):
- **GPU**: NVIDIA RTX 3090 (24 GB VRAM, 10496 CUDA cores)
- **CPU**: 16 cores (for CPU fallback + preprocessing)
- **RAM**: 64 GB
- **Storage**: NVMe RAID

### Enterprise (Extreme Scale):
- **GPU**: 8× NVIDIA A100 (40 GB each, 312 TFLOPS)
- **CPU**: 2× AMD EPYC 7763 (128 cores total)
- **RAM**: 512 GB DDR4
- **Storage**: NVMe RAID + 10 Gbps network (distributed graphs)

---

## Risks & Mitigations

### Risk 1: CGO Overhead
**Problem**: CGO calls are slow (~170ns per call).
**Mitigation**: Batch operations. Upload entire graph in one call, not edge-by-edge.

### Risk 2: Memory Transfer Bottleneck
**Problem**: PCIe bandwidth limits (16 GB/s).
**Mitigation**: Persistent GPU memory + incremental updates.

### Risk 3: GPU Not Available
**Problem**: User's machine has no NVIDIA GPU.
**Mitigation**: Auto-detect and fallback to CPU. GPU is optional.

### Risk 4: CUDA Complexity
**Problem**: CUDA programming is hard.
**Mitigation**: Start with cuBLAS/cuSPARSE (libraries), then optimize with custom kernels.

### Risk 5: Platform Lock-in
**Problem**: CUDA = NVIDIA only.
**Mitigation**: Abstraction layer. Add OpenCL backend for AMD/Intel.

---

## Cost-Benefit Analysis

### Development Cost
- **Time**: 6 weeks (1 engineer)
- **Hardware**: $2,000 (RTX 3090 for testing)
- **Total**: ~$20k (eng time + hardware)

### Benefit
- **Speedup**: 50-2000x for large graphs
- **New Use Cases Enabled**:
  - Real-time monitoring of 100k+ node networks
  - Batch analysis of historical data (years in hours)
  - Interactive exploration of million-node graphs

### ROI
For Argos (25k nodes): GPU is overkill (CPU + cache is enough).
For Black Hole Detection (10M+ nodes): GPU is **essential** (CPU would take hours).

**Recommendation**: Implement GPU as **optional plugin** for extreme-scale users.

---

## Conclusion

GPU acceleration can provide **50-2000x speedup** for ITT Engine on large graphs, but:
- **Not needed for most use cases** (< 10k nodes): CPU + cache is faster
- **Essential for extreme scale** (> 100k nodes): Only way to achieve real-time performance
- **Implementation is complex**: CGO, CUDA, memory management

**Next Steps**:
1. Validate demand: How many users need > 100k node graphs?
2. Proof of concept: Implement Milestone 1 (basic CUDA kernel)
3. Benchmark: Prove 50x+ speedup before full implementation
4. Optional: Offer as commercial plugin ($$$) to recoup dev cost

---

**Status**: Planning Complete — Ready for Implementation if Demand Exists
**Contact**: For GPU acceleration inquiries, open GitHub issue with use case details.
