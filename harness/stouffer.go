package harness

import (
	"math"
	"sort"
)

// SignedRoot returns the signed root of the likelihood-ratio statistic that
// compares two Poisson counts of one edge: a events before a split and o
// events after it. Under H0 (the same rate on both sides) a fraction q of the
// edge total n = a + o is expected after the split. Conditioning on n removes
// the unknown rate, so no baseline is estimated:
//
//	ô = n q,  â = n (1 − q)
//	G = 2 [a ln(a/â) + o ln(o/ô)]   (~ χ²₁ under H0)
//	r = sign(o − ô) · sqrt(G)       (~ N(0, 1) under H0)
//
// 0·ln 0 is taken as 0, so an edge that only shows up after the split
// (a = 0) gives a finite r > 0 and an edge that goes quiet (o = 0) gives
// r < 0. An edge with no events (n = 0) gives 0.
func SignedRoot(a, o, q float64) float64 {
	n := a + o
	if n == 0 {
		return 0
	}
	oh, ah := n*q, n*(1-q)
	g := 2 * (xlogx(a, ah) + xlogx(o, oh))
	if g < 0 { // rounding when a ≈ â
		g = 0
	}
	r := math.Sqrt(g)
	if o < oh {
		return -r
	}
	return r
}

// xlogx returns x ln(x/m), with the x = 0 limit 0.
func xlogx(x, m float64) float64 {
	if x == 0 {
		return 0
	}
	return x * math.Log(x/m)
}

// TwoPoissonZ returns, for every node, the Stouffer combination of the
// per-edge SignedRoot over the edges incident to it, before [0, T0) against
// after [T0, T), with q = (T − T0)/T:
//
//	Z(v) = Σ r / sqrt(k_v)
//
// k_v counts the edges of v with at least one event in [0, T); an edge
// never observed is not known to exist. Dividing by sqrt(k_v) keeps Z at
// N(0, 1) under H0 whatever the degree, which the deviance sum of M3 does
// not. Negative Z is silence (M5), positive Z is burst (M6). A node with no
// observed edge gets Z = 0.
func TwoPoissonZ(d Dataset) map[string]float64 {
	before := sumCounts(d, 0, d.T0)
	after := sumCounts(d, d.T0, d.T)
	q := float64(d.T-d.T0) / float64(d.T)

	sum := make(map[string]float64, len(d.Nodes))
	cnt := make(map[string]int, len(d.Nodes))
	for _, key := range edgeKeys(before, after) {
		r := SignedRoot(float64(before[key]), float64(after[key]), q)
		for _, v := range key {
			sum[v] += r
			cnt[v]++
		}
	}
	z := make(map[string]float64, len(d.Nodes))
	for _, id := range d.Nodes {
		if k := cnt[id]; k > 0 {
			z[id] = sum[id] / math.Sqrt(float64(k))
		} else {
			z[id] = 0
		}
	}
	return z
}

// edgeKeys returns the union of the keys of the given sums in a fixed order,
// so float sums over edges are reproducible.
func edgeKeys(sums ...map[[2]string]int) [][2]string {
	seen := map[[2]string]struct{}{}
	for _, s := range sums {
		for k := range s {
			seen[k] = struct{}{}
		}
	}
	keys := make([][2]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	return keys
}

// zCritical returns the one-sided Bonferroni critical value z(1 − α/n),
// that is, P(N(0, 1) > z) = α/n.
func zCritical(n int, alpha float64) float64 {
	return math.Sqrt2 * math.Erfinv(1-2*alpha/float64(n))
}

// SilenceZ is M5: the score is −Z(v) from TwoPoissonZ, so a node whose
// edges went quiet after T0 ranks high. It alarms when −Z > z(1 − α/N),
// one-sided with Bonferroni over the N nodes.
type SilenceZ struct{}

func (SilenceZ) Name() string { return "silence-z-2pois" }

func (SilenceZ) Scores(d Dataset) map[string]float64 {
	z := TwoPoissonZ(d)
	for k, v := range z {
		z[k] = -v
	}
	return z
}

func (SilenceZ) Alarm(score float64, n int, alpha float64) bool {
	return score > zCritical(n, alpha)
}

func (SilenceZ) AlarmRule() string { return "−Z > z(1−α/N)" }

// BurstZ is M6: the score is +Z(v) from TwoPoissonZ, with the same
// one-sided Bonferroni rule as M5.
type BurstZ struct{}

func (BurstZ) Name() string { return "burst-z-2pois" }

func (BurstZ) Scores(d Dataset) map[string]float64 { return TwoPoissonZ(d) }

func (BurstZ) Alarm(score float64, n int, alpha float64) bool {
	return score > zCritical(n, alpha)
}

func (BurstZ) AlarmRule() string { return "Z > z(1−α/N)" }
