# ARGOS — Arquitetura do Sistema

## 1. Visão Geral

```
┌─────────────────────────────────────────────────────────────────┐
│                         ARGOS CLI                               │
│  argos ingest | argos analyze | argos report | argos serve      │
└──────────────────────────────┬──────────────────────────────────┘
                               │
                    ┌──────────▼──────────┐
                    │    API SERVER       │  ← argos serve --port 8080
                    │  REST + WebSocket   │
                    │  /api/v1/*          │
                    └──────────┬──────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────┐
│                        ORCHESTRATOR                             │
│  Coordena fluxo: ingest → build → analyze → triage → report    │
└───┬──────────┬──────────┬──────────┬──────────┬────────────────┘
    │          │          │          │          │
┌───▼───┐ ┌───▼───┐ ┌───▼───┐ ┌───▼───┐ ┌───▼────┐
│INGEST │ │ GRAPH │ │ANALYZE│ │TRIAGE │ │ REPORT │
│       │ │ BUILD │ │       │ │       │ │        │
│edgar/ │ │graph/ │ │ ITT   │ │triage/│ │report/ │
│market/│ │       │ │Engine │ │       │ │        │
└───┬───┘ └───┬───┘ └───┬───┘ └───┬───┘ └───┬────┘
    │         │         │         │          │
┌───▼─────────▼─────────▼─────────▼──────────▼────┐
│                    STORAGE                       │
│  SQLite (dev) | PostgreSQL (prod)                │
│  Tabelas: transactions, events, holdings,        │
│           graph_snapshots, analysis_results       │
└──────────────────────────────────────────────────┘
```

## 2. Componentes

### 2.1 CLI Layer (`cmd/`)

Responsabilidade: Parsing de argumentos, validação de input, invocação do orchestrator.

```go
cmd/
├── root.go       // cobra root command, config loading
├── ingest.go     // `argos ingest` subcommand
├── build.go      // `argos build-graph` subcommand
├── analyze.go    // `argos analyze` subcommand
├── report.go     // `argos report` subcommand
├── investigate.go// `argos investigate` subcommand
├── validate.go   // `argos validate` subcommand
├── serve.go      // `argos serve` subcommand → starts API server
└── export.go     // `argos export` subcommand
```

**NÃO contém lógica de negócio.** Apenas delega para o orchestrator.

### 2.1.5 API Server Layer (`api/`)

Responsabilidade: Expor funcionalidades via REST + WebSocket para frontends.

```go
api/
├── server.go     // HTTP server setup, middleware, routing
├── routes.go     // Route definitions (/api/v1/*)
├── handlers/
│   ├── stats.go       // GET /api/v1/stats
│   ├── analysis.go    // GET /api/v1/analysis, /analysis/anomalies
│   ├── nodes.go       // GET /api/v1/nodes, /nodes/:id, /nodes/:id/neighbors
│   ├── graph.go       // GET /api/v1/graph/export
│   ├── events.go      // GET /api/v1/events, /transactions
│   ├── jobs.go        // POST /api/v1/ingest, /analyze, GET /jobs/:id
│   └── export.go      // GET /api/v1/graph/export (json, dot, csv)
├── ws/
│   ├── hub.go         // WebSocket hub (pub/sub, fan-out)
│   ├── client.go      // WebSocket client connection
│   └── bridge.go      // ITT Engine callbacks → WebSocket events
├── middleware/
│   ├── cors.go        // CORS handler
│   ├── logging.go     // Request logging
│   └── recovery.go    // Panic recovery
└── types.go           // API request/response DTOs
```

**Design**:
```go
type Server struct {
    engine      *itt.Engine
    store       storage.Store
    orchestrator *Orchestrator
    wsHub       *ws.Hub
    jobs        *JobManager
}

// Bridge: ITT callbacks → WebSocket
func (s *Server) setupBridge() {
    // OnAnomaly → ws channel "anomalies"
    s.engine.OnAnomaly(func(tr itt.TensionResult) {
        s.wsHub.Broadcast("anomalies", AnomalyEvent{
            NodeID:  tr.NodeID,
            Tension: tr.Tension,
            Degree:  tr.Degree,
        })
    })
    
    // OnTensionSpike → ws channel "spikes"
    s.engine.OnTensionSpike(func(nodeID string, delta float64) {
        s.wsHub.Broadcast("spikes", SpikeEvent{
            NodeID: nodeID,
            Delta:  delta,
        })
    })
    
    // OnChange → ws channel "changes"
    s.engine.OnChange(func(d itt.Delta) {
        if d.Type == itt.DeltaTensionChanged {
            s.wsHub.Broadcast("changes", ChangeEvent{
                NodeID: d.NodeID,
                Trend:  d.TensionTrend,
            })
        }
    })
}
```

### 2.2 Ingest Layer (`ingest/`)

Responsabilidade: Baixar e parsear dados de fontes externas.

```go
ingest/
├── source.go     // Interface DataSource
├── edgar/
│   ├── client.go     // HTTP client com rate-limiting (10 req/s)
│   ├── form4.go      // Parser de Form 4 XML → InsiderTransaction
│   ├── form8k.go     // Parser de Form 8-K → CorporateEvent
│   ├── form13f.go    // Parser de Form 13F → InstitutionalHolding
│   ├── company.go    // Resolução ticker → CIK
│   └── types.go      // InsiderTransaction, CorporateEvent, InstitutionalHolding
└── market/
    ├── yahoo.go      // Yahoo Finance API (preços, volume, earnings dates)
    └── types.go      // PriceBar, EarningsDate
```

**Interface central**:
```go
type DataSource interface {
    Name() string
    Ingest(ctx context.Context, params IngestParams) (<-chan Record, <-chan error)
}

type IngestParams struct {
    Tickers   []string
    CIKs      []string
    Since     time.Time
    Until     time.Time
    Forms     []string // "4", "8-K", "13F"
}

type Record struct {
    Type      string // "insider_tx", "corporate_event", "holding"
    Data      any
    Timestamp time.Time
}
```

### 2.3 Graph Builder (`graphbuild/`)

Responsabilidade: Transforma records em `itt.Event` e alimenta a ITT Engine.

```go
graphbuild/
├── builder.go    // Orquestra construção do grafo
├── weight.go     // WeightFunc: calcula peso relativo
├── nodes.go      // Estratégia de criação de nós (ID, Type)
├── edges.go      // Estratégia de criação de arestas (Type, Weight)
├── coinisder.go  // Gera arestas co-insider
└── filter.go     // Filtra 10b5-1 plans, gifts, etc.
```

**Design**:
```go
type GraphBuilder struct {
    engine   *itt.Engine
    store    storage.Store
    weightFn func(tx InsiderTransaction, history []InsiderTransaction) float64
}

func (gb *GraphBuilder) Build(ctx context.Context) error {
    // 1. Load transactions from store
    // 2. Compute historical averages per insider
    // 3. Generate events with relative weights
    // 4. Generate co-insider edges
    // 5. Feed to ITT Engine
}
```

### 2.4 Analysis Layer (`analysis/`)

Responsabilidade: Wrappers sobre a ITT Engine para métricas domain-specific.

```go
analysis/
├── sniper.go     // Sniper Gap Δ(v)
├── temporal.go   // Correlação temporal pré/pós evento
├── scoring.go    // Score composto final
└── validator.go  // Validação contra casos históricos
```

**Sniper Gap** (conceito ITT proprietário):
```go
func SniperGap(tension float64, degree int, stats itt.ResultStats) float64 {
    // Δ(v) = (τ(v) - E[τ|d(v)]) / σ[τ|d(v)]
    // "Quanto desvio-padrão de tensão acima do esperado para nós com esse grau"
    expectedTension := ExpectedTensionForDegree(degree, stats)
    stdDev := StdDevTensionForDegree(degree, stats)
    if stdDev == 0 { return 0 }
    return (tension - expectedTension) / stdDev
}
```

### 2.5 Triage Layer (`triage/`)

Responsabilidade: Combina métricas em score final, filtra, rankeia.

```go
triage/
├── ranker.go     // Ranking por score composto
├── config.go     // Pesos configuráveis (w1,w2,w3,w4)
└── types.go      // SuspicionCandidate struct
```

### 2.6 Report Layer (`report/`)

Responsabilidade: Gera output human/machine-readable.

```go
report/
├── markdown.go   // Relatório markdown
├── json.go       // Export JSON
└── templates/    // Templates de relatório
    └── report.md.tmpl
```

### 2.7 Storage Layer (`storage/`)

Responsabilidade: Persistência de dados ingeridos e resultados.

```go
storage/
├── store.go      // Interface Store
├── sqlite.go     // Implementação SQLite
├── postgres.go   // Implementação PostgreSQL (futuro)
└── migrations/   // Schema migrations
    ├── 001_initial.sql
    └── 002_analysis_results.sql
```

**Interface central**:
```go
type Store interface {
    // Transactions
    SaveTransactions(ctx context.Context, txs []InsiderTransaction) error
    GetTransactions(ctx context.Context, filter TxFilter) ([]InsiderTransaction, error)
    GetTransactionHistory(ctx context.Context, ownerCIK string, days int) ([]InsiderTransaction, error)
    
    // Events
    SaveEvents(ctx context.Context, events []CorporateEvent) error
    GetEvents(ctx context.Context, filter EventFilter) ([]CorporateEvent, error)
    
    // Holdings
    SaveHoldings(ctx context.Context, holdings []InstitutionalHolding) error
    GetHoldings(ctx context.Context, filter HoldingFilter) ([]InstitutionalHolding, error)
    
    // Analysis Results
    SaveAnalysis(ctx context.Context, result AnalysisResult) error
    GetLatestAnalysis(ctx context.Context) (*AnalysisResult, error)
}
```

## 3. Fluxo de Dados

```
Step 1: INGEST
  EDGAR API ──HTTP──→ edgar/client.go ──parse──→ InsiderTransaction
  EDGAR API ──HTTP──→ edgar/client.go ──parse──→ CorporateEvent
  Store.SaveTransactions() + Store.SaveEvents()

Step 2: BUILD GRAPH
  Store.GetTransactions() → GraphBuilder
  GraphBuilder computa peso relativo → WeightFunc
  GraphBuilder gera co-insider edges
  GraphBuilder → itt.Engine.AddEvents()

Step 3: ANALYZE
  itt.Engine.Snapshot().Analyze() → Results
  analysis.SniperGap() → Δ(v) por nó
  analysis.TemporalCorrelation() → ratio pré/pós
  Store.SaveAnalysis()

Step 4: TRIAGE
  triage.Rank(anomalies, sniperGaps, temporals) → []SuspicionCandidate
  Filtro: S > 0.7

Step 5: REPORT
  report.Generate(candidates) → markdown + JSON
```

## 4. Dependências

```
github.com/MatheusGrego/argos
├── github.com/MatheusGrego/itt-engine  // Core analysis SDK
├── github.com/spf13/cobra              // CLI framework
├── github.com/mattn/go-sqlite3         // Storage (dev)
├── encoding/xml                        // Form 4 parsing (stdlib)
├── net/http                            // EDGAR API (stdlib)
└── log/slog                            // Logging (stdlib)
```

## 5. Configuração

```yaml
# argos.yaml
edgar:
  user_agent: "Argos/1.0 (your@email.com)"
  rate_limit: 10  # req/s
  base_url: "https://data.sec.gov"

analysis:
  divergence: "jsd"
  alpha: 0.05
  mad_k: 3.0
  mad_warmup: 500

scoring:
  w_tension: 0.3
  w_sniper_gap: 0.3
  w_temporal: 0.3
  w_concealment: 0.1
  threshold: 0.7

temporal:
  pre_event_window: "30d"
  post_event_window: "5d"
  baseline_window: "60d"

weight:
  ema_days: 365
  temporal_boost: 2.0
  plan_10b51_attenuation: 0.1
  gift_weight: 0.01

storage:
  driver: "sqlite"
  dsn: "argos.db"
```
