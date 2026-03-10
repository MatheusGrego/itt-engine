# ITT Engine

A high-performance anomaly detection SDK for Go based on **Informational Tension Theory (ITT)**.

ITT Engine models data as a directed weighted graph and detects anomalies by measuring *informational tension* — the divergence between a node's observed interaction pattern and its expected distribution. Nodes whose tension exceeds a threshold are flagged as anomalous.

## Installation

```bash
go get github.com/MatheusGrego/itt-engine
```

Requires **Go 1.25+**. Zero external dependencies.

## Quick Start

```go
package main

import (
    "fmt"
    "time"

    itt "github.com/MatheusGrego/itt-engine"
)

func main() {
    engine, _ := itt.NewBuilder().
        Threshold(0.3).
        OnAnomaly(func(r itt.TensionResult) {
            fmt.Printf("Anomaly: node=%s tension=%.4f\n", r.NodeID, r.Tension)
        }).
        Build()

    // Ingest events
    events := []itt.Event{
        {Source: "user:alice", Target: "service:api", Weight: 1},
        {Source: "user:alice", Target: "service:api", Weight: 1},
        {Source: "user:alice", Target: "service:api", Weight: 1},
        {Source: "user:bob", Target: "service:db", Weight: 1},
        {Source: "user:bob", Target: "service:api", Weight: 1},
        {Source: "user:charlie", Target: "service:db", Weight: 50}, // suspicious
    }
    for _, ev := range events {
        engine.AddEvent(ev)
    }

    // Wait for events to process, then analyze
    time.Sleep(50 * time.Millisecond)

    results, _ := engine.Analyze()
    fmt.Printf("Analyzed %d nodes, found %d anomalies\n",
        results.Stats.NodesAnalyzed, results.Stats.AnomalyCount)

    engine.Stop()
}
```

## Architecture

```
Event --> Engine (MVCC) --> Graph (immutable, COW)
                |
        Snapshot (isolated)
                |
    Analysis (tension, curvature, concealment,
              detectability, temporal dynamics)
                |
        Results / Callbacks
```

**Layered design:** `types` --> `graph` --> `mvcc` --> `analysis` --> `engine` --> `builder`

| Package | Purpose |
|---------|---------|
| `itt` (root) | Builder, Engine, Snapshot, public types |
| `graph/` | Mutable graph, ImmutableGraph (COW), UnifiedView |
| `mvcc/` | Version controller, ref-counted snapshots, GC |
| `analysis/` | Divergence, tension, curvature, MAD calibration, concealment, detectability, temporal dynamics, Yharim limit, Fiedler value |
| `compact/` | Overlay-to-base compaction |
| `export/` | JSON and DOT format export |

## Core Concepts

### Informational Tension

Tension measures how "surprising" a node's connections are:

```
tau(v) = D(P_observed || P_expected)
```

Where `D` is a divergence function (JSD by default). High tension = the node's interaction pattern deviates from expectation.

### MVCC Snapshots

Every event creates a new immutable graph version via copy-on-write. Snapshots are isolated — analysis runs on a consistent point-in-time view while ingestion continues at full speed.

### Base + Overlay

The engine maintains a **base** (compacted history) and an **overlay** (recent events). Compaction periodically merges the overlay into the base to bound memory. Snapshots see both layers through a `UnifiedView`.

## Builder API

```go
engine, err := itt.NewBuilder().
    // Divergence function (default: JSD)
    Divergence(analysis.JSD{}).

    // Static anomaly threshold (default: 0.2)
    Threshold(0.3).

    // Dynamic threshold function (overrides static + calibrator)
    ThresholdFunc(func(node *itt.Node, tension float64) bool {
        return tension > 0.5 && node.Degree > 3
    }).

    // Curvature analysis (Ollivier-Ricci)
    CurvatureAlpha(0.5).

    // Custom CurvatureFunc (takes priority over CurvatureAlpha)
    Curvature(myCurvatureImpl).

    // MAD-based calibrator (overrides static threshold)
    WithCalibrator(analysis.NewMADCalibrator(3.0, 100)).

    // Detectability analysis (default: alpha=0.05)
    DetectabilityAlpha(0.05).

    // Concealment analysis (disabled by default)
    Concealment(0.5, 2). // lambda=0.5, maxHops=2

    // Temporal dynamics
    TemporalCapacity(100).        // ring buffer per node (default: 100)
    DiffusivityAlpha(0.1).        // diffusion constant (default: 0.1)
    TensionSpikeThreshold(0.3).   // spike callback threshold (default: 0.3)

    // Node type extraction
    NodeTypeFunc(func(id string) string {
        parts := strings.SplitN(id, ":", 2)
        return parts[0]
    }).

    // Custom edge weight
    WeightFunc(func(e itt.Event) float64 {
        return e.Weight * 2
    }).

    // Aggregation for region analysis
    AggregationFunc(itt.AggMean).

    // Compaction strategy
    CompactionStrategy(itt.CompactByVolume).
    CompactionThreshold(10000).

    // GC settings
    GCSnapshotWarning(5 * time.Minute).
    GCSnapshotForce(15 * time.Minute).

    // Storage persistence
    WithStorage(myStorageImpl).
    BaseGraph(preloadedGraphData).

    // Callbacks
    OnAnomaly(func(r itt.TensionResult) { /* ... */ }).
    OnChange(func(d itt.Delta) { /* ... */ }).
    OnCompact(func(s itt.CompactStats) { /* ... */ }).
    OnGC(func(s itt.GCStats) { /* ... */ }).
    OnTensionSpike(func(nodeID string, delta float64) { /* ... */ }).
    OnError(func(err error) { /* ... */ }).

    // Observability
    WithLogger(myLogger).

    Build()
```

### Defaults

| Setting | Default |
|---------|---------|
| Threshold | `0.2` |
| Divergence | JSD (Jensen-Shannon) |
| DetectabilityAlpha | `0.05` |
| TemporalCapacity | `100` |
| DiffusivityAlpha | `0.1` |
| TensionSpikeThreshold | `0.3` |
| Compaction | ByVolume, threshold 10,000 |
| GC warning | 5 min |
| GC force-close | 15 min |
| Channel size | 10,000 |

## Engine Operations

```go
// Lifecycle
engine.Start(ctx)       // start processing (auto-starts on first AddEvent)
engine.Stop()           // graceful shutdown (drains pending events)
engine.Running()        // check if running

// Ingestion
engine.AddEvent(event)  // single event
engine.AddEvents(batch) // batch (all-or-nothing validation)

// Analysis
results, err := engine.Analyze()                // full analysis
result, err := engine.AnalyzeNode("node-id")    // single node
region, err := engine.AnalyzeRegion([]string{…}) // subset

// Snapshots (for manual control)
snap := engine.Snapshot()
defer snap.Close()
results, _ := snap.Analyze()

// Maintenance
engine.Compact()        // force compaction
engine.Reset()          // clear all data
engine.Stats()          // runtime metrics
```

## Snapshot API

Snapshots provide isolated, point-in-time views:

```go
snap := engine.Snapshot()
defer snap.Close() // always close to release the version

// Analysis
results, _ := snap.Analyze()
result, _ := snap.AnalyzeNode("user:alice")
region, _ := snap.AnalyzeRegion([]string{"user:alice", "user:bob"})

// Graph queries
node, ok, _ := snap.GetNode("user:alice")
edge, ok, _ := snap.GetEdge("user:alice", "service:api")

// Iteration
snap.ForEachNode(func(n *graph.NodeData) bool { return true })
snap.ForEachEdge(func(e *graph.EdgeData) bool { return true })

// Neighbors
neighbors, _ := snap.Neighbors("user:alice")
in, _ := snap.InNeighbors("user:alice")
out, _ := snap.OutNeighbors("user:alice")

// Metadata
snap.Version()    // version ID
snap.Timestamp()  // version timestamp
snap.NodeCount()
snap.EdgeCount()

// Export
snap.Export(itt.ExportJSON, writer)
snap.Export(itt.ExportDOT, writer)
```

## Divergence Functions

Three built-in divergence measures in `analysis/`:

| Function | Formula | Properties | Bounded |
|----------|---------|------------|---------|
| `JSD` | Jensen-Shannon Divergence | Symmetric, [0, ln2] | Yes |
| `KL` | Kullback-Leibler Divergence | Asymmetric, unbounded | No |
| `Hellinger` | Hellinger Distance | Symmetric, [0, 1] | Yes |

```go
itt.NewBuilder().Divergence(analysis.JSD{})
itt.NewBuilder().Divergence(analysis.KL{})
itt.NewBuilder().Divergence(analysis.Hellinger{})
```

**Note:** The detectability framework requires bounded divergence (JSD or Hellinger). Using unbounded divergence (KL) with detectability will log a warning.

## Anomaly Detection

Three-level priority chain for classifying anomalies:

1. **ThresholdFunc** (highest priority) — custom function `(node, tension) -> bool`
2. **Calibrator** — dynamic MAD-based threshold (median + K * MAD)
3. **Static threshold** — fixed value (default: 0.2)

```go
// Static
builder.Threshold(0.3)

// Dynamic (MAD-based, K=3.0, warmup=100 samples)
builder.WithCalibrator(analysis.NewMADCalibrator(3.0, 100))

// Custom logic
builder.ThresholdFunc(func(node *itt.Node, tension float64) bool {
    if node.Type == "admin" {
        return tension > 0.8 // stricter for admins
    }
    return tension > 0.3
})
```

## Detectability Framework

The detectability framework answers: **"Can anomalies actually be detected in this data?"**

Based on the Yharim limit from ITT theory:

```
Upsilon(alpha) = sqrt(2 * ln(1/alpha))
```

Anomalies are classified into three detectability regions based on SNR (signal-to-noise ratio) vs the Yharim limit:

| Region | Condition | Meaning |
|--------|-----------|---------|
| **Undetectable** | SNR < Upsilon | No method can reliably detect anomalies |
| **WeaklyDetectable** | Upsilon < SNR < 2*Upsilon | Requires global analysis |
| **StronglyDetectable** | SNR > 2*Upsilon | Local analysis is sufficient |

```go
engine, _ := itt.NewBuilder().
    DetectabilityAlpha(0.05). // false positive rate (default)
    Build()

results, _ := engine.Analyze()

fmt.Printf("SNR: %.2f\n", results.Detectability.SNR)
fmt.Printf("Region: %d\n", results.Detectability.Region) // 0=Undetectable, 1=Weakly, 2=Strongly
fmt.Printf("Yharim limit: %.2f\n", results.Detectability.Threshold)
```

Region analysis also includes detectability:

```go
region, _ := snap.AnalyzeRegion(nodeIDs)
fmt.Printf("Region SNR: %.2f\n", region.Detectability.SNR)
```

## Concealment Analysis

Concealment measures how expensive it is to hide manipulation in a network neighborhood:

```
Omega(Ns) = sum_k sum_vi tau(vi) * exp(-lambda * k)
```

Higher concealment cost means the anomaly is harder to conceal — it requires manipulating more of the surrounding network.

```go
engine, _ := itt.NewBuilder().
    Concealment(0.5, 2). // lambda=0.5, maxHops=2
    Build()

results, _ := engine.Analyze()

for _, tr := range results.Tensions {
    fmt.Printf("node=%s concealment=%.4f\n", tr.NodeID, tr.Concealment)
}
```

### CPS (Concealment Probability Score)

CPS combines concealment cost with detectability. Available in `RegionResult`:

```
CPS(Sigma) = normalize(Omega) * 1_{SNR > Upsilon(alpha)}
```

CPS is 0 if the region is below the detectability threshold. Otherwise it's a normalized score in [0, 1].

```go
region, _ := snap.AnalyzeRegion(nodeIDs)
fmt.Printf("CPS: %.4f\n", region.CPS) // 0=undetectable or zero cost, 1=high concealment cost
```

## Temporal Dynamics

The temporal analysis tracks how tension evolves over time, implementing the ITT tension diffusion equation:

```
d_tau/dt = -alpha * L * tau + S(v, t)
```

### Tension History

Per-node ring buffer tracks tension samples across events. Configurable capacity:

```go
engine, _ := itt.NewBuilder().
    TemporalCapacity(100). // samples per node (default: 100)
    Build()
```

### Trends

Each node's `TensionResult` includes a `Trend` field:

| Trend | Meaning |
|-------|---------|
| `TrendStable` | Tension is steady (`|delta| < epsilon`) |
| `TrendIncreasing` | Tension is growing (active anomaly) |
| `TrendDecreasing` | Tension is decaying (recovery) |

```go
results, _ := snap.Analyze()
for _, tr := range results.Tensions {
    fmt.Printf("node=%s trend=%v\n", tr.NodeID, tr.Trend)
}
```

### Temporal Indicators

The `Results.Temporal` field contains:

| Indicator | Description |
|-----------|-------------|
| `TensionSpike` | max `|delta tau|` across nodes between snapshots |
| `DecayExponent` | gamma(t) — positive = recovery, negative = growth |
| `CurvatureShock` | max `|delta kappa|` across edges |
| `Phase` | Suppression phase (0-3) |
| `Velocity` | Velocity of silence — anomaly propagation speed |

### Phase Classification

Four suppression phases based on rho (suppression intensity) and pi (healing capacity):

| Phase | Name | rho | pi | Meaning |
|-------|------|-----|----|---------|
| 0 | FullRecovery | Low | High | Tension dissipates, structure heals |
| 1 | ScarredRecovery | Low | Low | Tension dissipates, structure retains damage |
| 2 | ChronicTension | High | High | Sustained tension, continuous adaptation |
| 3 | StructuralCollapse | High | Low | Runaway tension, eventual failure |

### OnTensionSpike Callback

Fires when a node's tension delta exceeds the spike threshold during ingestion:

```go
engine, _ := itt.NewBuilder().
    TensionSpikeThreshold(0.3). // minimum delta to trigger (default: 0.3)
    OnTensionSpike(func(nodeID string, delta float64) {
        fmt.Printf("Spike: node=%s delta=%.4f\n", nodeID, delta)
    }).
    Build()
```

### Velocity of Silence

Estimates how fast anomaly information propagates through the network:

```
v_silence = alpha * sqrt(lambda_1) * mean_edge_length
```

Where lambda_1 is the Fiedler value (algebraic connectivity). Available in `Results.Temporal.Velocity`.

## Curvature

Ollivier-Ricci curvature measures local geometric distortion:

```
kappa(x,y) = 1 - W1(mu_x, mu_y) / d(x,y)
```

Computed via Sinkhorn optimal transport. Negative curvature indicates structural bottlenecks.

```go
// Via alpha parameter
builder.CurvatureAlpha(0.5) // alpha in (0,1], controls neighbor weight mixing

// Via custom CurvatureFunc (takes priority over alpha)
builder.Curvature(myCurvatureImpl)
```

Results include curvature per node:

```go
result.Curvature // average edge curvature for the node
```

## Storage

Persist the base graph across engine restarts:

```go
type MyStorage struct { /* ... */ }
func (s *MyStorage) Load() (*itt.GraphData, error) { /* ... */ }
func (s *MyStorage) Save(data *itt.GraphData) error { /* ... */ }

engine, _ := itt.NewBuilder().
    WithStorage(&MyStorage{}).
    Build()
```

- `Load()` is called during engine initialization (before Start)
- `Save()` is called asynchronously after each compaction

You can also pre-populate the base graph without a storage backend:

```go
engine, _ := itt.NewBuilder().
    BaseGraph(&itt.GraphData{
        Nodes: []*itt.Node{{ID: "a"}, {ID: "b"}},
        Edges: []*itt.Edge{{From: "a", To: "b", Weight: 1.0}},
    }).
    Build()
```

## Callbacks

Real-time event streaming via callbacks:

```go
// Anomaly detected in real-time (during ingestion)
builder.OnAnomaly(func(r itt.TensionResult) {
    log.Printf("anomaly: %s tension=%.4f", r.NodeID, r.Tension)
})

// Tension spike detected
builder.OnTensionSpike(func(nodeID string, delta float64) {
    log.Printf("spike: %s delta=%.4f", nodeID, delta)
})

// Graph mutations
builder.OnChange(func(d itt.Delta) {
    switch d.Type {
    case itt.DeltaNodeAdded:
        // new node (d.Node populated)
    case itt.DeltaEdgeAdded:
        // new edge (d.Edge populated)
    case itt.DeltaEdgeUpdated:
        // edge weight/count changed (d.Edge + d.Previous)
    case itt.DeltaAnomalyDetected:
        // anomaly detected (d.Tension + d.Node)
    case itt.DeltaTensionChanged:
        // trend direction changed (d.NodeID + d.Tension)
    }
})

// Compaction completed
builder.OnCompact(func(s itt.CompactStats) {
    log.Printf("compacted: %d nodes, %d edges merged", s.NodesMerged, s.EdgesMerged)
})

// GC collected old versions
builder.OnGC(func(s itt.GCStats) {
    log.Printf("gc: %d versions removed", s.VersionsRemoved)
})

// Error handler (including panic recovery from callbacks)
builder.OnError(func(err error) {
    log.Printf("engine error: %v", err)
})
```

## Built-in Aggregations

For `AnalyzeRegion` and custom analysis:

```go
builder.AggregationFunc(itt.AggMean)   // arithmetic mean
builder.AggregationFunc(itt.AggMax)    // maximum
builder.AggregationFunc(itt.AggMedian) // median
builder.AggregationFunc(itt.AggSum)    // sum
```

## Compaction

Compaction merges the overlay into the base graph to bound memory:

```go
// Automatic: by event count (default)
builder.CompactionStrategy(itt.CompactByVolume).
    CompactionThreshold(10000) // compact every 10k events

// Automatic: by time interval
builder.CompactionStrategy(itt.CompactByTime).
    CompactionInterval(5 * time.Minute)

// Manual only
builder.CompactionStrategy(itt.CompactManual)
engine.Compact() // trigger manually
```

## Export

```go
snap := engine.Snapshot()
defer snap.Close()

// JSON
var buf bytes.Buffer
snap.Export(itt.ExportJSON, &buf)

// DOT (for Graphviz)
snap.Export(itt.ExportDOT, &buf)
```

## Analysis Utilities

Standalone tools in the `analysis/` package:

### Yharim Detectability

Theoretical detectability threshold based on signal-to-noise ratio:

```go
limit := analysis.YharimLimit(0.05)                // Upsilon(alpha)
snr := analysis.SNR(tensions)                       // signal-to-noise ratio
detectable := analysis.IsDetectable(tensions, 0.05) // SNR > Yharim limit?
result := analysis.Detectability(tensions, 0.05)    // full result
region := analysis.Classify(tensions, 0.05)         // region only
```

### Concealment Cost (standalone)

```go
tc := analysis.NewTensionCalculator(analysis.JSD{})
cc := analysis.NewConcealmentCalculator(0.5, tc) // lambda=0.5

cost := cc.Calculate(graphView, []string{"nodeA", "nodeB"}, 3)     // set
nodeCost := cc.CalculateNode(graphView, "nodeA", 3)                // single node
cps := analysis.CPS(tensions, cost, 0.05)                          // CPS score
```

### Temporal Analysis (standalone)

```go
// Tension history ring buffer
h := analysis.NewTensionHistory(100)
h.Push(analysis.TensionSample{Tension: 0.5, Timestamp: time.Now(), Version: 1})
latest, ok := h.Latest()
prev, ok := h.Previous()
samples := h.Slice()

// Temporal indicators
tc := analysis.NewTemporalCalculator(0.1) // alpha=0.1
indicators := tc.Indicators(currentTensions, previousTensions, dt)

// Phase classification
phase := analysis.ClassifyPhase(indicators, meanTension, prevMeanTension, connectivityRatio)

// Velocity of silence
velocity := analysis.VelocityOfSilence(alpha, lambda1, meanEdgeLength)
age := analysis.EstimateAge(distance, velocity)
```

### Fiedler Value (Algebraic Connectivity)

```go
// Exact (inverse power iteration + Jacobi solver)
lambda1 := analysis.FiedlerValue(graphView, nodeIDs, maxIter, tol)

// Fast approximation (Cheeger inequality)
lambda1 := analysis.FiedlerApprox(graphView, nodeIDs)
```

## Performance

Benchmarked on AMD Ryzen 5 3600:

| Operation | Latency | Notes |
|-----------|---------|-------|
| AddEvent | ~131 ns/op | ~7.6M events/sec |
| Snapshot | ~157 ns/op | |
| AnalyzeNode | ~1.2 us/op | |
| Analyze (100 nodes) | ~475 us | |
| Analyze (1k nodes) | ~277 ms | |
| ConcurrentAddEvent | ~172 ns/op | |
| Analyze w/ Concealment (100 nodes) | ~6.7 ms | lambda=0.5, hops=2 |
| Analyze w/ Detectability (100 nodes) | ~492 us | |
| Analyze w/ Temporal (100 nodes) | ~521 us | |
| TensionHistory.Push | ~5.9 ns/op | ring buffer |
| FiedlerApprox (100 nodes) | ~348 us | Cheeger bound |
| CheckAnomalies overhead | ~115 ns/op | per-event temporal tracking |

## License

BSL 1.1
