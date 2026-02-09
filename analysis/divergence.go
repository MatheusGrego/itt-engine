package analysis

import "math"

const epsilon = 1e-12

// DivergenceFunc computes divergence between two probability distributions.
type DivergenceFunc interface {
	Compute(p, q []float64) float64
	Name() string
}

// JSD implements Jensen-Shannon Divergence. Symmetric, bounded in [0, 1] (using log base 2).
type JSD struct{}

func (JSD) Name() string { return "jsd" }

func (JSD) Compute(p, q []float64) float64 {
	m := make([]float64, len(p))
	for i := range p {
		m[i] = 0.5*p[i] + 0.5*q[i]
	}
	return 0.5*klDiv(p, m) + 0.5*klDiv(q, m)
}

// KL implements Kullback-Leibler Divergence. Asymmetric, unbounded.
type KL struct{}

func (KL) Name() string { return "kl" }

func (KL) Compute(p, q []float64) float64 {
	return klDiv(p, q)
}

// Hellinger implements Hellinger Distance. Symmetric, bounded in [0, 1].
type Hellinger struct{}

func (Hellinger) Name() string { return "hellinger" }

func (Hellinger) Compute(p, q []float64) float64 {
	sum := 0.0
	for i := range p {
		diff := math.Sqrt(p[i]) - math.Sqrt(q[i])
		sum += diff * diff
	}
	return math.Sqrt(sum / 2.0)
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
