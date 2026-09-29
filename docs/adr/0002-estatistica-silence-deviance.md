# ADR-0002: Estatística de tensão: desvio de Poisson com sinal (silence deviance)

- Status: Proposto
- Data: 2026-09-29
- Decisores: Matheus
- Problemas: T2, T3, T5

## Contexto

- **T2:** no leave-one-out, JSD(P, P∖v) = f(p), com `f(p) = ½[p + (1−p)·log₂(2(1−p)/(2−p)) + log₂(2/(2−p))]`. Hellinger dá `√(1−√(1−p))`. Qualquer f-divergência dá o mesmo ranking. Verificado: 4 distribuições diferentes com p = 0.3 → JSD = 0.169195 em todas.
- **T3:** a JSD sobre vetores normalizados ignora o volume. 1 evento e 10.000 eventos na mesma proporção dão τ = 0.3113.
- **T5:** a média por vizinho dilui o sinal com vizinhos que não apontam para v.
- A JSD é simétrica, mas a tese é assimétrica (falta ≠ excesso).

## Decisão

Para cada aresta e (v↔n) com observado `o` na janela atual e esperado `e` vindo do nulo (ADR-0003):

```
d(e) = 2 [ o·ln(o/e) − (o − e) ]      (se o = 0: d = 2e)
S(v) = Σ d(e)  onde o < e     → silêncio (ausência)
B(v) = Σ d(e)  onde o > e     → burst (excesso)
τ(v) := S(v)
```

- Soma por aresta, não média por vizinho (resolve T5).
- Em unidades de log-verossimilhança (nats, ou bits se dividir por ln 2). Sob o nulo, S + B ≈ χ² → o limiar vem da teoria (ADR-0004).
- Aditivo por aresta → atualização O(1) por evento (ajuda a ADR-0005).
- Peso que não é contagem, ou dados sobredispersos: trocar Poisson por binomial negativa (parâmetro de dispersão estimado no baseline). Isso fica como extensão, não como v0.
- A JSD permanece só como descritor opcional de "forma" (limitado em [0,1]), sem papel na decisão de anomalia.

## Opções consideradas

1. **JSD sobre o nulo real (sem leave-one-out).** Resolve T2, mas não resolve T3 nem a simetria.
2. **G-test / N·KL(P̂‖Q) sobre a distribuição do nó.** Leva o volume em conta, mas mistura falta e excesso num número só.
3. **Bayesian surprise com Dirichlet.** Elegante e incremental, mas mais difícil de calibrar e explicar. Fica como alternativa se a v0 perder no harness.
4. **Desvio de Poisson com sinal.** Escolhida: leva o volume em conta, aguenta zero, é direcional, aditiva, e tem distribuição nula conhecida.

## Consequências

- (+) τ passa a ter unidade (evidência) e significância.
- (+) Base limpa para Ω (concealment) e para a difusão: basta trocar τ por S.
- (−) Depende de um nulo decente; nulo ruim = τ ruim (ADR-0003).
- (−) Invalida o kernel GPU atual (F4).

## Critério de aceite

Harness: AUC(S) > AUC(τ atual) em S1, S2 e S3; em S5 (burst), S não dispara e B dispara; FP sob o nulo ≤ α.

## Relacionados

ADR-0001, ADR-0003, ADR-0004, ADR-0006.

## Resultados (harness 2026-09-29)

Comando: `go run ./cmd/itt-harness` (N = 500, T = 12, t0 = 8, 20 réplicas com sementes 1..20, α = 0.05). Células: AUC média ± desvio / hit@10. Arquivos completos em `docs/refinement/results/`. O status desta ADR **não muda** com isto: só o Matheus aceita ou rejeita.

| Cenário | tau-jsd (M1) | tau-jsd-cumulative (M2) | silence-deviance-v0 (M3) | burst-deviance-v0 (M4) |
|---|---|---|---|---|
| S1 remoção | 0.074 ± 0.010 / 0.00 | 0.499 ± 0.217 / 0.00 | 1.000 ± 0.000 / 1.00 | 0.026 ± 0.005 / 0.00 |
| S2 afinamento | 0.408 ± 0.186 / 0.00 | 0.526 ± 0.222 / 0.00 | 0.999 ± 0.001 / 1.00 | 0.026 ± 0.005 / 0.00 |
| S3 sniper | 0.074 ± 0.010 / 0.00 | 0.074 ± 0.010 / 0.00 | 0.992 ± 0.024 / 0.90 | 0.026 ± 0.005 / 0.00 |
| S4 eremita (controle) | 0.073 ± 0.010 / 0.00 | 0.073 ± 0.010 / 0.00 | 0.187 ± 0.207 / 0.00 | 0.230 ± 0.206 / 0.00 |
| S5 burst | 0.775 ± 0.233 / 0.20 | 0.674 ± 0.238 / 0.05 | 0.024 ± 0.007 / 0.00 | 1.000 ± 0.000 / 1.00 |

Falsos positivos sob o nulo (S0), cada método com a própria regra de alarme:

| Método | Regra | Nós em alarme (média) | FWER |
|---|---|---|---|
| tau-jsd | τ > 0.2 | 1.74% | 1.00 |
| tau-jsd-cumulative | τ > 0.2 | 1.49% | 1.00 |
| silence-deviance-v0 | S/2 > ln(1/α) + ln N | 8.35% | 1.00 |
| burst-deviance-v0 | B/2 > ln(1/α) + ln N | 9.44% | 1.00 |

- **Ranking: o S ganhou onde o critério pede.** AUC do M3 contra o τ atual: S1 1.000 contra 0.074 (M1) e 0.499 (M2); S2 0.999 contra 0.408 e 0.526; S3 0.992 contra 0.074 e 0.074. Em S5 o S não sobe (0.024) e o B vai a 1.000, como a decisão prevê.
- **FP sob o nulo: o critério não passou.** Com a regra S/2 > ln(1/α) + ln N, 8.35% dos nós alarmam por réplica e as 20 réplicas têm pelo menos um alarme (FWER 1.00 contra α = 0.05). Medido: o desvio médio por aresta sob o nulo é 1.54, não 1, porque o esperado vem de só 8 janelas; e S(v) é uma soma sobre arestas, então cresce com o grau (grau médio 28.2 nos nós em alarme contra 9.4 no geral; entre os nós de grau ≤ 7, a taxa cai para 0.60%). O limiar não conta graus de liberdade nem o ruído do baseline. Isso fica para a ADR-0004 (ver Q5 em `docs/refinement/OPEN-QUESTIONS.md`).
- **Surpresa:** o τ atual com os pesos do período depois (M1) reage mais ao burst (S5: 0.775) do que a qualquer supressão, e o M2, que acumula tudo como a engine faz hoje, fica no nível do chute em S1 e S2 (~0.5).
