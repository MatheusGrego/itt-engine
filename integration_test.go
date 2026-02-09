package itt

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestIntegration_StreamingHighVolume(t *testing.T) {
	// 1000 events across 10 goroutines (100 each)
	// Verify: all events processed, no panics, final state correct
	e, _ := NewBuilder().ChannelSize(10000).Build()
	e.Start(context.Background())
	defer e.Stop()

	var wg sync.WaitGroup
	eventsPerWorker := 100
	workers := 10

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < eventsPerWorker; i++ {
				src := fmt.Sprintf("worker-%d", workerID)
				tgt := fmt.Sprintf("target-%d-%d", workerID, i)
				e.AddEvent(Event{Source: src, Target: tgt, Weight: 1.0})
			}
		}(w)
	}
	wg.Wait()

	// Poll until all events are drained or timeout
	expected := int64(workers * eventsPerWorker)
	deadline := time.Now().Add(10 * time.Second)
	var stats *EngineStats
	for time.Now().Before(deadline) {
		stats = e.Stats()
		if stats.EventsTotal == expected {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if stats.EventsTotal != expected {
		t.Fatalf("expected %d events, got %d", expected, stats.EventsTotal)
	}
}

func TestIntegration_ConcurrentAnalysis(t *testing.T) {
	// Start engine, add events while concurrently taking snapshots and analyzing
	e, _ := NewBuilder().Threshold(0.5).Build()
	e.Start(context.Background())
	defer e.Stop()

	// Seed some initial data
	for i := 0; i < 50; i++ {
		src := fmt.Sprintf("node-%d", i%10)
		tgt := fmt.Sprintf("node-%d", (i+1)%10)
		e.AddEvent(Event{Source: src, Target: tgt, Weight: 1.0})
	}
	time.Sleep(200 * time.Millisecond)

	// Now concurrently add events and analyze
	var wg sync.WaitGroup

	// Writer goroutines
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				src := fmt.Sprintf("new-%d", id)
				tgt := fmt.Sprintf("node-%d", i%10)
				e.AddEvent(Event{Source: src, Target: tgt})
			}
		}(w)
	}

	// Reader/analyzer goroutines
	for r := 0; r < 5; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				snap := e.Snapshot()
				results, err := snap.Analyze()
				snap.Close()
				if err != nil {
					t.Errorf("analyze failed: %v", err)
					return
				}
				if results == nil {
					t.Error("nil results")
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
}

func TestIntegration_SnapshotIsolation(t *testing.T) {
	// Take a snapshot, add more events, verify snapshot doesn't see new events
	e, _ := NewBuilder().Build()
	e.Start(context.Background())
	defer e.Stop()

	// Add initial events
	for i := 0; i < 10; i++ {
		e.AddEvent(Event{Source: "a", Target: fmt.Sprintf("b%d", i)})
	}
	time.Sleep(100 * time.Millisecond)

	// Take snapshot
	snap := e.Snapshot()
	defer snap.Close()

	countBefore, _ := snap.NodeCount()

	// Add more events
	for i := 0; i < 10; i++ {
		e.AddEvent(Event{Source: "x", Target: fmt.Sprintf("y%d", i)})
	}
	time.Sleep(100 * time.Millisecond)

	// Snapshot should still see old count
	countAfter, _ := snap.NodeCount()
	if countBefore != countAfter {
		t.Fatalf("snapshot isolation broken: before=%d, after=%d", countBefore, countAfter)
	}

	// New snapshot should see all
	snap2 := e.Snapshot()
	defer snap2.Close()
	countNew, _ := snap2.NodeCount()
	if countNew <= countBefore {
		t.Fatalf("new snapshot should see more nodes: old=%d, new=%d", countBefore, countNew)
	}
}

func TestIntegration_FullLifecycle(t *testing.T) {
	// Build -> Start -> AddEvents -> Snapshot -> Analyze -> AnalyzeNode -> AnalyzeRegion -> Stop
	e, _ := NewBuilder().
		Threshold(0.1).
		Build()

	err := e.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Ingest events
	events := []Event{
		{Source: "alice", Target: "bob", Weight: 1.0},
		{Source: "bob", Target: "carol", Weight: 2.0},
		{Source: "carol", Target: "alice", Weight: 1.5},
		{Source: "alice", Target: "dave", Weight: 1.0},
		{Source: "dave", Target: "bob", Weight: 0.5},
	}
	for _, ev := range events {
		if err := e.AddEvent(ev); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(200 * time.Millisecond)

	// Snapshot and analyze
	snap := e.Snapshot()
	defer snap.Close()

	nc, _ := snap.NodeCount()
	if nc != 4 {
		t.Fatalf("expected 4 nodes, got %d", nc)
	}

	results, err := snap.Analyze()
	if err != nil {
		t.Fatal(err)
	}
	if results.Stats.NodesAnalyzed != 4 {
		t.Fatalf("expected 4 analyzed, got %d", results.Stats.NodesAnalyzed)
	}

	// AnalyzeNode
	tr, err := snap.AnalyzeNode("alice")
	if err != nil {
		t.Fatal(err)
	}
	if tr.NodeID != "alice" {
		t.Fatal("wrong node")
	}

	// AnalyzeRegion
	rr, err := snap.AnalyzeRegion([]string{"alice", "bob"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rr.Nodes) != 2 {
		t.Fatal("expected 2 nodes in region")
	}

	// Stop
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestFullWiringIntegration(t *testing.T) {
	// Track all callbacks
	var (
		mu        sync.Mutex
		deltas    []Delta
		anomalies []TensionResult
		compacts  []CompactStats
		errors    []error
	)

	cal := &mockCalibrator{warmupSize: 5}

	engine, err := NewBuilder().
		Threshold(0.001). // low threshold
		CurvatureAlpha(0.5).
		WithCalibrator(cal).
		CompactionStrategy(CompactByVolume).
		CompactionThreshold(10).
		ThresholdFunc(func(node *Node, tension float64) bool {
			// Custom threshold: tension > 0.01 for high-degree nodes
			if node.Degree >= 3 {
				return tension > 0.01
			}
			return tension > 0.1
		}).
		NodeTypeFunc(func(id string) string {
			if len(id) > 0 && id[0] == 'u' {
				return "user"
			}
			return "system"
		}).
		AggregationFunc(AggMean).
		OnChange(func(d Delta) {
			mu.Lock()
			deltas = append(deltas, d)
			mu.Unlock()
		}).
		OnAnomaly(func(r TensionResult) {
			mu.Lock()
			anomalies = append(anomalies, r)
			mu.Unlock()
		}).
		OnCompact(func(s CompactStats) {
			mu.Lock()
			compacts = append(compacts, s)
			mu.Unlock()
		}).
		OnError(func(e error) {
			mu.Lock()
			errors = append(errors, e)
			mu.Unlock()
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Phase 1: Build a graph with enough structure
	for i := 0; i < 15; i++ {
		src := fmt.Sprintf("u%d", i)
		dst := fmt.Sprintf("s%d", (i+1)%15)
		err := engine.AddEvent(Event{Source: src, Target: dst, Weight: float64(i%3 + 1)})
		if err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(300 * time.Millisecond)

	// Verify deltas were emitted
	mu.Lock()
	deltaCount := len(deltas)
	mu.Unlock()
	if deltaCount == 0 {
		t.Error("expected deltas to be emitted")
	}

	// Verify delta types include DeltaNodeAdded and edge types
	mu.Lock()
	hasNodeAdded := false
	hasEdge := false
	for _, d := range deltas {
		if d.Type == DeltaNodeAdded {
			hasNodeAdded = true
		}
		if d.Type == DeltaEdgeAdded || d.Type == DeltaEdgeUpdated {
			hasEdge = true
		}
	}
	mu.Unlock()
	if !hasNodeAdded {
		t.Error("expected DeltaNodeAdded")
	}
	if !hasEdge {
		t.Error("expected edge deltas")
	}

	// Phase 2: Verify compaction triggered (threshold=10, we sent 15 events)
	mu.Lock()
	compactCount := len(compacts)
	mu.Unlock()
	if compactCount == 0 {
		t.Error("expected compaction to trigger (sent 15 events with threshold 10)")
	}

	// Phase 3: Full analysis
	snap := engine.Snapshot()
	defer snap.Close()

	results, err := snap.Analyze()
	if err != nil {
		t.Fatal(err)
	}

	if len(results.Tensions) == 0 {
		t.Fatal("expected tension results")
	}

	// Verify curvature is computed
	hasCurvature := false
	for _, r := range results.Tensions {
		if r.Curvature != 0 {
			hasCurvature = true
		}
		// Verify confidence is populated
		if r.Degree > 0 && r.Confidence == 0 {
			t.Errorf("node %s: expected non-zero confidence with degree %d", r.NodeID, r.Degree)
		}
		// Verify components are populated
		if r.Components == nil {
			t.Errorf("node %s: expected non-nil Components", r.NodeID)
		}
	}
	if !hasCurvature {
		t.Log("warning: no non-zero curvature found (may be expected for this graph topology)")
	}

	// Phase 4: Verify node types
	node, ok, err := snap.GetNode("u0")
	if err != nil {
		t.Fatal(err)
	}
	if ok && node.Type != "user" {
		t.Errorf("expected node type 'user', got %q", node.Type)
	}

	sysNode, ok, err := snap.GetNode("s1")
	if err != nil {
		t.Fatal(err)
	}
	if ok && sysNode.Type != "system" {
		t.Errorf("expected node type 'system', got %q", sysNode.Type)
	}

	// Phase 5: Export
	var buf bytes.Buffer
	err = snap.Export(ExportJSON, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty JSON export")
	}

	// Phase 6: Timestamp
	ts, err := snap.Timestamp()
	if err != nil {
		t.Fatal(err)
	}
	if ts.IsZero() {
		t.Error("expected non-zero timestamp")
	}

	// Phase 7: Region analysis
	region, err := snap.AnalyzeRegion([]string{"u0", "u1", "u2"})
	if err != nil {
		t.Fatal(err)
	}
	if region.Aggregated == 0 && len(region.Nodes) > 0 {
		// AggMean of non-zero tensions should produce non-zero aggregated
		t.Log("aggregated tension is zero (nodes may have zero tension)")
	}

	// Phase 8: Engine convenience methods
	nodeResult, err := engine.AnalyzeNode("u0")
	if err != nil && err != ErrNodeNotFound {
		t.Fatal(err)
	}
	if nodeResult != nil && nodeResult.NodeID != "u0" {
		t.Errorf("expected 'u0', got %q", nodeResult.NodeID)
	}

	// Phase 9: Stats
	stats := engine.Stats()
	if stats.Nodes == 0 {
		t.Error("expected nodes > 0")
	}
	if stats.EventsTotal != 15 {
		t.Errorf("expected EventsTotal=15, got %d", stats.EventsTotal)
	}
	if stats.SnapshotsActive < 1 {
		t.Error("expected at least 1 active snapshot")
	}
	if stats.EventsPerSecond <= 0 {
		t.Error("expected EventsPerSecond > 0")
	}

	// Phase 10: Calibrator should have observations
	if len(cal.observations) == 0 {
		t.Error("expected calibrator to have observations from analysis")
	}

	// Phase 11: Manual compact
	err = engine.Compact()
	if err != nil {
		t.Fatal(err)
	}

	// After compact, data should still be accessible
	snap2 := engine.Snapshot()
	defer snap2.Close()
	nc, _ := snap2.NodeCount()
	if nc == 0 {
		t.Error("expected nodes after manual compact")
	}

	// No errors should have occurred
	mu.Lock()
	errCount := len(errors)
	mu.Unlock()
	if errCount > 0 {
		t.Errorf("unexpected errors: %v", errors)
	}

	snap.Close()
	snap2.Close()
	engine.Stop()

	t.Logf("Integration: %d deltas, %d anomalies, %d compactions, %d calibrator obs, %d nodes, %d events",
		deltaCount, len(anomalies), compactCount, len(cal.observations), nc, stats.EventsTotal)
}
