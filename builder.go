package itt

import (
	"fmt"
	"time"
)

// Builder configures and creates an Engine.
type Builder struct {
	// Algorithms
	divergence    DivergenceFunc
	curvature     CurvatureFunc
	topology      TopologyFunc
	threshold     float64
	thresholdFunc ThresholdFunc

	// Weights
	weightFunc   WeightFunc
	nodeTypeFunc NodeTypeFunc
	aggregation  AggregationFunc

	// MVCC
	gcSnapshotWarning time.Duration
	gcSnapshotForce   time.Duration
	maxOverlaySize    int

	// Compaction
	compactionStrategy  CompactionStrategy
	compactionThreshold int
	compactionInterval  time.Duration

	// Callbacks
	onChange  func(Delta)
	onAnomaly func(TensionResult)
	onCompact func(CompactStats)
	onGC      func(GCStats)
	onError   func(error)

	// Observability
	logger Logger

	// Storage
	storage   Storage
	baseGraph *GraphData

	// Calibration
	calibrator     Calibrator
	curvatureAlpha float64

	// Internal
	channelSize int
}

func (b *Builder) Divergence(d DivergenceFunc) *Builder      { b.divergence = d; return b }
func (b *Builder) Curvature(c CurvatureFunc) *Builder {
	b.curvature = c
	if b.curvatureAlpha == 0 {
		b.curvatureAlpha = 0.5
	}
	return b
}
func (b *Builder) CurvatureAlpha(alpha float64) *Builder { b.curvatureAlpha = alpha; return b }
func (b *Builder) Topology(t TopologyFunc) *Builder           { b.topology = t; return b }
func (b *Builder) Threshold(t float64) *Builder               { b.threshold = t; return b }
func (b *Builder) ThresholdFunc(f ThresholdFunc) *Builder     { b.thresholdFunc = f; return b }
func (b *Builder) WeightFunc(f WeightFunc) *Builder           { b.weightFunc = f; return b }
func (b *Builder) NodeTypeFunc(f NodeTypeFunc) *Builder       { b.nodeTypeFunc = f; return b }
func (b *Builder) AggregationFunc(f AggregationFunc) *Builder { b.aggregation = f; return b }
func (b *Builder) GCSnapshotWarning(d time.Duration) *Builder { b.gcSnapshotWarning = d; return b }
func (b *Builder) GCSnapshotForce(d time.Duration) *Builder   { b.gcSnapshotForce = d; return b }
func (b *Builder) MaxOverlaySize(n int) *Builder              { b.maxOverlaySize = n; return b }
func (b *Builder) CompactionStrategy(s CompactionStrategy) *Builder {
	b.compactionStrategy = s
	return b
}
func (b *Builder) CompactionThreshold(n int) *Builder          { b.compactionThreshold = n; return b }
func (b *Builder) CompactionInterval(d time.Duration) *Builder { b.compactionInterval = d; return b }
func (b *Builder) OnChange(f func(Delta)) *Builder             { b.onChange = f; return b }
func (b *Builder) OnAnomaly(f func(TensionResult)) *Builder    { b.onAnomaly = f; return b }
func (b *Builder) OnCompact(f func(CompactStats)) *Builder     { b.onCompact = f; return b }
func (b *Builder) OnGC(f func(GCStats)) *Builder               { b.onGC = f; return b }
func (b *Builder) OnError(f func(error)) *Builder              { b.onError = f; return b }
func (b *Builder) SetLogger(l Logger) *Builder                 { b.logger = l; return b }
func (b *Builder) SetStorage(s Storage) *Builder               { b.storage = s; return b }
func (b *Builder) BaseGraph(g *GraphData) *Builder             { b.baseGraph = g; return b }
func (b *Builder) SetCalibrator(c Calibrator) *Builder         { b.calibrator = c; return b }

// WithLogger sets the structured logger.
func (b *Builder) WithLogger(l Logger) *Builder { b.logger = l; return b }

// WithStorage sets the persistence backend.
func (b *Builder) WithStorage(s Storage) *Builder { b.storage = s; return b }

// WithCalibrator sets the anomaly calibrator.
func (b *Builder) WithCalibrator(c Calibrator) *Builder { b.calibrator = c; return b }
func (b *Builder) ChannelSize(n int) *Builder                  { b.channelSize = n; return b }

// Build validates configuration and returns a new Engine.
func (b *Builder) Build() (*Engine, error) {
	if b.threshold < 0 {
		return nil, fmt.Errorf("%w: threshold must be >= 0", ErrInvalidConfig)
	}
	if b.gcSnapshotForce > 0 && b.gcSnapshotWarning > 0 && b.gcSnapshotForce < b.gcSnapshotWarning {
		return nil, fmt.Errorf("%w: gcSnapshotForce must be >= gcSnapshotWarning", ErrInvalidConfig)
	}
	if b.channelSize <= 0 {
		return nil, fmt.Errorf("%w: channelSize must be > 0", ErrInvalidConfig)
	}

	return newEngine(b), nil
}
