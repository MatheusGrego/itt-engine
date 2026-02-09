package analysis

import (
	"math"
	"testing"
)

func TestCalibrator_NotWarmedUpInitially(t *testing.T) {
	c := NewCalibrator(WithWarmupSize(10))
	if c.IsWarmedUp() {
		t.Fatal("should not be warmed up initially")
	}
}

func TestCalibrator_NeverAnomalyBeforeWarmup(t *testing.T) {
	c := NewCalibrator(WithWarmupSize(10))
	for i := 0; i < 5; i++ {
		c.Observe(0.1)
	}
	if c.IsAnomaly(999.0) {
		t.Fatal("should never report anomaly before warm-up")
	}
}

func TestCalibrator_WarmsUpAfterN(t *testing.T) {
	c := NewCalibrator(WithWarmupSize(10))
	for i := 0; i < 10; i++ {
		c.Observe(0.1 * float64(i))
	}
	if !c.IsWarmedUp() {
		t.Fatal("should be warmed up after 10 observations")
	}
}

func TestCalibrator_ThresholdFormula(t *testing.T) {
	// Use known data: [1, 2, 3, 4, 5]
	// Median = 3
	// Deviations = [2, 1, 0, 1, 2], MAD = 1
	// K=2 => threshold = 3 + 2*1 = 5
	c := NewCalibrator(WithK(2.0), WithWarmupSize(5))
	for _, v := range []float64{1, 2, 3, 4, 5} {
		c.Observe(v)
	}
	stats := c.Stats()
	if math.Abs(stats.Median-3.0) > 1e-10 {
		t.Fatalf("expected median 3.0, got %f", stats.Median)
	}
	if math.Abs(stats.MAD-1.0) > 1e-10 {
		t.Fatalf("expected MAD 1.0, got %f", stats.MAD)
	}
	if math.Abs(stats.Threshold-5.0) > 1e-10 {
		t.Fatalf("expected threshold 5.0, got %f", stats.Threshold)
	}
}

func TestCalibrator_IsAnomaly(t *testing.T) {
	c := NewCalibrator(WithK(2.0), WithWarmupSize(5))
	for _, v := range []float64{1, 2, 3, 4, 5} {
		c.Observe(v)
	}
	// threshold = 5.0
	if c.IsAnomaly(4.9) {
		t.Fatal("4.9 should not be anomaly (threshold 5.0)")
	}
	if !c.IsAnomaly(5.1) {
		t.Fatal("5.1 should be anomaly (threshold 5.0)")
	}
}

func TestCalibrator_Recalibrate(t *testing.T) {
	c := NewCalibrator(WithK(3.0), WithWarmupSize(5))
	for _, v := range []float64{1, 2, 3, 4, 5} {
		c.Observe(v)
	}
	t1 := c.Threshold()

	// Add more observations and recalibrate
	for _, v := range []float64{100, 200, 300} {
		c.Observe(v)
	}
	c.Recalibrate()
	t2 := c.Threshold()

	if t2 <= t1 {
		t.Fatalf("threshold should increase after adding large values: %f <= %f", t2, t1)
	}
}

func TestCalibrator_PrecomputedBaseline(t *testing.T) {
	c := NewCalibrator(WithK(3.0), WithPrecomputedBaseline(0.15, 0.08))
	if !c.IsWarmedUp() {
		t.Fatal("should be warmed up with precomputed baseline")
	}
	expected := 0.15 + 3.0*0.08
	if math.Abs(c.Threshold()-expected) > 1e-10 {
		t.Fatalf("expected threshold %f, got %f", expected, c.Threshold())
	}
}

func TestCalibrator_Stats(t *testing.T) {
	c := NewCalibrator(WithK(2.5), WithWarmupSize(3))
	c.Observe(1.0)
	c.Observe(2.0)
	c.Observe(3.0)

	stats := c.Stats()
	if stats.SamplesObserved != 3 {
		t.Fatalf("expected 3 samples, got %d", stats.SamplesObserved)
	}
	if stats.K != 2.5 {
		t.Fatalf("expected K=2.5, got %f", stats.K)
	}
	if !stats.IsWarmedUp {
		t.Fatal("should be warmed up")
	}
}

func TestCalibrator_EvenSampleMedian(t *testing.T) {
	// [1, 2, 3, 4] => median = 2.5
	c := NewCalibrator(WithK(1.0), WithWarmupSize(4))
	for _, v := range []float64{1, 2, 3, 4} {
		c.Observe(v)
	}
	stats := c.Stats()
	if math.Abs(stats.Median-2.5) > 1e-10 {
		t.Fatalf("expected median 2.5, got %f", stats.Median)
	}
}
