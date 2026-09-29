package harness

import (
	"fmt"
	"math"
	"strings"
)

// ZFunc computes a per-node Z; negative is silence, positive is burst.
type ZFunc func(d Dataset) map[string]float64

// CalibLevels are the one-sided nominal levels checked per node under S0.
var CalibLevels = []float64{1e-2, 1e-3, 1e-4}

// DegreeStrata bound the observed degree k_v of the calibration strata:
// [1, 3], [4, 10], [11, 30], [31, ∞).
var DegreeStrata = []int{1, 4, 11, 31}

// Calibration summarizes a Z over every node-replica of S0 with at least one
// observed edge. FWER with R = 200 has a ±0.03 interval; per-node rates use
// N·R ≈ 10⁵ samples, so they resolve miscalibration the FWER cannot. The
// samples are not independent (an edge enters two nodes), so the Wilson
// intervals of the per-node rates are approximate.
type Calibration struct {
	Samples   int
	Mean, Var float64
	// Silence[i] and Burst[i] count node-replicas with −Z (resp. Z) above
	// z(1 − CalibLevels[i]).
	Silence, Burst []int
	Strata         []StratumCalib
}

// StratumCalib is Calibration restricted to one observed-degree stratum, at
// the middle level CalibLevels[1].
type StratumCalib struct {
	Lo, Hi         int // Hi = 0 means unbounded
	Samples        int
	Mean, Var      float64
	Silence, Burst int
}

// NullCalibration runs R replicas of S0 under cfg and collects Z.
func NullCalibration(zf ZFunc, cfg Config, replicas, workers int) Calibration {
	type sample struct {
		z float64
		k int
	}
	per := make([][]sample, replicas)
	parallelFor(replicas, workers, func(r int) {
		d := Generate(S0Null, cfg, uint64(r+1))
		z := zf(d)
		k := observedDegree(d)
		var out []sample
		for _, id := range d.Nodes {
			if k[id] > 0 {
				out = append(out, sample{z[id], k[id]})
			}
		}
		per[r] = out
	})

	crit := make([]float64, len(CalibLevels))
	for i, a := range CalibLevels {
		crit[i] = zCritical(1, a)
	}
	c := Calibration{Silence: make([]int, len(CalibLevels)), Burst: make([]int, len(CalibLevels))}
	c.Strata = make([]StratumCalib, len(DegreeStrata))
	for i, lo := range DegreeStrata {
		c.Strata[i].Lo = lo
		if i+1 < len(DegreeStrata) {
			c.Strata[i].Hi = DegreeStrata[i+1] - 1
		}
	}
	var sum, sum2 float64
	ssum := make([]float64, len(DegreeStrata))
	ssum2 := make([]float64, len(DegreeStrata))
	for _, rep := range per {
		for _, s := range rep {
			c.Samples++
			sum += s.z
			sum2 += s.z * s.z
			for i, cr := range crit {
				if -s.z > cr {
					c.Silence[i]++
				}
				if s.z > cr {
					c.Burst[i]++
				}
			}
			j := len(DegreeStrata) - 1
			for j > 0 && s.k < DegreeStrata[j] {
				j--
			}
			st := &c.Strata[j]
			st.Samples++
			ssum[j] += s.z
			ssum2[j] += s.z * s.z
			if -s.z > crit[1] {
				st.Silence++
			}
			if s.z > crit[1] {
				st.Burst++
			}
		}
	}
	c.Mean, c.Var = momentsFromSums(sum, sum2, c.Samples)
	for j := range c.Strata {
		c.Strata[j].Mean, c.Strata[j].Var = momentsFromSums(ssum[j], ssum2[j], c.Strata[j].Samples)
	}
	return c
}

func momentsFromSums(sum, sum2 float64, n int) (mean, variance float64) {
	if n == 0 {
		return math.NaN(), math.NaN()
	}
	mean = sum / float64(n)
	return mean, sum2/float64(n) - mean*mean
}

// observedDegree counts, per node, the edges with at least one event.
func observedDegree(d Dataset) map[string]int {
	k := make(map[string]int, len(d.Nodes))
	for key := range sumCounts(d, 0, d.T) {
		k[key[0]]++
		k[key[1]]++
	}
	return k
}

// rateCell renders an observed per-node rate as "obs (×ratio)".
func rateCell(count, n int, nominal float64) string {
	if n == 0 {
		return "n/a"
	}
	p := float64(count) / float64(n)
	return fmt.Sprintf("%.2g (×%.2f)", p, p/nominal)
}

// CalibrationMarkdown renders one row per labeled calibration and a
// per-stratum table.
func CalibrationMarkdown(labels []string, cals []Calibration) string {
	var b strings.Builder
	b.WriteString("| Variante | nós-réplica | média Z | var Z |")
	for _, a := range CalibLevels {
		fmt.Fprintf(&b, " silêncio %.0e |", a)
	}
	for _, a := range CalibLevels {
		fmt.Fprintf(&b, " burst %.0e |", a)
	}
	b.WriteString("\n|---|---|---|---|")
	for range 2 * len(CalibLevels) {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for i, c := range cals {
		fmt.Fprintf(&b, "| %s | %d | %+.3f | %.3f |", labels[i], c.Samples, c.Mean, c.Var)
		for j, a := range CalibLevels {
			fmt.Fprintf(&b, " %s |", rateCell(c.Silence[j], c.Samples, a))
		}
		for j, a := range CalibLevels {
			fmt.Fprintf(&b, " %s |", rateCell(c.Burst[j], c.Samples, a))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\nPor faixa de grau observado k_v (taxa por nó no nível %.0e):\n\n", CalibLevels[1])
	b.WriteString("| Variante | k_v | nós-réplica | média Z | var Z | silêncio | burst |\n|---|---|---|---|---|---|---|\n")
	for i, c := range cals {
		for _, st := range c.Strata {
			rng := fmt.Sprintf("%d–%d", st.Lo, st.Hi)
			if st.Hi == 0 {
				rng = fmt.Sprintf("≥ %d", st.Lo)
			}
			fmt.Fprintf(&b, "| %s | %s | %d | %+.3f | %.3f | %s | %s |\n", labels[i], rng, st.Samples, st.Mean, st.Var,
				rateCell(st.Silence, st.Samples, CalibLevels[1]), rateCell(st.Burst, st.Samples, CalibLevels[1]))
		}
	}
	return b.String()
}
