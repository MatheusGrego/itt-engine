# ARGOS — Pipeline de Processamento

## Visão Geral

O pipeline ARGOS tem 6 estágios sequenciais. Cada estágio é idempotente e pode ser re-executado sem side effects.

```
┌────────┐   ┌────────┐   ┌────────┐   ┌────────┐   ┌────────┐   ┌────────┐
│INGEST  │──▶│ BUILD  │──▶│ANALYZE │──▶│TRIAGE  │──▶│REPORT  │──▶│VALIDATE│
│        │   │ GRAPH  │   │        │   │        │   │        │   │(opt)   │
│ edgar/ │   │graphb/ │   │ ITT +  │   │rank +  │   │ md +   │   │known   │
│ market │   │        │   │ sniper │   │filter  │   │ json   │   │cases   │
└────────┘   └────────┘   └────────┘   └────────┘   └────────┘   └────────┘
     │            │            │            │            │            │
     ▼            ▼            ▼            ▼            ▼            ▼
  [Storage]   [ITT Engine] [Analysis]  [Candidates]  [Reports]   [Metrics]
```

---

## Stage 1: INGEST

### Input
- CLI params: `--source edgar --forms 4,8K --ticker AAPL --since 2024-01-01`
- Ou batch: `--source edgar --forms 4 --universe sp500 --since 2024-01-01`

### Processamento
```
1. Resolver tickers → CIKs (usando company_tickers.json cache)
2. Para cada CIK:
   a. GET submissions/CIK{padded}.json
   b. Filtrar filings por form type e date range
   c. Para cada filing:
      - GET filing XML/HTML
      - Parse com parser específico (form4.go, form8k.go, form13f.go)
      - Validar campos obrigatórios
      - Deduplificar contra storage existente
      - Persistir no storage
3. Logar: total ingerido, erros, taxa de erro
```

### Output
- Tabela `insider_transactions` populada
- Tabela `corporate_events` populada
- Tabela `institutional_holdings` populada

### Idempotência
- Check antes de inserir: `(ownerCIK, issuerCIK, txDate, txCode, shares)` é unique key
- Re-executar ingest para o mesmo período não duplica dados

### Rate Limiting
```go
type RateLimiter struct {
    ticker *time.Ticker  // 100ms interval = 10 req/s
}

func (r *RateLimiter) Wait() {
    <-r.ticker.C
}
```

### Error Handling
| Erro | Ação | Continua? |
|---|---|---|
| HTTP 403/429 | Backoff exponencial (1s, 2s, 4s, 8s, max 60s) | Sim, retry |
| HTTP 404 | Log warning, skip filing | Sim |
| XML parse error | Log error com accession number | Sim |
| Network timeout | Retry 3x com backoff | Sim |
| Invalid CIK | Log error | Sim |

---

## Stage 2: BUILD GRAPH

### Input
- Dados do storage (transactions, events, holdings)

### Processamento
```
1. Carregar todas as transações do período
2. Calcular EMA-365 por insider (para peso relativo)
3. Gerar events de transação:
   Para cada InsiderTransaction:
     weight = WeightFunc(tx, history)
     emit itt.Event{
       Source: "insider:{ownerCIK}",
       Target: "company:{issuerCIK}",
       Type:   transactionType(tx.Code),
       Weight: weight,
       Timestamp: tx.TransactionDate,
       Metadata: {
         "shares": tx.Shares,
         "price": tx.PricePerShare,
         "is_10b51": tx.Is10b51Plan,
       },
     }
4. Gerar events de role:
   Para cada par (insider, empresa) único:
     emit itt.Event{Source, Target, Type:"role", Weight:1.0}
5. Gerar events co-insider:
   Computar pares de insiders com empresas em comum
   Para cada par: emit itt.Event{Type:"co_insider", Weight:count}
6. Gerar events de holding (se Form 13F ingerido):
   Para cada holding com deltaShares significativo:
     emit itt.Event{Type:"holding", Weight:normalized_delta}
7. Ordenar TODOS os events por Timestamp
8. Alimentar ITT Engine sequencialmente:
   engine.AddEvents(sortedEvents)
```

### Output
- ITT Engine populada com grafo financeiro
- Pronta para snapshot/análise

### Ordenação Temporal
**Crítico**: A ITT Engine processa eventos sequencialmente. A ordem importa para:
- Tensão temporal (TensionHistory)
- Correto cálculo de EMA (eventos devem ser cronológicos)
- MVCC versioning (versões devem ser monotônicas)

---

## Stage 3: ANALYZE

### Input
- ITT Engine populada (Stage 2)

### Processamento
```
1. snap = engine.Snapshot()
2. results = snap.Analyze()
   → Calcula tensão de TODOS os nós
   → MAD calibrator identifica anomalias
   → Detectability analysis (SNR, Yharim)
   → Temporal summary (spikes, decay, phase)
3. Para cada nó anômalo (results.Anomalies):
   sniperGap = analysis.SniperGap(node.Tension, node.Degree, results.Stats)
4. Para cada nó anômalo com sniperGap > 1.0:
   temporal = analysis.TemporalCorrelation(nodeID, corporateEvents, tensionHistory)
5. Persistir resultados no storage
```

### Sniper Gap Calculation
```go
func SniperGap(tension float64, degree int, stats ResultStats) float64 {
    // Agrupar nós por grau, calcular E[τ|d] e σ[τ|d]
    // Para o grau d deste nó:
    expectedTension := expectedTensionByDegree[degree]
    stdDevTension := stdDevTensionByDegree[degree]
    
    if stdDevTension < 1e-10 {
        return 0  // Grau com variância zero (muito poucos nós com esse grau)
    }
    
    return (tension - expectedTension) / stdDevTension
}
```

### Temporal Correlation
```go
func TemporalCorrelation(
    nodeID string,
    events []CorporateEvent,
    history *TensionHistory,
) []TemporalMatch {
    var matches []TemporalMatch
    
    for _, event := range events {
        // Encontrar empresa associada ao nó
        if !isRelated(nodeID, event.IssuerCIK) {
            continue
        }
        
        // Tensão baseline (60d antes da janela pré-evento)
        baselineStart := event.EventDate.Add(-90 * 24 * time.Hour)
        baselineEnd := event.EventDate.Add(-30 * 24 * time.Hour)
        τ_baseline := history.MeanInRange(baselineStart, baselineEnd)
        
        // Tensão pré-evento (30d antes do evento)
        preStart := event.EventDate.Add(-30 * 24 * time.Hour)
        preEnd := event.EventDate
        τ_pre := history.MeanInRange(preStart, preEnd)
        
        if τ_baseline == 0 {
            continue
        }
        
        ratio := τ_pre / τ_baseline
        if ratio > 2.0 {
            matches = append(matches, TemporalMatch{
                NodeID:    nodeID,
                EventCIK:  event.IssuerCIK,
                EventType: event.Type,
                EventDate: event.EventDate,
                Baseline:  τ_baseline,
                PreEvent:  τ_pre,
                Ratio:     ratio,
            })
        }
    }
    return matches
}
```

### Output
- `[]AnomalyResult` com tensão, sniperGap, correlaçãoTemporal
- Persistido em `analysis_results` table

---

## Stage 4: TRIAGE

### Input
- Anomaly results (Stage 3)

### Processamento
```
1. Para cada anomalia:
   score = w1*normalize(tension) + w2*normalize(sniperGap) + w3*normalize(temporalRatio) + w4*normalize(concealment)
   
   normalize(x) = (x - min) / (max - min)  // min-max scaling no batch
   
2. Filtrar: score > threshold (default 0.7)
3. Ordenar por score descendente
4. Enriquecer com dados contextuais:
   - Nome e cargo do insider
   - Ticker e nome da empresa
   - Lista de transações do período
   - Eventos corporativos correlacionados
```

### Output
- `[]SuspicionCandidate` ordenado por score

### SuspicionCandidate Structure
```go
type SuspicionCandidate struct {
    // Identity
    EntityID     string   // ex: "insider:0001234567"
    EntityName   string   // ex: "COOK TIMOTHY D"
    EntityType   string   // "insider" | "fund"
    
    // ITT Metrics
    Tension      float64  // τ(v)
    Degree       int      // d(v)
    SniperGap    float64  // Δ(v)
    Concealment  float64  // Ω(v)
    
    // Score
    Score        float64  // S(v) ∈ [0,1]
    Components   ScoreComponents
    
    // Context
    Companies    []CompanyContext
    Transactions []TransactionContext
    TemporalMatches []TemporalMatch
    
    // Recommendation
    Recommendation string  // "investigate" | "monitor" | "dismiss"
}

type CompanyContext struct {
    CIK     string
    Ticker  string
    Name    string
    Role    string  // "CEO", "Director", etc.
}

type TransactionContext struct {
    Date           time.Time
    Code           string   // P, S, M, etc.
    Shares         int
    PricePerShare  float64
    TotalValue     float64
    RelativeWeight float64  // Peso calculado pela WeightFunc
}
```

---

## Stage 5: REPORT

### Input
- `[]SuspicionCandidate` (Stage 4)

### Output Markdown
```markdown
# ARGOS Analysis Report
Generated: 2024-12-15T10:30:00Z
Period: 2024-06-01 — 2024-12-15
Universe: S&P 500

## Executive Summary
- Nodes analyzed: 25,432
- Anomalies detected: 47
- High-confidence candidates: 5
- Temporal correlations found: 3

## Top Candidates

### 1. JOHN DOE (Score: 0.92)
- **Entity**: insider:0009876543
- **Role**: Director at ACME Corp (ACME)
- **Tension**: 0.85 (rank: 3/25432)
- **Sniper Gap**: 4.2σ
- **Temporal Match**: ✅ Purchased $450k in ACME 12 days before M&A announcement (normally trades ~$30k)

#### Transaction Timeline
| Date | Type | Shares | Price | Value | Relative Weight |
|---|---|---|---|---|---|
| 2024-11-03 | Buy | 10,000 | $45.00 | $450,000 | 15.0x |
| 2024-08-15 | Sell | 2,000 | $38.00 | $76,000 | 2.5x |
| 2024-03-01 | Buy | 800 | $35.00 | $28,000 | 1.0x (baseline) |

#### Correlated Event
- **2024-11-15**: ACME announces acquisition by XYZ Corp (Form 8-K Item 2.01)
- **Price impact**: ACME +34% on announcement day

---

> ⚠️ This report is generated by automated analysis and requires manual verification before any action.
```

### Output JSON
```json
{
  "metadata": {
    "generated_at": "2024-12-15T10:30:00Z",
    "period": { "since": "2024-06-01", "until": "2024-12-15" },
    "universe": "sp500",
    "nodes_analyzed": 25432,
    "anomalies": 47
  },
  "candidates": [
    {
      "entity_id": "insider:0009876543",
      "entity_name": "JOHN DOE",
      "score": 0.92,
      "tension": 0.85,
      "sniper_gap": 4.2,
      "concealment": 0.34,
      "companies": [...],
      "transactions": [...],
      "temporal_matches": [...],
      "recommendation": "investigate"
    }
  ]
}
```

---

## Stage 6: VALIDATE (Opcional)

### Input
- Nome de caso histórico: `--case "martha-stewart"`

### Processamento
```
1. Carregar fixture do caso:
   - CIKs dos envolvidos
   - Data do evento
   - Período das transações suspeitas
2. Ingerir dados do EDGAR para o período
3. Executar pipeline completo (stages 1-5)
4. Verificar:
   - Insider está na lista de anomalias? (True Positive)
   - Sniper Gap > 1.0?
   - Correlação temporal detectada?
5. Output: métricas de detecção
```

### Fixtures Pré-configuradas

```go
var validationCases = map[string]ValidationCase{
    "martha-stewart": {
        Name:        "Martha Stewart / ImClone",
        InsiderCIKs: []string{"0001234567"},  // Sam Waksal
        TippeeCIKs:  []string{"0007654321"},  // Non-insider, limitação
        CompanyCIK:  "0000765502",            // ImClone Systems
        EventDate:   time.Date(2001, 12, 28, 0, 0, 0, 0, time.UTC),
        SuspectWindow: 30 * 24 * time.Hour,
        Description: "FDA denial of Erbitux, insider tipped Martha Stewart",
    },
    "raj-rajaratnam": {
        Name:        "Raj Rajaratnam / Galleon Group",
        InsiderCIKs: []string{...},
        CompanyCIK:  "multiple",
        EventDate:   time.Date(2009, 10, 16, 0, 0, 0, 0, time.UTC),
        Description: "Insider trading ring across tech companies",
    },
    "chris-collins": {
        Name:        "Chris Collins / Innate Immunotherapeutics",
        InsiderCIKs: []string{...},
        CompanyCIK:  "...",
        EventDate:   time.Date(2017, 6, 22, 0, 0, 0, 0, time.UTC),
        Description: "Congressman tipped son about failed drug trial",
    },
}
```

### Métricas de Validação
```
Case: Martha Stewart / ImClone
  Insider detected as anomaly: ✅
  Sniper Gap: 3.8σ (threshold: 1.0) ✅
  Temporal correlation: 2.4x baseline (threshold: 2.0) ✅
  Rank among all nodes: #7/1234
  Detection: TRUE POSITIVE
```

---

## Schema do Storage (SQLite)

```sql
-- Stage 1: Ingest
CREATE TABLE insider_transactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_cik TEXT NOT NULL,
    owner_name TEXT NOT NULL,
    issuer_cik TEXT NOT NULL,
    issuer_name TEXT NOT NULL,
    issuer_ticker TEXT,
    transaction_date DATE NOT NULL,
    filing_date DATE NOT NULL,
    transaction_code TEXT NOT NULL,  -- P, S, M, A, G, F, D
    shares INTEGER NOT NULL,
    price_per_share REAL,
    total_value REAL,
    shares_owned_post INTEGER,
    is_director BOOLEAN,
    is_officer BOOLEAN,
    officer_title TEXT,
    is_ten_pct_owner BOOLEAN,
    direct_or_indirect TEXT,  -- D or I
    is_10b51_plan BOOLEAN DEFAULT FALSE,
    accession_number TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(owner_cik, issuer_cik, transaction_date, transaction_code, shares)
);

CREATE TABLE corporate_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    issuer_cik TEXT NOT NULL,
    issuer_name TEXT NOT NULL,
    event_date DATE NOT NULL,
    filing_date DATE NOT NULL,
    event_type TEXT NOT NULL,  -- merger_acquisition, earnings, etc.
    item_numbers TEXT,  -- "1.01,2.01"
    description TEXT,
    accession_number TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE institutional_holdings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    filer_cik TEXT NOT NULL,
    filer_name TEXT NOT NULL,
    report_date DATE NOT NULL,
    issuer_cusip TEXT NOT NULL,
    issuer_name TEXT NOT NULL,
    shares INTEGER NOT NULL,
    value_thousands INTEGER NOT NULL,
    investment_discretion TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(filer_cik, report_date, issuer_cusip)
);

-- Stage 3/4: Results
CREATE TABLE analysis_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_date TIMESTAMP NOT NULL,
    entity_id TEXT NOT NULL,
    entity_name TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    tension REAL NOT NULL,
    degree INTEGER NOT NULL,
    sniper_gap REAL,
    concealment REAL,
    score REAL,
    recommendation TEXT,
    temporal_matches_json TEXT,  -- JSON array
    transactions_json TEXT,      -- JSON array
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX idx_tx_owner ON insider_transactions(owner_cik);
CREATE INDEX idx_tx_issuer ON insider_transactions(issuer_cik);
CREATE INDEX idx_tx_date ON insider_transactions(transaction_date);
CREATE INDEX idx_events_issuer ON corporate_events(issuer_cik);
CREATE INDEX idx_events_date ON corporate_events(event_date);
CREATE INDEX idx_holdings_filer ON institutional_holdings(filer_cik);
CREATE INDEX idx_holdings_cusip ON institutional_holdings(issuer_cusip);
CREATE INDEX idx_results_entity ON analysis_results(entity_id);
CREATE INDEX idx_results_score ON analysis_results(score DESC);
```
