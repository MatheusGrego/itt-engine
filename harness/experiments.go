package harness

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"runtime"
	"strconv"
	"strings"
)

// Condition is a named generator configuration for the fase 2b sweeps.
type Condition struct {
	Name   string
	Config Config
}

// CondResult is a Result tagged with the condition that produced it.
type CondResult struct {
	Condition string
	Result
}

// Sweep runs every method on every scenario of opts under each condition.
func Sweep(methods []Method, conds []Condition, opts Options) []CondResult {
	var out []CondResult
	for _, c := range conds {
		o := opts
		o.Config = c.Config
		for _, r := range Run(methods, o) {
			out = append(out, CondResult{c.Name, r})
		}
	}
	return out
}

// ExperimentOptions returns the fase 2b defaults: α = 0.05, top 10, R
// replicas, every CPU.
func ExperimentOptions(replicas int) Options {
	o := DefaultOptions()
	o.Replicas = replicas
	o.Workers = runtime.NumCPU()
	return o
}

func fwerCell(r Result) string {
	if math.IsNaN(r.FWER) {
		return "n/a"
	}
	lo, hi := r.FWERInterval()
	return fmt.Sprintf("%.3f [%.3f, %.3f]", r.FWER, lo, hi)
}

func aucCell(r Result) string {
	if math.IsNaN(r.AUCMean) {
		return "n/a"
	}
	return fmt.Sprintf("%.3f ± %.3f", r.AUCMean, r.AUCSD)
}

func powerCell(r Result) string {
	if math.IsNaN(r.Power) {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", r.Power)
}

// MethodTables renders, for each method, one row per condition with the S0
// FWER (95% Wilson), the AUC (mean ± sd) and the power (fraction of replicas
// with the target in alarm) of each planted scenario.
func MethodTables(results []CondResult, scenarios []Scenario) string {
	var methods, conds []string
	seenM, seenC := map[string]bool{}, map[string]bool{}
	by := map[[3]string]Result{}
	for _, r := range results {
		if !seenM[r.Method] {
			seenM[r.Method] = true
			methods = append(methods, r.Method)
		}
		if !seenC[r.Condition] {
			seenC[r.Condition] = true
			conds = append(conds, r.Condition)
		}
		by[[3]string{r.Condition, r.Method, string(r.Scenario)}] = r.Result
	}
	var planted []Scenario
	for _, s := range scenarios {
		if s != S0Null {
			planted = append(planted, s)
		}
	}
	var b strings.Builder
	for _, m := range methods {
		fmt.Fprintf(&b, "#### %s\n\n| Condição | FWER S0 [IC 95%%] |", m)
		for _, s := range planted {
			fmt.Fprintf(&b, " AUC %s |", s)
		}
		for _, s := range planted {
			fmt.Fprintf(&b, " poder %s |", s)
		}
		b.WriteString("\n|---|---|")
		for range 2 * len(planted) {
			b.WriteString("---|")
		}
		b.WriteString("\n")
		for _, c := range conds {
			fmt.Fprintf(&b, "| %s | %s |", c, fwerCell(by[[3]string{c, m, string(S0Null)}]))
			for _, s := range planted {
				fmt.Fprintf(&b, " %s |", aucCell(by[[3]string{c, m, string(s)}]))
			}
			for _, s := range planted {
				fmt.Fprintf(&b, " %s |", powerCell(by[[3]string{c, m, string(s)}]))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// WriteCondCSV writes one row per (condition, scenario, method); NaN fields
// are left empty.
func WriteCondCSV(w io.Writer, results []CondResult) error {
	cw := csv.NewWriter(w)
	header := []string{"condition", "scenario", "method", "replicas", "auc_mean", "auc_sd", "hit_at_k",
		"alarm_rule", "alarm_rate", "fwer", "fwer_count", "fwer_lo", "fwer_hi", "power"}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range results {
		lo, hi := r.FWERInterval()
		if math.IsNaN(r.FWER) {
			lo, hi = math.NaN(), math.NaN()
		}
		row := []string{r.Condition, string(r.Scenario), r.Method, strconv.Itoa(r.Replicas),
			num(r.AUCMean), num(r.AUCSD), num(r.HitAtK), r.AlarmRule, num(r.AlarmRate),
			num(r.FWER), strconv.Itoa(r.FWERCount), num(lo), num(hi), num(r.Power)}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// DiscretenessVariants are the three per-edge treatments of Tarefa 2.
var DiscretenessVariants = []ZConfig{
	{Stat: StatSignedRoot},
	{Stat: StatMidP},
	{Stat: StatSignedRoot, MinN: 3},
}

// LowRateMu is the low-count regime of the stress tests: median rate 0.5
// per window.
var LowRateMu = math.Log(0.5)

// Discreteness compares the variants of DiscretenessVariants (Tarefa 2) on
// S0..S5 and in a per-node null calibration, under the default generator and
// under the low-count regime.
func Discreteness(opts Options) (string, []CondResult) {
	var methods []Method
	for _, v := range DiscretenessVariants {
		methods = append(methods, SilenceZ{Cfg: v}, BurstZ{Cfg: v})
	}
	low := opts.Config
	low.RateMu = LowRateMu
	conds := []Condition{
		{Name: "default (RateMu = ln 4)", Config: opts.Config},
		{Name: "pouca contagem (RateMu = ln 0.5)", Config: low},
	}
	results := Sweep(methods, conds, opts)

	var b strings.Builder
	fmt.Fprintf(&b, "Parâmetros: N = %d, T = %d, t0 = %d, R = %d réplicas por cenário e condição (sementes 1..%d), α = %g.\n\n",
		opts.Config.N, opts.Config.T, opts.Config.T0, opts.Replicas, opts.Replicas, opts.Alpha)
	b.WriteString("Variantes:\n\n")
	for _, v := range DiscretenessVariants {
		fmt.Fprintf(&b, "- `%s` / `%s`: %s\n", SilenceZ{Cfg: v}.Name(), BurstZ{Cfg: v}.Name(), v.Describe())
	}
	b.WriteString("\n### FWER, AUC e poder por variante\n\n")
	b.WriteString("FWER: réplicas do S0 com ≥ 1 alarme (IC 95% de Wilson). AUC: média ± desvio. Poder: fração das réplicas com o alvo em alarme.\n\n")
	b.WriteString(MethodTables(results, Scenarios))

	for _, c := range conds {
		fmt.Fprintf(&b, "### Calibração por nó sob o nulo: %s\n\n", c.Name)
		b.WriteString("Taxa observada de −Z > z(1−a) (silêncio) e Z > z(1−a) (burst) por nó-réplica do S0, e a razão contra o nominal a. Só nós com ≥ 1 aresta observada.\n\n")
		var labels []string
		var cals []Calibration
		for _, v := range DiscretenessVariants {
			v := v
			labels = append(labels, "z-2pois"+v.Suffix())
			cals = append(cals, NullCalibration(func(d Dataset) map[string]float64 { return EdgeZ(d, d.T0, v) },
				c.Config, opts.Replicas, opts.Workers))
		}
		b.WriteString(CalibrationMarkdown(labels, cals))
		b.WriteString("\n")
	}
	return b.String(), results
}
