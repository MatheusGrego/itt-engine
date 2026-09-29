package harness

import (
	"math"
	"sort"
)

// TwoPoissonZ computes, per node, a Stouffer Z over its edges of the signed
// root of the two-sample Poisson likelihood ratio (before vs after), which
// conditions on the edge total and so needs no estimated baseline:
//
//	n = a + o, q = (T-T0)/T, ô = n q, â = n (1-q)
//	G = 2 [a ln(a/â) + o ln(o/ô)]   (~ χ²₁ under H0)
//	r = sign(o - ô) sqrt(G)         (~ N(0,1) under H0)
//	Z(v) = Σ r / sqrt(k_v)
//
// Negative Z = silence, positive Z = burst. Edges new after T0 are included.
func TwoPoissonZ(d Dataset) map[string]float64 {
	before := sumCounts(d, 0, d.T0)
	after := sumCounts(d, d.T0, d.T)
	keys := map[[2]string]struct{}{}
	for k := range before {
		keys[k] = struct{}{}
	}
	for k := range after {
		keys[k] = struct{}{}
	}
	ks := make([][2]string, 0, len(keys))
	for k := range keys {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		if ks[i][0] != ks[j][0] {
			return ks[i][0] < ks[j][0]
		}
		return ks[i][1] < ks[j][1]
	})
	q := float64(d.T-d.T0) / float64(d.T)
	sum := map[string]float64{}
	cnt := map[string]float64{}
	for _, id := range d.Nodes {
		sum[id], cnt[id] = 0, 0
	}
	xlx := func(x, m float64) float64 {
		if x == 0 {
			return 0
		}
		return x * math.Log(x/m)
	}
	for _, k := range ks {
		a, o := float64(before[k]), float64(after[k])
		n := a + o
		oh, ah := n*q, n*(1-q)
		g := 2 * (xlx(a, ah) + xlx(o, oh))
		if g < 0 {
			g = 0
		}
		r := math.Sqrt(g)
		if o < oh {
			r = -r
		}
		for _, v := range k {
			sum[v] += r
			cnt[v]++
		}
	}
	z := make(map[string]float64, len(sum))
	for v, s := range sum {
		if cnt[v] > 0 {
			z[v] = s / math.Sqrt(cnt[v])
		} else {
			z[v] = 0
		}
	}
	return z
}

func zAlarm(score float64, n int, alpha float64) bool {
	// one-sided Bonferroni: P(Z > z) = α/N
	return score > math.Sqrt2*math.Erfinv(1-2*alpha/float64(n))
}

type SilenceZ struct{}

func (SilenceZ) Name() string { return "silence-z-2pois" }
func (SilenceZ) Scores(d Dataset) map[string]float64 {
	z := TwoPoissonZ(d)
	for k, v := range z {
		z[k] = -v
	}
	return z
}
func (SilenceZ) Alarm(s float64, n int, a float64) bool { return zAlarm(s, n, a) }
func (SilenceZ) AlarmRule() string                     { return "−Z > z(1−α/N)" }

type BurstZ struct{}

func (BurstZ) Name() string                           { return "burst-z-2pois" }
func (BurstZ) Scores(d Dataset) map[string]float64    { return TwoPoissonZ(d) }
func (BurstZ) Alarm(s float64, n int, a float64) bool { return zAlarm(s, n, a) }
func (BurstZ) AlarmRule() string                      { return "Z > z(1−α/N)" }
