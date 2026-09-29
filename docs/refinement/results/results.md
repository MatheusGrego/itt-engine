# Resultados do harness

Gerado por `go run ./cmd/itt-harness`.

Parâmetros: N = 500, T = 12, t0 = 8, R = 20 réplicas (sementes 1..20), α = 0.05, k = 10.

### Ranking do alvo (AUC média ± desvio / hit@10)

| Cenário | tau-jsd | tau-jsd-cumulative | silence-deviance-v0 | burst-deviance-v0 |
|---|---|---|---|---|
| S1 removal | 0.074 ± 0.010 / 0.00 | 0.499 ± 0.217 / 0.00 | 1.000 ± 0.000 / 1.00 | 0.026 ± 0.005 / 0.00 |
| S2 thinning | 0.408 ± 0.186 / 0.00 | 0.526 ± 0.222 / 0.00 | 0.999 ± 0.001 / 1.00 | 0.026 ± 0.005 / 0.00 |
| S3 sniper | 0.074 ± 0.010 / 0.00 | 0.074 ± 0.010 / 0.00 | 0.992 ± 0.024 / 0.90 | 0.026 ± 0.005 / 0.00 |
| S4 hermit | 0.073 ± 0.010 / 0.00 | 0.073 ± 0.010 / 0.00 | 0.187 ± 0.207 / 0.00 | 0.230 ± 0.206 / 0.00 |
| S5 burst | 0.775 ± 0.233 / 0.20 | 0.674 ± 0.238 / 0.05 | 0.024 ± 0.007 / 0.00 | 1.000 ± 0.000 / 1.00 |

S4 é controle: o eremita não deveria subir no ranking, então AUC ≤ ~0.5 é o esperado.

### Falsos positivos sob o nulo (S0)

| Método | Regra de alarme | Nós em alarme (média) | FWER (réplicas com ≥ 1 alarme) |
|---|---|---|---|
| tau-jsd | τ > 0.2 | 1.74% | 1.00 |
| tau-jsd-cumulative | τ > 0.2 | 1.49% | 1.00 |
| silence-deviance-v0 | S/2 > ln(1/α) + ln N | 8.35% | 1.00 |
| burst-deviance-v0 | B/2 > ln(1/α) + ln N | 9.44% | 1.00 |

### Tempo por método (média de todos os cenários)

| Método | ms por réplica |
|---|---|
| tau-jsd | 22.01 |
| tau-jsd-cumulative | 22.37 |
| silence-deviance-v0 | 4.89 |
| burst-deviance-v0 | 4.76 |
