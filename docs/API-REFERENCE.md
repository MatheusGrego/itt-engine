# ITT Engine — API Reference

Complete API reference for the ITT Engine SDK.

---

## Root Package (`itt`)

### Builder

#### `NewBuilder() *Builder`

Creates a new engine builder with sensible defaults. Chain methods to configure, then call `Build()`.

**Default values:**

| Field | Default |
|-------|---------|
| `threshold` | `0.2` |
| `detectabilityAlpha` | `0.05` |
| `temporalCapacity` | `100` |
| `diffusivityAlpha` | `0.1` |
| `tensionSpikeThreshold` | `0.3` |
| `compactionStrategy` | `CompactByVolume` |
| `compactionThreshold` | `10000` |
| `channelSize` | `10000` |
| `gcSnapshotWarning` | `5m` |
| `gcSnapshotForce` | `15m` |

#### Builder Methods

All methods return `*Builder` for chaining.

| Method | Description |
|--------|-------------|
| `Divergence(d DivergenceFunc)` | Set divergence function (default: JSD) |
| `Threshold(t float64)` | Static anomaly threshold |
| `ThresholdFunc(f ThresholdFunc)` | Dynamic threshold (overrides calibrator + static) |
| `Curvature(c CurvatureFunc)` | Custom curvature implementation (priority over CurvatureAlpha) |
| `CurvatureAlpha(alpha float64)` | Built-in Ollivier-Ricci curvature with alpha parameter |
| `DetectabilityAlpha(alpha float64)` | False positive rate for Yharim limit (must be in (0,1)) |
| `Concealment(lambda float64, maxHops int)` | Enable concealment analysis. lambda=0 disables |
| `TemporalCapacity(n int)` | Per-node tension history ring buffer size |
| `DiffusivityAlpha(alpha float64)` | Diffusivity constant for temporal calculations |
| `TensionSpikeThreshold(t float64)` | Minimum delta to trigger OnTensionSpike |
| `Topology(t TopologyFunc)` | Topological analysis (reserved) |
| `WeightFunc(f WeightFunc)` | Custom edge weight from event |
| `NodeTypeFunc(f NodeTypeFunc)` | Extract node type from ID |
| `AggregationFunc(f AggregationFunc)` | Aggregation for AnalyzeRegion |
| `CompactionStrategy(s CompactionStrategy)` | ByVolume, ByTime, or Manual |
| `CompactionThreshold(n int)` | Event count trigger for ByVolume |
| `CompactionInterval(d time.Duration)` | Interval for ByTime |
| `GCSnapshotWarning(d time.Duration)` | Warn on long-held snapshots |
| `GCSnapshotForce(d time.Duration)` | Force-close snapshots (must be >= warning) |
| `OnAnomaly(f func(TensionResult))` | Real-time anomaly callback |
| `OnChange(f func(Delta))` | Graph mutation callback |
| `OnCompact(f func(CompactStats))` | Compaction callback |
| `OnGC(f func(GCStats))` | GC callback |
| `OnTensionSpike(f func(string, float64))` | Tension spike callback (nodeID, delta) |
| `OnError(f func(error))` | Error callback (includes panic recovery) |
| `WithLogger(l Logger)` / `SetLogger(l Logger)` | Structured logger |
| `WithStorage(s Storage)` / `SetStorage(s Storage)` | Persistence backend |
| `WithCalibrator(c Calibrator)` / `SetCalibrator(c Calibrator)` | Dynamic threshold calibrator |
| `BaseGraph(g *GraphData)` | Pre-populate base graph |
| `ChannelSize(n int)` | Event channel buffer size |

#### `Build() (*Engine, error)`

Validates configuration and returns a new Engine.

**Validation errors:**
- `threshold < 0`
- `gcSnapshotForce < gcSnapshotWarning` (when both set)
- `channelSize <= 0`
- `detectabilityAlpha <= 0 or >= 1`
- `concealmentLambda < 0`
- `concealmentHops < 0`

---

### Engine

#### Lifecycle

| Method | Signature | Description |
|--------|-----------|-------------|
| `Start` | `(ctx context.Context) error` | Begin processing events. Context cancellation triggers shutdown |
| `Stop` | `() error` | Graceful shutdown — drains pending events |
| `Running` | `() bool` | True if started and not stopped |

#### Ingestion

| Method | Signature | Description |
|--------|-----------|-------------|
| `AddEvent` | `(event Event) error` | Submit single event. Auto-starts engine if needed |
| `AddEvents` | `(events []Event) error` | Batch submit. All-or-nothing validation |

#### Analysis

| Method | Signature | Description |
|--------|-----------|-------------|
| `Analyze` | `() (*Results, error)` | Full analysis via temporary snapshot |
| `AnalyzeNode` | `(nodeID string) (*TensionResult, error)` | Single node analysis |
| `AnalyzeRegion` | `(nodeIDs []string) (*RegionResult, error)` | Subset analysis |
| `Snapshot` | `() *Snapshot` | Create isolated snapshot (must Close) |

#### Maintenance

| Method | Signature | Description |
|--------|-----------|-------------|
| `Compact` | `() error` | Force overlay compaction into base |
| `Reset` | `() error` | Clear all data, keep config |
| `Stats` | `() *EngineStats` | Runtime metrics |

---

### Snapshot

Created via `engine.Snapshot()`. Must be closed with `Close()`.

#### Analysis

| Method | Signature | Description |
|--------|-----------|-------------|
| `Analyze` | `() (*Results, error)` | Full analysis — tension, curvature, concealment, detectability, temporal |
| `AnalyzeNode` | `(nodeID string) (*TensionResult, error)` | Single node |
| `AnalyzeRegion` | `(nodeIDs []string) (*RegionResult, error)` | Subset with CPS |

#### Graph Queries

| Method | Signature | Description |
|--------|-----------|-------------|
| `GetNode` | `(id string) (*graph.NodeData, bool, error)` | Lookup by ID |
| `GetEdge` | `(from, to string) (*graph.EdgeData, bool, error)` | Lookup by endpoints |
| `Neighbors` | `(nodeID string) ([]string, error)` | All neighbor IDs |
| `InNeighbors` | `(nodeID string) ([]string, error)` | Incoming neighbors |
| `OutNeighbors` | `(nodeID string) ([]string, error)` | Outgoing neighbors |
| `ForEachNode` | `(fn func(*graph.NodeData) bool) error` | Iterate nodes |
| `ForEachEdge` | `(fn func(*graph.EdgeData) bool) error` | Iterate edges |

#### Metadata

| Method | Signature | Description |
|--------|-----------|-------------|
| `ID` | `() string` | Snapshot identifier (e.g. "snap-42") |
| `Version` | `() uint64` | MVCC version number |
| `Timestamp` | `() (time.Time, error)` | Version timestamp |
| `NodeCount` | `() (int, error)` | Total nodes |
| `EdgeCount` | `() (int, error)` | Total edges |

#### Export & Lifecycle

| Method | Signature | Description |
|--------|-----------|-------------|
| `Export` | `(format ExportFormat, w io.Writer) error` | JSON or DOT |
| `Close` | `() error` | Release version reference |

---

### Types

#### Event

```go
type Event struct {
    Source    string
    Target   string
    Type     string            // optional edge type
    Weight   float64           // default: 1.0
    Timestamp time.Time        // default: now
    Metadata  map[string]any   // arbitrary metadata
}
```

Methods: `Validate() error`, `Normalize() Event`

#### Node

```go
type Node struct {
    ID         string
    Type       string
    Degree     int
    InDegree   int
    OutDegree  int
    Attributes map[string]float64
    FirstSeen  time.Time
    LastSeen   time.Time
}
```

#### Edge

```go
type Edge struct {
    From      string
    To        string
    Weight    float64
    Type      string
    Count     int
    FirstSeen time.Time
    LastSeen  time.Time
}
```

#### TensionResult

```go
type TensionResult struct {
    NodeID      string
    Tension     float64
    Degree      int
    Curvature   float64
    Anomaly     bool
    Confidence  float64
    Concealment float64             // 0 if concealment disabled
    Trend       Trend               // Stable, Increasing, Decreasing
    Components  map[string]float64  // {"tension", "curvature", "concealment"}
}
```

#### Results

```go
type Results struct {
    Tensions      []TensionResult
    Anomalies     []TensionResult
    Stats         ResultStats
    Temporal      TemporalSummary
    SnapshotID    string
    AnalyzedAt    time.Time
    Duration      time.Duration
    Detectability DetectabilityResult
}
```

#### ResultStats

```go
type ResultStats struct {
    NodesAnalyzed int
    MeanTension   float64
    MedianTension float64
    MaxTension    float64
    StdDevTension float64
    AnomalyCount  int
    AnomalyRate   float64
}
```

#### RegionResult

```go
type RegionResult struct {
    Nodes         []TensionResult
    MeanTension   float64
    MaxTension    float64
    AnomalyCount  int
    Aggregated    float64             // via AggregationFunc
    Detectability DetectabilityResult
    CPS           float64             // Concealment Probability Score [0, 1]
}
```

#### DetectabilityResult

```go
type DetectabilityResult struct {
    SNR       float64  // signal-to-noise ratio
    Threshold float64  // Yharim limit
    Region    int      // 0=Undetectable, 1=WeaklyDetectable, 2=StronglyDetectable
    Alpha     float64  // false positive rate used
}
```

#### TemporalSummary

```go
type TemporalSummary struct {
    TensionSpike   float64  // max |delta tau| across nodes
    DecayExponent  float64  // gamma(t): positive=recovery, negative=growth
    CurvatureShock float64  // max |delta kappa| across edges
    Phase          int      // 0=FullRecovery, 1=ScarredRecovery, 2=ChronicTension, 3=StructuralCollapse
    PhaseRho       float64  // suppression intensity
    PhasePi        float64  // healing capacity
    Velocity       float64  // velocity of silence
}
```

#### Trend

```go
type Trend int

const (
    TrendStable     Trend = 0  // |delta tau| < epsilon
    TrendIncreasing Trend = 1  // tension growing
    TrendDecreasing Trend = 2  // tension decaying
)
```

Methods: `String() string`

#### Delta

```go
type Delta struct {
    Type      DeltaType
    Timestamp time.Time
    Version   uint64
    NodeID    string
    Node      *Node          // populated for DeltaNodeAdded, DeltaAnomalyDetected
    EdgeFrom  string
    EdgeTo    string
    Edge      *Edge          // populated for DeltaEdgeAdded, DeltaEdgeUpdated
    Tension   float64        // for DeltaAnomalyDetected, DeltaTensionChanged
    Previous  float64        // previous edge weight for DeltaEdgeUpdated
    Data      map[string]any
}
```

#### DeltaType

```go
const (
    DeltaNodeAdded       DeltaType = 0
    DeltaNodeUpdated     DeltaType = 1
    DeltaNodeRemoved     DeltaType = 2
    DeltaEdgeAdded       DeltaType = 3
    DeltaEdgeUpdated     DeltaType = 4
    DeltaEdgeRemoved     DeltaType = 5
    DeltaTensionChanged  DeltaType = 6
    DeltaAnomalyDetected DeltaType = 7
    DeltaAnomalyResolved DeltaType = 8
)
```

#### Other Types

```go
type EngineStats struct {
    Nodes, Edges, OverlayEvents int
    BaseNodes, BaseEdges int
    VersionsCurrent, VersionsTotal uint64
    SnapshotsActive int
    EventsTotal int64
    EventsPerSecond float64
    Uptime time.Duration
}

type CompactStats struct {
    NodesMerged, EdgesMerged int
    OverlayBefore, OverlayAfter int
    Duration time.Duration
    Timestamp time.Time
}

type GCStats struct {
    VersionsRemoved int
    MemoryFreed int64
    OldestRemoved uint64
    Timestamp time.Time
}

type GraphData struct {
    Nodes     []*Node
    Edges     []*Edge
    Metadata  map[string]any
    Timestamp time.Time
}

type ExportFormat int  // ExportJSON=0, ExportDOT=1
type CompactionStrategy int  // CompactByVolume=0, CompactByTime=1, CompactManual=2
```

---

### Interfaces

#### DivergenceFunc

```go
type DivergenceFunc interface {
    Compute(p, q []float64) float64
    Name() string
}
```

#### CurvatureFunc

```go
type CurvatureFunc interface {
    Compute(g GraphView, from, to string) float64
    Name() string
}
```

#### GraphView (root package)

```go
type GraphView interface {
    GetNode(id string) (*Node, bool)
    GetEdge(from, to string) (*Edge, bool)
    Neighbors(nodeID string) []string
    InNeighbors(nodeID string) []string
    OutNeighbors(nodeID string) []string
}
```

#### Storage

```go
type Storage interface {
    Load() (*GraphData, error)
    Save(data *GraphData) error
}
```

#### Calibrator

```go
type Calibrator interface {
    Observe(tension float64)
    IsWarmedUp() bool
    Threshold() float64
    IsAnomaly(tension float64) bool
    Stats() CalibratorStats
    Recalibrate()
}
```

#### Logger

```go
type Logger interface {
    Debug(msg string, keysAndValues ...any)
    Info(msg string, keysAndValues ...any)
    Warn(msg string, keysAndValues ...any)
    Error(msg string, keysAndValues ...any)
}
```

#### Function Types

```go
type WeightFunc      func(Event) float64
type NodeTypeFunc    func(nodeID string) string
type ThresholdFunc   func(node *Node, tension float64) bool
type AggregationFunc func(tensions []float64) float64
```

---

### Built-in Aggregations

```go
var AggMean   AggregationFunc  // arithmetic mean
var AggMax    AggregationFunc  // maximum
var AggMedian AggregationFunc  // median
var AggSum    AggregationFunc  // sum
```

### Errors

```go
var (
    ErrEmptySource    = errors.New("event source cannot be empty")
    ErrEmptyTarget    = errors.New("event target cannot be empty")
    ErrNegativeWeight = errors.New("event weight cannot be negative")
    ErrEngineStopped  = errors.New("engine is not running")
    ErrEngineRunning  = errors.New("engine is already running")
    ErrSnapshotClosed = errors.New("snapshot is closed")
    ErrNodeNotFound   = errors.New("node not found")
    ErrEdgeNotFound   = errors.New("edge not found")
    ErrInvalidConfig  = errors.New("invalid configuration")
)
```

---

## `analysis/` Package

### Divergence

#### Types

```go
type DivergenceFunc interface {
    Compute(p, q []float64) float64
    Name() string
}

type BoundedDivergence interface {
    IsBounded() bool
}

type JSD struct{}       // Compute, Name, IsBounded (true)
type KL struct{}        // Compute, Name, IsBounded (false)
type Hellinger struct{}  // Compute, Name, IsBounded (true)
```

#### Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `Normalize` | `(dist []float64) []float64` | Normalize distribution to sum=1 |

### Tension

```go
type GraphView interface {
    GetNode(id string) (*graph.NodeData, bool)
    GetEdge(from, to string) (*graph.EdgeData, bool)
    Neighbors(nodeID string) []string
    InNeighbors(nodeID string) []string
    OutNeighbors(nodeID string) []string
    NodeCount() int
    EdgeCount() int
    ForEachNode(fn func(*graph.NodeData) bool)
    ForEachEdge(fn func(*graph.EdgeData) bool)
}
```

| Function | Signature | Description |
|----------|-----------|-------------|
| `NewTensionCalculator` | `(div DivergenceFunc) *TensionCalculator` | Create calculator |
| `Calculate` | `(g GraphView, nodeID string) float64` | Tension for one node |
| `CalculateAll` | `(g GraphView) map[string]float64` | Tension for all nodes |

### Curvature

| Function | Signature | Description |
|----------|-----------|-------------|
| `NewCurvatureCalculator` | `(alpha float64) *CurvatureCalculator` | Create with alpha parameter |
| `Calculate` | `(g GraphView, from, to string) float64` | Curvature for one edge |
| `CalculateAll` | `(g GraphView) map[[2]string]float64` | Curvature for all edges |

### Calibrator (MAD)

| Function | Signature | Description |
|----------|-----------|-------------|
| `NewCalibrator` | `(opts ...CalibratorOption) *MADCalibrator` | Create with options |
| `NewMADCalibrator` | `(k float64, warmupSize int) *MADCalibrator` | Convenience constructor |
| `WithK` | `(k float64) CalibratorOption` | Set K multiplier |
| `WithWarmupSize` | `(n int) CalibratorOption` | Set warmup count |
| `WithPrecomputedBaseline` | `(median, mad float64) CalibratorOption` | Skip warmup |

`MADCalibrator` methods: `Observe(tension)`, `IsWarmedUp() bool`, `Threshold() float64`, `IsAnomaly(tension) bool`, `Stats() CalibratorStats`, `Recalibrate()`

### Concealment

| Function | Signature | Description |
|----------|-----------|-------------|
| `NewConcealmentCalculator` | `(lambda float64, tc *TensionCalculator) *ConcealmentCalculator` | Create with decay parameter |
| `Calculate` | `(g GraphView, nodeIDs []string, maxHops int) float64` | Cost for node set |
| `CalculateNode` | `(g GraphView, nodeID string, maxHops int) float64` | Cost for single node |

### Detectability

| Function | Signature | Description |
|----------|-----------|-------------|
| `Classify` | `(tensions []float64, alpha float64) DetectabilityRegion` | Region only |
| `Detectability` | `(tensions []float64, alpha float64) DetectabilityResult` | Full result |
| `CPS` | `(tensions []float64, concealmentCost float64, alpha float64) float64` | Concealment Probability Score |

```go
type DetectabilityRegion int  // Undetectable=0, WeaklyDetectable=1, StronglyDetectable=2
type DetectabilityResult struct {
    SNR, Threshold float64
    Region DetectabilityRegion
    Alpha float64
}
```

### Yharim

| Function | Signature | Description |
|----------|-----------|-------------|
| `YharimLimit` | `(alpha float64) float64` | Upsilon = sqrt(2*ln(1/alpha)) |
| `SNR` | `(tensions []float64) float64` | Signal-to-noise ratio = (mean/stddev)*sqrt(n) |
| `IsDetectable` | `(tensions []float64, alpha float64) bool` | SNR > YharimLimit(alpha) |

### Temporal

#### TensionHistory

```go
func NewTensionHistory(capacity int) *TensionHistory
```

| Method | Signature | Description |
|--------|-----------|-------------|
| `Push` | `(s TensionSample)` | Add sample (overwrites oldest if full) |
| `Len` | `() int` | Count (capped at capacity) |
| `Latest` | `() (TensionSample, bool)` | Most recent |
| `Previous` | `() (TensionSample, bool)` | Second-to-last |
| `Slice` | `() []TensionSample` | All samples, oldest first |

```go
type TensionSample struct {
    Tension   float64
    Timestamp time.Time
    Version   uint64
}
```

#### TemporalCalculator

```go
func NewTemporalCalculator(alpha float64) *TemporalCalculator
```

| Method | Signature | Description |
|--------|-----------|-------------|
| `Indicators` | `(current, previous map[string]float64, dt time.Duration) TemporalIndicators` | Without curvature |
| `IndicatorsWithCurvature` | `(current, previous map[string]float64, currentCurv, prevCurv map[[2]string]float64, dt time.Duration) TemporalIndicators` | Full indicators |

```go
type TemporalIndicators struct {
    TensionSpike   float64
    DecayExponent  float64
    CurvatureShock float64
    Timestamp      time.Time
}
```

#### Phase Classification

```go
func ClassifyPhase(indicators TemporalIndicators, meanTension, prevMeanTension, connectivityRatio float64) PhaseResult

type Phase int  // PhaseFullRecovery=0, PhaseScarredRecovery=1, PhaseChronicTension=2, PhaseStructuralCollapse=3
type PhaseResult struct {
    Phase Phase
    Rho   float64  // suppression intensity
    Pi    float64  // healing capacity
}
```

#### Velocity & Age

| Function | Signature | Description |
|----------|-----------|-------------|
| `VelocityOfSilence` | `(alpha, lambda1, meanEdgeLength float64) float64` | Propagation speed |
| `EstimateAge` | `(distance, velocity float64) time.Duration` | Time since anomaly started |

### Laplacian

| Function | Signature | Description |
|----------|-----------|-------------|
| `FiedlerValue` | `(g GraphView, nodeIDs []string, maxIter int, tol float64) float64` | Exact algebraic connectivity (inverse power iteration) |
| `FiedlerApprox` | `(g GraphView, nodeIDs []string) float64` | Fast lower bound (Cheeger inequality) |

---

## `graph/` Package

| Type | Description |
|------|-------------|
| `Graph` | Mutable directed weighted graph |
| `ImmutableGraph` | Copy-on-write immutable graph |
| `UnifiedView` | Base + overlay composite view |
| `NodeData` | Node with ID, Type, Degree, In/OutDegree, FirstSeen, LastSeen |
| `EdgeData` | Edge with From, To, Weight, Type, Count, FirstSeen, LastSeen |

Key methods:
- `New() *Graph`
- `NewImmutable(g *Graph) *ImmutableGraph`
- `NewImmutableEmpty() *ImmutableGraph`
- `NewUnifiedView(base, overlay *ImmutableGraph) *UnifiedView`
- `g.AddNode(n *NodeData)`, `g.AddEdge(from, to string, weight float64, edgeType string, ts time.Time)`
- `ig.WithEvent(src, tgt string, weight float64, edgeType string, ts time.Time) *ImmutableGraph`

All implement `analysis.GraphView` interface.

---

## `mvcc/` Package

| Type | Description |
|------|-------------|
| `Version` | Ref-counted graph version (ID, Graph, Timestamp, Dirty map) |
| `Controller` | Atomic pointer to current version |
| `GC` | Garbage collector for old versions |
| `GCConfig` | Interval, WarningTimeout, ForceTimeout, callbacks |

Key methods:
- `NewController() *Controller`
- `controller.Store(v *Version)`, `controller.Load() *Version`, `controller.Acquire() *Version`
- `NewGC(vc *Controller, cfg GCConfig) *GC`
- `gc.Track(v *Version)`, `gc.Collect() GCStats`

---

## `compact/` Package

| Function | Signature | Description |
|----------|-----------|-------------|
| `Compact` | `(base *graph.ImmutableGraph, overlay *graph.ImmutableGraph) (*graph.ImmutableGraph, Stats)` | Merge overlay into base |

```go
type Stats struct {
    NodesMerged, EdgesMerged int
    OverlayBefore, OverlayAfter int
}
```

---

## `export/` Package

| Function | Signature | Description |
|----------|-----------|-------------|
| `JSON` | `(w io.Writer, g GraphView) error` | JSON export |
| `DOT` | `(w io.Writer, g GraphView) error` | DOT format (Graphviz) |

```go
type GraphView interface {
    ForEachNode(func(*graph.NodeData) bool)
    ForEachEdge(func(*graph.EdgeData) bool)
    NodeCount() int
    EdgeCount() int
}
```
