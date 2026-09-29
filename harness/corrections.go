package harness

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// globalQ returns q̂ = total after / total over every edge, or fallback when
// the graph has no events.
func globalQ(before, after map[[2]string]int, fallback float64) float64 {
	var a, o float64
	for _, c := range before {
		a += float64(c)
	}
	for _, c := range after {
		o += float64(c)
	}
	if a+o == 0 || a == 0 || o == 0 {
		return fallback
	}
	return o / (a + o)
}

// phiBandEdges are the lower bounds, in events per before-window, of the
// PhiBands rate bands: [1, 2), [2, 4), [4, 8), [8, 16), [16, ∞). Edges with
// a mean below 1 use the first band, whose φ̂ is at least as large as the
// one they would need (φ grows with the rate under gamma-Poisson), so the
// choice is conservative.
var phiBandEdges = []float64{1, 2, 4, 8, 16}

// estimatePhi estimates the overdispersion from the before windows
// [0, split) only. For each edge with mean c̄ ≥ 1 per window, the Pearson
// statistic Σ(c_t − c̄)²/c̄ over (split − 1) degrees of freedom is ≈ 1 for
// Poisson; φ̂ is its median over the edges (PhiMedian) or over the edges of
// each rate band (PhiBands), floored at 1. The returned function gives the
// φ̂ that applies to an edge.
func estimatePhi(d Dataset, split int, mode PhiMode) func([2]string) float64 {
	one := func([2]string) float64 { return 1 }
	if split < 2 {
		return one
	}
	sums := make(map[[2]string]float64)
	sq := make(map[[2]string]float64)
	for t := 0; t < split; t++ {
		for key, c := range d.Counts[t] {
			sums[key] += float64(c)
			sq[key] += float64(c) * float64(c)
		}
	}
	s := float64(split)
	band := func(mean float64) int {
		b := 0
		for b+1 < len(phiBandEdges) && mean >= phiBandEdges[b+1] {
			b++
		}
		return b
	}
	var all []float64
	byBand := make([][]float64, len(phiBandEdges))
	for key, sum := range sums {
		mean := sum / s
		if mean < 1 {
			continue
		}
		pearson := (sq[key] - s*mean*mean) / mean / (s - 1)
		all = append(all, pearson)
		b := band(mean)
		byBand[b] = append(byBand[b], pearson)
	}
	global := 1.0
	if len(all) > 0 {
		global = math.Max(1, median(all))
	}
	if mode == PhiMedian {
		return func([2]string) float64 { return global }
	}
	phis := make([]float64, len(phiBandEdges))
	for b, xs := range byBand {
		if len(xs) == 0 {
			phis[b] = global
		} else {
			phis[b] = math.Max(1, median(xs))
		}
	}
	return func(key [2]string) float64 { return phis[band(sums[key]/s)] }
}

// CorrectionVariants are the Tarefa 4 options on top of DefaultZConfig, one
// at a time and then combined.
func CorrectionVariants() []ZConfig {
	base := DefaultZConfig()
	with := func(f func(*ZConfig)) ZConfig {
		c := base
		f(&c)
		return c
	}
	return []ZConfig{
		base,
		with(func(c *ZConfig) { c.GlobalQ = true }),
		with(func(c *ZConfig) { c.Phi = PhiMedian }),
		with(func(c *ZConfig) { c.Phi = PhiBands }),
		with(func(c *ZConfig) { c.GlobalQ, c.Phi = true, PhiMedian }),
		with(func(c *ZConfig) { c.GlobalQ, c.Phi = true, PhiBands }),
	}
}

// Corrections runs every CorrectionVariants entry, as M5 and M6, on S0..S5
// under every StressConditions entry (Tarefa 4).
func Corrections(opts Options) (string, []CondResult) {
	var methods []Method
	for _, v := range CorrectionVariants() {
		methods = append(methods, SilenceZ{Cfg: v}, BurstZ{Cfg: v})
	}
	results := Sweep(methods, StressConditions(opts.Config), opts)

	var b strings.Builder
	fmt.Fprintf(&b, "Parâmetros: N = %d, T = %d, t0 = %d, R = %d réplicas por cenário e condição (sementes 1..%d), α = %g. Condições da Tarefa 3.\n\n",
		opts.Config.N, opts.Config.T, opts.Config.T0, opts.Replicas, opts.Replicas, opts.Alpha)
	b.WriteString("Variantes (todas sobre o mid-p default):\n\n")
	for _, v := range CorrectionVariants() {
		fmt.Fprintf(&b, "- `%s` / `%s`: %s\n", SilenceZ{Cfg: v}.Name(), BurstZ{Cfg: v}.Name(), v.Describe())
	}
	b.WriteString("\n### Resumo: FWER no S0 por condição × variante\n\n")
	b.WriteString(summaryTable(results, S0Null, fwerCell))
	b.WriteString("\n### Resumo: AUC do alvo (silêncio em S2, burst em S5)\n\n")
	b.WriteString(summaryTable(results, S2Thinning, aucCell))
	b.WriteString("\n")
	b.WriteString(summaryTable(results, S5Burst, aucCell))
	b.WriteString("\n### Tabelas completas por método\n\n")
	b.WriteString("FWER: réplicas do S0 com ≥ 1 alarme (IC 95% de Wilson). AUC: média ± desvio. Poder: fração das réplicas com o alvo em alarme.\n\n")
	b.WriteString(MethodTables(results, Scenarios))
	return b.String(), results
}

// summaryTable renders rows = condition, columns = method, cells = cell(r)
// for scenario s.
func summaryTable(results []CondResult, s Scenario, cell func(Result) string) string {
	var methods, conds []string
	seenM, seenC := map[string]bool{}, map[string]bool{}
	by := map[[2]string]Result{}
	for _, r := range results {
		if r.Scenario != s {
			continue
		}
		if !seenM[r.Method] {
			seenM[r.Method] = true
			methods = append(methods, r.Method)
		}
		if !seenC[r.Condition] {
			seenC[r.Condition] = true
			conds = append(conds, r.Condition)
		}
		by[[2]string{r.Condition, r.Method}] = r.Result
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s:\n\n| Condição |", s)
	for _, m := range methods {
		fmt.Fprintf(&b, " %s |", m)
	}
	b.WriteString("\n|---|")
	for range methods {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for _, c := range conds {
		fmt.Fprintf(&b, "| %s |", c)
		for _, m := range methods {
			fmt.Fprintf(&b, " %s |", cell(by[[2]string{c, m}]))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// median returns the median of xs (NaN if empty); xs is reordered.
func median(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	sort.Float64s(xs)
	m := len(xs) / 2
	if len(xs)%2 == 1 {
		return xs[m]
	}
	return (xs[m-1] + xs[m]) / 2
}

// CorrectedZConfig is the M5/M6 configuration with the Tarefa 4 corrections
// that the stress run showed necessary: q̂ global (trend, out-of-phase
// season) and φ̂ per rate band (overdispersion). Tarefas 5 and 6 use it.
func CorrectedZConfig() ZConfig {
	c := DefaultZConfig()
	c.GlobalQ, c.Phi = true, PhiBands
	return c
}
