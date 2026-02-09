package itt

import "time"

// NewBuilder creates a new engine builder with sensible defaults.
func NewBuilder() *Builder {
	return &Builder{
		threshold:           0.2,
		gcSnapshotWarning:   5 * time.Minute,
		gcSnapshotForce:     15 * time.Minute,
		maxOverlaySize:      100000,
		compactionStrategy:  CompactByVolume,
		compactionThreshold: 10000,
		channelSize:         10000,
	}
}
