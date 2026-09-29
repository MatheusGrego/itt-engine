# ADR-0005: Ingestão sem deep copy por evento e análise incremental

- Status: Proposto
- Data: 2026-09-29
- Decisores: Matheus
- Problemas: F1, F2, F3, F4, B4

## Contexto

- **F1:** `engine.processEvent` chama `ImmutableGraph.WithEvent`, que faz `deepCopyGraph` do grafo inteiro. Isso custa O(V+E) por evento e O(E²) para ingerir E eventos.
- **B4:** o benchmark de 131 ns/op mede só o enqueue no channel.
- **F2:** Analyze 100 → 1k nós vai de 475 µs → 277 ms.
- **F3:** duas alocações por vizinho em cada `Calculate`.
- **F4:** o kernel GPU implementa o τ antigo, que a ADR-0002 substitui.

## Decisão

1. **Ordem:** a performance vem *depois* das ADRs 0001 a 0003. Não otimizar o cálculo que vai ser trocado.
2. **Ingestão:** trocar o deep copy por evento por uma das duas opções, a escolher no bench (a mais simples primeiro):
   - (a) **Snapshot por lote:** mutável internamente, e publica uma versão imutável a cada K eventos ou T ms.
   - (b) **Estrutura persistente:** mapas com structural sharing (HAMT), onde cada versão copia só o caminho alterado.
3. **Análise incremental:** como S(v) é soma por aresta (ADR-0002), manter um acumulador por nó e um Dirty set. Só as arestas tocadas desde a última análise são recalculadas.
4. **GPU congelada** até a ADR-0002 ser aceita; depois, reavaliar se ainda compensa (soma por aresta é barata em CPU).
5. **Bench honesto:** end-to-end (AddEvent + drenar + versão publicada) e Analyze com 1k, 10k e 100k nós.

## Opções consideradas

1. **Otimizar o deep copy (pool, menos alocação).** Continua O(E²).
2. **Snapshot por lote.** Simples, e o isolamento MVCC continua valendo por lote.
3. **HAMT persistente.** Mais elegante, com mais código e custo constante maior.

## Consequências

- (+) Ingestão ~linear.
- (+) Analyze proporcional às mudanças, não ao tamanho do grafo.
- (−) Com lote, um snapshot não enxerga o evento "de 1 ms atrás" até o flush (latência configurável).
- (−) O trabalho de GPU atual provavelmente vira código morto.

## Critério de aceite

Bench end-to-end com 100k eventos e 10k nós: o tempo de ingestão cresce ~linear com E (dobrar E ≈ dobrar o tempo). Analyze incremental após 1% de arestas alteradas custa ≤ 5% de um Analyze completo.

## Relacionados

ADR-0002. Hipóteses H1 a H7 do `PERF-INVESTIGATION-BRIEF.md`.
