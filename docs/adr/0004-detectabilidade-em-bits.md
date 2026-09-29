# ADR-0004: Detectabilidade em bits de evidência (Yharim via Sanov)

- Status: Proposto
- Data: 2026-09-29
- Decisores: Matheus
- Problemas: D1, D2, X3

## Contexto

- **D1:** o `SNR()` do código é `mean/std·√n` das tensões, um t-stat contra zero. Como τ ≥ 0, ele cresce com √n e grafo grande sempre sai "StronglyDetectable".
- **D2:** o paper justifica Υ = √(2 ln 1/α) via Gumbel e "JSD limitada". Mas √(2 ln 1/α) é a cauda gaussiana (Chernoff), não o máximo de Gumbel (√(2 ln n)). E o máximo de variáveis limitadas cai no domínio Weibull, não no Gumbel.
- **X3:** o SNR do paper (d/σ_d·√C) e o do código são definições diferentes.

## Decisão

Detectabilidade passa a ser medida na mesma unidade do τ novo (ADR-0002), em evidência:

```
Pelo teorema de Sanov:   P(P̂ | Q) ≈ exp(−N·KL(P̂‖Q))
Desvio pequeno:          N·KL ≈ z²/2
Logo:  z > √(2 ln 1/α)   ⟺   N·KL > ln(1/α)
```

- **Limiar por nó:** S(v)/2 > ln(1/α) + ln(n), com a correção de n testes simultâneos. A alternativa exata é o quantil de χ² com os graus de liberdade = nº de arestas em déficit.
- **O Yharim fica**, reinterpretado como a "sombra gaussiana" do limite de evidência. A fórmula é a mesma, a justificativa é nova e correta.
- As regiões Undetectable / Weakly / Strongly são redefinidas pela evidência total da região vs ln(1/α), e não por `mean/std·√n`.
- O `SNR()` atual é removido ou rebaixado a descritor, com o nome real (`TStatVsZero`).

## Opções consideradas

1. **Manter o SNR e só corrigir a derivação no paper.** Não resolve D1.
2. **Calibração só por MAD.** Funciona, mas é empírica e exige warmup. Fica como fallback quando o nulo não for Poisson.
3. **Limiar em evidência (Sanov/χ²).** Escolhida.

## Consequências

- (+) O limiar sai da teoria e é auditável.
- (+) Unifica o SNR do paper com o do código.
- (−) Depende da ADR-0002 aceita.
- (−) Sobredispersão real faz a aproximação Poisson subestimar o ruído; precisa do fallback (binomial negativa ou MAD).

## Critério de aceite

Harness sob o nulo puro: a fração de nós acima do limiar ≈ α (±IC). No harness com plantados: power reportado por cenário.

## Relacionados

ADR-0002, ADR-0006.
