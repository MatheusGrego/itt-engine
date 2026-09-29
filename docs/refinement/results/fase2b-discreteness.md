# Tarefa 2: discretude do M5/M6

Gerado por `go run ./cmd/itt-harness -exp discreteness -r 200`.

Parâmetros: N = 500, T = 12, t0 = 8, R = 200 réplicas por cenário e condição (sementes 1..200), α = 0.05.

Variantes:

- `silence-z-2pois` / `burst-z-2pois`: signed root
- `silence-z-2pois-midp` / `burst-z-2pois-midp`: mid-p exato
- `silence-z-2pois-nmin3` / `burst-z-2pois-nmin3`: signed root, arestas com n < 3 fora

### FWER, AUC e poder por variante

FWER: réplicas do S0 com ≥ 1 alarme (IC 95% de Wilson). AUC: média ± desvio. Poder: fração das réplicas com o alvo em alarme.

#### silence-z-2pois

| Condição | FWER S0 [IC 95%] | AUC S1 | AUC S2 | AUC S3 | AUC S4 | AUC S5 | poder S1 | poder S2 | poder S3 | poder S4 | poder S5 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| default (RateMu = ln 4) | 0.070 [0.042, 0.114] | 1.000 ± 0.000 | 1.000 ± 0.000 | 1.000 ± 0.000 | 0.521 ± 0.287 | 0.000 ± 0.000 | 1.00 | 1.00 | 0.99 | 0.00 | 0.00 |
| pouca contagem (RateMu = ln 0.5) | 0.130 [0.090, 0.184] | 1.000 ± 0.002 | 0.978 ± 0.064 | 0.979 ± 0.055 | 0.468 ± 0.303 | 0.001 ± 0.004 | 0.90 | 0.31 | 0.18 | 0.00 | 0.00 |

#### burst-z-2pois

| Condição | FWER S0 [IC 95%] | AUC S1 | AUC S2 | AUC S3 | AUC S4 | AUC S5 | poder S1 | poder S2 | poder S3 | poder S4 | poder S5 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| default (RateMu = ln 4) | 0.055 [0.031, 0.096] | 0.000 ± 0.000 | 0.000 ± 0.000 | 0.000 ± 0.000 | 0.479 ± 0.287 | 1.000 ± 0.000 | 0.00 | 0.00 | 0.00 | 0.00 | 1.00 |
| pouca contagem (RateMu = ln 0.5) | 0.055 [0.031, 0.096] | 0.000 ± 0.002 | 0.022 ± 0.064 | 0.021 ± 0.055 | 0.532 ± 0.303 | 0.999 ± 0.004 | 0.00 | 0.00 | 0.00 | 0.00 | 0.77 |

#### silence-z-2pois-midp

| Condição | FWER S0 [IC 95%] | AUC S1 | AUC S2 | AUC S3 | AUC S4 | AUC S5 | poder S1 | poder S2 | poder S3 | poder S4 | poder S5 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| default (RateMu = ln 4) | 0.035 [0.017, 0.070] | 1.000 ± 0.000 | 1.000 ± 0.000 | 1.000 ± 0.000 | 0.531 ± 0.288 | 0.000 ± 0.000 | 1.00 | 1.00 | 0.98 | 0.00 | 0.00 |
| pouca contagem (RateMu = ln 0.5) | 0.005 [0.001, 0.028] | 0.999 ± 0.004 | 0.975 ± 0.063 | 0.971 ± 0.061 | 0.500 ± 0.304 | 0.001 ± 0.002 | 0.49 | 0.09 | 0.07 | 0.00 | 0.00 |

#### burst-z-2pois-midp

| Condição | FWER S0 [IC 95%] | AUC S1 | AUC S2 | AUC S3 | AUC S4 | AUC S5 | poder S1 | poder S2 | poder S3 | poder S4 | poder S5 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| default (RateMu = ln 4) | 0.060 [0.035, 0.102] | 0.000 ± 0.000 | 0.000 ± 0.000 | 0.000 ± 0.000 | 0.469 ± 0.288 | 1.000 ± 0.000 | 0.00 | 0.00 | 0.00 | 0.00 | 1.00 |
| pouca contagem (RateMu = ln 0.5) | 0.010 [0.003, 0.036] | 0.001 ± 0.004 | 0.025 ± 0.063 | 0.029 ± 0.061 | 0.500 ± 0.304 | 0.999 ± 0.002 | 0.00 | 0.00 | 0.00 | 0.00 | 0.74 |

#### silence-z-2pois-nmin3

| Condição | FWER S0 [IC 95%] | AUC S1 | AUC S2 | AUC S3 | AUC S4 | AUC S5 | poder S1 | poder S2 | poder S3 | poder S4 | poder S5 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| default (RateMu = ln 4) | 0.070 [0.042, 0.114] | 1.000 ± 0.000 | 1.000 ± 0.000 | 1.000 ± 0.000 | 0.522 ± 0.288 | 0.000 ± 0.000 | 1.00 | 1.00 | 0.99 | 0.00 | 0.00 |
| pouca contagem (RateMu = ln 0.5) | 0.160 [0.116, 0.217] | 0.999 ± 0.006 | 0.964 ± 0.097 | 0.952 ± 0.134 | 0.471 ± 0.296 | 0.001 ± 0.002 | 0.84 | 0.31 | 0.20 | 0.00 | 0.00 |

#### burst-z-2pois-nmin3

| Condição | FWER S0 [IC 95%] | AUC S1 | AUC S2 | AUC S3 | AUC S4 | AUC S5 | poder S1 | poder S2 | poder S3 | poder S4 | poder S5 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| default (RateMu = ln 4) | 0.055 [0.031, 0.096] | 0.000 ± 0.000 | 0.000 ± 0.000 | 0.000 ± 0.000 | 0.478 ± 0.288 | 1.000 ± 0.000 | 0.00 | 0.00 | 0.00 | 0.00 | 1.00 |
| pouca contagem (RateMu = ln 0.5) | 0.055 [0.031, 0.096] | 0.001 ± 0.006 | 0.036 ± 0.097 | 0.048 ± 0.134 | 0.529 ± 0.296 | 0.999 ± 0.002 | 0.00 | 0.00 | 0.00 | 0.00 | 0.77 |

### Calibração por nó sob o nulo: default (RateMu = ln 4)

Taxa observada de −Z > z(1−a) (silêncio) e Z > z(1−a) (burst) por nó-réplica do S0, e a razão contra o nominal a. Só nós com ≥ 1 aresta observada.

| Variante | nós-réplica | média Z | var Z | silêncio 1e-02 | silêncio 1e-03 | silêncio 1e-04 | burst 1e-02 | burst 1e-03 | burst 1e-04 |
|---|---|---|---|---|---|---|---|---|---|
| z-2pois | 100000 | -0.056 | 1.022 | 0.012 (×1.22) | 0.0015 (×1.48) | 0.00014 (×1.40) | 0.0092 (×0.92) | 0.00095 (×0.95) | 0.00013 (×1.30) |
| z-2pois-midp | 100000 | +0.004 | 0.977 | 0.0092 (×0.92) | 0.00098 (×0.98) | 7e-05 (×0.70) | 0.0095 (×0.95) | 0.00095 (×0.95) | 0.00014 (×1.40) |
| z-2pois-nmin3 | 100000 | -0.056 | 1.022 | 0.012 (×1.22) | 0.0015 (×1.47) | 0.00014 (×1.40) | 0.0093 (×0.93) | 0.00094 (×0.94) | 0.00013 (×1.30) |

Por faixa de grau observado k_v (taxa por nó no nível 1e-03):

| Variante | k_v | nós-réplica | média Z | var Z | silêncio | burst |
|---|---|---|---|---|---|---|
| z-2pois | 1–3 | 16623 | -0.032 | 1.015 | 0.0014 (×1.44) | 0.0009 (×0.90) |
| z-2pois | 4–10 | 54974 | -0.054 | 1.018 | 0.0014 (×1.44) | 0.00087 (×0.87) |
| z-2pois | 11–30 | 24876 | -0.069 | 1.037 | 0.0016 (×1.61) | 0.0012 (×1.25) |
| z-2pois | ≥ 31 | 3527 | -0.121 | 1.005 | 0.0014 (×1.42) | 0.00028 (×0.28) |
| z-2pois-midp | 1–3 | 16623 | +0.002 | 0.969 | 0.0011 (×1.08) | 0.00084 (×0.84) |
| z-2pois-midp | 4–10 | 54974 | -0.001 | 0.974 | 0.00093 (×0.93) | 0.00087 (×0.87) |
| z-2pois-midp | 11–30 | 24876 | +0.014 | 0.992 | 0.001 (×1.05) | 0.0012 (×1.25) |
| z-2pois-midp | ≥ 31 | 3527 | +0.015 | 0.961 | 0.00085 (×0.85) | 0.00057 (×0.57) |
| z-2pois-nmin3 | 1–3 | 16623 | -0.032 | 1.014 | 0.0014 (×1.44) | 0.0009 (×0.90) |
| z-2pois-nmin3 | 4–10 | 54974 | -0.053 | 1.018 | 0.0014 (×1.42) | 0.00087 (×0.87) |
| z-2pois-nmin3 | 11–30 | 24876 | -0.068 | 1.037 | 0.0016 (×1.61) | 0.0012 (×1.21) |
| z-2pois-nmin3 | ≥ 31 | 3527 | -0.121 | 1.003 | 0.0014 (×1.42) | 0.00028 (×0.28) |

### Calibração por nó sob o nulo: pouca contagem (RateMu = ln 0.5)

Taxa observada de −Z > z(1−a) (silêncio) e Z > z(1−a) (burst) por nó-réplica do S0, e a razão contra o nominal a. Só nós com ≥ 1 aresta observada.

| Variante | nós-réplica | média Z | var Z | silêncio 1e-02 | silêncio 1e-03 | silêncio 1e-04 | burst 1e-02 | burst 1e-03 | burst 1e-04 |
|---|---|---|---|---|---|---|---|---|---|
| z-2pois | 99989 | -0.190 | 1.152 | 0.022 (×2.19) | 0.0029 (×2.88) | 0.00029 (×2.90) | 0.0097 (×0.97) | 0.0011 (×1.12) | 0.00011 (×1.10) |
| z-2pois-midp | 99989 | +0.036 | 0.813 | 0.0036 (×0.36) | 0.00018 (×0.18) | 1e-05 (×0.10) | 0.006 (×0.60) | 0.00038 (×0.38) | 2e-05 (×0.20) |
| z-2pois-nmin3 | 99989 | -0.149 | 1.123 | 0.02 (×2.01) | 0.0026 (×2.63) | 0.00033 (×3.30) | 0.01 (×1.01) | 0.001 (×1.04) | 0.00011 (×1.10) |

Por faixa de grau observado k_v (taxa por nó no nível 1e-03):

| Variante | k_v | nós-réplica | média Z | var Z | silêncio | burst |
|---|---|---|---|---|---|---|
| z-2pois | 1–3 | 18267 | -0.107 | 1.124 | 0.0014 (×1.42) | 0.0015 (×1.48) |
| z-2pois | 4–10 | 54843 | -0.167 | 1.152 | 0.0027 (×2.68) | 0.0011 (×1.09) |
| z-2pois | 11–30 | 23616 | -0.272 | 1.158 | 0.0041 (×4.11) | 0.00097 (×0.97) |
| z-2pois | ≥ 31 | 3263 | -0.451 | 1.114 | 0.0055 (×5.52) | 0.00061 (×0.61) |
| z-2pois-midp | 1–3 | 18267 | +0.021 | 0.798 | 0.00022 (×0.22) | 0.00038 (×0.38) |
| z-2pois-midp | 4–10 | 54843 | +0.033 | 0.816 | 0.00018 (×0.18) | 0.00038 (×0.38) |
| z-2pois-midp | 11–30 | 23616 | +0.048 | 0.820 | 0.00017 (×0.17) | 0.00034 (×0.34) |
| z-2pois-midp | ≥ 31 | 3263 | +0.073 | 0.790 | 0 (×0.00) | 0.00061 (×0.61) |
| z-2pois-nmin3 | 1–3 | 18267 | -0.077 | 1.083 | 0.0016 (×1.64) | 0.0015 (×1.48) |
| z-2pois-nmin3 | 4–10 | 54843 | -0.131 | 1.129 | 0.0025 (×2.46) | 0.0011 (×1.08) |
| z-2pois-nmin3 | 11–30 | 23616 | -0.216 | 1.128 | 0.0036 (×3.56) | 0.00068 (×0.68) |
| z-2pois-nmin3 | ≥ 31 | 3263 | -0.370 | 1.088 | 0.0043 (×4.29) | 0.00061 (×0.61) |


## Leitura e escolha (escrita à mão; as tabelas acima são geradas)

**Regra do plano:** FWER mais perto de α no S0 default, e a menor perda de AUC.

| Variante | \|FWER − α\| silêncio | \|FWER − α\| burst | média | perda de AUC (S1, S2, S3, S5) |
|---|---|---|---|---|
| signed root | 0.020 | 0.005 | 0.0125 | 0 |
| mid-p exato | 0.015 | 0.010 | 0.0125 | ≤ 0.001 |
| signed root, n ≥ 3 | 0.020 | 0.005 | 0.0125 | 0 |

- **Pela regra literal, deu empate.** Com R = 200, o IC do FWER tem ±0.03, e as três variantes ficam dentro do IC umas das outras. No default, quase nenhuma aresta tem n < 3, então o n_min não muda nada.
- **O desempate veio da calibração por nó**, que usa 10⁵ nós-réplica:
  - **signed root:** tem viés negativo por aresta. Média de Z −0.056, silêncio a 1e-3 em ×1.48 do nominal.
  - **O Stouffer amplifica esse viés com o grau.** Um viés b por aresta vira b·√k no Z. A média de Z vai de −0.03 (k ≤ 3) a −0.12 (k ≥ 31) no default, e a −0.45 nos hubs com pouca contagem.
  - **mid-p:** média ≈ 0 em todas as faixas de grau, silêncio em ×0.92 a ×0.98 do nominal.
- **Pouca contagem (RateMu = ln 0.5) quebra o signed root:**
  - FWER de silêncio 0.130 [0.090, 0.184], e a taxa por nó chega a ×5.5 nos hubs.
  - **O n_min = 3 piora** (0.160 [0.116, 0.217]): tirar as arestas com n = 1 e 2 não tira o viés das de n = 3 a 10, e ainda reduz o k.
  - **O mid-p segura** (0.005 silêncio, 0.010 burst), mas fica conservador: var(Z) = 0.81. O custo é poder (S1 0.49 contra 0.90 do signed root; S2 0.09 contra 0.31). O poder do signed root nessa condição vem junto com um FWER de 0.13, então não é comparável.
- **Escolha: mid-p exato é o default do M5/M6** (`DefaultZConfig`). O signed root (`ZConfig{}`) e o n_min (`ZConfig{MinN: 3}`) continuam como opções. Detalhes e o que fica aberto: Q6 em `docs/refinement/OPEN-QUESTIONS.md`.
- **O que quebrou o M5 aqui:** o próprio signed root em contagem baixa, por um viés pequeno por aresta que a normalização de Stouffer multiplica por √k. O protótipo passava no default porque as arestas têm n ≈ 48 em média.
