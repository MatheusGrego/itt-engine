# ADR-0003: Modelo nulo explícito e plugável, default baseline temporal

- Status: Proposto
- Data: 2026-09-29
- Decisores: Matheus
- Problemas: T4, X2

## Contexto

O paper define τ = D(P_obs ‖ P_exp), mas o código não tem P_exp. O roadmap v2 propõe P_eq como uniforme, max-entropia ou proporcional ao grau. Com P_eq uniforme, JSD(P‖U) é quase função da entropia de P, o que tende a refutar o próprio H3 (irredutibilidade à entropia). "Ausência" só existe relativa a algo esperado.

## Decisão

Interface `NullModel` plugável, que devolve o esperado `e` por aresta (ou por par nó-categoria) na janela atual.

- **Default: baseline temporal.** e = EWMA dos fluxos da mesma aresta nas janelas anteriores. É a leitura direta de "algo que existia sumiu" e aproveita o MVCC e o histórico que a engine já tem.
- **Alternativa estrutural:** configuration model ponderado, `e(i→j) = s_out(i)·s_in(j)/W`, para grafos sem histórico (primeira janela, dados estáticos).
- **Alternativa por tipo:** o esperado vem de pares do mesmo `NodeType`.

Parâmetros do default: tamanho da janela, meia-vida da EWMA e o mínimo de janelas antes de ativar (warmup).

## Opções consideradas

1. **Uniforme / max-entropia (roadmap v2).** Descartada como default, por colapsar em entropia (X2).
2. **Só estrutural.** Não captura o "existia e sumiu".
3. **Temporal com fallback estrutural.** Escolhida.

## Consequências

- (+) τ = 0 quando nada mudou, por construção.
- (+) Resolve o status ontológico de P_exp de forma operacional: o esperado é o próprio passado.
- (−) Precisa de pelo menos N janelas de histórico; nó novo não tem baseline.
- (−) Mudanças legítimas (sazonalidade, crescimento) viram silêncio falso. Mitigação futura: nulo sazonal.

## Critério de aceite

Harness, cenário S4 (nó que nunca existiu / isolado natural): τ ≈ 0. Sob o nulo puro, FP ≤ α com o baseline temporal.

## Relacionados

ADR-0002, ADR-0006. Nota `docs/notes/PQ_COUNTERFACTUAL_BASELINE.md` (mesma intuição: o esperado é o próprio sistema sem a perturbação; o baseline temporal é a versão operacional disso).
