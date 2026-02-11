# ITT Engine — Architecture & Theory Guide

A guide connecting Informational Tension Theory to the implementation.

---

## 1. Theory Overview

### What is Informational Tension Theory?

ITT is a mathematical framework for anomaly detection in networks. Instead of looking at individual data points, ITT models the *information flow* between entities as a graph and measures how each node's behavior deviates from expectation.

The central quantity is **tension**:

```
tau(v) = D(P_obs(v) || P_exp(v))
```

Where:
- `P_obs(v)` is the **observed** distribution of node v's interactions (normalized edge weights to neighbors)
- `P_exp(v)` is the **expected** distribution (uniform over neighbors, by default)
- `D` is a divergence function measuring the "distance" between distributions

High tension means the node's interaction pattern is surprising — it concentrates traffic on unexpected neighbors or shows unusual weight distributions.

### Why JSD?

Jensen-Shannon Divergence is the default because it has three critical properties:

1. **Symmetric**: `JSD(P||Q) = JSD(Q||P)` — anomaly detection shouldn't depend on direction
2. **Bounded**: `JSD in [0, ln(2)]` — finite range enables statistical guarantees
3. **Valid for power-law networks**: Boundedness ensures finite moments even on scale-free graphs, which is required for the Yharim detectability limit (see below)

KL divergence is asymmetric and unbounded, making it unsuitable for the detectability framework. Hellinger distance is also valid (bounded, symmetric).

### Curvature

Ollivier-Ricci curvature measures local geometric distortion of edges:

```
kappa(x,y) = 1 - W1(mu_x, mu_y) / d(x,y)
```

Where `W1` is the Wasserstein-1 (Earth Mover's) distance between the probability measures at nodes x and y, computed via Sinkhorn optimal transport.

- **Positive curvature**: nodes are "well-connected" (clique-like)
- **Zero curvature**: lattice-like structure
- **Negative curvature**: structural bottleneck (bridge edge)

In the engine, curvature is computed per edge and averaged per node in the `TensionResult.Curvature` field.

### MAD Calibration

The Median Absolute Deviation (MAD) calibrator provides robust anomaly thresholds for heavy-tailed distributions:

```
threshold = median + K * MAD
```

Where `MAD = median(|x_i - median(x)|)`. This is more robust than mean+stddev for networks with power-law degree distributions.

---

## 2. Detectability Theory

### Yharim Limit

The Yharim limit defines the theoretical boundary below which no anomaly detection method can reliably work:

```
Upsilon(alpha) = sqrt(2 * ln(1/alpha))
```

Where alpha is the desired false positive rate. For alpha=0.05: Upsilon ~= 2.448.

### Signal-to-Noise Ratio

```
SNR = (mean(tau) / stddev(tau)) * sqrt(n)
```

The SNR compares the strength of the anomaly signal to the background noise.

### Three Detectability Regions

| Region | Condition | Implication |
|--------|-----------|-------------|
| **Undetectable** | SNR < Upsilon | The anomaly signal is buried in noise. No method — not just ITT — can detect it |
| **WeaklyDetectable** | Upsilon < SNR < 2*Upsilon | Detection requires global analysis (looking at the full graph structure) |
| **StronglyDetectable** | SNR > 2*Upsilon | Local analysis per-node is sufficient |

### Why Boundedness Matters

The Yharim limit relies on Extreme Value Theory (EVT), which requires finite moments. JSD and Hellinger produce bounded values, guaranteeing finite moments. KL divergence is unbounded — on a power-law graph, it can produce infinite variance, making the SNR undefined and the detectability analysis unreliable.

The engine logs a warning when using unbounded divergence with detectability.

---

## 3. Concealment Theory

### Concealment Cost

The concealment cost Omega measures how expensive it is to hide manipulation:

```
Omega(N_s) = sum_{k=0}^{K} sum_{v_i in ring_k} tau(v_i) * exp(-lambda * k)
```

Where `ring_k` is the set of nodes at BFS distance k from the anomaly source, and lambda controls exponential decay.

Key insight: **concealment cost scales superlinearly**. To reduce tension at a node, an attacker must also manipulate its neighbors (to make the target's distribution look normal). But manipulating neighbors creates tension at *their* neighbors, and so on.

### CPS (Concealment Probability Score)

CPS combines concealment cost with detectability:

```
CPS(Sigma) = normalize(Omega) * 1_{SNR > Upsilon(alpha)}
```

Where normalize uses a sigmoid function: `CPS = 2/(1+exp(-Omega/mean_tau)) - 1`

CPS answers: "Given that we can detect something, how hard would it be to conceal it?"

- CPS = 0: either undetectable (so concealment is irrelevant) or zero concealment cost
- CPS near 1: high concealment cost — the anomaly is deeply embedded and hard to hide

---

## 4. Temporal Dynamics

### Tension Diffusion Equation

The theory models tension propagation as a diffusion process on the graph:

```
d_tau/dt = -alpha * L * tau + S(v, t)
```

Where:
- `L` is the graph Laplacian (encodes network structure)
- `alpha` is the diffusivity constant (how fast tension spreads)
- `S(v, t)` is the source term (new anomalies injected)

### Temporal Indicators

Three temporal anomaly signals:

| Indicator | Formula | Meaning |
|-----------|---------|---------|
| **TensionSpike** | `max_v |tau(v,t) - tau(v,t-1)|` | Sudden change in tension across nodes |
| **DecayExponent** | `gamma(t) = -d/dt log(mean_tau(t))` | Positive = recovery, negative = growth |
| **CurvatureShock** | `max_e |kappa(e,t) - kappa(e,t-1)|` | Sudden structural change |

### Phase Diagram

The theory identifies four suppression phases based on two parameters:

- **rho** (suppression intensity): how strongly anomalies are being generated
- **pi** (healing capacity): the network's ability to recover

```
                pi (healing)
              High        Low
         +----------+-----------+
rho Low  |  Phase I |  Phase II |
(weak)   | Full     | Scarred   |
         | Recovery | Recovery  |
         +----------+-----------+
rho High | Phase III| Phase IV  |
(strong) | Chronic  | Structural|
         | Tension  | Collapse  |
         +----------+-----------+
```

Phase boundaries: rho_c = 1.0, pi_c = 0.5

### Velocity of Silence

How fast anomaly information propagates through the network:

```
v_silence = alpha * sqrt(lambda_1) * mean_edge_length
```

Where lambda_1 is the **Fiedler value** (algebraic connectivity = second smallest eigenvalue of the graph Laplacian). Higher connectivity = faster propagation.

Age estimation: if an anomaly is observed at distance r from the epicenter:

```
t_suppression ~= r / v_silence
```

### Fiedler Value

The algebraic connectivity lambda_1 is computed via:

1. **Exact**: Inverse power iteration with Jacobi iterative solver (`FiedlerValue`)
2. **Fast approximation**: Cheeger inequality with BFS partition (`FiedlerApprox`)

lambda_1 = 0 means the graph is disconnected.

---

## 5. Architecture Diagram

### Event Flow

```
Event --> Engine.AddEvent()
              |
              v
          eventCh (buffered channel)
              |
              v
          worker goroutine
              |
              +---> processEvent()
              |         |
              |         +---> graph.WithEvent() --> new ImmutableGraph
              |         +---> mvcc.Version{} --> vc.Store()
              |         +---> fire OnChange callbacks (DeltaNodeAdded, DeltaEdgeAdded, etc.)
              |         +---> checkAnomalies()
              |         |         |
              |         |         +---> TensionCalculator for dirty nodes
              |         |         +---> Push to TensionHistory
              |         |         +---> Check OnTensionSpike threshold
              |         |         +---> Determine Trend, emit DeltaTensionChanged
              |         |         +---> fire OnAnomaly if threshold exceeded
              |         |
              |         +---> shouldCompact() --> doCompact()
              |                                       |
              |                                       +---> compact.Compact(base, overlay)
              |                                       +---> Storage.Save() (async)
              |                                       +---> fire OnCompact callback
              |
              +---> gcWorker goroutine (every 30s)
                        |
                        +---> gc.Collect() --> fire OnGC callback
```

### Analysis Flow

```
Engine.Snapshot()
    |
    +---> vc.Acquire() (ref-counted)
    +---> copy base pointer
    +---> share tensionHistory reference (RWMutex)
    |
    v
Snapshot.Analyze()
    |
    +---> graphView() = UnifiedView(base, overlay) or overlay alone
    +---> TensionCalculator.CalculateAll()
    +---> CurvatureCalculator.CalculateAll() (optional)
    +---> ConcealmentCalculator per node (optional)
    +---> Detectability analysis (SNR, Yharim, Classify)
    +---> JSD warning for unbounded divergence
    +---> Temporal analysis:
    |         +---> Build current/previous tension maps from history
    |         +---> TemporalCalculator.Indicators()
    |         +---> Compute per-node Trends
    |         +---> ClassifyPhase()
    |         +---> FiedlerApprox() --> VelocityOfSilence()
    |
    v
Results{Tensions, Anomalies, Stats, Temporal, Detectability}
```

---

## 6. Package Dependency Graph

```
                 itt (root)
                /    |    \
               /     |     \
          analysis  graph   export
              |      |
              |    mvcc
              |
           (graph)  <-- analysis uses graph types
              |
           compact  <-- uses graph types
```

Import rules (enforced by Go compiler):
- `itt` imports `analysis`, `graph`, `mvcc`, `compact`, `export`
- `analysis` imports `graph`
- `compact` imports `graph`
- `export` imports `graph`
- `mvcc` imports `graph`
- `graph` imports nothing from this module

No circular dependencies. The root package uses structural typing (mirror structs like `DetectabilityResult`) to avoid importing `analysis` types directly into public APIs.

---

## 7. Concurrency Model

### MVCC (Multi-Version Concurrency Control)

The engine uses MVCC for lock-free reads:

- **Writer** (single goroutine): processes events sequentially via `eventCh`, creates new `ImmutableGraph` versions via copy-on-write, stores via atomic pointer
- **Readers** (any goroutine): `Snapshot()` acquires the current version with ref-counting. The snapshot sees a frozen point-in-time view

```
Writer:  Store(V1) --> Store(V2) --> Store(V3) --> ...
                                         ^
Reader1:                     Acquire(V2) ---> Analyze ---> Release(V2)
Reader2:              Acquire(V1) ----> Analyze ---------> Release(V1)
Reader3:                                     Acquire(V3) -> ...
```

### Ref-counting & GC

- `Acquire()` increments the version's refcount
- `Release()` (via `Snapshot.Close()`) decrements it
- GC runs every 30s, collecting versions with refcount=0
- Only versions acquired by `Snapshot()` are tracked (not every intermediate version) to avoid OOM in high-throughput

### Synchronization Primitives

| Resource | Protection | Access Pattern |
|----------|-----------|----------------|
| Current version | `atomic.Pointer[Version]` | Lock-free read, single writer |
| Base graph | `sync.RWMutex` (baseMu) | Read-heavy (snapshots), write-rare (compaction) |
| Tension history | `sync.RWMutex` (tensionHistoryMu) | Write in worker, read in Analyze |
| Last trend | `sync.RWMutex` (lastTrendMu) | Write in worker, read for delta emission |
| Snapshot state | `sync.Mutex` (per snapshot) | Protects closed flag |
| Event counters | `atomic.Int64` / `atomic.Uint64` | Lock-free increment |

### Goroutines

| Goroutine | Lifetime | Purpose |
|-----------|----------|---------|
| `worker` | Start() to Stop() | Process events from channel |
| `gcWorker` | Start() to Stop() | Periodic garbage collection |
| Storage.Save | Fire-and-forget | Async persistence after compaction |

---

## 8. Extension Points

### Custom Divergence Function

```go
type MyDivergence struct{}
func (d MyDivergence) Compute(p, q []float64) float64 { /* ... */ }
func (d MyDivergence) Name() string { return "my-divergence" }
// Optional: implement IsBounded() bool for detectability compatibility

builder.Divergence(MyDivergence{})
```

### Custom Curvature Function

```go
type MyCurvature struct{}
func (c MyCurvature) Compute(g itt.GraphView, from, to string) float64 { /* ... */ }
func (c MyCurvature) Name() string { return "my-curvature" }

builder.Curvature(MyCurvature{})
```

Note: `itt.GraphView` uses `*itt.Node` and `*itt.Edge`, while `analysis.GraphView` uses `*graph.NodeData` and `*graph.EdgeData`. The engine handles the adapter internally.

### Custom Storage

```go
type RedisStorage struct { client *redis.Client }

func (s *RedisStorage) Load() (*itt.GraphData, error) {
    // Deserialize from Redis
}

func (s *RedisStorage) Save(data *itt.GraphData) error {
    // Serialize to Redis
}

builder.WithStorage(&RedisStorage{client: redisClient})
```

### Custom Calibrator

```go
type MyCalibrator struct { /* ... */ }
func (c *MyCalibrator) Observe(tension float64)     { /* ... */ }
func (c *MyCalibrator) IsWarmedUp() bool             { /* ... */ }
func (c *MyCalibrator) Threshold() float64            { /* ... */ }
func (c *MyCalibrator) IsAnomaly(tension float64) bool { /* ... */ }
func (c *MyCalibrator) Stats() itt.CalibratorStats    { /* ... */ }
func (c *MyCalibrator) Recalibrate()                   { /* ... */ }

builder.WithCalibrator(&MyCalibrator{})
```

### Custom Threshold Logic

```go
builder.ThresholdFunc(func(node *itt.Node, tension float64) bool {
    // Time-based: stricter during business hours
    if time.Now().Hour() >= 9 && time.Now().Hour() <= 17 {
        return tension > 0.1
    }
    return tension > 0.5
})
```

---

## 9. Data Flow Summary

```
External System
      |
      | Event{Source, Target, Weight, ...}
      v
   Engine
      |
      | processEvent: graph mutation + anomaly check
      v
   MVCC Version (ImmutableGraph + metadata)
      |
      +-----> Snapshot (read-only, isolated)
      |           |
      |           +-----> Analyze() --> Results
      |           |           |
      |           |           +-- Tension per node
      |           |           +-- Curvature per node
      |           |           +-- Concealment per node
      |           |           +-- Detectability (SNR, Region)
      |           |           +-- Temporal (Spike, Phase, Velocity)
      |           |           +-- Trends (Stable/Increasing/Decreasing)
      |           |
      |           +-----> Export(JSON/DOT)
      |
      +-----> Callbacks (real-time)
      |           +-- OnAnomaly (per-event)
      |           +-- OnTensionSpike (per-event)
      |           +-- OnChange (deltas)
      |
      +-----> Compaction --> Storage.Save()
      +-----> GC --> cleanup old versions
```
