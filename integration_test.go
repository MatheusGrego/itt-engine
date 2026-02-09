package itt

import (
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
