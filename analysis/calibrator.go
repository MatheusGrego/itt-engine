package analysis

import (
	"math"
	"sort"
	"sync"
	"time"
)

// CalibratorStats holds calibration state.
type CalibratorStats struct {
	SamplesObserved   int
	Median            float64
	MAD               float64
	Threshold         float64
	K                 float64
	IsWarmedUp        bool
	LastRecalibration time.Time
}

// MADCalibrator implements dynamic threshold calibration using
// Median Absolute Deviation, which is robust to power-law outliers.
type MADCalibrator struct {
	mu           sync.Mutex
	observations []float64
	k            float64
	warmupSize   int
	warmedUp     bool
	median       float64
	mad          float64
	threshold    float64
	lastRecal    time.Time
}

// CalibratorOption configures a MADCalibrator.
type CalibratorOption func(*MADCalibrator)

// WithK sets the sensitivity multiplier (default 3.0).
func WithK(k float64) CalibratorOption {
	return func(c *MADCalibrator) { c.k = k }
}

// WithWarmupSize sets the number of samples before calibration activates (default 1000).
func WithWarmupSize(n int) CalibratorOption {
	return func(c *MADCalibrator) { c.warmupSize = n }
}

// WithPrecomputedBaseline sets a known median and MAD, skipping warm-up.
func WithPrecomputedBaseline(median, mad float64) CalibratorOption {
	return func(c *MADCalibrator) {
		c.median = median
		c.mad = mad
		c.threshold = median + c.k*mad
		c.warmedUp = true
		c.lastRecal = time.Now()
	}
}

// NewCalibrator creates a MADCalibrator with the given options.
func NewCalibrator(opts ...CalibratorOption) *MADCalibrator {
	c := &MADCalibrator{
		k:          3.0,
		warmupSize: 1000,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Observe records a tension value for calibration.
func (c *MADCalibrator) Observe(tension float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.observations = append(c.observations, tension)
	if !c.warmedUp && len(c.observations) >= c.warmupSize {
		c.recalibrate()
	}
}

// IsWarmedUp returns true if calibration has completed initial warm-up.
func (c *MADCalibrator) IsWarmedUp() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.warmedUp
}

// Threshold returns the current anomaly threshold.
func (c *MADCalibrator) Threshold() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.threshold
}

// IsAnomaly returns true if the tension exceeds the current threshold.
func (c *MADCalibrator) IsAnomaly(tension float64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.warmedUp {
		return false
	}
	return tension > c.threshold
}

// Stats returns current calibration statistics.
func (c *MADCalibrator) Stats() CalibratorStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CalibratorStats{
		SamplesObserved:   len(c.observations),
		Median:            c.median,
		MAD:               c.mad,
		Threshold:         c.threshold,
		K:                 c.k,
		IsWarmedUp:        c.warmedUp,
		LastRecalibration: c.lastRecal,
	}
}

// Recalibrate forces a recalculation of the threshold from current observations.
func (c *MADCalibrator) Recalibrate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.observations) > 0 {
		c.recalibrate()
	}
}

func (c *MADCalibrator) recalibrate() {
	c.median = computeMedian(c.observations)
	deviations := make([]float64, len(c.observations))
	for i, v := range c.observations {
		deviations[i] = math.Abs(v - c.median)
	}
	c.mad = computeMedian(deviations)
	c.threshold = c.median + c.k*c.mad
	c.warmedUp = true
	c.lastRecal = time.Now()
}

func computeMedian(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2.0
	}
	return sorted[n/2]
}
