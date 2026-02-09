package itt

import (
	"fmt"
	"sync"

	"github.com/mfreiregr/itt-engine/graph"
	"github.com/mfreiregr/itt-engine/mvcc"
)

// Snapshot is a read-only, point-in-time view of the graph.
type Snapshot struct {
	version *mvcc.Version
	config  *Builder
	closed  bool
	mu      sync.Mutex
}

func newSnapshot(v *mvcc.Version, cfg *Builder) *Snapshot {
	return &Snapshot{version: v, config: cfg}
}

// ID returns the snapshot identifier.
func (s *Snapshot) ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.version == nil {
		return ""
	}
	return fmt.Sprintf("snap-%d", s.version.ID)
}

// Version returns the MVCC version number.
func (s *Snapshot) Version() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.version == nil {
		return 0
	}
	return s.version.ID
}

// Close releases the snapshot's reference to its version.
func (s *Snapshot) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.version != nil {
		s.version.Release()
	}
	return nil
}

func (s *Snapshot) checkClosed() error {
	if s.closed {
		return ErrSnapshotClosed
	}
	return nil
}

// NodeCount returns the number of nodes.
func (s *Snapshot) NodeCount() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return 0, err
	}
	return s.version.Graph.NodeCount(), nil
}

// EdgeCount returns the number of edges.
func (s *Snapshot) EdgeCount() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return 0, err
	}
	return s.version.Graph.EdgeCount(), nil
}

// GetNode returns a node by ID.
func (s *Snapshot) GetNode(id string) (*graph.NodeData, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, false, err
	}
	n, ok := s.version.Graph.GetNode(id)
	return n, ok, nil
}

// GetEdge returns an edge by endpoints.
func (s *Snapshot) GetEdge(from, to string) (*graph.EdgeData, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, false, err
	}
	e, ok := s.version.Graph.GetEdge(from, to)
	return e, ok, nil
}

// Neighbors returns all neighbor IDs.
func (s *Snapshot) Neighbors(nodeID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	return s.version.Graph.Neighbors(nodeID), nil
}

// InNeighbors returns incoming neighbor IDs.
func (s *Snapshot) InNeighbors(nodeID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	return s.version.Graph.InNeighbors(nodeID), nil
}

// OutNeighbors returns outgoing neighbor IDs.
func (s *Snapshot) OutNeighbors(nodeID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return nil, err
	}
	return s.version.Graph.OutNeighbors(nodeID), nil
}

// ForEachNode iterates all nodes. Return false to stop.
func (s *Snapshot) ForEachNode(fn func(*graph.NodeData) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return err
	}
	s.version.Graph.ForEachNode(fn)
	return nil
}

// ForEachEdge iterates all edges. Return false to stop.
func (s *Snapshot) ForEachEdge(fn func(*graph.EdgeData) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkClosed(); err != nil {
		return err
	}
	s.version.Graph.ForEachEdge(fn)
	return nil
}
