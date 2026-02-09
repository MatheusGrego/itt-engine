# ITT SDK - Documento de Arquitetura

**Versão:** 1.0.0-draft  
**Data:** Janeiro 2025  
**Baseado em:** Grego, M. "Informational Tension Theory: A Framework for Entropic Detection of Latent Structures" (2025)

---

## 1. Visão Geral

### 1.1 Propósito

A ITT SDK é uma biblioteca Go para detecção de anomalias em grafos usando Informational Tension Theory. Ela permite identificar nós "ocultos" ou anômalos através da medição de deformação entrópica nas vizinhanças do grafo.

### 1.2 Filosofia de Design

**Princípio Central:** Funciona sem configuração, mas abre tudo para customização.

| Princípio | Descrição |
|-----------|-----------|
| **Agnóstica ao domínio** | SDK não conhece Solana, NPM, ou ecologia. Entende grafos, eventos, e tensão. Domínio é responsabilidade do usuário. |
| **Zero-config funcional** | `itt.NewBuilder().Build()` retorna engine funcional com defaults sensatos. |
| **Observabilidade opt-in** | SDK é silenciosa por padrão. Logger e hooks são injetados por quem precisa. |
| **Performance sem sacrificar clareza** | MVCC internamente, API simples externamente. |

### 1.3 Casos de Uso

- Análise batch de datasets históricos
- Monitoramento streaming em tempo real
- Híbrido: baseline histórico + eventos recentes
- Exportação para dashboards/visualização

### 1.4 Objetivos v1

| Incluído | Descrição |
|----------|-----------|
| ✅ | Divergência (JSD, KL, Hellinger) |
| ✅ | Curvatura de Ollivier-Ricci |
| ✅ | Homologia Persistente (detecção topológica) |
| ✅ | MVCC para concorrência |
| ✅ | Base + Overlay com view unificada |
| ✅ | Streaming de deltas para visualização |
| ✅ | Interfaces plugáveis para todos os algoritmos |

### 1.5 Não-Objetivos v1

| Excluído | Justificativa |
|----------|---------------|
| ❌ Implementação GPU | Apenas interface `BatchDivergenceFunc` definida |
| ❌ Persistência embutida | Apenas interface `Storage` definida |
| ❌ Visualização embutida | SDK exporta deltas, consumidor visualiza |

---

## 2. Modelo de Dados

### 2.1 Event

Unidade atômica de ingestão. Representa uma interação/relação no domínio.

```go
type Event struct {
    Source    string
    Target    string
    Type      string
    Weight    float64
    Timestamp time.Time
    Metadata  map[string]any
}
```

| Campo | Obrigatório | Descrição |
|-------|-------------|-----------|
| `Source` | Sim | ID do nó origem |
| `Target` | Sim | ID do nó destino |
| `Type` | Não | Tipo semântico da aresta (definido pelo domínio) |
| `Weight` | Não | Peso da interação. Default: 1.0 |
| `Timestamp` | Não | Momento da interação. Default: time.Now() |
| `Metadata` | Não | Dados arbitrários do domínio |

**Invariantes:**
- `Source` e `Target` NÃO PODEM ser strings vazias
- `Weight` DEVE ser >= 0
- `Source` PODE ser igual a `Target` (self-loop)

### 2.2 Node

Vértice no grafo de informação.

```go
type Node struct {
    ID         string
    Type       string
    Degree     int
    InDegree   int
    OutDegree  int
    Attributes map[string]float64
    FirstSeen  time.Time
    LastSeen   time.Time
}
```

| Campo | Calculado | Descrição |
|-------|-----------|-----------|
| `ID` | Não | Identificador único (fornecido via Event) |
| `Type` | Não | Tipo semântico (via `NodeTypeFunc` ou extraído do ID) |
| `Degree` | Sim | InDegree + OutDegree |
| `InDegree` | Sim | Arestas entrando no nó |
| `OutDegree` | Sim | Arestas saindo do nó |
| `Attributes` | Não | Estado φ(v) da teoria - extensível |
| `FirstSeen` | Sim | Primeiro evento envolvendo este nó |
| `LastSeen` | Sim | Último evento envolvendo este nó |

**Invariantes:**
- `Degree` SEMPRE igual a `InDegree + OutDegree`
- `FirstSeen` SEMPRE <= `LastSeen`
- `ID` é imutável após criação

### 2.3 Edge

Aresta direcionada com peso acumulado.

```go
type Edge struct {
    From      string
    To        string
    Weight    float64
    Type      string
    Count     int
    FirstSeen time.Time
    LastSeen  time.Time
}
```

| Campo | Calculado | Descrição |
|-------|-----------|-----------|
| `From` | Não | ID do nó origem |
| `To` | Não | ID do nó destino |
| `Weight` | Acumulado | Soma dos pesos de todas as interações |
| `Type` | Não | Tipo da primeira interação (ou mais recente - configurável) |
| `Count` | Sim | Número de interações nesta aresta |
| `FirstSeen` | Sim | Primeira interação |
| `LastSeen` | Sim | Última interação |

**Invariantes:**
- `Count` >= 1
- `Weight` >= 0
- Par `(From, To)` é único no grafo

### 2.4 TensionResult

Resultado da análise de tensão para um nó.

```go
type TensionResult struct {
    NodeID     string
    Tension    float64
    Degree     int
    Curvature  float64
    Anomaly    bool
    Confidence float64
    Components map[string]float64
}
```

| Campo | Descrição |
|-------|-----------|
| `NodeID` | Identificador do nó analisado |
| `Tension` | τ(v) normalizado em [0, 1] |
| `Degree` | Grau do nó no momento da análise |
| `Curvature` | Curvatura média de Ollivier-Ricci das arestas adjacentes |
| `Anomaly` | `true` se τ > threshold configurado |
| `Confidence` | Confiança estatística da detecção [0, 1] |
| `Components` | Breakdown por tipo de medida (divergence, curvature, etc.) |

### 2.5 Delta

Representa uma mudança no grafo para streaming.

```go
type Delta struct {
    Type      DeltaType
    Timestamp time.Time
    NodeID    string
    EdgeFrom  string
    EdgeTo    string
    Tension   float64
    Data      map[string]any
}

type DeltaType int

const (
    DeltaNodeAdded DeltaType = iota
    DeltaNodeRemoved
    DeltaEdgeAdded
    DeltaEdgeUpdated
    DeltaEdgeRemoved
    DeltaTensionChanged
    DeltaAnomalyDetected
)
```

**Uso:** Callbacks `OnChange` recebem deltas. Consumidor (dashboard, API) reconstrói estado a partir do stream.

### 2.6 Snapshot

Fotografia imutável do estado do grafo em um momento.

```go
type Snapshot struct {
    ID        string
    Version   uint64
    Timestamp time.Time
    // Métodos de acesso - não expõe estrutura interna
}
```

**Comportamento:**
- Snapshot é read-only
- Operações de análise trabalham no snapshot, não no grafo atual
- DEVE chamar `Close()` quando terminar (libera versão para GC)
- Timeout de segurança libera snapshots abandonados

---

## 3. Arquitetura de Camadas

### 3.1 Visão Geral

```
┌─────────────────────────────────────────────────────────────────┐
│                        Usuário                                  │
├─────────────────────────────────────────────────────────────────┤
│                     API Pública                                 │
│            (Builder, Engine, Snapshot, Results)                 │
├─────────────────────────────────────────────────────────────────┤
│                    View Unificada                               │
│              (Base + Overlay como um só)                        │
├───────────────────────┬─────────────────────────────────────────┤
│     Base Graph        │           Overlay Graph                 │
│  (Storage/Imutável)   │         (MVCC/Hot Data)                 │
├───────────────────────┴─────────────────────────────────────────┤
│                   Storage Interface                             │
│                (Opcional - pode ser nil)                        │
└─────────────────────────────────────────────────────────────────┘
```

### 3.2 Storage Interface (Layer 0)

Interface para persistência externa. Totalmente opcional.

```go
type Storage interface {
    LoadGraph() (*GraphData, error)
    SaveGraph(data *GraphData) error
}
```

**Comportamento:**
- Se `nil`, SDK opera 100% em memória
- Usuário implementa para SQLite, Redis, arquivo, etc.
- SDK não fornece implementações v1

### 3.3 Base Graph (Layer 1)

Grafo de referência, tipicamente histórico/consolidado.

**Características:**
- Carregado do Storage ou construído incrementalmente
- Atualizado raramente (manual ou por compactação)
- Pode ser `nil` (domínios puramente streaming)
- Imutável durante operações normais

**Quando usar:**
| Domínio | Base contém |
|---------|-------------|
| NPM | Registry completo de pacotes |
| Solana | Transações consolidadas |
| Ecologia | Dataset histórico |
| Ações | Histórico de preços/correlações |

### 3.4 Overlay Graph (Layer 2)

Eventos recentes, versionado com MVCC.

**Características:**
- Recebe todos os `AddEvent()`
- Versionado para snapshot isolation
- Compactado periodicamente para o Base
- GC remove versões antigas

### 3.5 View Unificada

Análise vê Base + Overlay como grafo único.

**Regra de merge:**
- Nó existe em ambos → Overlay vence (dados mais recentes)
- Aresta existe em ambos → Pesos são SOMADOS
- Conflito de tipo → Overlay vence

---

## 4. Arquitetura MVCC

### 4.1 Problema

Cenário: múltiplos produtores adicionando eventos enquanto consumidores analisam.

Com locks tradicionais:
- RWMutex: leituras paralelas, mas escritas bloqueiam tudo
- Contenção cresce com número de goroutines
- Análise longa bloqueia ingestão

### 4.2 Solução: Snapshot Isolation

```
Escritor:
  1. NÃO modifica grafo atual
  2. Cria NOVA VERSÃO do grafo
  3. Atualiza ponteiro atômico para nova versão

Leitor:
  1. Captura ponteiro atual (atômico, ~5ns)
  2. Trabalha na versão capturada
  3. Versão é imutável - zero contenção
```

### 4.3 Estrutura de Versões

```
┌──────────────────────────────────────────────────────────────┐
│                        Engine                                 │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  current ──────────────────────────────────┐                 │
│  (atomic.Pointer)                          ▼                 │
│                                    ┌──────────────┐          │
│                                    │ Version N    │          │
│                                    │ (current)    │          │
│                                    └──────────────┘          │
│                                           ▲                  │
│  ┌──────────────┐  ┌──────────────┐       │                  │
│  │ Version N-2  │  │ Version N-1  │       │                  │
│  │ (orphan)     │  │ (in use)     │       │                  │
│  └──────────────┘  └──────────────┘       │                  │
│         │                 ▲               │                  │
│         │                 │               │                  │
│         ▼           Snapshot A      Snapshot B               │
│        GC                                                    │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

### 4.4 GraphVersion

```go
type GraphVersion struct {
    ID        uint64
    Graph     *ImmutableGraph
    Timestamp time.Time
    Dirty     map[string]bool
    RefCount  int64
}
```

| Campo | Descrição |
|-------|-----------|
| `ID` | Identificador monotônico da versão |
| `Graph` | Estrutura imutável do grafo |
| `Timestamp` | Momento de criação da versão |
| `Dirty` | Nós que mudaram desde versão anterior |
| `RefCount` | Número de snapshots usando esta versão |

### 4.5 Ciclo de Vida do Snapshot

```go
// 1. Criar snapshot (incrementa RefCount)
snap := engine.Snapshot()

// 2. Usar snapshot (versão imutável)
results := snap.Analyze()
tension := snap.GetTension("node:xyz")

// 3. Fechar snapshot (decrementa RefCount)
snap.Close()

// 4. GC eventualmente remove versão se RefCount == 0
```

**Garantias:**
- Snapshot SEMPRE vê estado consistente
- Modificações durante análise NÃO afetam snapshot
- Snapshot fechado NÃO pode ser usado

### 4.6 Garbage Collection

**Estratégia:** Reference Counting + Timeout

```
Versão pode ser removida quando:
  1. RefCount == 0 (nenhum snapshot usando)
  2. OU timeout excedido (snapshot abandonado)
```

**Comportamento do timeout:**
- Snapshot não fechado após X minutos → SDK loga warning
- Após Y minutos → força Close(), loga error
- Versão é liberada para GC

**Configurável via Builder:**
```go
itt.NewBuilder().
    GCSnapshotWarning(5 * time.Minute).
    GCSnapshotForce(15 * time.Minute).
    Build()
```

---

## 5. Compactação Base/Overlay

### 5.1 Problema

Overlay cresce indefinidamente se não for consolidado no Base.

### 5.2 Estratégias de Compactação

| Estratégia | Trigger | Comportamento |
|------------|---------|---------------|
| **Por volume** (default) | Overlay > N eventos | Merge automático |
| **Temporal** | Eventos > X horas | Move para Base |
| **Manual** | `engine.Compact()` | Usuário controla |
| **Custom** | Hook `ShouldCompact()` | Lógica do usuário |

### 5.3 Comportamento do Merge

Quando compactação ocorre:

1. Snapshot do Overlay atual é criado
2. Base + Overlay são merged em novo Base
3. Overlay é resetado (nova versão vazia)
4. Snapshots antigos continuam funcionando (MVCC)

**Regras de merge:**
- Nós: união de ambos, atributos do Overlay prevalecem
- Arestas: pesos são somados, timestamps são min/max

### 5.4 Configuração

```go
itt.NewBuilder().
    CompactionStrategy(itt.CompactByVolume).
    CompactionThreshold(10000).  // eventos
    OnCompact(func(stats CompactStats) {
        log.Printf("Compacted: %d nodes merged", stats.NodesMerged)
    }).
    Build()
```

---

## 6. Sistema de Callbacks

### 6.1 Callbacks Disponíveis

| Callback | Trigger | Payload |
|----------|---------|---------|
| `OnChange` | Qualquer mudança no grafo | `Delta` |
| `OnAnomaly` | τ(v) > threshold | `TensionResult` |
| `OnCompact` | Compactação executada | `CompactStats` |
| `OnGC` | Versão removida pelo GC | `GCStats` |
| `OnError` | Erro recuperável | `error` |

### 6.2 Comportamento

**Garantias:**
- Callbacks são invocados na goroutine do worker interno
- Callback lento ATRASA processamento de eventos
- Callback que faz panic é recuperado e logado

**Recomendações:**
- Callbacks devem ser rápidos (< 1ms)
- Para processamento pesado, enviar para canal próprio
- Não fazer I/O bloqueante no callback

### 6.3 Exemplo de Uso

```go
engine := itt.NewBuilder().
    OnChange(func(d itt.Delta) {
        // Enviar para websocket
        wsChan <- d
    }).
    OnAnomaly(func(r itt.TensionResult) {
        // Alerta imediato
        alertService.Fire(r)
    }).
    Build()
```

---

## 7. Algoritmos de Análise

### 7.1 Divergência (Tensão Local)

**Base teórica:** τ(v) = D(P_obs || P_exp) onde D é uma medida de divergência.

**Implementações built-in:**
- Jensen-Shannon Divergence (JSD) - default, simétrica, limitada [0, log2]
- Kullback-Leibler (KL) - assimétrica, ilimitada
- Hellinger Distance - simétrica, limitada [0, 1]

**Cálculo:**
1. Para cada vizinho de v, construir distribuição de pesos das arestas
2. Simular "remoção" de v: zerar arestas conectadas a v
3. Calcular divergência entre distribuição original e perturbada
4. τ(v) = média das divergências dos vizinhos, normalizada

### 7.2 Curvatura de Ollivier-Ricci

**Base teórica:** Mede como a geometria do grafo "curva" ao redor de uma aresta.

**Cálculo para aresta (x, y):**
1. Construir distribuição de probabilidade em vizinhança de x
2. Construir distribuição de probabilidade em vizinhança de y
3. Calcular distância de Wasserstein entre distribuições
4. κ(x,y) = 1 - W(μ_x, μ_y) / d(x,y)

**Uso na ITT:**
- Curvatura negativa → "gargalo" no grafo
- Remoção de nó central causa mudança brusca de curvatura
- Complementa tensão para detecção mais robusta

### 7.3 Homologia Persistente

**Base teórica:** Detecta "buracos" topológicos no grafo.

**Conceitos:**
- Betti-0: número de componentes conectados
- Betti-1: número de ciclos independentes
- Betti-2: número de cavidades (em complexos simpliciais)

**Uso na ITT:**
- Remoção de nó pode "criar" ou "destruir" ciclos
- Mudança nos números de Betti indica importância estrutural
- Persistência indica robustez do feature topológico

### 7.4 Interface Plugável

Todos os algoritmos seguem interfaces, permitindo customização:

```go
type DivergenceFunc interface {
    Compute(p, q []float64) float64
    Name() string
}

type CurvatureFunc interface {
    Compute(g Graph, from, to string) float64
    Name() string
}

type TopologyFunc interface {
    Compute(g Graph) TopologyResult
    Name() string
}
```

### 7.5 Batch Interface (GPU-ready)

Para análise em larga escala, interface de batch:

```go
type BatchDivergenceFunc interface {
    ComputeBatch(pairs []DistributionPair) []float64
    Name() string
    SupportsBatch() bool
}
```

**Nota v1:** Interface definida, implementação GPU não incluída.

---

## 8. Sistema de Calibração

### 8.1 Regra de Ouro

> **Nada da teoria deve ser hardcoded, apenas calibrado.**

A ITT define COMO calcular tensão (JSD, curvatura, etc). Mas O QUE constitui anomalia é derivado dos dados, não de constantes mágicas.

**Anti-pattern:**
```go
// ❌ ERRADO: threshold hardcoded
if tension > 0.5 {
    alert("Anomaly!")
}
```

**Correto:**
```go
// ✅ CORRETO: threshold derivado dos dados
threshold := calibrator.Threshold()
if tension > threshold {
    alert("Anomaly!")
}
```

### 8.2 Por que MAD em vez de Desvio Padrão

Redes de dependência (NPM, Solana, etc) exibem **distribuição Power-Law**:

$$P(k) \sim k^{-\gamma}, \quad \gamma \in [2, 3]$$

Nessas distribuições:
- O **segundo momento diverge** (σ → ∞)
- A **média** é dominada por poucos hubs extremos
- Threshold baseado em `μ ± kσ` falha completamente

**MAD (Median Absolute Deviation)** é robusto:

$$\text{MAD} = \text{median}(|X_i - \tilde{X}|)$$

| Propriedade | Desvio Padrão (σ) | MAD |
|-------------|-------------------|-----|
| Breakdown point | 0% | 50% |
| Power-law | Diverge | Converge |
| Outliers | Domina | Ignora |

### 8.3 Fórmula de Threshold Dinâmico

$$\tau_{threshold} = \tilde{\tau} + K \cdot \text{MAD}(\tau)$$

Onde:
- $\tilde{\tau}$ = Mediana das tensões observadas
- $\text{MAD}(\tau)$ = MAD das tensões
- $K$ = Multiplicador de sensibilidade

| K | Interpretação | Uso |
|---|---------------|-----|
| 2.0 | Sensível | Auditorias de segurança, alto risco |
| 3.0 | Padrão | Detecção geral (default) |
| 4.0 | Conservador | Dados ruidosos, reduzir falsos positivos |

### 8.4 Tratamento de Amostras Pequenas

Para nós com grau baixo (d < MinDegree), a estimativa de tensão é ruidosa.

**Estratégias:**

1. **Filtro de grau mínimo**: Não calcular τ para d < MinDegree. Retornar status `INSUFFICIENT_DATA`.

2. **Regularização com prior**: Para d < 10, suavizar distribuição observada:
   $$P_{smooth} = \alpha \cdot P_{obs} + (1-\alpha) \cdot P_{uniform}$$
   Onde $\alpha = \frac{d}{d + \lambda}$ e $\lambda$ é constante de suavização.

3. **Flag sem alerta**: Nós de baixo grau geram warning, não anomalia.

### 8.5 Componente Calibrator

```go
type Calibrator interface {
    // Observe registra uma tensão observada
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
}
```

### 8.6 Protocolo de Calibração

**Fase 1: Warm-up**
- Processar primeiros N eventos/nós sem gerar alertas
- N default = 1000 (configurável)
- Coletar todas as tensões observadas

**Fase 2: Baseline**
- Após warm-up, calcular:
  - `median(tensions)`
  - `MAD(tensions)`
  - `threshold = median + K * MAD`
- Marcar `IsWarmedUp = true`

**Fase 3: Produção**
- Comparar tensões contra threshold calculado
- Gerar alertas quando τ > threshold

**Fase 4: Recalibração (opcional)**
- Periodicamente (tempo ou volume), recalcular baseline
- Acomoda drift do ecossistema
- Configurável: intervalo ou desabilitado

### 8.7 Configuração via Builder

```go
engine := itt.NewBuilder().
    // Calibração
    Calibrator(itt.NewCalibrator(
        itt.WithK(3.0),              // sensibilidade
        itt.WithWarmupSize(1000),    // amostras para warm-up
        itt.WithMinDegree(5),        // grau mínimo
        itt.WithRecalibrationInterval(24 * time.Hour),
    )).
    Build()
```

Ou usar calibrador custom:

```go
engine := itt.NewBuilder().
    Calibrator(myCustomCalibrator{}).
    Build()
```

### 8.8 Defaults

| Parâmetro | Default | Justificativa |
|-----------|---------|---------------|
| K | 3.0 | Padrão estatístico para detecção |
| WarmupSize | 1000 | Estabilidade estatística |
| MinDegree | 5 | Redução de ruído |
| RecalibrationInterval | 0 (desabilitado) | Usuário decide |
| Smoothing λ | 5.0 | Regularização moderada |

### 8.9 Integração com Análise

O `Calibrator` é consultado automaticamente durante análise:

```
Analyze()
  │
  ├─ Para cada nó:
  │    ├─ Verificar grau >= MinDegree
  │    ├─ Calcular tensão τ
  │    ├─ calibrator.Observe(τ)  // alimenta calibração
  │    ├─ Se calibrator.IsWarmedUp():
  │    │    └─ anomaly = calibrator.IsAnomaly(τ)
  │    └─ Senão:
  │         └─ anomaly = false (nunca alerta durante warm-up)
  │
  └─ Retornar Results
```

### 8.10 Calibração Manual vs Automática

**Automática (default):** SDK calibra conforme recebe dados.

**Manual:** Usuário pode injetar baseline conhecido:

```go
calibrator := itt.NewCalibrator(
    itt.WithPrecomputedBaseline(0.15, 0.08), // median, MAD
)
```

Útil para:
- Testes reproduzíveis
- Migração de ambiente
- Baseline de referência conhecido

---

## 9. Fluxos de Dados

### 8.1 Fluxo de Ingestão

```
Event
  │
  ▼
[Validação]
  │
  ├─ Erro → OnError callback
  │
  ▼
[Write Channel]
  │
  ▼
[Worker Dedicado]
  │
  ├─ Criar nós se necessário
  ├─ Criar/atualizar aresta
  ├─ Marcar dirty nodes
  ├─ Criar nova versão MVCC
  │
  ▼
[atomic.Store(current)]
  │
  ▼
[OnChange callback com Delta]
```

### 8.2 Fluxo de Análise

```
engine.Snapshot()
  │
  ▼
[Captura versão atual]
[Incrementa RefCount]
  │
  ▼
snapshot.Analyze()
  │
  ├─ Para cada nó:
  │    ├─ Calcular distribuição vizinhança
  │    ├─ Calcular divergência
  │    ├─ Calcular curvatura (se habilitado)
  │    └─ Agregar resultado
  │
  ▼
[Results]
  │
  ├─ Filtrar por threshold
  ├─ Ordenar por tensão
  │
  ▼
[OnAnomaly callback para cada anomalia]
  │
  ▼
snapshot.Close()
  │
  ▼
[Decrementa RefCount]
[GC pode limpar versão]
```

### 8.3 Fluxo de Compactação

```
[Trigger: volume/tempo/manual]
  │
  ▼
[Snapshot do Overlay]
  │
  ▼
[Merge Base + Overlay]
  │
  ├─ Nós: união
  ├─ Arestas: soma de pesos
  │
  ▼
[Novo Base]
  │
  ▼
[Reset Overlay]
  │
  ▼
[OnCompact callback]
```

---

## 10. Tratamento de Erros

### 10.1 Classificação

| Tipo | Tratamento | Exemplo |
|------|------------|---------|
| Recuperável | Retorna `error` | Evento inválido, storage indisponível |
| Irrecuperável | `panic` | Corrupção de estado, invariante violada |

### 10.2 Erros Recuperáveis

```go
// Validação de evento
if event.Source == "" {
    return ErrEmptySource
}

// Storage falha
if err := storage.Save(data); err != nil {
    engine.logger.Error("storage failed", "err", err)
    // Continua operando em memória
    return err
}
```

### 10.3 Erros Irrecuperáveis

```go
// Invariante violada
if version.RefCount < 0 {
    panic("invariant violated: negative refcount")
}

// Estado corrompido
if current == nil && started {
    panic("corrupted state: nil current after start")
}
```

### 10.4 Callback de Erro

```go
engine := itt.NewBuilder().
    OnError(func(err error) {
        metrics.IncCounter("itt.errors")
        logger.Warn("ITT error", "err", err)
    }).
    Build()
```

---

## 11. Observabilidade

### 11.1 Logger Interface

```go
type Logger interface {
    Debug(msg string, keysAndValues ...any)
    Info(msg string, keysAndValues ...any)
    Warn(msg string, keysAndValues ...any)
    Error(msg string, keysAndValues ...any)
}
```

**Comportamento:**
- Se não fornecido, SDK é completamente silenciosa
- Compatível com slog, zap, logrus via adapter

### 11.2 Níveis de Log

| Nível | Conteúdo |
|-------|----------|
| Debug | Cada evento processado, cada versão criada |
| Info | Compactações, snapshots criados/fechados |
| Warn | Snapshot não fechado (timeout warning) |
| Error | Erros recuperáveis, snapshot forçadamente fechado |

### 11.3 Métricas Sugeridas

SDK não implementa métricas, mas sugere pontos de instrumentação:

| Métrica | Tipo | Descrição |
|---------|------|-----------|
| `itt_events_total` | Counter | Eventos ingestados |
| `itt_events_invalid` | Counter | Eventos rejeitados |
| `itt_versions_total` | Counter | Versões MVCC criadas |
| `itt_snapshots_active` | Gauge | Snapshots abertos |
| `itt_analysis_duration` | Histogram | Tempo de análise |
| `itt_anomalies_detected` | Counter | Anomalias detectadas |

---

## 12. Decisões Técnicas e Justificativas

### 12.1 Por que MVCC em vez de RWMutex?

| Critério | RWMutex | MVCC |
|----------|---------|------|
| Simplicidade | ✅ Simples | ❌ Complexo |
| Contenção | ❌ Cresce com goroutines | ✅ Zero |
| Análise longa | ❌ Bloqueia escritas | ✅ Não bloqueia |
| Memória | ✅ Uma cópia | ❌ Múltiplas versões |

**Decisão:** MVCC porque análises ITT podem ser lentas e não devem bloquear ingestão.

### 12.2 Por que Base + Overlay?

**Problema:** Grafos grandes não cabem em memória se mantemos histórico completo.

**Solução:** Separar dados consolidados (Base) de dados recentes (Overlay).

**Benefícios:**
- Base pode vir de storage externo
- Overlay é compacto e rápido
- Compactação controla crescimento

### 12.3 Por que Builder Pattern?

**Alternativas consideradas:**
- Struct grande: verboso, campos opcionais confusos
- Functional options: idiomático Go, mas menos familiar

**Decisão:** Builder porque:
- Familiar para desenvolvedores Java (background do autor)
- IDE auto-complete funciona bem
- Erros de configuração detectados no `Build()`

### 12.4 Por que Callbacks em vez de Channels?

**Channels:** Backpressure natural, mas usuário precisa consumir.

**Callbacks:** Mais simples, SDK controla invocação.

**Decisão:** Callbacks como padrão porque:
- Uso mais comum é fire-and-forget (enviar para websocket)
- Callback lento é problema do usuário, não da SDK
- Channels disponíveis via wrapper se necessário

---

## 13. Glossário

| Termo | Definição |
|-------|-----------|
| **Tensão (τ)** | Medida de deformação entrópica na vizinhança de um nó |
| **Divergência** | Medida de diferença entre distribuições de probabilidade |
| **Snapshot** | Visão imutável do grafo em um momento específico |
| **Base** | Grafo consolidado, histórico, raramente modificado |
| **Overlay** | Eventos recentes, versionado com MVCC |
| **Delta** | Representação de uma mudança atômica no grafo |
| **Compactação** | Processo de mover Overlay para Base |
| **MVCC** | Multi-Version Concurrency Control |
| **Curvatura** | Medida geométrica de como o grafo "curva" localmente |
| **Homologia** | Estudo de "buracos" topológicos em estruturas |

---

## Apêndice A: Referências Teóricas

1. Grego, M. (2025). "Informational Tension Theory: A Framework for Entropic Detection of Latent Structures"
2. Lin, J. (1991). "Divergence measures based on the Shannon entropy"
3. Ollivier, Y. (2009). "Ricci curvature of Markov chains on metric spaces"
4. Edelsbrunner, H. & Harer, J. (2010). "Computational Topology: An Introduction"

---

## Apêndice B: Compatibilidade com Nexus

A SDK é compatível com o Nexus Crawler OSINT:

| Nexus | ITT SDK | Mapeamento |
|-------|---------|------------|
| `Node.ID` | `Node.ID` | Direto |
| `Node.Value` | `Node.Attributes["value"]` | Via atributo |
| `Node.Type` (HUB, SOCIAL, GHOST...) | `Node.Type` | Direto |
| `Node.Source` | `Edge.From` | Vira aresta |

**Casos de uso:**
- Qual nó (perfil) tem maior tensão? → Hub central da identidade
- Remoção de qual nó causa maior deformação? → Ponto crítico
- Clusters anômalos? → Personas que não deveriam estar conectadas
