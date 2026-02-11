# ARGOS — Anomaly Recognition in Graphs for Oversight of Securities

> *"Panoptes, the all-seeing"* — Named after Argos Panoptes, o gigante de cem olhos da mitologia grega que tudo via.

## Missão

Detectar padrões anômalos em transações de insiders do mercado financeiro americano, utilizando a **ITT Engine** (Informational Tension Theory) como núcleo de análise de grafos, para identificar potenciais violações de securities law reportáveis ao **SEC Whistleblower Program**.

## Stack Tecnológico

| Componente | Tecnologia |
|---|---|
| **Linguagem** | Go 1.22+ |
| **Core Analysis** | `github.com/MatheusGrego/itt-engine` (SDK) |
| **Data Source** | SEC EDGAR API (`data.sec.gov`) |
| **Market Data** | Yahoo Finance API |
| **Storage** | SQLite (local) ou PostgreSQL (produção) |
| **CLI** | `cobra` framework |
| **Logging** | `slog` (stdlib) |

## Repositório

```
github.com/MatheusGrego/argos
```

## Documentação

| Documento | Descrição |
|---|---|
| [REQUIREMENTS.md](./REQUIREMENTS.md) | Requisitos funcionais e não-funcionais |
| [ARCHITECTURE.md](./ARCHITECTURE.md) | Arquitetura do sistema |
| [DATA-SOURCES.md](./DATA-SOURCES.md) | Especificação das fontes de dados |
| [GRAPH-MODEL.md](./GRAPH-MODEL.md) | Modelagem do grafo financeiro |
| [PIPELINE.md](./PIPELINE.md) | Pipeline de processamento end-to-end |

## Quick Start (Futuro)

```bash
# Ingestão de dados
argos ingest --source edgar --forms 4,8K --since 2024-01-01

# Análise
argos analyze --window 30d --threshold 3.0

# Relatório dos nós mais suspeitos
argos report --top 20 --format markdown

# Investigação de um nó específico
argos investigate --entity CIK0001234567
```
