# ARGOS — Modelo do Grafo Financeiro

## 1. Ontologia

O grafo financeiro mapeia entidades e relações do mercado de valores para a representação interna da ITT Engine.

```
                    role                          holding
      Insider ──────────────→ Company ←──────────────── Fund
     (insider:CIK)            (company:CIK)              (fund:CIK)
        │                        ▲
        │ buy/sell/exercise       │
        └────────────────────────┘
        │
        │ co_insider
        ▼
      Insider
     (insider:CIK)
```

---

## 2. Nós

### 2.1 Insider Node

| Campo | Valor | Exemplo |
|---|---|---|
| **ID** | `insider:{ownerCIK}` | `insider:0001234567` |
| **Type** | `"insider"` | — |
| **Attributes** | — | — |
| `name` | `reportingOwnerName` do Form 4 | `"COOK TIMOTHY D"` |
| `roles` | Lista de cargos | `["CEO", "Director"]` |
| `companies` | CIKs das empresas associadas | `["0000320193"]` |

**Criação**: Primeira vez que o CIK aparece como `reportingOwner` em qualquer Form 4.

**NodeTypeFunc**:
```go
func nodeTypeFunc(id string) string {
    if strings.HasPrefix(id, "insider:") {
        return "insider"
    }
    if strings.HasPrefix(id, "company:") {
        return "company"
    }
    if strings.HasPrefix(id, "fund:") {
        return "fund"
    }
    return ""
}
```

### 2.2 Company Node

| Campo | Valor | Exemplo |
|---|---|---|
| **ID** | `company:{issuerCIK}` | `company:0000320193` |
| **Type** | `"company"` | — |
| **Attributes** | — | — |
| `name` | `issuerName` do Form 4 | `"Apple Inc."` |
| `ticker` | `issuerTradingSymbol` | `"AAPL"` |
| `sector` | Do mapping Yahoo Finance / SIC | `"Technology"` |

**Criação**: Primeira vez que o CIK aparece como `issuer` em qualquer Form 4.

### 2.3 Fund Node

| Campo | Valor | Exemplo |
|---|---|---|
| **ID** | `fund:{filerCIK}` | `fund:0001067983` |
| **Type** | `"fund"` | — |
| **Attributes** | — | — |
| `name` | Nome do fundo/institution | `"BERKSHIRE HATHAWAY INC"` |

**Criação**: Primeira vez que o CIK aparece como filer em Form 13F.

---

## 3. Arestas

### 3.1 Transaction Edge (Insider → Company)

**Quando criar**: Para cada `nonDerivativeTransaction` em um Form 4.

| Campo | Mapeamento | Exemplo |
|---|---|---|
| **Source** | `insider:{ownerCIK}` | `insider:0001234567` |
| **Target** | `company:{issuerCIK}` | `company:0000320193` |
| **Type** | Baseado em `transactionCode` | `"buy"`, `"sell"`, `"option_exercise"` |
| **Weight** | Calculado pela WeightFunc | `3.5` (3.5x o volume normal) |
| **Timestamp** | `transactionDate` | `2024-06-10` |

**Mapeamento de Type**:
```go
func transactionType(code string, acquired bool) string {
    switch code {
    case "P":
        return "buy"
    case "S":
        return "sell"
    case "M":
        return "option_exercise"
    case "A":
        return "award"
    case "G":
        return "gift"
    case "F":
        return "tax_withhold"
    default:
        return "other"
    }
}
```

**Peso (WeightFunc detalhada)**:
```go
func weightFunc(ev itt.Event, store storage.Store) float64 {
    txValue := ev.Weight  // valor bruto em dólares passado inicialmente
    
    // 1. Buscar histórico do insider (EMA 365 dias)
    history, _ := store.GetTransactionHistory(ev.Source, 365)
    ema := computeEMA(history, 0.02)  // alpha = 2/(365+1) ≈ 0.005
    
    if ema == 0 {
        return 1.0  // Primeira transação = baseline
    }
    
    // 2. Peso relativo
    relativeWeight := txValue / ema
    
    // 3. Atenuação por tipo
    switch ev.Type {
    case "gift":
        relativeWeight *= 0.01  // Gifts são irrelevantes
    case "tax_withhold":
        relativeWeight *= 0.1   // Automático, não voluntário
    case "award":
        relativeWeight *= 0.5   // Compensação, semi-voluntário
    }
    
    // 4. Atenuação por 10b5-1 plan
    if isPlan10b51(ev.Metadata) {
        relativeWeight *= 0.1   // Planificado previamente
    }
    
    return relativeWeight
}
```

### 3.2 Role Edge (Insider → Company)

**Quando criar**: Ao processar o primeiro Form 4 de um insider para uma empresa.

| Campo | Valor |
|---|---|
| **Source** | `insider:{ownerCIK}` |
| **Target** | `company:{issuerCIK}` |
| **Type** | `"role"` |
| **Weight** | `1.0` (invariável) |

**Propósito**: Garante que todo insider tem pelo menos uma aresta para a empresa, mesmo sem transações recentes. Aumenta conectividade do grafo.

### 3.3 Co-Insider Edge (Insider ↔ Insider)

**Quando criar**: Pós-ingestão, quando dois insiders compartilham pelo menos 1 empresa.

| Campo | Valor |
|---|---|
| **Source** | `insider:{ownerCIK_A}` |
| **Target** | `insider:{ownerCIK_B}` |
| **Type** | `"co_insider"` |
| **Weight** | Número de empresas em comum |

**Geração**:
```go
func generateCoInsiderEdges(store storage.Store) []itt.Event {
    // 1. Para cada empresa, listar todos os insiders
    // 2. Para cada par (A, B) de insiders na mesma empresa, criar aresta
    // 3. Peso = # de empresas distintas que A e B compartilham
    
    companyInsiders := map[string][]string{} // company → []insiderCIK
    insiderPairs := map[string]int{}         // "A|B" → count
    
    // Build pairs
    for company, insiders := range companyInsiders {
        for i := 0; i < len(insiders); i++ {
            for j := i + 1; j < len(insiders); j++ {
                key := pairKey(insiders[i], insiders[j])
                insiderPairs[key]++
            }
        }
    }
    
    // Generate events
    var events []itt.Event
    for key, count := range insiderPairs {
        a, b := splitPairKey(key)
        events = append(events, itt.Event{
            Source:    a,
            Target:    b,
            Type:     "co_insider",
            Weight:   float64(count),
            Timestamp: time.Now(),
        })
    }
    return events
}
```

**Propósito**: Densifica o grafo (grafos esparsos → tensões não significativas). Permite detectar redes de insiders co-conectados (ex: Raj Rajaratnam / Galleon — ring de insiders em múltiplas tech companies).

### 3.4 Holding Edge (Fund → Company)

**Quando criar**: Para cada holding significativa no Form 13F.

| Campo | Valor |
|---|---|
| **Source** | `fund:{filerCIK}` |
| **Target** | `company:{issuerCIK}` (via CUSIP → CIK mapping) |
| **Type** | `"holding"` |
| **Weight** | `abs(deltaShares) / stddev_historico_fundo` |

**Propósito**: Detectar fundos que tomam posições anômalas antes de eventos corporativos (tippees institucionais).

---

## 4. Restrições do Modelo

### O que NÃO está no grafo (limitações conscientes)

| Entidade | Por quê não está | Impacto |
|---|---|---|
| Traders individuais (non-insiders) | Não preenchem Form 4. Dados de corretora são privados | Tippees individuais invisíveis |
| Dark pools / OTC | Dados não públicos | Trades off-exchange invisíveis |
| Opções de mercado (non-insider) | Não são insider transactions | Options unusual activity não detectável |
| Relações pessoais | Não são dados públicos | Rede social de tippees invisível |

### Limitações por volume de dados

| Cenário | Nós estimados | Arestas estimadas | Viável? |
|---|---|---|---|
| 1 empresa, 1 ano | ~20 insiders + 1 company | ~200 transactions | ✅ Trivial |
| S&P 500, 1 ano | ~25k insiders + 500 companies | ~500k transactions | ✅ OK |
| Todo EDGAR, 5 anos | ~500k insiders + 10k companies | ~5M transactions | ⚠️ Precisa batch |
| Todo EDGAR, full history | ~2M insiders + 20k companies | ~50M transactions | ❌ Precisa cluster |

**Recomendação**: Começar com S&P 500, 1 ano. É suficiente para validação e primeiro caso real.

---

## 5. Diagrama do Grafo (Exemplo)

```
                                        ┌──────────────────┐
                                        │  company:AAPL    │
     ┌───────[role]─────────────────────▶│  Apple Inc.      │◀────[holding]──── fund:BRK
     │                                  │                  │     Berkshire
     │   ┌───[sell,$3.5x]──────────────▶│                  │
     │   │                              └──────────────────┘
     │   │                                       ▲
  ┌──┴───┴──┐                                    │
  │ insider │                                    │[buy,$1.2x]
  │ COOK T  │──[co_insider, 2]──→ ┌──────────┐  │
  │ CEO     │                     │ insider  │──┘
  └─────────┘                     │ WILLIAMS │
                                  │ CFO      │──[role]──→ company:AAPL
                                  └──────────┘

  Tensão(COOK) = JSD causado por remover COOK do grafo de vizinhos
  Se sell de $3.5x (3.5x o normal) → altera distribuição de AAPL significativamente
  Sniper Gap: se degree(COOK)=3 e τ(COOK)=0.8 (alto) → Δ alto → FLAG
```
