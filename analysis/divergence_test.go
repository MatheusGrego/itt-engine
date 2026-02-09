package analysis

import (
	"math"
	"testing"
)

func TestJSD_IdenticalDistributions(t *testing.T) {
	p := []float64{0.25, 0.25, 0.25, 0.25}
	d := JSD{}
	result := d.Compute(p, p)
	if result > 1e-10 {
		t.Fatalf("JSD(p,p) should be ~0, got %f", result)
	}
}

func TestJSD_Symmetric(t *testing.T) {
	p := []float64{0.1, 0.4, 0.5}
	q := []float64{0.3, 0.3, 0.4}
	d := JSD{}
	pq := d.Compute(p, q)
	qp := d.Compute(q, p)
	if math.Abs(pq-qp) > 1e-10 {
		t.Fatalf("JSD should be symmetric: %f != %f", pq, qp)
	}
}

func TestJSD_Bounded(t *testing.T) {
	p := []float64{1.0, 0.0, 0.0}
	q := []float64{0.0, 0.0, 1.0}
	d := JSD{}
	result := d.Compute(p, q)
	if result < 0 || result > 1.0+1e-10 {
		t.Fatalf("JSD should be in [0, 1]: got %f", result)
	}
}

func TestJSD_NonNegative(t *testing.T) {
	p := []float64{0.7, 0.2, 0.1}
	q := []float64{0.1, 0.1, 0.8}
	d := JSD{}
	result := d.Compute(p, q)
	if result < -1e-10 {
		t.Fatalf("JSD should be >= 0, got %f", result)
	}
}

func TestKL_IdenticalDistributions(t *testing.T) {
	p := []float64{0.25, 0.25, 0.25, 0.25}
	d := KL{}
	result := d.Compute(p, p)
	if result > 1e-10 {
		t.Fatalf("KL(p,p) should be ~0, got %f", result)
	}
}

func TestKL_NonNegative(t *testing.T) {
	p := []float64{0.1, 0.4, 0.5}
	q := []float64{0.3, 0.3, 0.4}
	d := KL{}
	result := d.Compute(p, q)
	if result < -1e-10 {
		t.Fatalf("KL should be >= 0, got %f", result)
	}
}

func TestKL_Asymmetric(t *testing.T) {
	p := []float64{0.1, 0.4, 0.5}
	q := []float64{0.3, 0.3, 0.4}
	d := KL{}
	pq := d.Compute(p, q)
	qp := d.Compute(q, p)
	if math.Abs(pq-qp) < 1e-10 {
		t.Fatal("KL should be asymmetric for different distributions")
	}
}

func TestHellinger_IdenticalDistributions(t *testing.T) {
	p := []float64{0.25, 0.25, 0.25, 0.25}
	d := Hellinger{}
	result := d.Compute(p, p)
	if result > 1e-10 {
		t.Fatalf("Hellinger(p,p) should be ~0, got %f", result)
	}
}

func TestHellinger_Bounded(t *testing.T) {
	p := []float64{1.0, 0.0}
	q := []float64{0.0, 1.0}
	d := Hellinger{}
	result := d.Compute(p, q)
	if result < 0 || result > 1.0+1e-10 {
		t.Fatalf("Hellinger should be in [0, 1]: got %f", result)
	}
}

func TestHellinger_Symmetric(t *testing.T) {
	p := []float64{0.1, 0.4, 0.5}
	q := []float64{0.3, 0.3, 0.4}
	d := Hellinger{}
	pq := d.Compute(p, q)
	qp := d.Compute(q, p)
	if math.Abs(pq-qp) > 1e-10 {
		t.Fatalf("Hellinger should be symmetric: %f != %f", pq, qp)
	}
}

func TestDivergence_WithZeros(t *testing.T) {
	p := []float64{0.5, 0.5, 0.0}
	q := []float64{0.0, 0.5, 0.5}
	for _, d := range []DivergenceFunc{JSD{}, KL{}, Hellinger{}} {
		result := d.Compute(p, q)
		if math.IsNaN(result) || math.IsInf(result, 0) {
			t.Fatalf("%s produced NaN/Inf with zeros", d.Name())
		}
	}
}

func TestNormalize(t *testing.T) {
	dist := []float64{2.0, 3.0, 5.0}
	n := Normalize(dist)
	if math.Abs(n[0]-0.2) > 1e-10 || math.Abs(n[1]-0.3) > 1e-10 || math.Abs(n[2]-0.5) > 1e-10 {
		t.Fatalf("expected [0.2, 0.3, 0.5], got %v", n)
	}
}

func TestNormalize_AllZeros(t *testing.T) {
	dist := []float64{0.0, 0.0, 0.0}
	n := Normalize(dist)
	expected := 1.0 / 3.0
	for i, v := range n {
		if math.Abs(v-expected) > 1e-10 {
			t.Fatalf("index %d: expected %f, got %f", i, expected, v)
		}
	}
}
