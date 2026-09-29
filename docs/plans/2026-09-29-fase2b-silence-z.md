# Plano: Fase 2b, silêncio z calibrado + testes anti-viés + primeiro caso real

- Branch: `refinement/fase2b-silence-z`. Ela já contém a Fase 0/1 (PR #2) e o protótipo `harness/stouffer.go`.
- PR: contra `master`. Se o PR #2 já tiver sido mergeado, o diff mostra só o que é novo.
- Referências:
  - ADR-0002, ADR-0003, ADR-0004, ADR-0006;
  - `docs/refinement/OPEN-QUESTIONS.md` (Q4, Q5);
  - `docs/refinement/RUN-2026-09-29.md`.

## Contexto (leia antes)

O PR #2 mostrou que o silence deviance v0 (M3) ranqueia bem, mas falha no falso positivo: FWER 1.00. As duas causas medidas:
- o esperado é estimado de 8 janelas e tratado como exato (fator ~1.5);
- S(v) soma arestas, então cresce com o grau.

O protótipo M5/M6 (`harness/stouffer.go`) ataca as duas.

**Por aresta**, teste de razão de verossimilhança entre dois Poisson (antes × depois). Ele condiciona no total da aresta, então **não estima baseline**, e aresta nova entra naturalmente (resolve o Q4):

```
a = soma antes, o = soma depois, n = a + o, q = (T - T0) / T
ô = n q,  â = n (1 - q)
G = 2 [a ln(a/â) + o ln(o/ô)]      (~ χ²₁ sob H0)
r = sign(o - ô) · sqrt(G)          (~ N(0,1) sob H0)
```

**Por nó**, Stouffer: `Z(v) = Σ r / sqrt(k_v)`. Isso normaliza pelo grau.

- Silêncio = −Z, burst = +Z.
- Alarme: score > z(1 − α/N), unilateral com Bonferroni.

Resultado do revisor com 200 réplicas: AUC 1.00 em S1/S2/S3 (hit@10 = 1.00), FWER 0.07 (silêncio) e 0.06 (burst) contra α = 0.05.

**Viés conhecido:** o gerador é Poisson puro, com taxa constante no tempo e t0 conhecido. São as mesmas hipóteses do método, então esse resultado é otimista por construção. **O objetivo desta fase é tentar quebrar o M5**, não fazê-lo passar.

Regras extras desta fase:
- **Não ajustar nada olhando um caso real específico.** Toda escolha de parâmetro sai do harness sintético. O caso real (Tarefa 8) é só teste.
- Dados baixados ficam fora do git (`.cache/`, que deve ir no `.gitignore`). Só entram no git agregados pequenos (< 1 MB).
- Toda tabela de resultado reporta a incerteza (± desvio, ou o nº de réplicas e o IC do FWER).

---

## Tarefa 1: oficializar o M5/M6

- Revise `harness/stouffer.go`: nomes, comentários e as fórmulas acima.
- Testes:
  - sob H0 com n grande (ex.: λ = 50), `r` tem média ≈ 0 e variância ≈ 1 em 10k amostras;
  - aresta só depois (a = 0, o > 0) dá r finito e positivo;
  - aresta que zera (o = 0) dá r negativo;
  - nó sem arestas dá Z = 0.
- Rode `go run ./cmd/itt-harness -r 200` e salve em `docs/refinement/results/fase2b-baseline.md`.

## Tarefa 2: discretude (arestas com pouca contagem)

O signed root sqrt(G) é ruim para n pequeno (n = 1, 2, 3). Implemente e compare no S0 com R = 200:
- (a) signed root puro (atual);
- (b) **mid-p exato:** sob H0, `o ~ Binomial(n, q)`. Calcule o mid-p unilateral e converta com `z = Φ⁻¹(·)`, dos dois lados (silêncio e burst);
- (c) (a) ignorando arestas com n < n_min (n_min = 3).

Escolha a variante com o FWER mais perto de α e a menor perda de AUC. Registre a tabela e a escolha. A escolhida vira o M5/M6 default; as outras ficam como opções.

## Tarefa 3: geradores anti-viés (stress)

Adicione opções em `harness.Config` (default = comportamento atual) e flags no CLI:

| Opção | O que faz | Valores a testar |
|---|---|---|
| `Dispersion` | contagem gamma-Poisson (binomial negativa) com shape k; 0 = Poisson | 0, 10, 3, 1 |
| `Trend` | multiplicador global de taxa por janela, `(1+trend)^t` | 0, 0.05, 0.15 |
| `Seasonality` | taxa × (1 + A·sin(2πt/P)), com P = 4 | A = 0, 0.3 |
| `RateMu` baixo | regime de pouca contagem | ln(4) (atual), ln(0.5) |

Rode S0..S5 para cada eixo **variando um de cada vez** (R = 100). Saída em `docs/refinement/results/fase2b-stress.md`: FWER e AUC por método × condição.

É esperado que o M5 quebre com dispersão e com tendência. Isso é o objetivo da tarefa, não um problema.

## Tarefa 4: correções que saem da Tarefa 3

Implemente só o que a Tarefa 3 mostrar necessário, cada uma como opção separada e medida:

- **Tendência / taxa global:** trocar o q fixo `(T−T0)/T` por `q̂ = total depois / total geral` do grafo inteiro. Isso absorve a tendência global. Variante: q̂ por tipo de nó, se houver tipos.
- **Sobredispersão:** estimar um fator φ só com as janelas antes, sem olhar o depois:
  - para cada aresta, estatística de Pearson entre as janelas antes (Σ(c_t − c̄)²/c̄) sobre (T0 − 1) graus de liberdade;
  - φ̂ = mediana robusta entre as arestas com c̄ ≥ 1, com φ̂ ≥ 1;
  - usar r/√φ̂.

  Variante: φ̂ por faixa de taxa.

Reporte, antes e depois de cada correção, o FWER e a AUC nas condições da Tarefa 3.

## Tarefa 5: curva de poder = limite de detecção empírico

Este é o "Yharim empírico": até onde o método enxerga.

- Novo cenário paramétrico S2x: afinamento com mult ∈ {0.9, 0.8, 0.7, 0.5, 0.3, 0}.
- Cruze com:
  - faixa de grau do alvo: baixo (p10–p30), médio (p45–p55), alto (p80–p95);
  - nº de janelas depois ∈ {1, 2, 4}.
- Métrica: **power** = fração de réplicas em que o alvo entra em alarme, com o limiar da Tarefa 2 e as correções da Tarefa 4 ligadas. R = 100 por célula.
- Saída: `fase2b-power.md` + CSV (mult × grau × janelas → power). A leitura esperada é uma frase do tipo "com X eventos esperados perdidos o método detecta com 80% de poder".
- Compare com a previsão teórica: para Poisson, o poder aproximado depende de `Σ (√e − √o)²`. Registre se bate.

## Tarefa 6: t0 desconhecido

Na prática ninguém sabe quando a supressão começou.

- Modo scan: para cada split s em [T−4, T−1], calcule Z_s(v) usando só as janelas [0, s) contra [s, T). O score é o maior −Z_s.
- Limiar: Bonferroni em N × (nº de splits). Registre que isso é conservador (os splits são correlacionados).
- Novos cenários com t0 real sorteado em [T−4, T−1] e desconhecido para o método.
- Reporte AUC, FWER e poder, com t0 conhecido e com scan.

## Tarefa 7: ranking vs latente (limite honesto da teoria)

O gerador atual tem arestas independentes. Se um nó some **e nunca foi observado**, as outras arestas não mudam, e nada é detectável por construção. A tese "ausência deixa rastro" para entidades nunca observadas precisa de dependência entre arestas.

- Adicione um gerador opcional de **fluxo com relay**: uma fração ρ do tráfego n1→n2 passa por um intermediário v (n1→v→n2). Quando v some, esse tráfego cai (e não é redistribuído).
- Cenário S6 latente: v é removido **dos dados** (nem antes nem depois aparece), e o alvo passa a ser o conjunto de vizinhos de v.
- Métrica: AUC dos vizinhos de v contra o resto no M5, para ρ ∈ {0.2, 0.5, 0.8}.
- É um experimento exploratório. Registre o resultado como está, inclusive se for nulo.

## Tarefa 8: primeiro caso real, faker.js / colors.js (orçamento limitado)

Objetivo: primeiro contato com dados reais (sobredispersão, rajadas, zeros). **Não é pra ajustar nada aqui.**

- Fonte: GH Archive, `https://data.gharchive.org/YYYY-MM-DD-H.json.gz`, com H = 0..23 sem zero à esquerda. Período: 2021-12-13 a 2022-01-16 (35 dias).
- Processar em streaming, arquivo a arquivo: baixar → gunzip → filtrar → descartar. Guardar só os eventos com `repo.name` em `Marak/faker.js` ou `Marak/colors.js`.
- Primeiro meça a vazão com 1 dia. Se a projeção passar de **45 minutos** ou de 20 GB, reduza o período (mínimo: 2021-12-27 a 2022-01-10). Se o host estiver bloqueado, registre e pule a tarefa, **sem inventar dados**.
- Grafo:
  - nós = `actor.login` + os 2 repos;
  - aresta ator→repo, contagem por dia;
  - T = nº de dias. t0 no modo scan (Tarefa 6) **e** fixo em 2022-01-06, reportando os dois.
- Salve o agregado (contagens por dia por aresta, CSV pequeno) em `docs/refinement/results/faker/`.
- Reporte:
  - o top 20 do M5 silêncio e do M6 burst;
  - onde fica o ator `Marak`;
  - quantos alarmes com o limiar calibrado;
  - o φ̂ estimado;
  - o τ-jsd para comparar.
- Ground truth fraco: a conta foi suspensa perto de 2022-01-06 e o faker.js foi esvaziado em 2022-01-04. Não trate isso como verdade absoluta.
- O ecológico (Claytonia) **não** entra agora. O arquivo histórico em `itt-poc/data/ecology/interactions.csv` é uma página HTML de anti-bot, não é dado. Registre isso no relatório.

## Tarefa 9: resultados, ADRs e PR

- Seções `## Resultados (harness fase 2b)` nas ADRs 0002, 0004 e 0006, com as tabelas curtas e links para os arquivos completos. **Status continua `Proposto`.**
- Decisões novas em `OPEN-QUESTIONS.md` (continue a numeração).
- Relatório em `docs/refinement/RUN-2026-09-29-fase2b.md`, com a mesma estrutura do anterior. Cabeçalho obrigatório: "o que quebrou o M5 e o que consertou".
- PR `refinement/fase2b-silence-z → master` com o título `Fase 2b: silêncio z calibrado, stress e caso faker`.

## Fora de escopo

- Integrar na engine, mexer no deep copy (F1), mudar defaults ou a API pública.
- Aceitar ou rejeitar ADR.
- Paper, `demo/`, ARGOS.

## Ordem se o tempo apertar

1, 2, 3, 4, 5, 9 são obrigatórias. 6 vem depois. 8 e 7 são as últimas. Se faltar tempo, pule a 7 antes da 8.
