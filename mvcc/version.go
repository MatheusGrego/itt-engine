package mvcc

import (
	"sync/atomic"
	"time"

	"github.com/MatheusGrego/itt-engine/graph"
)

// Version is an immutable snapshot of graph state with reference counting.
type Version struct {
	ID        uint64
	Graph     *graph.ImmutableGraph
	Timestamp time.Time
	Dirty     map[string]bool
	refCount  atomic.Int64
}

// RefCount returns the current number of active references.
func (v *Version) RefCount() int64 {
	return v.refCount.Load()
}

// Acquire increments the reference count.
func (v *Version) Acquire() {
	v.refCount.Add(1)
}

// Release decrements the reference count. Safe to call multiple times at zero.
func (v *Version) Release() {
	for {
		cur := v.refCount.Load()
		if cur <= 0 {
			return
		}
		if v.refCount.CompareAndSwap(cur, cur-1) {
			return
		}
	}
}

// Controller manages the current graph version using atomic pointer swap.
type Controller struct {
	current atomic.Pointer[Version]
}

// NewController creates a new MVCC controller.
func NewController() *Controller {
	return &Controller{}
}

// Store atomically replaces the current version.
func (c *Controller) Store(v *Version) {
	c.current.Store(v)
}

// Load returns the current version without incrementing refcount.
func (c *Controller) Load() *Version {
	return c.current.Load()
}

// Acquire atomically loads the current version and increments its refcount.
func (c *Controller) Acquire() *Version {
	v := c.current.Load()
	if v != nil {
		v.Acquire()
	}
	return v
}
