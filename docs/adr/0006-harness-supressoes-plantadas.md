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
