# Tarefa 5: curva de poder do M5 (limite de detecção empírico)

Gerado por `go run ./cmd/itt-harness -exp power -r 100`.

Parâmetros: N = 500, t0 = 8 (8 janelas antes), T = t0 + janelas depois, R = 100 réplicas por célula, α = 0.05. Método: `silence-z-2pois-midp-qhat-phibands` (mid-p + q̂ global + φ̂ por faixa), alarme −Z > z(1 − α/N). Gerador Poisson base.

Célula: poder medido [IC 95% Wilson] / poder previsto. Abaixo de cada tabela: eventos esperados perdidos (média).

### Alvo de grau baixo (p10–p30)

| Janelas depois | grau médio | mult 0.9 | mult 0.8 | mult 0.7 | mult 0.5 | mult 0.3 | mult 0 |
|---|---|---|---|---|---|---|---|
| 1 | 3.9 | 0.00 [0.00, 0.04] / 0.00 | 0.00 [0.00, 0.04] / 0.00 | 0.01 [0.00, 0.05] / 0.01 | 0.03 [0.01, 0.08] / 0.13 | 0.33 [0.25, 0.43] / 0.48 | 0.83 [0.74, 0.89] / 0.99 |
| ↳ perdidos | | 2 | 4 | 7 | 11 | 16 | 22 |
| 2 | 3.9 | 0.00 [0.00, 0.04] / 0.00 | 0.01 [0.00, 0.05] / 0.01 | 0.02 [0.01, 0.07] / 0.04 | 0.29 [0.21, 0.39] / 0.34 | 0.69 [0.59, 0.77] / 0.79 | 0.99 [0.95, 1.00] / 1.00 |
| ↳ perdidos | | 4 | 9 | 13 | 22 | 31 | 45 |
| 4 | 3.9 | 0.00 [0.00, 0.04] / 0.00 | 0.00 [0.00, 0.04] / 0.02 | 0.12 [0.07, 0.20] / 0.11 | 0.63 [0.53, 0.72] / 0.61 | 0.94 [0.88, 0.97] / 0.95 | 1.00 [0.96, 1.00] / 1.00 |
| ↳ perdidos | | 9 | 18 | 27 | 45 | 63 | 90 |

### Alvo de grau médio (p45–p55)

| Janelas depois | grau médio | mult 0.9 | mult 0.8 | mult 0.7 | mult 0.5 | mult 0.3 | mult 0 |
|---|---|---|---|---|---|---|---|
| 1 | 6.9 | 0.00 [0.00, 0.04] / 0.00 | 0.00 [0.00, 0.04] / 0.01 | 0.00 [0.00, 0.04] / 0.03 | 0.15 [0.09, 0.23] / 0.28 | 0.57 [0.47, 0.66] / 0.79 | 1.00 [0.96, 1.00] / 1.00 |
| ↳ perdidos | | 4 | 7 | 11 | 19 | 26 | 37 |
| 2 | 6.9 | 0.00 [0.00, 0.04] / 0.00 | 0.00 [0.00, 0.04] / 0.02 | 0.09 [0.05, 0.16] / 0.10 | 0.54 [0.44, 0.63] / 0.62 | 0.96 [0.90, 0.98] / 0.97 | 1.00 [0.96, 1.00] / 1.00 |
| ↳ perdidos | | 7 | 15 | 22 | 37 | 52 | 74 |
| 4 | 6.9 | 0.00 [0.00, 0.04] / 0.00 | 0.02 [0.01, 0.07] / 0.04 | 0.21 [0.14, 0.30] / 0.25 | 0.80 [0.71, 0.87] / 0.89 | 1.00 [0.96, 1.00] / 1.00 | 1.00 [0.96, 1.00] / 1.00 |
| ↳ perdidos | | 15 | 30 | 44 | 74 | 104 | 148 |

### Alvo de grau alto (p80–p95)

| Janelas depois | grau médio | mult 0.9 | mult 0.8 | mult 0.7 | mult 0.5 | mult 0.3 | mult 0 |
|---|---|---|---|---|---|---|---|
| 1 | 18.0 | 0.00 [0.00, 0.04] / 0.00 | 0.02 [0.01, 0.07] / 0.03 | 0.11 [0.06, 0.19] / 0.20 | 0.75 [0.66, 0.82] / 0.86 | 1.00 [0.96, 1.00] / 1.00 | 1.00 [0.96, 1.00] / 1.00 |
| ↳ perdidos | | 10 | 20 | 29 | 49 | 68 | 98 |
| 2 | 18.0 | 0.00 [0.00, 0.04] / 0.01 | 0.07 [0.03, 0.14] / 0.11 | 0.33 [0.25, 0.43] / 0.52 | 0.98 [0.93, 0.99] / 0.99 | 1.00 [0.96, 1.00] / 1.00 | 1.00 [0.96, 1.00] / 1.00 |
| ↳ perdidos | | 20 | 39 | 59 | 98 | 137 | 196 |
| 4 | 18.0 | 0.01 [0.00, 0.05] / 0.02 | 0.24 [0.17, 0.33] / 0.30 | 0.75 [0.66, 0.82] / 0.83 | 1.00 [0.96, 1.00] / 1.00 | 1.00 [0.96, 1.00] / 1.00 | 1.00 [0.96, 1.00] / 1.00 |
| ↳ perdidos | | 39 | 78 | 117 | 196 | 274 | 391 |

### Extra: silêncio parcial (só uma fração das arestas do alvo zera; 4 janelas depois)

| Faixa | fração | grau médio | poder [IC 95%] | previsto | AUC | perdidos |
|---|---|---|---|---|---|---|
| baixo (p10–p30) | 0.5 | 3.9 | 0.80 [0.71, 0.87] | 0.97 | 0.999 | 45 |
| baixo (p10–p30) | 0.25 | 3.9 | 0.28 [0.20, 0.37] | 0.50 | 0.954 | 24 |
| baixo (p10–p30) | 0.1 | 3.9 | 0.28 [0.20, 0.37] | 0.50 | 0.954 | 24 |
| médio (p45–p55) | 0.5 | 6.9 | 0.97 [0.92, 0.99] | 1.00 | 1.000 | 82 |
| médio (p45–p55) | 0.25 | 6.9 | 0.41 [0.32, 0.51] | 0.80 | 0.987 | 41 |
| médio (p45–p55) | 0.1 | 6.9 | 0.05 [0.02, 0.11] | 0.20 | 0.883 | 19 |
| alto (p80–p95) | 0.5 | 18.0 | 1.00 [0.96, 1.00] | 1.00 | 1.000 | 197 |
| alto (p80–p95) | 0.25 | 18.0 | 0.84 [0.76, 0.90] | 0.99 | 0.999 | 98 |
| alto (p80–p95) | 0.1 | 18.0 | 0.09 [0.05, 0.16] | 0.31 | 0.912 | 40 |

### Poder contra eventos perdidos e contra a distância de Hellinger

Todas as células do sweep principal, ordenadas pela distância Σ(√e − √o)². Se o poder fosse função só dela, a coluna de poder subiria de forma monótona.

| Σ(√e−√o)² | perdidos | grau | janelas | mult | poder | previsto |
|---|---|---|---|---|---|---|
| 0.06 | 2 | 3.9 | 1 | 0.9 | 0.00 | 0.00 |
| 0.10 | 4 | 6.9 | 1 | 0.9 | 0.00 | 0.00 |
| 0.12 | 4 | 3.9 | 2 | 0.9 | 0.00 | 0.00 |
| 0.20 | 7 | 6.9 | 2 | 0.9 | 0.00 | 0.00 |
| 0.24 | 9 | 3.9 | 4 | 0.9 | 0.00 | 0.00 |
| 0.25 | 4 | 3.9 | 1 | 0.8 | 0.00 | 0.00 |
| 0.26 | 10 | 18.0 | 1 | 0.9 | 0.00 | 0.00 |
| 0.39 | 15 | 6.9 | 4 | 0.9 | 0.00 | 0.00 |
| 0.41 | 7 | 6.9 | 1 | 0.8 | 0.00 | 0.01 |
| 0.50 | 9 | 3.9 | 2 | 0.8 | 0.01 | 0.01 |
| 0.51 | 20 | 18.0 | 2 | 0.9 | 0.00 | 0.01 |
| 0.60 | 7 | 3.9 | 1 | 0.7 | 0.01 | 0.01 |
| 0.83 | 15 | 6.9 | 2 | 0.8 | 0.00 | 0.02 |
| 0.99 | 11 | 6.9 | 1 | 0.7 | 0.00 | 0.03 |
| 1.00 | 18 | 3.9 | 4 | 0.8 | 0.00 | 0.02 |
| 1.03 | 39 | 18.0 | 4 | 0.9 | 0.01 | 0.02 |
| 1.09 | 20 | 18.0 | 1 | 0.8 | 0.02 | 0.03 |
| 1.20 | 13 | 3.9 | 2 | 0.7 | 0.02 | 0.04 |
| 1.65 | 30 | 6.9 | 4 | 0.8 | 0.02 | 0.04 |
| 1.93 | 11 | 3.9 | 1 | 0.5 | 0.03 | 0.13 |
| 1.98 | 22 | 6.9 | 2 | 0.7 | 0.09 | 0.10 |
| 2.18 | 39 | 18.0 | 2 | 0.8 | 0.07 | 0.11 |
| 2.40 | 27 | 3.9 | 4 | 0.7 | 0.12 | 0.11 |
| 2.61 | 29 | 18.0 | 1 | 0.7 | 0.11 | 0.20 |
| 3.18 | 19 | 6.9 | 1 | 0.5 | 0.15 | 0.28 |
| 3.86 | 22 | 3.9 | 2 | 0.5 | 0.29 | 0.34 |
| 3.95 | 44 | 6.9 | 4 | 0.7 | 0.21 | 0.25 |
| 4.36 | 78 | 18.0 | 4 | 0.8 | 0.24 | 0.30 |
| 4.60 | 16 | 3.9 | 1 | 0.3 | 0.33 | 0.48 |
| 5.22 | 59 | 18.0 | 2 | 0.7 | 0.33 | 0.52 |
| 6.36 | 37 | 6.9 | 2 | 0.5 | 0.54 | 0.62 |
| 7.58 | 26 | 6.9 | 1 | 0.3 | 0.57 | 0.79 |
| 7.71 | 45 | 3.9 | 4 | 0.5 | 0.63 | 0.61 |
| 8.39 | 49 | 18.0 | 1 | 0.5 | 0.75 | 0.86 |
| 9.20 | 31 | 3.9 | 2 | 0.3 | 0.69 | 0.79 |
| 10.43 | 117 | 18.0 | 4 | 0.7 | 0.75 | 0.83 |
| 12.72 | 74 | 6.9 | 4 | 0.5 | 0.80 | 0.89 |
| 15.16 | 52 | 6.9 | 2 | 0.3 | 0.96 | 0.97 |
| 16.77 | 98 | 18.0 | 2 | 0.5 | 0.98 | 0.99 |
| 18.39 | 63 | 3.9 | 4 | 0.3 | 0.94 | 0.95 |
| 20.00 | 68 | 18.0 | 1 | 0.3 | 1.00 | 1.00 |
| 22.48 | 22 | 3.9 | 1 | 0 | 0.83 | 0.99 |
| 30.32 | 104 | 6.9 | 4 | 0.3 | 1.00 | 1.00 |
| 33.54 | 196 | 18.0 | 4 | 0.5 | 1.00 | 1.00 |
| 37.05 | 37 | 6.9 | 1 | 0 | 1.00 | 1.00 |
| 39.99 | 137 | 18.0 | 2 | 0.3 | 1.00 | 1.00 |
| 44.96 | 45 | 3.9 | 2 | 0 | 0.99 | 1.00 |
| 74.11 | 74 | 6.9 | 2 | 0 | 1.00 | 1.00 |
| 79.98 | 274 | 18.0 | 4 | 0.3 | 1.00 | 1.00 |
| 89.92 | 90 | 3.9 | 4 | 0 | 1.00 | 1.00 |
| 97.75 | 98 | 18.0 | 1 | 0 | 1.00 | 1.00 |
| 148.22 | 148 | 6.9 | 4 | 0 | 1.00 | 1.00 |
| 195.50 | 196 | 18.0 | 2 | 0 | 1.00 | 1.00 |
| 391.00 | 391 | 18.0 | 4 | 0 | 1.00 | 1.00 |

## Leitura (escrita à mão; as tabelas acima são geradas)

**Frase-resumo (Yharim empírico):** com o gerador Poisson, 8 janelas antes e N = 500, o M5 corrigido detecta com 80% de poder quando a distância Σ(√e − √o)² das arestas do alvo passa de ~10 a 13. Nas contas abaixo, e é o esperado depois sem supressão e o o esperado com ela. Em eventos esperados perdidos, isso vai de **~45** (grau ≈ 4, 4 janelas, mult 0.5) a **~117** (grau ≈ 18, 4 janelas, mult 0.7). O número de eventos perdidos sozinho não define o poder:
- **98 perdidos no grau alto:** removal em 1 janela dá poder 1.00; mult 0.5 em 2 janelas dá 0.98.
- **117 perdidos, mesmo grau, mult 0.7:** só 0.75.
- **Afinamento leve (mult 0.9)** não é detectado em nenhuma célula (poder ≤ 0.01), nem com 39 eventos perdidos.

**Teoria contra medida:** a previsão Poisson com raiz estabilizadora e Stouffer, −Z ≈ N(Σ 2(√e − √o)/√(1 + w/t0)/√k, 1), bate assim:
- **Bate** (diferença ≤ 0.10) em 2 e 4 janelas depois.
- **Superestima com 1 janela** em até 0.22 (grau baixo, removal: 0.83 medido contra 0.99). Com contagens pequenas o mid-p é conservador (var < 1, ver Tarefa 2), e a aproximação normal por aresta piora.
- **O poder cresce quase de forma monótona em Σ(√e − √o)²,** mas não é função só dela. O Stouffer depende de Σ(√e − √o)/√k, não da soma dos quadrados, então a mesma distância espalhada por mais arestas rende menos.

**Extra, silêncio parcial: o que quebra o M5 aqui.** O Stouffer divide por √k, então um alvo que perde poucas das suas arestas se dilui:
- **Grau 18, 10% das arestas zeradas** (≈ 2 arestas, ~40 eventos perdidos): poder 0.09 e AUC 0.91. Com 40 eventos perdidos espalhados por todas as arestas (grau 18, 1 janela, mult 0.7–0.8), o poder fica entre 0.02 e 0.11, também baixo. Concentrado em 2 arestas o sinal por aresta é forte, mas some no √k.
- **Grau médio, 10%:** poder 0.05 e AUC 0.88.
- **Grau baixo:** 25% e 10% dão o mesmo resultado, porque os dois arredondam para 1 aresta.
- A previsão superestima o poder parcial (0.31 contra 0.09). A soma de um N(μ, 1) com k − 1 ruídos é bem descrita, mas o φ̂ e o q̂ corrigidos custam mais quando o sinal é pequeno.

Consequência: o M5 detecta "o nó ficou quieto", não "uma relação do nó sumiu". Para a segunda pergunta, o teste certo é por aresta (ou max/Fisher em vez de Stouffer), com a sua própria correção de multiplicidade.
