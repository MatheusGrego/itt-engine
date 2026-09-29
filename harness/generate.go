// Package harness evaluates tension methods on synthetic graphs with planted
// suppressions (ADR-0006). It generates Poisson edge counts over T time
// windows, alters a seed-chosen target from window T0 on, and measures how
// well each method ranks that target.
package harness

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
)

// Scenario identifies what is planted in a dataset.
type Scenario string

const (
	S0Null     Scenario = "S0" // nothing planted; measures false positives
	S1Removal  Scenario = "S1" // all target edges drop to 0
	S2Thinning Scenario = "S2" // all target edges drop to 30%
	S3Sniper   Scenario = "S3" // planted low-degree node linked to hubs, then silent
	S4Hermit   Scenario = "S4" // no change; target is the lowest-degree node (control)
	S5Burst    Scenario = "S5" // all target edges rise to 300%

	// S2xParam is the parametric thinning of Tarefa 5: the target's edges
	// drop to Config.ThinMult. Not in Scenarios; used by the power sweep.
	S2xParam Scenario = "S2x"
)

// Scenarios lists every scenario in report order.
var Scenarios = []Scenario{S0Null, S1Removal, S2Thinning, S3Sniper, S4Hermit, S5Burst}

// Name returns a short human-readable name for the scenario.
func (s Scenario) Name() string {
	switch s {
	case S0Null:
		return "null"
	case S1Removal:
		return "removal"
	case S2Thinning:
		return "thinning"
	case S3Sniper:
		return "sniper"
	case S4Hermit:
		return "hermit"
	case S5Burst:
		return "burst"
	case S2xParam:
		return "thinning-x"
	}
	return string(s)
}

// Config holds the generator parameters.
type Config struct {
	N  int // number of nodes (a planted S3 node comes on top)
	T  int // number of time windows
	T0 int // first "after" window: [0, T0) is before, [T0, T) is after

	ParetoAlpha  float64 // tail of the out-degree distribution
	MaxOutDegree int     // out-degree cap
	FitnessSigma float64 // sigma of the lognormal destination fitness
	RateMu       float64 // mu of the lognormal edge rate
	RateSigma    float64 // sigma of the lognormal edge rate

	// Stress options (fase 2b, Tarefa 3). Their zero values keep the pure
	// Poisson generator with constant rates, bit for bit.

	// Dispersion is the gamma shape k of a gamma-Poisson (negative binomial)
	// count: each window draws its rate from Gamma(k, λ/k), so the variance
	// is λ + λ²/k. 0 means Poisson.
	Dispersion float64
	// Trend multiplies every rate in window t by (1 + Trend)^t.
	Trend float64
	// Seasonality is the amplitude A of a rate multiplier
	// 1 + A sin(2πt/P), with P = SeasonPeriod (4 when 0).
	Seasonality  float64
	SeasonPeriod int
	// Drift (extra, not in the plan) gives each edge its own random walk on
	// the log rate, with a N(0, Drift²) step per window: heterogeneous,
	// per-edge non-stationarity that no global correction can absorb.
	Drift float64

	// Power-curve options (fase 2b, Tarefa 5).

	// ThinMult is the after-multiplier of the S2x target's edges.
	ThinMult float64
	// TargetLo and TargetHi bound, as fractions of the degree ranking, the
	// band the S1/S2/S5/S2x target is drawn from; both 0 means the middle
	// band 45%-55%.
	TargetLo, TargetHi float64
	// PartialFrac (extra, not in the plan), when in (0, 1), alters only
	// that fraction of the target's edges (rounded, at least one); the
	// rest keep their rate. 0 alters them all.
	PartialFrac float64
}

// windowMult is the global rate multiplier of window t (Trend and
// Seasonality); exactly 1 when both are zero.
func (c Config) windowMult(t int) float64 {
	m := 1.0
	if c.Trend != 0 {
		m *= math.Pow(1+c.Trend, float64(t))
	}
	if c.Seasonality != 0 {
		p := c.SeasonPeriod
		if p == 0 {
			p = 4
		}
		m *= 1 + c.Seasonality*math.Sin(2*math.Pi*float64(t)/float64(p))
	}
	return m
}

// DefaultConfig returns the parameters from the 2026-09-29 plan.
func DefaultConfig() Config {
	return Config{
		N:            500,
		T:            12,
		T0:           8,
		ParetoAlpha:  1.5,
		MaxOutDegree: 50,
		FitnessSigma: 1.0,
		RateMu:       math.Log(4),
		RateSigma:    0.8,
	}
}

// Dataset is one generated replica.
type Dataset struct {
	Scenario Scenario
	N, T, T0 int
	Nodes    []string              // all node IDs, including a planted one
	Target   string                // "" in S0
	Counts   []map[[2]string]int   // Counts[t][{from,to}]; only counts > 0 are stored
	Rates    map[[2]string]float64 // ground-truth base rates; methods must not read it
	Altered  [][2]string           // target edges changed after T0 (ground truth, like Rates)
}

// RNG streams: the base graph and its rates depend only on the seed, so every
// scenario of a given seed shares them. Each edge draws its counts from its
// own stream, so the "before" windows are identical across scenarios too.
const (
	streamGraph  uint64 = 1
	streamTarget uint64 = 2
	streamCounts uint64 = 1 << 32
)

type edge struct {
	from, to int
	rate     float64
}

// Generate builds a dataset for scenario s with the given seed.
func Generate(s Scenario, cfg Config, seed uint64) Dataset {
	rng := rand.New(rand.NewPCG(seed, streamGraph))
	n := cfg.N

	fitness := make([]float64, n)
	for i := range fitness {
		fitness[i] = math.Exp(cfg.FitnessSigma * rng.NormFloat64())
	}
	cum := make([]float64, n)
	total := 0.0
	for i, f := range fitness {
		total += f
		cum[i] = total
	}

	var edges []edge
	for i := 0; i < n; i++ {
		deg := outDegree(rng, cfg, n)
		seen := map[int]bool{i: true}
		for picked, tries := 0, 0; picked < deg && tries < 100*deg; tries++ {
			j := sort.SearchFloat64s(cum, rng.Float64()*total)
			if j >= n || seen[j] {
				continue
			}
			seen[j] = true
			edges = append(edges, edge{from: i, to: j})
			picked++
		}
	}
	for k := range edges {
		edges[k].rate = lognormal(rng, cfg.RateMu, cfg.RateSigma)
	}

	trng := rand.New(rand.NewPCG(seed, streamTarget))
	target := -1
	numNodes := n
	switch s {
	case S1Removal, S2Thinning, S5Burst, S2xParam:
		target = bandDegree(trng, edges, n, cfg.TargetLo, cfg.TargetHi)
	case S4Hermit:
		target = lowestDegree(trng, edges, n)
	case S3Sniper:
		target = n
		numNodes = n + 1
		hubs := topFitness(fitness, (n+9)/10)
		k := 2 + trng.IntN(2)
		for _, h := range trng.Perm(len(hubs))[:k] {
			edges = append(edges, edge{from: target, to: hubs[h], rate: lognormal(trng, cfg.RateMu, cfg.RateSigma)})
		}
	}

	var afterMult float64
	switch s {
	case S1Removal, S3Sniper:
		afterMult = 0
	case S2Thinning:
		afterMult = 0.3
	case S5Burst:
		afterMult = 3
	case S2xParam:
		afterMult = cfg.ThinMult
	default:
		afterMult = 1
	}

	d := Dataset{
		Scenario: s,
		N:        numNodes,
		T:        cfg.T,
		T0:       cfg.T0,
		Nodes:    make([]string, numNodes),
		Counts:   make([]map[[2]string]int, cfg.T),
		Rates:    make(map[[2]string]float64, len(edges)),
	}
	for i := range d.Nodes {
		d.Nodes[i] = nodeID(i)
	}
	if target >= 0 {
		d.Target = nodeID(target)
	}
	for t := range d.Counts {
		d.Counts[t] = make(map[[2]string]int)
	}

	// With PartialFrac, only a random subset of the target's edges changes.
	var spared map[int]bool
	if target >= 0 && cfg.PartialFrac > 0 && cfg.PartialFrac < 1 {
		var inc []int
		for k, e := range edges {
			if e.from == target || e.to == target {
				inc = append(inc, k)
			}
		}
		keep := max(1, int(math.Round(cfg.PartialFrac*float64(len(inc)))))
		spared = map[int]bool{}
		for _, i := range trng.Perm(len(inc))[keep:] {
			spared[inc[i]] = true
		}
	}

	for k, e := range edges {
		key := [2]string{nodeID(e.from), nodeID(e.to)}
		d.Rates[key] = e.rate
		incident := (e.from == target || e.to == target) && !spared[k]
		if incident && afterMult != 1 {
			d.Altered = append(d.Altered, key)
		}
		erng := rand.New(rand.NewPCG(seed, streamCounts+uint64(k)))
		logDrift := 0.0
		for t := 0; t < cfg.T; t++ {
			lambda := e.rate * cfg.windowMult(t)
			if cfg.Drift > 0 {
				if t > 0 {
					logDrift += cfg.Drift * erng.NormFloat64()
				}
				lambda *= math.Exp(logDrift)
			}
			if incident && t >= cfg.T0 {
				lambda *= afterMult
			}
			if cfg.Dispersion > 0 && lambda > 0 {
				lambda *= gammaRand(erng, cfg.Dispersion) / cfg.Dispersion
			}
			if c := poisson(erng, lambda); c > 0 {
				d.Counts[t][key] = c
			}
		}
	}
	return d
}

func nodeID(i int) string { return fmt.Sprintf("n%04d", i) }

// outDegree draws min(MaxOutDegree, floor(2 * Pareto(alpha))), at least 2
// and at most n-1.
func outDegree(rng *rand.Rand, cfg Config, n int) int {
	x := 1 / math.Pow(1-rng.Float64(), 1/cfg.ParetoAlpha) // Pareto(alpha), x_m = 1
	deg := int(math.Floor(2 * x))
	deg = min(deg, cfg.MaxOutDegree, n-1)
	return max(deg, min(2, n-1))
}

func lognormal(rng *rand.Rand, mu, sigma float64) float64 {
	return math.Exp(mu + sigma*rng.NormFloat64())
}

// totalDegrees returns in+out degree per node over the base edges.
func totalDegrees(edges []edge, n int) []int {
	deg := make([]int, n)
	for _, e := range edges {
		deg[e.from]++
		deg[e.to]++
	}
	return deg
}

// bandDegree picks, at random, one node whose total degree ranks in
// [lo, hi) as fractions of all nodes; lo = hi = 0 is the middle 10%.
func bandDegree(rng *rand.Rand, edges []edge, n int, lo, hi float64) int {
	if lo == 0 && hi == 0 {
		return nearMedianDegree(rng, edges, n)
	}
	deg := totalDegrees(edges, n)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return deg[idx[a]] < deg[idx[b]] })
	l, h := int(lo*float64(n)), int(hi*float64(n))
	if h <= l {
		h = l + 1
	}
	return idx[l+rng.IntN(h-l)]
}

// nearMedianDegree picks, at random, one node whose total degree ranks in
// the middle 10% of all nodes.
func nearMedianDegree(rng *rand.Rand, edges []edge, n int) int {
	deg := totalDegrees(edges, n)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return deg[idx[a]] < deg[idx[b]] })
	lo, hi := n*45/100, n*55/100
	if hi <= lo {
		lo, hi = n/2, n/2+1
	}
	return idx[lo+rng.IntN(hi-lo)]
}

// lowestDegree picks, at random among ties, a node with the lowest total degree.
func lowestDegree(rng *rand.Rand, edges []edge, n int) int {
	deg := totalDegrees(edges, n)
	best := math.MaxInt
	var ties []int
	for i, d := range deg {
		switch {
		case d < best:
			best, ties = d, []int{i}
		case d == best:
			ties = append(ties, i)
		}
	}
	return ties[rng.IntN(len(ties))]
}

// topFitness returns the k nodes with the highest fitness.
func topFitness(fitness []float64, k int) []int {
	idx := make([]int, len(fitness))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return fitness[idx[a]] > fitness[idx[b]] })
	return idx[:k]
}

// poisson draws from Poisson(lambda). Large rates are split into chunks of
// at most 30, using additivity, to keep exp(-lambda) away from underflow.
func poisson(rng *rand.Rand, lambda float64) int {
	if lambda <= 0 {
		return 0
	}
	k := 0
	for lambda > 30 {
		k += poissonKnuth(rng, 30)
		lambda -= 30
	}
	return k + poissonKnuth(rng, lambda)
}

// gammaRand draws from Gamma(shape, 1) with the Marsaglia-Tsang method;
// shapes below 1 use the boost Gamma(k) = Gamma(k+1)·U^(1/k).
func gammaRand(rng *rand.Rand, shape float64) float64 {
	if shape < 1 {
		u := rng.Float64()
		return gammaRand(rng, shape+1) * math.Pow(u, 1/shape)
	}
	d := shape - 1.0/3
	c := 1 / math.Sqrt(9*d)
	for {
		var x, v float64
		for {
			x = rng.NormFloat64()
			v = 1 + c*x
			if v > 0 {
				break
			}
		}
		v = v * v * v
		u := rng.Float64()
		if u < 1-0.0331*x*x*x*x {
			return d * v
		}
		if math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
}

func poissonKnuth(rng *rand.Rand, lambda float64) int {
	limit := math.Exp(-lambda)
	p := 1.0
	for k := 0; ; k++ {
		p *= rng.Float64()
		if p <= limit {
			return k
		}
	}
}
