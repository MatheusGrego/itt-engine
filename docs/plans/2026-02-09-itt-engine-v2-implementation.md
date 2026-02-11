# ITT Engine v2 — Implementation Plan

**Spec**: `docs/plans/2026-02-09-itt-engine-v2.md`
**Status**: Pending approval

---

## Traceability Matrix

Every task maps back to the spec section it implements.

| Task | Spec Section | Phase | Depends On | Files |
|------|-------------|-------|------------|-------|
| T01 | §4 Remove | P1 | — | callbacks.go, builder.go, compact/compact.go |
| T02 | §4 Fix Delta | P1 | — | engine.go |
| T03 | §4 CurvatureFunc | P1 | — | snapshot.go, builder.go |
| T04 | §4.2 BaseGraph | P1 | — | engine.go |
| T05 | §4.1 Storage | P1 | T04 | engine.go, snapshot.go |
| T06 | §2.1 Detectability types | P2 | — | analysis/detectability.go (new) |
| T07 | §2.2 Wire Detectability | P2 | T06 | snapshot.go, types.go |
| T08 | §2.3 Builder alpha | P2 | T07 | builder.go |
| T09 | §2.4 JSD warning | P2 | T08 | snapshot.go, analysis/divergence.go |
| T10 | §1.1 Wire Concealment | P3 | — | snapshot.go, builder.go, types.go |
| T11 | §1.1 Builder.Concealment | P3 | T10 | builder.go |
| T12 | §1.2 CPS function | P3 | T06 | analysis/detectability.go |
| T13 | §1.2 Wire CPS | P3 | T12, T10 | snapshot.go, types.go |
| T14 | §3.1 TensionHistory | P4 | — | analysis/temporal.go (new) |
| T15 | §3.2 TemporalCalculator | P4 | T14 | analysis/temporal.go |
| T16 | §3.3 ClassifyPhase | P4 | T15 | analysis/temporal.go |
| T17 | §3.4 Fiedler value | P4 | — | analysis/laplacian.go (new) |
| T18 | §3.4 VelocityOfSilence | P4 | T17 | analysis/temporal.go |
| T19 | §3.5 Engine temporal state | P4 | T14 | engine.go, builder.go |
| T20 | §3.6 Wire temporal in Analyze | P4 | T15, T19 | snapshot.go |
| T21 | §3.6 Trend in TensionResult | P4 | T14, T19 | types.go, snapshot.go |
| T22 | §3.6 TemporalSummary in Results | P4 | T15, T16, T18, T20 | types.go, snapshot.go |
| T23 | §3.7 OnTensionSpike callback | P4 | T19 | engine.go, builder.go |
| T24 | §3.8 DeltaTensionChanged | P4 | T19, T23 | engine.go |
| T25 | §Tests: unit tests | P5 | T01-T24 | analysis/*_test.go, engine_test.go |
| T26 | §Tests: integration | P5 | T25 | integration_test.go |
| T27 | §Tests: benchmarks | P5 | T25 | bench_test.go |

---

## Phase 1: Cleanup + Low-Hanging Fruit (T01–T05)

### T01 — Remove dead code
**Spec**: §4 Remove
**Files**: callbacks.go, builder.go, compact/compact.go
**Parallelizable**: Yes (independent)

Steps:
1. **callbacks.go**: Delete `BatchDivergenceFunc` interface (lines 91-96) and `DistributionPair` struct (lines 85-89).
2. **builder.go**: Delete `maxOverlaySize` field (line 25), delete `MaxOverlaySize` method (line 71), remove from `NewBuilder()` defaults in itt.go (line 11).
3. **compact/compact.go**: Delete or unexport `ShouldCompact()` function (line 125+). It's public but unused — engine has its own `shouldCompact()`. Make it `shouldCompact` (unexported) if any internal test depends on it, otherwise delete.
4. Run `go build ./...` to verify no compilation errors.

**Acceptance**: `go build ./...` succeeds, no references to removed symbols.

---

### T02 — Populate Delta fields
**Spec**: §4 Fix Delta
**Files**: engine.go
**Parallelizable**: Yes (independent)

Currently, `processEvent()` emits deltas with only `Type`, `Timestamp`, `Version`, `NodeID`, `EdgeFrom`, `EdgeTo`. The spec requires populating `Node`, `Edge`, and `Previous`.

Steps:
1. In `processEvent()`, for `DeltaNodeAdded` deltas (lines 388, 396): after `newGraph` is created, fetch the actual node and populate `Delta.Node`:
   ```go
   if n, ok := newGraph.GetNode(ev.Source); ok {
       delta.Node = nodeFromGraph(n)
   }
   ```
2. For `DeltaEdgeAdded` / `DeltaEdgeUpdated` (line 408): fetch the edge and populate `Delta.Edge`:
   ```go
   if e, ok := newGraph.GetEdge(ev.Source, ev.Target); ok {
       delta.Edge = &Edge{From: e.From, To: e.To, Weight: e.Weight, ...}
   }
   ```
3. For `DeltaEdgeUpdated`: capture the old edge weight BEFORE `WithEvent()` and set `Delta.Previous`:
   ```go
   var previousWeight float64
   if oldEdge, ok := current.Graph.GetEdge(ev.Source, ev.Target); ok {
       previousWeight = oldEdge.Weight
   }
   // ... later in delta emission:
   delta.Previous = previousWeight
   ```
4. For `DeltaAnomalyDetected` (line 483): populate `Delta.Tension` (already done) and add `Delta.Node`:
   ```go
   if n, ok := newGraph.GetNode(nodeID); ok {
       delta.Node = nodeFromGraph(n)
   }
   ```

**Acceptance**: Delta callbacks receive fully populated structs. Test with assertion on `Delta.Node != nil` for `DeltaNodeAdded`.

---

### T03 — Wire CurvatureFunc adapter
**Spec**: §4 Rewire CurvatureFunc
**Files**: snapshot.go, builder.go
**Parallelizable**: Yes (independent)

Currently, `CurvatureFunc` interface is stored in Builder but never called — `curvatureAlpha` is used directly to create `analysis.CurvatureCalculator`. The spec says: keep both paths.

Steps:
1. In `Snapshot.Analyze()`, `AnalyzeNode()`, `AnalyzeRegion()`: before creating `analysis.CurvatureCalculator`, check if `s.config.curvature` is set:
   ```go
   if s.config.curvature != nil {
       // Use the user-provided CurvatureFunc interface
       curv = s.config.curvature.Compute(/* adapter to itt.GraphView */, from, to)
   } else if s.config.curvatureAlpha > 0 {
       // Use built-in analysis.CurvatureCalculator
       cc := analysis.NewCurvatureCalculator(s.config.curvatureAlpha)
       ...
   }
   ```
2. **GraphView adapter**: The `CurvatureFunc` interface expects `itt.GraphView`, but the snapshot has `analysis.GraphView`. Create a small adapter struct in snapshot.go that wraps `analysis.GraphView` to satisfy `itt.GraphView` (converting `*graph.NodeData` → `*itt.Node`).
3. Ensure the `Curvature(c CurvatureFunc)` builder method still sets `curvatureAlpha` to 0.5 as default (already does).

**Acceptance**: User can set either `Curvature(customFunc)` or `CurvatureAlpha(0.5)` — both produce curvature values in results.

---

### T04 — Wire BaseGraph initialization
**Spec**: §4.2 BaseGraph
**Files**: engine.go
**Parallelizable**: Yes (independent)

Steps:
1. Add `graphFromData(*GraphData) *graph.ImmutableGraph` helper function in engine.go. Iterate `data.Nodes` and `data.Edges`, build a mutable `graph.Graph`, then freeze into `ImmutableGraph`.
2. In `newEngine()`, after `e.base = graph.NewImmutableEmpty()` (line 73), check:
   ```go
   if cfg.baseGraph != nil {
       e.base = graphFromData(cfg.baseGraph)
   }
   ```
3. Also add `graphToData(*graph.ImmutableGraph) *GraphData` for symmetry (needed by T05).

**Acceptance**: `NewBuilder().BaseGraph(data).Build()` starts engine with pre-populated base graph visible in snapshots.

---

### T05 — Wire Storage Load/Save
**Spec**: §4.1 Storage
**Files**: engine.go
**Depends on**: T04 (needs `graphFromData` / `graphToData`)

Steps:
1. In `newEngine()`, after baseGraph initialization, try loading from storage:
   ```go
   if cfg.storage != nil {
       data, err := cfg.storage.Load()
       if err != nil {
           if cfg.logger != nil {
               cfg.logger.Warn("failed to load from storage", "error", err)
           }
       } else if data != nil {
           e.base = graphFromData(data)
       }
   }
   ```
   Note: Storage takes precedence over baseGraph (if both set, storage wins).
2. In `doCompact()`, after merging, save asynchronously:
   ```go
   if e.config.storage != nil {
       base := e.base // capture under lock
       go func() {
           data := graphToData(base)
           if err := e.config.storage.Save(data); err != nil {
               e.reportError(err)
           }
       }()
   }
   ```
3. Ensure `graphToData` properly converts all nodes and edges.

**Acceptance**: Engine with a Storage implementation persists base graph on compaction and loads on startup. Test with an in-memory mock storage.

---

## Phase 2: Detectability Framework (T06–T09)

### T06 — Implement Detectability types and functions
**Spec**: §2.1
**Files**: analysis/detectability.go (new file)
**Parallelizable**: Yes (independent)

Steps:
1. Create `analysis/detectability.go` with:
   - `DetectabilityRegion` type (int enum: `Undetectable`, `WeaklyDetectable`, `StronglyDetectable`)
   - `DetectabilityResult` struct with fields: `SNR`, `Threshold`, `Region`, `Alpha`
   - `Classify(tensions []float64, alpha float64) DetectabilityRegion`:
     ```go
     snr := SNR(tensions)
     limit := YharimLimit(alpha)
     if snr > 2*limit { return StronglyDetectable }
     if snr > limit { return WeaklyDetectable }
     return Undetectable
     ```
   - `Detectability(tensions []float64, alpha float64) DetectabilityResult`:
     ```go
     return DetectabilityResult{
         SNR:       SNR(tensions),
         Threshold: YharimLimit(alpha),
         Region:    Classify(tensions, alpha),
         Alpha:     alpha,
     }
     ```
2. Handle edge cases: empty tensions → `Undetectable` with SNR=0.
3. Add `String()` method on `DetectabilityRegion` for debugging.

**Acceptance**: Unit tests for all three regions, edge cases, and String() output.

---

### T07 — Wire Detectability into Results and RegionResult
**Spec**: §2.2
**Files**: types.go, snapshot.go
**Depends on**: T06

Steps:
1. **types.go**: Add `Detectability` field to `Results`:
   ```go
   type Results struct {
       // ...existing...
       Detectability analysis.DetectabilityResult
   }
   ```
   Add `Detectability` field to `RegionResult`:
   ```go
   type RegionResult struct {
       // ...existing...
       Detectability analysis.DetectabilityResult
   }
   ```
2. **snapshot.go** `Analyze()`: After `computeResultStats()`, compute detectability:
   ```go
   alpha := 0.05
   if s.config.detectabilityAlpha > 0 {
       alpha = s.config.detectabilityAlpha
   }
   results.Detectability = analysis.Detectability(tensionValues, alpha)
   ```
3. **snapshot.go** `AnalyzeRegion()`: Same pattern after collecting tensionValues.

**Acceptance**: `Results.Detectability.Region` is populated on every Analyze call. Region matches expected classification for test data.

---

### T08 — Add DetectabilityAlpha to Builder
**Spec**: §2.3
**Files**: builder.go, itt.go
**Depends on**: T07

Steps:
1. Add `detectabilityAlpha float64` field to Builder struct.
2. Add builder method:
   ```go
   func (b *Builder) DetectabilityAlpha(alpha float64) *Builder {
       b.detectabilityAlpha = alpha
       return b
   }
   ```
3. Default: 0.05 in `NewBuilder()`:
   ```go
   detectabilityAlpha: 0.05,
   ```
4. Validation in `Build()`: `0 < alpha < 1`:
   ```go
   if b.detectabilityAlpha <= 0 || b.detectabilityAlpha >= 1 {
       return nil, fmt.Errorf("%w: detectabilityAlpha must be in (0, 1)", ErrInvalidConfig)
   }
   ```

**Acceptance**: Builder accepts custom alpha; validation rejects 0 and 1.

---

### T09 — Add JSD validation warning
**Spec**: §2.4
**Files**: snapshot.go, analysis/divergence.go
**Depends on**: T08

Steps:
1. Add `IsBounded() bool` method to each divergence type in `analysis/`:
   - `JSD.IsBounded() → true`
   - `KL.IsBounded() → false`
   - `Hellinger.IsBounded() → true`
2. Add a `BoundedDivergence` interface in analysis:
   ```go
   type BoundedDivergence interface {
       IsBounded() bool
   }
   ```
3. In `Snapshot.Analyze()`, after computing detectability, if logger is set:
   ```go
   if s.config.logger != nil && s.config.divergence != nil {
       if bd, ok := s.config.divergence.(analysis.BoundedDivergence); ok && !bd.IsBounded() {
           s.config.logger.Warn("detectability results may be unreliable with unbounded divergence; JSD or Hellinger recommended")
       }
   }
   ```
   This only logs once per Analyze call, and only if KL (or custom unbounded) is used.

**Acceptance**: Using KL + detectability logs a warning. Using JSD or Hellinger does not.

---

## Phase 3: Concealment Integration (T10–T13)

### T10 — Wire ConcealmentCalculator into Snapshot analysis
**Spec**: §1.1
**Files**: snapshot.go, types.go
**Parallelizable**: Yes (independent)

Steps:
1. **types.go**: Add `Concealment float64` field to `TensionResult`.
2. **snapshot.go** `Analyze()`: After computing tension per node, if concealment is configured:
   ```go
   if s.config.concealmentLambda > 0 {
       cc := analysis.NewConcealmentCalculator(s.config.concealmentLambda, tc)
       for i, tr := range results {
           results[i].Concealment = cc.CalculateNode(gv, tr.NodeID, s.config.concealmentHops)
       }
   }
   ```
3. Same for `AnalyzeNode()` and `AnalyzeRegion()`.
4. Add `"concealment"` to `Components` map when computed:
   ```go
   tr.Components["concealment"] = tr.Concealment
   ```

**Acceptance**: With `Concealment(0.5, 3)` configured, `TensionResult.Concealment > 0` for connected nodes.

---

### T11 — Add Concealment builder methods
**Spec**: §1.1
**Files**: builder.go
**Depends on**: T10

Steps:
1. Add fields to Builder:
   ```go
   concealmentLambda float64
   concealmentHops   int
   ```
2. Add builder method:
   ```go
   func (b *Builder) Concealment(lambda float64, maxHops int) *Builder {
       b.concealmentLambda = lambda
       b.concealmentHops = maxHops
       return b
   }
   ```
3. Validation in `Build()`:
   ```go
   if b.concealmentLambda < 0 {
       return nil, fmt.Errorf("%w: concealmentLambda must be >= 0", ErrInvalidConfig)
   }
   if b.concealmentHops < 0 {
       return nil, fmt.Errorf("%w: concealmentHops must be >= 0", ErrInvalidConfig)
   }
   ```
4. No default (0 = disabled).

**Acceptance**: Builder accepts concealment config; validation rejects negative values.

---

### T12 — Implement CPS function
**Spec**: §1.2
**Files**: analysis/detectability.go
**Depends on**: T06

Steps:
1. Add `CPS` function to `analysis/detectability.go`:
   ```go
   // CPS computes the Concealment Probability Score.
   // Returns 0 if SNR <= Yharim limit (below detectability threshold).
   // Otherwise returns a normalized concealment score in [0, 1].
   //
   // Formula: CPS(Σ) = normalize(Ω) · 1_{SNR > Υ(α)}
   //
   // Normalization: sigmoid(Ω / mean_tension) clamped to [0, 1].
   // This maps concealment cost relative to the mean tension into a probability-like score.
   func CPS(tensions []float64, concealmentCost float64, alpha float64) float64
   ```
2. Implementation:
   ```go
   if !IsDetectable(tensions, alpha) {
       return 0
   }
   if len(tensions) == 0 || concealmentCost <= 0 {
       return 0
   }
   meanT := mean(tensions)
   if meanT <= 0 {
       return 0
   }
   ratio := concealmentCost / meanT
   // Sigmoid normalization: 2/(1+exp(-ratio)) - 1, maps (0,∞) → (0,1)
   return 2.0/(1.0+math.Exp(-ratio)) - 1.0
   ```

**Acceptance**: CPS returns 0 when SNR < Yharim. CPS returns (0,1) when detectable. Higher concealment cost → higher CPS.

---

### T13 — Wire CPS into RegionResult
**Spec**: §1.2
**Files**: types.go, snapshot.go
**Depends on**: T12, T10

Steps:
1. **types.go**: Add `CPS float64` field to `RegionResult`.
2. **snapshot.go** `AnalyzeRegion()`: After computing tension values and concealment:
   ```go
   if s.config.concealmentLambda > 0 {
       // Sum concealment costs for the region
       totalConcealment := 0.0
       for _, tr := range nodes {
           totalConcealment += tr.Concealment
       }
       alpha := s.config.detectabilityAlpha
       if alpha <= 0 {
           alpha = 0.05
       }
       region.CPS = analysis.CPS(tensionValues, totalConcealment, alpha)
   }
   ```

**Acceptance**: `RegionResult.CPS` populated when concealment is configured. Zero when not detectable.

---

## Phase 4: Temporal Dynamics (T14–T24)

### T14 — Implement TensionHistory ring buffer
**Spec**: §3.1
**Files**: analysis/temporal.go (new file)
**Parallelizable**: Yes (independent)

Steps:
1. Create `analysis/temporal.go`.
2. Implement `TensionSample` struct (Tension float64, Timestamp time.Time, Version uint64).
3. Implement `TensionHistory` as a fixed-size circular buffer:
   - `samples []TensionSample`, `size int`, `head int`, `count int`
   - `NewTensionHistory(capacity int)` — allocates slice
   - `Push(s)` — overwrites oldest when full, advances head
   - `Latest()` — returns `samples[(head-1+size)%size]` if count > 0
   - `Previous()` — returns sample before latest if count > 1
   - `Slice()` — returns all samples in chronological order (oldest first)
   - `Len()` — returns min(count, size)

**Acceptance**: Unit test: push 150 samples into capacity-100 buffer → Len()=100, Slice() returns last 100 in order, Latest() returns last pushed.

---

### T15 — Implement TemporalCalculator + TemporalIndicators
**Spec**: §3.2
**Files**: analysis/temporal.go
**Depends on**: T14

Steps:
1. Implement `TemporalIndicators` struct (TensionSpike, DecayExponent, CurvatureShock, Timestamp).
2. Implement `TemporalCalculator` with `alpha float64`.
3. `Indicators(current, previous map[string]float64, dt time.Duration) TemporalIndicators`:
   - **TensionSpike**: `max over all nodes |current[v] - previous[v]|`
     - Include nodes that exist in only one map (treat missing as 0).
   - **DecayExponent**: `γ(t) = -d/dt log(τ̄(t))`
     - Compute mean of current and mean of previous
     - `γ = -(log(meanCurrent) - log(meanPrevious)) / dt.Seconds()`
     - Guard: if meanCurrent ≤ 0 or meanPrevious ≤ 0, γ = 0
   - **CurvatureShock**: Set to 0 (computed externally when curvature data available; see T20).
   - **Timestamp**: `time.Now()`

4. Add `IndicatorsWithCurvature(current, previous map[string]float64, currentCurv, prevCurv map[[2]string]float64, dt time.Duration) TemporalIndicators`:
   - Same as above but also computes CurvatureShock:
   - `max over all edges |currentCurv[e] - prevCurv[e]|`

**Acceptance**: Unit tests for spike detection, decay exponent sign (positive = recovery, negative = growth), curvature shock.

---

### T16 — Implement ClassifyPhase
**Spec**: §3.3
**Files**: analysis/temporal.go
**Depends on**: T15

Steps:
1. Implement `Phase` type (int enum: FullRecovery, ScarredRecovery, ChronicTension, StructuralCollapse).
2. Implement `PhaseResult` struct (Phase, Rho, Pi).
3. `ClassifyPhase(indicators, meanTension, prevMeanTension, connectivityRatio) PhaseResult`:
   - Compute ρ (suppression intensity):
     - `rho = indicators.DecayExponent` mapped to intensity scale
     - If `DecayExponent < 0` (tension growing): `rho = |DecayExponent|` (high intensity)
     - If `DecayExponent > 0` (tension decaying): `rho = 0` (low intensity)
     - If `DecayExponent ≈ 0` and `meanTension > threshold`: `rho = meanTension` (chronic)
     - Normalize: `rho_norm = min(rho / rho_c, 2.0)` where `rho_c = 1.0`
   - Compute π (healing capacity):
     - `pi = connectivityRatio` — how much structure survived
     - High connectivity (≥ 0.5) = high healing capacity
   - Classify:
     ```
     rho < rho_c && pi >= pi_c → Phase I  (Full Recovery)
     rho < rho_c && pi <  pi_c → Phase II (Scarred Recovery)
     rho >= rho_c && pi >= pi_c → Phase III (Chronic Tension)
     rho >= rho_c && pi <  pi_c → Phase IV (Structural Collapse)
     ```
     Where `rho_c = 1.0`, `pi_c = 0.5`.
4. Add `String()` method on `Phase`.

**Acceptance**: Unit tests for each of the 4 phases with known inputs.

---

### T17 — Implement Fiedler value approximation
**Spec**: §3.4
**Files**: analysis/laplacian.go (new file)
**Parallelizable**: Yes (independent)

Steps:
1. Create `analysis/laplacian.go`.
2. Implement `FiedlerValue(g GraphView, nodeIDs []string, maxIter int, tol float64) float64`:
   - Build graph Laplacian L = D - A (degree matrix - adjacency matrix) for the given nodeIDs.
   - Represent as sparse map since GraphView is sparse.
   - Use **inverse power iteration** to find smallest non-zero eigenvalue:
     a. Start with random vector orthogonal to the Fiedler vector (constant vector).
     b. Iterate: solve Lx = b approximately (using Jacobi or Gauss-Seidel).
     c. Converge when |λ_new - λ_old| < tol.
   - Return 0 if graph is disconnected.
3. Implement `FiedlerApprox(g GraphView, nodeIDs []string) float64`:
   - Cheeger inequality: λ₁ ≥ h²/(2·d_max)
   - Compute Cheeger constant h (edge expansion) via BFS-based approximation:
     - Pick random subsets S, compute |E(S, V\S)| / min(vol(S), vol(V\S))
     - Use minimum over several random splits
   - Much faster but gives lower bound only.

**Acceptance**: FiedlerValue on a known graph (e.g., path graph P_n: λ₁ = 2·(1 - cos(π/n))) matches within tolerance. FiedlerApprox returns a positive lower bound for connected graphs.

---

### T18 — Implement VelocityOfSilence + EstimateAge
**Spec**: §3.4
**Files**: analysis/temporal.go
**Depends on**: T17

Steps:
1. `VelocityOfSilence(alpha, lambda1, meanEdgeLength float64) float64`:
   ```go
   if lambda1 <= 0 || meanEdgeLength <= 0 {
       return 0
   }
   return alpha * math.Sqrt(lambda1) * meanEdgeLength
   ```
2. `EstimateAge(distance float64, velocity float64) time.Duration`:
   ```go
   if velocity <= 0 {
       return 0
   }
   seconds := distance / velocity
   return time.Duration(seconds * float64(time.Second))
   ```

**Acceptance**: Known inputs produce expected outputs. EstimateAge(10, 5) = 2s.

---

### T19 — Add temporal state to Engine
**Spec**: §3.5
**Files**: engine.go, builder.go
**Depends on**: T14

Steps:
1. **builder.go**: Add fields:
   ```go
   temporalCapacity    int           // ring buffer size (default: 100)
   diffusivityAlpha    float64       // α for diffusion (default: 0.1)
   onTensionSpike      func(nodeID string, delta float64)
   tensionSpikeThreshold float64     // default: 0.3
   ```
2. **builder.go**: Add methods:
   ```go
   func (b *Builder) TemporalCapacity(n int) *Builder
   func (b *Builder) DiffusivityAlpha(alpha float64) *Builder
   func (b *Builder) OnTensionSpike(f func(string, float64)) *Builder
   func (b *Builder) TensionSpikeThreshold(t float64) *Builder
   ```
3. **itt.go**: Add defaults to `NewBuilder()`:
   ```go
   temporalCapacity:      100,
   diffusivityAlpha:      0.1,
   tensionSpikeThreshold: 0.3,
   ```
4. **engine.go**: Add fields to Engine:
   ```go
   tensionHistory   map[string]*analysis.TensionHistory
   tensionHistoryMu sync.RWMutex
   lastTensions     map[string]float64 // previous snapshot's tensions (for indicators)
   lastTensionsMu   sync.RWMutex
   ```
5. Initialize in `newEngine()`:
   ```go
   e.tensionHistory = make(map[string]*analysis.TensionHistory)
   e.lastTensions = make(map[string]float64)
   ```
6. In `checkAnomalies()`, after computing tension for a node, push to history:
   ```go
   e.tensionHistoryMu.Lock()
   h, ok := e.tensionHistory[nodeID]
   if !ok {
       h = analysis.NewTensionHistory(e.config.temporalCapacity)
       e.tensionHistory[nodeID] = h
   }
   h.Push(analysis.TensionSample{Tension: t, Timestamp: ev.Timestamp, Version: version})
   e.tensionHistoryMu.Unlock()
   ```

**Acceptance**: After ingesting events, `tensionHistory` contains entries for dirty nodes. Ring buffer doesn't grow unbounded.

---

### T20 — Wire temporal computation into Snapshot.Analyze
**Spec**: §3.6
**Files**: snapshot.go
**Depends on**: T15, T19

Steps:
1. Add `tensionHistory map[string]*analysis.TensionHistory` and `tensionHistoryMu *sync.RWMutex` fields to Snapshot struct.
2. In `Engine.Snapshot()`: pass references:
   ```go
   snap.tensionHistory = e.tensionHistory
   snap.tensionHistoryMu = &e.tensionHistoryMu
   ```
3. In `Snapshot.Analyze()`, after computing all current tensions, build `currentTensions` map and compute temporal indicators:
   ```go
   // Build current tension map
   currentTensions := make(map[string]float64, len(results))
   for _, tr := range results {
       currentTensions[tr.NodeID] = tr.Tension
   }

   // Get previous tensions from history
   s.tensionHistoryMu.RLock()
   prevTensions := make(map[string]float64)
   for nodeID, h := range s.tensionHistory {
       if prev, ok := h.Previous(); ok {
           prevTensions[nodeID] = prev.Tension
       }
   }
   s.tensionHistoryMu.RUnlock()

   // Compute temporal indicators if we have history
   if len(prevTensions) > 0 {
       tempCalc := analysis.NewTemporalCalculator(s.config.diffusivityAlpha)
       dt := /* estimate from timestamps */ time.Since(s.version.Timestamp)
       indicators := tempCalc.Indicators(currentTensions, prevTensions, dt)
       // ... populate TemporalSummary
   }
   ```

**Acceptance**: Second call to Analyze() returns non-zero TemporalIndicators. First call returns zero-valued temporal summary.

---

### T21 — Add Trend to TensionResult
**Spec**: §3.6
**Files**: types.go, snapshot.go
**Depends on**: T14, T19

Steps:
1. **types.go**: Add `Trend` type and constants:
   ```go
   type Trend int
   const (
       TrendStable     Trend = iota
       TrendIncreasing
       TrendDecreasing
   )
   ```
2. **types.go**: Add `Trend Trend` field to `TensionResult`.
3. **snapshot.go** `Analyze()`: For each node, check history:
   ```go
   s.tensionHistoryMu.RLock()
   if h, ok := s.tensionHistory[nodeID]; ok {
       if prev, ok := h.Previous(); ok {
           delta := t - prev.Tension
           epsilon := 0.01 // stability threshold
           if delta > epsilon {
               tr.Trend = TrendIncreasing
           } else if delta < -epsilon {
               tr.Trend = TrendDecreasing
           }
           // else TrendStable (zero value)
       }
   }
   s.tensionHistoryMu.RUnlock()
   ```
4. Same pattern for `AnalyzeNode()`.

**Acceptance**: After tension increases between analyses, `TensionResult.Trend == TrendIncreasing`.

---

### T22 — Add TemporalSummary to Results
**Spec**: §3.6
**Files**: types.go, snapshot.go
**Depends on**: T15, T16, T18, T20

Steps:
1. **types.go**: Add types:
   ```go
   type TemporalSummary struct {
       Indicators analysis.TemporalIndicators
       Phase      analysis.PhaseResult
       Velocity   float64
   }
   ```
   Add `Temporal TemporalSummary` field to `Results`.
2. **snapshot.go** `Analyze()`: After T20's temporal computation:
   ```go
   if len(prevTensions) > 0 {
       indicators := tempCalc.Indicators(currentTensions, prevTensions, dt)

       // Phase classification
       prevMean := mean(prevTensions values)
       connectivityRatio := computeConnectivityRatio(gv, prevTensions)
       phase := analysis.ClassifyPhase(indicators, stats.MeanTension, prevMean, connectivityRatio)

       // Velocity of silence (if graph is large enough)
       velocity := 0.0
       nodeIDs := collectNodeIDs(gv)
       if len(nodeIDs) >= 3 {
           lambda1 := analysis.FiedlerApprox(gv, nodeIDs)
           meanEdgeLen := computeMeanEdgeWeight(gv)
           velocity = analysis.VelocityOfSilence(s.config.diffusivityAlpha, lambda1, meanEdgeLen)
       }

       results.Temporal = TemporalSummary{
           Indicators: indicators,
           Phase:      phase,
           Velocity:   velocity,
       }
   }
   ```
3. Helper `computeConnectivityRatio`: count edges in current graph whose endpoints also existed in prevTensions map, divide by total previous edges. Approximation: `len(currentEdges) / max(len(prevEdges), 1)`.

**Acceptance**: `Results.Temporal.Phase.Phase` returns correct phase classification for known scenarios. Velocity > 0 for connected graphs.

---

### T23 — Implement OnTensionSpike callback
**Spec**: §3.7
**Files**: engine.go, builder.go
**Depends on**: T19

Steps:
1. Builder fields and methods already added in T19.
2. In `checkAnomalies()`, after pushing to history AND before anomaly check:
   ```go
   // Check for tension spike
   if e.config.onTensionSpike != nil {
       e.tensionHistoryMu.RLock()
       if h, ok := e.tensionHistory[nodeID]; ok {
           if prev, ok := h.Previous(); ok {
               delta := math.Abs(t - prev.Tension)
               if delta > e.config.tensionSpikeThreshold {
                   spike := delta
                   nid := nodeID
                   e.safeCallback(func() {
                       e.config.onTensionSpike(nid, spike)
                   })
               }
           }
       }
       e.tensionHistoryMu.RUnlock()
   }
   ```

**Acceptance**: OnTensionSpike fires when a node's tension jumps by more than the threshold. Does not fire for gradual changes.

---

### T24 — Wire DeltaTensionChanged
**Spec**: §3.8
**Files**: engine.go
**Depends on**: T19, T23

Steps:
1. In `checkAnomalies()`, track the previous trend per node (need to compare consecutive samples):
   ```go
   // Determine trend
   var currentTrend Trend = TrendStable
   e.tensionHistoryMu.RLock()
   if h, ok := e.tensionHistory[nodeID]; ok {
       if prev, ok := h.Previous(); ok {
           delta := t - prev.Tension
           if delta > 0.01 { currentTrend = TrendIncreasing }
           if delta < -0.01 { currentTrend = TrendDecreasing }
       }
   }
   e.tensionHistoryMu.RUnlock()
   ```
2. Emit `DeltaTensionChanged` when trend changes:
   - Need to store last known trend per node. Add `lastTrend map[string]Trend` + mutex to Engine.
   - If `currentTrend != lastTrend[nodeID]` and both are non-zero (i.e., not first observation):
     ```go
     if e.config.onChange != nil {
         e.safeCallback(func() {
             e.config.onChange(Delta{
                 Type:      DeltaTensionChanged,
                 Timestamp: ev.Timestamp,
                 Version:   version,
                 NodeID:    nodeID,
                 Tension:   t,
             })
         })
     }
     ```
   - Update `lastTrend[nodeID] = currentTrend`.

**Acceptance**: `DeltaTensionChanged` emitted when a node transitions from `TrendStable` → `TrendIncreasing`. Not emitted when trend stays the same.

---

## Phase 5: Tests (T25–T27)

### T25 — Unit tests
**Spec**: §Tests
**Files**: analysis/detectability_test.go, analysis/temporal_test.go, analysis/laplacian_test.go, engine_test.go
**Depends on**: T01–T24

Test coverage per feature:

**analysis/detectability_test.go**:
- `TestClassify_StronglyDetectable` — high SNR
- `TestClassify_WeaklyDetectable` — medium SNR
- `TestClassify_Undetectable` — low SNR
- `TestClassify_EmptyTensions` — edge case
- `TestDetectability_FullResult` — verify all fields
- `TestCPS_BelowThreshold` — returns 0
- `TestCPS_AboveThreshold` — returns (0,1)
- `TestCPS_HighConcealment` — higher cost → higher CPS
- `TestCPS_ZeroCost` — returns 0
- `TestDetectabilityRegion_String`

**analysis/temporal_test.go**:
- `TestTensionHistory_PushAndSlice` — basic ring buffer
- `TestTensionHistory_Overflow` — wraps correctly
- `TestTensionHistory_Latest` — returns most recent
- `TestTensionHistory_Previous` — returns second most recent
- `TestTensionHistory_Empty` — returns false
- `TestTemporalIndicators_TensionSpike` — max delta detected
- `TestTemporalIndicators_DecayExponent_Positive` — tension decaying
- `TestTemporalIndicators_DecayExponent_Negative` — tension growing
- `TestTemporalIndicators_CurvatureShock` — curvature disruption
- `TestClassifyPhase_FullRecovery`
- `TestClassifyPhase_ScarredRecovery`
- `TestClassifyPhase_ChronicTension`
- `TestClassifyPhase_StructuralCollapse`
- `TestVelocityOfSilence` — known inputs
- `TestEstimateAge` — distance / velocity
- `TestPhase_String`

**analysis/laplacian_test.go**:
- `TestFiedlerValue_PathGraph` — known eigenvalue
- `TestFiedlerValue_CompleteGraph` — λ₁ = n for K_n
- `TestFiedlerValue_DisconnectedGraph` — returns 0
- `TestFiedlerApprox_PositiveForConnected`
- `TestFiedlerApprox_ZeroForDisconnected`

**engine_test.go** (additions):
- `TestDeltaFieldsPopulated` — Node, Edge, Previous fields non-nil
- `TestBaseGraphInitialization` — snapshot sees pre-loaded data
- `TestStorageLoadOnStart` — mock storage loaded
- `TestStorageSaveOnCompact` — mock storage saved after compact
- `TestDetectabilityInResults` — Analyze returns detectability
- `TestConcealmentInResults` — Analyze returns concealment cost
- `TestCPSInRegionResult` — AnalyzeRegion returns CPS
- `TestTensionHistoryPopulated` — history grows after events
- `TestOnTensionSpike` — callback fires on spike
- `TestDeltaTensionChanged` — delta emitted on trend change
- `TestTemporalSummaryInResults` — Analyze returns temporal indicators
- `TestTrendInTensionResult` — correct trend direction
- `TestCurvatureFuncAdapter` — custom CurvatureFunc works
- `TestJSDWarningLogged` — KL + detectability logs warning
- `TestDeadCodeRemoved` — verify removed symbols don't exist (build test)

**Acceptance**: All new tests pass. `go test ./... -count=1` green.

---

### T26 — Integration test
**Spec**: §Tests
**Files**: integration_test.go
**Depends on**: T25

`TestFullTemporalLifecycle`:
1. **Setup**: Build engine with concealment, detectability, temporal, curvature, and OnTensionSpike.
2. **Phase A — Baseline**: Ingest 50 normal events. Analyze → `TrendStable`, `Phase I`.
3. **Phase B — Anomaly injection**: Ingest 10 events with extreme weight to one node. Wait. Verify `OnTensionSpike` fired.
4. **Phase C — Analysis during anomaly**: Analyze → `TrendIncreasing`, `Phase III or IV`. Verify `Detectability.Region == StronglyDetectable`. Verify `Concealment > 0`. Verify `CPS > 0` for region.
5. **Phase D — Recovery**: Ingest 50 more normal events. Analyze → `TrendDecreasing`, phase shifts toward recovery.
6. **Phase E — Verify deltas**: Check that `DeltaTensionChanged` was emitted at least once.
7. **Phase F — Temporal summary**: Verify `Results.Temporal.Indicators.TensionSpike > 0` during anomaly phase. Verify `Velocity > 0`.

**Acceptance**: Full lifecycle passes. All phases produce expected classifications.

---

### T27 — Benchmarks
**Spec**: §Tests
**Files**: bench_test.go
**Depends on**: T25

New benchmarks:
- `BenchmarkAnalyzeWithConcealment` — Analyze 100 nodes with concealment enabled (lambda=0.5, hops=2).
- `BenchmarkAnalyzeWithDetectability` — Analyze 100 nodes with detectability computation.
- `BenchmarkAnalyzeWithTemporal` — Analyze 100 nodes with temporal indicators + history.
- `BenchmarkTensionHistoryPush` — push into ring buffer (should be <10ns).
- `BenchmarkFiedlerApprox` — Fiedler approximation for 100/1000 node graphs.
- `BenchmarkCheckAnomaliesWithTemporal` — per-event overhead of temporal tracking in processEvent.

**Acceptance**: No regression in existing benchmarks (AddEvent, Snapshot). New benchmarks run without OOM. Temporal overhead per event < 500ns.

---

## Dependency Graph (visual)

```
Phase 1 (cleanup):
  T01 ─────────────────────────────────
  T02 ─────────────────────────────────  (all parallel)
  T03 ─────────────────────────────────
  T04 ─── T05 (sequential)

Phase 2 (detectability):
  T06 ─── T07 ─── T08 ─── T09

Phase 3 (concealment):
  T10 ─── T11
  T06 ─── T12 ─── T13 (needs T10 too)

Phase 4 (temporal):
  T14 ─── T15 ─── T16
  T17 ─── T18
  T14 ─── T19 ─── T20 (needs T15 too)
                 ├─ T21
                 ├─ T22 (needs T15, T16, T18, T20)
                 ├─ T23
                 └─ T24 (needs T23)

Phase 5 (tests):
  T25 ─── T26
       └── T27 (parallel with T26)
```

## Parallel Execution Opportunities

Within each phase, the following can run in parallel:

| Phase | Parallel Groups |
|-------|----------------|
| P1 | [T01, T02, T03, T04] then T05 |
| P2 | T06 first, then [T07, T08, T09] sequentially |
| P3 | [T10, T12] in parallel (both depend on T06), then [T11, T13] |
| P4 | [T14, T17] first, then [T15, T19, T18], then [T16, T20, T21, T23], then [T22, T24] |
| P5 | T25 first, then [T26, T27] in parallel |

---

## Estimated Test Counts (post-v2)

| Package | Current | New | Total |
|---------|---------|-----|-------|
| Root (itt) | ~57 | ~18 | ~75 |
| analysis/ | ~55 | ~26 | ~81 |
| graph/ | ~25 | 0 | ~25 |
| mvcc/ | ~14 | 0 | ~14 |
| compact/ | ~9 | 0 | ~9 |
| export/ | ~10 | 0 | ~10 |
| **Total** | **~186** | **~44** | **~230** |

Plus 6 new benchmarks (6 existing + 6 new = 12 total).

---

## Phase 6: Documentation (T28–T30)

### T28 — Update README.md with v2 features
**Spec**: Documentation
**Files**: README.md
**Depends on**: T25

Steps:
1. Add **Detectability Framework** section:
   - Explain the three regions (Undetectable, Weakly, Strongly Detectable)
   - Show `Results.Detectability` usage
   - Explain SNR, Yharim limit, and why JSD is required
   - Show `DetectabilityAlpha` builder config
2. Add **Concealment Analysis** section:
   - Explain concealment cost Ω(v) and what it measures
   - Show `Builder.Concealment(lambda, maxHops)` usage
   - Show `TensionResult.Concealment` in output
   - Explain CPS (Concealment Probability Score) in `RegionResult`
3. Add **Temporal Dynamics** section:
   - Explain tension history, trends, temporal indicators
   - Show `TemporalSummary` in Results (indicators, phase, velocity)
   - Explain the 4 phases (Full Recovery, Scarred Recovery, Chronic Tension, Structural Collapse)
   - Show `OnTensionSpike` callback usage
   - Explain velocity of silence and age estimation
   - Show builder config: `TemporalCapacity`, `DiffusivityAlpha`, `TensionSpikeThreshold`
4. Update **Builder API** section with all new methods.
5. Update **Callbacks** section with `OnTensionSpike`.
6. Update **Delta types** with `DeltaTensionChanged`.
7. Update **Performance** table with new benchmark results.
8. Add **Storage** section explaining persistence via `Storage` interface and `BaseGraph` initialization.

**Acceptance**: README covers all v2 features with code examples. A new user can understand and use every feature from the README alone.

---

### T29 — Write API Reference documentation
**Spec**: Documentation
**Files**: docs/API-REFERENCE.md (new file)
**Depends on**: T25

Complete API reference organized by package:

1. **Root package (`itt`)** — every public type, method, function, constant:
   - `NewBuilder() *Builder` — all builder methods with signatures and defaults
   - `Engine` — Start, Stop, AddEvent, AddEvents, Analyze, AnalyzeNode, AnalyzeRegion, Snapshot, Compact, Reset, Stats
   - `Snapshot` — all methods including Export, Timestamp, Close
   - Types: Event, Node, Edge, TensionResult (with Concealment, Trend), Results (with Detectability, Temporal), RegionResult (with CPS, Detectability), EngineStats, CompactStats, GCStats, Delta, DeltaType, Trend, ExportFormat, CompactionStrategy
   - Interfaces: DivergenceFunc, CurvatureFunc, Storage, Calibrator, Logger, NodeTypeFunc, ThresholdFunc, WeightFunc, AggregationFunc
   - Built-in aggregations: AggMean, AggMax, AggMedian, AggSum
   - Errors: all Err* variables
2. **`analysis/` package**:
   - Divergence: JSD, KL, Hellinger — Compute, Name, IsBounded
   - TensionCalculator — Calculate, CalculateAll
   - CurvatureCalculator — Calculate, CalculateAll
   - MADCalibrator — NewMADCalibrator, Observe, IsWarmedUp, Threshold, IsAnomaly, Stats, Recalibrate
   - ConcealmentCalculator — Calculate, CalculateNode
   - Detectability: DetectabilityRegion, DetectabilityResult, Classify, Detectability, CPS
   - Temporal: TensionSample, TensionHistory, TemporalIndicators, TemporalCalculator, Phase, PhaseResult, ClassifyPhase, VelocityOfSilence, EstimateAge
   - Laplacian: FiedlerValue, FiedlerApprox
   - Yharim: YharimLimit, SNR, IsDetectable
   - GraphView interface
3. **`graph/` package**: Graph, ImmutableGraph, UnifiedView, NodeData, EdgeData
4. **`mvcc/` package**: Version, Controller, GC, GCConfig
5. **`compact/` package**: Compact, Stats
6. **`export/` package**: JSON, DOT, GraphView interface

Each entry includes: signature, description, parameters, return values, example.

**Acceptance**: Every public symbol in the codebase is documented. `go doc` output matches.

---

### T30 — Write Architecture & Theory Guide
**Spec**: Documentation
**Files**: docs/ARCHITECTURE.md (new file)
**Depends on**: T28

A guide connecting the theory to the implementation:

1. **Theory Overview**: What is Informational Tension Theory — 1-page summary for developers.
   - Tension τ(v) = D(P_obs || P_exp)
   - Why JSD (bounded, symmetric, valid for power-law networks)
   - Curvature κ(x,y) via Sinkhorn OT
   - MAD calibration for heavy-tail robustness
2. **Concealment Theory**:
   - Concealment cost Ω and superlinear scaling
   - CPS: Concealment Probability Score
   - "Perfect concealment is impossible above the detectability threshold"
3. **Detectability Theory**:
   - Yharim limit Υ = √(2·ln(1/α))
   - SNR and the three detectability regions
   - Why JSD is mathematically necessary (boundedness → finite moments → EVT)
4. **Temporal Dynamics**:
   - Tension diffusion equation ∂τ/∂t = -αLτ + S(v,t)
   - Temporal indicators: tension spike, decay exponent, curvature shock
   - Phase diagram: 4 phases with ρ (suppression intensity) and π (healing capacity)
   - Velocity of silence and age estimation
   - Fiedler value (algebraic connectivity) and its role
5. **Architecture Diagram**:
   ```
   Event → Engine → processEvent → MVCC Version
                  → checkAnomalies → TensionHistory → OnTensionSpike
                                   → OnAnomaly
                                   → DeltaTensionChanged
                  → shouldCompact → doCompact → Storage.Save
                  → gcWorker → GC.Collect

   Engine.Snapshot() → Snapshot(base + overlay)
                     → Analyze() → Tension
                                 → Curvature
                                 → Concealment
                                 → Detectability
                                 → Temporal (indicators, phase, velocity)
                                 → Results
   ```
6. **Package Dependency Graph**: Layered diagram showing which package imports which.
7. **Concurrency Model**: Explain MVCC, snapshot isolation, mutex usage, goroutines.
8. **Extension Points**: How to implement custom DivergenceFunc, CurvatureFunc, Storage, Calibrator, etc.

**Acceptance**: A developer unfamiliar with ITT can read this document and understand both the theory and the codebase architecture.

---

## Updated Traceability Matrix (T28–T30)

| Task | Spec Section | Phase | Depends On | Files |
|------|-------------|-------|------------|-------|
| T28 | Documentation | P6 | T25 | README.md |
| T29 | Documentation | P6 | T25 | docs/API-REFERENCE.md |
| T30 | Documentation | P6 | T28 | docs/ARCHITECTURE.md |

## Updated Dependency Graph

```
Phase 6 (documentation):
  T28 ─── T30
  T29 ──────── (parallel with T28)
```

## Updated Parallel Execution Opportunities

| Phase | Parallel Groups |
|-------|----------------|
| P1 | [T01, T02, T03, T04] then T05 |
| P2 | T06 first, then [T07, T08, T09] sequentially |
| P3 | [T10, T12] in parallel (both depend on T06), then [T11, T13] |
| P4 | [T14, T17] first, then [T15, T19, T18], then [T16, T20, T21, T23], then [T22, T24] |
| P5 | T25 first, then [T26, T27] in parallel |
| P6 | [T28, T29] in parallel, then T30 |
