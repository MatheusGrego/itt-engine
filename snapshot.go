package itt

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/MatheusGrego/itt-engine/analysis"
	"github.com/MatheusGrego/itt-engine/graph"
	"github.com/MatheusGrego/itt-engine/mvcc"
)

// Snapshot is a read-only, point-in-time view of the graph.
type Snapshot struct {
	version *mvcc.Version
	config  *Builder
	closed  bool
	mu      sync.Mutex
}

func newSnapshot(v *mvcc.Version, cfg *Builder) *Snapshot {
	return &Snapshot{version: v, config: cfg}
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
	return nil
}

func (s *Snapshot) checkClosed() error {
	if s.closed {
		return ErrSnapshotClosed
	}
	return nil
}

// NodeCount returns the number of nodes.
func (s *Snapshot) NodeCount() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return 0, err
	}
	return s.version.Graph.NodeCount(), nil
}

// EdgeCount returns the number of edges.
func (s *Snapshot) EdgeCount() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return 0, err
	}
	return s.version.Graph.EdgeCount(), nil
}

// GetNode returns a node by ID.
func (s *Snapshot) GetNode(id string) (*graph.NodeData, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, false, err
	}
	n, ok := s.version.Graph.GetNode(id)
	return n, ok, nil
}

// GetEdge returns an edge by endpoints.
func (s *Snapshot) GetEdge(from, to string) (*graph.EdgeData, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, false, err
	}
	e, ok := s.version.Graph.GetEdge(from, to)
	return e, ok, nil
}

// Neighbors returns all neighbor IDs.
func (s *Snapshot) Neighbors(nodeID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	return s.version.Graph.Neighbors(nodeID), nil
}

// InNeighbors returns incoming neighbor IDs.
func (s *Snapshot) InNeighbors(nodeID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	return s.version.Graph.InNeighbors(nodeID), nil
}

// OutNeighbors returns outgoing neighbor IDs.
func (s *Snapshot) OutNeighbors(nodeID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	return s.version.Graph.OutNeighbors(nodeID), nil
}

// ForEachNode iterates all nodes. Return false to stop.
func (s *Snapshot) ForEachNode(fn func(*graph.NodeData) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return err
	}
	s.version.Graph.ForEachNode(fn)
	return nil
}

// ForEachEdge iterates all edges. Return false to stop.
func (s *Snapshot) ForEachEdge(fn func(*graph.EdgeData) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return err
	}
	s.version.Graph.ForEachEdge(fn)
	return nil
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

	var div analysis.DivergenceFunc = analysis.JSD{}
	if s.config.divergence != nil {
		if ad, ok := s.config.divergence.(analysis.DivergenceFunc); ok {
			div = ad
		}
	}

	tc := analysis.NewTensionCalculator(div)
	tensions := tc.CalculateAll(s.version.Graph)

	var results []TensionResult
	var anomalies []TensionResult
	var tensionValues []float64

	s.version.Graph.ForEachNode(func(n *graph.NodeData) bool {
		t := tensions[n.ID]
		tensionValues = append(tensionValues, t)

		isAnomaly := t > s.config.threshold
		tr := TensionResult{
			NodeID:  n.ID,
			Tension: t,
			Degree:  n.Degree,
			Anomaly: isAnomaly,
		}
		results = append(results, tr)
		if isAnomaly {
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

	n, ok := s.version.Graph.GetNode(nodeID)
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
	t := tc.Calculate(s.version.Graph, nodeID)

	isAnomaly := t > s.config.threshold
	return &TensionResult{
		NodeID:  nodeID,
		Tension: t,
		Degree:  n.Degree,
		Anomaly: isAnomaly,
	}, nil
}

// AnalyzeRegion computes tension for a subset of nodes.
func (s *Snapshot) AnalyzeRegion(nodeIDs []string) (*RegionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
	}

	var div analysis.DivergenceFunc = analysis.JSD{}
	if s.config.divergence != nil {
		if ad, ok := s.config.divergence.(analysis.DivergenceFunc); ok {
			div = ad
		}
	}

	tc := analysis.NewTensionCalculator(div)
	var nodes []TensionResult
	var tensionValues []float64
	anomalyCount := 0

	for _, id := range nodeIDs {
		n, ok := s.version.Graph.GetNode(id)
		if !ok {
			continue // skip missing nodes
		}
		t := tc.Calculate(s.version.Graph, id)
		tensionValues = append(tensionValues, t)

		isAnomaly := t > s.config.threshold
		tr := TensionResult{
			NodeID:  id,
			Tension: t,
			Degree:  n.Degree,
			Anomaly: isAnomaly,
		}
		nodes = append(nodes, tr)
		if isAnomaly {
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
