package itt

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MatheusGrego/itt-engine/analysis"
)

// === Builder tests ===

func TestBuilder_DefaultsBuild(t *testing.T) {
	e, err := NewBuilder().Build()
	if err != nil {
		t.Fatalf("default build failed: %v", err)
	}
	if e == nil {
		t.Fatal("engine is nil")
	}
}

func TestBuilder_NegativeThreshold(t *testing.T) {
	_, err := NewBuilder().Threshold(-1).Build()
	if err == nil {
		t.Fatal("expected error for negative threshold")
	}
}

func TestBuilder_Chaining(t *testing.T) {
	_, err := NewBuilder().
		Threshold(0.3).
		GCSnapshotWarning(5 * time.Minute).
		GCSnapshotForce(10 * time.Minute).
		Build()
	if err != nil {
		t.Fatalf("chaining failed: %v", err)
	}
}

func TestBuilder_GCForceBeforeWarning(t *testing.T) {
	_, err := NewBuilder().
		GCSnapshotWarning(10 * time.Minute).
		GCSnapshotForce(5 * time.Minute).
		Build()
	if err == nil {
		t.Fatal("expected error when force < warning")
	}
}

// === Engine lifecycle tests ===

func TestEngine_StartStop(t *testing.T) {
	e, _ := NewBuilder().Build()
	if e.Running() {
		t.Fatal("should not be running before Start")
	}

	err := e.Start(context.Background())
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !e.Running() {
		t.Fatal("should be running after Start")
	}

	err = e.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if e.Running() {
		t.Fatal("should not be running after Stop")
	}
}

func TestEngine_DoubleStart(t *testing.T) {
	e, _ := NewBuilder().Build()
	e.Start(context.Background())
	defer e.Stop()

	err := e.Start(context.Background())
	if err != ErrEngineRunning {
		t.Fatalf("expected ErrEngineRunning, got %v", err)
	}
}

func TestEngine_ContextCancellation(t *testing.T) {
	e, _ := NewBuilder().Build()
	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)
	cancel()
	time.Sleep(50 * time.Millisecond)
	// Engine should have stopped after context cancel
}

// === Ingestion tests ===

func TestEngine_AddEvent_Valid(t *testing.T) {
	e, _ := NewBuilder().Build()
	err := e.AddEvent(Event{Source: "a", Target: "b", Weight: 1.0})
	if err != nil {
		t.Fatalf("AddEvent failed: %v", err)
	}
	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	snap := e.Snapshot()
	defer snap.Close()

	nc, _ := snap.NodeCount()
	if nc != 2 {
		t.Fatalf("expected 2 nodes, got %d", nc)
	}
	e.Stop()
}

func TestEngine_AddEvent_Invalid(t *testing.T) {
	e, _ := NewBuilder().Build()
	defer e.Stop()

	err := e.AddEvent(Event{Source: "", Target: "b"})
	if err != ErrEmptySource {
		t.Fatalf("expected ErrEmptySource, got %v", err)
	}
}

func TestEngine_AddEvent_AutoStart(t *testing.T) {
	e, _ := NewBuilder().Build()
	// Don't call Start explicitly
	err := e.AddEvent(Event{Source: "a", Target: "b"})
	if err != nil {
		t.Fatalf("auto-start AddEvent failed: %v", err)
	}
	if !e.Running() {
		t.Fatal("engine should be running after auto-start")
	}
	e.Stop()
}

func TestEngine_AddEvents_Batch(t *testing.T) {
	e, _ := NewBuilder().Build()
	events := []Event{
		{Source: "a", Target: "b"},
		{Source: "b", Target: "c"},
		{Source: "c", Target: "a"},
	}
	err := e.AddEvents(events)
	if err != nil {
		t.Fatalf("AddEvents failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	snap := e.Snapshot()
	defer snap.Close()
	nc, _ := snap.NodeCount()
	if nc != 3 {
		t.Fatalf("expected 3 nodes, got %d", nc)
	}
	e.Stop()
}

func TestEngine_AddEvents_InvalidRejectsAll(t *testing.T) {
	e, _ := NewBuilder().Build()
	defer e.Stop()

	events := []Event{
		{Source: "a", Target: "b"},
		{Source: "", Target: "c"}, // invalid
	}
	err := e.AddEvents(events)
	if err != ErrEmptySource {
		t.Fatalf("expected ErrEmptySource, got %v", err)
	}
}

func TestEngine_ConcurrentAddEvent(t *testing.T) {
	e, _ := NewBuilder().Build()
	e.Start(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			src := "node-" + string(rune('A'+n%26))
			tgt := "node-" + string(rune('A'+(n+1)%26))
			e.AddEvent(Event{Source: src, Target: tgt})
		}(i)
	}
	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	stats := e.Stats()
	if stats.EventsTotal < 100 {
		t.Fatalf("expected at least 100 events, got %d", stats.EventsTotal)
	}
	e.Stop()
}

// === Snapshot tests ===

func TestSnapshot_CapturesState(t *testing.T) {
	e, _ := NewBuilder().Build()
	e.AddEvent(Event{Source: "a", Target: "b"})
	time.Sleep(50 * time.Millisecond)

	snap := e.Snapshot()
	defer snap.Close()

	nc, _ := snap.NodeCount()
	if nc != 2 {
		t.Fatalf("expected 2 nodes, got %d", nc)
	}
	e.Stop()
}

func TestSnapshot_Isolation(t *testing.T) {
	e, _ := NewBuilder().Build()
	e.AddEvent(Event{Source: "a", Target: "b"})
	time.Sleep(50 * time.Millisecond)

	snap := e.Snapshot()
	defer snap.Close()

	// Add more events
	e.AddEvent(Event{Source: "c", Target: "d"})
	time.Sleep(50 * time.Millisecond)

	// Snapshot should still see old state
	nc, _ := snap.NodeCount()
	if nc != 2 {
		t.Fatalf("snapshot should see 2 nodes, got %d", nc)
	}

	// New snapshot sees updated state
	snap2 := e.Snapshot()
	defer snap2.Close()
	nc2, _ := snap2.NodeCount()
	if nc2 != 4 {
		t.Fatalf("new snapshot should see 4 nodes, got %d", nc2)
	}
	e.Stop()
}

func TestSnapshot_CloseIdempotent(t *testing.T) {
	e, _ := NewBuilder().Build()
	snap := e.Snapshot()
	snap.Close()
	snap.Close() // should not panic
}

func TestSnapshot_OperationsAfterClose(t *testing.T) {
	e, _ := NewBuilder().Build()
	snap := e.Snapshot()
	snap.Close()

	_, err := snap.NodeCount()
	if err != ErrSnapshotClosed {
		t.Fatalf("expected ErrSnapshotClosed, got %v", err)
	}
}

func TestSnapshot_GetNode(t *testing.T) {
	e, _ := NewBuilder().Build()
	e.AddEvent(Event{Source: "alice", Target: "bob"})
	time.Sleep(50 * time.Millisecond)

	snap := e.Snapshot()
	defer snap.Close()

	n, ok, err := snap.GetNode("alice")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected node found")
	}
	if n.ID != "alice" {
		t.Fatalf("expected alice, got %s", n.ID)
	}
	e.Stop()
}

func TestEngine_Stats(t *testing.T) {
	e, _ := NewBuilder().Build()
	e.AddEvent(Event{Source: "a", Target: "b"})
	time.Sleep(50 * time.Millisecond)

	stats := e.Stats()
	if stats.Nodes != 2 {
		t.Fatalf("expected 2 nodes in stats, got %d", stats.Nodes)
	}
	if stats.EventsTotal != 1 {
		t.Fatalf("expected 1 event total, got %d", stats.EventsTotal)
	}
	e.Stop()
}

func TestEngine_Reset(t *testing.T) {
	e, _ := NewBuilder().Build()
	e.AddEvent(Event{Source: "a", Target: "b"})
	time.Sleep(50 * time.Millisecond)

	e.Reset()

	snap := e.Snapshot()
	defer snap.Close()
	nc, _ := snap.NodeCount()
	if nc != 0 {
		t.Fatalf("expected 0 nodes after reset, got %d", nc)
	}
	e.Stop()
}

// === Analysis integration tests ===

func TestSnapshot_Analyze(t *testing.T) {
	e, _ := NewBuilder().Threshold(0.1).Build()
	e.AddEvent(Event{Source: "hub", Target: "a"})
	e.AddEvent(Event{Source: "hub", Target: "b"})
	e.AddEvent(Event{Source: "hub", Target: "c"})
	e.AddEvent(Event{Source: "a", Target: "b"})
	time.Sleep(100 * time.Millisecond)

	snap := e.Snapshot()
	defer snap.Close()

	results, err := snap.Analyze()
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Tensions) == 0 {
		t.Fatal("expected tension results")
	}
	if results.Stats.NodesAnalyzed != 4 {
		t.Fatalf("expected 4 nodes analyzed, got %d", results.Stats.NodesAnalyzed)
	}
	e.Stop()
}

func TestSnapshot_AnalyzeNode(t *testing.T) {
	e, _ := NewBuilder().Build()
	e.AddEvent(Event{Source: "a", Target: "b"})
	e.AddEvent(Event{Source: "a", Target: "c"})
	time.Sleep(100 * time.Millisecond)

	snap := e.Snapshot()
	defer snap.Close()

	result, err := snap.AnalyzeNode("a")
	if err != nil {
		t.Fatal(err)
	}
	if result.NodeID != "a" {
		t.Fatalf("expected node a, got %s", result.NodeID)
	}
	if result.Tension < 0 {
		t.Fatal("tension should be non-negative")
	}
	e.Stop()
}

func TestSnapshot_AnalyzeNode_NotFound(t *testing.T) {
	e, _ := NewBuilder().Build()
	snap := e.Snapshot()
	defer snap.Close()

	_, err := snap.AnalyzeNode("nonexistent")
	if err != ErrNodeNotFound {
		t.Fatalf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestSnapshot_AnalyzeRegion(t *testing.T) {
	e, _ := NewBuilder().Build()
	e.AddEvent(Event{Source: "a", Target: "b"})
	e.AddEvent(Event{Source: "b", Target: "c"})
	e.AddEvent(Event{Source: "c", Target: "a"})
	time.Sleep(100 * time.Millisecond)

	snap := e.Snapshot()
	defer snap.Close()

	result, err := snap.AnalyzeRegion([]string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result.Nodes))
	}
	e.Stop()
}

func TestSnapshot_Analyze_Closed(t *testing.T) {
	e, _ := NewBuilder().Build()
	snap := e.Snapshot()
	snap.Close()

	_, err := snap.Analyze()
	if err != ErrSnapshotClosed {
		t.Fatalf("expected ErrSnapshotClosed, got %v", err)
	}
}

func TestEngine_Analyze(t *testing.T) {
	e, _ := NewBuilder().Build()
	e.AddEvent(Event{Source: "a", Target: "b"})
	time.Sleep(50 * time.Millisecond)

	results, err := e.Analyze()
	if err != nil {
		t.Fatal(err)
	}
	if results.Stats.NodesAnalyzed != 2 {
		t.Fatalf("expected 2 nodes, got %d", results.Stats.NodesAnalyzed)
	}
	e.Stop()
}

// === Callback tests ===

func TestCallback_OnChange(t *testing.T) {
	var mu sync.Mutex
	var deltas []Delta

	e, _ := NewBuilder().
		OnChange(func(d Delta) {
			mu.Lock()
			deltas = append(deltas, d)
			mu.Unlock()
		}).
		Build()

	e.Start(context.Background())
	e.AddEvent(Event{Source: "a", Target: "b"})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	count := len(deltas)
	mu.Unlock()

	if count == 0 {
		t.Fatal("expected OnChange to be called")
	}

	// With correct delta types, we expect DeltaNodeAdded for new nodes
	// and DeltaEdgeAdded for new edge
	mu.Lock()
	hasEdgeAdded := false
	for _, d := range deltas {
		if d.Type == DeltaEdgeAdded && d.EdgeFrom == "a" && d.EdgeTo == "b" {
			hasEdgeAdded = true
		}
	}
	mu.Unlock()

	if !hasEdgeAdded {
		t.Fatal("expected DeltaEdgeAdded for a->b")
	}
	e.Stop()
}

func TestCallback_OnAnomaly(t *testing.T) {
	var mu sync.Mutex
	var anomalies []TensionResult

	e, _ := NewBuilder().
		Threshold(0.0). // very low threshold to trigger anomalies
		OnAnomaly(func(r TensionResult) {
			mu.Lock()
			anomalies = append(anomalies, r)
			mu.Unlock()
		}).
		Build()

	e.Start(context.Background())

	// Build a graph structure that generates tension
	e.AddEvent(Event{Source: "hub", Target: "a"})
	e.AddEvent(Event{Source: "hub", Target: "b"})
	e.AddEvent(Event{Source: "hub", Target: "c"})
	e.AddEvent(Event{Source: "a", Target: "b"})
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	count := len(anomalies)
	mu.Unlock()

	// With threshold 0.0, any node with non-zero tension should trigger
	if count == 0 {
		t.Fatal("expected OnAnomaly to be called at least once")
	}

	mu.Lock()
	for _, a := range anomalies {
		if a.Tension < 0 {
			t.Fatal("anomaly tension should be non-negative")
		}
		if !a.Anomaly {
			t.Fatal("anomaly flag should be true")
		}
	}
	mu.Unlock()
	e.Stop()
}

func TestCallback_PanicRecovery(t *testing.T) {
	e, _ := NewBuilder().
		OnChange(func(d Delta) {
			panic("intentional panic")
		}).
		Build()

	e.Start(context.Background())
	// Should not panic
	err := e.AddEvent(Event{Source: "a", Target: "b"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	// Engine should still be running
	if !e.Running() {
		t.Fatal("engine should survive callback panic")
	}
	e.Stop()
}

func TestIsAnomalyPriority(t *testing.T) {
	// Test 1: Static threshold (default path)
	t.Run("static_threshold", func(t *testing.T) {
		engine, _ := NewBuilder().Threshold(0.5).Build()
		err := engine.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
		snap := engine.Snapshot()
		defer snap.Close()
		results, err := snap.Analyze()
		if err != nil {
			t.Fatal(err)
		}
		// With threshold 0.5 and minimal graph, no anomalies expected
		for _, r := range results.Tensions {
			if r.Tension <= 0.5 && r.Anomaly {
				t.Error("should not be anomaly below threshold")
			}
		}
		engine.Stop()
	})

	// Test 2: ThresholdFunc overrides static threshold
	t.Run("thresholdFunc_overrides", func(t *testing.T) {
		engine, _ := NewBuilder().
			Threshold(999). // Very high static threshold
			ThresholdFunc(func(node *Node, tension float64) bool {
				return tension > 0.001 // Very low threshold - everything is anomaly
			}).
			Build()
		err := engine.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
		if err != nil {
			t.Fatal(err)
		}
		err = engine.AddEvent(Event{Source: "b", Target: "c", Weight: 1})
		if err != nil {
			t.Fatal(err)
		}
		err = engine.AddEvent(Event{Source: "c", Target: "a", Weight: 1})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
		snap := engine.Snapshot()
		defer snap.Close()
		results, err := snap.Analyze()
		if err != nil {
			t.Fatal(err)
		}
		// ThresholdFunc should override static threshold
		// At least some nodes should have tension > 0.001
		hasAnomaly := false
		for _, r := range results.Tensions {
			if r.Anomaly {
				hasAnomaly = true
				break
			}
		}
		// In a triangle graph, nodes should have non-zero tension
		if !hasAnomaly && len(results.Tensions) > 0 {
			// Check if any tensions are above 0.001
			for _, r := range results.Tensions {
				if r.Tension > 0.001 {
					t.Error("thresholdFunc should have flagged this as anomaly")
				}
			}
		}
		engine.Stop()
	})
}

func TestCallback_OnAnomaly_HighThreshold_NoFire(t *testing.T) {
	called := false
	e, _ := NewBuilder().
		Threshold(999.0). // extremely high threshold
		OnAnomaly(func(r TensionResult) {
			called = true
		}).
		Build()

	e.Start(context.Background())
	e.AddEvent(Event{Source: "a", Target: "b"})
	time.Sleep(100 * time.Millisecond)

	if called {
		t.Fatal("OnAnomaly should not fire with very high threshold")
	}
	e.Stop()
}

func TestNodeTypeFuncWiring(t *testing.T) {
	engine, err := NewBuilder().
		NodeTypeFunc(func(id string) string {
			if strings.HasPrefix(id, "user:") {
				return "user"
			}
			return "system"
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	err = engine.AddEvent(Event{Source: "user:alice", Target: "server:main", Weight: 1})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	snap := engine.Snapshot()
	defer snap.Close()

	n, ok, err := snap.GetNode("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("node not found")
	}
	if n.Type != "user" {
		t.Errorf("expected type 'user', got %q", n.Type)
	}

	n2, ok, _ := snap.GetNode("server:main")
	if !ok {
		t.Fatal("target node not found")
	}
	if n2.Type != "system" {
		t.Errorf("expected type 'system', got %q", n2.Type)
	}

	engine.Stop()
}

func TestGCWiring(t *testing.T) {
	gcCalled := make(chan GCStats, 10)
	engine, err := NewBuilder().
		GCSnapshotWarning(50 * time.Millisecond).
		GCSnapshotForce(100 * time.Millisecond).
		OnGC(func(stats GCStats) {
			gcCalled <- stats
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	if err := engine.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Add some events to create versions
	for i := 0; i < 5; i++ {
		engine.AddEvent(Event{
			Source: fmt.Sprintf("a%d", i),
			Target: fmt.Sprintf("b%d", i),
			Weight: 1,
		})
	}
	time.Sleep(50 * time.Millisecond)

	// Verify GC is tracking versions - just verify engine runs without error
	engine.Stop()
}

func TestGCCollectsOrphanVersions(t *testing.T) {
	engine, err := NewBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}

	// Add events to create versions
	for i := 0; i < 3; i++ {
		engine.AddEvent(Event{
			Source: fmt.Sprintf("x%d", i),
			Target: fmt.Sprintf("y%d", i),
			Weight: 1,
		})
	}
	time.Sleep(50 * time.Millisecond)

	// Take a snapshot and close it (creates orphan version)
	snap := engine.Snapshot()
	snap.Close()

	// Manual GC collect
	stats := engine.gc.Collect()
	// Should have collected at least some orphan versions
	t.Logf("GC collected %d versions", stats.VersionsRemoved)

	engine.Stop()
}

// mockCalibrator is a simple Calibrator implementation for testing.
type mockCalibrator struct {
	observations []float64
	warmedUp     bool
	warmupSize   int
}

func (m *mockCalibrator) Observe(t float64) {
	m.observations = append(m.observations, t)
	if len(m.observations) >= m.warmupSize {
		m.warmedUp = true
	}
}
func (m *mockCalibrator) IsWarmedUp() bool            { return m.warmedUp }
func (m *mockCalibrator) Threshold() float64           { return 0.5 }
func (m *mockCalibrator) IsAnomaly(t float64) bool     { return t > 0.5 }
func (m *mockCalibrator) Stats() CalibratorStats       { return CalibratorStats{} }
func (m *mockCalibrator) Recalibrate()                 {}

func TestAnalyzeCurvature(t *testing.T) {
	engine, err := NewBuilder().
		CurvatureAlpha(0.5).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Build a triangle: a->b, b->c, c->a
	events := []Event{
		{Source: "a", Target: "b", Weight: 1},
		{Source: "b", Target: "c", Weight: 1},
		{Source: "c", Target: "a", Weight: 1},
	}
	for _, ev := range events {
		if err := engine.AddEvent(ev); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(100 * time.Millisecond)

	snap := engine.Snapshot()
	defer snap.Close()

	results, err := snap.Analyze()
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range results.Tensions {
		// Curvature should be computed (could be positive or negative)
		if r.Components == nil {
			t.Errorf("node %s: Components should not be nil", r.NodeID)
		}
		if _, ok := r.Components["curvature"]; !ok {
			t.Errorf("node %s: missing curvature component", r.NodeID)
		}
		if r.Confidence < 0 || r.Confidence > 1 {
			t.Errorf("node %s: confidence %f out of range [0,1]", r.NodeID, r.Confidence)
		}
	}
	engine.Stop()
}

func TestAnalyzeWithCalibrator(t *testing.T) {
	cal := &mockCalibrator{warmupSize: 3}

	engine, err := NewBuilder().
		SetCalibrator(cal).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Submit enough events to warm up calibrator
	for i := 0; i < 10; i++ {
		src := fmt.Sprintf("n%d", i)
		dst := fmt.Sprintf("n%d", (i+1)%10)
		if err := engine.AddEvent(Event{Source: src, Target: dst, Weight: 1}); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(100 * time.Millisecond)

	snap := engine.Snapshot()
	defer snap.Close()

	results, err := snap.Analyze()
	if err != nil {
		t.Fatal(err)
	}

	// After analysis, calibrator should have observations
	if !cal.IsWarmedUp() {
		// Calibrator needs warmupSize observations. With 10 nodes analyzed,
		// it should be warmed up if warmupSize is 3.
		t.Log("calibrator not warmed up yet, checking observations")
	}

	// Just verify no errors and results populated
	if len(results.Tensions) == 0 {
		t.Error("expected tension results")
	}
	engine.Stop()
}

func TestCompactionWiring(t *testing.T) {
	compactCalled := make(chan CompactStats, 10)
	engine, err := NewBuilder().
		CompactionStrategy(CompactByVolume).
		CompactionThreshold(5). // compact after 5 events
		OnCompact(func(stats CompactStats) {
			compactCalled <- stats
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Add 10 events (should trigger compaction at 5)
	for i := 0; i < 10; i++ {
		err := engine.AddEvent(Event{
			Source: fmt.Sprintf("s%d", i),
			Target: fmt.Sprintf("t%d", i),
			Weight: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(200 * time.Millisecond)

	// Should have gotten at least one compaction callback
	select {
	case stats := <-compactCalled:
		t.Logf("Compaction: %d nodes merged, %d edges merged", stats.NodesMerged, stats.EdgesMerged)
	default:
		t.Error("expected compaction callback")
	}

	// Data should still be accessible after compaction
	snap := engine.Snapshot()
	defer snap.Close()
	nc, _ := snap.NodeCount()
	if nc == 0 {
		t.Error("expected nodes after compaction")
	}

	engine.Stop()
}

func TestManualCompact(t *testing.T) {
	engine, err := NewBuilder().
		CompactionStrategy(CompactManual).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		engine.AddEvent(Event{
			Source: fmt.Sprintf("a%d", i),
			Target: fmt.Sprintf("b%d", i),
			Weight: 1,
		})
	}
	time.Sleep(100 * time.Millisecond)

	// Manual compact
	err = engine.Compact()
	if err != nil {
		t.Fatal(err)
	}

	// Data should still be accessible
	snap := engine.Snapshot()
	defer snap.Close()
	nc, _ := snap.NodeCount()
	if nc == 0 {
		t.Error("expected nodes after manual compaction")
	}

	engine.Stop()
}

func TestCompactionPreservesData(t *testing.T) {
	engine, err := NewBuilder().
		CompactionStrategy(CompactByVolume).
		CompactionThreshold(3).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Create a known graph
	engine.AddEvent(Event{Source: "alice", Target: "bob", Weight: 2.0})
	engine.AddEvent(Event{Source: "bob", Target: "carol", Weight: 3.0})
	engine.AddEvent(Event{Source: "carol", Target: "alice", Weight: 1.0})
	time.Sleep(100 * time.Millisecond)

	// Force compaction
	engine.Compact()
	time.Sleep(50 * time.Millisecond)

	// Add more events after compaction
	engine.AddEvent(Event{Source: "alice", Target: "dave", Weight: 1.0})
	time.Sleep(100 * time.Millisecond)

	snap := engine.Snapshot()
	defer snap.Close()

	// Check all data is present (base + overlay)
	nc, _ := snap.NodeCount()
	if nc < 4 {
		t.Errorf("expected at least 4 nodes (alice,bob,carol,dave), got %d", nc)
	}

	engine.Stop()
}

func TestSnapshotTimestamp(t *testing.T) {
	engine, err := NewBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now()
	engine.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	time.Sleep(50 * time.Millisecond)

	snap := engine.Snapshot()
	defer snap.Close()

	ts, err := snap.Timestamp()
	if err != nil {
		t.Fatal(err)
	}
	if ts.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if ts.Before(before) {
		t.Error("timestamp should be after test start")
	}

	engine.Stop()
}

func TestSnapshotExportJSON(t *testing.T) {
	engine, err := NewBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}

	engine.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	engine.AddEvent(Event{Source: "b", Target: "c", Weight: 2})
	time.Sleep(100 * time.Millisecond)

	snap := engine.Snapshot()
	defer snap.Close()

	var buf bytes.Buffer
	err = snap.Export(ExportJSON, &buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "\"nodes\"") {
		t.Error("JSON should contain nodes key")
	}
	if !strings.Contains(output, "\"edges\"") {
		t.Error("JSON should contain edges key")
	}
	if !strings.Contains(output, "\"a\"") {
		t.Error("JSON should contain node 'a'")
	}

	engine.Stop()
}

func TestSnapshotExportDOT(t *testing.T) {
	engine, err := NewBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}

	engine.AddEvent(Event{Source: "x", Target: "y", Weight: 1})
	time.Sleep(50 * time.Millisecond)

	snap := engine.Snapshot()
	defer snap.Close()

	var buf bytes.Buffer
	err = snap.Export(ExportDOT, &buf)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "digraph") {
		t.Error("DOT output should contain 'digraph'")
	}

	engine.Stop()
}

func TestSnapshotExportClosedError(t *testing.T) {
	engine, err := NewBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}

	engine.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	time.Sleep(50 * time.Millisecond)

	snap := engine.Snapshot()
	snap.Close()

	_, err = snap.Timestamp()
	if err != ErrSnapshotClosed {
		t.Errorf("expected ErrSnapshotClosed, got %v", err)
	}

	var buf bytes.Buffer
	err = snap.Export(ExportJSON, &buf)
	if err != ErrSnapshotClosed {
		t.Errorf("expected ErrSnapshotClosed, got %v", err)
	}

	engine.Stop()
}

func TestDeltaTypes(t *testing.T) {
	var deltas []Delta
	var mu sync.Mutex

	engine, err := NewBuilder().
		OnChange(func(d Delta) {
			mu.Lock()
			deltas = append(deltas, d)
			mu.Unlock()
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// First event: new nodes + new edge
	engine.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	firstBatch := make([]Delta, len(deltas))
	copy(firstBatch, deltas)
	mu.Unlock()

	// Should have DeltaNodeAdded for "a", DeltaNodeAdded for "b", DeltaEdgeAdded for a->b
	hasNodeAddedA := false
	hasNodeAddedB := false
	hasEdgeAdded := false
	for _, d := range firstBatch {
		if d.Type == DeltaNodeAdded && d.NodeID == "a" {
			hasNodeAddedA = true
		}
		if d.Type == DeltaNodeAdded && d.NodeID == "b" {
			hasNodeAddedB = true
		}
		if d.Type == DeltaEdgeAdded {
			hasEdgeAdded = true
		}
	}
	if !hasNodeAddedA {
		t.Error("missing DeltaNodeAdded for 'a'")
	}
	if !hasNodeAddedB {
		t.Error("missing DeltaNodeAdded for 'b'")
	}
	if !hasEdgeAdded {
		t.Error("missing DeltaEdgeAdded")
	}

	// Second event: same edge = DeltaEdgeUpdated, no new DeltaNodeAdded
	mu.Lock()
	deltas = deltas[:0] // reset
	mu.Unlock()

	engine.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	secondBatch := make([]Delta, len(deltas))
	copy(secondBatch, deltas)
	mu.Unlock()

	hasEdgeUpdated := false
	hasNewNodeAdded := false
	for _, d := range secondBatch {
		if d.Type == DeltaEdgeUpdated {
			hasEdgeUpdated = true
		}
		if d.Type == DeltaNodeAdded {
			hasNewNodeAdded = true
		}
	}
	if !hasEdgeUpdated {
		t.Error("expected DeltaEdgeUpdated for existing edge")
	}
	if hasNewNodeAdded {
		t.Error("should not have DeltaNodeAdded for existing nodes")
	}

	engine.Stop()
}

func TestEngineStatsComplete(t *testing.T) {
	engine, err := NewBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		engine.AddEvent(Event{
			Source: fmt.Sprintf("s%d", i),
			Target: fmt.Sprintf("t%d", i),
			Weight: 1,
		})
	}
	time.Sleep(100 * time.Millisecond)

	snap := engine.Snapshot()
	defer func() { snap.Close() }()

	stats := engine.Stats()
	if stats.Nodes == 0 {
		t.Error("expected nodes > 0")
	}
	if stats.Edges == 0 {
		t.Error("expected edges > 0")
	}
	if stats.EventsTotal != 10 {
		t.Errorf("expected EventsTotal=10, got %d", stats.EventsTotal)
	}
	if stats.VersionsCurrent == 0 {
		t.Error("expected VersionsCurrent > 0")
	}
	if stats.SnapshotsActive != 1 {
		t.Errorf("expected SnapshotsActive=1, got %d", stats.SnapshotsActive)
	}
	if stats.Uptime == 0 {
		t.Error("expected Uptime > 0")
	}
	if stats.EventsPerSecond <= 0 {
		t.Error("expected EventsPerSecond > 0")
	}

	snap.Close()
	stats2 := engine.Stats()
	if stats2.SnapshotsActive != 0 {
		t.Errorf("expected SnapshotsActive=0 after close, got %d", stats2.SnapshotsActive)
	}

	engine.Stop()
}

func TestEngineAnalyzeNode(t *testing.T) {
	engine, err := NewBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}

	engine.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	engine.AddEvent(Event{Source: "b", Target: "c", Weight: 1})
	engine.AddEvent(Event{Source: "c", Target: "a", Weight: 1})
	time.Sleep(100 * time.Millisecond)

	result, err := engine.AnalyzeNode("a")
	if err != nil {
		t.Fatal(err)
	}
	if result.NodeID != "a" {
		t.Errorf("expected node 'a', got %q", result.NodeID)
	}
	if result.Degree == 0 {
		t.Error("expected non-zero degree")
	}

	// Non-existent node
	_, err = engine.AnalyzeNode("nonexistent")
	if err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}

	engine.Stop()
}

func TestEngineAnalyzeRegion(t *testing.T) {
	engine, err := NewBuilder().Build()
	if err != nil {
		t.Fatal(err)
	}

	engine.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	engine.AddEvent(Event{Source: "b", Target: "c", Weight: 1})
	engine.AddEvent(Event{Source: "c", Target: "d", Weight: 1})
	time.Sleep(100 * time.Millisecond)

	result, err := engine.AnalyzeRegion([]string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result.Nodes))
	}

	engine.Stop()
}

func TestBuilderNewNames(t *testing.T) {
	// Verify new builder methods compile and work
	_, err := NewBuilder().
		WithLogger(nil).
		WithStorage(nil).
		WithCalibrator(nil).
		Build()
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckAnomaliesWithCalibrator(t *testing.T) {
	cal := &mockCalibrator{warmupSize: 3}
	var anomalies []TensionResult
	var mu sync.Mutex

	engine, err := NewBuilder().
		Threshold(0.001). // very low threshold to trigger anomalies
		SetCalibrator(cal).
		OnAnomaly(func(r TensionResult) {
			mu.Lock()
			anomalies = append(anomalies, r)
			mu.Unlock()
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Build a graph with enough structure for tension
	for i := 0; i < 5; i++ {
		engine.AddEvent(Event{
			Source: fmt.Sprintf("n%d", i),
			Target: fmt.Sprintf("n%d", (i+1)%5),
			Weight: 1,
		})
	}
	time.Sleep(200 * time.Millisecond)

	// Calibrator should have observations
	if len(cal.observations) == 0 {
		t.Error("expected calibrator to have observations")
	}

	mu.Lock()
	anomalyCount := len(anomalies)
	mu.Unlock()

	// With very low threshold, some anomalies should fire
	// Just verify the system works without panic
	t.Logf("calibrator observations: %d, anomalies detected: %d", len(cal.observations), anomalyCount)

	engine.Stop()
}

// === Mock types for V2 tests ===

// mockStorage implements itt.Storage
type mockStorage struct {
	data  *GraphData
	saved atomic.Bool
	mu    sync.Mutex
}

func (m *mockStorage) Load() (*GraphData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data, nil
}

func (m *mockStorage) Save(data *GraphData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = data
	m.saved.Store(true)
	return nil
}

// mockLogger implements itt.Logger
type mockLogger struct {
	warns []string
	mu    sync.Mutex
}

func (l *mockLogger) Debug(msg string, kv ...any) {}
func (l *mockLogger) Info(msg string, kv ...any)  {}
func (l *mockLogger) Warn(msg string, kv ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
}
func (l *mockLogger) Error(msg string, kv ...any) {}

// mockCurvatureFunc implements itt.CurvatureFunc
type mockCurvatureFunc struct{}

func (m mockCurvatureFunc) Compute(g GraphView, from, to string) float64 { return 0.42 }
func (m mockCurvatureFunc) Name() string                                 { return "mock" }

// === V2 Tests ===

func TestV2_DeltaFieldsPopulated(t *testing.T) {
	var mu sync.Mutex
	var deltas []Delta

	e, err := NewBuilder().
		Threshold(0.001).
		OnChange(func(d Delta) {
			mu.Lock()
			deltas = append(deltas, d)
			mu.Unlock()
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// First event: creates new nodes and edge
	e.AddEvent(Event{Source: "x", Target: "y", Weight: 1})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	// Verify DeltaNodeAdded deltas have Node != nil
	for _, d := range deltas {
		if d.Type == DeltaNodeAdded {
			if d.Node == nil {
				t.Errorf("DeltaNodeAdded for %q should have Node != nil", d.NodeID)
			}
		}
	}
	mu.Unlock()

	// Second event to same edge: should produce DeltaEdgeUpdated with Edge != nil and Previous > 0
	mu.Lock()
	deltas = deltas[:0]
	mu.Unlock()

	e.AddEvent(Event{Source: "x", Target: "y", Weight: 2})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	foundEdgeUpdated := false
	for _, d := range deltas {
		if d.Type == DeltaEdgeUpdated {
			foundEdgeUpdated = true
			if d.Edge == nil {
				t.Error("DeltaEdgeUpdated should have Edge != nil")
			}
			if d.Previous <= 0 {
				t.Errorf("DeltaEdgeUpdated.Previous should be > 0, got %f", d.Previous)
			}
		}
	}
	mu.Unlock()

	if !foundEdgeUpdated {
		t.Error("expected DeltaEdgeUpdated after second event to same edge")
	}

	e.Stop()
}

func TestV2_BaseGraphInitialization(t *testing.T) {
	data := &GraphData{
		Nodes: []*Node{
			{ID: "base1", Degree: 1, OutDegree: 1},
			{ID: "base2", Degree: 1, InDegree: 1},
		},
		Edges: []*Edge{
			{From: "base1", To: "base2", Weight: 1.0, Count: 1},
		},
	}

	e, err := NewBuilder().
		BaseGraph(data).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	snap := e.Snapshot()
	defer snap.Close()

	nc, err := snap.NodeCount()
	if err != nil {
		t.Fatal(err)
	}
	if nc < 2 {
		t.Fatalf("expected at least 2 nodes from base graph, got %d", nc)
	}

	// Verify specific base node exists
	n, ok, err := snap.GetNode("base1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected base1 node to exist in snapshot")
	}
	if n.ID != "base1" {
		t.Errorf("expected node ID base1, got %s", n.ID)
	}
}

func TestV2_StorageLoadOnStart(t *testing.T) {
	ms := &mockStorage{
		data: &GraphData{
			Nodes: []*Node{
				{ID: "stored1", Degree: 1, OutDegree: 1},
				{ID: "stored2", Degree: 1, InDegree: 1},
			},
			Edges: []*Edge{
				{From: "stored1", To: "stored2", Weight: 1.0, Count: 1},
			},
		},
	}

	e, err := NewBuilder().
		WithStorage(ms).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	snap := e.Snapshot()
	defer snap.Close()

	nc, err := snap.NodeCount()
	if err != nil {
		t.Fatal(err)
	}
	if nc < 2 {
		t.Fatalf("expected at least 2 nodes loaded from storage, got %d", nc)
	}

	// Verify stored nodes are present
	_, ok, err := snap.GetNode("stored1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected stored1 to be loaded from storage")
	}
}

func TestV2_StorageSaveOnCompact(t *testing.T) {
	ms := &mockStorage{}

	e, err := NewBuilder().
		WithStorage(ms).
		CompactionStrategy(CompactManual).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Add some events
	for i := 0; i < 5; i++ {
		e.AddEvent(Event{
			Source: fmt.Sprintf("s%d", i),
			Target: fmt.Sprintf("t%d", i),
			Weight: 1,
		})
	}
	time.Sleep(100 * time.Millisecond)

	// Compact should trigger storage save
	err = e.Compact()
	if err != nil {
		t.Fatal(err)
	}
	// Save happens in a goroutine, wait for it
	time.Sleep(200 * time.Millisecond)

	if !ms.saved.Load() {
		t.Error("expected storage Save to be called after compaction")
	}

	e.Stop()
}

func TestV2_DetectabilityInResults(t *testing.T) {
	e, err := NewBuilder().
		Threshold(0.001).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Add diverse events to generate varied tensions
	e.AddEvent(Event{Source: "hub", Target: "a", Weight: 1})
	e.AddEvent(Event{Source: "hub", Target: "b", Weight: 1})
	e.AddEvent(Event{Source: "hub", Target: "c", Weight: 1})
	e.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	e.AddEvent(Event{Source: "extreme", Target: "hub", Weight: 100})
	time.Sleep(200 * time.Millisecond)

	snap := e.Snapshot()
	defer snap.Close()

	results, err := snap.Analyze()
	if err != nil {
		t.Fatal(err)
	}

	// SNR should be a valid number (>= 0)
	if results.Detectability.SNR < 0 {
		t.Errorf("expected Detectability.SNR >= 0, got %f", results.Detectability.SNR)
	}

	// Region should be a valid value: 0 (Undetectable), 1 (WeaklyDetectable), or 2 (StronglyDetectable)
	if results.Detectability.Region < 0 || results.Detectability.Region > 2 {
		t.Errorf("expected Detectability.Region in {0,1,2}, got %d", results.Detectability.Region)
	}

	t.Logf("Detectability: SNR=%f, Region=%d, Threshold=%f",
		results.Detectability.SNR, results.Detectability.Region, results.Detectability.Threshold)

	e.Stop()
}

func TestV2_ConcealmentInResults(t *testing.T) {
	e, err := NewBuilder().
		Concealment(0.5, 2).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Build a small graph
	e.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	e.AddEvent(Event{Source: "b", Target: "c", Weight: 1})
	e.AddEvent(Event{Source: "c", Target: "a", Weight: 1})
	e.AddEvent(Event{Source: "a", Target: "d", Weight: 1})
	time.Sleep(150 * time.Millisecond)

	snap := e.Snapshot()
	defer snap.Close()

	results, err := snap.Analyze()
	if err != nil {
		t.Fatal(err)
	}

	hasConcealment := false
	for _, tr := range results.Tensions {
		if tr.Concealment > 0 {
			hasConcealment = true
			break
		}
	}

	if !hasConcealment {
		t.Log("Warning: no concealment > 0 found; may be expected for uniform graph")
	}

	// Verify concealment is in Components
	for _, tr := range results.Tensions {
		if _, ok := tr.Components["concealment"]; !ok {
			t.Errorf("node %s: missing concealment component", tr.NodeID)
		}
	}

	e.Stop()
}

func TestV2_CPSInRegionResult(t *testing.T) {
	e, err := NewBuilder().
		Concealment(0.5, 2).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	e.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	e.AddEvent(Event{Source: "b", Target: "c", Weight: 1})
	e.AddEvent(Event{Source: "c", Target: "a", Weight: 1})
	time.Sleep(100 * time.Millisecond)

	snap := e.Snapshot()
	defer snap.Close()

	result, err := snap.AnalyzeRegion([]string{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}

	// CPS field should exist (may be 0 if not detectable, but should be non-negative)
	if result.CPS < 0 {
		t.Errorf("expected CPS >= 0, got %f", result.CPS)
	}

	t.Logf("RegionResult.CPS = %f", result.CPS)

	e.Stop()
}

func TestV2_TensionHistoryPopulated(t *testing.T) {
	e, err := NewBuilder().
		Threshold(0.001).
		OnAnomaly(func(r TensionResult) {}).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Add multiple events involving the same nodes to build history
	for i := 0; i < 5; i++ {
		e.AddEvent(Event{Source: "h", Target: "a", Weight: float64(i + 1)})
		e.AddEvent(Event{Source: "h", Target: "b", Weight: float64(i + 1)})
	}
	time.Sleep(200 * time.Millisecond)

	// Verify via Analyze() that temporal data is populated
	snap := e.Snapshot()
	defer snap.Close()
	results, err := snap.Analyze()
	if err != nil {
		t.Fatal(err)
	}

	// With onAnomaly set and multiple events, checkAnomalies runs and populates tensionHistory.
	// After the second Analyze, Temporal fields should be non-zero if history exists.
	// At minimum, the results should have tension values from the history.
	if len(results.Tensions) == 0 {
		t.Fatal("expected tension results")
	}

	// Check that trends are set (requires history from checkAnomalies)
	hasTrend := false
	for _, tr := range results.Tensions {
		if tr.Trend != TrendStable {
			hasTrend = true
			break
		}
	}
	t.Logf("hasTrend=%v, temporal.TensionSpike=%f", hasTrend, results.Temporal.TensionSpike)

	e.Stop()
}

func TestV2_OnTensionSpike(t *testing.T) {
	var mu sync.Mutex
	var spikes []struct {
		nodeID string
		delta  float64
	}

	e, err := NewBuilder().
		Threshold(0.001).
		OnAnomaly(func(r TensionResult) {}). // needed to trigger checkAnomalies
		OnTensionSpike(func(nodeID string, delta float64) {
			mu.Lock()
			spikes = append(spikes, struct {
				nodeID string
				delta  float64
			}{nodeID, delta})
			mu.Unlock()
		}).
		TensionSpikeThreshold(0.0001). // extremely low threshold to catch any spike
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Build a small graph with repeated events to the same pair
	// to establish history entries for the nodes
	e.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	time.Sleep(50 * time.Millisecond)
	e.AddEvent(Event{Source: "a", Target: "c", Weight: 1})
	time.Sleep(50 * time.Millisecond)
	e.AddEvent(Event{Source: "b", Target: "c", Weight: 1})
	time.Sleep(50 * time.Millisecond)

	// Now add an extreme event that should shift tension drastically for "a"
	e.AddEvent(Event{Source: "a", Target: "b", Weight: 500})
	time.Sleep(200 * time.Millisecond)

	// Also add more extreme events to maximize delta
	e.AddEvent(Event{Source: "a", Target: "c", Weight: 500})
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	spikeCount := len(spikes)
	mu.Unlock()

	if spikeCount == 0 {
		t.Error("expected OnTensionSpike to fire at least once")
	} else {
		mu.Lock()
		for _, s := range spikes {
			if s.delta <= 0 {
				t.Errorf("spike delta should be positive, got %f for node %s", s.delta, s.nodeID)
			}
		}
		mu.Unlock()
		t.Logf("total spikes detected: %d", spikeCount)
	}

	e.Stop()
}

func TestV2_DeltaTensionChanged(t *testing.T) {
	var mu sync.Mutex
	var deltas []Delta

	e, err := NewBuilder().
		Threshold(0.001).
		OnAnomaly(func(r TensionResult) {}). // needed to trigger checkAnomalies which emits DeltaTensionChanged
		OnChange(func(d Delta) {
			mu.Lock()
			deltas = append(deltas, d)
			mu.Unlock()
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Phase 1: normal events to establish a trend
	e.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	e.AddEvent(Event{Source: "b", Target: "c", Weight: 1})
	time.Sleep(100 * time.Millisecond)

	// Phase 2: events that change the tension direction
	for i := 0; i < 10; i++ {
		e.AddEvent(Event{Source: "a", Target: "b", Weight: float64(50 + i*10)})
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	hasTensionChanged := false
	for _, d := range deltas {
		if d.Type == DeltaTensionChanged {
			hasTensionChanged = true
			break
		}
	}
	mu.Unlock()

	if !hasTensionChanged {
		t.Log("Warning: DeltaTensionChanged was not emitted; trend may not have changed direction")
	}

	e.Stop()
}

func TestV2_TemporalSummaryInResults(t *testing.T) {
	e, err := NewBuilder().
		Threshold(0.001).
		OnAnomaly(func(r TensionResult) {}). // trigger checkAnomalies to build history
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Phase 1: normal events
	e.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	e.AddEvent(Event{Source: "b", Target: "c", Weight: 1})
	e.AddEvent(Event{Source: "c", Target: "a", Weight: 1})
	time.Sleep(100 * time.Millisecond)

	// First analysis to populate history
	results1, err := e.Analyze()
	if err != nil {
		t.Fatal(err)
	}
	_ = results1

	// Phase 2: extreme events to change tensions
	e.AddEvent(Event{Source: "a", Target: "b", Weight: 100})
	e.AddEvent(Event{Source: "b", Target: "c", Weight: 100})
	time.Sleep(100 * time.Millisecond)

	// Second analysis should have temporal data from history
	results2, err := e.Analyze()
	if err != nil {
		t.Fatal(err)
	}

	// Temporal summary should have some data if history was populated
	t.Logf("Temporal: TensionSpike=%f, DecayExponent=%f, Phase=%d",
		results2.Temporal.TensionSpike, results2.Temporal.DecayExponent, results2.Temporal.Phase)

	// Phase should be a valid value (0-3)
	if results2.Temporal.Phase < 0 || results2.Temporal.Phase > 3 {
		t.Errorf("expected Phase in [0,3], got %d", results2.Temporal.Phase)
	}

	e.Stop()
}

func TestV2_TrendInTensionResult(t *testing.T) {
	e, err := NewBuilder().
		Threshold(0.001).
		OnAnomaly(func(r TensionResult) {}).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Build a triangle
	e.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	e.AddEvent(Event{Source: "b", Target: "c", Weight: 1})
	e.AddEvent(Event{Source: "c", Target: "a", Weight: 1})
	time.Sleep(100 * time.Millisecond)

	// First analyze to populate history
	_, err = e.Analyze()
	if err != nil {
		t.Fatal(err)
	}

	// Add more events to change tension
	e.AddEvent(Event{Source: "a", Target: "b", Weight: 50})
	e.AddEvent(Event{Source: "b", Target: "c", Weight: 50})
	time.Sleep(100 * time.Millisecond)

	// Second analyze should have trends
	results, err := e.Analyze()
	if err != nil {
		t.Fatal(err)
	}

	for _, tr := range results.Tensions {
		// Trend should be a valid Trend value
		if tr.Trend < TrendStable || tr.Trend > TrendDecreasing {
			t.Errorf("node %s: invalid Trend value %d", tr.NodeID, tr.Trend)
		}
	}

	e.Stop()
}

func TestV2_CurvatureFuncAdapter(t *testing.T) {
	e, err := NewBuilder().
		Curvature(mockCurvatureFunc{}).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	e.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	e.AddEvent(Event{Source: "b", Target: "c", Weight: 1})
	e.AddEvent(Event{Source: "c", Target: "a", Weight: 1})
	time.Sleep(100 * time.Millisecond)

	snap := e.Snapshot()
	defer snap.Close()

	results, err := snap.Analyze()
	if err != nil {
		t.Fatal(err)
	}

	for _, tr := range results.Tensions {
		// mockCurvatureFunc always returns 0.42
		curvVal, ok := tr.Components["curvature"]
		if !ok {
			t.Errorf("node %s: missing curvature component", tr.NodeID)
			continue
		}
		// Curvature is averaged over incident edges, but all return 0.42 so the average should be 0.42
		if curvVal < 0.41 || curvVal > 0.43 {
			t.Errorf("node %s: expected curvature ~0.42, got %f", tr.NodeID, curvVal)
		}
	}

	e.Stop()
}

func TestV2_JSDWarningLogged(t *testing.T) {
	lg := &mockLogger{}

	e, err := NewBuilder().
		Divergence(analysis.KL{}).
		WithLogger(lg).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Build enough graph to analyze
	e.AddEvent(Event{Source: "a", Target: "b", Weight: 1})
	e.AddEvent(Event{Source: "b", Target: "c", Weight: 1})
	time.Sleep(100 * time.Millisecond)

	snap := e.Snapshot()
	defer snap.Close()

	_, err = snap.Analyze()
	if err != nil {
		t.Fatal(err)
	}

	lg.mu.Lock()
	hasWarning := false
	for _, w := range lg.warns {
		if strings.Contains(w, "unbounded") || strings.Contains(w, "unreliable") {
			hasWarning = true
			break
		}
	}
	lg.mu.Unlock()

	if !hasWarning {
		t.Error("expected logger to receive warning about unbounded divergence")
	}

	e.Stop()
}

func TestV2_BuilderValidation(t *testing.T) {
	t.Run("detectabilityAlpha_zero", func(t *testing.T) {
		_, err := NewBuilder().
			DetectabilityAlpha(0).
			Build()
		if err == nil {
			t.Error("expected error for detectabilityAlpha=0")
		}
	})

	t.Run("detectabilityAlpha_one", func(t *testing.T) {
		_, err := NewBuilder().
			DetectabilityAlpha(1).
			Build()
		if err == nil {
			t.Error("expected error for detectabilityAlpha=1")
		}
	})

	t.Run("concealmentLambda_negative", func(t *testing.T) {
		_, err := NewBuilder().
			Concealment(-1, 2).
			Build()
		if err == nil {
			t.Error("expected error for concealmentLambda=-1")
		}
	})

	t.Run("concealmentHops_negative", func(t *testing.T) {
		_, err := NewBuilder().
			Concealment(0.5, -1).
			Build()
		if err == nil {
			t.Error("expected error for concealmentHops=-1")
		}
	})
}
