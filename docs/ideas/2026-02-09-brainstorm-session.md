# ITT Engine — Sessão de Brainstorm (2026-02-09)

## Decisões Tomadas

1. **Domínio de aplicação**: SEC Whistleblower Program (10-30% de sanções >$1M)
2. **Yharim Limit**: Manter o gaussiano atual. Correção Weibull = orgulho pessoal / paper, não afeta a engine
3. **Threshold prático na engine**: MAD Calibrator (`analysis/calibrator.go`) já existe e basta
4. **Fio 2 (Loss Function / Mapa)**: Arquivado em `docs/ideas/fio2-loss-function-mapa.md`
5. **A ITT engine é SDK** — o projeto SEC seria um consumidor separado

## O Que o Projeto SEC Precisa (Próximos Passos)

### Deep Research necessário antes de codar:
- **Fontes de dados**: EDGAR (SEC filings), FINRA trade data, WRDS, Yahoo Finance, APIs públicas
- **Modelagem do grafo**: Como transformar transações financeiras em nós/arestas
  - Nós = entidades (empresas, fundos, pessoas, contas)
  - Arestas = transações, ownership, board memberships, filing relationships
  - Pesos = volume, frequência, temporalidade
- **Features por nó**: O que vira a "distribuição" pra calcular JSD
- **Temporal**: Como modelar janelas de tempo (pré-anúncio vs pós-anúncio)
- **Ground truth**: Casos passados da SEC (litigated releases) pra validar

### Pipeline conceitual:
```
Dados financeiros (EDGAR/FINRA)
    → Parser/Ingest → itt.Event{}
        → ITT Engine (SDK) → Snapshot.Analyze()
            → Nós com tensão alta (MAD threshold)
                → Investigação manual → TCR (report à SEC)
```

## Contribuições Proprietárias da ITT (Resumo)

| Genuinamente novo | Status |
|---|---|
| Axiomas (Conservation, Deformation, Structural Cost) | ✅ No paper |
| Sniper Effect + Sniper Gap Δ(v) | ✅ No paper, Δ(v) pode ser formalizado |
| Domain Equivalence Theorem | ✅ No paper |
| Correção Weibull (JSD bounded → ξ<0, não Gumbel) | 🔲 Não publicado |

## Alternativas ao Yharim Limit (Arquivo)

Análise completa em `yharim_alternatives_analysis.md` (artifact do brainstorm). Resumo:
- Gaussiano atual OK pra produção
- MAD Adaptive = melhor pra streaming
- Weibull Domain = correção teórica do paper (orgulho)
- POT/GPD, Quantile, Concentration Inequality = opções futuras

## Anotações Soltas

- Frase guardada: "É como medir um buraco negro não pelo buraco negro em si, mas pela curvatura do espaço ao redor" → POC astronômico futuro
- SEC Whistleblower: anonimato garantido por lei, mas considerar riscos pessoais
- Fintech SaaS: ideia anotada pra futuro (mais demorado, mais dinheiro)
