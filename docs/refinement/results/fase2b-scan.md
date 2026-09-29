# Tarefa 6: t0 desconhecido (modo scan)

Gerado por `go run ./cmd/itt-harness -exp scan -r 100`.

Parâmetros: N = 500, T = 12, t0 real sorteado por réplica em [8, 11], R = 100 réplicas por cenário e condição (sementes 1..100), α = 0.05.

- `silence-z-2pois*` / `burst-z-2pois*`: t0 conhecido (usa o t0 real da réplica), limiar z(1 − α/N).
- `*-z-scan*`: t0 desconhecido, score = máximo sobre os splits s ∈ [T−4, T−1], limiar z(1 − α/(N·4)).

### Resumo: FWER no S0

S0:

| Condição | silence-z-2pois-midp | silence-z-scan-midp | burst-z-2pois-midp | burst-z-scan-midp | silence-z-2pois-midp-qhat-phibands | silence-z-scan-midp-qhat-phibands | burst-z-2pois-midp-qhat-phibands | burst-z-scan-midp-qhat-phibands |
|---|---|---|---|---|---|---|---|---|
| t0 sorteado em [T−4, T−1] | 0.030 [0.010, 0.085] | 0.030 [0.010, 0.085] | 0.030 [0.010, 0.085] | 0.050 [0.022, 0.112] | 0.020 [0.006, 0.070] | 0.030 [0.010, 0.085] | 0.040 [0.016, 0.098] | 0.020 [0.006, 0.070] |
| extra: t0 sorteado + Seasonality A = 0.3, P = 4 | 0.620 [0.522, 0.709] | 1.000 [0.963, 1.000] | 0.020 [0.006, 0.070] | 0.020 [0.006, 0.070] | 0.000 [0.000, 0.037] | 0.010 [0.002, 0.054] | 0.000 [0.000, 0.037] | 0.000 [0.000, 0.037] |

### Resumo: AUC e poder por cenário

S1:

| Condição | silence-z-2pois-midp | silence-z-scan-midp | burst-z-2pois-midp | burst-z-scan-midp | silence-z-2pois-midp-qhat-phibands | silence-z-scan-midp-qhat-phibands | burst-z-2pois-midp-qhat-phibands | burst-z-scan-midp-qhat-phibands |
|---|---|---|---|---|---|---|---|---|
| t0 sorteado em [T−4, T−1] | 1.000 ± 0.000 / 1.00 | 1.000 ± 0.000 / 1.00 | 0.000 ± 0.000 / 0.00 | 0.007 ± 0.029 / 0.00 | 1.000 ± 0.000 / 1.00 | 1.000 ± 0.000 / 1.00 | 0.000 ± 0.000 / 0.00 | 0.007 ± 0.030 / 0.00 |
| extra: t0 sorteado + Seasonality A = 0.3, P = 4 | 0.998 ± 0.006 / 1.00 | 0.998 ± 0.006 / 1.00 | 0.002 ± 0.006 / 0.00 | 0.047 ± 0.132 / 0.00 | 1.000 ± 0.000 / 0.94 | 1.000 ± 0.000 / 0.92 | 0.000 ± 0.000 / 0.00 | 0.028 ± 0.091 / 0.00 |

S2:

| Condição | silence-z-2pois-midp | silence-z-scan-midp | burst-z-2pois-midp | burst-z-scan-midp | silence-z-2pois-midp-qhat-phibands | silence-z-scan-midp-qhat-phibands | burst-z-2pois-midp-qhat-phibands | burst-z-scan-midp-qhat-phibands |
|---|---|---|---|---|---|---|---|---|
| t0 sorteado em [T−4, T−1] | 1.000 ± 0.002 / 0.89 | 0.999 ± 0.005 / 0.81 | 0.000 ± 0.002 / 0.00 | 0.025 ± 0.081 / 0.00 | 1.000 ± 0.002 / 0.89 | 0.999 ± 0.005 / 0.80 | 0.000 ± 0.002 / 0.00 | 0.024 ± 0.081 / 0.00 |
| extra: t0 sorteado + Seasonality A = 0.3, P = 4 | 0.988 ± 0.033 / 0.95 | 0.983 ± 0.038 / 0.90 | 0.012 ± 0.033 / 0.00 | 0.077 ± 0.176 / 0.00 | 0.999 ± 0.004 / 0.73 | 0.997 ± 0.014 / 0.66 | 0.001 ± 0.004 / 0.00 | 0.048 ± 0.133 / 0.00 |

S3:

| Condição | silence-z-2pois-midp | silence-z-scan-midp | burst-z-2pois-midp | burst-z-scan-midp | silence-z-2pois-midp-qhat-phibands | silence-z-scan-midp-qhat-phibands | burst-z-2pois-midp-qhat-phibands | burst-z-scan-midp-qhat-phibands |
|---|---|---|---|---|---|---|---|---|
| t0 sorteado em [T−4, T−1] | 0.998 ± 0.012 / 0.86 | 0.998 ± 0.010 / 0.76 | 0.002 ± 0.012 / 0.00 | 0.030 ± 0.089 / 0.00 | 0.998 ± 0.012 / 0.86 | 0.998 ± 0.010 / 0.76 | 0.002 ± 0.012 / 0.00 | 0.029 ± 0.088 / 0.00 |
| extra: t0 sorteado + Seasonality A = 0.3, P = 4 | 0.960 ± 0.112 / 0.86 | 0.954 ± 0.107 / 0.78 | 0.040 ± 0.112 / 0.00 | 0.076 ± 0.171 / 0.00 | 0.997 ± 0.018 / 0.69 | 0.995 ± 0.018 / 0.65 | 0.003 ± 0.018 / 0.00 | 0.049 ± 0.128 / 0.00 |

S5:

| Condição | silence-z-2pois-midp | silence-z-scan-midp | burst-z-2pois-midp | burst-z-scan-midp | silence-z-2pois-midp-qhat-phibands | silence-z-scan-midp-qhat-phibands | burst-z-2pois-midp-qhat-phibands | burst-z-scan-midp-qhat-phibands |
|---|---|---|---|---|---|---|---|---|
| t0 sorteado em [T−4, T−1] | 0.000 ± 0.000 / 0.00 | 0.001 ± 0.008 / 0.00 | 1.000 ± 0.000 / 0.99 | 1.000 ± 0.000 / 0.99 | 0.000 ± 0.000 / 0.00 | 0.001 ± 0.007 / 0.00 | 1.000 ± 0.000 / 0.99 | 1.000 ± 0.000 / 0.99 |
| extra: t0 sorteado + Seasonality A = 0.3, P = 4 | 0.000 ± 0.000 / 0.00 | 0.001 ± 0.004 / 0.00 | 1.000 ± 0.000 / 0.93 | 1.000 ± 0.002 / 0.93 | 0.000 ± 0.000 / 0.00 | 0.000 ± 0.002 / 0.00 | 1.000 ± 0.000 / 0.98 | 1.000 ± 0.000 / 0.96 |


## Leitura (escrita à mão; as tabelas acima são geradas)

**t0 desconhecido, dado Poisson.** O scan sobre os 4 splits com Bonferroni em N·4 mantém o FWER em 0.02–0.05. O custo em poder é de 8 a 10 pontos:

| Cenário | t0 conhecido | scan |
|---|---|---|
| S2 | 0.89 | 0.81 |
| S3 | 0.86 | 0.76 |
| S1 | 1.00 | 1.00 |

A AUC é praticamente a mesma. O poder menor que 1 no S2 e S3 já existe com t0 conhecido: quando o t0 sorteado é 11, só há 1 janela depois (ver Tarefa 5). O Bonferroni em N·4 é conservador, porque os splits compartilham janelas. Como o FWER já está ≤ α, não medi um limiar menos conservador.

**O que quebrou o M5 aqui (extra): sazonalidade P = 4 com t0 fora de fase.**
- **Com t0 = 8 fixo** (Tarefa 3), a onda some por construção.
- **Com t0 sorteado,** o M5 sem correção tem FWER **0.62** mesmo sabendo o t0, e **1.00 no scan**. O scan procura o máximo de −Z_s, então escolhe justamente o split em que o depois cai no vale da onda. **O scan não só herda o viés do q fixo, ele o maximiza.**
- **O q̂ global** (`CorrectedZConfig`) conserta: FWER 0.00–0.01. O custo é poder, S2 0.73 → 0.66 no scan. Aqui o φ̂ vê a oscilação nas janelas antes como sobredispersão e fica conservador.

**Conclusão:** o modo scan só é seguro com o q̂ ligado. Sem ele, qualquer componente periódico com período que não divida os splits vira alarme.
