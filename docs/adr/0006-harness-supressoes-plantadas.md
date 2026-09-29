# ADR-0006: Harness sintético com supressões plantadas como critério de aceite

- Status: Proposto
- Data: 2026-09-29
- Decisores: Matheus
- Problemas: E1, X1, X4

## Contexto

Hoje não existe ground truth. As validações são casos isolados (Marak, Claytonia, Peizer), e o τ da Marak coincide exatamente com f(½) = 0.311, um valor que qualquer nó recebe quando os vizinhos mandam metade do peso pra ele. Sem um teste com resposta conhecida, nenhuma mudança de τ pode ser julgada.

## Decisão

Criar `bench/harness` (ou `cmd/harness`) com:

- **Gerador:** grafo com T janelas de tempo e pesos Poisson por aresta, com a taxa vinda de um configuration model ou de blocos. Semente fixa, e parâmetros de tamanho, densidade e heterogeneidade.
- **Cenários plantados** (a partir da janela t0):
  - S1: remoção total de um nó.
  - S2: afinamento, com as arestas do nó a 30%.
  - S3: sniper, um nó de grau 2 a 3 ligado a hubs.
  - S4: controles, um nó que nunca existiu e um nó naturalmente isolado.
  - S5: burst, arestas a 300%.
  - S0: nulo puro, sem nada plantado.
- **Métricas:**
  - AUC e precision@k por cenário.
  - FP sob S0 vs α.
  - Tempo por método.
- **Métodos comparados:** τ atual (com B1 corrigido), importância contrafactual, silence deviance e as variantes futuras.
- **Saída:** uma tabela em markdown + CSV, reproduzível com um comando.

**Regra:** nenhuma ADR de teoria ou estatística passa de Proposto para Aceito sem uma linha nessa tabela.

## Opções consideradas

1. **Validar só em casos reais.** Poucos casos, sem ground truth confiável e caro.
2. **Harness sintético primeiro, casos reais depois.** Escolhida.

## Consequências

- (+) Decisões passam a ser medidas, não argumentadas.
- (+) Resultados negativos ficam registrados.
- (−) O sintético pode não refletir dados reais. Mitigação: depois, rodar num caso real com antes/depois conhecido (npm event-stream, ua-parser-js).

## Critério de aceite

Um comando gera a tabela de todos os cenários para o τ atual em < 1 min num grafo de 1k nós.

## Relacionados

Todas as ADRs de teoria (0001 a 0004).

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

- **Critério desta ADR: passou.** Um comando gera a tabela de todos os cenários em ~9 s com N = 500 e em ~19 s com N = 1000 (container de 4 vCPU, Intel Xeon @ 2.80GHz). Com N = 1000 o ranking fica praticamente igual (S1: M1 0.071, M3 1.000).
- **O que ganhou onde:** o silence deviance (M3) ranqueia o alvo em S1, S2 e S3 com AUC ≥ 0.99; o burst deviance (M4) ranqueia o S5 com 1.000. O τ atual (M1/M2) não passa de ~0.5 em nenhum cenário de supressão. O M1 dá τ = 0 ao nó removido (T1 aparecendo na tabela: a AUC de 0.074 é metade da fração de nós que também têm τ = 0), e o sniper do S3, que só tem arestas de saída, tem τ = 0 em qualquer janela, até no acumulado.
- **Surpresa:** nenhum método segura o falso positivo sob o nulo. Todos têm FWER 1.00, inclusive o M3 com o limiar em bits (causa na ADR-0002). No S3, o hit@10 do M3 é 0.90: às vezes os hubs que recebiam o sniper ficam acima dele, porque herdam o silêncio das mesmas arestas e somam o ruído das suas.

## Resultados (harness fase 2b)

O status continua **Proposto**. O harness ganhou, sem mudar o default do gerador (teste de igualdade bit a bit):

| Recurso | Opções |
|---|---|
| Stress | `Dispersion`, `Trend`, `Seasonality`, `Drift` (extra), `RateMu` |
| Poder | `S2x` com `ThinMult`, faixa de grau `TargetLo/Hi` e `PartialFrac` (extra) |
| t0 | `T0Random` |
| Relay latente | `GenerateRelay` |

- **Experimentos:** cada um roda com um comando, `go run ./cmd/itt-harness -exp <discreteness|stress|corrections|power|scan|relay> -r <R>`. Tempos no container de 4 vCPU, em paralelo: 47 s, 2m15s, 3m37s, 38 s, 52 s e 3 s. O `-exp` grava `fase2b-<exp>.md` e `.csv` em `docs/refinement/results/`.
- **Incerteza:** toda tabela reporta a AUC como média ± desvio e o FWER com IC 95% de Wilson.
- **O viés conhecido desta ADR (o gerador tem as mesmas hipóteses do método) foi confirmado.** O M5 passa no gerador base e quebra com qualquer um dos eixos de stress (FWER de 0.14 a 1.00). A lição para a regra desta ADR: uma linha na tabela do gerador base **não basta** para aceitar uma ADR de estatística; é preciso a tabela de stress também.
- **Caso real (mitigação desta ADR):** não rodou. O `data.gharchive.org` foi bloqueado pela rede do ambiente (Q8).
