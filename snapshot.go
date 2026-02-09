package itt

import (
	"fmt"
	"io"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/MatheusGrego/itt-engine/analysis"
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
}

func newSnapshot(v *mvcc.Version, cfg *Builder, base *graph.ImmutableGraph) *Snapshot {
	return &Snapshot{version: v, config: cfg, base: base}
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

	start := time.Now()
	gv := s.graphView()

	var div analysis.DivergenceFunc = analysis.JSD{}
	if s.config.divergence != nil {
		if ad, ok := s.config.divergence.(analysis.DivergenceFunc); ok {
			div = ad
		}
	}

	tc := analysis.NewTensionCalculator(div)
	tensions := tc.CalculateAll(gv)

	// Curvature (optional)
	var edgeCurvatures map[[2]string]float64
	if s.config.curvatureAlpha > 0 {
		cc := analysis.NewCurvatureCalculator(s.config.curvatureAlpha)
		edgeCurvatures = cc.CalculateAll(gv)
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

		tr := TensionResult{
			NodeID:     n.ID,
			Tension:    t,
			Degree:     n.Degree,
			Curvature:  curv,
			Anomaly:    anomaly,
			Confidence: confidence,
			Components: map[string]float64{
				"tension":   t,
				"curvature": curv,
			},
		}
		results = append(results, tr)
		if anomaly {
			anomalies = append(anomalies, tr)
		}
		return true
	})

	stats := computeResultStats(tensionValues, len(anomalies))

	return &Results{
		Tensions:   results,
		Anomalies:  anomalies,
		Stats:      stats,
		SnapshotID: fmt.Sprintf("snap-%d", s.version.ID),
		AnalyzedAt: time.Now(),
		Duration:   time.Since(start),
	}, nil
}

// AnalyzeNode computes tension for a single node.
func (s *Snapshot) AnalyzeNode(nodeID string) (*TensionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
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
	if s.config.curvatureAlpha > 0 {
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

	return &TensionResult{
		NodeID:     nodeID,
		Tension:    t,
		Degree:     n.Degree,
		Curvature:  curv,
		Anomaly:    anomaly,
		Confidence: confidence,
		Components: map[string]float64{
			"tension":   t,
			"curvature": curv,
		},
	}, nil
}

// AnalyzeRegion computes tension for a subset of nodes.
func (s *Snapshot) AnalyzeRegion(nodeIDs []string) (*RegionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
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
	if s.config.curvatureAlpha > 0 {
		cc = analysis.NewCurvatureCalculator(s.config.curvatureAlpha)
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
		if cc != nil {
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

		tr := TensionResult{
			NodeID:     id,
			Tension:    t,
			Degree:     n.Degree,
			Curvature:  curv,
			Anomaly:    anomaly,
			Confidence: confidence,
			Components: map[string]float64{
				"tension":   t,
				"curvature": curv,
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

	return &RegionResult{
		Nodes:        nodes,
		MeanTension:  mean,
		MaxTension:   maxVal,
		AnomalyCount: anomalyCount,
		Aggregated:   aggregated,
	}, nil
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
