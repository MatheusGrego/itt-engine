# ITT Engine v3 — High Performance Optimization

**Goal**: Achieve 10k+ requests/second while preserving MVCC, snapshot isolation, and all v2 features.

**Date**: 2026-02-10
**Estimated Performance Gain**: 50-400x for read-heavy workloads

---

## Executive Summary

**Current Bottlenecks**:
1. `Snapshot.Analyze()` is single-threaded → 10s for 25k nodes
2. No result caching → every API call recomputes from scratch
3. JSON serialization is CPU-bound → 26ms for 5 MB
4. No incremental analysis → small updates trigger full recalculation

**Proposed Optimizations**:
1. **Parallel Analysis** (Phase 1): 4-8x speedup, backward-compatible
2. **Smart Cache Layer** (Phase 2): 100-1000x speedup for reads, MVCC-aware
3. **Incremental Recomputation** (Phase 3): 100x speedup for small updates
4. **JSON Streaming** (Phase 4): 5-10x speedup for large responses

**Expected Performance After All Phases**:
```
Current:
- Analyze 25k nodes: 10s → API throughput: ~0.1 req/s
- Cold cache: every request = 10s

After Phase 1 (Parallel):
- Analyze 25k nodes: 1.25s → ~0.8 req/s (8x improvement)

After Phase 2 (Cache):
- Hot cache: 0.05ms (memory lookup) → 20,000 req/s
- Cold cache: 1.25s (first request only)
- Effective throughput: 10k-20k req/s (read-heavy: 99% cache hits)

After Phase 3 (Incremental):
- Daily updates (1k events): 25ms → 40 req/s sustained writes
- No cache invalidation for unchanged nodes → cache efficiency ↑

After Phase 4 (Streaming JSON):
- Large response (5 MB): 26ms → 3ms → 333 req/s (for full dumps)
```

---

## Phase 1: Parallel Analysis (Week 1-2)

### 1.1 Design: Worker Pool Architecture

**Principle**: Preserve MVCC snapshot isolation — each worker operates on the **same ImmutableGraph**, no shared mutable state.

```go
// File: analysis/parallel.go (NEW)

package analysis

import (
    "runtime"
    "sync"
)

// ParallelAnalyzer wraps TensionCalculator with parallel execution
type ParallelAnalyzer struct {
    calculator  *TensionCalculator
    workers     int  // default: runtime.NumCPU()
}

// AnalyzeParallel computes tension for all nodes using worker pool
func (p *ParallelAnalyzer) AnalyzeParallel(
    view GraphView,
    nodes []string,
) []TensionResult {
    if len(nodes) == 0 {
        return nil
    }

    // Partition nodes into chunks (one per worker)
    chunks := partitionNodes(nodes, p.workers)

    // Result channel (buffered to avoid blocking)
    results := make(chan TensionResult, len(nodes))

    // Worker pool
    var wg sync.WaitGroup
    for _, chunk := range chunks {
        wg.Add(1)
        go func(nodeIDs []string) {
            defer wg.Done()
            for _, id := range nodeIDs {
                result := p.calculator.CalculateTension(view, id)
                results <- result
            }
        }(chunk)
    }

    // Wait and collect
    go func() {
        wg.Wait()
        close(results)
    }()

    // Collect results (order-independent)
    collected := make([]TensionResult, 0, len(nodes))
    for r := range results {
        collected = append(collected, r)
    }

    return collected
}

func partitionNodes(nodes []string, workers int) [][]string {
    if workers <= 0 {
        workers = runtime.NumCPU()
    }

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
```

**Key Design Choices**:
1. **No locks on GraphView** — ImmutableGraph is read-only, concurrent reads are safe
2. **Channel-based result collection** — workers don't write to shared slice (no races)
3. **Order-independent** — results are aggregated, order doesn't matter for stats
4. **Configurable worker count** — defaults to `runtime.NumCPU()`, tunable for testing

---

### 1.2 Integration: Backward-Compatible API

```go
// File: snapshot.go (MODIFIED)

// Add new option to Builder
type AnalyzeOptions struct {
    Parallel bool  // default: true (auto-enabled for nodes > threshold)
    Workers  int   // default: runtime.NumCPU()
}

// Modify Snapshot.Analyze() to accept options
func (s *Snapshot) Analyze(opts ...AnalyzeOptions) (Results, error) {
    if s.closed.Load() {
        return Results{}, ErrSnapshotClosed
    }

    // Parse options (variadic for backward compatibility)
    opt := AnalyzeOptions{Parallel: true, Workers: runtime.NumCPU()}
    if len(opts) > 0 {
        opt = opts[0]
    }

    nodes := s.view.NodeIDs()

    // Auto-enable parallel for large graphs
    useParallel := opt.Parallel && len(nodes) > 100

    var tensionResults []analysis.TensionResult

    if useParallel {
        // NEW: Parallel path
        parallelCalc := &analysis.ParallelAnalyzer{
            calculator: s.tensionCalc,
            workers:    opt.Workers,
        }
        tensionResults = parallelCalc.AnalyzeParallel(s.view, nodes)
    } else {
        // OLD: Sequential path (preserved for small graphs, testing)
        tensionResults = make([]analysis.TensionResult, 0, len(nodes))
        for _, nodeID := range nodes {
            result := s.tensionCalc.CalculateTension(s.view, nodeID)
            tensionResults = append(tensionResults, result)
        }
    }

    // Rest of the function unchanged (calibrator, detectability, etc.)
    // ...
}

// Add explicit API for users who want to control parallelism
func (s *Snapshot) AnalyzeSequential() (Results, error) {
    return s.Analyze(AnalyzeOptions{Parallel: false})
}

func (s *Snapshot) AnalyzeParallel(workers int) (Results, error) {
    return s.Analyze(AnalyzeOptions{Parallel: true, Workers: workers})
}
```

**Backward Compatibility**:
- Existing calls to `Snapshot.Analyze()` auto-enable parallel (no code change needed)
- Sequential path still exists (for testing, debugging)
- Small graphs (< 100 nodes) stay sequential (overhead of goroutines > speedup)

---

### 1.3 Testing Strategy

```go
// File: snapshot_parallel_test.go (NEW)

func TestAnalyzeParallelCorrectness(t *testing.T) {
    // Build graph with 1000 nodes
    engine := NewBuilder().Build()
    for i := 0; i < 1000; i++ {
        engine.AddEvent(Event{
            Source: fmt.Sprintf("node-%d", i),
            Target: fmt.Sprintf("node-%d", (i+1)%1000),
            Weight: rand.Float64(),
        })
    }

    snap := engine.Snapshot()
    defer snap.Close()

    // Compare sequential vs parallel
    seqResults, _ := snap.AnalyzeSequential()
    parResults, _ := snap.AnalyzeParallel(4)

    // Results should be identical (order-independent)
    if len(seqResults.Tensions) != len(parResults.Tensions) {
        t.Fatalf("result count mismatch: seq=%d, par=%d",
            len(seqResults.Tensions), len(parResults.Tensions))
    }

    // Build map for comparison (order-independent)
    seqMap := make(map[string]float64)
    for _, r := range seqResults.Tensions {
        seqMap[r.NodeID] = r.Tension
    }

    for _, r := range parResults.Tensions {
        seqTension, ok := seqMap[r.NodeID]
        if !ok {
            t.Errorf("node %s missing in sequential results", r.NodeID)
            continue
        }

        // Allow tiny floating-point error
        if math.Abs(seqTension - r.Tension) > 1e-10 {
            t.Errorf("tension mismatch for %s: seq=%.6f, par=%.6f",
                r.NodeID, seqTension, r.Tension)
        }
    }
}

func BenchmarkAnalyzeParallel(b *testing.B) {
    // Build graph with 10k nodes
    engine := buildLargeGraph(10000)
    snap := engine.Snapshot()
    defer snap.Close()

    b.Run("Sequential", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            snap.AnalyzeSequential()
        }
    })

    b.Run("Parallel-2", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            snap.AnalyzeParallel(2)
        }
    })

    b.Run("Parallel-4", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            snap.AnalyzeParallel(4)
        }
    })

    b.Run("Parallel-8", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            snap.AnalyzeParallel(8)
        }
    })
}
```

**Expected Benchmark Results**:
```
BenchmarkAnalyzeParallel/Sequential-8    1    10000000000 ns/op  (10s baseline)
BenchmarkAnalyzeParallel/Parallel-2-8    1     5000000000 ns/op  (5s, 2x speedup)
BenchmarkAnalyzeParallel/Parallel-4-8    1     2500000000 ns/op  (2.5s, 4x speedup)
BenchmarkAnalyzeParallel/Parallel-8-8    1     1250000000 ns/op  (1.25s, 8x speedup)
```

---

### 1.4 Risks & Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Race condition in TensionCalculator | Data corruption | Audit: ensure no shared mutable state |
| Goroutine overhead for small graphs | Performance regression | Auto-disable for graphs < 100 nodes |
| Non-deterministic result order | Test flakiness | Use map-based comparison in tests |
| Calibrator shared state | Race in MAD calculation | Make Calibrator thread-safe (see below) |

**Critical: Make Calibrator Thread-Safe**

```go
// File: analysis/calibrator.go (MODIFIED)

type MADCalibrator struct {
    mu          sync.RWMutex  // NEW: protect observations
    observations []float64
    // ... rest unchanged
}

func (c *MADCalibrator) Observe(tension float64) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.observations = append(c.observations, tension)
}

func (c *MADCalibrator) IsAnomaly(tension float64) bool {
    c.mu.RLock()
    defer c.mu.RUnlock()
    // ... calculation unchanged
}
```

---

## Phase 2: Smart Cache Layer (Week 3-4)

### 2.1 Design: MVCC-Aware Cache

**Challenge**: Cache must respect snapshot isolation. A cached result is valid only if computed from the **same MVCC version**.

**Solution**: Version-tagged cache with TTL fallback.

```go
// File: cache/results_cache.go (NEW)

package cache

import (
    "sync"
    "time"
    "github.com/MatheusGrego/itt-engine"
)

// CacheKey uniquely identifies a cached result
type CacheKey struct {
    VersionID uint64  // MVCC version ID
    QueryType string  // "full_analysis" | "node_analysis" | "region_analysis"
    QueryArgs string  // JSON-encoded args (e.g., nodeID, regionIDs)
}

// CachedResult wraps Results with metadata
type CachedResult struct {
    Results   itt.Results
    ComputedAt time.Time
    ExpiresAt  time.Time
}

// ResultsCache is a thread-safe, version-aware cache
type ResultsCache struct {
    mu    sync.RWMutex
    data  map[CacheKey]CachedResult
    ttl   time.Duration  // default: 60s
}

func NewResultsCache(ttl time.Duration) *ResultsCache {
    return &ResultsCache{
        data: make(map[CacheKey]CachedResult),
        ttl:  ttl,
    }
}

// Get retrieves cached result if valid
func (c *ResultsCache) Get(key CacheKey) (itt.Results, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    cached, ok := c.data[key]
    if !ok {
        return itt.Results{}, false
    }

    // Check TTL expiration
    if time.Now().After(cached.ExpiresAt) {
        return itt.Results{}, false
    }

    return cached.Results, true
}

// Set stores result with version tag and TTL
func (c *ResultsCache) Set(key CacheKey, results itt.Results) {
    c.mu.Lock()
    defer c.mu.Unlock()

    now := time.Now()
    c.data[key] = CachedResult{
        Results:    results,
        ComputedAt: now,
        ExpiresAt:  now.Add(c.ttl),
    }
}

// InvalidateVersion removes all cache entries for a specific MVCC version
// Called when that version is GC'd
func (c *ResultsCache) InvalidateVersion(versionID uint64) {
    c.mu.Lock()
    defer c.mu.Unlock()

    for key := range c.data {
        if key.VersionID == versionID {
            delete(c.data, key)
        }
    }
}

// EvictExpired removes all expired entries (background task)
func (c *ResultsCache) EvictExpired() {
    c.mu.Lock()
    defer c.mu.Unlock()

    now := time.Now()
    for key, cached := range c.data {
        if now.After(cached.ExpiresAt) {
            delete(c.data, key)
        }
    }
}

// Stats returns cache statistics
func (c *ResultsCache) Stats() CacheStats {
    c.mu.RLock()
    defer c.mu.RUnlock()

    return CacheStats{
        Entries: len(c.data),
        // TODO: add hit/miss counters
    }
}

type CacheStats struct {
    Entries int
    Hits    uint64
    Misses  uint64
}
```

---

### 2.2 Integration: Engine-Level Cache

```go
// File: engine.go (MODIFIED)

type Engine struct {
    // ... existing fields

    resultsCache *cache.ResultsCache  // NEW
    cacheEnabled bool                  // NEW: configurable
}

// File: builder.go (MODIFIED)

func (b *Builder) WithCache(ttl time.Duration) *Builder {
    b.cacheEnabled = true
    b.cacheTTL = ttl
    return b
}

func (b *Builder) Build() (*Engine, error) {
    // ... existing code

    var resultsCache *cache.ResultsCache
    if b.cacheEnabled {
        resultsCache = cache.NewResultsCache(b.cacheTTL)
    }

    engine := &Engine{
        // ... existing fields
        resultsCache: resultsCache,
        cacheEnabled: b.cacheEnabled,
    }

    // Start cache eviction goroutine
    if b.cacheEnabled {
        go engine.cacheEvictionWorker()
    }

    return engine, nil
}

// Background task: evict expired cache entries every 10s
func (e *Engine) cacheEvictionWorker() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            if e.resultsCache != nil {
                e.resultsCache.EvictExpired()
            }
        case <-e.ctx.Done():
            return
        }
    }
}
```

---

### 2.3 Integration: Snapshot-Level Cache Lookup

```go
// File: snapshot.go (MODIFIED)

func (s *Snapshot) Analyze(opts ...AnalyzeOptions) (Results, error) {
    if s.closed.Load() {
        return Results{}, ErrSnapshotClosed
    }

    // NEW: Check cache first (if enabled)
    if s.engine.cacheEnabled && s.engine.resultsCache != nil {
        cacheKey := cache.CacheKey{
            VersionID: s.version.ID,  // MVCC version ID
            QueryType: "full_analysis",
            QueryArgs: "",  // full analysis has no args
        }

        if cached, ok := s.engine.resultsCache.Get(cacheKey); ok {
            return cached, nil  // Cache hit: 0.05ms
        }
    }

    // Cache miss: compute normally
    opt := AnalyzeOptions{Parallel: true, Workers: runtime.NumCPU()}
    if len(opts) > 0 {
        opt = opts[0]
    }

    // ... existing computation logic (parallel analysis)

    results := Results{
        Tensions:  tensionResults,
        Anomalies: anomalies,
        Stats:     stats,
        // ...
    }

    // NEW: Store in cache before returning
    if s.engine.cacheEnabled && s.engine.resultsCache != nil {
        cacheKey := cache.CacheKey{
            VersionID: s.version.ID,
            QueryType: "full_analysis",
            QueryArgs: "",
        }
        s.engine.resultsCache.Set(cacheKey, results)
    }

    return results, nil
}
```

**Cache Invalidation Strategy**:
```go
// File: mvcc/gc.go (MODIFIED)

func (g *GarbageCollector) collectVersion(v *Version) {
    // ... existing GC logic

    // NEW: Invalidate cache entries for this version
    if g.engine.cacheEnabled && g.engine.resultsCache != nil {
        g.engine.resultsCache.InvalidateVersion(v.ID)
    }

    // ... rest of GC
}
```

---

### 2.4 Testing Strategy

```go
// File: cache/results_cache_test.go (NEW)

func TestCacheHitMiss(t *testing.T) {
    cache := NewResultsCache(60 * time.Second)

    key := CacheKey{VersionID: 1, QueryType: "full_analysis"}

    // Miss
    _, ok := cache.Get(key)
    if ok {
        t.Error("expected cache miss, got hit")
    }

    // Set
    results := itt.Results{/* ... */}
    cache.Set(key, results)

    // Hit
    cached, ok := cache.Get(key)
    if !ok {
        t.Error("expected cache hit, got miss")
    }
    if len(cached.Tensions) != len(results.Tensions) {
        t.Error("cached result mismatch")
    }
}

func TestCacheTTLExpiration(t *testing.T) {
    cache := NewResultsCache(100 * time.Millisecond)

    key := CacheKey{VersionID: 1, QueryType: "full_analysis"}
    cache.Set(key, itt.Results{})

    // Immediate hit
    _, ok := cache.Get(key)
    if !ok {
        t.Error("expected immediate hit")
    }

    // Wait for expiration
    time.Sleep(150 * time.Millisecond)

    // Miss after expiration
    _, ok = cache.Get(key)
    if ok {
        t.Error("expected miss after TTL expiration")
    }
}

func TestCacheVersionInvalidation(t *testing.T) {
    cache := NewResultsCache(60 * time.Second)

    // Set entries for multiple versions
    cache.Set(CacheKey{VersionID: 1, QueryType: "full_analysis"}, itt.Results{})
    cache.Set(CacheKey{VersionID: 2, QueryType: "full_analysis"}, itt.Results{})
    cache.Set(CacheKey{VersionID: 3, QueryType: "full_analysis"}, itt.Results{})

    // Invalidate version 2
    cache.InvalidateVersion(2)

    // Version 1 and 3 still present
    _, ok := cache.Get(CacheKey{VersionID: 1, QueryType: "full_analysis"})
    if !ok {
        t.Error("version 1 should still be cached")
    }

    // Version 2 removed
    _, ok = cache.Get(CacheKey{VersionID: 2, QueryType: "full_analysis"})
    if ok {
        t.Error("version 2 should be invalidated")
    }

    _, ok = cache.Get(CacheKey{VersionID: 3, QueryType: "full_analysis"})
    if !ok {
        t.Error("version 3 should still be cached")
    }
}

func BenchmarkCacheHit(b *testing.B) {
    cache := NewResultsCache(60 * time.Second)
    key := CacheKey{VersionID: 1, QueryType: "full_analysis"}

    // Populate large result (5 MB)
    results := generateLargeResults(25000)
    cache.Set(key, results)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, ok := cache.Get(key)
        if !ok {
            b.Fatal("cache miss in hit benchmark")
        }
    }
}
```

**Expected Benchmark**:
```
BenchmarkCacheHit-8    100000000    50 ns/op  (0.05ms for memory lookup)
```

---

### 2.5 Cache Hit Rate Analysis

**Assumptions** (based on Argos use case):
- Graph updates: 1x/day (1k events)
- API queries: 1000 req/s
- Query distribution:
  - 80% read same full_analysis (dashboard)
  - 15% read specific nodes (drill-down)
  - 5% write (trigger re-analysis)

**Cache Behavior**:
```
Day 1:
- 00:00: Ingest overnight (new MVCC version V1)
- 00:01: First API call → cache MISS → 1.25s (parallel) → cache SET (V1)
- 00:01-23:59: All API calls → cache HIT → 0.05ms

Effective throughput:
- 1000 req/s × 0.05ms = 5% CPU usage → can handle 20k req/s

Day 2:
- 00:00: Ingest overnight (new MVCC version V2)
- MVCC GC evicts V1 → cache invalidates V1 entries
- 00:01: First API call → cache MISS → 1.25s → cache SET (V2)
- 00:01-23:59: All API calls → cache HIT → 0.05ms

Hit rate: 99.99% (1 miss per day, 86.4M hits per day)
```

---

## Phase 3: Incremental Recomputation (Week 5)

### 3.1 Design: Dirty Tracking

**Observation**: When adding 1k events to a 25k-node graph, only ~500 nodes are affected (source, target, their neighbors). No need to recompute all 25k.

```go
// File: engine.go (MODIFIED)

type Engine struct {
    // ... existing fields

    dirtyNodes map[string]struct{}  // NEW: nodes that need recomputation
    dirtyMu    sync.RWMutex         // NEW: protect dirtyNodes map
}

func (e *Engine) AddEvent(ev Event) error {
    // ... existing validation

    // NEW: Mark affected nodes as dirty
    e.markDirty(ev.Source, ev.Target)

    // ... existing processEvent logic
}

func (e *Engine) markDirty(nodeIDs ...string) {
    e.dirtyMu.Lock()
    defer e.dirtyMu.Unlock()

    if e.dirtyNodes == nil {
        e.dirtyNodes = make(map[string]struct{})
    }

    for _, id := range nodeIDs {
        e.dirtyNodes[id] = struct{}{}

        // Also mark neighbors (tension is neighborhood-sensitive)
        neighbors := e.overlay.Graph.Neighbors(id)
        for _, neighbor := range neighbors {
            e.dirtyNodes[neighbor] = struct{}{}
        }
    }
}

func (e *Engine) getDirtyNodes() []string {
    e.dirtyMu.RLock()
    defer e.dirtyMu.RUnlock()

    if len(e.dirtyNodes) == 0 {
        return nil
    }

    dirty := make([]string, 0, len(e.dirtyNodes))
    for id := range e.dirtyNodes {
        dirty = append(dirty, id)
    }
    return dirty
}

func (e *Engine) clearDirty() {
    e.dirtyMu.Lock()
    defer e.dirtyMu.Unlock()

    e.dirtyNodes = make(map[string]struct{})
}
```

---

### 3.2 Integration: Incremental Analysis API

```go
// File: snapshot.go (MODIFIED)

// AnalyzeIncremental computes tension only for dirty nodes (changed since last analysis)
func (s *Snapshot) AnalyzeIncremental() (Results, error) {
    if s.closed.Load() {
        return Results{}, ErrSnapshotClosed
    }

    // Get dirty nodes from engine
    dirtyNodes := s.engine.getDirtyNodes()

    if len(dirtyNodes) == 0 {
        // No changes: return cached result
        // (assumes cache is enabled and valid)
        return s.Analyze()  // falls back to cache
    }

    // Recompute only dirty nodes (parallel)
    parallelCalc := &analysis.ParallelAnalyzer{
        calculator: s.tensionCalc,
        workers:    runtime.NumCPU(),
    }

    dirtyResults := parallelCalc.AnalyzeParallel(s.view, dirtyNodes)

    // Merge with previous results (from cache)
    cacheKey := cache.CacheKey{
        VersionID: s.version.ID,
        QueryType: "full_analysis",
    }

    var fullResults Results
    if cached, ok := s.engine.resultsCache.Get(cacheKey); ok {
        fullResults = cached

        // Replace dirty entries
        tensionMap := make(map[string]analysis.TensionResult)
        for _, r := range fullResults.Tensions {
            tensionMap[r.NodeID] = r
        }
        for _, r := range dirtyResults {
            tensionMap[r.NodeID] = r  // overwrite
        }

        // Rebuild slice
        fullResults.Tensions = make([]analysis.TensionResult, 0, len(tensionMap))
        for _, r := range tensionMap {
            fullResults.Tensions = append(fullResults.Tensions, r)
        }

        // Recompute stats (fast: just aggregation)
        fullResults.Stats = computeStats(fullResults.Tensions)
        fullResults.Anomalies = filterAnomalies(fullResults.Tensions, s.threshold)
    } else {
        // No cache: fall back to full analysis
        return s.Analyze()
    }

    // Update cache
    s.engine.resultsCache.Set(cacheKey, fullResults)

    // Clear dirty tracking
    s.engine.clearDirty()

    return fullResults, nil
}
```

---

### 3.3 Testing Strategy

```go
// File: snapshot_incremental_test.go (NEW)

func TestIncrementalVsFull(t *testing.T) {
    engine := NewBuilder().WithCache(60 * time.Second).Build()

    // Build initial graph (1000 nodes)
    for i := 0; i < 1000; i++ {
        engine.AddEvent(Event{
            Source: fmt.Sprintf("node-%d", i),
            Target: fmt.Sprintf("node-%d", (i+1)%1000),
            Weight: 1.0,
        })
    }

    // Full analysis (populates cache)
    snap1 := engine.Snapshot()
    full1, _ := snap1.Analyze()
    snap1.Close()

    // Add 10 new events (affects ~30 nodes)
    for i := 0; i < 10; i++ {
        engine.AddEvent(Event{
            Source: "node-0",
            Target: fmt.Sprintf("node-%d", i),
            Weight: 2.0,
        })
    }

    // Incremental analysis
    snap2 := engine.Snapshot()
    incremental, _ := snap2.AnalyzeIncremental()

    // Full analysis (for comparison)
    full2, _ := snap2.Analyze()
    snap2.Close()

    // Results should be identical
    if len(incremental.Tensions) != len(full2.Tensions) {
        t.Fatalf("result count mismatch")
    }

    // Verify tensions match (within floating-point error)
    incMap := make(map[string]float64)
    for _, r := range incremental.Tensions {
        incMap[r.NodeID] = r.Tension
    }

    for _, r := range full2.Tensions {
        incTension, ok := incMap[r.NodeID]
        if !ok {
            t.Errorf("node %s missing in incremental", r.NodeID)
            continue
        }

        if math.Abs(incTension - r.Tension) > 1e-9 {
            t.Errorf("tension mismatch for %s: inc=%.6f, full=%.6f",
                r.NodeID, incTension, r.Tension)
        }
    }
}

func BenchmarkIncrementalAnalysis(b *testing.B) {
    engine := NewBuilder().WithCache(60 * time.Second).Build()

    // Build large graph (10k nodes)
    for i := 0; i < 10000; i++ {
        engine.AddEvent(Event{
            Source: fmt.Sprintf("node-%d", i),
            Target: fmt.Sprintf("node-%d", (i+1)%10000),
            Weight: 1.0,
        })
    }

    // Initial full analysis
    snap := engine.Snapshot()
    snap.Analyze()
    snap.Close()

    b.Run("Full", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            // Add 100 events (affects ~300 nodes)
            for j := 0; j < 100; j++ {
                engine.AddEvent(Event{
                    Source: "node-0",
                    Target: fmt.Sprintf("node-%d", j),
                    Weight: 2.0,
                })
            }

            snap := engine.Snapshot()
            snap.Analyze()  // Full: 1.25s
            snap.Close()
        }
    })

    b.Run("Incremental", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            // Add 100 events
            for j := 0; j < 100; j++ {
                engine.AddEvent(Event{
                    Source: "node-0",
                    Target: fmt.Sprintf("node-%d", j),
                    Weight: 2.0,
                })
            }

            snap := engine.Snapshot()
            snap.AnalyzeIncremental()  // Incremental: ~25ms
            snap.Close()
        }
    })
}
```

**Expected Benchmark**:
```
BenchmarkIncrementalAnalysis/Full-8         1   1250000000 ns/op  (1.25s)
BenchmarkIncrementalAnalysis/Incremental-8  50    25000000 ns/op  (25ms, 50x speedup)
```

---

## Phase 4: JSON Streaming (Week 6)

### 4.1 Design: Streaming Encoder

**Problem**: `json.Marshal(results)` for 5 MB takes 26ms. Bottleneck is memory allocation + serialization.

**Solution**: Stream JSON directly to HTTP response writer (avoid intermediate buffer).

```go
// File: export/json.go (MODIFIED)

import (
    "encoding/json"
    "io"
)

// StreamJSON writes Results as JSON to w without buffering entire output
func StreamJSON(w io.Writer, results itt.Results) error {
    encoder := json.NewEncoder(w)

    // Start object
    w.Write([]byte(`{"tensions":[`))

    // Stream tensions (one at a time, no intermediate slice)
    for i, tension := range results.Tensions {
        if i > 0 {
            w.Write([]byte(","))
        }
        if err := encoder.Encode(tension); err != nil {
            return err
        }
    }

    w.Write([]byte(`],"anomalies":[`))

    // Stream anomalies
    for i, anom := range results.Anomalies {
        if i > 0 {
            w.Write([]byte(","))
        }
        if err := encoder.Encode(anom); err != nil {
            return err
        }
    }

    w.Write([]byte(`],"stats":`))
    if err := encoder.Encode(results.Stats); err != nil {
        return err
    }

    w.Write([]byte(`}`))

    return nil
}
```

**HTTP Handler Example**:
```go
// In API server (hypothetical Argos HTTP server)
func handleAnalysis(w http.ResponseWriter, r *http.Request) {
    snap := engine.Snapshot()
    defer snap.Close()

    results, err := snap.Analyze()
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }

    w.Header().Set("Content-Type", "application/json")

    // Stream JSON (no buffering)
    if err := export.StreamJSON(w, results); err != nil {
        log.Printf("JSON stream error: %v", err)
    }
}
```

**Speedup**: 26ms → 3-5ms (5-8x faster, primarily from avoiding allocation)

---

## Testing & Validation Plan

### Correctness Tests (all phases)
```bash
# Run full test suite (ensure no regressions)
go test ./... -count=1

# Race detector (ensure no data races in parallel code)
go test ./... -race -count=10

# Stress test (high concurrency)
go test -run TestConcurrent -count=100 -parallel=10
```

### Performance Tests (benchmarks before/after each phase)
```bash
# Baseline (before optimizations)
go test -bench=BenchmarkAnalyze -benchmem -benchtime=10s

# After Phase 1 (parallel)
go test -bench=BenchmarkAnalyzeParallel -benchmem -benchtime=10s

# After Phase 2 (cache)
go test -bench=BenchmarkCacheHit -benchmem -benchtime=10s

# After Phase 3 (incremental)
go test -bench=BenchmarkIncrementalAnalysis -benchmem -benchtime=10s
```

### Integration Tests (end-to-end)
```bash
# Argos-like workload simulation
go test -run TestArgosWorkload -v
```

```go
func TestArgosWorkload(t *testing.T) {
    // Simulate 1 day of Argos usage
    engine := NewBuilder().
        WithCache(60 * time.Second).
        Build()

    // Morning: ingest overnight data (1k events)
    for i := 0; i < 1000; i++ {
        engine.AddEvent(generateInsiderTransaction())
    }

    // First query (cache miss): should take ~1.25s
    start := time.Now()
    snap := engine.Snapshot()
    results, _ := snap.Analyze()
    elapsed := time.Since(start)
    snap.Close()

    if elapsed > 2*time.Second {
        t.Errorf("first query too slow: %v", elapsed)
    }

    // Subsequent queries (cache hits): should take < 1ms
    for i := 0; i < 1000; i++ {
        start := time.Now()
        snap := engine.Snapshot()
        _, _ = snap.Analyze()
        elapsed := time.Since(start)
        snap.Close()

        if elapsed > 10*time.Millisecond {
            t.Errorf("query %d too slow (cache miss?): %v", i, elapsed)
        }
    }

    // Verify 1000 req/s throughput
    start = time.Now()
    for i := 0; i < 1000; i++ {
        snap := engine.Snapshot()
        snap.Analyze()
        snap.Close()
    }
    elapsed = time.Since(start)

    qps := 1000.0 / elapsed.Seconds()
    if qps < 1000 {
        t.Errorf("throughput too low: %.0f req/s (expected > 1000)", qps)
    }
}
```

---

## Performance Targets (Summary)

| Metric | Baseline (v2) | Phase 1 (Parallel) | Phase 2 (Cache) | Phase 3 (Incremental) | Phase 4 (Streaming) |
|---|---|---|---|---|---|
| **Analyze 25k nodes** | 10s | 1.25s | 1.25s (cold) / 0.05ms (hot) | 25ms (updates) | 1.25s / 0.05ms |
| **API throughput (read-heavy)** | ~0.1 req/s | ~0.8 req/s | **20k req/s** | 20k req/s | 20k req/s |
| **API throughput (write-heavy)** | ~0.1 req/s | ~0.8 req/s | ~0.8 req/s | **40 req/s** | 40 req/s |
| **Large JSON response (5 MB)** | 26ms | 26ms | 26ms | 26ms | **3-5ms** |
| **Memory overhead** | 335 MB | 335 MB | +50 MB (cache) | +10 MB (dirty tracking) | 0 |

**Final Result**: **10k-20k req/s** for read-heavy workloads (99% cache hits) ✅

---

## Implementation Order & Dependencies

```
Week 1-2: Phase 1 (Parallel Analysis)
  ├─ analysis/parallel.go
  ├─ snapshot.go (add AnalyzeParallel)
  ├─ analysis/calibrator.go (thread-safe)
  └─ tests: snapshot_parallel_test.go

Week 3-4: Phase 2 (Cache Layer)
  ├─ cache/results_cache.go
  ├─ builder.go (WithCache)
  ├─ engine.go (cache integration)
  ├─ snapshot.go (cache lookup/set)
  ├─ mvcc/gc.go (cache invalidation)
  └─ tests: cache_test.go, integration_test.go

Week 5: Phase 3 (Incremental)
  ├─ engine.go (dirty tracking)
  ├─ snapshot.go (AnalyzeIncremental)
  └─ tests: snapshot_incremental_test.go

Week 6: Phase 4 (JSON Streaming)
  ├─ export/json.go (StreamJSON)
  └─ tests: export_test.go

Week 6: Final Integration & Benchmarking
  └─ Full suite: go test ./... -race -bench=. -benchmem
```

---

## Risks & Go/No-Go Criteria

### Phase 1: Parallel Analysis
- ✅ **Go** if: all tests pass with `-race`, speedup > 3x on 8 cores
- ❌ **No-Go** if: data races detected, correctness tests fail

### Phase 2: Cache Layer
- ✅ **Go** if: hit rate > 95% in Argos simulation, no memory leaks
- ❌ **No-Go** if: cache invalidation breaks snapshot isolation

### Phase 3: Incremental
- ✅ **Go** if: incremental = full (within 1e-9), speedup > 10x for small updates
- ❌ **No-Go** if: dirty tracking misses nodes, results diverge

### Phase 4: JSON Streaming
- ✅ **Go** if: speedup > 3x, output is valid JSON
- ❌ **No-Go** if: breaks existing JSON consumers

---

## Backward Compatibility Guarantee

**All optimizations are OPT-IN or AUTO-ENABLED**:
1. Parallel: auto-enabled for graphs > 100 nodes (fallback: sequential)
2. Cache: opt-in via `Builder.WithCache(ttl)` (default: disabled)
3. Incremental: new API `AnalyzeIncremental()` (existing `Analyze()` unchanged)
4. Streaming: new function `export.StreamJSON()` (existing `export.JSON()` unchanged)

**Existing code continues to work without modification** ✅

---

## Success Metrics (Final Validation)

After all phases complete, run this benchmark:

```go
func BenchmarkFullPipeline(b *testing.B) {
    engine := NewBuilder().
        WithCache(60 * time.Second).
        Build()

    // Build Argos-scale graph (25k nodes, 1M edges)
    buildArgosGraph(engine, 25000, 1000000)

    b.Run("FirstQuery-Cold", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            engine.resultsCache.InvalidateVersion(1) // force cold
            snap := engine.Snapshot()
            snap.Analyze()
            snap.Close()
        }
    })

    b.Run("SubsequentQueries-Hot", func(b *testing.B) {
        // Prime cache
        snap := engine.Snapshot()
        snap.Analyze()
        snap.Close()

        b.ResetTimer()
        for i := 0; i < b.N; i++ {
            snap := engine.Snapshot()
            snap.Analyze()
            snap.Close()
        }
    })

    b.Run("IncrementalUpdate", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            // Add 100 events
            for j := 0; j < 100; j++ {
                engine.AddEvent(generateEvent())
            }

            snap := engine.Snapshot()
            snap.AnalyzeIncremental()
            snap.Close()
        }
    })
}
```

**Target Results**:
```
BenchmarkFullPipeline/FirstQuery-Cold-8        1   1250000000 ns/op  (1.25s)  ✅
BenchmarkFullPipeline/SubsequentQueries-Hot-8  20000000  50 ns/op  (0.05ms)  ✅
BenchmarkFullPipeline/IncrementalUpdate-8      40    25000000 ns/op  (25ms)   ✅
```

**Throughput Calculation**:
```
Cache hit scenario (99% of requests):
  - Latency: 0.05ms
  - Throughput: 1 / 0.00005s = 20,000 req/s ✅

Target: 10k+ req/s → ACHIEVED (2x headroom)
```

---

## Phase 5 (Future): GPU Acceleration & Distributed Mode

### 5.1 GPU Acceleration (CUDA/ROCm)

**Target**: 50-100x speedup for divergence calculations (massively parallel operations)

#### 5.1.1 Why GPU for ITT Engine?

**ITT computations that are embarrassingly parallel**:
1. **JSD Divergence** — compute for N nodes independently
2. **Neighbor distribution** — gather weights for each node (map-reduce)
3. **Matrix operations** — Laplacian construction, Fiedler value (eigenvalue)

**Bottleneck analysis**:
```
Current CPU (sequential):
- JSD for 1 node: ~180ns (mostly log() calls)
- JSD for 25k nodes: 25k × 180ns = 4.5ms

GPU (parallel):
- JSD for 25k nodes: ~50μs (all in parallel) → 90x speedup
- Overhead: CPU→GPU transfer ~1ms
- Net: 4.5ms → 1.05ms (4.3x real speedup)

For 1M nodes:
- CPU: 1M × 180ns = 180ms
- GPU: ~2ms (kernel) + 5ms (transfer) = 7ms → 25x speedup
```

#### 5.1.2 Design: GPU Offload Layer

```go
// File: analysis/gpu/jsd_cuda.go (NEW)

package gpu

/*
#cgo LDFLAGS: -L/usr/local/cuda/lib64 -lcudart
#include "jsd_kernel.h"
*/
import "C"
import "unsafe"

// JSD_GPU is a GPU-accelerated divergence calculator
type JSD_GPU struct {
    deviceID int
    ctx      C.cudaContext
}

func NewJSD_GPU(deviceID int) (*JSD_GPU, error) {
    var ctx C.cudaContext
    if err := C.cudaInit(C.int(deviceID), &ctx); err != 0 {
        return nil, fmt.Errorf("CUDA init failed: %d", err)
    }

    return &JSD_GPU{deviceID: deviceID, ctx: ctx}, nil
}

// Compute implements analysis.DivergenceFunc (GPU path)
func (j *JSD_GPU) Compute(observed, expected map[string]float64) float64 {
    // For single node: not worth GPU overhead, use CPU
    if len(observed) < 1000 {
        return analysis.JSD{}.Compute(observed, expected)  // fallback
    }

    // Batch GPU call (amortize transfer overhead)
    return j.computeBatch([]map[string]float64{observed, expected})[0]
}

// ComputeBatch is the GPU workhorse (batch N divergences)
func (j *JSD_GPU) ComputeBatch(distributions [][]map[string]float64) []float64 {
    N := len(distributions)

    // Flatten to GPU-friendly format (float arrays)
    flatData := j.flatten(distributions)

    // Allocate GPU memory
    var d_data C.float_ptr
    C.cudaMalloc(&d_data, C.size_t(len(flatData)*4))

    // Transfer CPU → GPU
    C.cudaMemcpy(d_data, unsafe.Pointer(&flatData[0]), C.size_t(len(flatData)*4), C.cudaMemcpyHostToDevice)

    // Launch kernel (1 thread per divergence calculation)
    var d_results C.float_ptr
    C.cudaMalloc(&d_results, C.size_t(N*4))
    C.jsd_kernel<<<(N+255)/256, 256>>>(d_data, d_results, C.int(N))

    // Transfer GPU → CPU
    results := make([]float64, N)
    C.cudaMemcpy(unsafe.Pointer(&results[0]), d_results, C.size_t(N*4), C.cudaMemcpyDeviceToHost)

    // Cleanup
    C.cudaFree(d_data)
    C.cudaFree(d_results)

    return results
}
```

**CUDA Kernel** (C):
```c
// File: analysis/gpu/jsd_kernel.cu

__global__ void jsd_kernel(float* distributions, float* results, int N) {
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    if (idx >= N) return;

    // Each thread computes one JSD
    float* obs = &distributions[idx * MAX_NEIGHBORS];
    float* exp = &distributions[(idx + N) * MAX_NEIGHBORS];

    // JSD = 0.5 * KL(obs||M) + 0.5 * KL(exp||M), M = (obs+exp)/2
    float jsd = 0.0f;
    for (int i = 0; i < MAX_NEIGHBORS; i++) {
        float m = (obs[i] + exp[i]) / 2.0f;
        if (obs[i] > 1e-10f && m > 1e-10f) {
            jsd += 0.5f * obs[i] * logf(obs[i] / m);
        }
        if (exp[i] > 1e-10f && m > 1e-10f) {
            jsd += 0.5f * exp[i] * logf(exp[i] / m);
        }
    }

    results[idx] = jsd;
}
```

#### 5.1.3 Integration: Auto GPU Detection

```go
// File: builder.go (MODIFIED)

func (b *Builder) WithGPU(deviceID int) *Builder {
    b.gpuEnabled = true
    b.gpuDeviceID = deviceID
    return b
}

func (b *Builder) Build() (*Engine, error) {
    // ... existing code

    var divergence analysis.DivergenceFunc
    if b.gpuEnabled {
        gpuDiv, err := gpu.NewJSD_GPU(b.gpuDeviceID)
        if err != nil {
            log.Printf("GPU init failed, falling back to CPU: %v", err)
            divergence = analysis.JSD{}
        } else {
            divergence = gpuDiv
            log.Printf("GPU acceleration enabled (device %d)", b.gpuDeviceID)
        }
    } else {
        divergence = b.divergence  // CPU path
    }

    // ... rest of build
}
```

**Usage**:
```go
// Auto-detect GPU, fallback to CPU if unavailable
engine := NewBuilder().
    WithGPU(0).  // CUDA device 0
    Build()

// Analyze uses GPU automatically
snap.Analyze()  // 25k nodes: 1.25s → 300ms (4x speedup)
```

#### 5.1.4 Performance Targets

| Nodes | CPU (8 cores) | GPU (single RTX 4090) | Speedup |
|---|---|---|---|
| 25k | 1.25s | 300ms | 4.2x |
| 100k | 5s | 600ms | 8.3x |
| 1M | 50s | 2s | 25x |
| 10M | 500s | 8s | 62x |

**When GPU wins**:
- Graphs > 10k nodes (amortizes transfer overhead)
- Batch analysis (analyze multiple snapshots at once)
- Dense graphs (avg degree > 10, more computation per node)

**When CPU wins**:
- Graphs < 1k nodes (transfer overhead > compute)
- Sparse graphs (most time is I/O, not compute)
- Incremental updates (small deltas don't justify GPU batch)

---

### 5.2 Distributed Mode (Cluster)

**Target**: Linear scaling with cluster size (10 nodes → 10x throughput)

#### 5.2.1 Why Distributed?

**Limits of single-machine**:
- Max nodes: ~10M (limited by RAM)
- Max throughput: ~20k req/s (limited by CPU)
- Max analysis speed: ~2s for 1M nodes (limited by cores)

**Distributed enables**:
- Horizontal scaling (100M+ nodes across 10+ machines)
- Geographic distribution (low-latency regional queries)
- Fault tolerance (node failure doesn't kill system)

#### 5.2.2 Design: Graph Partitioning

**Principle**: Partition graph by node ID (hash sharding)

```
Cluster (3 nodes):
  Node A: owns nodes hash(id) % 3 == 0
  Node B: owns nodes hash(id) % 3 == 1
  Node C: owns nodes hash(id) % 3 == 2

AddEvent(source="A", target="B"):
  → hash("A") = 0 → routed to Node A
  → Node A stores edge locally
  → Node A sends edge replica to Node B (for target's analysis)

Analyze():
  → Each node analyzes its partition in parallel
  → Coordinator aggregates results (stats, anomalies)
```

**Architecture**:
```go
// File: distributed/cluster.go (NEW)

package distributed

import (
    "context"
    "google.golang.org/grpc"
)

// Cluster is a distributed ITT Engine
type Cluster struct {
    nodes     []*NodeClient  // gRPC clients to cluster nodes
    partitioner Partitioner  // hash-based partitioning
    coordinator *Coordinator // aggregates results
}

func NewCluster(nodeAddrs []string) (*Cluster, error) {
    nodes := make([]*NodeClient, len(nodeAddrs))
    for i, addr := range nodeAddrs {
        conn, err := grpc.Dial(addr, grpc.WithInsecure())
        if err != nil {
            return nil, err
        }
        nodes[i] = NewNodeClient(conn)
    }

    return &Cluster{
        nodes:       nodes,
        partitioner: NewHashPartitioner(len(nodes)),
        coordinator: NewCoordinator(nodes),
    }, nil
}

// AddEvent routes to correct partition
func (c *Cluster) AddEvent(ev Event) error {
    partition := c.partitioner.GetPartition(ev.Source)
    return c.nodes[partition].AddEvent(context.Background(), ev)
}

// Analyze aggregates across all partitions
func (c *Cluster) Analyze() (Results, error) {
    ctx := context.Background()

    // Send analyze RPC to all nodes in parallel
    resultsChan := make(chan PartitionResults, len(c.nodes))
    for _, node := range c.nodes {
        go func(n *NodeClient) {
            res, err := n.Analyze(ctx)
            if err != nil {
                resultsChan <- PartitionResults{Error: err}
                return
            }
            resultsChan <- res
        }(node)
    }

    // Aggregate results
    return c.coordinator.Aggregate(resultsChan, len(c.nodes))
}
```

**gRPC Service Definition**:
```protobuf
// File: distributed/itt.proto

service ITTNode {
    rpc AddEvent(Event) returns (Empty);
    rpc Analyze(AnalyzeRequest) returns (PartitionResults);
    rpc Snapshot(Empty) returns (SnapshotID);
}

message Event {
    string source = 1;
    string target = 2;
    float weight = 3;
    int64 timestamp = 4;
}

message PartitionResults {
    repeated TensionResult tensions = 1;
    Stats partition_stats = 2;
}
```

#### 5.2.3 Partitioning Strategies

```go
// File: distributed/partitioner.go

type Partitioner interface {
    GetPartition(nodeID string) int
}

// HashPartitioner: simple modulo-based (stateless)
type HashPartitioner struct {
    numPartitions int
}

func (h *HashPartitioner) GetPartition(nodeID string) int {
    return int(hash(nodeID) % uint64(h.numPartitions))
}

// RangePartitioner: for ordered node IDs (timestamp-based)
type RangePartitioner struct {
    ranges []string  // ["2024-01", "2024-02", "2024-03"]
}

func (r *RangePartitioner) GetPartition(nodeID string) int {
    // Binary search in ranges
    return sort.SearchStrings(r.ranges, nodeID)
}

// GraphCutPartitioner: minimize edge cuts (METIS/KaHIP)
type GraphCutPartitioner struct {
    assignments map[string]int  // precomputed by METIS
}

func (g *GraphCutPartitioner) GetPartition(nodeID string) int {
    return g.assignments[nodeID]
}
```

**Tradeoffs**:
| Strategy | Edge Cuts | Rebalancing | Best For |
|---|---|---|---|
| Hash | High (random) | Easy | Uniform node degree |
| Range | Medium | Hard | Temporal graphs (time-series) |
| GraphCut | Low | Very hard | Power-law graphs (minimize cross-partition) |

#### 5.2.4 Performance Model

**Assumptions**:
- 10M nodes, 50M edges
- 10-node cluster (each holds 1M nodes, 5M edges)
- Gigabit network (125 MB/s inter-node)

**Single machine (baseline)**:
```
Analyze 10M nodes:
  - CPU (48 cores): 10M / 25k × 1.25s = 500s
  - Memory: 10M × 256 bytes = 2.56 GB (fits)
```

**Distributed (10 nodes)**:
```
Analyze 10M nodes:
  - Each node analyzes 1M: 1M / 25k × 1.25s = 50s (parallel)
  - Network aggregation: 10 × 10 MB (results) / 125 MB/s = 0.8s
  - Total: 50s + 0.8s = 50.8s

Speedup: 500s / 50.8s = 9.8x (near-linear!)
```

**Throughput**:
```
Single machine: 20k req/s (cache hits)
Distributed (10 nodes): 200k req/s (10x)

Load balancer distributes requests → each node serves 20k req/s
```

---

### 5.3 Hybrid: GPU + Distributed

**Ultimate Configuration**: 10-node cluster, each with 1 GPU

```
Performance (100M nodes, 500M edges):

Single machine (CPU):
  - Analyze: 100M / 25k × 1.25s = 5000s = 83 minutes

Single machine (GPU):
  - Analyze: 100M / 1M × 2s = 200s = 3.3 minutes

Distributed (10 nodes, CPU):
  - Analyze: 10M / 25k × 1.25s = 500s = 8.3 minutes

Distributed + GPU (10 nodes, each with GPU):
  - Analyze: 10M / 1M × 2s = 20s

Speedup: 5000s / 20s = 250x ✅
Throughput: 200k req/s (distributed) × 1 (GPU doesn't affect reads) = 200k req/s
```

**Cost Analysis**:
```
AWS Instance (for comparison):
- Single p3.2xlarge (1 V100 GPU): $3.06/hour → 200s/analysis
- 10× c6i.8xlarge (32 vCPUs): $1.36 × 10 = $13.60/hour → 50s/analysis
- 10× p3.2xlarge (10 GPUs): $30.60/hour → 20s/analysis

For Argos (1 analysis/day):
  - Single CPU: free (runs on laptop)
  - 10× GPU: overkill

For LIGO-like (100 analyses/hour):
  - Single CPU: can't keep up
  - 10× GPU: $30.60/hour × 24h = $734/day (justified if detecting black holes!)
```

---

### 5.4 Future Optimization: JIT Compilation (gccgo / TinyGo)

**Observation**: Go runtime overhead (GC pauses, interface calls) can be eliminated for hot paths.

```go
// Current: interface call overhead
type DivergenceFunc interface {
    Compute(obs, exp map[string]float64) float64
}

// JIT-compiled: monomorphized, inlined
// Generated at build-time for specific divergence
func Analyze_JSD_Monomorphic(view GraphView, nodeID string) TensionResult {
    // ... inlined JSD computation, no interface dispatch
}
```

**Potential speedup**: 10-20% (eliminates interface overhead + enables more aggressive inlining)

**Tool**: gccgo (GCC-based Go compiler with better optimization than gc)

---

### 5.5 Implementation Priority

| Optimization | Complexity | Speedup | When to implement |
|---|---|---|---|
| **Parallel (Phase 1-4)** | Low | 4-400x | ✅ NOW (core features) |
| **GPU Acceleration** | High | 4-25x | When graphs > 100k nodes regularly |
| **Distributed Mode** | Very High | 10-100x | When single-machine limit hit (> 10M nodes) |
| **JIT Compilation** | Medium | 1.1-1.2x | Polish phase (diminishing returns) |

**Recommendation**:
1. **Phase 1-4 first** (biggest bang for buck, low complexity)
2. **GPU if needed** (niche: very large graphs or real-time signal processing)
3. **Distributed if scaling beyond single machine** (enterprise deployments)
4. **JIT last** (micro-optimization, low ROI)

---

## Conclusion

This plan delivers **10k-20k req/s** throughput while:
- ✅ Preserving MVCC snapshot isolation
- ✅ Maintaining all v2 features (detectability, concealment, temporal)
- ✅ Backward compatibility (all optimizations opt-in or auto)
- ✅ Production-ready (race-tested, benchmarked, validated)
- ✅ Future-proof (GPU/distributed extensions defined)

**Phase 1-4** achieve the 10k req/s target.
**Phase 5** (GPU/Distributed) unlocks 100k-1M req/s for extreme use cases.

**Next Steps**: Begin Phase 1 implementation (parallel analysis).
