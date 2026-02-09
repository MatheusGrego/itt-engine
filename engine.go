package itt

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MatheusGrego/itt-engine/analysis"
	"github.com/MatheusGrego/itt-engine/compact"
	"github.com/MatheusGrego/itt-engine/graph"
	"github.com/MatheusGrego/itt-engine/mvcc"
)

// Engine is the core ITT processing engine.
type Engine struct {
	config *Builder

	vc        *mvcc.Controller
	versionID atomic.Uint64
	gc        *mvcc.GC

	eventCh chan Event
	started atomic.Bool
	stopped atomic.Bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	startTime   time.Time
	eventsTotal atomic.Int64

	base            *graph.ImmutableGraph
	overlayCount    atomic.Int64
	lastCompact     time.Time
	baseMu          sync.RWMutex
	snapshotsActive atomic.Int64
}

func newEngine(cfg *Builder) *Engine {
	e := &Engine{
		config:  cfg,
		vc:      mvcc.NewController(),
		eventCh: make(chan Event, cfg.channelSize),
	}
	// Initialize with empty graph version
	ig := graph.NewImmutableEmpty()
	v := &mvcc.Version{
		ID:        0,
		Graph:     ig,
		Timestamp: time.Now(),
	}
	e.vc.Store(v)

	// Create GC
	e.gc = mvcc.NewGC(e.vc, mvcc.GCConfig{
		Interval:       30 * time.Second,
		WarningTimeout: cfg.gcSnapshotWarning,
		ForceTimeout:   cfg.gcSnapshotForce,
		OnWarning: func(versionID uint64, age time.Duration) {
			if cfg.logger != nil {
				cfg.logger.Warn("snapshot held too long", "version", versionID, "age", age)
			}
		},
		OnForce: func(versionID uint64, age time.Duration) {
			if cfg.logger != nil {
				cfg.logger.Warn("snapshot force-closed", "version", versionID, "age", age)
			}
		},
	})
	e.base = graph.NewImmutableEmpty()
	e.lastCompact = time.Now()

	return e
}

// Start begins processing events. Context cancellation triggers graceful shutdown.
func (e *Engine) Start(ctx context.Context) error {
	if e.started.Load() {
		return ErrEngineRunning
	}

	e.ctx, e.cancel = context.WithCancel(ctx)
	e.started.Store(true)
	e.stopped.Store(false)
	e.startTime = time.Now()

	e.wg.Add(2) // worker + gcWorker
	go e.worker()
	go e.gcWorker()

	return nil
}

// Stop gracefully shuts down the engine.
func (e *Engine) Stop() error {
	if !e.started.Load() || e.stopped.Load() {
		return ErrEngineStopped
	}
	e.cancel()
	e.wg.Wait()
	e.stopped.Store(true)
	e.started.Store(false)
	return nil
}

// Running returns true if the engine is processing events.
func (e *Engine) Running() bool {
	return e.started.Load() && !e.stopped.Load()
}

// AddEvent submits a single event for processing.
func (e *Engine) AddEvent(event Event) error {
	if err := event.Validate(); err != nil {
		return err
	}

	// Auto-start if not started
	if !e.started.Load() {
		if err := e.Start(context.Background()); err != nil && err != ErrEngineRunning {
			return err
		}
	}

	if e.stopped.Load() {
		return ErrEngineStopped
	}

	select {
	case e.eventCh <- event.Normalize():
		return nil
	case <-e.ctx.Done():
		return ErrEngineStopped
	}
}

// AddEvents submits a batch of events. All-or-nothing validation.
func (e *Engine) AddEvents(events []Event) error {
	// Validate all first
	for i := range events {
		if err := events[i].Validate(); err != nil {
			return err
		}
	}

	for i := range events {
		if err := e.AddEvent(events[i]); err != nil {
			return err
		}
	}
	return nil
}

// Snapshot returns an immutable snapshot of the current graph state.
func (e *Engine) Snapshot() *Snapshot {
	e.snapshotsActive.Add(1)
	v := e.vc.Acquire()
	e.gc.Track(v) // Track acquired versions for GC lifecycle management
	e.baseMu.RLock()
	base := e.base
	e.baseMu.RUnlock()
	snap := newSnapshot(v, e.config, base)
	snap.onClose = func() { e.snapshotsActive.Add(-1) }
	return snap
}

// Stats returns current engine statistics.
func (e *Engine) Stats() *EngineStats {
	v := e.vc.Load()
	var overlayNodes, overlayEdges int
	if v != nil && v.Graph != nil {
		overlayNodes = v.Graph.NodeCount()
		overlayEdges = v.Graph.EdgeCount()
	}

	var baseNodes, baseEdges int
	e.baseMu.RLock()
	if e.base != nil {
		baseNodes = e.base.NodeCount()
		baseEdges = e.base.EdgeCount()
	}
	e.baseMu.RUnlock()

	var uptime time.Duration
	var eps float64
	if e.started.Load() {
		uptime = time.Since(e.startTime)
		if uptime.Seconds() > 0 {
			eps = float64(e.eventsTotal.Load()) / uptime.Seconds()
		}
	}

	return &EngineStats{
		Nodes:           overlayNodes + baseNodes,
		Edges:           overlayEdges + baseEdges,
		OverlayEvents:   int(e.overlayCount.Load()),
		BaseNodes:       baseNodes,
		BaseEdges:       baseEdges,
		VersionsCurrent: e.versionID.Load(),
		VersionsTotal:   e.versionID.Load(),
		SnapshotsActive: int(e.snapshotsActive.Load()),
		EventsTotal:     e.eventsTotal.Load(),
		EventsPerSecond: eps,
		Uptime:          uptime,
	}
}

func (e *Engine) shouldCompact() bool {
	switch e.config.compactionStrategy {
	case CompactByVolume:
		return int(e.overlayCount.Load()) >= e.config.compactionThreshold
	case CompactByTime:
		return time.Since(e.lastCompact) >= e.config.compactionInterval
	default:
		return false
	}
}

func (e *Engine) doCompact() {
	start := time.Now()
	e.baseMu.Lock()
	current := e.vc.Load()
	if current == nil || current.Graph == nil {
		e.baseMu.Unlock()
		return
	}
	merged, cStats := compact.Compact(e.base, current.Graph)
	e.base = merged
	e.baseMu.Unlock()

	// Create new version with empty overlay
	ig := graph.NewImmutableEmpty()
	nextID := e.versionID.Add(1)
	v := &mvcc.Version{ID: nextID, Graph: ig, Timestamp: time.Now()}
	e.vc.Store(v)

	e.overlayCount.Store(0)
	e.lastCompact = time.Now()

	if e.config.onCompact != nil {
		e.safeCallback(func() {
			e.config.onCompact(CompactStats{
				NodesMerged:   cStats.NodesMerged,
				EdgesMerged:   cStats.EdgesMerged,
				OverlayBefore: cStats.OverlayBefore,
				OverlayAfter:  cStats.OverlayAfter,
				Duration:      time.Since(start),
				Timestamp:     start,
			})
		})
	}
}

// Compact forces overlay compaction into base.
func (e *Engine) Compact() error {
	if !e.Running() {
		return ErrEngineStopped
	}
	e.doCompact()
	return nil
}

// Reset removes all data but keeps configuration.
func (e *Engine) Reset() error {
	e.baseMu.Lock()
	e.base = graph.NewImmutableEmpty()
	e.baseMu.Unlock()
	e.overlayCount.Store(0)
	e.lastCompact = time.Now()

	ig := graph.NewImmutableEmpty()
	nextID := e.versionID.Add(1)
	v := &mvcc.Version{
		ID:        nextID,
		Graph:     ig,
		Timestamp: time.Now(),
	}
	e.vc.Store(v)
	return nil
}

func (e *Engine) worker() {
	defer e.wg.Done()
	for {
		select {
		case ev := <-e.eventCh:
			e.processEvent(ev)
		case <-e.ctx.Done():
			// Drain remaining events
			for {
				select {
				case ev := <-e.eventCh:
					e.processEvent(ev)
				default:
					return
				}
			}
		}
	}
}

func (e *Engine) gcWorker() {
	defer e.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			stats := e.gc.Collect()
			if stats.VersionsRemoved > 0 && e.config.onGC != nil {
				e.safeCallback(func() {
					e.config.onGC(GCStats{
						VersionsRemoved: stats.VersionsRemoved,
						OldestRemoved:   stats.OldestRemoved,
						Timestamp:       stats.Timestamp,
					})
				})
			}
		}
	}
}

func (e *Engine) processEvent(ev Event) {
	current := e.vc.Load()
	if current == nil {
		return
	}

	nodeType := ""
	if e.config.nodeTypeFunc != nil {
		nodeType = e.config.nodeTypeFunc(ev.Source)
	}

	weight := ev.Weight
	if e.config.weightFunc != nil {
		weight = e.config.weightFunc(ev)
	}

	// Check pre-event state for correct delta types
	_, sourceExisted := current.Graph.GetNode(ev.Source)
	_, targetExisted := current.Graph.GetNode(ev.Target)
	_, edgeExisted := current.Graph.GetEdge(ev.Source, ev.Target)

	newGraph := current.Graph.WithEvent(ev.Source, ev.Target, weight, ev.Type, ev.Timestamp)

	if nodeType != "" {
		if n, ok := newGraph.GetNode(ev.Source); ok {
			n.Type = nodeType
		}
	}
	if e.config.nodeTypeFunc != nil {
		targetType := e.config.nodeTypeFunc(ev.Target)
		if targetType != "" {
			if n, ok := newGraph.GetNode(ev.Target); ok {
				n.Type = targetType
			}
		}
	}

	nextID := e.versionID.Add(1)
	dirty := map[string]bool{ev.Source: true, ev.Target: true}

	v := &mvcc.Version{
		ID:        nextID,
		Graph:     newGraph,
		Timestamp: ev.Timestamp,
		Dirty:     dirty,
	}
	e.vc.Store(v)

	e.eventsTotal.Add(1)

	e.overlayCount.Add(1)

	// Auto-compact check
	if e.shouldCompact() {
		e.doCompact()
	}

	// Fire OnChange callbacks
	if e.config.onChange != nil {
		if !sourceExisted {
			e.safeCallback(func() {
				e.config.onChange(Delta{
					Type: DeltaNodeAdded, Timestamp: ev.Timestamp,
					Version: nextID, NodeID: ev.Source,
				})
			})
		}
		if !targetExisted {
			e.safeCallback(func() {
				e.config.onChange(Delta{
					Type: DeltaNodeAdded, Timestamp: ev.Timestamp,
					Version: nextID, NodeID: ev.Target,
				})
			})
		}

		edgeDelta := DeltaEdgeAdded
		if edgeExisted {
			edgeDelta = DeltaEdgeUpdated
		}
		e.safeCallback(func() {
			e.config.onChange(Delta{
				Type: edgeDelta, Timestamp: ev.Timestamp,
				Version: nextID, EdgeFrom: ev.Source, EdgeTo: ev.Target,
			})
		})
	}

	// Real-time anomaly detection for dirty nodes
	if e.config.onAnomaly != nil {
		e.checkAnomalies(newGraph, ev, nextID)
	}
}

// Analyze takes a snapshot, runs analysis, and returns results.
func (e *Engine) Analyze() (*Results, error) {
	snap := e.Snapshot()
	defer snap.Close()
	return snap.Analyze()
}

// AnalyzeNode computes tension for a single node using a temporary snapshot.
func (e *Engine) AnalyzeNode(nodeID string) (*TensionResult, error) {
	snap := e.Snapshot()
	defer snap.Close()
	return snap.AnalyzeNode(nodeID)
}

// AnalyzeRegion computes tension for a subset of nodes using a temporary snapshot.
func (e *Engine) AnalyzeRegion(nodeIDs []string) (*RegionResult, error) {
	snap := e.Snapshot()
	defer snap.Close()
	return snap.AnalyzeRegion(nodeIDs)
}

func (e *Engine) checkAnomalies(g *graph.ImmutableGraph, ev Event, version uint64) {
	tc := analysis.NewTensionCalculator(e.getDivergence())

	for _, nodeID := range []string{ev.Source, ev.Target} {
		node, _ := g.GetNode(nodeID)
		if node == nil {
			continue
		}
		t := tc.Calculate(g, nodeID)

		// Observe in calibrator
		if e.config.calibrator != nil {
			e.config.calibrator.Observe(t)
		}

		anomaly := isAnomaly(e.config, node, t)

		if anomaly {
			confidence := 0.0
			if node.Degree > 0 {
				confidence = min(1.0, float64(node.Degree)/10.0)
			}

			result := TensionResult{
				NodeID:     nodeID,
				Tension:    t,
				Degree:     node.Degree,
				Anomaly:    true,
				Confidence: confidence,
				Components: map[string]float64{"tension": t},
			}

			if e.config.onAnomaly != nil {
				e.safeCallback(func() {
					e.config.onAnomaly(result)
				})
			}

			// Also emit anomaly delta
			if e.config.onChange != nil {
				e.safeCallback(func() {
					e.config.onChange(Delta{
						Type:      DeltaAnomalyDetected,
						Timestamp: ev.Timestamp,
						Version:   version,
						NodeID:    nodeID,
						Tension:   t,
					})
				})
			}
		}
	}
}

func (e *Engine) getDivergence() analysis.DivergenceFunc {
	if e.config.divergence != nil {
		if ad, ok := e.config.divergence.(analysis.DivergenceFunc); ok {
			return ad
		}
	}
	return analysis.JSD{}
}

func (e *Engine) safeCallback(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			if e.config.logger != nil {
				e.config.logger.Error("callback panic recovered", "panic", r)
			}
			if e.config.onError != nil {
				// Don't recurse through safeCallback for error reporting
				func() {
					defer func() { recover() }()
					e.config.onError(fmt.Errorf("callback panic: %v", r))
				}()
			}
		}
	}()
	fn()
}

func (e *Engine) reportError(err error) {
	if err != nil && e.config.onError != nil {
		e.safeCallback(func() { e.config.onError(err) })
	}
}
