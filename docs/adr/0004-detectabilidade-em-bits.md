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

## Resultados (harness fase 2b)

Arquivos completos em `docs/refinement/results/fase2b-*.md`. O status continua **Proposto**. O limiar testado não foi o S/2 > ln(1/α) + ln N da decisão (que dá FWER 1.00, ver Q5), e sim **−Z > z(1 − α/N)** sobre o Z de Stouffer do M5, unilateral com Bonferroni.

- **Nulo puro (critério desta ADR, "fração acima do limiar ≈ α"):**
  - FWER 0.035 [0.017, 0.070] com o mid-p, R = 200.
  - Por nó, a taxa a 1e-3 fica em ×0.98 do nominal (10⁵ nós-réplica).
  - **Passa sob Poisson.** O signed root do protótipo passa por pouco no default (×1.48 por nó) e **falha com pouca contagem** (FWER 0.130), porque o viés por aresta vira b·√k no Stouffer (Q6).
- **Poder (Yharim empírico, Tarefa 5):**
  - 80% de poder quando Σ(√e − √o)² ≈ 10–13, o que dá 45 a 117 eventos esperados perdidos, conforme o grau e as janelas.
  - A previsão Poisson/Stouffer bate em ±0.10 com ≥ 2 janelas depois e é otimista em até 0.22 com 1 janela.
  - Afinamento de 10% nunca é detectado.
- **t0 desconhecido (Tarefa 6):** scan com Bonferroni em N·4 tem FWER 0.02–0.05 e custa de 8 a 10 pontos de poder. **Sem o q̂**, uma sazonalidade fora de fase leva o scan a FWER 1.00.
- **Consequência para a decisão:** o limiar vindo da teoria só é auditável se o nulo for Poisson estacionário. A consequência (−) desta ADR ("sobredispersão real faz a aproximação Poisson subestimar o ruído") foi medida: k = 10 dá FWER 0.54 sem φ̂. O fallback é obrigatório, não opcional.
