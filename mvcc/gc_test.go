package mvcc

import (
	"context"
	"testing"
	"time"
)

func TestGC_OrphanVersionRemoved(t *testing.T) {
	ctrl := NewController()
	gc := NewGC(ctrl, GCConfig{Interval: 10 * time.Millisecond})

	v1 := &Version{ID: 1, Graph: nil, Timestamp: time.Now()}
	ctrl.Store(v1)
	gc.Track(v1)

	// Create v2, making v1 no longer current
	v2 := &Version{ID: 2, Graph: nil, Timestamp: time.Now()}
	ctrl.Store(v2)
	gc.Track(v2)

	// v1 has no refs and isn't current -> should be collected
	stats := gc.Collect()
	if stats.VersionsRemoved != 1 {
		t.Fatalf("expected 1 removed, got %d", stats.VersionsRemoved)
	}
}

func TestGC_ActiveSnapshotPreserved(t *testing.T) {
	ctrl := NewController()
	gc := NewGC(ctrl, GCConfig{Interval: 10 * time.Millisecond})

	v1 := &Version{ID: 1, Graph: nil, Timestamp: time.Now()}
	v1.Acquire() // simulate active snapshot
	ctrl.Store(v1)
	gc.Track(v1)

	// Replace with v2
	v2 := &Version{ID: 2, Graph: nil, Timestamp: time.Now()}
	ctrl.Store(v2)
	gc.Track(v2)

	// v1 has active refs -> should NOT be collected
	stats := gc.Collect()
	if stats.VersionsRemoved != 0 {
		t.Fatalf("expected 0 removed, got %d", stats.VersionsRemoved)
	}

	// Release the snapshot -> now should be collected
	v1.Release()
	stats = gc.Collect()
	if stats.VersionsRemoved != 1 {
		t.Fatalf("expected 1 removed, got %d", stats.VersionsRemoved)
	}
}

func TestGC_WarningTimeout(t *testing.T) {
	warned := false
	ctrl := NewController()
	gc := NewGC(ctrl, GCConfig{
		Interval:       10 * time.Millisecond,
		WarningTimeout: 1 * time.Millisecond,
		OnWarning: func(vID uint64, age time.Duration) {
			warned = true
		},
	})

	v1 := &Version{ID: 1, Graph: nil, Timestamp: time.Now()}
	v1.Acquire()
	ctrl.Store(v1)
	gc.Track(v1)

	v2 := &Version{ID: 2, Graph: nil, Timestamp: time.Now()}
	ctrl.Store(v2)
	gc.Track(v2)

	time.Sleep(5 * time.Millisecond)
	gc.Collect()

	if !warned {
		t.Fatal("expected warning callback")
	}
	v1.Release()
}

func TestGC_ForceClose(t *testing.T) {
	forced := false
	ctrl := NewController()
	gc := NewGC(ctrl, GCConfig{
		Interval:     10 * time.Millisecond,
		ForceTimeout: 1 * time.Millisecond,
		OnForce: func(vID uint64, age time.Duration) {
			forced = true
		},
	})

	v1 := &Version{ID: 1, Graph: nil, Timestamp: time.Now()}
	v1.Acquire()
	ctrl.Store(v1)
	gc.Track(v1)

	v2 := &Version{ID: 2, Graph: nil, Timestamp: time.Now()}
	ctrl.Store(v2)
	gc.Track(v2)

	time.Sleep(5 * time.Millisecond)
	stats := gc.Collect()

	if !forced {
		t.Fatal("expected force callback")
	}
	if stats.VersionsRemoved != 1 {
		t.Fatalf("expected 1 removed, got %d", stats.VersionsRemoved)
	}
}

func TestGC_StartStop(t *testing.T) {
	ctrl := NewController()
	gc := NewGC(ctrl, GCConfig{Interval: 10 * time.Millisecond})

	ctx := context.Background()
	gc.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	gc.Stop()
	// Should not panic
}

func TestGC_CurrentVersionNotCollected(t *testing.T) {
	ctrl := NewController()
	gc := NewGC(ctrl, GCConfig{})

	v1 := &Version{ID: 1, Graph: nil, Timestamp: time.Now()}
	ctrl.Store(v1)
	gc.Track(v1)

	stats := gc.Collect()
	if stats.VersionsRemoved != 0 {
		t.Fatalf("current version should not be collected")
	}
}
