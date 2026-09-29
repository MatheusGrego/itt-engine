# ADRs: itt-engine

Decisões de arquitetura e de teoria da ITT. Formato: MADR enxuto em PT-BR.

Regras:
- Arquivo `NNNN-titulo-com-hifens.md`, numeração sequencial.
- Status: `Proposto` → `Aceito` | `Rejeitado`; depois `Obsoleto` ou `Substituído por ADR-XXXX`.
- ADR aceito não se edita. Mudou de ideia? Escreve um novo que substitui o antigo e linka os dois.
- Todo ADR tem um **critério de aceite mensurável** (normalmente no harness da ADR-0006).
- Os IDs de problema (B1, T2, F1...) referem-se a `docs/refinement/BACKLOG.md`.

| # | Título | Status | Data |
|---|--------|--------|------|
| [0001](0001-tau-mede-ausencia.md) | τ mede ausência, não importância contrafactual | Proposto | 2026-09-29 |
| [0002](0002-estatistica-silence-deviance.md) | Estatística de tensão: desvio de Poisson com sinal (silence deviance) | Proposto | 2026-09-29 |
| [0003](0003-modelo-nulo-baseline-temporal.md) | Modelo nulo explícito e plugável, default baseline temporal | Proposto | 2026-09-29 |
| [0004](0004-detectabilidade-em-bits.md) | Detectabilidade em bits de evidência (Yharim via Sanov) | Proposto | 2026-09-29 |
| [0005](0005-ingestao-sem-deep-copy.md) | Ingestão sem deep copy por evento e análise incremental | Proposto | 2026-09-29 |
| [0006](0006-harness-supressoes-plantadas.md) | Harness sintético com supressões plantadas como critério de aceite | Proposto | 2026-09-29 |
