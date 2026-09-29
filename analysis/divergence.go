package analysis

import "math"

const epsilon = 1e-12

// DivergenceFunc computes divergence between two probability distributions.
type DivergenceFunc interface {
	Compute(p, q []float64) float64
	Name() string
}

// BoundedDivergence indicates whether a divergence function produces bounded values.
type BoundedDivergence interface {
	IsBounded() bool
}

// JSD implements Jensen-Shannon Divergence. Symmetric, bounded in [0, 1] (using log base 2).
type JSD struct{}

func (JSD) Name() string      { return "jsd" }
func (JSD) IsBounded() bool   { return true }
func (JSD) MaxValue() float64 { return 1 }

func (JSD) Compute(p, q []float64) float64 {
	m := make([]float64, len(p))
	for i := range p {
		m[i] = 0.5*p[i] + 0.5*q[i]
	}
	return 0.5*klDiv(p, m) + 0.5*klDiv(q, m)
}

// KL implements Kullback-Leibler Divergence. Asymmetric, unbounded.
type KL struct{}

func (KL) Name() string    { return "kl" }
func (KL) IsBounded() bool { return false }

// MaxValue returns the ceiling used for KL, which is unbounded. With the
// epsilon smoothing in klDiv, a point mass against a disjoint one yields
// ~log2(1/epsilon), so that value is used as the cap.
func (KL) MaxValue() float64 { return math.Log2(1 / epsilon) }

func (KL) Compute(p, q []float64) float64 {
	return klDiv(p, q)
}

// Hellinger implements Hellinger Distance. Symmetric, bounded in [0, 1].
type Hellinger struct{}

func (Hellinger) Name() string      { return "hellinger" }
func (Hellinger) IsBounded() bool   { return true }
func (Hellinger) MaxValue() float64 { return 1 }

func (Hellinger) Compute(p, q []float64) float64 {
	sum := 0.0
	for i := range p {
		diff := math.Sqrt(p[i]) - math.Sqrt(q[i])
		sum += diff * diff
	}
	return math.Sqrt(sum / 2.0)
}

// maxDivergence returns the maximum value of d, used when one distribution
// has no mass left (see TensionCalculator.Calculate). Divergences that do not
// implement MaxValue are evaluated on two point masses with disjoint support,
// which is the supremum of any f-divergence.
func maxDivergence(d DivergenceFunc) float64 {
	if m, ok := d.(interface{ MaxValue() float64 }); ok {
		return m.MaxValue()
	}
	return d.Compute([]float64{1, 0}, []float64{0, 1})
}

func klDiv(p, q []float64) float64 {
	sum := 0.0
	for i := range p {
		pi := p[i] + epsilon
		qi := q[i] + epsilon
		if pi > epsilon {
			sum += pi * math.Log2(pi/qi)
		}
	}
	return sum
}

// Normalize ensures a slice sums to 1.0. Returns a new slice.
func Normalize(dist []float64) []float64 {
	total := 0.0
	for _, v := range dist {
		total += v
	}
	if total == 0 {
		// Uniform distribution
		n := float64(len(dist))
		result := make([]float64, len(dist))
		for i := range result {
			result[i] = 1.0 / n
		}
		return result
	}
	result := make([]float64, len(dist))
	for i, v := range dist {
		result[i] = v / total
	}
	return result
}
