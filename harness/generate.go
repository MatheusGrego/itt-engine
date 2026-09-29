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
	case S1Removal, S2Thinning, S5Burst:
		target = nearMedianDegree(trng, edges, n)
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

	for k, e := range edges {
		key := [2]string{nodeID(e.from), nodeID(e.to)}
		d.Rates[key] = e.rate
		incident := e.from == target || e.to == target
		erng := rand.New(rand.NewPCG(seed, streamCounts+uint64(k)))
		for t := 0; t < cfg.T; t++ {
			lambda := e.rate
			if incident && t >= cfg.T0 {
				lambda *= afterMult
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
