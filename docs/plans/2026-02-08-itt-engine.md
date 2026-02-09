# ITT Engine - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a modular Go SDK for anomaly detection in graphs using Informational Tension Theory (ITT), with MVCC concurrency, pluggable algorithms, and streaming delta support.

**Architecture:** Clean layered architecture with clear separation: domain types → graph (immutable + MVCC) → analysis (divergence, curvature, topology) → engine (orchestration) → builder (public API). Each layer depends only on the layer below. All algorithms are pluggable via interfaces. MVCC provides lock-free snapshot isolation for concurrent read/write.

**Tech Stack:** Go 1.25+, standard library only (no external deps for core). `math`, `sync/atomic`, `sort`, `context`.

**Module:** `github.com/MatheusGrego/itt-engine`

---

## Package Structure

```
itt-engine/
├── go.mod
├── itt.go                  # Public re-exports, NewBuilder()
├── builder.go              # Builder pattern
├── engine.go               # Engine orchestration
├── snapshot.go             # Snapshot (read-only view)
├── types.go                # Event, Node, Edge, TensionResult, Delta, etc.
├── errors.go               # Sentinel errors
├── callbacks.go            # Callback types
│
├── graph/
│   ├── graph.go            # Core graph data structure (adjacency list)
│   ├── graph_test.go
│   ├── immutable.go        # ImmutableGraph (copy-on-write)
│   ├── immutable_test.go
│   ├── view.go             # UnifiedView (Base + Overlay merge)
│   └── view_test.go
│
├── mvcc/
│   ├── version.go          # GraphVersion + version store
│   ├── version_test.go
│   ├── gc.go               # Garbage collector
│   └── gc_test.go
│
├── analysis/
│   ├── divergence.go       # JSD, KL, Hellinger
│   ├── divergence_test.go
│   ├── tension.go          # Tension calculation
│   ├── tension_test.go
│   ├── curvature.go        # Ollivier-Ricci
│   ├── curvature_test.go
│   ├── calibrator.go       # MAD-based calibration
│   └── calibrator_test.go
│
├── compact/
│   ├── compact.go          # Compaction strategies + merge
│   └── compact_test.go
│
└── export/
    ├── export.go           # JSON, DOT export
    └── export_test.go
```

---

## Phase 1: Foundation — Domain Types + Graph

### Task 1: Project Init + Domain Types

**Files:**
- Create: `go.mod`
- Create: `types.go`
- Create: `errors.go`
- Create: `callbacks.go`
- Test: `types_test.go`

**Step 1: Init Go module**

Run: `go mod init github.com/MatheusGrego/itt-engine`

**Step 2: Write types.go — all domain structs**

```go
package itt

import "time"

// Event is the atomic unit of ingestion.
type Event struct {
    Source    string
    Target   string
    Type     string
    Weight   float64
    Timestamp time.Time
    Metadata  map[string]any
}

// Validate checks Event invariants.
func (e Event) Validate() error {
    if e.Source == "" {
        return ErrEmptySource
    }
    if e.Target == "" {
        return ErrEmptyTarget
    }
    if e.Weight < 0 {
        return ErrNegativeWeight
    }
    return nil
}

// Normalize fills defaults for optional fields.
func (e Event) Normalize() Event {
    if e.Weight == 0 {
        e.Weight = 1.0
    }
    if e.Timestamp.IsZero() {
        e.Timestamp = time.Now()
    }
    return e
}

// Node is a vertex in the information graph.
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

// Edge is a directed weighted edge.
type Edge struct {
    From      string
    To        string
    Weight    float64
    Type      string
    Count     int
    FirstSeen time.Time
    LastSeen  time.Time
}

// TensionResult holds the analysis output for a single node.
type TensionResult struct {
    NodeID     string
    Tension    float64
    Degree     int
    Curvature  float64
    Anomaly    bool
    Confidence float64
    Components map[string]float64
}

// Results holds the full analysis output.
type Results struct {
    Tensions   []TensionResult
    Anomalies  []TensionResult
    Stats      ResultStats
    SnapshotID string
    AnalyzedAt time.Time
    Duration   time.Duration
}

// ResultStats holds aggregate statistics from analysis.
type ResultStats struct {
    NodesAnalyzed int
    MeanTension   float64
    MedianTension float64
    MaxTension    float64
    StdDevTension float64
    AnomalyCount  int
    AnomalyRate   float64
}

// RegionResult holds analysis for a subset of nodes.
type RegionResult struct {
    Nodes       []TensionResult
    MeanTension float64
    MaxTension  float64
    AnomalyCount int
    Aggregated  float64
}

// DeltaType enumerates graph change types.
type DeltaType int

const (
    DeltaNodeAdded DeltaType = iota
    DeltaNodeUpdated
    DeltaNodeRemoved
    DeltaEdgeAdded
    DeltaEdgeUpdated
    DeltaEdgeRemoved
    DeltaTensionChanged
    DeltaAnomalyDetected
    DeltaAnomalyResolved
)

// Delta represents a single graph mutation for streaming.
type Delta struct {
    Type      DeltaType
    Timestamp time.Time
    Version   uint64
    NodeID    string
    Node      *Node
    EdgeFrom  string
    EdgeTo    string
    Edge      *Edge
    Tension   float64
    Previous  float64
    Data      map[string]any
}

// CompactStats holds compaction metrics.
type CompactStats struct {
    NodesMerged   int
    EdgesMerged   int
    OverlayBefore int
    OverlayAfter  int
    Duration      time.Duration
    Timestamp     time.Time
}

// GCStats holds garbage collection metrics.
type GCStats struct {
    VersionsRemoved int
    MemoryFreed     int64
    OldestRemoved   uint64
    Timestamp       time.Time
}

// EngineStats holds runtime engine metrics.
type EngineStats struct {
    Nodes           int
    Edges           int
    OverlayEvents   int
    BaseNodes       int
    BaseEdges       int
    VersionsCurrent uint64
    VersionsTotal   uint64
    SnapshotsActive int
    EventsTotal     int64
    EventsPerSecond float64
    Uptime          time.Duration
}

// GraphData is the serialization format for Storage.
type GraphData struct {
    Nodes     []*Node
    Edges     []*Edge
    Metadata  map[string]any
    Timestamp time.Time
}

// ExportFormat enumerates supported export formats.
type ExportFormat int

const (
    ExportJSON ExportFormat = iota
    ExportDOT
)

// CompactionStrategy enumerates compaction trigger types.
type CompactionStrategy int

const (
    CompactByVolume CompactionStrategy = iota
    CompactByTime
    CompactManual
)
```

**Step 3: Write errors.go**

```go
package itt

import "errors"

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

**Step 4: Write callbacks.go — pluggable function types**

```go
package itt

// DivergenceFunc computes divergence between two probability distributions.
type DivergenceFunc interface {
    Compute(p, q []float64) float64
    Name() string
}

// CurvatureFunc computes edge curvature on a graph.
type CurvatureFunc interface {
    Compute(g GraphView, from, to string) float64
    Name() string
}

// GraphView is a read-only view of a graph for algorithm use.
type GraphView interface {
    GetNode(id string) (*Node, bool)
    GetEdge(from, to string) (*Edge, bool)
    Neighbors(nodeID string) []string
    InNeighbors(nodeID string) []string
    OutNeighbors(nodeID string) []string
}

// TopologyResult holds topological invariants.
type TopologyResult struct {
    Betti0 int
    Betti1 int
}

// TopologyFunc computes topological features.
type TopologyFunc interface {
    Compute(g GraphView) TopologyResult
    Name() string
}

// WeightFunc calculates edge weight from an event.
type WeightFunc func(Event) float64

// NodeTypeFunc extracts a node type from its ID.
type NodeTypeFunc func(nodeID string) string

// ThresholdFunc determines if a node is anomalous.
type ThresholdFunc func(node *Node, tension float64) bool

// AggregationFunc aggregates a slice of tensions into one value.
type AggregationFunc func(tensions []float64) float64

// Logger is an optional structured logger.
type Logger interface {
    Debug(msg string, keysAndValues ...any)
    Info(msg string, keysAndValues ...any)
    Warn(msg string, keysAndValues ...any)
    Error(msg string, keysAndValues ...any)
}

// Storage is an optional persistence interface.
type Storage interface {
    Load() (*GraphData, error)
    Save(data *GraphData) error
}

// Calibrator provides dynamic anomaly threshold calibration.
type Calibrator interface {
    Observe(tension float64)
    IsWarmedUp() bool
    Threshold() float64
    IsAnomaly(tension float64) bool
    Stats() CalibratorStats
    Recalibrate()
}

// CalibratorStats holds calibration state.
type CalibratorStats struct {
    SamplesObserved   int
    Median            float64
    MAD               float64
    Threshold         float64
    K                 float64
    IsWarmedUp        bool
    LastRecalibration time.Time
}

// DistributionPair holds two distributions for batch divergence.
type DistributionPair struct {
    P []float64
    Q []float64
}

// BatchDivergenceFunc extends DivergenceFunc with batch support.
type BatchDivergenceFunc interface {
    DivergenceFunc
    ComputeBatch(pairs []DistributionPair) []float64
    SupportsBatch() bool
}
```

**Step 5: Write failing tests for Event validation**

```go
// types_test.go
package itt

import (
    "testing"
    "time"
)

func TestEventValidation_EmptySource(t *testing.T) {
    e := Event{Source: "", Target: "b"}
    if err := e.Validate(); err != ErrEmptySource {
        t.Fatalf("expected ErrEmptySource, got %v", err)
    }
}

func TestEventValidation_EmptyTarget(t *testing.T) {
    e := Event{Source: "a", Target: ""}
    if err := e.Validate(); err != ErrEmptyTarget {
        t.Fatalf("expected ErrEmptyTarget, got %v", err)
    }
}

func TestEventValidation_NegativeWeight(t *testing.T) {
    e := Event{Source: "a", Target: "b", Weight: -1}
    if err := e.Validate(); err != ErrNegativeWeight {
        t.Fatalf("expected ErrNegativeWeight, got %v", err)
    }
}

func TestEventValidation_Valid(t *testing.T) {
    e := Event{Source: "a", Target: "b", Weight: 1.0}
    if err := e.Validate(); err != nil {
        t.Fatalf("expected nil, got %v", err)
    }
}

func TestEventValidation_ZeroWeightIsValid(t *testing.T) {
    e := Event{Source: "a", Target: "b", Weight: 0}
    if err := e.Validate(); err != nil {
        t.Fatalf("expected nil, got %v", err)
    }
}

func TestEventNormalize_Defaults(t *testing.T) {
    e := Event{Source: "a", Target: "b"}
    n := e.Normalize()
    if n.Weight != 1.0 {
        t.Fatalf("expected weight 1.0, got %f", n.Weight)
    }
    if n.Timestamp.IsZero() {
        t.Fatal("expected non-zero timestamp")
    }
}

func TestEventNormalize_PreservesExplicit(t *testing.T) {
    ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
    e := Event{Source: "a", Target: "b", Weight: 5.0, Timestamp: ts}
    n := e.Normalize()
    if n.Weight != 5.0 {
        t.Fatalf("expected weight 5.0, got %f", n.Weight)
    }
    if !n.Timestamp.Equal(ts) {
        t.Fatalf("expected preserved timestamp")
    }
}
```

**Step 6: Run tests**

Run: `go test ./... -v -count=1`
Expected: ALL PASS

**Step 7: Commit**

```bash
git init
git add -A
git commit -m "feat: project init with domain types, errors, and interfaces"
```

---

### Task 2: Core Graph Data Structure

**Files:**
- Create: `graph/graph.go`
- Test: `graph/graph_test.go`

**Step 1: Write failing tests**

```go
// graph/graph_test.go
package graph

import (
    "testing"
    "time"
)

func TestGraph_AddNodeAndGet(t *testing.T) {
    g := New()
    n := &NodeData{ID: "a", Type: "test"}
    g.AddNode(n)

    got, ok := g.GetNode("a")
    if !ok {
        t.Fatal("expected node found")
    }
    if got.ID != "a" {
        t.Fatalf("expected id 'a', got %q", got.ID)
    }
}

func TestGraph_GetNode_NotFound(t *testing.T) {
    g := New()
    _, ok := g.GetNode("x")
    if ok {
        t.Fatal("expected node not found")
    }
}

func TestGraph_AddEdge(t *testing.T) {
    g := New()
    g.AddNode(&NodeData{ID: "a"})
    g.AddNode(&NodeData{ID: "b"})

    ts := time.Now()
    g.AddEdge("a", "b", 1.5, "tx", ts)

    e, ok := g.GetEdge("a", "b")
    if !ok {
        t.Fatal("expected edge found")
    }
    if e.Weight != 1.5 {
        t.Fatalf("expected weight 1.5, got %f", e.Weight)
    }
    if e.Count != 1 {
        t.Fatalf("expected count 1, got %d", e.Count)
    }
}

func TestGraph_AddEdge_Accumulates(t *testing.T) {
    g := New()
    g.AddNode(&NodeData{ID: "a"})
    g.AddNode(&NodeData{ID: "b"})

    ts := time.Now()
    g.AddEdge("a", "b", 1.0, "tx", ts)
    g.AddEdge("a", "b", 2.0, "tx", ts)

    e, _ := g.GetEdge("a", "b")
    if e.Weight != 3.0 {
        t.Fatalf("expected accumulated weight 3.0, got %f", e.Weight)
    }
    if e.Count != 2 {
        t.Fatalf("expected count 2, got %d", e.Count)
    }
}

func TestGraph_Neighbors(t *testing.T) {
    g := New()
    g.AddNode(&NodeData{ID: "a"})
    g.AddNode(&NodeData{ID: "b"})
    g.AddNode(&NodeData{ID: "c"})

    ts := time.Now()
    g.AddEdge("a", "b", 1, "", ts)
    g.AddEdge("c", "a", 1, "", ts)

    out := g.OutNeighbors("a")
    if len(out) != 1 || out[0] != "b" {
        t.Fatalf("expected out [b], got %v", out)
    }

    in := g.InNeighbors("a")
    if len(in) != 1 || in[0] != "c" {
        t.Fatalf("expected in [c], got %v", in)
    }

    all := g.Neighbors("a")
    if len(all) != 2 {
        t.Fatalf("expected 2 neighbors, got %d", len(all))
    }
}

func TestGraph_NodeCount_EdgeCount(t *testing.T) {
    g := New()
    g.AddNode(&NodeData{ID: "a"})
    g.AddNode(&NodeData{ID: "b"})

    ts := time.Now()
    g.AddEdge("a", "b", 1, "", ts)

    if g.NodeCount() != 2 {
        t.Fatalf("expected 2 nodes, got %d", g.NodeCount())
    }
    if g.EdgeCount() != 1 {
        t.Fatalf("expected 1 edge, got %d", g.EdgeCount())
    }
}

func TestGraph_Degree(t *testing.T) {
    g := New()
    g.AddNode(&NodeData{ID: "a"})
    g.AddNode(&NodeData{ID: "b"})
    g.AddNode(&NodeData{ID: "c"})

    ts := time.Now()
    g.AddEdge("a", "b", 1, "", ts)
    g.AddEdge("a", "c", 1, "", ts)
    g.AddEdge("c", "a", 1, "", ts)

    n, _ := g.GetNode("a")
    if n.OutDegree != 2 {
        t.Fatalf("expected outDegree 2, got %d", n.OutDegree)
    }
    if n.InDegree != 1 {
        t.Fatalf("expected inDegree 1, got %d", n.InDegree)
    }
    if n.Degree != 3 {
        t.Fatalf("expected degree 3, got %d", n.Degree)
    }
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./graph/ -v -count=1`
Expected: FAIL (package doesn't exist yet)

**Step 3: Implement graph.go**

```go
// graph/graph.go
package graph

import "time"

// NodeData holds mutable node state inside the graph.
type NodeData struct {
    ID         string
    Type       string
    Degree     int
    InDegree   int
    OutDegree  int
    Attributes map[string]float64
    FirstSeen  time.Time
    LastSeen   time.Time
}

// EdgeData holds mutable edge state inside the graph.
type EdgeData struct {
    From      string
    To        string
    Weight    float64
    Type      string
    Count     int
    FirstSeen time.Time
    LastSeen  time.Time
}

// edgeKey returns a canonical key for a directed edge.
func edgeKey(from, to string) string {
    return from + "\x00" + to
}

// Graph is a mutable directed weighted graph using adjacency lists.
type Graph struct {
    nodes map[string]*NodeData
    edges map[string]*EdgeData
    out   map[string]map[string]bool // nodeID -> set of outgoing neighbor IDs
    in    map[string]map[string]bool // nodeID -> set of incoming neighbor IDs
}

// New creates an empty graph.
func New() *Graph {
    return &Graph{
        nodes: make(map[string]*NodeData),
        edges: make(map[string]*EdgeData),
        out:   make(map[string]map[string]bool),
        in:    make(map[string]map[string]bool),
    }
}

// AddNode adds or updates a node.
func (g *Graph) AddNode(n *NodeData) {
    if existing, ok := g.nodes[n.ID]; ok {
        existing.Type = n.Type
        if !n.LastSeen.IsZero() {
            existing.LastSeen = n.LastSeen
        }
        return
    }
    g.nodes[n.ID] = n
    if g.out[n.ID] == nil {
        g.out[n.ID] = make(map[string]bool)
    }
    if g.in[n.ID] == nil {
        g.in[n.ID] = make(map[string]bool)
    }
}

// GetNode returns a node by ID.
func (g *Graph) GetNode(id string) (*NodeData, bool) {
    n, ok := g.nodes[id]
    return n, ok
}

// AddEdge adds or accumulates an edge. Creates nodes if missing.
func (g *Graph) AddEdge(from, to string, weight float64, edgeType string, ts time.Time) {
    // Ensure nodes exist
    if _, ok := g.nodes[from]; !ok {
        g.AddNode(&NodeData{ID: from, FirstSeen: ts, LastSeen: ts})
    }
    if _, ok := g.nodes[to]; !ok {
        g.AddNode(&NodeData{ID: to, FirstSeen: ts, LastSeen: ts})
    }

    key := edgeKey(from, to)
    if e, ok := g.edges[key]; ok {
        e.Weight += weight
        e.Count++
        if ts.After(e.LastSeen) {
            e.LastSeen = ts
        }
        if ts.Before(e.FirstSeen) {
            e.FirstSeen = ts
        }
        return
    }

    g.edges[key] = &EdgeData{
        From:      from,
        To:        to,
        Weight:    weight,
        Type:      edgeType,
        Count:     1,
        FirstSeen: ts,
        LastSeen:  ts,
    }

    // Update adjacency
    g.out[from][to] = true
    g.in[to][from] = true

    // Update degrees
    g.nodes[from].OutDegree++
    g.nodes[from].Degree++
    g.nodes[to].InDegree++
    g.nodes[to].Degree++
}

// GetEdge returns an edge by endpoints.
func (g *Graph) GetEdge(from, to string) (*EdgeData, bool) {
    e, ok := g.edges[edgeKey(from, to)]
    return e, ok
}

// Neighbors returns all neighbors (in + out), deduplicated.
func (g *Graph) Neighbors(nodeID string) []string {
    seen := make(map[string]bool)
    var result []string
    for id := range g.out[nodeID] {
        if !seen[id] {
            seen[id] = true
            result = append(result, id)
        }
    }
    for id := range g.in[nodeID] {
        if !seen[id] {
            seen[id] = true
            result = append(result, id)
        }
    }
    return result
}

// OutNeighbors returns nodes pointed to by nodeID.
func (g *Graph) OutNeighbors(nodeID string) []string {
    var result []string
    for id := range g.out[nodeID] {
        result = append(result, id)
    }
    return result
}

// InNeighbors returns nodes pointing to nodeID.
func (g *Graph) InNeighbors(nodeID string) []string {
    var result []string
    for id := range g.in[nodeID] {
        result = append(result, id)
    }
    return result
}

// NodeCount returns total number of nodes.
func (g *Graph) NodeCount() int { return len(g.nodes) }

// EdgeCount returns total number of unique edges.
func (g *Graph) EdgeCount() int { return len(g.edges) }

// ForEachNode iterates nodes. Return false to stop.
func (g *Graph) ForEachNode(fn func(*NodeData) bool) {
    for _, n := range g.nodes {
        if !fn(n) {
            return
        }
    }
}

// ForEachEdge iterates edges. Return false to stop.
func (g *Graph) ForEachEdge(fn func(*EdgeData) bool) {
    for _, e := range g.edges {
        if !fn(e) {
            return
        }
    }
}
```

**Step 4: Run tests**

Run: `go test ./graph/ -v -count=1`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add graph/
git commit -m "feat: core graph data structure with adjacency lists"
```

---

### Task 3: Immutable Graph (Copy-on-Write)

**Files:**
- Create: `graph/immutable.go`
- Test: `graph/immutable_test.go`

**Step 1: Write failing tests**

```go
// graph/immutable_test.go
package graph

import (
    "testing"
    "time"
)

func TestImmutableGraph_FromGraph(t *testing.T) {
    g := New()
    g.AddNode(&NodeData{ID: "a"})
    g.AddNode(&NodeData{ID: "b"})
    g.AddEdge("a", "b", 1.0, "tx", time.Now())

    ig := NewImmutable(g)
    if ig.NodeCount() != 2 {
        t.Fatalf("expected 2 nodes, got %d", ig.NodeCount())
    }
    if ig.EdgeCount() != 1 {
        t.Fatalf("expected 1 edge, got %d", ig.EdgeCount())
    }
}

func TestImmutableGraph_DeepCopy(t *testing.T) {
    g := New()
    g.AddNode(&NodeData{ID: "a", Attributes: map[string]float64{"x": 1.0}})

    ig := NewImmutable(g)

    // Mutate original — must not affect immutable
    g.AddNode(&NodeData{ID: "c"})
    n, _ := g.GetNode("a")
    n.Attributes["x"] = 999

    if ig.NodeCount() != 1 {
        t.Fatalf("immutable should have 1 node, got %d", ig.NodeCount())
    }
    igNode, _ := ig.GetNode("a")
    if igNode.Attributes["x"] != 1.0 {
        t.Fatalf("immutable attribute mutated: got %f", igNode.Attributes["x"])
    }
}

func TestImmutableGraph_WithEvent(t *testing.T) {
    g := New()
    g.AddNode(&NodeData{ID: "a"})
    g.AddNode(&NodeData{ID: "b"})
    g.AddEdge("a", "b", 1.0, "tx", time.Now())

    ig := NewImmutable(g)
    ts := time.Now()
    ig2 := ig.WithEvent("b", "c", 2.0, "tx", ts)

    // Original unchanged
    if ig.NodeCount() != 2 {
        t.Fatalf("original should have 2 nodes, got %d", ig.NodeCount())
    }
    _, ok := ig.GetNode("c")
    if ok {
        t.Fatal("original should not have node c")
    }

    // New version has the update
    if ig2.NodeCount() != 3 {
        t.Fatalf("new should have 3 nodes, got %d", ig2.NodeCount())
    }
    _, ok = ig2.GetNode("c")
    if !ok {
        t.Fatal("new should have node c")
    }
}

func TestImmutableGraph_GraphView(t *testing.T) {
    g := New()
    g.AddNode(&NodeData{ID: "a"})
    g.AddNode(&NodeData{ID: "b"})
    g.AddEdge("a", "b", 1.0, "tx", time.Now())

    ig := NewImmutable(g)

    // ImmutableGraph should satisfy GraphView-like reads
    n, ok := ig.GetNode("a")
    if !ok || n.ID != "a" {
        t.Fatal("GetNode failed")
    }
    e, ok := ig.GetEdge("a", "b")
    if !ok || e.Weight != 1.0 {
        t.Fatal("GetEdge failed")
    }
    neighbors := ig.Neighbors("a")
    if len(neighbors) != 1 {
        t.Fatalf("expected 1 neighbor, got %d", len(neighbors))
    }
}
```

**Step 2: Run to verify failure**

Run: `go test ./graph/ -v -count=1 -run Immutable`
Expected: FAIL

**Step 3: Implement immutable.go**

```go
// graph/immutable.go
package graph

import "time"

// ImmutableGraph is a read-only, deep-copied snapshot of a Graph.
// Creating a new version with modifications returns a new ImmutableGraph
// without mutating the original (copy-on-write).
type ImmutableGraph struct {
    inner *Graph
}

// NewImmutable deep-copies a mutable Graph into an immutable one.
func NewImmutable(src *Graph) *ImmutableGraph {
    return &ImmutableGraph{inner: deepCopyGraph(src)}
}

// NewImmutableEmpty creates an empty immutable graph.
func NewImmutableEmpty() *ImmutableGraph {
    return &ImmutableGraph{inner: New()}
}

// WithEvent returns a NEW ImmutableGraph with the event applied.
// The receiver is not modified.
func (ig *ImmutableGraph) WithEvent(from, to string, weight float64, edgeType string, ts time.Time) *ImmutableGraph {
    cp := deepCopyGraph(ig.inner)
    cp.AddEdge(from, to, weight, edgeType, ts)

    // Update node timestamps
    if n, ok := cp.GetNode(from); ok {
        if ts.Before(n.FirstSeen) || n.FirstSeen.IsZero() {
            n.FirstSeen = ts
        }
        if ts.After(n.LastSeen) {
            n.LastSeen = ts
        }
    }
    if n, ok := cp.GetNode(to); ok {
        if ts.Before(n.FirstSeen) || n.FirstSeen.IsZero() {
            n.FirstSeen = ts
        }
        if ts.After(n.LastSeen) {
            n.LastSeen = ts
        }
    }

    return &ImmutableGraph{inner: cp}
}

// GetNode returns a node by ID.
func (ig *ImmutableGraph) GetNode(id string) (*NodeData, bool) {
    return ig.inner.GetNode(id)
}

// GetEdge returns an edge by endpoints.
func (ig *ImmutableGraph) GetEdge(from, to string) (*EdgeData, bool) {
    return ig.inner.GetEdge(from, to)
}

// Neighbors returns all neighbor IDs.
func (ig *ImmutableGraph) Neighbors(nodeID string) []string {
    return ig.inner.Neighbors(nodeID)
}

// OutNeighbors returns outgoing neighbor IDs.
func (ig *ImmutableGraph) OutNeighbors(nodeID string) []string {
    return ig.inner.OutNeighbors(nodeID)
}

// InNeighbors returns incoming neighbor IDs.
func (ig *ImmutableGraph) InNeighbors(nodeID string) []string {
    return ig.inner.InNeighbors(nodeID)
}

// NodeCount returns node count.
func (ig *ImmutableGraph) NodeCount() int { return ig.inner.NodeCount() }

// EdgeCount returns edge count.
func (ig *ImmutableGraph) EdgeCount() int { return ig.inner.EdgeCount() }

// ForEachNode iterates nodes. Return false to stop.
func (ig *ImmutableGraph) ForEachNode(fn func(*NodeData) bool) { ig.inner.ForEachNode(fn) }

// ForEachEdge iterates edges. Return false to stop.
func (ig *ImmutableGraph) ForEachEdge(fn func(*EdgeData) bool) { ig.inner.ForEachEdge(fn) }

// deepCopyGraph creates a fully independent copy of a Graph.
func deepCopyGraph(src *Graph) *Graph {
    dst := New()

    src.ForEachNode(func(n *NodeData) bool {
        cp := &NodeData{
            ID:        n.ID,
            Type:      n.Type,
            Degree:    n.Degree,
            InDegree:  n.InDegree,
            OutDegree: n.OutDegree,
            FirstSeen: n.FirstSeen,
            LastSeen:  n.LastSeen,
        }
        if n.Attributes != nil {
            cp.Attributes = make(map[string]float64, len(n.Attributes))
            for k, v := range n.Attributes {
                cp.Attributes[k] = v
            }
        }
        dst.nodes[cp.ID] = cp
        if dst.out[cp.ID] == nil {
            dst.out[cp.ID] = make(map[string]bool)
        }
        if dst.in[cp.ID] == nil {
            dst.in[cp.ID] = make(map[string]bool)
        }
        return true
    })

    src.ForEachEdge(func(e *EdgeData) bool {
        cp := &EdgeData{
            From:      e.From,
            To:        e.To,
            Weight:    e.Weight,
            Type:      e.Type,
            Count:     e.Count,
            FirstSeen: e.FirstSeen,
            LastSeen:  e.LastSeen,
        }
        dst.edges[edgeKey(cp.From, cp.To)] = cp
        dst.out[cp.From][cp.To] = true
        dst.in[cp.To][cp.From] = true
        return true
    })

    return dst
}
```

**Step 4: Run tests**

Run: `go test ./graph/ -v -count=1`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add graph/immutable.go graph/immutable_test.go
git commit -m "feat: immutable graph with copy-on-write semantics"
```

---

### Task 4: MVCC Version Controller

**Files:**
- Create: `mvcc/version.go`
- Test: `mvcc/version_test.go`

**Step 1: Write failing tests**

```go
// mvcc/version_test.go
package mvcc

import (
    "testing"
    "time"

    "github.com/MatheusGrego/itt-engine/graph"
)

func TestVersion_StoreAndLoad(t *testing.T) {
    vc := NewController()
    ig := graph.NewImmutableEmpty()
    v := &Version{
        ID:        1,
        Graph:     ig,
        Timestamp: time.Now(),
    }
    vc.Store(v)

    got := vc.Load()
    if got.ID != 1 {
        t.Fatalf("expected version 1, got %d", got.ID)
    }
}

func TestVersion_Acquire_Release(t *testing.T) {
    vc := NewController()
    ig := graph.NewImmutableEmpty()
    v := &Version{ID: 1, Graph: ig, Timestamp: time.Now()}
    vc.Store(v)

    got := vc.Acquire()
    if got.RefCount() != 1 {
        t.Fatalf("expected refcount 1, got %d", got.RefCount())
    }

    got.Release()
    if got.RefCount() != 0 {
        t.Fatalf("expected refcount 0, got %d", got.RefCount())
    }
}

func TestVersion_Acquire_MultipleSnapshots(t *testing.T) {
    vc := NewController()
    ig := graph.NewImmutableEmpty()
    v := &Version{ID: 1, Graph: ig, Timestamp: time.Now()}
    vc.Store(v)

    a := vc.Acquire()
    b := vc.Acquire()

    if a.RefCount() != 2 {
        t.Fatalf("expected refcount 2, got %d", a.RefCount())
    }

    a.Release()
    if b.RefCount() != 1 {
        t.Fatalf("expected refcount 1 after one release, got %d", b.RefCount())
    }

    b.Release()
    if a.RefCount() != 0 {
        t.Fatalf("expected refcount 0, got %d", a.RefCount())
    }
}

func TestVersion_ReleaseIdempotent(t *testing.T) {
    vc := NewController()
    ig := graph.NewImmutableEmpty()
    v := &Version{ID: 1, Graph: ig, Timestamp: time.Now()}
    vc.Store(v)

    snap := vc.Acquire()
    snap.Release()
    snap.Release() // should not panic or go negative
    if snap.RefCount() != 0 {
        t.Fatalf("expected refcount 0, got %d", snap.RefCount())
    }
}

func TestVersion_SnapshotIsolation(t *testing.T) {
    vc := NewController()
    g1 := graph.New()
    g1.AddNode(&graph.NodeData{ID: "a"})
    ig1 := graph.NewImmutable(g1)
    vc.Store(&Version{ID: 1, Graph: ig1, Timestamp: time.Now()})

    snap := vc.Acquire()

    // New version
    g2 := graph.New()
    g2.AddNode(&graph.NodeData{ID: "a"})
    g2.AddNode(&graph.NodeData{ID: "b"})
    ig2 := graph.NewImmutable(g2)
    vc.Store(&Version{ID: 2, Graph: ig2, Timestamp: time.Now()})

    // Snapshot still sees old version
    if snap.Graph.NodeCount() != 1 {
        t.Fatalf("snapshot should see 1 node, got %d", snap.Graph.NodeCount())
    }

    // Current sees new version
    current := vc.Load()
    if current.Graph.NodeCount() != 2 {
        t.Fatalf("current should see 2 nodes, got %d", current.Graph.NodeCount())
    }

    snap.Release()
}
```

**Step 2: Run to verify failure**

Run: `go test ./mvcc/ -v -count=1`
Expected: FAIL

**Step 3: Implement version.go**

```go
// mvcc/version.go
package mvcc

import (
    "sync/atomic"
    "time"
    "unsafe"

    "github.com/MatheusGrego/itt-engine/graph"
)

// Version is an immutable snapshot of graph state.
type Version struct {
    ID        uint64
    Graph     *graph.ImmutableGraph
    Timestamp time.Time
    Dirty     map[string]bool
    refCount  atomic.Int64
}

// RefCount returns the current number of active references.
func (v *Version) RefCount() int64 {
    return v.refCount.Load()
}

// Acquire increments the reference count.
func (v *Version) Acquire() {
    v.refCount.Add(1)
}

// Release decrements the reference count. Idempotent at zero.
func (v *Version) Release() {
    for {
        cur := v.refCount.Load()
        if cur <= 0 {
            return
        }
        if v.refCount.CompareAndSwap(cur, cur-1) {
            return
        }
    }
}

// Controller manages the current graph version using atomic pointer swap.
type Controller struct {
    current unsafe.Pointer // *Version
}

// NewController creates a new MVCC controller.
func NewController() *Controller {
    return &Controller{}
}

// Store atomically replaces the current version.
func (c *Controller) Store(v *Version) {
    atomic.StorePointer(&c.current, unsafe.Pointer(v))
}

// Load returns the current version without incrementing refcount.
func (c *Controller) Load() *Version {
    p := atomic.LoadPointer(&c.current)
    if p == nil {
        return nil
    }
    return (*Version)(p)
}

// Acquire atomically loads the current version and increments its refcount.
// Returns the acquired version.
func (c *Controller) Acquire() *Version {
    v := c.Load()
    if v != nil {
        v.Acquire()
    }
    return v
}
```

**Step 4: Run tests**

Run: `go test ./mvcc/ -v -count=1`
Expected: ALL PASS

**Step 5: Run race detector**

Run: `go test ./mvcc/ -race -count=1`
Expected: PASS with no races

**Step 6: Commit**

```bash
git add mvcc/
git commit -m "feat: MVCC version controller with atomic refcounting"
```

---

## Phase 2: Engine Core

### Task 5: Builder Pattern

**Files:**
- Create: `builder.go`
- Create: `itt.go`
- Test: `builder_test.go`

**Step 1: Write failing tests**

```go
// builder_test.go
package itt

import "testing"

func TestBuilder_DefaultsBuild(t *testing.T) {
    e, err := NewBuilder().Build()
    if err != nil {
        t.Fatalf("default build should not fail: %v", err)
    }
    if e == nil {
        t.Fatal("engine should not be nil")
    }
}

func TestBuilder_NegativeThreshold(t *testing.T) {
    _, err := NewBuilder().Threshold(-1).Build()
    if err == nil {
        t.Fatal("expected error for negative threshold")
    }
}

func TestBuilder_Chaining(t *testing.T) {
    _, err := NewBuilder().
        Threshold(0.3).
        GCSnapshotWarning(5 * 60e9). // 5 min in ns
        Build()
    if err != nil {
        t.Fatalf("chaining should work: %v", err)
    }
}
```

**Step 2: Implement builder.go and itt.go**

`itt.go`:
```go
package itt

// NewBuilder creates a new engine builder with sensible defaults.
func NewBuilder() *Builder {
    return &Builder{
        threshold:            0.2,
        gcSnapshotWarning:    5 * 60e9,   // 5 min
        gcSnapshotForce:      15 * 60e9,  // 15 min
        maxOverlaySize:       100000,
        compactionStrategy:   CompactByVolume,
        compactionThreshold:  10000,
        channelSize:          10000,
    }
}
```

`builder.go` — full builder with all documented methods, validation in Build(), returns `*engine` (initially a stub struct).

**Step 3: Run tests**

Run: `go test ./ -v -count=1 -run Builder`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add itt.go builder.go builder_test.go
git commit -m "feat: builder pattern with validation and sensible defaults"
```

---

### Task 6: Engine Lifecycle + Ingestion

**Files:**
- Create: `engine.go`
- Test: `engine_test.go`

**Step 1: Write failing tests for lifecycle**

Tests for Start/Stop/Running, auto-start on AddEvent, AddEvent validation, concurrent AddEvent with -race.

**Step 2: Implement engine.go**

Engine struct holds: mvcc.Controller, config from builder, write channel, context/cancel, started atomic bool. Worker goroutine reads from channel, applies events to immutable graph, stores new version.

**Step 3: Run tests**

Run: `go test ./ -v -race -count=1`

**Step 4: Commit**

```bash
git add engine.go engine_test.go
git commit -m "feat: engine lifecycle, ingestion, and MVCC write path"
```

---

### Task 7: Snapshot

**Files:**
- Create: `snapshot.go`
- Test: `snapshot_test.go`

**Step 1: Write failing tests**

Tests for: snapshot captures immutable state, Close() makes ops return error, snapshot isolation (add events after snapshot don't appear), Close() is idempotent.

**Step 2: Implement snapshot.go**

Snapshot wraps a *mvcc.Version, exposes read methods (GetNode, GetEdge, Neighbors, ForEachNode, ForEachEdge, NodeCount, EdgeCount). Close() releases refcount and sets closed flag.

**Step 3: Run tests**

Run: `go test ./ -v -race -count=1 -run Snapshot`

**Step 4: Commit**

```bash
git add snapshot.go snapshot_test.go
git commit -m "feat: snapshot with MVCC isolation and ref counting"
```

---

## Phase 3: Analysis Algorithms

### Task 8: Divergence Functions (JSD, KL, Hellinger)

**Files:**
- Create: `analysis/divergence.go`
- Test: `analysis/divergence_test.go`

**Step 1: Write failing tests**

```go
// analysis/divergence_test.go
package analysis

import (
    "math"
    "testing"
)

func TestJSD_IdenticalDistributions(t *testing.T) {
    p := []float64{0.25, 0.25, 0.25, 0.25}
    d := JSD{}
    result := d.Compute(p, p)
    if result > 1e-10 {
        t.Fatalf("JSD(p,p) should be 0, got %f", result)
    }
}

func TestJSD_Symmetric(t *testing.T) {
    p := []float64{0.1, 0.4, 0.5}
    q := []float64{0.3, 0.3, 0.4}
    d := JSD{}
    pq := d.Compute(p, q)
    qp := d.Compute(q, p)
    if math.Abs(pq-qp) > 1e-10 {
        t.Fatalf("JSD should be symmetric: %f != %f", pq, qp)
    }
}

func TestJSD_Bounded(t *testing.T) {
    p := []float64{1.0, 0.0, 0.0}
    q := []float64{0.0, 0.0, 1.0}
    d := JSD{}
    result := d.Compute(p, q)
    if result < 0 || result > math.Log2(2)+1e-10 {
        t.Fatalf("JSD should be in [0, log2]: got %f", result)
    }
}

func TestKL_IdenticalDistributions(t *testing.T) {
    p := []float64{0.25, 0.25, 0.25, 0.25}
    d := KL{}
    result := d.Compute(p, p)
    if result > 1e-10 {
        t.Fatalf("KL(p,p) should be 0, got %f", result)
    }
}

func TestKL_NonNegative(t *testing.T) {
    p := []float64{0.1, 0.4, 0.5}
    q := []float64{0.3, 0.3, 0.4}
    d := KL{}
    result := d.Compute(p, q)
    if result < -1e-10 {
        t.Fatalf("KL should be >= 0, got %f", result)
    }
}

func TestHellinger_IdenticalDistributions(t *testing.T) {
    p := []float64{0.25, 0.25, 0.25, 0.25}
    d := Hellinger{}
    result := d.Compute(p, p)
    if result > 1e-10 {
        t.Fatalf("Hellinger(p,p) should be 0, got %f", result)
    }
}

func TestHellinger_Bounded(t *testing.T) {
    p := []float64{1.0, 0.0}
    q := []float64{0.0, 1.0}
    d := Hellinger{}
    result := d.Compute(p, q)
    if result < 0 || result > 1+1e-10 {
        t.Fatalf("Hellinger should be in [0, 1]: got %f", result)
    }
}

func TestDivergence_WithZeros(t *testing.T) {
    p := []float64{0.5, 0.5, 0.0}
    q := []float64{0.0, 0.5, 0.5}
    for _, d := range []DivergenceFunc{JSD{}, KL{}, Hellinger{}} {
        result := d.Compute(p, q)
        if math.IsNaN(result) || math.IsInf(result, 0) {
            t.Fatalf("%s produced NaN/Inf with zeros", d.Name())
        }
    }
}
```

**Step 2: Implement divergence.go**

Note: The `DivergenceFunc` interface is already in `callbacks.go` at root. Import it here. The analysis package will use its own concrete types that satisfy the interface.

```go
// analysis/divergence.go
package analysis

import "math"

const epsilon = 1e-12

// DivergenceFunc matches the interface from parent package.
type DivergenceFunc interface {
    Compute(p, q []float64) float64
    Name() string
}

// JSD implements Jensen-Shannon Divergence.
type JSD struct{}

func (JSD) Name() string { return "jsd" }

func (JSD) Compute(p, q []float64) float64 {
    m := make([]float64, len(p))
    for i := range p {
        m[i] = 0.5*p[i] + 0.5*q[i]
    }
    return 0.5*klDiv(p, m) + 0.5*klDiv(q, m)
}

// KL implements Kullback-Leibler Divergence.
type KL struct{}

func (KL) Name() string { return "kl" }

func (KL) Compute(p, q []float64) float64 {
    return klDiv(p, q)
}

// Hellinger implements Hellinger Distance.
type Hellinger struct{}

func (Hellinger) Name() string { return "hellinger" }

func (Hellinger) Compute(p, q []float64) float64 {
    sum := 0.0
    for i := range p {
        diff := math.Sqrt(p[i]) - math.Sqrt(q[i])
        sum += diff * diff
    }
    return math.Sqrt(sum / 2.0)
}

func klDiv(p, q []float64) float64 {
    sum := 0.0
    for i := range p {
        pi := p[i] + epsilon
        qi := q[i] + epsilon
        if pi > epsilon {
            sum += pi * math.Log2(pi/qi)
        }
    }
    return sum
}
```

**Step 3: Run tests**

Run: `go test ./analysis/ -v -count=1`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add analysis/
git commit -m "feat: divergence functions — JSD, KL, Hellinger"
```

---

### Task 9: Tension Calculation

**Files:**
- Create: `analysis/tension.go`
- Test: `analysis/tension_test.go`

**Step 1: Write failing tests**

Tests for: isolated node → tension 0, single-edge node, hub node has higher tension than leaf, result always in [0, 1].

**Step 2: Implement tension.go**

Algorithm per spec:
1. For each neighbor of v, build weight distribution of their edges
2. Simulate removal of v: zero edges connected to v
3. Compute divergence between original and perturbed distributions
4. τ(v) = mean of neighbor divergences, normalized to [0, 1]

The TensionCalculator takes a DivergenceFunc and a GraphView (the immutable graph satisfies this).

**Step 3: Run tests**

Run: `go test ./analysis/ -v -count=1 -run Tension`

**Step 4: Commit**

```bash
git add analysis/tension.go analysis/tension_test.go
git commit -m "feat: tension calculation with pluggable divergence"
```

---

### Task 10: MAD Calibrator

**Files:**
- Create: `analysis/calibrator.go`
- Test: `analysis/calibrator_test.go`

**Step 1: Write failing tests**

Tests for: not warmed up initially, becomes warmed up after N observations, threshold = median + K*MAD, IsAnomaly returns true above threshold, Recalibrate resets, precomputed baseline.

**Step 2: Implement calibrator.go**

MADCalibrator stores observations in a ring buffer, computes median and MAD on Recalibrate or when warm-up completes. Formula: threshold = median + K * MAD.

**Step 3: Run tests**

Run: `go test ./analysis/ -v -count=1 -run Calibrator`

**Step 4: Commit**

```bash
git add analysis/calibrator.go analysis/calibrator_test.go
git commit -m "feat: MAD-based calibrator with warm-up protocol"
```

---

### Task 11: Ollivier-Ricci Curvature

**Files:**
- Create: `analysis/curvature.go`
- Test: `analysis/curvature_test.go`

**Step 1: Write failing tests**

Tests for: complete graph edge → positive curvature, bridge edge → negative curvature, self-consistency.

**Step 2: Implement curvature.go**

Ollivier-Ricci: κ(x,y) = 1 - W(μ_x, μ_y) / d(x,y). Uses Sinkhorn approximation for Wasserstein distance. μ_x is the uniform distribution over neighbors of x.

**Step 3: Run tests**

Run: `go test ./analysis/ -v -count=1 -run Curvature`

**Step 4: Commit**

```bash
git add analysis/curvature.go analysis/curvature_test.go
git commit -m "feat: Ollivier-Ricci curvature with Wasserstein approx"
```

---

### Task 11.5: Concealment Cost + Yharim Limit

**Files:**
- Modify: `analysis/tension.go` — add ConcealmentCost function
- Create: `analysis/yharim.go` — Yharim detectability limit
- Test: `analysis/tension_test.go` — concealment cost tests
- Test: `analysis/yharim_test.go`

**Theory Reference (from paper):**
- Concealment Cost: Ω(Ns) = Σₖ Σᵥᵢ τ(vᵢ) · ω(k), where ω(k) = e^(-λk)
- Yharim Limit: Υ = √(2·log(1/α)), fundamental detectability threshold
- SNR(Ns) = (d_Ns / σ_d) · √C_Ns; detectable iff SNR > Υ

**Step 1: Write failing tests**

```go
func TestConcealmentCost_HighDegreeNode(t *testing.T) {
    // Hub node should have high concealment cost
    // Ω scales superlinearly with degree
}

func TestConcealmentCost_ExponentialDecay(t *testing.T) {
    // Tension contribution decays with topological distance
    // Layer 1 contributes more than layer 2
}

func TestYharimLimit_StandardAlpha(t *testing.T) {
    // α=0.05 → Υ ≈ 2.45
    // α=0.01 → Υ ≈ 3.03
    // α=0.001 → Υ ≈ 3.72
}

func TestYharimLimit_SniperEffect(t *testing.T) {
    // Low-degree node with high-degree neighbors
    // should be detectable via adjacency layer divergence
}
```

**Step 2: Implement concealment cost and Yharim limit**

ConcealmentCost walks k-hop neighborhoods, sums τ(vᵢ)·exp(-λk).
YharimLimit computes √(2·log(1/α)) for a given false positive rate.
IsDetectable checks SNR > Υ.

**Step 3: Run tests**

Run: `go test ./analysis/ -v -count=1 -run "Concealment|Yharim"`

**Step 4: Commit**

```bash
git add analysis/tension.go analysis/yharim.go analysis/yharim_test.go analysis/tension_test.go
git commit -m "feat: concealment cost and Yharim detectability limit"
```

---

### Task 12: Analysis Integration into Snapshot

**Files:**
- Modify: `snapshot.go` — add Analyze(), AnalyzeNode(), AnalyzeRegion()
- Modify: `engine.go` — add Analyze() convenience
- Test: `snapshot_test.go` — add analysis tests
- Test: `engine_test.go` — add analysis integration tests

**Step 1: Write failing tests**

Tests for: Analyze returns results for all nodes, Anomalies filtered by threshold, AnalyzeNode for specific node, AnalyzeNode for missing node → error, AnalyzeRegion with stats.

**Step 2: Wire tension calculator + calibrator into snapshot**

Snapshot.Analyze() iterates all nodes, computes tension via TensionCalculator, feeds calibrator, classifies anomalies.

**Step 3: Run tests**

Run: `go test ./ -v -race -count=1`

**Step 4: Commit**

```bash
git add snapshot.go engine.go snapshot_test.go engine_test.go
git commit -m "feat: analysis integration — Analyze, AnalyzeNode, AnalyzeRegion"
```

---

## Phase 4: MVCC Advanced + Compaction

### Task 13: Garbage Collector

**Files:**
- Create: `mvcc/gc.go`
- Test: `mvcc/gc_test.go`

**Step 1: Write failing tests**

Tests for: orphan versions removed, versions with active snapshots preserved, timeout warning, force close after timeout.

**Step 2: Implement gc.go**

GC runs as a goroutine, periodically checks version list, removes versions with RefCount=0 that aren't current. Tracks snapshot age for timeout warnings.

**Step 3: Run tests**

Run: `go test ./mvcc/ -v -race -count=1`

**Step 4: Commit**

```bash
git add mvcc/gc.go mvcc/gc_test.go
git commit -m "feat: MVCC garbage collector with timeout safety"
```

---

### Task 14: Unified View (Base + Overlay) + Compaction

**Files:**
- Create: `graph/view.go`
- Test: `graph/view_test.go`
- Create: `compact/compact.go`
- Test: `compact/compact_test.go`

**Step 1: Write failing tests for view**

Tests for: node only in base → found, node only in overlay → found, node in both → overlay wins, edge in both → weights summed, iteration covers both without duplicates.

**Step 2: Implement view.go**

UnifiedView wraps two ImmutableGraphs (base + overlay), presents merged read interface.

**Step 3: Write failing tests for compaction**

Tests for: compact merges overlay into base, overlay is reset after, existing snapshots still valid.

**Step 4: Implement compact.go**

Strategies: ByVolume (threshold event count), ByTime, Manual. Merge creates new base graph from union.

**Step 5: Run tests**

Run: `go test ./graph/ ./compact/ -v -race -count=1`

**Step 6: Commit**

```bash
git add graph/view.go graph/view_test.go compact/
git commit -m "feat: unified view (Base+Overlay) and compaction"
```

---

## Phase 5: Callbacks + Export

### Task 15: Callback System

**Files:**
- Modify: `engine.go` — wire callbacks into worker loop
- Test: `engine_test.go` — callback tests

**Step 1: Write failing tests**

Tests for: OnChange called per mutation with correct Delta, OnAnomaly called when tension > threshold, panic in callback is recovered, OnCompact called after compaction, OnError called on recoverable error.

**Step 2: Wire callbacks into engine worker**

Worker loop emits deltas to OnChange after each version. Analysis results trigger OnAnomaly. Panic recovery wraps all callback invocations.

**Step 3: Run tests**

Run: `go test ./ -v -race -count=1 -run Callback`

**Step 4: Commit**

```bash
git add engine.go engine_test.go
git commit -m "feat: callback system with panic recovery"
```

---

### Task 16: Export (JSON, DOT)

**Files:**
- Create: `export/export.go`
- Test: `export/export_test.go`

**Step 1: Write failing tests**

Tests for: ExportJSON produces valid JSON with all nodes/edges, ExportDOT produces valid DOT syntax.

**Step 2: Implement export.go**

JSON: marshal graph to `{nodes: [...], edges: [...]}`. DOT: generate `digraph { ... }` with labels.

**Step 3: Run tests**

Run: `go test ./export/ -v -count=1`

**Step 4: Commit**

```bash
git add export/
git commit -m "feat: JSON and DOT graph export"
```

---

## Phase 6: Polish

### Task 17: Integration Tests

**Files:**
- Create: `integration_test.go`

Tests:
- Streaming scenario: 10k events across 10 goroutines, verify all processed, no races
- Analysis during ingestion: concurrent AddEvent + Snapshot + Analyze
- Compaction under load: trigger multiple compactions, verify correctness

Run: `go test ./ -v -race -count=1 -run Integration -timeout 60s`

**Step 1: Commit**

```bash
git add integration_test.go
git commit -m "test: integration tests for streaming, concurrency, compaction"
```

---

### Task 18: Benchmarks

**Files:**
- Create: `benchmark_test.go`

Benchmarks: BenchmarkAddEvent, BenchmarkSnapshot, BenchmarkAnalyze1k, BenchmarkAnalyze10k, BenchmarkConcurrentAddEvent.

Targets: ingestion >= 15k events/s, snapshot < 1μs, analysis 1k nodes < 100ms.

Run: `go test ./ -bench=. -benchmem -count=3`

**Step 1: Commit**

```bash
git add benchmark_test.go
git commit -m "bench: performance benchmarks for ingestion, snapshot, analysis"
```

---

## Summary

| Phase | Tasks | Components |
|-------|-------|------------|
| 1: Foundation | 1-4 | Types, Graph, ImmutableGraph, MVCC |
| 2: Engine Core | 5-7 | Builder, Engine lifecycle/ingestion, Snapshot |
| 3: Analysis | 8-12 | Divergence, Tension, Calibrator, Curvature, Concealment Cost, Yharim Limit, Integration |
| 4: MVCC Advanced | 13-14 | GC, Unified View, Compaction |
| 5: Callbacks | 15-16 | Callback system, Export |
| 6: Polish | 17-18 | Integration tests, Benchmarks |

**Total: 18 tasks, ~36 commits following TDD cycle.**

Every task follows: write failing test → implement minimal code → verify pass → commit.
