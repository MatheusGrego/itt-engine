package itt

import (
	"context"
	"sync"
	"testing"
	"time"
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
