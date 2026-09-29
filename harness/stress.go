package harness

import (
	"fmt"
	"strings"
)

// StressConditions returns the Tarefa 3 conditions, one axis at a time from
// base. The axes and values of the plan come first; the ones marked "extra"
// are not in the plan and were added to attack assumptions the plan's axes
// leave alone: a declining trend (the plan only has positive ones, which
// make silence conservative), a season whose period does not divide the
// before and after periods (P = 4 with t0 = 8 and T = 12 averages out by
// construction), and per-edge drift.
func StressConditions(base Config) []Condition {
	with := func(name string, f func(*Config)) Condition {
		c := base
		f(&c)
		return Condition{Name: name, Config: c}
	}
	return []Condition{
		with("base (Poisson, taxa constante)", func(*Config) {}),
		with("Dispersion k = 10", func(c *Config) { c.Dispersion = 10 }),
		with("Dispersion k = 3", func(c *Config) { c.Dispersion = 3 }),
		with("Dispersion k = 1", func(c *Config) { c.Dispersion = 1 }),
		with("Trend +0.05", func(c *Config) { c.Trend = 0.05 }),
		with("Trend +0.15", func(c *Config) { c.Trend = 0.15 }),
		with("Seasonality A = 0.3, P = 4", func(c *Config) { c.Seasonality = 0.3 }),
		with("RateMu = ln 0.5", func(c *Config) { c.RateMu = LowRateMu }),
		with("extra: Trend −0.05", func(c *Config) { c.Trend = -0.05 }),
		with("extra: Trend −0.15", func(c *Config) { c.Trend = -0.15 }),
		with("extra: Seasonality A = 0.3, P = 5", func(c *Config) { c.Seasonality, c.SeasonPeriod = 0.3, 5 }),
		with("extra: Drift σ = 0.05", func(c *Config) { c.Drift = 0.05 }),
		with("extra: Drift σ = 0.15", func(c *Config) { c.Drift = 0.15 }),
	}
}

// stressString describes the non-default generator options, or "".
func (c Config) stressString() string {
	var parts []string
	if c.RateMu != DefaultConfig().RateMu {
		parts = append(parts, fmt.Sprintf("RateMu = %.3f", c.RateMu))
	}
	if c.Dispersion != 0 {
		parts = append(parts, fmt.Sprintf("Dispersion k = %g", c.Dispersion))
	}
	if c.Trend != 0 {
		parts = append(parts, fmt.Sprintf("Trend = %g", c.Trend))
	}
	if c.Seasonality != 0 {
		p := c.SeasonPeriod
		if p == 0 {
			p = 4
		}
		parts = append(parts, fmt.Sprintf("Seasonality A = %g, P = %d", c.Seasonality, p))
	}
	if c.Drift != 0 {
		parts = append(parts, fmt.Sprintf("Drift σ = %g", c.Drift))
	}
	return strings.Join(parts, ", ")
}

// Stress runs DefaultMethods on S0..S5 under every StressConditions entry
// (Tarefa 3).
func Stress(opts Options) (string, []CondResult) {
	conds := StressConditions(opts.Config)
	results := Sweep(DefaultMethods(), conds, opts)

	var b strings.Builder
	fmt.Fprintf(&b, "Parâmetros: N = %d, T = %d, t0 = %d, R = %d réplicas por cenário e condição (sementes 1..%d), α = %g. Cada condição muda um eixo só.\n\n",
		opts.Config.N, opts.Config.T, opts.Config.T0, opts.Replicas, opts.Replicas, opts.Alpha)
	b.WriteString("FWER: réplicas do S0 com ≥ 1 alarme (IC 95% de Wilson), cada método com a própria regra. AUC: média ± desvio. Poder: fração das réplicas com o alvo em alarme.\n\n")
	b.WriteString(MethodTables(results, Scenarios))
	return b.String(), results
}
