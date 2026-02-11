# ITT Engine v2 — Specification

**Goal**: Implement the remaining theoretical framework from the ITT papers as
domain-agnostic, generic features in the engine. Everything described here applies
to any domain (blockchain, NPM, social networks, ecology, etc.).

**Principle**: The engine answers generic questions — "is this anomalous?",
"is this detectable?", "how is tension evolving?", "what phase is this region in?"
— never domain-specific ones. Domain knowledge lives in callbacks and configuration.

---

## Overview

Four feature groups, each grounded in the existing theory papers:

| # | Feature | Theory Source | Status |
|---|---------|--------------|--------|
| 1 | Concealment Integration | §3.4 core_refactor, Axiom 4 v4_validated | Standalone → Pipeline |
| 2 | Detectability Framework | §3.3 core_refactor, §5.1 v4_validated | Partial → Complete |
| 3 | Temporal Dynamics | §3.5 core_refactor, Chapter 3 v4_validated | Not implemented |
| 4 | Dead Code Cleanup | — | Declared → Remove/Fix |

---

## 1. Concealment Integration

### What exists

`analysis.ConcealmentCalculator` is implemented and tested but standalone — not
called from Engine or Snapshot.

`analysis.YharimLimit`, `analysis.SNR`, `analysis.IsDetectable` — same situation.

### What the theory says

> Ω(Ns) = Σ_k Σ_vi τ(vi) · exp(-λk)
>
> "Concealment cost measures the work required to maintain a density gradient."

> CPS(Σ) = P(H_S | O, C) · 1_{SNR(Σ) > Υ}
>
> "Equals zero if the region is below the detectability threshold."

The theory defines three layers:
1. **Tension** τ(v) — local divergence (already wired)
2. **Concealment cost** Ω(v) — neighborhood propagation cost (standalone)
3. **CPS** — probability score combining posterior + detectability (not implemented)

### What to implement

#### 1.1 Wire ConcealmentCalculator into Snapshot.Analyze

When computing `TensionResult` for each node, also compute concealment cost if
a concealment configuration is provided.

```go
// Builder additions
type Builder struct {
    // ...existing fields...
    concealmentLambda float64 // exponential decay parameter (0 = disabled)
    concealmentHops   int     // max BFS hops for neighborhood
}

func (b *Builder) Concealment(lambda float64, maxHops int) *Builder
```

```go
// TensionResult additions
type TensionResult struct {
    // ...existing fields...
    Concealment float64 // concealment cost Ω(v), 0 if disabled
}
```

In `Snapshot.Analyze()`, after computing tension for each node:

```go
if cfg.concealmentLambda > 0 {
    cc := analysis.NewConcealmentCalculator(cfg.concealmentLambda, tc)
    result.Concealment = cc.CalculateNode(gv, nodeID, cfg.concealmentHops)
}
```

Same for `AnalyzeNode()` and `AnalyzeRegion()`.

#### 1.2 CPS (Concealment Probability Score)

New function in `analysis/`:

```go
// CPS computes the Concealment Probability Score for a set of tension values.
// Returns 0 if the region is below the detectability threshold (SNR <= Yharim).
// Otherwise returns a normalized score in [0, 1] based on concealment cost
// relative to the theoretical maximum for the given topology.
//
// Formula: CPS(Σ) = normalize(Ω) · 1_{SNR > Υ(α)}
func CPS(tensions []float64, concealmentCost float64, alpha float64) float64
```

Wire into `RegionResult`:

```go
type RegionResult struct {
    // ...existing fields...
    CPS float64 // concealment probability score
}
```

---

## 2. Detectability Framework

### What exists

- `analysis.YharimLimit(alpha)` — returns Υ = √(2·ln(1/α))
- `analysis.SNR(tensions)` — returns (mean/stddev)·√n
- `analysis.IsDetectable(tensions, alpha)` — returns SNR > Υ

All standalone, not integrated.

### What the theory says

> JSD is bounded ∈ [0,1]. Therefore τ_JSD has finite variance σ_τ² < 1, even on
> scale-free networks. The Yharim threshold remains valid.
>
> "The choice of JSD is not merely robust — it is mathematically necessary for the
> Yharim Limit to hold in real networks."

The theory already proves Yharim works for heavy-tail/power-law networks **when
using JSD**. No adaptation needed — JSD's boundedness is the adaptation. The key
is that KL divergence would fail (unbounded → heavy-tailed tension → Gumbel
assumption violated), but JSD doesn't.

The theory also defines three detectability regions:

| Region | Condition | Meaning |
|--------|-----------|---------|
| I. Undetectable | SNR < Υ | No method works |
| II. Weakly Detectable | Υ < SNR < 2Υ | Requires global analysis |
| III. Strongly Detectable | SNR > 2Υ | Local analysis sufficient |

### What to implement

#### 2.1 DetectabilityRegion type

```go
// analysis/detectability.go

type DetectabilityRegion int

const (
    Undetectable      DetectabilityRegion = iota // SNR < Υ
    WeaklyDetectable                              // Υ < SNR < 2Υ
    StronglyDetectable                            // SNR > 2Υ
)

// Classify returns the detectability region for a set of tensions at the
// given false positive rate alpha.
func Classify(tensions []float64, alpha float64) DetectabilityRegion

// DetectabilityResult holds the full detectability analysis.
type DetectabilityResult struct {
    SNR       float64
    Threshold float64             // Υ(α)
    Region    DetectabilityRegion
    Alpha     float64             // false positive rate used
}

// Detectability computes the full detectability analysis.
func Detectability(tensions []float64, alpha float64) DetectabilityResult
```

#### 2.2 Wire into Results

```go
type Results struct {
    // ...existing fields...
    Detectability DetectabilityResult // overall detectability assessment
}

type RegionResult struct {
    // ...existing fields...
    Detectability DetectabilityResult // region-level detectability
}
```

In `Snapshot.Analyze()`, after collecting all tension values:

```go
tensions := make([]float64, len(results.Tensions))
for i, t := range results.Tensions {
    tensions[i] = t.Tension
}
results.Detectability = analysis.Detectability(tensions, 0.05) // default α=0.05
```

#### 2.3 Configurable alpha

```go
// Builder addition
func (b *Builder) DetectabilityAlpha(alpha float64) *Builder
```

Default: 0.05 (5% false positive rate). The user can tune this per domain without
the engine knowing what the domain is.

#### 2.4 Enforce JSD for Yharim correctness

The Yharim limit is only mathematically valid with bounded divergence functions
(JSD, Hellinger). KL is unbounded and violates Gumbel-domain assumptions.

Add a validation note in the `Detectability` function documentation. Do NOT
restrict which divergence the user picks — they may want KL for tension
calculation even if detectability becomes approximate. But log a warning if
the engine has detectability enabled + non-bounded divergence:

```go
// In Snapshot.Analyze(), if detectability is computed:
if cfg.logger != nil && !isBoundedDivergence(cfg.divergence) {
    cfg.logger.Warn("detectability results may be unreliable with unbounded divergence; JSD or Hellinger recommended")
}
```

---

## 3. Temporal Dynamics

This is the largest feature group. The theory defines a complete temporal
framework that is entirely unimplemented.

### What the theory says

**Tension Diffusion Equation:**
> ∂τ/∂t = -αLτ + S(v,t)
>
> L = graph Laplacian, α = diffusivity, S = source term (suppression)

**Temporal Anomaly Indicators (4 simultaneous signals):**
> 1. Tension spike: Δτ_max(t) = max_v |τ(v,t) - τ(v,t-1)|
> 2. Decay exponent: γ(t) = -d/dt log(τ̄(t))
> 3. Betti velocity: β̇_k(t) = d(β_k)/dt
> 4. Curvature shock: Δκ_min(t) = min_{(i,j)} |κ_ij(t) - κ_ij(t-1)|

**Phase Classification (4 phases):**
> I. Full Recovery — tension dissipates, topology heals
> II. Scarred Recovery — tension dissipates, topology scarred
> III. Chronic Tension — sustained tension, continuous adaptation
> IV. Structural Collapse — runaway accumulation, failure

**Velocity of Silence:**
> v_silence = α · √λ₁ · ℓ̄
>
> How fast the "news" of an anomaly propagates through the network.

**Age Estimation:**
> t_supp ≈ r / (α · √λ₁ · ℓ̄)
>
> Estimate how long ago the anomaly started.

### Design: Domain-Agnostic Temporal Analysis

The key insight: this is NOT about detecting "rug-pulls" or "supply-chain attacks".
It's about answering generic temporal questions:

- **"Is tension increasing, stable, or decaying?"** → Decay exponent γ(t)
- **"Did something just happen?"** → Tension spike Δτ_max
- **"What phase is this region in?"** → Phase classification
- **"How old is this anomaly?"** → Age estimation
- **"How fast is the anomaly spreading?"** → Velocity of silence

These questions apply to ANY domain.

### What to implement

#### 3.1 TensionHistory — Per-Node Temporal Tracking

A ring buffer of tension values per node, maintained by the engine.

```go
// analysis/temporal.go

// TensionSample is a single tension measurement at a point in time.
type TensionSample struct {
    Tension   float64
    Timestamp time.Time
    Version   uint64
}

// TensionHistory tracks tension evolution for a single node.
// Implemented as a fixed-size ring buffer.
type TensionHistory struct {
    samples []TensionSample
    size    int
    head    int
    count   int
}

func NewTensionHistory(capacity int) *TensionHistory

// Push adds a new sample.
func (h *TensionHistory) Push(s TensionSample)

// Latest returns the most recent sample.
func (h *TensionHistory) Latest() (TensionSample, bool)

// Previous returns the sample before the latest.
func (h *TensionHistory) Previous() (TensionSample, bool)

// Slice returns all samples in chronological order.
func (h *TensionHistory) Slice() []TensionSample

// Len returns the number of samples stored.
func (h *TensionHistory) Len() int
```

#### 3.2 TemporalCalculator — Compute Temporal Indicators

```go
// analysis/temporal.go

// TemporalIndicators holds the four temporal anomaly signals from the theory.
type TemporalIndicators struct {
    // TensionSpike: Δτ_max(t) = max_v |τ(v,t) - τ(v,t-1)|
    // Measures the largest single-node tension change between two snapshots.
    TensionSpike float64

    // DecayExponent: γ(t) = -d/dt log(τ̄(t))
    // Positive = tension decaying (recovery). Negative = tension growing (active anomaly).
    // Near zero = chronic/stable state.
    DecayExponent float64

    // CurvatureShock: Δκ_min(t) = min_{(i,j)} |κ_ij(t) - κ_ij(t-1)|
    // Measures the largest curvature disruption between two snapshots.
    // Only computed if curvature is enabled.
    CurvatureShock float64

    // Timestamp of the measurement.
    Timestamp time.Time
}

// TemporalCalculator computes temporal dynamics from tension histories.
type TemporalCalculator struct {
    alpha float64 // diffusivity constant
}

func NewTemporalCalculator(alpha float64) *TemporalCalculator

// Indicators computes temporal anomaly indicators from current and previous
// tension snapshots. Both maps are nodeID → tension value.
func (tc *TemporalCalculator) Indicators(
    current map[string]float64,
    previous map[string]float64,
    dt time.Duration,
) TemporalIndicators

// Note: Betti velocity (β̇_k) is omitted. It requires persistent homology
// computation which depends on topology (currently dead code). It can be
// added later when TopologyFunc is implemented. The other three indicators
// are sufficient for temporal anomaly detection per the theory.
```

#### 3.3 Phase Classification

```go
// analysis/temporal.go

// Phase represents the suppression behavior phase from the theory.
type Phase int

const (
    // PhaseFullRecovery: tension dissipates, structure heals completely.
    // Low suppression intensity, high healing capacity.
    PhaseFullRecovery Phase = iota

    // PhaseScarredRecovery: tension dissipates, but structure retains damage.
    // Low suppression intensity, low healing capacity.
    PhaseScarredRecovery

    // PhaseChronicTension: sustained tension, structure continuously adapts.
    // High suppression intensity, high healing capacity.
    PhaseChronicTension

    // PhaseStructuralCollapse: runaway tension accumulation, eventual failure.
    // High suppression intensity, low healing capacity.
    PhaseStructuralCollapse
)

// PhaseResult holds the phase classification for a region.
type PhaseResult struct {
    Phase Phase
    Rho   float64 // ρ = suppression intensity (σ / α·λ₁)
    Pi    float64 // π = healing capacity (η / μ)
}

// ClassifyPhase determines the current suppression phase based on
// temporal indicators and graph structure.
//
// Parameters:
//   - indicators: temporal indicators from at least 2 consecutive snapshots
//   - meanTension: current mean tension across the region
//   - prevMeanTension: previous mean tension (for trend detection)
//   - connectivityRatio: fraction of edges that survived between snapshots
//     (measures structural healing; 1.0 = no edges lost)
//
// The phase boundaries are:
//   - ρ_c = 1.0 (suppression intensity threshold)
//   - π_c derived from tension trend + connectivity
func ClassifyPhase(
    indicators TemporalIndicators,
    meanTension float64,
    prevMeanTension float64,
    connectivityRatio float64,
) PhaseResult
```

#### 3.4 Velocity of Silence

```go
// analysis/temporal.go

// VelocityOfSilence computes how fast anomaly information propagates
// through the network.
//
// Formula: v_silence = alpha * sqrt(lambda1) * meanEdgeLength
//
// Parameters:
//   - alpha: diffusivity constant
//   - lambda1: smallest non-zero eigenvalue of the graph Laplacian (Fiedler value)
//   - meanEdgeLength: average edge weight (inverse, since higher weight = shorter path)
//
// Returns propagation speed in "hops per unit time".
func VelocityOfSilence(alpha, lambda1, meanEdgeLength float64) float64

// EstimateAge estimates how long ago an anomaly started, given the
// distance (in hops) from the anomaly epicenter to the observation point.
//
// Formula: t_supp ≈ distance / v_silence
func EstimateAge(distance float64, velocity float64) time.Duration
```

**Note on λ₁ (Fiedler value):** Computing the exact Fiedler value requires
eigendecomposition of the graph Laplacian, which is O(n³) for dense graphs.
For the engine, provide two approaches:

```go
// analysis/laplacian.go

// FiedlerValue computes the algebraic connectivity (smallest non-zero eigenvalue
// of the graph Laplacian) using power iteration.
//
// This is an approximation suitable for sparse graphs. For large dense graphs,
// consider sampling or approximation methods.
//
// Returns 0 if the graph is disconnected (λ₁ = 0 means multiple components).
func FiedlerValue(g GraphView, nodeIDs []string, maxIter int, tol float64) float64

// FiedlerApprox returns a fast approximation of algebraic connectivity
// based on the Cheeger inequality: λ₁ ≥ h²/(2·d_max), where h is the
// edge expansion (Cheeger constant).
//
// Much faster than eigendecomposition. Lower bound only.
func FiedlerApprox(g GraphView, nodeIDs []string) float64
```

#### 3.5 Engine Integration

The engine needs to maintain temporal state. Two approaches:

**Approach A: History in Engine (chosen)**

The engine maintains a map of tension histories. On each analysis (Analyze,
AnalyzeNode, real-time checkAnomalies), it pushes the new tension into the
history. Temporal indicators are computed from the history.

```go
// Engine additions
type Engine struct {
    // ...existing fields...
    tensionHistory   map[string]*analysis.TensionHistory // nodeID → history
    tensionHistoryMu sync.RWMutex
    lastIndicators   *analysis.TemporalIndicators        // most recent global indicators
}

// Builder additions
func (b *Builder) TemporalCapacity(n int) *Builder  // ring buffer size per node (default: 100)
func (b *Builder) DiffusivityAlpha(alpha float64) *Builder // α for diffusion (default: 0.1)
```

In `checkAnomalies()`, after computing tension:

```go
e.tensionHistoryMu.Lock()
if e.tensionHistory == nil {
    e.tensionHistory = make(map[string]*analysis.TensionHistory)
}
h, ok := e.tensionHistory[nodeID]
if !ok {
    h = analysis.NewTensionHistory(e.config.temporalCapacity)
    e.tensionHistory[nodeID] = h
}
h.Push(analysis.TensionSample{Tension: t, Timestamp: ev.Timestamp, Version: version})
e.tensionHistoryMu.Unlock()
```

#### 3.6 Temporal Results

```go
// Root package additions

type TensionResult struct {
    // ...existing fields...
    Trend    Trend   // tension trend for this node
    Velocity float64 // estimated propagation velocity (0 if insufficient history)
}

// Trend indicates the direction of tension change.
type Trend int

const (
    TrendStable     Trend = iota // |Δτ| < ε
    TrendIncreasing              // Δτ > ε (tension growing — active anomaly)
    TrendDecreasing              // Δτ < -ε (tension decaying — recovery)
)

type Results struct {
    // ...existing fields...
    Temporal TemporalSummary // temporal analysis summary
}

// TemporalSummary holds temporal dynamics for the full analysis.
type TemporalSummary struct {
    Indicators TemporalIndicators
    Phase      PhaseResult
    Velocity   float64       // velocity of silence (0 if insufficient data)
}
```

In `Snapshot.Analyze()`:

- If the engine has tension history AND previous snapshot indicators available,
  compute `TemporalIndicators`, `ClassifyPhase`, and `VelocityOfSilence`.
- If this is the first analysis (no history), `Temporal` is zero-valued.

The Snapshot needs access to the engine's tension history. Two options:
- Pass the history map reference when creating the snapshot (read-only)
- Copy relevant history entries into the snapshot on creation

Chosen: **pass a read-only reference**. The history map is protected by RWMutex.
The snapshot takes a read lock when accessing it. This avoids copying and keeps
snapshots lightweight.

```go
// Snapshot additions
type Snapshot struct {
    // ...existing fields...
    tensionHistory   map[string]*analysis.TensionHistory
    tensionHistoryMu *sync.RWMutex
}
```

#### 3.7 OnTensionSpike Callback

New callback for real-time temporal events:

```go
// Builder addition
func (b *Builder) OnTensionSpike(f func(nodeID string, delta float64)) *Builder
```

Fired in `checkAnomalies()` when `|τ(v,t) - τ(v,t-1)| > spikeThreshold`.

```go
func (b *Builder) TensionSpikeThreshold(t float64) *Builder // default: 0.3
```

This is domain-agnostic: the engine says "tension at node X just jumped by 0.5".
The user's callback decides what to do (alert, log, trade, etc.).

#### 3.8 New Delta Types

Wire the currently-unused delta types for temporal events:

```go
DeltaTensionChanged  // emitted when tension trend changes (stable→increasing, etc.)
```

The remaining unused deltas (NodeUpdated, NodeRemoved, EdgeRemoved,
AnomalyResolved) belong to graph mutation events that the engine doesn't
currently support (node/edge removal). These stay as-is until removal
operations are implemented.

---

## 4. Dead Code Cleanup

### Remove (truly unused, no future path)

| Item | File | Reason |
|------|------|--------|
| `BatchDivergenceFunc` | callbacks.go | Never used, no plan to batch divergence |
| `DistributionPair` | callbacks.go | Only used by BatchDivergenceFunc |
| `Builder.maxOverlaySize` | builder.go | Never enforced, compaction handles this |
| `compact.ShouldCompact()` | compact/compact.go | Engine has its own shouldCompact() |

### Fix (populate properly)

| Item | File | Fix |
|------|------|-----|
| `GCStats.MemoryFreed` | types.go | Estimate from version count × avg graph size, or remove field |
| `Delta.Node` | types.go | Populate in DeltaNodeAdded with the actual Node data |
| `Delta.Edge` | types.go | Populate in DeltaEdgeAdded/Updated with actual Edge data |
| `Delta.Previous` | types.go | Populate in DeltaEdgeUpdated with previous weight |
| `ErrEdgeNotFound` | errors.go | Will be used when edge query methods are added, keep |

### Rewire (interface exists but bypassed)

| Item | Current State | Action |
|------|--------------|--------|
| `CurvatureFunc` | Bypassed by curvatureAlpha | Keep interface. Add adapter: if user sets CurvatureFunc, use it; if they set curvatureAlpha, create internal CurvatureCalculator. Both paths work. |
| `Storage` | Interface exists, never called | Implement Load on engine start, Save on compaction. See §4.1. |
| `Builder.baseGraph` | Field stored, never read | Use in newEngine to initialize base graph. See §4.2. |
| `TopologyFunc` | Interface exists, never called | Keep as extension point. Document as "future: Betti numbers for temporal Betti velocity". |

#### 4.1 Storage Implementation

```go
// In newEngine(), after creating GC:
if cfg.storage != nil {
    data, err := cfg.storage.Load()
    if err != nil {
        // Log warning but don't fail — start with empty graph
        if cfg.logger != nil {
            cfg.logger.Warn("failed to load from storage", "error", err)
        }
    } else if data != nil {
        e.base = graphFromData(data)
    }
}
```

On compaction (in `doCompact()`), after merging:

```go
if e.config.storage != nil {
    go func() {
        data := graphToData(e.base)
        if err := e.config.storage.Save(data); err != nil {
            e.reportError(err)
        }
    }()
}
```

`graphFromData` and `graphToData` are conversion functions between
`*GraphData` and `*graph.ImmutableGraph`.

#### 4.2 BaseGraph Initialization

```go
// In newEngine():
if cfg.baseGraph != nil {
    e.base = graphFromData(cfg.baseGraph)
}
```

This allows users to bootstrap the engine with historical data without
implementing the full Storage interface.

---

## Implementation Order

### Phase 1: Cleanup + Low-Hanging Fruit
1. Remove dead code (BatchDivergenceFunc, DistributionPair, maxOverlaySize, compact.ShouldCompact)
2. Fix Delta field population
3. Wire CurvatureFunc adapter
4. Wire baseGraph initialization
5. Wire Storage Load/Save

### Phase 2: Detectability Framework
6. Implement DetectabilityRegion, Classify, DetectabilityResult
7. Wire into Results and RegionResult
8. Add DetectabilityAlpha to Builder
9. Add JSD validation warning

### Phase 3: Concealment Integration
10. Wire ConcealmentCalculator into Snapshot.Analyze/AnalyzeNode/AnalyzeRegion
11. Add Concealment(lambda, hops) to Builder
12. Implement CPS function
13. Wire CPS into RegionResult

### Phase 4: Temporal Dynamics
14. Implement TensionHistory (ring buffer)
15. Implement TemporalCalculator + TemporalIndicators
16. Implement ClassifyPhase
17. Implement FiedlerValue / FiedlerApprox
18. Implement VelocityOfSilence + EstimateAge
19. Add temporal state to Engine (tensionHistory map)
20. Wire temporal computation into Snapshot.Analyze
21. Add Trend to TensionResult
22. Add TemporalSummary to Results
23. Implement OnTensionSpike callback
24. Wire DeltaTensionChanged

### Phase 5: Tests
25. Unit tests for each new analysis function
26. Integration test: full temporal lifecycle (ingest → spike → decay → classify phase)
27. Benchmark: temporal overhead per event

---

## Non-Goals (for this version)

- **Persistent homology / Betti numbers**: Requires TopologyFunc implementation.
  Deferred until topology support is designed.
- **Graph Laplacian eigendecomposition**: Full eigendecomposition is O(n³).
  We provide FiedlerValue via power iteration (approximate) and FiedlerApprox
  via Cheeger inequality (lower bound). Exact eigendecomposition is a non-goal.
- **Node/edge removal**: The engine currently only adds nodes and edges (append-only
  graph). Removal operations (and corresponding DeltaNodeRemoved, DeltaEdgeRemoved,
  DeltaAnomalyResolved) are deferred.
- **Distributed engine**: Single-process only. Sharding/distribution is a separate project.
