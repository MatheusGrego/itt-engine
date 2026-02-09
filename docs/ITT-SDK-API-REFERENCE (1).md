# ITT SDK - Referência da API

**Versão:** 1.0.0-draft  
**Data:** Janeiro 2025  

---

## 1. Builder API

### 1.1 Criação

```go
engine := itt.NewBuilder().Build()
```

Cria engine com todos os defaults. Funcional imediatamente.

### 1.2 Métodos do Builder

#### Configuração de Algoritmos

| Método | Default | Descrição |
|--------|---------|-----------|
| `.Divergence(DivergenceFunc)` | JSD | Função de divergência para cálculo de tensão |
| `.Curvature(CurvatureFunc)` | Ollivier-Ricci | Função de curvatura |
| `.Topology(TopologyFunc)` | nil (desabilitado) | Função de homologia persistente |
| `.Threshold(float64)` | 0.2 | Limiar para classificar como anomalia |
| `.ThresholdFunc(ThresholdFunc)` | nil | Função customizada de threshold |

#### Configuração de Pesos

| Método | Default | Descrição |
|--------|---------|-----------|
| `.WeightFunc(WeightFunc)` | nil (usa Event.Weight) | Função para calcular peso de arestas |
| `.NodeTypeFunc(NodeTypeFunc)` | nil (tipo = "default") | Função para extrair tipo do nó |
| `.AggregationFunc(AggregationFunc)` | Mean | Como agregar tensão de região |

#### Configuração MVCC e Memória

| Método | Default | Descrição |
|--------|---------|-----------|
| `.GCSnapshotWarning(time.Duration)` | 5 min | Tempo até warning de snapshot não fechado |
| `.GCSnapshotForce(time.Duration)` | 15 min | Tempo até forçar close de snapshot |
| `.MaxOverlaySize(int)` | 100000 | Eventos no overlay antes de compactação |

#### Configuração de Compactação

| Método | Default | Descrição |
|--------|---------|-----------|
| `.CompactionStrategy(Strategy)` | ByVolume | Estratégia de compactação |
| `.CompactionThreshold(int)` | 10000 | Threshold para compactação por volume |
| `.CompactionInterval(time.Duration)` | 0 (desabilitado) | Intervalo para compactação temporal |

#### Callbacks

| Método | Default | Descrição |
|--------|---------|-----------|
| `.OnChange(func(Delta))` | nil | Chamado a cada mudança no grafo |
| `.OnAnomaly(func(TensionResult))` | nil | Chamado quando anomalia detectada |
| `.OnCompact(func(CompactStats))` | nil | Chamado após compactação |
| `.OnGC(func(GCStats))` | nil | Chamado após GC de versão |
| `.OnError(func(error))` | nil | Chamado em erros recuperáveis |

#### Observabilidade

| Método | Default | Descrição |
|--------|---------|-----------|
| `.Logger(Logger)` | nil (silencioso) | Logger para observabilidade |

#### Storage

| Método | Default | Descrição |
|--------|---------|-----------|
| `.Storage(Storage)` | nil (só memória) | Interface de persistência externa |
| `.BaseGraph(*GraphData)` | nil | Grafo base inicial |

### 1.3 Exemplo Completo

```go
engine := itt.NewBuilder().
    // Algoritmos
    Divergence(itt.JSD).
    Curvature(itt.OllivierRicci).
    Threshold(0.15).
    
    // Pesos customizados
    WeightFunc(func(e itt.Event) float64 {
        if e.Type == "transfer" {
            amount, _ := e.Metadata["amount"].(float64)
            return amount
        }
        return 1.0
    }).
    
    // MVCC
    GCSnapshotWarning(3 * time.Minute).
    GCSnapshotForce(10 * time.Minute).
    MaxOverlaySize(50000).
    
    // Compactação
    CompactionStrategy(itt.CompactByVolume).
    CompactionThreshold(20000).
    
    // Callbacks
    OnChange(func(d itt.Delta) {
        websocket.Broadcast(d)
    }).
    OnAnomaly(func(r itt.TensionResult) {
        alert.Fire(r.NodeID, r.Tension)
    }).
    
    // Observabilidade
    Logger(slogAdapter{slog.Default()}).
    
    Build()
```

### 1.4 Validação no Build

`Build()` valida configuração e retorna erro se inválida:

```go
engine, err := itt.NewBuilder().
    Threshold(-0.5).  // inválido: threshold negativo
    Build()

if err != nil {
    // err = "threshold must be >= 0"
}
```

---

## 2. Engine Interface

### 2.1 Contrato

```go
type Engine interface {
    // Ciclo de vida
    Start(ctx context.Context) error
    Stop() error
    Running() bool
    
    // Ingestão
    AddEvent(event Event) error
    AddEvents(events []Event) error
    
    // Estado
    Snapshot() *Snapshot
    Stats() *EngineStats
    
    // Análise direta (usa snapshot interno)
    Analyze() (*Results, error)
    AnalyzeNode(nodeID string) (*TensionResult, error)
    AnalyzeRegion(nodeIDs []string) (*RegionResult, error)
    
    // Controle
    Compact() error
    Reset() error
}
```

### 2.2 Ciclo de Vida

#### Start

```go
func (e *Engine) Start(ctx context.Context) error
```

**Comportamento:**
- Inicia goroutines internas (write worker, GC)
- Context controla shutdown
- Retorna erro se já iniciado
- Pode ser omitido - auto-start no primeiro AddEvent

**Garantias:**
- `ctx.Done()` causa shutdown graceful
- Todos os eventos pendentes são processados antes de parar
- Snapshots abertos continuam válidos após Stop

#### Stop

```go
func (e *Engine) Stop() error
```

**Comportamento:**
- Para de aceitar novos eventos
- Aguarda processamento de eventos pendentes
- Fecha goroutines internas
- NÃO fecha snapshots abertos

#### Running

```go
func (e *Engine) Running() bool
```

Retorna `true` se engine está processando eventos.

### 2.3 Ingestão

#### AddEvent

```go
func (e *Engine) AddEvent(event Event) error
```

**Contrato:**
- DEVE ser thread-safe
- DEVE auto-start se engine não iniciada
- DEVE retornar erro se evento inválido
- DEVE retornar erro se engine parada
- NÃO DEVE bloquear por mais de 1ms em condições normais

**Comportamento:**
1. Valida evento (Source e Target não vazios, Weight >= 0)
2. Envia para canal interno
3. Worker processa assincronamente
4. Callback OnChange invocado após processamento

**Erros:**
- `ErrEmptySource` - Source vazio
- `ErrEmptyTarget` - Target vazio
- `ErrNegativeWeight` - Weight < 0
- `ErrEngineStopped` - Engine não está rodando

#### AddEvents

```go
func (e *Engine) AddEvents(events []Event) error
```

**Contrato:**
- Semântica de batch - todos ou nenhum
- Se qualquer evento inválido, retorna erro sem processar nenhum
- Mais eficiente que múltiplos AddEvent para grandes volumes

### 2.4 Estado

#### Snapshot

```go
func (e *Engine) Snapshot() *Snapshot
```

**Contrato:**
- DEVE retornar snapshot imutável do estado atual
- DEVE incrementar reference count da versão
- NUNCA retorna nil (mesmo se grafo vazio)
- Caller DEVE chamar `Close()` quando terminar

**Comportamento:**
- Captura ponteiro atômico (~5ns)
- Snapshot vê estado no momento da captura
- Modificações posteriores não afetam snapshot

#### Stats

```go
func (e *Engine) Stats() *EngineStats

type EngineStats struct {
    Nodes           int
    Edges           int
    OverlayEvents   int
    BaseNodes       int
    BaseEdges       int
    VersionsCurrent uint64
    VersionsTotal   uint64
    SnapshotsActive int
    EventsTotal     int64
    EventsPerSecond float64
    Uptime          time.Duration
}
```

### 2.5 Análise

#### Analyze

```go
func (e *Engine) Analyze() (*Results, error)
```

**Contrato:**
- Cria snapshot interno, analisa, fecha
- Retorna todos os nós com tensão calculada
- Conveniência - equivalente a `snap := Snapshot(); defer snap.Close(); return snap.Analyze()`

#### AnalyzeNode

```go
func (e *Engine) AnalyzeNode(nodeID string) (*TensionResult, error)
```

**Contrato:**
- Calcula tensão de um único nó
- Retorna `ErrNodeNotFound` se nó não existe
- Mais eficiente que `Analyze()` para consultas pontuais

#### AnalyzeRegion

```go
func (e *Engine) AnalyzeRegion(nodeIDs []string) (*RegionResult, error)

type RegionResult struct {
    Nodes          []TensionResult
    MeanTension    float64
    MaxTension     float64
    AnomalyCount   int
    Aggregated     float64  // via AggregationFunc
}
```

**Contrato:**
- Analisa subconjunto de nós
- Retorna estatísticas agregadas da região
- Nós não encontrados são ignorados (não erro)

### 2.6 Controle

#### Compact

```go
func (e *Engine) Compact() error
```

**Contrato:**
- Força compactação do Overlay no Base
- Bloqueia até compactação completa
- Callback OnCompact invocado após

#### Reset

```go
func (e *Engine) Reset() error
```

**Contrato:**
- Remove todos os dados (Base e Overlay)
- Mantém configuração
- Snapshots existentes continuam válidos (MVCC)

---

## 3. Snapshot Interface

### 3.1 Contrato

```go
type Snapshot struct {
    // Não expõe campos - apenas métodos
}

func (s *Snapshot) ID() string
func (s *Snapshot) Version() uint64
func (s *Snapshot) Timestamp() time.Time
func (s *Snapshot) Close() error

// Consultas
func (s *Snapshot) NodeCount() int
func (s *Snapshot) EdgeCount() int
func (s *Snapshot) GetNode(id string) (*Node, bool)
func (s *Snapshot) GetEdge(from, to string) (*Edge, bool)
func (s *Snapshot) Neighbors(nodeID string) []string
func (s *Snapshot) InNeighbors(nodeID string) []string
func (s *Snapshot) OutNeighbors(nodeID string) []string

// Análise
func (s *Snapshot) Analyze() (*Results, error)
func (s *Snapshot) AnalyzeNode(nodeID string) (*TensionResult, error)
func (s *Snapshot) AnalyzeRegion(nodeIDs []string) (*RegionResult, error)
func (s *Snapshot) GetTension(nodeID string) (float64, error)
func (s *Snapshot) GetCurvature(from, to string) (float64, error)

// Iteração
func (s *Snapshot) ForEachNode(fn func(*Node) bool)
func (s *Snapshot) ForEachEdge(fn func(*Edge) bool)

// Exportação
func (s *Snapshot) Export(format ExportFormat) ([]byte, error)
```

### 3.2 Ciclo de Vida

```go
snap := engine.Snapshot()  // cria
defer snap.Close()         // SEMPRE fechar

// usar...
results := snap.Analyze()
```

**Garantia:** Snapshot fechado não pode ser usado. Métodos retornam `ErrSnapshotClosed`.

### 3.3 Consultas

#### GetNode

```go
func (s *Snapshot) GetNode(id string) (*Node, bool)
```

Retorna nó e `true` se existe, `nil` e `false` se não.

#### Neighbors

```go
func (s *Snapshot) Neighbors(nodeID string) []string
```

Retorna IDs de todos os vizinhos (in + out). Ordem não garantida.

### 3.4 Análise

Métodos de análise no Snapshot são idênticos aos da Engine, mas operam na versão capturada.

### 3.5 Iteração

#### ForEachNode

```go
func (s *Snapshot) ForEachNode(fn func(*Node) bool)
```

Itera sobre todos os nós. Retornar `false` do callback para parada.

```go
snap.ForEachNode(func(n *itt.Node) bool {
    if n.Degree > 100 {
        fmt.Println("Hub:", n.ID)
    }
    return true  // continuar
})
```

### 3.6 Exportação

```go
type ExportFormat int

const (
    ExportJSON ExportFormat = iota
    ExportGraphML
    ExportGEXF
    ExportDOT
)

func (s *Snapshot) Export(format ExportFormat) ([]byte, error)
```

Exporta grafo completo no formato especificado.

---

## 4. Interfaces Plugáveis

### 4.1 DivergenceFunc

```go
type DivergenceFunc interface {
    // Compute calcula divergência entre duas distribuições
    // p e q são distribuições de probabilidade (somam 1.0)
    // Retorna valor >= 0
    Compute(p, q []float64) float64
    
    // Name retorna identificador para logs/métricas
    Name() string
}
```

**Implementações built-in:**
- `itt.JSD` - Jensen-Shannon Divergence (default)
- `itt.KL` - Kullback-Leibler
- `itt.Hellinger` - Hellinger Distance

**Exemplo custom:**

```go
type WassersteinDivergence struct {
    // config...
}

func (w WassersteinDivergence) Compute(p, q []float64) float64 {
    // implementar Earth Mover's Distance
    return emd(p, q)
}

func (w WassersteinDivergence) Name() string {
    return "wasserstein"
}

// Uso
engine := itt.NewBuilder().
    Divergence(WassersteinDivergence{}).
    Build()
```

### 4.2 BatchDivergenceFunc

```go
type DistributionPair struct {
    P []float64
    Q []float64
}

type BatchDivergenceFunc interface {
    DivergenceFunc
    
    // ComputeBatch calcula divergência para múltiplos pares
    // Implementações GPU podem paralelizar
    ComputeBatch(pairs []DistributionPair) []float64
    
    // SupportsBatch indica se batch é mais eficiente que loop
    SupportsBatch() bool
}
```

**Nota:** Interface definida para futuras implementações GPU. Built-ins não implementam batch otimizado v1.

### 4.3 WeightFunc

```go
type WeightFunc func(event Event) float64
```

Calcula peso de uma aresta a partir do evento.

**Exemplo:**

```go
// Peso baseado em tipo e metadata
weightFunc := func(e itt.Event) float64 {
    base := 1.0
    switch e.Type {
    case "transfer":
        if amount, ok := e.Metadata["amount"].(float64); ok {
            return amount
        }
    case "mint":
        return base * 2.0  // mint tem peso maior
    case "burn":
        return base * 1.5
    }
    return base
}

engine := itt.NewBuilder().
    WeightFunc(weightFunc).
    Build()
```

### 4.4 NodeTypeFunc

```go
type NodeTypeFunc func(nodeID string) string
```

Extrai tipo do nó a partir do ID.

**Exemplo:**

```go
// IDs no formato "tipo:identificador"
nodeTypeFunc := func(id string) string {
    parts := strings.SplitN(id, ":", 2)
    if len(parts) == 2 {
        return parts[0]  // "wallet", "token", "pool"
    }
    return "unknown"
}
```

### 4.5 ThresholdFunc

```go
type ThresholdFunc func(node *Node, tension float64) bool
```

Determina se um nó é anomalia. Permite threshold dinâmico.

**Exemplo:**

```go
// Threshold dinâmico baseado no grau
thresholdFunc := func(n *itt.Node, tension float64) bool {
    // Nós de baixo grau precisam de tensão maior para serem anomalia
    if n.Degree < 5 {
        return tension > 0.4
    }
    // Hubs: tensão menor já é significativa
    if n.Degree > 100 {
        return tension > 0.1
    }
    return tension > 0.2
}
```

### 4.6 AggregationFunc

```go
type AggregationFunc func(tensions []float64) float64
```

Agrega tensões de uma região em valor único.

**Implementações built-in:**
- `itt.Mean` - média aritmética (default)
- `itt.Max` - máximo
- `itt.Median` - mediana
- `itt.Sum` - soma

### 4.7 CurvatureFunc

```go
type CurvatureFunc interface {
    // Compute calcula curvatura de uma aresta
    Compute(g GraphView, from, to string) float64
    Name() string
}

type GraphView interface {
    GetNode(id string) (*Node, bool)
    GetEdge(from, to string) (*Edge, bool)
    Neighbors(nodeID string) []string
    // ... subset de métodos do Snapshot
}
```

**Built-in:** `itt.OllivierRicci`

### 4.8 TopologyFunc

```go
type TopologyResult struct {
    Betti0 int  // componentes conectados
    Betti1 int  // ciclos independentes
    // ...
}

type TopologyFunc interface {
    Compute(g GraphView) TopologyResult
    Name() string
}
```

### 4.9 Storage

```go
type GraphData struct {
    Nodes     []*Node
    Edges     []*Edge
    Metadata  map[string]any
    Timestamp time.Time
}

type Storage interface {
    Load() (*GraphData, error)
    Save(data *GraphData) error
}
```

**Exemplo implementação SQLite:**

```go
type SQLiteStorage struct {
    db *sql.DB
}

func (s *SQLiteStorage) Load() (*itt.GraphData, error) {
    // SELECT * FROM nodes...
    // SELECT * FROM edges...
}

func (s *SQLiteStorage) Save(data *itt.GraphData) error {
    // INSERT/UPDATE nodes...
    // INSERT/UPDATE edges...
}
```

### 4.10 Logger

```go
type Logger interface {
    Debug(msg string, keysAndValues ...any)
    Info(msg string, keysAndValues ...any)
    Warn(msg string, keysAndValues ...any)
    Error(msg string, keysAndValues ...any)
}
```

**Adapter para slog:**

```go
type SlogAdapter struct {
    logger *slog.Logger
}

func (a SlogAdapter) Debug(msg string, kv ...any) {
    a.logger.Debug(msg, kv...)
}
// ... outros métodos
```

### 4.11 Calibrator

```go
type Calibrator interface {
    // Observe registra uma tensão observada para calibração
    Observe(tension float64)
    
    // IsWarmedUp retorna true se calibração inicial completa
    IsWarmedUp() bool
    
    // Threshold retorna o threshold atual calculado
    Threshold() float64
    
    // IsAnomaly verifica se tensão excede threshold
    IsAnomaly(tension float64) bool
    
    // Stats retorna estatísticas de calibração
    Stats() CalibratorStats
    
    // Recalibrate força recálculo do threshold
    Recalibrate()
}

type CalibratorStats struct {
    SamplesObserved int
    Median          float64
    MAD             float64
    Threshold       float64
    K               float64
    IsWarmedUp      bool
    LastRecalibration time.Time
}
```

**Builder do Calibrator default:**

```go
calibrator := itt.NewCalibrator(
    itt.WithK(3.0),                    // sensibilidade (default 3.0)
    itt.WithWarmupSize(1000),          // amostras para warm-up
    itt.WithMinDegree(5),              // grau mínimo para considerar
    itt.WithRecalibrationInterval(0),  // 0 = desabilitado
    itt.WithSmoothing(5.0),            // lambda para regularização
)
```

**Com baseline precomputado:**

```go
calibrator := itt.NewCalibrator(
    itt.WithPrecomputedBaseline(0.15, 0.08), // median, MAD conhecidos
)
```

**Exemplo custom:**

```go
type MyCalibrator struct {
    // lógica específica do domínio
}

func (c *MyCalibrator) Observe(tension float64) {
    // implementação custom
}

func (c *MyCalibrator) IsAnomaly(tension float64) bool {
    // lógica específica
}

// ... outros métodos

engine := itt.NewBuilder().
    Calibrator(&MyCalibrator{}).
    Build()
```

---

## 5. Types Auxiliares

### 5.1 Results

```go
type Results struct {
    Tensions     []TensionResult
    Anomalies    []TensionResult  // filtrado por threshold
    Stats        ResultStats
    SnapshotID   string
    AnalyzedAt   time.Time
    Duration     time.Duration
}

type ResultStats struct {
    NodesAnalyzed  int
    MeanTension    float64
    MedianTension  float64
    MaxTension     float64
    StdDevTension  float64
    AnomalyCount   int
    AnomalyRate    float64  // AnomalyCount / NodesAnalyzed
}
```

### 5.2 Delta

```go
type Delta struct {
    Type      DeltaType
    Timestamp time.Time
    Version   uint64
    
    // Para nós
    NodeID    string
    Node      *Node  // nil se remoção
    
    // Para arestas
    EdgeFrom  string
    EdgeTo    string
    Edge      *Edge  // nil se remoção
    
    // Para tensão
    Tension   float64
    Previous  float64  // tensão anterior
    
    // Extensível
    Data      map[string]any
}

type DeltaType int

const (
    DeltaNodeAdded DeltaType = iota
    DeltaNodeUpdated
    DeltaNodeRemoved
    DeltaEdgeAdded
    DeltaEdgeUpdated
    DeltaEdgeRemoved
    DeltaTensionChanged
    DeltaAnomalyDetected
    DeltaAnomalyResolved
)
```

### 5.3 CompactStats

```go
type CompactStats struct {
    NodesMerged   int
    EdgesMerged   int
    OverlayBefore int
    OverlayAfter  int
    Duration      time.Duration
    Timestamp     time.Time
}
```

### 5.4 GCStats

```go
type GCStats struct {
    VersionsRemoved int
    MemoryFreed     int64  // bytes estimados
    OldestRemoved   uint64 // version ID
    Timestamp       time.Time
}
```

### 5.5 Erros

```go
var (
    // Validação
    ErrEmptySource    = errors.New("event source cannot be empty")
    ErrEmptyTarget    = errors.New("event target cannot be empty")
    ErrNegativeWeight = errors.New("event weight cannot be negative")
    
    // Estado
    ErrEngineStopped  = errors.New("engine is not running")
    ErrEngineRunning  = errors.New("engine is already running")
    ErrSnapshotClosed = errors.New("snapshot is closed")
    
    // Consulta
    ErrNodeNotFound   = errors.New("node not found")
    ErrEdgeNotFound   = errors.New("edge not found")
    
    // Configuração
    ErrInvalidConfig  = errors.New("invalid configuration")
)
```

---

## 6. Exemplos de Uso

### 6.1 Uso Mínimo

```go
package main

import "github.com/user/itt"

func main() {
    engine := itt.NewBuilder().Build()
    
    engine.AddEvent(itt.Event{
        Source: "user:alice",
        Target: "repo:myproject",
        Type:   "commit",
    })
    
    results, _ := engine.Analyze()
    for _, r := range results.Anomalies {
        fmt.Printf("Anomaly: %s (τ=%.3f)\n", r.NodeID, r.Tension)
    }
}
```

### 6.2 Streaming com Callbacks

```go
func main() {
    engine := itt.NewBuilder().
        OnChange(func(d itt.Delta) {
            json.NewEncoder(os.Stdout).Encode(d)
        }).
        OnAnomaly(func(r itt.TensionResult) {
            alert(r.NodeID, r.Tension)
        }).
        Build()
    
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    engine.Start(ctx)
    
    // Stream de eventos
    for event := range eventSource {
        engine.AddEvent(event)
    }
}
```

### 6.3 Análise com Snapshot

```go
func analyzeWithSnapshot(engine *itt.Engine) {
    snap := engine.Snapshot()
    defer snap.Close()
    
    // Análise pode demorar - não bloqueia ingestão
    results, _ := snap.Analyze()
    
    // Exportar para visualização
    graphJSON, _ := snap.Export(itt.ExportJSON)
    saveToFile("graph.json", graphJSON)
    
    // Análise de região específica
    hubs := findHighDegreeNodes(snap)
    regionResult, _ := snap.AnalyzeRegion(hubs)
    fmt.Printf("Hub region mean tension: %.3f\n", regionResult.MeanTension)
}
```

### 6.4 Base + Overlay

```go
func main() {
    // Carregar base de storage
    storage := NewSQLiteStorage("graph.db")
    
    engine := itt.NewBuilder().
        Storage(storage).
        CompactionStrategy(itt.CompactByVolume).
        CompactionThreshold(50000).
        OnCompact(func(s itt.CompactStats) {
            log.Printf("Compacted %d nodes to base", s.NodesMerged)
        }).
        Build()
    
    // Carregar base existente
    engine.LoadBase()
    
    // Novos eventos vão para overlay
    for event := range liveStream {
        engine.AddEvent(event)
    }
    
    // Compactação manual se necessário
    engine.Compact()
}
```

### 6.5 Customização Completa

```go
func main() {
    engine := itt.NewBuilder().
        // Divergência customizada
        Divergence(MyWassersteinDivergence{epsilon: 0.01}).
        
        // Peso baseado em metadata
        WeightFunc(func(e itt.Event) float64 {
            if amount, ok := e.Metadata["amount"].(float64); ok {
                return math.Log1p(amount)  // log scale
            }
            return 1.0
        }).
        
        // Tipo de nó pelo prefixo do ID
        NodeTypeFunc(func(id string) string {
            return strings.Split(id, ":")[0]
        }).
        
        // Threshold dinâmico
        ThresholdFunc(func(n *itt.Node, t float64) bool {
            // Hubs: threshold menor
            if n.Degree > 50 {
                return t > 0.1
            }
            return t > 0.25
        }).
        
        // Agregação por máximo
        AggregationFunc(itt.Max).
        
        // Logger
        Logger(NewZapAdapter(zap.L())).
        
        Build()
    
    // ...
}
```

---

## 7. Thread Safety

### 7.1 Garantias

| Operação | Thread-Safe | Notas |
|----------|-------------|-------|
| `AddEvent` | ✅ Sim | Múltiplos produtores ok |
| `AddEvents` | ✅ Sim | Batch atômico |
| `Snapshot` | ✅ Sim | Captura atômica |
| `Analyze` | ✅ Sim | Usa snapshot interno |
| `Snapshot.Analyze` | ✅ Sim | Snapshot é imutável |
| `Snapshot.Close` | ✅ Sim | Idempotente |
| `Compact` | ✅ Sim | Bloqueia até completar |
| `Stats` | ✅ Sim | Leitura atômica |

### 7.2 Padrões Recomendados

**Múltiplos produtores:**
```go
var wg sync.WaitGroup
for i := 0; i < numWorkers; i++ {
    wg.Add(1)
    go func(events []itt.Event) {
        defer wg.Done()
        for _, e := range events {
            engine.AddEvent(e)  // safe
        }
    }(partition[i])
}
wg.Wait()
```

**Análise em background:**
```go
go func() {
    ticker := time.NewTicker(1 * time.Minute)
    for range ticker.C {
        snap := engine.Snapshot()
        results, _ := snap.Analyze()
        publishResults(results)
        snap.Close()  // importante!
    }
}()
```

---

## 8. Performance Considerations

### 8.1 Ingestão

- Eventos são bufferizados em canal interno
- Tamanho do canal: 10000 (configurável)
- Se canal cheio, `AddEvent` bloqueia (backpressure)

### 8.2 Análise

- Análise é O(n * m) onde n = nós, m = média de vizinhos
- Snapshots permitem análise paralela à ingestão
- Análise de região é mais eficiente que análise completa

### 8.3 Memória

- MVCC mantém múltiplas versões
- GC remove versões não utilizadas
- Compactação controla crescimento do overlay

### 8.4 Recomendações

| Cenário | Recomendação |
|---------|--------------|
| Alta ingestão | Usar `AddEvents` em batch |
| Análise frequente | Reutilizar snapshot para múltiplas consultas |
| Memória limitada | Reduzir `MaxOverlaySize`, compactar frequentemente |
| Análise longa | Usar snapshot em goroutine separada |
