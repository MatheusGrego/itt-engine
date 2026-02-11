package analysis

import (
	"math"
	"testing"
	"time"
)

func TestTensionHistory_PushAndSlice(t *testing.T) {
	h := NewTensionHistory(10)
	for i := 0; i < 5; i++ {
		h.Push(TensionSample{
			Tension:   float64(i),
			Timestamp: time.Now(),
			Version:   uint64(i),
		})
	}
	if h.Len() != 5 {
		t.Fatalf("expected Len()==5, got %d", h.Len())
	}
	s := h.Slice()
	if len(s) != 5 {
		t.Fatalf("expected Slice() length 5, got %d", len(s))
	}
	for i, sample := range s {
		if sample.Tension != float64(i) {
			t.Fatalf("Slice()[%d].Tension = %f, want %f", i, sample.Tension, float64(i))
		}
	}
}

func TestTensionHistory_Overflow(t *testing.T) {
	h := NewTensionHistory(100)
	for i := 0; i < 150; i++ {
		h.Push(TensionSample{
			Tension:   float64(i),
			Timestamp: time.Now(),
			Version:   uint64(i),
		})
	}
	if h.Len() != 100 {
		t.Fatalf("expected Len()==100 after overflow, got %d", h.Len())
	}
	s := h.Slice()
	if len(s) != 100 {
		t.Fatalf("expected Slice() length 100, got %d", len(s))
	}
	// First element should be sample #50 (the oldest surviving sample).
	if s[0].Tension != 50.0 {
		t.Fatalf("expected first element tension=50.0, got %f", s[0].Tension)
	}
	// Last element should be sample #149.
	if s[99].Tension != 149.0 {
		t.Fatalf("expected last element tension=149.0, got %f", s[99].Tension)
	}
	// Check chronological order.
	for i := 1; i < len(s); i++ {
		if s[i].Tension <= s[i-1].Tension {
			t.Fatalf("Slice() not in chronological order at index %d: %f <= %f", i, s[i].Tension, s[i-1].Tension)
		}
	}
}

func TestTensionHistory_Latest(t *testing.T) {
	h := NewTensionHistory(10)
	for i := 0; i < 3; i++ {
		h.Push(TensionSample{
			Tension:   float64(i + 1),
			Timestamp: time.Now(),
			Version:   uint64(i),
		})
	}
	latest, ok := h.Latest()
	if !ok {
		t.Fatal("expected Latest() to return true")
	}
	if latest.Tension != 3.0 {
		t.Fatalf("expected Latest().Tension == 3.0, got %f", latest.Tension)
	}
}

func TestTensionHistory_Previous(t *testing.T) {
	h := NewTensionHistory(10)
	for i := 0; i < 3; i++ {
		h.Push(TensionSample{
			Tension:   float64(i + 1),
			Timestamp: time.Now(),
			Version:   uint64(i),
		})
	}
	prev, ok := h.Previous()
	if !ok {
		t.Fatal("expected Previous() to return true")
	}
	if prev.Tension != 2.0 {
		t.Fatalf("expected Previous().Tension == 2.0, got %f", prev.Tension)
	}
}

func TestTensionHistory_Empty(t *testing.T) {
	h := NewTensionHistory(10)
	_, ok := h.Latest()
	if ok {
		t.Fatal("expected Latest() to return false on empty buffer")
	}
	_, ok = h.Previous()
	if ok {
		t.Fatal("expected Previous() to return false on empty buffer")
	}
}

func TestTemporalIndicators_TensionSpike(t *testing.T) {
	tc := NewTemporalCalculator(1.0)
	current := map[string]float64{"A": 0.5, "B": 0.8}
	previous := map[string]float64{"A": 0.1, "B": 0.7}
	dt := 1 * time.Second

	ind := tc.Indicators(current, previous, dt)
	// TensionSpike = max(|0.5-0.1|, |0.8-0.7|) = max(0.4, 0.1) = 0.4
	if math.Abs(ind.TensionSpike-0.4) > 1e-10 {
		t.Fatalf("expected TensionSpike == 0.4, got %f", ind.TensionSpike)
	}
}

func TestTemporalIndicators_DecayExponent_Positive(t *testing.T) {
	tc := NewTemporalCalculator(1.0)
	// Previous mean > current mean => log(current) < log(previous)
	// decay = -(log(current_mean) - log(previous_mean)) / dt > 0
	current := map[string]float64{"A": 1.0}
	previous := map[string]float64{"A": 2.0}
	dt := 1 * time.Second

	ind := tc.Indicators(current, previous, dt)
	if ind.DecayExponent <= 0 {
		t.Fatalf("expected positive DecayExponent (recovery), got %f", ind.DecayExponent)
	}
}

func TestTemporalIndicators_DecayExponent_Negative(t *testing.T) {
	tc := NewTemporalCalculator(1.0)
	// Current mean > previous mean => log(current) > log(previous)
	// decay = -(log(current_mean) - log(previous_mean)) / dt < 0
	current := map[string]float64{"A": 4.0}
	previous := map[string]float64{"A": 1.0}
	dt := 1 * time.Second

	ind := tc.Indicators(current, previous, dt)
	if ind.DecayExponent >= 0 {
		t.Fatalf("expected negative DecayExponent (growth), got %f", ind.DecayExponent)
	}
}

func TestTemporalIndicators_CurvatureShock(t *testing.T) {
	tc := NewTemporalCalculator(1.0)
	current := map[string]float64{"A": 1.0, "B": 2.0}
	previous := map[string]float64{"A": 1.0, "B": 2.0}

	curCurv := map[[2]string]float64{
		{"A", "B"}: 3.0,
		{"B", "C"}: 1.0,
	}
	prevCurv := map[[2]string]float64{
		{"A", "B"}: 1.0,
		{"B", "C"}: 0.5,
	}

	dt := 1 * time.Second
	ind := tc.IndicatorsWithCurvature(current, previous, curCurv, prevCurv, dt)

	// CurvatureShock = max(|3.0-1.0|, |1.0-0.5|) = max(2.0, 0.5) = 2.0
	if math.Abs(ind.CurvatureShock-2.0) > 1e-10 {
		t.Fatalf("expected CurvatureShock == 2.0, got %f", ind.CurvatureShock)
	}
}

func TestClassifyPhase_FullRecovery(t *testing.T) {
	// Low rho (tension decaying, small spike), high pi (good connectivity).
	// DecayExponent > 0 => rho starts at 0 (no negative decay contribution).
	// TensionSpike near 0 => rho stays below rhoC=1.0.
	// connectivityRatio=0.9 => pi=0.9 >= piC=0.5.
	ind := TemporalIndicators{
		TensionSpike:  0.1,
		DecayExponent: 2.0, // positive = decaying
	}
	result := ClassifyPhase(ind, 0.1, 0.5, 0.9)
	if result.Phase != PhaseFullRecovery {
		t.Fatalf("expected PhaseFullRecovery, got %v (rho=%f, pi=%f)", result.Phase, result.Rho, result.Pi)
	}
}

func TestClassifyPhase_ScarredRecovery(t *testing.T) {
	// Low rho, low pi.
	// DecayExponent > 0 => rho starts at 0. Small spike => rho < 1.0.
	// connectivityRatio = 0.2 => pi = 0.2 < piC=0.5.
	ind := TemporalIndicators{
		TensionSpike:  0.1,
		DecayExponent: 2.0,
	}
	result := ClassifyPhase(ind, 0.1, 0.5, 0.2)
	if result.Phase != PhaseScarredRecovery {
		t.Fatalf("expected PhaseScarredRecovery, got %v (rho=%f, pi=%f)", result.Phase, result.Rho, result.Pi)
	}
}

func TestClassifyPhase_ChronicTension(t *testing.T) {
	// High rho, high pi.
	// DecayExponent < 0 => rho = |DecayExponent| + TensionSpike.
	// Need rho >= 1.0. E.g. decay=-0.5, spike=0.8 => rho=1.3.
	// connectivityRatio=0.8 => pi=0.8 >= piC=0.5.
	ind := TemporalIndicators{
		TensionSpike:  0.8,
		DecayExponent: -0.5,
	}
	result := ClassifyPhase(ind, 1.0, 0.5, 0.8)
	if result.Phase != PhaseChronicTension {
		t.Fatalf("expected PhaseChronicTension, got %v (rho=%f, pi=%f)", result.Phase, result.Rho, result.Pi)
	}
}

func TestClassifyPhase_StructuralCollapse(t *testing.T) {
	// High rho, low pi.
	// DecayExponent < 0 => rho = |DecayExponent| + TensionSpike.
	// Need rho >= 1.0 and pi < 0.5.
	ind := TemporalIndicators{
		TensionSpike:  0.8,
		DecayExponent: -0.5,
	}
	result := ClassifyPhase(ind, 1.0, 0.5, 0.2)
	if result.Phase != PhaseStructuralCollapse {
		t.Fatalf("expected PhaseStructuralCollapse, got %v (rho=%f, pi=%f)", result.Phase, result.Rho, result.Pi)
	}
}

func TestVelocityOfSilence(t *testing.T) {
	// v = alpha * sqrt(lambda1) * meanEdgeLength
	// v = 0.1 * sqrt(4.0) * 2.0 = 0.1 * 2.0 * 2.0 = 0.4
	v := VelocityOfSilence(0.1, 4.0, 2.0)
	if math.Abs(v-0.4) > 1e-10 {
		t.Fatalf("expected VelocityOfSilence == 0.4, got %f", v)
	}
}

func TestVelocityOfSilence_NonPositiveInputs(t *testing.T) {
	// Any non-positive input should return 0.
	if v := VelocityOfSilence(0, 4.0, 2.0); v != 0 {
		t.Fatalf("expected 0 for alpha=0, got %f", v)
	}
	if v := VelocityOfSilence(0.1, 0, 2.0); v != 0 {
		t.Fatalf("expected 0 for lambda1=0, got %f", v)
	}
	if v := VelocityOfSilence(0.1, 4.0, 0); v != 0 {
		t.Fatalf("expected 0 for meanEdgeLength=0, got %f", v)
	}
	if v := VelocityOfSilence(-1, 4.0, 2.0); v != 0 {
		t.Fatalf("expected 0 for negative alpha, got %f", v)
	}
}

func TestEstimateAge(t *testing.T) {
	// t_supp = distance / velocity = 10.0 / 5.0 = 2.0 seconds
	age := EstimateAge(10.0, 5.0)
	expected := 2 * time.Second
	if age != expected {
		t.Fatalf("expected EstimateAge == %v, got %v", expected, age)
	}
}

func TestEstimateAge_ZeroVelocity(t *testing.T) {
	age := EstimateAge(10.0, 0)
	if age != 0 {
		t.Fatalf("expected 0 for zero velocity, got %v", age)
	}
}

func TestEstimateAge_NegativeDistance(t *testing.T) {
	age := EstimateAge(-5.0, 5.0)
	if age != 0 {
		t.Fatalf("expected 0 for negative distance, got %v", age)
	}
}

func TestPhase_String(t *testing.T) {
	tests := []struct {
		phase    Phase
		expected string
	}{
		{PhaseFullRecovery, "FullRecovery"},
		{PhaseScarredRecovery, "ScarredRecovery"},
		{PhaseChronicTension, "ChronicTension"},
		{PhaseStructuralCollapse, "StructuralCollapse"},
	}
	for _, tt := range tests {
		got := tt.phase.String()
		if got != tt.expected {
			t.Fatalf("Phase(%d).String() = %q, want %q", int(tt.phase), got, tt.expected)
		}
	}

	// Unknown phase should produce a fallback.
	unknown := Phase(99)
	s := unknown.String()
	if s == "" || s == "FullRecovery" || s == "ScarredRecovery" || s == "ChronicTension" || s == "StructuralCollapse" {
		t.Fatalf("expected fallback string for unknown phase, got %q", s)
	}
}

func TestTensionHistory_SingleElement(t *testing.T) {
	h := NewTensionHistory(10)
	h.Push(TensionSample{Tension: 42.0, Timestamp: time.Now(), Version: 1})

	if h.Len() != 1 {
		t.Fatalf("expected Len()==1, got %d", h.Len())
	}

	latest, ok := h.Latest()
	if !ok || latest.Tension != 42.0 {
		t.Fatalf("expected Latest().Tension==42.0, got ok=%v, tension=%f", ok, latest.Tension)
	}

	_, ok = h.Previous()
	if ok {
		t.Fatal("expected Previous() to return false with only 1 element")
	}
}

func TestTensionHistory_DefaultCapacity(t *testing.T) {
	// Passing 0 or negative capacity should default to 100.
	h := NewTensionHistory(0)
	for i := 0; i < 100; i++ {
		h.Push(TensionSample{Tension: float64(i)})
	}
	if h.Len() != 100 {
		t.Fatalf("expected Len()==100 for default capacity, got %d", h.Len())
	}
}

func TestTemporalIndicators_NoCurvature(t *testing.T) {
	tc := NewTemporalCalculator(1.0)
	current := map[string]float64{"A": 1.0}
	previous := map[string]float64{"A": 2.0}

	ind := tc.Indicators(current, previous, time.Second)
	// When no curvature provided, CurvatureShock should be 0.
	if ind.CurvatureShock != 0 {
		t.Fatalf("expected CurvatureShock == 0 without curvature data, got %f", ind.CurvatureShock)
	}
}

func TestTemporalIndicators_ZeroDt(t *testing.T) {
	tc := NewTemporalCalculator(1.0)
	current := map[string]float64{"A": 1.0}
	previous := map[string]float64{"A": 2.0}

	ind := tc.Indicators(current, previous, 0)
	// With dt=0, DecayExponent should be 0 (division by zero protected).
	if ind.DecayExponent != 0 {
		t.Fatalf("expected DecayExponent == 0 with dt=0, got %f", ind.DecayExponent)
	}
}
