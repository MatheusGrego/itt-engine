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
	Nodes        []TensionResult
	MeanTension  float64
	MaxTension   float64
	AnomalyCount int
	Aggregated   float64
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
