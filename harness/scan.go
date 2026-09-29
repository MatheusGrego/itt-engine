package harness

import (
	"fmt"
	"math"
	"strings"
)

// ScanSplits returns the candidate splits of the scan mode for T windows:
// [T − 4, T − 1], clipped to [1, T − 1].
func ScanSplits(t int) []int {
	var s []int
	for x := max(1, t-4); x <= t-1; x++ {
		s = append(s, x)
	}
	return s
}

// ScanZ is M5/M6 with t0 unknown (Tarefa 6): for every split s in
// ScanSplits(T) it computes Z_s with EdgeZ on [0, s) against [s, T), and the
// score is max_s(−Z_s) for silence (Burst false) or max_s(Z_s) for burst.
// The alarm is Bonferroni over N × (number of splits), which is
// conservative because the splits share windows and are correlated. T must
// match the datasets it scores (it sets the number of splits of the alarm).
type ScanZ struct {
	Cfg   ZConfig
	Burst bool
	T     int
}

func (m ScanZ) Name() string {
	side := "silence"
	if m.Burst {
		side = "burst"
	}
	return side + "-z-scan" + m.Cfg.Suffix()
}

func (m ScanZ) Scores(d Dataset) map[string]float64 {
	best := make(map[string]float64, len(d.Nodes))
	for _, id := range d.Nodes {
		best[id] = math.Inf(-1)
	}
	for _, s := range ScanSplits(d.T) {
		for id, z := range EdgeZ(d, s, m.Cfg) {
			if !m.Burst {
				z = -z
			}
			if z > best[id] {
				best[id] = z
			}
		}
	}
	return best
}

func (m ScanZ) Alarm(score float64, n int, alpha float64) bool {
	return score > zCritical(n*len(ScanSplits(m.T)), alpha)
}

func (m ScanZ) AlarmRule() string {
	if m.Burst {
		return "max Z_s > z(1−α/(N·splits))"
	}
	return "max −Z_s > z(1−α/(N·splits))"
}

// ScanConditions are the Tarefa 6 generator conditions, all with T0Random:
// the base generator, and (extra) the P = 4 season, which the fixed t0 = 8
// hides but a split at 9, 10 or 11 does not.
func ScanConditions(base Config) []Condition {
	b := base
	b.T0Random = true
	s := b
	s.Seasonality = 0.3
	return []Condition{
		{Name: "t0 sorteado em [T−4, T−1]", Config: b},
		{Name: "extra: t0 sorteado + Seasonality A = 0.3, P = 4", Config: s},
	}
}

// Scan compares, on replicas whose real t0 is drawn in [T − 4, T − 1], the
// Z with t0 known (Dataset.T0) against the scan over ScanSplits (Tarefa 6),
// with the default and the corrected configuration.
func Scan(opts Options) (string, []CondResult) {
	var methods []Method
	for _, c := range []ZConfig{DefaultZConfig(), CorrectedZConfig()} {
		methods = append(methods,
			SilenceZ{Cfg: c}, ScanZ{Cfg: c, T: opts.Config.T},
			BurstZ{Cfg: c}, ScanZ{Cfg: c, T: opts.Config.T, Burst: true})
	}
	results := Sweep(methods, ScanConditions(opts.Config), opts)

	var b strings.Builder
	fmt.Fprintf(&b, "Parâmetros: N = %d, T = %d, t0 real sorteado por réplica em [%d, %d], R = %d réplicas por cenário e condição (sementes 1..%d), α = %g.\n\n",
		opts.Config.N, opts.Config.T, max(1, opts.Config.T-4), opts.Config.T-1, opts.Replicas, opts.Replicas, opts.Alpha)
	b.WriteString("- `silence-z-2pois*` / `burst-z-2pois*`: t0 conhecido (usa o t0 real da réplica), limiar z(1 − α/N).\n")
	b.WriteString("- `*-z-scan*`: t0 desconhecido, score = máximo sobre os splits s ∈ [T−4, T−1], limiar z(1 − α/(N·4)).\n\n")
	b.WriteString("### Resumo: FWER no S0\n\n")
	b.WriteString(summaryTable(results, S0Null, fwerCell))
	b.WriteString("\n### Resumo: AUC e poder por cenário\n\n")
	for _, s := range []Scenario{S1Removal, S2Thinning, S3Sniper, S5Burst} {
		b.WriteString(summaryTable(results, s, func(r Result) string { return aucCell(r) + " / " + powerCell(r) }))
		b.WriteString("\n")
	}
	return b.String(), results
}
