package harness

import (
	"fmt"
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

// EdgeStat selects the per-edge statistic r(a, o; q) of the Z family.
type EdgeStat int

const (
	// StatSignedRoot is sign(o − ô)·sqrt(G), the prototype (SignedRoot).
	StatSignedRoot EdgeStat = iota
	// StatMidP is Φ⁻¹ of the exact binomial mid-p (MidPZ).
	StatMidP
)

// ZConfig configures the per-node Z of TwoPoissonZ and EdgeZ. The zero value
// is the prototype: signed root, every observed edge.
type ZConfig struct {
	Stat EdgeStat
	// MinN leaves out, from the sum and from k_v, the edges whose total
	// n = a + o is below MinN (0 or 1 keeps every observed edge).
	MinN int
	// GlobalQ replaces the fixed q = (T − split)/T by the graph-wide
	// q̂ = total after / total (Tarefa 4), which absorbs a global trend.
	GlobalQ bool
	// Phi divides every r by sqrt(φ̂), an overdispersion factor estimated
	// from the before windows only (Tarefa 4).
	Phi PhiMode
}

// PhiMode selects the overdispersion correction of ZConfig.
type PhiMode int

const (
	PhiNone   PhiMode = iota
	PhiMedian         // one φ̂ for the graph: median Pearson dispersion
	PhiBands          // one φ̂ per band of before-rate (log2 bins)
)

// DefaultZConfig is the M5/M6 default chosen in Tarefa 2 of fase 2b: the
// exact mid-p. The signed root (zero value) and the n_min filter stay as
// options; see docs/refinement/results/fase2b-discreteness.md and Q6 in
// docs/refinement/OPEN-QUESTIONS.md.
func DefaultZConfig() ZConfig { return ZConfig{Stat: StatMidP} }

// Suffix names the variant for method names ("" for the zero value).
func (c ZConfig) Suffix() string {
	s := ""
	if c.Stat == StatMidP {
		s += "-midp"
	}
	if c.MinN > 1 {
		s += fmt.Sprintf("-nmin%d", c.MinN)
	}
	if c.GlobalQ {
		s += "-qhat"
	}
	switch c.Phi {
	case PhiMedian:
		s += "-phi"
	case PhiBands:
		s += "-phibands"
	}
	return s
}

// Describe returns a short PT-BR description of the variant for reports.
func (c ZConfig) Describe() string {
	s := "signed root"
	if c.Stat == StatMidP {
		s = "mid-p exato"
	}
	if c.MinN > 1 {
		s += fmt.Sprintf(", arestas com n < %d fora", c.MinN)
	}
	if c.GlobalQ {
		s += ", q̂ global"
	}
	switch c.Phi {
	case PhiMedian:
		s += ", φ̂ mediana"
	case PhiBands:
		s += ", φ̂ por faixa de taxa"
	}
	return s
}

func (c ZConfig) edge(a, o, q float64) float64 {
	if c.Stat == StatMidP {
		return MidPZ(a, o, q)
	}
	return SignedRoot(a, o, q)
}

// TwoPoissonZ is EdgeZ with the prototype configuration and the split at T0.
func TwoPoissonZ(d Dataset) map[string]float64 {
	return EdgeZ(d, d.T0, ZConfig{})
}

// EdgeZ returns, for every node, the Stouffer combination of the per-edge
// statistic over the edges incident to it, before [0, split) against after
// [split, T), with q = (T − split)/T:
//
//	Z(v) = Σ r / sqrt(k_v)
//
// k_v counts the edges of v with at least one event in [0, T) (and at least
// MinN); an edge never observed is not known to exist. Dividing by sqrt(k_v)
// keeps Z at N(0, 1) under H0 whatever the degree, which the deviance sum of
// M3 does not. Negative Z is silence (M5), positive Z is burst (M6). A node
// with no counted edge gets Z = 0.
func EdgeZ(d Dataset, split int, cfg ZConfig) map[string]float64 {
	before := sumCounts(d, 0, split)
	after := sumCounts(d, split, d.T)
	q := float64(d.T-split) / float64(d.T)
	if cfg.GlobalQ {
		q = globalQ(before, after, q)
	}
	phi := func([2]string) float64 { return 1 }
	if cfg.Phi != PhiNone {
		phi = estimatePhi(d, split, cfg.Phi)
	}

	sum := make(map[string]float64, len(d.Nodes))
	cnt := make(map[string]int, len(d.Nodes))
	for _, key := range edgeKeys(before, after) {
		a, o := before[key], after[key]
		if a+o < cfg.MinN {
			continue
		}
		r := cfg.edge(float64(a), float64(o), q)
		if p := phi(key); p != 1 {
			r /= math.Sqrt(p)
		}
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

// SilenceZ is M5: the score is −Z(v) from EdgeZ at T0, so a node whose
// edges went quiet after T0 ranks high. It alarms when −Z > z(1 − α/N),
// one-sided with Bonferroni over the N nodes.
type SilenceZ struct {
	Cfg ZConfig
}

func (m SilenceZ) Name() string { return "silence-z-2pois" + m.Cfg.Suffix() }

func (m SilenceZ) Scores(d Dataset) map[string]float64 {
	z := EdgeZ(d, d.T0, m.Cfg)
	for k, v := range z {
		z[k] = -v
	}
	return z
}

func (SilenceZ) Alarm(score float64, n int, alpha float64) bool {
	return score > zCritical(n, alpha)
}

func (SilenceZ) AlarmRule() string { return "−Z > z(1−α/N)" }

// BurstZ is M6: the score is +Z(v) from EdgeZ at T0, with the same
// one-sided Bonferroni rule as M5.
type BurstZ struct {
	Cfg ZConfig
}

func (m BurstZ) Name() string { return "burst-z-2pois" + m.Cfg.Suffix() }

func (m BurstZ) Scores(d Dataset) map[string]float64 { return EdgeZ(d, d.T0, m.Cfg) }

func (BurstZ) Alarm(score float64, n int, alpha float64) bool {
	return score > zCritical(n, alpha)
}

func (BurstZ) AlarmRule() string { return "Z > z(1−α/N)" }
