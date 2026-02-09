package mvcc

import (
	"sync"
	"testing"
	"time"

	"github.com/MatheusGrego/itt-engine/graph"
)

func TestVersion_StoreAndLoad(t *testing.T) {
	vc := NewController()
	ig := graph.NewImmutableEmpty()
	v := &Version{
		ID:        1,
		Graph:     ig,
		Timestamp: time.Now(),
	}
	vc.Store(v)

	got := vc.Load()
	if got.ID != 1 {
		t.Fatalf("expected version 1, got %d", got.ID)
	}
}

func TestVersion_Acquire_Release(t *testing.T) {
	vc := NewController()
	ig := graph.NewImmutableEmpty()
	v := &Version{ID: 1, Graph: ig, Timestamp: time.Now()}
	vc.Store(v)

	got := vc.Acquire()
	if got.RefCount() != 1 {
		t.Fatalf("expected refcount 1, got %d", got.RefCount())
	}

	got.Release()
	if got.RefCount() != 0 {
		t.Fatalf("expected refcount 0, got %d", got.RefCount())
	}
}

func TestVersion_Acquire_MultipleSnapshots(t *testing.T) {
	vc := NewController()
	ig := graph.NewImmutableEmpty()
	v := &Version{ID: 1, Graph: ig, Timestamp: time.Now()}
	vc.Store(v)

	a := vc.Acquire()
	b := vc.Acquire()

	if a.RefCount() != 2 {
		t.Fatalf("expected refcount 2, got %d", a.RefCount())
	}

	a.Release()
	if b.RefCount() != 1 {
		t.Fatalf("expected refcount 1 after one release, got %d", b.RefCount())
	}

	b.Release()
	if a.RefCount() != 0 {
		t.Fatalf("expected refcount 0, got %d", a.RefCount())
	}
}

func TestVersion_ReleaseIdempotent(t *testing.T) {
	vc := NewController()
	ig := graph.NewImmutableEmpty()
	v := &Version{ID: 1, Graph: ig, Timestamp: time.Now()}
	vc.Store(v)

	snap := vc.Acquire()
	snap.Release()
	snap.Release() // should not panic or go negative
	if snap.RefCount() != 0 {
		t.Fatalf("expected refcount 0, got %d", snap.RefCount())
	}
}

func TestVersion_SnapshotIsolation(t *testing.T) {
	vc := NewController()
	g1 := graph.New()
	g1.AddNode(&graph.NodeData{ID: "a"})
	ig1 := graph.NewImmutable(g1)
	vc.Store(&Version{ID: 1, Graph: ig1, Timestamp: time.Now()})

	snap := vc.Acquire()

	// New version
	g2 := graph.New()
	g2.AddNode(&graph.NodeData{ID: "a"})
	g2.AddNode(&graph.NodeData{ID: "b"})
	ig2 := graph.NewImmutable(g2)
	vc.Store(&Version{ID: 2, Graph: ig2, Timestamp: time.Now()})

	// Snapshot still sees old version
	if snap.Graph.NodeCount() != 1 {
		t.Fatalf("snapshot should see 1 node, got %d", snap.Graph.NodeCount())
	}

	// Current sees new version
	current := vc.Load()
	if current.Graph.NodeCount() != 2 {
		t.Fatalf("current should see 2 nodes, got %d", current.Graph.NodeCount())
	}

	snap.Release()
}

func TestVersion_LoadNil(t *testing.T) {
	vc := NewController()
	v := vc.Load()
	if v != nil {
		t.Fatal("expected nil from empty controller")
	}
}

func TestVersion_AcquireNil(t *testing.T) {
	vc := NewController()
	v := vc.Acquire()
	if v != nil {
		t.Fatal("expected nil from empty controller")
	}
}

func TestVersion_ConcurrentAcquireRelease(t *testing.T) {
	vc := NewController()
	ig := graph.NewImmutableEmpty()
	v := &Version{ID: 1, Graph: ig, Timestamp: time.Now()}
	vc.Store(v)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snap := vc.Acquire()
			if snap == nil {
				t.Error("expected non-nil snapshot")
				return
			}
			snap.Release()
		}()
	}
	wg.Wait()

	if v.RefCount() != 0 {
		t.Fatalf("expected refcount 0 after all releases, got %d", v.RefCount())
	}
}
