package harness

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// DegreeBand is a slice of the degree ranking the target is drawn from.
type DegreeBand struct {
	Name   string
	Lo, Hi float64
}

// PowerBands are the Tarefa 5 target bands: low p10-p30, mid p45-p55,
// high p80-p95.
var PowerBands = []DegreeBand{{"baixo (p10–p30)", 0.10, 0.30}, {"médio (p45–p55)", 0.45, 0.55}, {"alto (p80–p95)", 0.80, 0.95}}

// PowerMults are the S2x after-multipliers; PowerWindows the after windows.
var (
	PowerMults   = []float64{0.9, 0.8, 0.7, 0.5, 0.3, 0}
	PowerWindows = []int{1, 2, 4}
)

// PowerCell is one (mult, band, windows, fraction) cell of the power sweep.
type PowerCell struct {
	Mult    float64
	Band    DegreeBand
	Windows int
	Frac    float64 // PartialFrac; 0 = every target edge

	Replicas  int
	Power     float64 // fraction of replicas with the target in alarm
	AUCMean   float64
	Degree    float64 // mean observed degree of the target
	Lost      float64 // mean expected events lost, Σ λ_e w (1 − mult)
	Hellinger float64 // mean Σ (√e − √o)², e = λ w, o = λ w mult
	Predicted float64 // mean predicted power (see predictPower)
}

// predictPower is the Poisson prediction for the Stouffer Z of a target
// whose affected edges have rates lam (the other k − len(lam) edges are
// unchanged): with the variance-stabilizing root, each affected edge gives
// r ≈ N(μ_e, 1) with μ_e = −2(√e − √o)/sqrt(1 + w/T0), where the
// denominator is the noise of the before-period estimate. Then −Z ≈
// N(Σ|μ_e|/√k, 1) and the power is P(−Z > zc).
func predictPower(lam []float64, k int, mult float64, w, t0 int, zc float64) (power, hell float64) {
	var mu float64
	for _, l := range lam {
		e, o := l*float64(w), l*float64(w)*mult
		d := math.Sqrt(e) - math.Sqrt(o)
		hell += d * d
		mu += 2 * d / math.Sqrt(1+float64(w)/float64(t0))
	}
	if k == 0 {
		return 0, hell
	}
	return 0.5 * math.Erfc((zc-mu/math.Sqrt(float64(k)))/math.Sqrt2), hell
}

// RunPowerCell measures one cell with the corrected silence Z.
func RunPowerCell(c PowerCell, base Config, replicas, workers int, alpha float64) PowerCell {
	cfg := base
	cfg.T = cfg.T0 + c.Windows
	cfg.ThinMult = c.Mult
	cfg.TargetLo, cfg.TargetHi = c.Band.Lo, c.Band.Hi
	cfg.PartialFrac = c.Frac
	m := SilenceZ{Cfg: CorrectedZConfig()}
	type rep struct{ alarm, auc, deg, lost, hell, pred float64 }
	reps := make([]rep, replicas)
	parallelFor(replicas, workers, func(r int) {
		d := Generate(S2xParam, cfg, uint64(r+1))
		s := m.Scores(d)
		var out rep
		if m.Alarm(s[d.Target], d.N, alpha) {
			out.alarm = 1
		}
		out.auc = AUC(s, d.Nodes, d.Target)
		k := observedDegree(d)[d.Target]
		out.deg = float64(k)
		var lam []float64
		for _, key := range d.Altered {
			lam = append(lam, d.Rates[key])
		}
		for _, l := range lam {
			out.lost += l * float64(c.Windows) * (1 - c.Mult)
		}
		out.pred, out.hell = predictPower(lam, k, c.Mult, c.Windows, cfg.T0, zCritical(d.N, alpha))
		reps[r] = out
	})
	var aucs []float64
	for _, r := range reps {
		c.Power += r.alarm
		aucs = append(aucs, r.auc)
		c.Degree += r.deg
		c.Lost += r.lost
		c.Hellinger += r.hell
		c.Predicted += r.pred
	}
	n := float64(replicas)
	c.Replicas = replicas
	c.Power /= n
	c.AUCMean, _ = meanSD(aucs)
	c.Degree /= n
	c.Lost /= n
	c.Hellinger /= n
	c.Predicted /= n
	return c
}

// Power runs the Tarefa 5 sweep (mult × band × windows) plus the extra
// partial-silence rows, and returns the markdown and the CSV rows.
func Power(opts Options) (string, [][]string) {
	var cells []PowerCell
	for _, b := range PowerBands {
		for _, w := range PowerWindows {
			for _, m := range PowerMults {
				cells = append(cells, RunPowerCell(PowerCell{Mult: m, Band: b, Windows: w}, opts.Config, opts.Replicas, opts.Workers, opts.Alpha))
			}
		}
	}
	var partial []PowerCell
	for _, b := range PowerBands {
		for _, f := range []float64{0.5, 0.25, 0.1} {
			partial = append(partial, RunPowerCell(PowerCell{Mult: 0, Band: b, Windows: 4, Frac: f}, opts.Config, opts.Replicas, opts.Workers, opts.Alpha))
		}
	}

	rows := [][]string{{"mult", "band", "windows_after", "partial_frac", "replicas", "power", "power_lo", "power_hi",
		"auc_mean", "target_degree", "expected_lost", "hellinger", "predicted_power"}}
	for _, c := range append(append([]PowerCell{}, cells...), partial...) {
		lo, hi := Wilson(int(math.Round(c.Power*float64(c.Replicas))), c.Replicas)
		rows = append(rows, []string{num(c.Mult), c.Band.Name, strconv.Itoa(c.Windows), num(c.Frac), strconv.Itoa(c.Replicas),
			num(c.Power), num(lo), num(hi), num(c.AUCMean), num(c.Degree), num(c.Lost), num(c.Hellinger), num(c.Predicted)})
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Parâmetros: N = %d, t0 = %d (8 janelas antes), T = t0 + janelas depois, R = %d réplicas por célula, α = %g. Método: `%s` (mid-p + q̂ global + φ̂ por faixa), alarme −Z > z(1 − α/N). Gerador Poisson base.\n\n",
		opts.Config.N, opts.Config.T0, opts.Replicas, opts.Alpha, SilenceZ{Cfg: CorrectedZConfig()}.Name())
	b.WriteString("Célula: poder medido [IC 95% Wilson] / poder previsto. Abaixo de cada tabela: eventos esperados perdidos (média).\n\n")
	for _, band := range PowerBands {
		fmt.Fprintf(&b, "### Alvo de grau %s\n\n| Janelas depois | grau médio |", band.Name)
		for _, m := range PowerMults {
			fmt.Fprintf(&b, " mult %g |", m)
		}
		b.WriteString("\n|---|---|")
		for range PowerMults {
			b.WriteString("---|")
		}
		b.WriteString("\n")
		for _, w := range PowerWindows {
			var row []PowerCell
			for _, c := range cells {
				if c.Band == band && c.Windows == w {
					row = append(row, c)
				}
			}
			fmt.Fprintf(&b, "| %d | %.1f |", w, row[0].Degree)
			for _, c := range row {
				lo, hi := Wilson(int(math.Round(c.Power*float64(c.Replicas))), c.Replicas)
				fmt.Fprintf(&b, " %.2f [%.2f, %.2f] / %.2f |", c.Power, lo, hi, c.Predicted)
			}
			b.WriteString("\n")
			fmt.Fprintf(&b, "| ↳ perdidos | |")
			for _, c := range row {
				fmt.Fprintf(&b, " %.0f |", c.Lost)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("### Extra: silêncio parcial (só uma fração das arestas do alvo zera; 4 janelas depois)\n\n")
	b.WriteString("| Faixa | fração | grau médio | poder [IC 95%] | previsto | AUC | perdidos |\n|---|---|---|---|---|---|---|\n")
	for _, c := range partial {
		lo, hi := Wilson(int(math.Round(c.Power*float64(c.Replicas))), c.Replicas)
		fmt.Fprintf(&b, "| %s | %g | %.1f | %.2f [%.2f, %.2f] | %.2f | %.3f | %.0f |\n", c.Band.Name, c.Frac, c.Degree, c.Power, lo, hi, c.Predicted, c.AUCMean, c.Lost)
	}

	b.WriteString("\n### Poder contra eventos perdidos e contra a distância de Hellinger\n\n")
	b.WriteString("Todas as células do sweep principal, ordenadas pela distância Σ(√e − √o)². Se o poder fosse função só dela, a coluna de poder subiria de forma monótona.\n\n")
	b.WriteString("| Σ(√e−√o)² | perdidos | grau | janelas | mult | poder | previsto |\n|---|---|---|---|---|---|---|\n")
	sorted := append([]PowerCell{}, cells...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Hellinger < sorted[j].Hellinger })
	for _, c := range sorted {
		if c.Mult == 1 {
			continue
		}
		fmt.Fprintf(&b, "| %.2f | %.0f | %.1f | %d | %g | %.2f | %.2f |\n", c.Hellinger, c.Lost, c.Degree, c.Windows, c.Mult, c.Power, c.Predicted)
	}
	return b.String(), rows
}
