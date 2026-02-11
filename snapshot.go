package itt

import (
	"fmt"
	"io"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MatheusGrego/itt-engine/analysis"
	"github.com/MatheusGrego/itt-engine/cache"
	"github.com/MatheusGrego/itt-engine/export"
	"github.com/MatheusGrego/itt-engine/graph"
	"github.com/MatheusGrego/itt-engine/mvcc"
)

// Snapshot is a read-only, point-in-time view of the graph.
type Snapshot struct {
	version *mvcc.Version
	config  *Builder
	base    *graph.ImmutableGraph
	closed  bool
	mu      sync.Mutex
	onClose func()

	// Cache (Phase 2)
	cache *cache.ResultsCache

	tensionHistory   map[string]*analysis.TensionHistory
	tensionHistoryMu *sync.RWMutex
	diffusivityAlpha float64
}

func newSnapshot(v *mvcc.Version, cfg *Builder, base *graph.ImmutableGraph, c *cache.ResultsCache) *Snapshot {
	return &Snapshot{version: v, config: cfg, base: base, cache: c}
}

// ID returns the snapshot identifier.
func (s *Snapshot) ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.version == nil {
		return ""
	}
	return fmt.Sprintf("snap-%d", s.version.ID)
}

// Version returns the MVCC version number.
func (s *Snapshot) Version() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.version == nil {
		return 0
	}
	return s.version.ID
}

// Close releases the snapshot's reference to its version.
func (s *Snapshot) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.version != nil {
		s.version.Release()
	}
	if s.onClose != nil {
		s.onClose()
	}
	return nil
}

func (s *Snapshot) checkClosed() error {
	if s.closed {
		return ErrSnapshotClosed
	}
	return nil
}

// graphView returns the graph view to use for reads.
// Must be called with s.mu held.
func (s *Snapshot) graphView() analysis.GraphView {
	if s.base != nil && s.base.NodeCount() > 0 {
		return graph.NewUnifiedView(s.base, s.version.Graph)
	}
	return s.version.Graph
}

// NodeCount returns the number of nodes.
func (s *Snapshot) NodeCount() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return 0, err
	}
	return s.graphView().NodeCount(), nil
}

// EdgeCount returns the number of edges.
func (s *Snapshot) EdgeCount() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return 0, err
	}
	return s.graphView().EdgeCount(), nil
}

// GetNode returns a node by ID.
func (s *Snapshot) GetNode(id string) (*graph.NodeData, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, false, err
	}
	n, ok := s.graphView().GetNode(id)
	return n, ok, nil
}

// GetEdge returns an edge by endpoints.
func (s *Snapshot) GetEdge(from, to string) (*graph.EdgeData, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, false, err
	}
	e, ok := s.graphView().GetEdge(from, to)
	return e, ok, nil
}

// Neighbors returns all neighbor IDs.
func (s *Snapshot) Neighbors(nodeID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	return s.graphView().Neighbors(nodeID), nil
}

// InNeighbors returns incoming neighbor IDs.
func (s *Snapshot) InNeighbors(nodeID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	return s.graphView().InNeighbors(nodeID), nil
}

// OutNeighbors returns outgoing neighbor IDs.
func (s *Snapshot) OutNeighbors(nodeID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	return s.graphView().OutNeighbors(nodeID), nil
}

// ForEachNode iterates all nodes. Return false to stop.
func (s *Snapshot) ForEachNode(fn func(*graph.NodeData) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return err
	}
	s.graphView().ForEachNode(fn)
	return nil
}

// ForEachEdge iterates all edges. Return false to stop.
func (s *Snapshot) ForEachEdge(fn func(*graph.EdgeData) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return err
	}
	s.graphView().ForEachEdge(fn)
	return nil
}

// Timestamp returns the snapshot's creation time.
func (s *Snapshot) Timestamp() (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return time.Time{}, err
	}
	return s.version.Timestamp, nil
}

// Export writes the snapshot's graph in the given format to the writer.
func (s *Snapshot) Export(format ExportFormat, w io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return err
	}

	// Build an export-compatible view.
	var eg export.GraphView
	if s.base != nil && s.base.NodeCount() > 0 {
		eg = graph.NewUnifiedView(s.base, s.version.Graph)
	} else {
		eg = s.version.Graph
	}

	switch format {
	case ExportJSON:
		return export.JSON(w, eg)
	case ExportDOT:
		return export.DOT(w, eg)
	default:
		return fmt.Errorf("%w: unsupported export format", ErrInvalidConfig)
	}
}

// Analyze computes tension for all nodes in the snapshot.
// Returns a Results struct with tensions, anomalies, and stats.
func (s *Snapshot) Analyze() (*Results, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
	}

	// Phase 2: Cache lookup
	if s.cache != nil {
		key := cache.CacheKey{
			VersionID: s.version.ID,
			QueryType: "full_analysis",
			QueryArgs: "",
		}
		if cached, ok := s.cache.Get(key); ok {
			if result, ok := cached.(*Results); ok {
				return result, nil
			}
		}
	}

	start := time.Now()
	gv := s.graphView()

	var div analysis.DivergenceFunc = analysis.JSD{}
	if s.config.divergence != nil {
		if ad, ok := s.config.divergence.(analysis.DivergenceFunc); ok {
			div = ad
		}
	}

	tc := analysis.NewTensionCalculator(div)

	// Use parallel analysis for large graphs (auto-fallback to sequential for < 100 nodes)
	workers := runtime.NumCPU()
	if s.config.parallelWorkers > 0 {
		workers = s.config.parallelWorkers
	}
	tensions := analysis.CalculateAllParallel(tc, gv, workers)

	// Curvature (optional)
	var edgeCurvatures map[[2]string]float64
	if s.config.curvature != nil {
		// Use user-provided CurvatureFunc
		adapter := &graphViewAdapter{gv: gv}
		edgeCurvatures = make(map[[2]string]float64)
		gv.ForEachEdge(func(ed *graph.EdgeData) bool {
			c := s.config.curvature.Compute(adapter, ed.From, ed.To)
			edgeCurvatures[[2]string{ed.From, ed.To}] = c
			return true
		})
	} else if s.config.curvatureAlpha > 0 {
		cc := analysis.NewCurvatureCalculator(s.config.curvatureAlpha)
		edgeCurvatures = cc.CalculateAll(gv)
	}

	// Concealment calculator (optional)
	var concCalc *analysis.ConcealmentCalculator
	if s.config.concealmentLambda > 0 {
		concCalc = analysis.NewConcealmentCalculator(s.config.concealmentLambda, tc)
	}

	var results []TensionResult
	var anomalies []TensionResult
	var tensionValues []float64

	gv.ForEachNode(func(n *graph.NodeData) bool {
		t := tensions[n.ID]
		tensionValues = append(tensionValues, t)

		// Observe in calibrator
		if s.config.calibrator != nil {
			s.config.calibrator.Observe(t)
		}

		// Curvature: mean of incident edges
		curv := 0.0
		if edgeCurvatures != nil {
			curvSum := 0.0
			curvCount := 0
			for key, c := range edgeCurvatures {
				if key[0] == n.ID || key[1] == n.ID {
					curvSum += c
					curvCount++
				}
			}
			if curvCount > 0 {
				curv = curvSum / float64(curvCount)
			}
		}

		anomaly := isAnomaly(s.config, n, t)

		// Confidence: degree-based, capped at 1.0
		confidence := 0.0
		if n.Degree > 0 {
			confidence = math.Min(1.0, float64(n.Degree)/10.0)
		}

		// Concealment (optional)
		concealment := 0.0
		if concCalc != nil {
			concealment = concCalc.CalculateNode(gv, n.ID, s.config.concealmentHops)
		}

		tr := TensionResult{
			NodeID:      n.ID,
			Tension:     t,
			Degree:      n.Degree,
			Curvature:   curv,
			Anomaly:     anomaly,
			Confidence:  confidence,
			Concealment: concealment,
			Components: map[string]float64{
				"tension":     t,
				"curvature":   curv,
				"concealment": concealment,
			},
		}
		results = append(results, tr)
		if anomaly {
			anomalies = append(anomalies, tr)
		}
		return true
	})

	stats := computeResultStats(tensionValues, len(anomalies))

	// Detectability analysis
	detAlpha := 0.05
	if s.config.detectabilityAlpha > 0 {
		detAlpha = s.config.detectabilityAlpha
	}
	det := analysis.Detectability(tensionValues, detAlpha)

	// Warn if using unbounded divergence with detectability
	if s.config.logger != nil && s.config.divergence != nil {
		if bd, ok := s.config.divergence.(interface{ IsBounded() bool }); ok && !bd.IsBounded() {
			s.config.logger.Warn("detectability results may be unreliable with unbounded divergence; JSD or Hellinger recommended")
		}
	}

	// Temporal analysis (requires history from engine)
	var temporal TemporalSummary
	if s.tensionHistory != nil && s.tensionHistoryMu != nil {
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
			tempCalc := analysis.NewTemporalCalculator(s.diffusivityAlpha)
			dt := time.Since(s.version.Timestamp)
			if dt <= 0 {
				dt = time.Millisecond // avoid division by zero
			}
			indicators := tempCalc.Indicators(currentTensions, prevTensions, dt)

			// Compute trends per node
			s.tensionHistoryMu.RLock()
			for i, tr := range results {
				if h, ok := s.tensionHistory[tr.NodeID]; ok {
					if prev, ok := h.Previous(); ok {
						delta := tr.Tension - prev.Tension
						epsilon := 0.01
						if delta > epsilon {
							results[i].Trend = TrendIncreasing
						} else if delta < -epsilon {
							results[i].Trend = TrendDecreasing
						}
					}
				}
			}
			s.tensionHistoryMu.RUnlock()

			// Phase classification
			prevMean := 0.0
			if len(prevTensions) > 0 {
				sum := 0.0
				for _, v := range prevTensions {
					sum += v
				}
				prevMean = sum / float64(len(prevTensions))
			}

			// Connectivity ratio approximation
			connectivityRatio := 1.0 // default: assume no edge loss
			if len(prevTensions) > 0 {
				survived := 0
				for nodeID := range prevTensions {
					if _, ok := currentTensions[nodeID]; ok {
						survived++
					}
				}
				connectivityRatio = float64(survived) / float64(len(prevTensions))
			}

			phase := analysis.ClassifyPhase(indicators, stats.MeanTension, prevMean, connectivityRatio)

			// Velocity of silence
			velocity := 0.0
			nodeIDs := make([]string, 0)
			gv.ForEachNode(func(n *graph.NodeData) bool {
				nodeIDs = append(nodeIDs, n.ID)
				return true
			})
			if len(nodeIDs) >= 3 {
				lambda1 := analysis.FiedlerApprox(gv, nodeIDs)
				// Mean edge weight
				edgeSum := 0.0
				edgeCount := 0
				gv.ForEachEdge(func(e *graph.EdgeData) bool {
					edgeSum += e.Weight
					edgeCount++
					return true
				})
				meanEdgeLen := 1.0
				if edgeCount > 0 {
					meanEdgeLen = edgeSum / float64(edgeCount)
				}
				velocity = analysis.VelocityOfSilence(s.diffusivityAlpha, lambda1, meanEdgeLen)
			}

			temporal = TemporalSummary{
				TensionSpike:   indicators.TensionSpike,
				DecayExponent:  indicators.DecayExponent,
				CurvatureShock: indicators.CurvatureShock,
				Phase:          int(phase.Phase),
				PhaseRho:       phase.Rho,
				PhasePi:        phase.Pi,
				Velocity:       velocity,
			}
		}
	}

	result := &Results{
		Tensions:   results,
		Anomalies:  anomalies,
		Stats:      stats,
		Temporal:   temporal,
		SnapshotID: fmt.Sprintf("snap-%d", s.version.ID),
		AnalyzedAt: time.Now(),
		Duration:   time.Since(start),
		Detectability: DetectabilityResult{
			SNR:       det.SNR,
			Threshold: det.Threshold,
			Region:    int(det.Region),
			Alpha:     det.Alpha,
		},
	}

	// Phase 2: Store in cache
	if s.cache != nil {
		key := cache.CacheKey{
			VersionID: s.version.ID,
			QueryType: "full_analysis",
			QueryArgs: "",
		}
		s.cache.Set(key, result)
	}

	return result, nil
}

// AnalyzeNode computes tension for a single node.
func (s *Snapshot) AnalyzeNode(nodeID string) (*TensionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
	}

	// Phase 2: Cache lookup
	if s.cache != nil {
		key := cache.CacheKey{
			VersionID: s.version.ID,
			QueryType: "node_analysis",
			QueryArgs: nodeID,
		}
		if cached, ok := s.cache.Get(key); ok {
			if result, ok := cached.(*TensionResult); ok {
				return result, nil
			}
		}
	}

	gv := s.graphView()

	n, ok := gv.GetNode(nodeID)
	if !ok {
		return nil, ErrNodeNotFound
	}

	var div analysis.DivergenceFunc = analysis.JSD{}
	if s.config.divergence != nil {
		if ad, ok := s.config.divergence.(analysis.DivergenceFunc); ok {
			div = ad
		}
	}

	tc := analysis.NewTensionCalculator(div)
	t := tc.Calculate(gv, nodeID)

	// Observe in calibrator
	if s.config.calibrator != nil {
		s.config.calibrator.Observe(t)
	}

	// Curvature: mean of incident edges
	curv := 0.0
	if s.config.curvature != nil {
		// Use user-provided CurvatureFunc
		adapter := &graphViewAdapter{gv: gv}
		curvSum := 0.0
		curvCount := 0
		for _, neighbor := range gv.OutNeighbors(nodeID) {
			c := s.config.curvature.Compute(adapter, nodeID, neighbor)
			curvSum += c
			curvCount++
		}
		for _, neighbor := range gv.InNeighbors(nodeID) {
			c := s.config.curvature.Compute(adapter, neighbor, nodeID)
			curvSum += c
			curvCount++
		}
		if curvCount > 0 {
			curv = curvSum / float64(curvCount)
		}
	} else if s.config.curvatureAlpha > 0 {
		cc := analysis.NewCurvatureCalculator(s.config.curvatureAlpha)
		curvSum := 0.0
		curvCount := 0
		for _, neighbor := range gv.OutNeighbors(nodeID) {
			c := cc.Calculate(gv, nodeID, neighbor)
			curvSum += c
			curvCount++
		}
		for _, neighbor := range gv.InNeighbors(nodeID) {
			c := cc.Calculate(gv, neighbor, nodeID)
			curvSum += c
			curvCount++
		}
		if curvCount > 0 {
			curv = curvSum / float64(curvCount)
		}
	}

	anomaly := isAnomaly(s.config, n, t)

	// Confidence: degree-based, capped at 1.0
	confidence := 0.0
	if n.Degree > 0 {
		confidence = math.Min(1.0, float64(n.Degree)/10.0)
	}

	// Concealment (optional)
	concealment := 0.0
	if s.config.concealmentLambda > 0 {
		concCalc := analysis.NewConcealmentCalculator(s.config.concealmentLambda, tc)
		concealment = concCalc.CalculateNode(gv, nodeID, s.config.concealmentHops)
	}

	result := &TensionResult{
		NodeID:      nodeID,
		Tension:     t,
		Degree:      n.Degree,
		Curvature:   curv,
		Anomaly:     anomaly,
		Confidence:  confidence,
		Concealment: concealment,
		Components: map[string]float64{
			"tension":     t,
			"curvature":   curv,
			"concealment": concealment,
		},
	}

	// Phase 2: Store in cache
	if s.cache != nil {
		key := cache.CacheKey{
			VersionID: s.version.ID,
			QueryType: "node_analysis",
			QueryArgs: nodeID,
		}
		s.cache.Set(key, result)
	}

	return result, nil
}

// AnalyzeRegion computes tension for a subset of nodes.
func (s *Snapshot) AnalyzeRegion(nodeIDs []string) (*RegionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
	}

	// Phase 2: Cache lookup
	if s.cache != nil {
		// Sort nodeIDs for consistent cache key
		sortedIDs := make([]string, len(nodeIDs))
		copy(sortedIDs, nodeIDs)
		sort.Strings(sortedIDs)
		key := cache.CacheKey{
			VersionID: s.version.ID,
			QueryType: "region_analysis",
			QueryArgs: strings.Join(sortedIDs, ","),
		}
		if cached, ok := s.cache.Get(key); ok {
			if result, ok := cached.(*RegionResult); ok {
				return result, nil
			}
		}
	}

	gv := s.graphView()

	var div analysis.DivergenceFunc = analysis.JSD{}
	if s.config.divergence != nil {
		if ad, ok := s.config.divergence.(analysis.DivergenceFunc); ok {
			div = ad
		}
	}

	tc := analysis.NewTensionCalculator(div)

	// Curvature calculator (optional)
	var cc *analysis.CurvatureCalculator
	if s.config.curvature == nil && s.config.curvatureAlpha > 0 {
		cc = analysis.NewCurvatureCalculator(s.config.curvatureAlpha)
	}

	var adapter *graphViewAdapter
	if s.config.curvature != nil {
		adapter = &graphViewAdapter{gv: gv}
	}

	// Concealment calculator (optional)
	var concCalc *analysis.ConcealmentCalculator
	if s.config.concealmentLambda > 0 {
		concCalc = analysis.NewConcealmentCalculator(s.config.concealmentLambda, tc)
	}

	var nodes []TensionResult
	var tensionValues []float64
	anomalyCount := 0

	for _, id := range nodeIDs {
		n, ok := gv.GetNode(id)
		if !ok {
			continue // skip missing nodes
		}
		t := tc.Calculate(gv, id)
		tensionValues = append(tensionValues, t)

		// Observe in calibrator
		if s.config.calibrator != nil {
			s.config.calibrator.Observe(t)
		}

		// Curvature: mean of incident edges
		curv := 0.0
		if s.config.curvature != nil {
			curvSum := 0.0
			curvCount := 0
			for _, neighbor := range gv.OutNeighbors(id) {
				c := s.config.curvature.Compute(adapter, id, neighbor)
				curvSum += c
				curvCount++
			}
			for _, neighbor := range gv.InNeighbors(id) {
				c := s.config.curvature.Compute(adapter, neighbor, id)
				curvSum += c
				curvCount++
			}
			if curvCount > 0 {
				curv = curvSum / float64(curvCount)
			}
		} else if cc != nil {
			curvSum := 0.0
			curvCount := 0
			for _, neighbor := range gv.OutNeighbors(id) {
				c := cc.Calculate(gv, id, neighbor)
				curvSum += c
				curvCount++
			}
			for _, neighbor := range gv.InNeighbors(id) {
				c := cc.Calculate(gv, neighbor, id)
				curvSum += c
				curvCount++
			}
			if curvCount > 0 {
				curv = curvSum / float64(curvCount)
			}
		}

		anomaly := isAnomaly(s.config, n, t)

		// Confidence: degree-based, capped at 1.0
		confidence := 0.0
		if n.Degree > 0 {
			confidence = math.Min(1.0, float64(n.Degree)/10.0)
		}

		// Concealment (optional)
		concealment := 0.0
		if concCalc != nil {
			concealment = concCalc.CalculateNode(gv, id, s.config.concealmentHops)
		}

		tr := TensionResult{
			NodeID:      id,
			Tension:     t,
			Degree:      n.Degree,
			Curvature:   curv,
			Anomaly:     anomaly,
			Confidence:  confidence,
			Concealment: concealment,
			Components: map[string]float64{
				"tension":     t,
				"curvature":   curv,
				"concealment": concealment,
			},
		}
		nodes = append(nodes, tr)
		if anomaly {
			anomalyCount++
		}
	}

	mean, maxVal := 0.0, 0.0
	if len(tensionValues) > 0 {
		sum := 0.0
		for _, v := range tensionValues {
			sum += v
			if v > maxVal {
				maxVal = v
			}
		}
		mean = sum / float64(len(tensionValues))
	}

	aggregated := mean
	if s.config.aggregation != nil {
		aggregated = s.config.aggregation(tensionValues)
	}

	// Detectability analysis
	detAlpha := 0.05
	if s.config.detectabilityAlpha > 0 {
		detAlpha = s.config.detectabilityAlpha
	}
	det := analysis.Detectability(tensionValues, detAlpha)

	region := &RegionResult{
		Nodes:        nodes,
		MeanTension:  mean,
		MaxTension:   maxVal,
		AnomalyCount: anomalyCount,
		Aggregated:   aggregated,
		Detectability: DetectabilityResult{
			SNR:       det.SNR,
			Threshold: det.Threshold,
			Region:    int(det.Region),
			Alpha:     det.Alpha,
		},
	}

	// CPS: Concealment Probability Score (optional)
	if s.config.concealmentLambda > 0 {
		totalConcealment := 0.0
		for _, tr := range nodes {
			totalConcealment += tr.Concealment
		}
		region.CPS = analysis.CPS(tensionValues, totalConcealment, detAlpha)
	}

	// Phase 2: Store in cache
	if s.cache != nil {
		// Sort nodeIDs for consistent cache key
		sortedIDs := make([]string, len(nodeIDs))
		copy(sortedIDs, nodeIDs)
		sort.Strings(sortedIDs)
		key := cache.CacheKey{
			VersionID: s.version.ID,
			QueryType: "region_analysis",
			QueryArgs: strings.Join(sortedIDs, ","),
		}
		s.cache.Set(key, region)
	}

	return region, nil
}

// graphViewAdapter wraps analysis.GraphView to satisfy itt.GraphView.
type graphViewAdapter struct {
	gv analysis.GraphView
}

func (a *graphViewAdapter) GetNode(id string) (*Node, bool) {
	n, ok := a.gv.GetNode(id)
	if !ok {
		return nil, false
	}
	return nodeFromGraph(n), true
}

func (a *graphViewAdapter) GetEdge(from, to string) (*Edge, bool) {
	e, ok := a.gv.GetEdge(from, to)
	if !ok {
		return nil, false
	}
	return &Edge{
		From: e.From, To: e.To, Weight: e.Weight,
		Type: e.Type, Count: e.Count,
		FirstSeen: e.FirstSeen, LastSeen: e.LastSeen,
	}, true
}

func (a *graphViewAdapter) Neighbors(nodeID string) []string {
	return a.gv.Neighbors(nodeID)
}

func (a *graphViewAdapter) InNeighbors(nodeID string) []string {
	return a.gv.InNeighbors(nodeID)
}

func (a *graphViewAdapter) OutNeighbors(nodeID string) []string {
	return a.gv.OutNeighbors(nodeID)
}

// nodeFromGraph converts graph.NodeData to itt.Node for callback interfaces.
func nodeFromGraph(n *graph.NodeData) *Node {
	return &Node{
		ID:        n.ID,
		Type:      n.Type,
		Degree:    n.Degree,
		InDegree:  n.InDegree,
		OutDegree: n.OutDegree,
		FirstSeen: n.FirstSeen,
		LastSeen:  n.LastSeen,
	}
}

// isAnomaly checks anomaly status with priority: thresholdFunc > calibrator > static threshold.
func isAnomaly(cfg *Builder, node *graph.NodeData, tension float64) bool {
	if cfg.thresholdFunc != nil {
		return cfg.thresholdFunc(nodeFromGraph(node), tension)
	}
	if cfg.calibrator != nil && cfg.calibrator.IsWarmedUp() {
		return cfg.calibrator.IsAnomaly(tension)
	}
	return tension > cfg.threshold
}

// computeResultStats computes aggregate statistics from a slice of tension values.
func computeResultStats(values []float64, anomalyCount int) ResultStats {
	n := len(values)
	if n == 0 {
		return ResultStats{}
	}

	sum := 0.0
	maxVal := 0.0
	for _, v := range values {
		sum += v
		if v > maxVal {
			maxVal = v
		}
	}
	mean := sum / float64(n)

	// Variance
	varSum := 0.0
	for _, v := range values {
		d := v - mean
		varSum += d * d
	}
	stddev := math.Sqrt(varSum / float64(n))

	// Median
	sorted := make([]float64, n)
	copy(sorted, values)
	sort.Float64s(sorted)
	var median float64
	if n%2 == 0 {
		median = (sorted[n/2-1] + sorted[n/2]) / 2.0
	} else {
		median = sorted[n/2]
	}

	anomalyRate := 0.0
	if n > 0 {
		anomalyRate = float64(anomalyCount) / float64(n)
	}

	return ResultStats{
		NodesAnalyzed: n,
		MeanTension:   mean,
		MedianTension: median,
		MaxTension:    maxVal,
		StdDevTension: stddev,
		AnomalyCount:  anomalyCount,
		AnomalyRate:   anomalyRate,
	}
}
