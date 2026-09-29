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
