package harness

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// Markdown renders results as a report in Portuguese (docs are PT-BR): one
// ranking table (rows = scenario, columns = method, cells = AUC ± sd /
// hit@k), the S0 false-positive table and the time per method.
func Markdown(results []Result, opts Options) string {
	methods, byKey := index(results)
	var b strings.Builder

	cfg := opts.Config
	fmt.Fprintf(&b, "Parâmetros: N = %d, T = %d, t0 = %d, R = %d réplicas (sementes 1..%d), α = %g, k = %d.",
		cfg.N, cfg.T, cfg.T0, opts.Replicas, opts.Replicas, opts.Alpha, opts.TopK)
	if s := cfg.stressString(); s != "" {
		fmt.Fprintf(&b, " Stress: %s.", s)
	}
	b.WriteString("\n\n")

	fmt.Fprintf(&b, "### Ranking do alvo (AUC média ± desvio / hit@%d)\n\n", opts.TopK)
	b.WriteString("| Cenário |")
	for _, m := range methods {
		fmt.Fprintf(&b, " %s |", m)
	}
	b.WriteString("\n|---|")
	for range methods {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for _, s := range Scenarios {
		if s == S0Null {
			continue
		}
		fmt.Fprintf(&b, "| %s %s |", s, s.Name())
		for _, m := range methods {
			r := byKey[[2]string{string(s), m}]
			fmt.Fprintf(&b, " %.3f ± %.3f / %.2f |", r.AUCMean, r.AUCSD, r.HitAtK)
		}
		b.WriteString("\n")
	}
	b.WriteString("\nS4 é controle: o eremita não deveria subir no ranking, então AUC ≤ ~0.5 é o esperado.\n\n")

	b.WriteString("### Falsos positivos sob o nulo (S0)\n\n")
	b.WriteString("| Método | Regra de alarme | Nós em alarme (média) | FWER (réplicas com ≥ 1 alarme) | IC 95% do FWER (Wilson) |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, m := range methods {
		r := byKey[[2]string{string(S0Null), m}]
		if r.AlarmRule == "" {
			fmt.Fprintf(&b, "| %s | n/a | n/a | n/a | n/a |\n", m)
			continue
		}
		lo, hi := r.FWERInterval()
		fmt.Fprintf(&b, "| %s | %s | %.3f%% | %.3f | [%.3f, %.3f] |\n", m, r.AlarmRule, 100*r.AlarmRate, r.FWER, lo, hi)
	}

	b.WriteString("\n### Tempo por método (média de todos os cenários)\n\n")
	b.WriteString("| Método | ms por réplica |\n|---|---|\n")
	for _, m := range methods {
		total := 0.0
		for _, s := range Scenarios {
			total += byKey[[2]string{string(s), m}].MillisPerReplica
		}
		fmt.Fprintf(&b, "| %s | %.2f |\n", m, total/float64(len(Scenarios)))
	}
	return b.String()
}

// WriteCSV writes one row per (scenario, method); NaN fields are left empty.
func WriteCSV(w io.Writer, results []Result) error {
	cw := csv.NewWriter(w)
	header := []string{"scenario", "scenario_name", "method", "replicas", "auc_mean", "auc_sd",
		"hit_at_k", "alarm_rule", "alarm_rate", "fwer", "ms_per_replica"}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range results {
		row := []string{
			string(r.Scenario), r.Scenario.Name(), r.Method, strconv.Itoa(r.Replicas),
			num(r.AUCMean), num(r.AUCSD), num(r.HitAtK),
			r.AlarmRule, num(r.AlarmRate), num(r.FWER), num(r.MillisPerReplica),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func num(x float64) string {
	if math.IsNaN(x) {
		return ""
	}
	return strconv.FormatFloat(x, 'f', 4, 64)
}

// index returns method names in first-seen order and results keyed by
// (scenario, method).
func index(results []Result) ([]string, map[[2]string]Result) {
	var methods []string
	seen := map[string]bool{}
	byKey := make(map[[2]string]Result, len(results))
	for _, r := range results {
		if !seen[r.Method] {
			seen[r.Method] = true
			methods = append(methods, r.Method)
		}
		byKey[[2]string{string(r.Scenario), r.Method}] = r
	}
	return methods, byKey
}
