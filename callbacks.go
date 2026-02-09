package itt

import "time"

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
