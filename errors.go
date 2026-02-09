package itt

import "errors"

var (
	ErrEmptySource    = errors.New("event source cannot be empty")
	ErrEmptyTarget    = errors.New("event target cannot be empty")
	ErrNegativeWeight = errors.New("event weight cannot be negative")

	ErrEngineStopped  = errors.New("engine is not running")
	ErrEngineRunning  = errors.New("engine is already running")
	ErrSnapshotClosed = errors.New("snapshot is closed")

	ErrNodeNotFound = errors.New("node not found")
	ErrEdgeNotFound = errors.New("edge not found")

	ErrInvalidConfig = errors.New("invalid configuration")
)
