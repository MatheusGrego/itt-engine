package analysis

import (
	"fmt"
	"math"
	"time"
)

// TensionSample is a single tension measurement at a point in time.
type TensionSample struct {
	Tension   float64
	Timestamp time.Time
	Version   uint64
}

// TensionHistory tracks tension evolution for a single node.
// Implemented as a fixed-size ring buffer.
type TensionHistory struct {
	samples []TensionSample
	size    int
	head    int // next write position
	count   int // total items written (may exceed size)
}

// NewTensionHistory creates a TensionHistory with the given capacity.
func NewTensionHistory(capacity int) *TensionHistory {
	if capacity <= 0 {
		capacity = 100
	}
	return &TensionHistory{
		samples: make([]TensionSample, capacity),
		size:    capacity,
	}
}

// Push adds a new sample, overwriting the oldest if full.
func (h *TensionHistory) Push(s TensionSample) {
	h.samples[h.head] = s
	h.head = (h.head + 1) % h.size
	h.count++
}

// Len returns the number of samples stored (capped at capacity).
func (h *TensionHistory) Len() int {
	if h.count < h.size {
		return h.count
	}
	return h.size
}

// Latest returns the most recent sample, or false if empty.
func (h *TensionHistory) Latest() (TensionSample, bool) {
	if h.count == 0 {
		return TensionSample{}, false
	}
	idx := (h.head - 1 + h.size) % h.size
	return h.samples[idx], true
}

// Previous returns the sample before the latest, or false if fewer than 2 samples.
func (h *TensionHistory) Previous() (TensionSample, bool) {
	if h.Len() < 2 {
		return TensionSample{}, false
	}
	idx := (h.head - 2 + h.size) % h.size
	return h.samples[idx], true
}

// Slice returns all stored samples in chronological order (oldest first).
func (h *TensionHistory) Slice() []TensionSample {
	n := h.Len()
	if n == 0 {
		return nil
	}
	result := make([]TensionSample, n)
	if h.count <= h.size {
		// Buffer hasn't wrapped yet
		copy(result, h.samples[:n])
	} else {
		// Buffer has wrapped: oldest is at head, newest is at head-1
		start := h.head // oldest position
		for i := 0; i < n; i++ {
			result[i] = h.samples[(start+i)%h.size]
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// T15 — TemporalCalculator + TemporalIndicators
// ---------------------------------------------------------------------------

// TemporalIndicators holds the temporal anomaly signals from the theory.
type TemporalIndicators struct {
	// TensionSpike: Δτ_max(t) = max_v |τ(v,t) - τ(v,t-1)|
	TensionSpike float64

	// DecayExponent: γ(t) = -d/dt log(τ̄(t))
	// Positive = tension decaying (recovery). Negative = tension growing.
	DecayExponent float64

	// CurvatureShock: Δκ_min(t) = max over edges |κ(t) - κ(t-1)|
	// Only computed when curvature data is provided.
	CurvatureShock float64

	// Timestamp of the measurement.
	Timestamp time.Time
}

// TemporalCalculator computes temporal dynamics from tension snapshots.
type TemporalCalculator struct {
	alpha float64 // diffusivity constant
}

// NewTemporalCalculator creates a TemporalCalculator with the given diffusivity alpha.
func NewTemporalCalculator(alpha float64) *TemporalCalculator {
	return &TemporalCalculator{alpha: alpha}
}

// Indicators computes temporal anomaly indicators from current and previous
// tension snapshots. Both maps are nodeID → tension value.
// dt is the time elapsed between the two snapshots.
func (tc *TemporalCalculator) Indicators(
	current map[string]float64,
	previous map[string]float64,
	dt time.Duration,
) TemporalIndicators {
	return tc.IndicatorsWithCurvature(current, previous, nil, nil, dt)
}

// IndicatorsWithCurvature computes temporal indicators including curvature shock.
// currentCurv and prevCurv map [2]string{from,to} → curvature value.
func (tc *TemporalCalculator) IndicatorsWithCurvature(
	current map[string]float64,
	previous map[string]float64,
	currentCurv map[[2]string]float64,
	prevCurv map[[2]string]float64,
	dt time.Duration,
) TemporalIndicators {
	ind := TemporalIndicators{
		Timestamp: time.Now(),
	}

	// 1. Tension Spike: max |current[v] - previous[v]| over all nodes
	allNodes := make(map[string]bool)
	for k := range current {
		allNodes[k] = true
	}
	for k := range previous {
		allNodes[k] = true
	}

	maxDelta := 0.0
	for node := range allNodes {
		c := current[node]  // 0 if missing
		p := previous[node] // 0 if missing
		delta := math.Abs(c - p)
		if delta > maxDelta {
			maxDelta = delta
		}
	}
	ind.TensionSpike = maxDelta

	// 2. Decay Exponent: γ(t) = -(log(meanCurrent) - log(meanPrevious)) / dt
	dtSec := dt.Seconds()
	if dtSec > 0 {
		meanCur := mapMean(current)
		meanPrev := mapMean(previous)
		if meanCur > 0 && meanPrev > 0 {
			ind.DecayExponent = -(math.Log(meanCur) - math.Log(meanPrev)) / dtSec
		}
	}

	// 3. Curvature Shock: max |currentCurv[e] - prevCurv[e]| over all edges
	if currentCurv != nil && prevCurv != nil {
		allEdges := make(map[[2]string]bool)
		for k := range currentCurv {
			allEdges[k] = true
		}
		for k := range prevCurv {
			allEdges[k] = true
		}
		maxCurvDelta := 0.0
		for edge := range allEdges {
			c := currentCurv[edge]
			p := prevCurv[edge]
			delta := math.Abs(c - p)
			if delta > maxCurvDelta {
				maxCurvDelta = delta
			}
		}
		ind.CurvatureShock = maxCurvDelta
	}

	return ind
}

func mapMean(m map[string]float64) float64 {
	if len(m) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range m {
		sum += v
	}
	return sum / float64(len(m))
}

// ---------------------------------------------------------------------------
// T16 — ClassifyPhase (4 suppression phases)
// ---------------------------------------------------------------------------

// Phase represents the suppression behavior phase from the theory.
type Phase int

const (
	// PhaseFullRecovery: tension dissipates, structure heals completely.
	PhaseFullRecovery Phase = iota
	// PhaseScarredRecovery: tension dissipates, but structure retains damage.
	PhaseScarredRecovery
	// PhaseChronicTension: sustained tension, structure continuously adapts.
	PhaseChronicTension
	// PhaseStructuralCollapse: runaway tension accumulation, eventual failure.
	PhaseStructuralCollapse
)

// String returns a human-readable name for the phase.
func (p Phase) String() string {
	switch p {
	case PhaseFullRecovery:
		return "FullRecovery"
	case PhaseScarredRecovery:
		return "ScarredRecovery"
	case PhaseChronicTension:
		return "ChronicTension"
	case PhaseStructuralCollapse:
		return "StructuralCollapse"
	default:
		return fmt.Sprintf("Phase(%d)", int(p))
	}
}

// PhaseResult holds the phase classification for a region.
type PhaseResult struct {
	Phase Phase
	Rho   float64 // ρ = suppression intensity
	Pi    float64 // π = healing capacity
}

// ClassifyPhase determines the current suppression phase based on
// temporal indicators and graph structure.
//
// Parameters:
//   - indicators: temporal indicators from consecutive snapshots
//   - meanTension: current mean tension across the region
//   - prevMeanTension: previous mean tension
//   - connectivityRatio: fraction of edges surviving between snapshots (1.0 = no loss)
//
// Phase boundaries: ρ_c = 1.0, π_c = 0.5
func ClassifyPhase(
	indicators TemporalIndicators,
	meanTension float64,
	prevMeanTension float64,
	connectivityRatio float64,
) PhaseResult {
	// Compute ρ (suppression intensity)
	// DecayExponent > 0 means tension decaying (low intensity)
	// DecayExponent < 0 means tension growing (high intensity)
	// Near 0 with high tension = chronic
	rho := 0.0
	if indicators.DecayExponent < 0 {
		rho = math.Abs(indicators.DecayExponent)
	} else if indicators.DecayExponent < 0.01 && meanTension > 0 {
		// Near-zero decay with non-zero tension = chronic state
		rho = meanTension
	}
	// Also consider tension spike as indicator of active suppression
	rho += indicators.TensionSpike

	// Normalize: cap at 2.0 for classification
	rhoC := 1.0 // critical threshold
	if rho > 2.0 {
		rho = 2.0
	}

	// Compute π (healing capacity)
	// Based on connectivity ratio: high = structure maintained
	pi := connectivityRatio
	if pi > 1.0 {
		pi = 1.0
	}
	if pi < 0 {
		pi = 0
	}
	piC := 0.5 // critical threshold

	// Classify
	var phase Phase
	if rho < rhoC {
		if pi >= piC {
			phase = PhaseFullRecovery
		} else {
			phase = PhaseScarredRecovery
		}
	} else {
		if pi >= piC {
			phase = PhaseChronicTension
		} else {
			phase = PhaseStructuralCollapse
		}
	}

	return PhaseResult{
		Phase: phase,
		Rho:   rho,
		Pi:    pi,
	}
}

// ---------------------------------------------------------------------------
// T18 — VelocityOfSilence + EstimateAge
// ---------------------------------------------------------------------------

// VelocityOfSilence computes how fast anomaly information propagates
// through the network.
//
// Formula: v_silence = alpha * sqrt(lambda1) * meanEdgeLength
//
// Returns 0 if any input is non-positive.
func VelocityOfSilence(alpha, lambda1, meanEdgeLength float64) float64 {
	if alpha <= 0 || lambda1 <= 0 || meanEdgeLength <= 0 {
		return 0
	}
	return alpha * math.Sqrt(lambda1) * meanEdgeLength
}

// EstimateAge estimates how long ago an anomaly started based on
// the distance (in hops) from the epicenter and propagation velocity.
//
// Formula: t_supp ≈ distance / v_silence
//
// Returns 0 if velocity is non-positive.
func EstimateAge(distance float64, velocity float64) time.Duration {
	if velocity <= 0 || distance <= 0 {
		return 0
	}
	seconds := distance / velocity
	return time.Duration(seconds * float64(time.Second))
}
