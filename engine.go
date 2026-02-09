package itt

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MatheusGrego/itt-engine/analysis"
	"github.com/MatheusGrego/itt-engine/graph"
	"github.com/MatheusGrego/itt-engine/mvcc"
)

// Engine is the core ITT processing engine.
type Engine struct {
	config *Builder

	vc        *mvcc.Controller
	versionID atomic.Uint64

	eventCh chan Event
	started atomic.Bool
	stopped atomic.Bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	startTime   time.Time
	eventsTotal atomic.Int64
}

func newEngine(cfg *Builder) *Engine {
	e := &Engine{
		config:  cfg,
		vc:      mvcc.NewController(),
		eventCh: make(chan Event, cfg.channelSize),
	}
	// Initialize with empty graph version
	ig := graph.NewImmutableEmpty()
	e.vc.Store(&mvcc.Version{
		ID:        0,
		Graph:     ig,
		Timestamp: time.Now(),
	})
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

	e.wg.Add(1)
	go e.worker()

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
	v := e.vc.Acquire()
	return newSnapshot(v, e.config)
}

// Stats returns current engine statistics.
func (e *Engine) Stats() *EngineStats {
	v := e.vc.Load()
	var nodes, edges int
	if v != nil && v.Graph != nil {
		nodes = v.Graph.NodeCount()
		edges = v.Graph.EdgeCount()
	}

	var uptime time.Duration
	if e.started.Load() {
		uptime = time.Since(e.startTime)
	}

	return &EngineStats{
		Nodes:           nodes,
		Edges:           edges,
		VersionsCurrent: e.versionID.Load(),
		EventsTotal:     e.eventsTotal.Load(),
		Uptime:          uptime,
	}
}

// Compact forces overlay compaction into base. (Stub for now.)
func (e *Engine) Compact() error {
	return nil
}

// Reset removes all data but keeps configuration.
func (e *Engine) Reset() error {
	ig := graph.NewImmutableEmpty()
	nextID := e.versionID.Add(1)
	e.vc.Store(&mvcc.Version{
		ID:        nextID,
		Graph:     ig,
		Timestamp: time.Now(),
	})
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

func (e *Engine) processEvent(ev Event) {
	current := e.vc.Load()
	if current == nil {
		return
	}

	nodeType := ""
	if e.config.nodeTypeFunc != nil {
		nodeType = e.config.nodeTypeFunc(ev.Source)
	}
	_ = nodeType // will be used when we wire node type assignment

	weight := ev.Weight
	if e.config.weightFunc != nil {
		weight = e.config.weightFunc(ev)
	}

	newGraph := current.Graph.WithEvent(ev.Source, ev.Target, weight, ev.Type, ev.Timestamp)

	nextID := e.versionID.Add(1)
	dirty := map[string]bool{ev.Source: true, ev.Target: true}

	e.vc.Store(&mvcc.Version{
		ID:        nextID,
		Graph:     newGraph,
		Timestamp: ev.Timestamp,
		Dirty:     dirty,
	})

	e.eventsTotal.Add(1)

	// Fire OnChange callback
	if e.config.onChange != nil {
		e.safeCallback(func() {
			e.config.onChange(Delta{
				Type:      DeltaEdgeAdded,
				Timestamp: ev.Timestamp,
				Version:   nextID,
				NodeID:    ev.Source,
				EdgeFrom:  ev.Source,
				EdgeTo:    ev.Target,
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

func (e *Engine) checkAnomalies(g *graph.ImmutableGraph, ev Event, version uint64) {
	tc := analysis.NewTensionCalculator(e.getDivergence())

	for _, nodeID := range []string{ev.Source, ev.Target} {
		t := tc.Calculate(g, nodeID)
		isAnomaly := t > e.config.threshold

		if isAnomaly {
			node, _ := g.GetNode(nodeID)
			degree := 0
			if node != nil {
				degree = node.Degree
			}

			result := TensionResult{
				NodeID:  nodeID,
				Tension: t,
				Degree:  degree,
				Anomaly: true,
			}

			e.safeCallback(func() {
				e.config.onAnomaly(result)
			})
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
		}
	}()
	fn()
}
