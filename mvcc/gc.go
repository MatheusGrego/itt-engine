package mvcc

import (
	"context"
	"sync"
	"time"
)

// GCConfig holds garbage collector configuration.
type GCConfig struct {
	Interval       time.Duration                              // how often to run GC sweep (default 30s)
	WarningTimeout time.Duration                              // warn if snapshot held longer than this
	ForceTimeout   time.Duration                              // force-close snapshots held longer than this
	OnWarning      func(versionID uint64, age time.Duration)  // optional callback
	OnForce        func(versionID uint64, age time.Duration)  // optional callback
}

// CollectStats holds GC run results.
type CollectStats struct {
	VersionsRemoved int
	MemoryFreed     int64
	OldestRemoved   uint64
	Timestamp       time.Time
}

// GC is a garbage collector for MVCC versions.
type GC struct {
	controller *Controller
	config     GCConfig

	// Track registered versions with their creation time
	mu       sync.Mutex
	versions map[uint64]*trackedVersion

	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

type trackedVersion struct {
	version   *Version
	createdAt time.Time
	warned    bool
}

// NewGC creates a new garbage collector.
func NewGC(ctrl *Controller, cfg GCConfig) *GC {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	return &GC{
		controller: ctrl,
		config:     cfg,
		versions:   make(map[uint64]*trackedVersion),
	}
}

// Track registers a version for GC tracking.
func (gc *GC) Track(v *Version) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.versions[v.ID] = &trackedVersion{
		version:   v,
		createdAt: time.Now(),
	}
}

// Start begins the GC goroutine.
func (gc *GC) Start(ctx context.Context) {
	gc.mu.Lock()
	if gc.running {
		gc.mu.Unlock()
		return
	}
	gc.running = true
	childCtx, cancel := context.WithCancel(ctx)
	gc.cancel = cancel
	gc.mu.Unlock()

	gc.wg.Add(1)
	go func() {
		defer gc.wg.Done()
		ticker := time.NewTicker(gc.config.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-childCtx.Done():
				return
			case <-ticker.C:
				gc.Collect()
			}
		}
	}()
}

// Stop halts the GC goroutine.
func (gc *GC) Stop() {
	gc.mu.Lock()
	if !gc.running {
		gc.mu.Unlock()
		return
	}
	gc.running = false
	cancel := gc.cancel
	gc.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	gc.wg.Wait()
}

// Collect runs one GC sweep immediately and returns stats.
func (gc *GC) Collect() CollectStats {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	stats := CollectStats{
		Timestamp: time.Now(),
	}

	current := gc.controller.Load()

	var toRemove []uint64

	for id, tv := range gc.versions {
		// Never collect the current version.
		if current != nil && id == current.ID {
			continue
		}

		age := time.Since(tv.createdAt)
		refCount := tv.version.RefCount()

		if refCount == 0 {
			// Orphaned version -- collect it.
			toRemove = append(toRemove, id)
			continue
		}

		// RefCount > 0: version is still held by a snapshot.

		// Force-close check (takes precedence over warning).
		if gc.config.ForceTimeout > 0 && age > gc.config.ForceTimeout {
			if gc.config.OnForce != nil {
				gc.config.OnForce(id, age)
			}
			// Force release all references.
			for tv.version.RefCount() > 0 {
				tv.version.Release()
			}
			toRemove = append(toRemove, id)
			continue
		}

		// Warning check.
		if gc.config.WarningTimeout > 0 && age > gc.config.WarningTimeout && !tv.warned {
			tv.warned = true
			if gc.config.OnWarning != nil {
				gc.config.OnWarning(id, age)
			}
		}
	}

	for _, id := range toRemove {
		tv := gc.versions[id]
		if stats.OldestRemoved == 0 || tv.version.ID < stats.OldestRemoved {
			stats.OldestRemoved = tv.version.ID
		}
		delete(gc.versions, id)
		stats.VersionsRemoved++
	}

	return stats
}
