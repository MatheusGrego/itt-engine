# ADR-0001: τ mede ausência, não importância contrafactual

- Status: Proposto
- Data: 2026-09-29
- Decisores: Matheus
- Problemas: T1, X4

## Contexto

A tese da ITT é "ausência deixa rastro": uma entidade que *existia* e foi suprimida deforma os vizinhos. O `TensionCalculator` atual faz outra pergunta. Para cada vizinho n de v, ele compara a distribuição atual de n com e sem v, ou seja: "quanto estrago v causaria se sumisse agora?". Isso é importância contrafactual (parente da constraint de Burt).

Consequência direta: quando v já sumiu, a fatia dele nos vizinhos é 0 e τ = 0. A engine não enxerga justamente o evento que a teoria promete detectar. As validações do paper (Claytonia, Marak) simulam a remoção de um nó ainda presente, então medem importância.

## Decisão

Separar os dois conceitos na API e na teoria:

- **τ (tensão)** passa a significar *evidência de ausência*: o quanto os fluxos observados em torno de v ficaram abaixo do esperado. Implementação: ADR-0002 + ADR-0003.
- A medida atual é renomeada para **importância contrafactual** (`Importance` / `ι`) e continua disponível como descritor. Ela é útil pra priorizar o que monitorar, não pra detectar.

## Opções consideradas

1. **Manter o τ atual e só trocar a divergência.** Descartada, porque qualquer f-divergência dá o mesmo ranking no leave-one-out (T2).
2. **Redefinir τ como ausência e manter a importância como métrica separada.** Escolhida.
3. **Apagar a importância.** Descartada, porque ela tem uso legítimo (pré-seleção de nós críticos) e é barata.

## Consequências

- (+) A teoria e o código passam a responder a mesma pergunta.
- (+) Nó suprimido deixa de ter τ = 0 por construção.
- (−) Quebra a semântica de `TensionResult.Tension` para quem já usa (ARGOS, NPM Hunter). Precisa de um período com os dois campos.
- (−) Os números do paper (0.311, 0.293) deixam de valer como evidência de detecção.

## Critério de aceite

No harness (ADR-0006), cenário S1 (remoção total): o nó removido fica no top-k do novo τ com AUC > a da importância contrafactual.

## Relacionados

ADR-0002, ADR-0003, ADR-0006.
