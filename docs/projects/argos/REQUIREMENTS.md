# ARGOS — Requisitos

## 1. Requisitos Funcionais

---

### RF-01: Ingestão de Form 4 (Insider Transactions)

**Descrição**: O sistema deve ser capaz de baixar, parsear e armazenar transações de insiders do SEC EDGAR.

**Input**: CIK de empresa ou ticker, intervalo de datas.

**Output**: Registros `InsiderTransaction` persistidos no storage local.

**Critérios de Aceite**:
- [ ] CA-01.1: Dado um ticker (ex: `AAPL`), o sistema busca o CIK correspondente via EDGAR company search
- [ ] CA-01.2: Dado um CIK, o sistema baixa todos os Form 4 filings no intervalo de datas especificado
- [ ] CA-01.3: Cada Form 4 XML é parseado extraindo: `reportingOwnerCIK`, `reportingOwnerName`, `issuerCIK`, `issuerName`, `transactionDate`, `transactionCode` (P/S/A/M/F/D/G), `sharesTransacted`, `pricePerShare`, `sharesOwnedPost`, `isDirector`, `isOfficer`, `officerTitle`, `isTenPercentOwner`, `directOrIndirect`
- [ ] CA-01.4: Transações são deduplificadas por (ownerCIK, issuerCIK, transactionDate, transactionCode, shares)
- [ ] CA-01.5: Erros de parsing são logados mas não interrompem o processamento batch
- [ ] CA-01.6: Rate limiting de 10 req/s é respeitado conforme policy do EDGAR
- [ ] CA-01.7: User-Agent header inclui identificação conforme requerido pelo EDGAR

---

### RF-02: Ingestão de Form 8-K (Corporate Events)

**Descrição**: O sistema deve baixar e parsear eventos corporativos materiais.

**Input**: CIK de empresa, intervalo de datas.

**Output**: Registros `CorporateEvent` persistidos no storage local.

**Critérios de Aceite**:
- [ ] CA-02.1: Form 8-K filings são baixados do EDGAR Full-Text Search API
- [ ] CA-02.2: Classificação automática do tipo de evento: `merger_acquisition`, `earnings`, `fda_approval`, `executive_change`, `bankruptcy`, `other`
- [ ] CA-02.3: Cada evento tem: `issuerCIK`, `eventDate`, `filingDate`, `eventType`, `description`
- [ ] CA-02.4: Eventos são correlacionáveis com transações por `issuerCIK` e janela temporal

---

### RF-03: Ingestão de Form 13F (Institutional Holdings)

**Descrição**: O sistema deve baixar holdings trimestrais de fundos institucionais.

**Input**: Intervalo de datas (trimestre).

**Output**: Registros `InstitutionalHolding` persistidos.

**Critérios de Aceite**:
- [ ] CA-03.1: Para cada 13F filing: `filerCIK`, `filerName`, `reportDate`, lista de holdings
- [ ] CA-03.2: Cada holding tem: `issuerCUSIP`, `issuerName`, `shares`, `value`, `investmentDiscretion`
- [ ] CA-03.3: Variação trimestral é calculada: `deltaShares = sharesQ_current - sharesQ_previous`
- [ ] CA-03.4: Holdings com `deltaShares` significativo (> 2σ do histórico do fundo) são flagged

---

### RF-04: Construção do Grafo Financeiro

**Descrição**: O sistema deve transformar dados financeiros em um grafo de entidades e relações compatível com a ITT Engine.

**Input**: Registros `InsiderTransaction`, `CorporateEvent`, `InstitutionalHolding`.

**Output**: Stream de `itt.Event` alimentando a ITT Engine.

**Critérios de Aceite**:
- [ ] CA-04.1: **Nós Pessoa** são criados com ID = `insider:{ownerCIK}`, Type = `insider`
- [ ] CA-04.2: **Nós Empresa** são criados com ID = `company:{issuerCIK}`, Type = `company`
- [ ] CA-04.3: **Nós Fundo** são criados com ID = `fund:{filerCIK}`, Type = `fund`
- [ ] CA-04.4: **Arestas Transaction** (insider → company): `Type = "buy"|"sell"|"option_exercise"`, `Weight = volumeRelativo` (ver RF-05)
- [ ] CA-04.5: **Arestas Role** (insider → company): `Type = "role"`, `Weight = 1.0`, derivadas de `isDirector/isOfficer/isTenPercentOwner`
- [ ] CA-04.6: **Arestas Co-Insider** (insider ↔ insider): `Type = "co_insider"`, `Weight = # empresas em comum`, criadas automaticamente quando dois insiders compartilham pelo menos 1 empresa
- [ ] CA-04.7: **Arestas Holding** (fund → company): `Type = "holding"`, `Weight = deltaShares_normalized`
- [ ] CA-04.8: Eventos são ordenados cronologicamente antes de ingestão na ITT Engine
- [ ] CA-04.9: O `Event.Timestamp` é a `transactionDate` (não a `filingDate`)

---

### RF-05: Cálculo de Peso Relativo

**Descrição**: O peso de cada aresta de transação deve refletir a significância relativa da transação, não o valor absoluto.

**Input**: `InsiderTransaction` + histórico de transações do insider.

**Output**: `float64` peso normalizado.

**Critérios de Aceite**:
- [ ] CA-05.1: Para cada insider, manter média móvel exponencial (EMA) de volume transacionado nos últimos 365 dias
- [ ] CA-05.2: `weightBase = transactionValue / EMA_365(insider)`. Se EMA = 0 (primeira transação), usar `weightBase = 1.0`
- [ ] CA-05.3: Aplicar fator temporal: se a transação ocorre dentro de janela pré-evento (RF-07), multiplicar por `temporalBoost` (default: 2.0)
- [ ] CA-05.4: Transações sob Rule 10b5-1 plans devem ter `weightBase *= 0.1` (atenuação de ruído)
- [ ] CA-05.5: O peso final é passado via `Builder.WeightFunc()` da ITT Engine
- [ ] CA-05.6: Transações Gift (código G) recebem peso 0.01 (quase irrelevantes)

---

### RF-06: Análise de Tensão e Detecção de Anomalias

**Descrição**: O sistema executa análise ITT no grafo construído e identifica nós anômalos.

**Input**: Grafo financeiro na ITT Engine.

**Output**: Lista de nós com tensão acima do threshold MAD, ordenados por tensão.

**Critérios de Aceite**:
- [ ] CA-06.1: ITT Engine é configurada com `Divergence(analysis.JSD{})`, `DetectabilityAlpha(0.05)`, `Calibrator(analysis.NewCalibrator(analysis.WithK(3.0), analysis.WithWarmupSize(500)))`
- [ ] CA-06.2: `Snapshot.Analyze()` é executado e `Results.Anomalies` é retornado
- [ ] CA-06.3: Para cada nó anômalo, calcular o **Sniper Gap**: `Δ(v) = (τ(v) - τ_esperada(d(v))) / σ_τ|d`
- [ ] CA-06.4: Nós são rankeados por Sniper Gap descendente
- [ ] CA-06.5: Apenas nós com `τ > MAD_threshold` E `Δ > 1.0` são considerados candidatos
- [ ] CA-06.6: Output inclui: `nodeID`, `tension`, `degree`, `sniperGap`, `nodeType`, `concealment`

---

### RF-07: Análise Temporal (Janela Pré/Pós Evento)

**Descrição**: O sistema compara tensão antes e depois de eventos corporativos para detectar correlação com insider information.

**Input**: Nós candidatos (RF-06) + Corporate Events (RF-02).

**Output**: Lista de correlações (insider, empresa, evento, tensão pré/pós).

**Critérios de Aceite**:
- [ ] CA-07.1: Para cada empresa com eventos materiais (8-K), definir janela pré-evento de 30 dias e pós-evento de 5 dias
- [ ] CA-07.2: Para cada insider que transacionou na empresa durante a janela pré-evento, comparar:
  - `τ_pre` = tensão média do insider nos 30 dias pré-evento
  - `τ_baseline` = tensão média do insider nos 60 dias antes da janela pré
- [ ] CA-07.3: Se `τ_pre / τ_baseline > 2.0` → correlação temporal significativa
- [ ] CA-07.4: Output inclui: `insiderCIK`, `companyCIK`, `eventType`, `eventDate`, `τ_pre`, `τ_baseline`, `ratio`, `transactionsInWindow[]`
- [ ] CA-07.5: Correlações são rankeadas por `ratio` descendente

---

### RF-08: Triage e Scoring Final

**Descrição**: O sistema combina tensão, Sniper Gap e correlação temporal em um score final de suspeição.

**Input**: Candidatos de RF-06 + Correlações de RF-07.

**Output**: Score final por entidade investigável.

**Critérios de Aceite**:
- [ ] CA-08.1: Score composto: `S = w1·τ_normalized + w2·Δ_normalized + w3·temporal_ratio + w4·concealment_normalized`
- [ ] CA-08.2: Pesos default: `w1=0.3, w2=0.3, w3=0.3, w4=0.1`
- [ ] CA-08.3: Pesos são configuráveis via CLI flags
- [ ] CA-08.4: Entidades com `S > threshold_score` (default: 0.7) são marcadas como "investigate"
- [ ] CA-08.5: Output final em JSON estruturado: `{ entity, score, components, transactions[], events[], recommendation }`

---

### RF-09: Geração de Relatório

**Descrição**: O sistema gera um relatório estruturado para apoiar investigação manual e preenchimento do TCR.

**Input**: Entidades triaged (RF-08).

**Output**: Relatório markdown + JSON.

**Critérios de Aceite**:
- [ ] CA-09.1: Relatório markdown contém: sumário executivo, top N entidades, timeline de transações, eventos correlacionados, métricas ITT
- [ ] CA-09.2: Para cada entidade: nome, CIK, empresas associadas, transações, tensão, Sniper Gap, correlação temporal
- [ ] CA-09.3: JSON export com todos os dados estruturados para programmatic access
- [ ] CA-09.4: Relatório inclui aviso de que é output de análise automatizada e requer verificação manual

---

### RF-10: CLI Interface

**Descrição**: Toda funcionalidade é acessível via CLI.

**Critérios de Aceite**:
- [ ] CA-10.1: `argos ingest --source edgar --forms 4 --ticker AAPL --since 2024-01-01 --until 2024-12-31`
- [ ] CA-10.2: `argos ingest --source edgar --forms 8K --ticker AAPL --since 2024-01-01`
- [ ] CA-10.3: `argos ingest --source edgar --forms 13F --quarter 2024Q3`
- [ ] CA-10.4: `argos build-graph` — constrói o grafo a partir dos dados ingeridos
- [ ] CA-10.5: `argos analyze --window 30d --threshold 3.0` — executa análise ITT
- [ ] CA-10.6: `argos report --top 20 --format md|json` — gera relatório
- [ ] CA-10.7: `argos investigate --entity CIK0001234567` — detalhes de uma entidade
- [ ] CA-10.8: `argos validate --case "martha-stewart"` — validação contra caso conhecido
- [ ] CA-10.9: Todos os comandos logam progresso via stderr e resultado via stdout
- [ ] CA-10.10: `--verbose` flag para log detalhado

---

## 2. Requisitos Não-Funcionais

### RNF-01: Performance
- [ ] Ingestão de 100k Form 4 filings em < 30 minutos (limitado por rate limit EDGAR)
- [ ] Análise ITT de grafo com 50k nós em < 60 segundos
- [ ] Memory footprint < 2GB para grafos de até 100k nós

### RNF-02: Reliability
- [ ] Ingestão é idempotente — re-executar não duplica dados
- [ ] Crash recovery — dados ingeridos são persistidos antes de análise
- [ ] Graceful degradation — se EDGAR estiver offline, usar cache local

### RNF-03: Compliance
- [ ] Rate limiting de 10 req/s ao EDGAR (conforme SEC fair access policy)
- [ ] User-Agent header identifica a aplicação conforme requerido
- [ ] Nenhum dado proprietário é utilizado — apenas dados públicos
- [ ] Relatórios incluem disclaimer de que são output algorítmico

### RNF-04: Testabilidade
- [ ] Cobertura de testes unitários > 80% para parsing e graph building
- [ ] Testes de integração com fixtures de Form 4 XML reais
- [ ] Validação contra pelo menos 5 casos históricos de insider trading da SEC

### RNF-05: Extensibilidade
- [ ] Novas fontes de dados podem ser adicionadas implementando interface `DataSource`
- [ ] Novos tipos de evento podem ser adicionados sem mudanças no core
- [ ] Scoring weights são configuráveis sem recompilação

---

## 3. Requisitos de Monitoramento em Tempo Real

---

### RF-11: Real-Time Monitoring via ITT Callbacks

**Descrição**: O sistema deve suportar monitoramento em tempo real utilizando os callbacks nativos da ITT Engine (`OnChange`, `OnAnomaly`, `OnTensionSpike`).

**Fundamento técnico**: A ITT Engine oferece 3 callbacks de streaming:
- `Builder.OnChange(func(Delta))` — emite TODA mutação do grafo (node added, edge updated, tension changed, anomaly detected/resolved)
- `Builder.OnAnomaly(func(TensionResult))` — emite anomalias em tempo real quando detectadas durante ingestão
- `Builder.OnTensionSpike(func(nodeID string, delta float64))` — emite quando a variação de tensão de um nó excede o threshold configurado

**Critérios de Aceite**:
- [ ] CA-11.1: `OnAnomaly` é configurado para logar em tempo real insiders flagged durante ingestão contínua (live mode)
- [ ] CA-11.2: `OnTensionSpike` é configurado com threshold = 0.3 (default), emitindo alerta quando tensão de um nó sobe abruptamente
- [ ] CA-11.3: `OnChange` com `DeltaTensionChanged` é usado para rastrear transições de trend (Stable → Increasing → Decreasing) por insider
- [ ] CA-11.4: Alertas em tempo real são escritos para `stdout` (formato JSON-lines) E para um log file configurável
- [ ] CA-11.5: Live mode (`argos live --universe sp500 --poll 1h`) executa ingestão incremental + análise em loop contínuo
- [ ] CA-11.6: Live mode emite métricas periódicas: total de nós, anomalias ativas, média de tensão, top 5 insiders por tensão

---

### RF-12: Export e Visualização do Grafo

**Descrição**: O sistema deve exportar o grafo financeiro em formatos consumíveis por ferramentas de visualização.

**Fundamento técnico**: A ITT Engine oferece `export.JSON()` e `export.DOT()` (Graphviz) para serializar o grafo completo.

**Critérios de Aceite**:
- [ ] CA-12.1: `argos export --format json --output graph.json` — exporta o grafo completo via `export.JSON()`
- [ ] CA-12.2: `argos export --format dot --output graph.dot` — exporta em formato Graphviz DOT via `export.DOT()`
- [ ] CA-12.3: `argos export --format json --filter anomalies` — exporta apenas nós anômalos e suas vizinhanças (k=2 hops)
- [ ] CA-12.4: Export JSON inclui tensão, grau, sniperGap, e tipo de nó como atributos
- [ ] CA-12.5: Export DOT usa cores diferenciadas por tipo de nó (insider=blue, company=green, fund=orange) e tamanho proporcional à tensão
- [ ] CA-12.6: Export DOT pode ser renderizado via `dot -Tsvg graph.dot -o graph.svg` sem erros
- [ ] CA-12.7: `argos export --format csv --output tensions.csv` — exporta tabela de tensões (nodeID, tension, degree, sniperGap, score) para análise em planilha

---

## 4. Requisitos de Extensibilidade e Descoberta de Dados

---

### RF-13: Framework de Extensibilidade para Novas Fontes de Dados

**Descrição**: O sistema deve ser projetado para que implementadores (IA ou humano) possam **descobrir e integrar novas fontes de dados** durante a implementação, seguindo critérios rigorosos, sem comprometer a precisão.

**Motivação**: As APIs financeiras evoluem, novos datasets se tornam disponíveis, e durante a implementação o desenvolvedor pode descobrir fontes mais ricas que o planejado. O sistema deve encorajar essa extensão **com governança**.

**Critérios de Aceite**:

#### Governança de Novas Fontes
- [ ] CA-13.1: Toda nova fonte de dados DEVE implementar a interface `DataSource`:
  ```go
  type DataSource interface {
      Name() string
      Description() string
      Ingest(ctx context.Context, params IngestParams) (<-chan Record, <-chan error)
      Validate() error  // Verifica acesso e formato
  }
  ```
- [ ] CA-13.2: Toda nova fonte DEVE ter um `README.md` em `ingest/{source_name}/README.md` documentando:
  - URL da fonte / API
  - Autenticação necessária (se houver)
  - Rate limits
  - Schema dos dados retornados
  - Mapeamento para `itt.Event` (quais campos viram Source, Target, Weight, Type)
  - Justificativa de por que essa fonte aumenta a precisão
- [ ] CA-13.3: Toda nova fonte DEVE ter testes contra fixtures reais (mínimo 3 registros reais)
- [ ] CA-13.4: O impacto de uma nova fonte na precisão DEVE ser medido via validation suite (RF-10 CA-10.8)

#### Fontes Candidatas para Descoberta (Guia ao Implementador)
- [ ] CA-13.5: Durante a implementação, o desenvolvedor DEVE investigar as seguintes fontes adicionais e avaliar viabilidade:

| Fonte | O que buscar | Impacto potencial |
|---|---|---|
| **SEC EDGAR XBRL** | Dados financeiros estruturados (receita, lucro) | Contextualizar transações com performance da empresa |
| **SEC Litigation Releases RSS** | Novos casos de enforcement em tempo real | Auto-atualizar validation suite |
| **FINRA Short Interest** | Posições short em ações | Detectar short selling pré-evento |
| **OpenInsider.com** | Aggregated insider trading (scraping) | Dados pré-processados de insider trading |
| **Unusual Whales / Quiver Quantitative** | Trading de congressistas, lobbying | Rede de influência política |
| **FinancialModelingPrep API** | Fundamentals, earnings surprises | Enriquecer eventos corporativos |
| **Alpha Vantage / Polygon.io** | Market data em tempo real | Correlação preço vs insider trading |
| **Google News API / GDELT** | Notícias sobre empresas | Detecção de eventos não-filed |
| **CUSIP → CIK mapping** | Conversão entre identificadores | Conectar Form 13F holdings ao grafo |
| **SEC EDGAR Full-Text Search** | Busca textual em filings | Relações implícitas entre entidades |

- [ ] CA-13.6: O implementador DEVE documentar fontes investigadas e rejeitadas com justificativa em `docs/rejected-sources.md`

#### Critérios de Aceite para Novas Arestas
- [ ] CA-13.7: Toda nova aresta adicionada ao grafo DEVE obedecer:
  - `Source` e `Target` usam a convenção de ID existente (`insider:`, `company:`, `fund:`)
  - Se introduz novo tipo de nó, documentar em `GRAPH-MODEL.md`
  - `Weight` tem semântica clara (documentada no README da fonte)
  - `Weight` é normalizado/relativo (não valor absoluto)
  - Não introduz arestas com peso = 0 (noise injection)

#### Critérios para Scoring Enhancement
- [ ] CA-13.8: Novas features adicionadas ao scoring (RF-08) DEVEM:
  - Ser aditivas (novo `w_n * feature_n` no score composto)
  - Ter peso default = 0.0 (desabilitado por padrão)
  - Ser testadas individualmente antes de ativar
  - Mostrar melhoria mensurável na validation suite (pelo menos 1 caso a mais detectado, ou falso positivo a menos)

---

## 5. Requisitos de API para Frontend

---

### RF-14: HTTP REST API

**Descrição**: O ARGOS deve expor uma API REST para que aplicações frontend (dashboards, SPAs) consumam dados de análise, grafo e relatórios.

**Critérios de Aceite**:

#### Server
- [ ] CA-14.1: `argos serve --port 8080` inicia o servidor HTTP
- [ ] CA-14.2: Todas as respostas são JSON com `Content-Type: application/json`
- [ ] CA-14.3: Erros retornam `{ "error": "message", "code": "ERROR_CODE" }` com HTTP status adequado
- [ ] CA-14.4: CORS configurável via `argos.yaml` (default: `*` em dev, restrito em prod)
- [ ] CA-14.5: Health check em `GET /api/health` retorna `{ "status": "ok", "uptime": "...", "nodes": N, "edges": N }`

#### Endpoints de Leitura

```
GET  /api/v1/stats
     → EngineStats (nós, arestas, anomalias ativas, uptime)

GET  /api/v1/analysis
     → Results completo da última análise (tensões, anomalias, temporal, detectability)

GET  /api/v1/analysis/anomalies?limit=20&sort=score
     → Top N entidades anômalas com score, tensão, sniperGap, transações

GET  /api/v1/analysis/anomalies/:entityID
     → Detalhe completo de uma entidade (SuspicionCandidate)

GET  /api/v1/nodes?type=insider&limit=100&offset=0
     → Lista paginada de nós com tensão, grau, tipo

GET  /api/v1/nodes/:nodeID
     → Detalhes de um nó (tensão, vizinhos, transações, histórico temporal)

GET  /api/v1/nodes/:nodeID/neighbors?depth=2
     → Subgrafo da vizinhança (k-hop) para visualização focal

GET  /api/v1/nodes/:nodeID/history?window=90d
     → Série temporal de tensão do nó (para gráficos)

GET  /api/v1/graph/export?format=json|dot|csv
     → Export do grafo completo ou filtrado

GET  /api/v1/events?issuer=CIK&since=2024-01-01
     → Corporate events (Form 8-K) para timeline

GET  /api/v1/transactions?owner=CIK&since=2024-01-01
     → Insider transactions para detalhamento
```

#### Endpoints de Ação
```
POST /api/v1/ingest
     Body: { "source": "edgar", "forms": ["4"], "tickers": ["AAPL"], "since": "2024-01-01" }
     → Dispara ingestão assíncrona, retorna job ID

POST /api/v1/analyze
     Body: { "window": "30d", "threshold": 3.0 }
     → Dispara análise, retorna job ID

GET  /api/v1/jobs/:jobID
     → Status do job (pending, running, completed, failed) + progresso
```

- [ ] CA-14.6: Endpoints de leitura respondem em < 200ms para grafos de até 50k nós
- [ ] CA-14.7: Endpoints de ação são assíncronos (retornam imediatamente com job ID)
- [ ] CA-14.8: Paginação via `?limit=N&offset=N` em todos os endpoints de lista
- [ ] CA-14.9: Filtros via query parameters (`?type=insider`, `?sort=tension`, `?min_score=0.7`)

---

### RF-15: WebSocket — Streaming em Tempo Real

**Descrição**: O ARGOS deve expor um WebSocket para streaming de eventos em tempo real, permitindo dashboards reativos.

**Fundamento**: Os callbacks da ITT Engine (`OnAnomaly`, `OnTensionSpike`, `OnChange`) são bridged para o WebSocket, emitindo eventos conforme ocorrem.

**Critérios de Aceite**:

#### Conexão
- [ ] CA-15.1: WebSocket em `ws://localhost:8080/api/v1/ws`
- [ ] CA-15.2: Cliente pode se inscrever em canais: `{ "subscribe": ["anomalies", "spikes", "changes", "stats"] }`
- [ ] CA-15.3: Cliente pode filtrar por entidade: `{ "subscribe": ["anomalies"], "filter": { "entity": "insider:CIK123" } }`

#### Eventos Emitidos

```jsonc
// Canal: anomalies (bridge de OnAnomaly)
{
  "channel": "anomalies",
  "type": "anomaly_detected",
  "data": {
    "node_id": "insider:0001234567",
    "node_name": "JOHN DOE",
    "tension": 0.85,
    "degree": 3,
    "sniper_gap": 4.2,
    "company": "ACME Corp (ACME)",
    "timestamp": "2024-12-15T10:30:00Z"
  }
}

// Canal: spikes (bridge de OnTensionSpike)
{
  "channel": "spikes",
  "type": "tension_spike",
  "data": {
    "node_id": "insider:0001234567",
    "delta": 0.45,
    "previous": 0.30,
    "current": 0.75,
    "timestamp": "2024-12-15T10:30:00Z"
  }
}

// Canal: changes (bridge de OnChange com DeltaTensionChanged)
{
  "channel": "changes",
  "type": "trend_change",
  "data": {
    "node_id": "insider:0001234567",
    "trend": "Increasing",  // Stable | Increasing | Decreasing
    "tension": 0.75,
    "timestamp": "2024-12-15T10:30:00Z"
  }
}

// Canal: stats (emitido periodicamente, ex: a cada 30s)
{
  "channel": "stats",
  "type": "periodic_stats",
  "data": {
    "nodes": 25432,
    "edges": 189234,
    "anomalies_active": 12,
    "mean_tension": 0.23,
    "max_tension": 0.91,
    "events_per_second": 45.2,
    "top_5": [
      { "node_id": "insider:001", "tension": 0.91, "name": "..." },
      ...
    ]
  }
}
```

- [ ] CA-15.4: Heartbeat a cada 30s: `{ "type": "ping" }` → cliente responde `{ "type": "pong" }`
- [ ] CA-15.5: Reconexão automática: se o cliente desconectar e reconectar em < 60s, recebe buffer dos eventos perdidos
- [ ] CA-15.6: Máximo de 100 clientes WebSocket simultâneos (configurável)
- [ ] CA-15.7: Bridge ITT→WS é assíncrono (callbacks não bloqueiam a engine)

---

### RNF-06: Governança de Extensibilidade

**Descrição**: Regras para manter a qualidade ao estender o sistema.

- [ ] Toda mudança que adiciona nova fonte de dados DEVE passar por validation suite antes de merge
- [ ] Toda mudança que altera pesos default de scoring DEVE ser documentada com antes/depois nos resultados de validação
- [ ] Novas fontes que requerem autenticação (API key, OAuth) DEVEM usar `argos.yaml` para configuração, nunca hardcoded
- [ ] Novas fontes que requerem scraping DEVEM respeitar robots.txt e Terms of Service
- [ ] Performance regression: nenhuma nova fonte pode aumentar tempo de ingestão > 2x para o mesmo dataset
