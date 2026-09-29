# Resultados do harness

Gerado por `go run ./cmd/itt-harness -r 200 -name fase2b-baseline`.

Parâmetros: N = 500, T = 12, t0 = 8, R = 200 réplicas (sementes 1..200), α = 0.05, k = 10.

### Ranking do alvo (AUC média ± desvio / hit@10)

| Cenário | tau-jsd | tau-jsd-cumulative | silence-deviance-v0 | burst-deviance-v0 | silence-z-2pois | burst-z-2pois |
|---|---|---|---|---|---|---|
| S1 removal | 0.071 ± 0.010 / 0.00 | 0.473 ± 0.224 / 0.01 | 1.000 ± 0.000 / 1.00 | 0.025 ± 0.005 / 0.00 | 1.000 ± 0.000 / 1.00 | 0.000 ± 0.000 / 0.00 |
| S2 thinning | 0.373 ± 0.191 / 0.00 | 0.498 ± 0.233 / 0.01 | 0.999 ± 0.002 / 1.00 | 0.027 ± 0.015 / 0.00 | 1.000 ± 0.000 / 1.00 | 0.000 ± 0.000 / 0.00 |
| S3 sniper | 0.071 ± 0.010 / 0.00 | 0.070 ± 0.010 / 0.00 | 0.997 ± 0.011 / 0.96 | 0.025 ± 0.005 / 0.00 | 1.000 ± 0.000 / 1.00 | 0.000 ± 0.000 / 0.00 |
| S4 hermit | 0.070 ± 0.010 / 0.00 | 0.069 ± 0.010 / 0.00 | 0.208 ± 0.191 / 0.00 | 0.188 ± 0.196 / 0.00 | 0.521 ± 0.287 / 0.02 | 0.479 ± 0.287 / 0.03 |
| S5 burst | 0.735 ± 0.272 / 0.14 | 0.639 ± 0.264 / 0.05 | 0.023 ± 0.008 / 0.00 | 1.000 ± 0.001 / 1.00 | 0.000 ± 0.000 / 0.00 | 1.000 ± 0.000 / 1.00 |

S4 é controle: o eremita não deveria subir no ranking, então AUC ≤ ~0.5 é o esperado.

### Falsos positivos sob o nulo (S0)

| Método | Regra de alarme | Nós em alarme (média) | FWER (réplicas com ≥ 1 alarme) | IC 95% do FWER (Wilson) |
|---|---|---|---|---|
| tau-jsd | τ > 0.2 | 1.744% | 1.000 | [0.981, 1.000] |
| tau-jsd-cumulative | τ > 0.2 | 1.474% | 1.000 | [0.981, 1.000] |
| silence-deviance-v0 | S/2 > ln(1/α) + ln N | 8.313% | 1.000 | [0.981, 1.000] |
| burst-deviance-v0 | B/2 > ln(1/α) + ln N | 9.798% | 1.000 | [0.981, 1.000] |
| silence-z-2pois | −Z > z(1−α/N) | 0.014% | 0.070 | [0.042, 0.114] |
| burst-z-2pois | Z > z(1−α/N) | 0.013% | 0.055 | [0.031, 0.096] |

### Tempo por método (média de todos os cenários)

| Método | ms por réplica |
|---|---|
| tau-jsd | 23.14 |
| tau-jsd-cumulative | 24.31 |
| silence-deviance-v0 | 5.14 |
| burst-deviance-v0 | 5.04 |
| silence-z-2pois | 6.48 |
| burst-z-2pois | 6.51 |
