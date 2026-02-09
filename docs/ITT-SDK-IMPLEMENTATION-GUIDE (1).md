# ITT SDK - Guia de Implementação

**Versão:** 1.0.0-draft  
**Data:** Janeiro 2025  

---

## 1. Visão Geral

Este documento guia a implementação da ITT SDK. Ele define:
- Ordem de implementação recomendada
- Dependências entre componentes
- Contratos que DEVEM ser satisfeitos
- Testes esperados para cada componente
- Armadilhas conhecidas

**Filosofia:** O documento define O QUE construir, não COMO. Decisões de implementação são do desenvolvedor.

---

## 2. Estrutura de Pacotes Sugerida

```
itt/
├── itt.go              # Exports públicos, NewBuilder()
├── builder.go          # Builder pattern implementation
├── engine.go           # Engine interface e implementação
├── snapshot.go         # Snapshot interface e implementação
├── event.go            # Event, validação
├── node.go             # Node struct
├── edge.go             # Edge struct
├── results.go          # TensionResult, Results, etc
├── delta.go            # Delta, DeltaType
├── errors.go           # Erros públicos
│
├── graph/
│   ├── immutable.go    # ImmutableGraph (MVCC)
│   ├── version.go      # GraphVersion, versionamento
│   └── view.go         # View unificada Base+Overlay
│
├── mvcc/
│   ├── controller.go   # Controle de versões
│   ├── gc.go           # Garbage collector
│   └── refcount.go     # Reference counting
│
├── analysis/
│   ├── tension.go      # Cálculo de tensão
│   ├── divergence.go   # Implementações de divergência
│   ├── curvature.go    # Ollivier-Ricci
│   └── topology.go     # Homologia persistente
│
├── compact/
│   ├── strategy.go     # Estratégias de compactação
│   └── merge.go        # Merge Base+Overlay
│
└── internal/
    ├── pool.go         # sync.Pool para alocações
    └── atomic.go       # Helpers atômicos
```

---

## 3. Ordem de Implementação

### Fase 1: Fundação (Semanas 1-2)

#### 1.1 Estruturas de Dados Básicas

**Arquivos:** `event.go`, `node.go`, `edge.go`, `errors.go`

**Contrato:**
- Structs conforme especificação
- Validação de Event implementada
- Erros definidos

**Testes:**
```go
func TestEventValidation(t *testing.T) {
    // Event com Source vazio -> ErrEmptySource
    // Event com Target vazio -> ErrEmptyTarget
    // Event com Weight negativo -> ErrNegativeWeight
    // Event válido -> nil
}
```

#### 1.2 Grafo Imutável

**Arquivos:** `graph/immutable.go`

**Contrato:**
- Estrutura que representa grafo de forma imutável
- Operações de leitura: GetNode, GetEdge, Neighbors
- Método para criar cópia com modificação: WithNode, WithEdge
- Nunca modifica instância existente

**Testes:**
```go
func TestImmutableGraphReadOperations(t *testing.T) {
    // GetNode retorna nó existente
    // GetNode retorna nil para nó inexistente
    // Neighbors retorna lista correta
}

func TestImmutableGraphImmutability(t *testing.T) {
    // WithEdge retorna NOVO grafo
    // Grafo original não é modificado
}
```

**Armadilhas:**
- Não usar ponteiros compartilhados entre versões
- Deep copy de maps e slices

#### 1.3 Versionamento MVCC

**Arquivos:** `graph/version.go`, `mvcc/controller.go`, `mvcc/refcount.go`

**Contrato:**
- GraphVersion encapsula ImmutableGraph + metadata
- Controller mantém versão atual (atomic.Pointer)
- RefCount incrementa/decrementa atomicamente
- Versão com RefCount=0 é elegível para GC

**Testes:**
```go
func TestVersionController(t *testing.T) {
    // Store atualiza versão atual
    // Load retorna versão atual
    // Operações são atômicas
}

func TestRefCount(t *testing.T) {
    // Increment aumenta contador
    // Decrement diminui contador
    // Decrement em 0 não vai negativo (ou panic)
}
```

**Armadilhas:**
- Race conditions no refcount
- Memory ordering em atomic operations

---

### Fase 2: Engine Core (Semanas 3-4)

#### 2.1 Builder

**Arquivos:** `builder.go`

**Contrato:**
- Todos os métodos retornam `*Builder` para chaining
- `Build()` valida configuração
- `Build()` retorna erro se configuração inválida
- Defaults sensatos para tudo

**Testes:**
```go
func TestBuilderDefaults(t *testing.T) {
    // NewBuilder().Build() funciona
    // Divergence default é JSD
    // Threshold default é 0.2
}

func TestBuilderValidation(t *testing.T) {
    // Threshold negativo -> erro
    // GCSnapshotForce < GCSnapshotWarning -> erro
}

func TestBuilderChaining(t *testing.T) {
    // Métodos podem ser encadeados
    // Ordem não importa
}
```

#### 2.2 Engine - Ciclo de Vida

**Arquivos:** `engine.go`

**Contrato:**
- `Start(ctx)` inicia goroutines
- `Stop()` para gracefully
- `Running()` retorna estado
- Context cancelado causa shutdown

**Testes:**
```go
func TestEngineLifecycle(t *testing.T) {
    // Start inicia engine
    // Running() retorna true após Start
    // Stop para engine
    // Running() retorna false após Stop
}

func TestEngineContextCancellation(t *testing.T) {
    // Cancel no context causa shutdown
    // Eventos pendentes são processados
}

func TestEngineAutoStart(t *testing.T) {
    // AddEvent em engine não iniciada -> auto start
}
```

#### 2.3 Engine - Ingestão

**Contrato:**
- `AddEvent` é thread-safe
- Eventos vão para canal interno
- Worker processa eventos do canal
- Nova versão MVCC criada a cada evento (ou batch)

**Testes:**
```go
func TestEngineAddEvent(t *testing.T) {
    // Evento válido é aceito
    // Evento inválido retorna erro
    // Nó é criado se não existe
    // Aresta é criada/atualizada
}

func TestEngineConcurrentAddEvent(t *testing.T) {
    // Múltiplas goroutines podem adicionar
    // Todos os eventos são processados
    // Sem race conditions
}

func TestEngineAddEvents(t *testing.T) {
    // Batch de eventos válidos é aceito
    // Um evento inválido -> nenhum processado
}
```

**Armadilhas:**
- Canal cheio causa blocking - decidir política
- Evento após Stop() deve retornar erro

#### 2.4 Snapshot

**Arquivos:** `snapshot.go`

**Contrato:**
- `Snapshot()` captura versão atual atomicamente
- Incrementa RefCount da versão
- `Close()` decrementa RefCount
- Métodos em snapshot fechado retornam erro

**Testes:**
```go
func TestSnapshotCapture(t *testing.T) {
    // Snapshot captura estado atual
    // Modificações após Snapshot não afetam
}

func TestSnapshotClose(t *testing.T) {
    // Close decrementa refcount
    // Close é idempotente
    // Operações após Close retornam erro
}

func TestSnapshotIsolation(t *testing.T) {
    // Snapshot A e B podem existir simultaneamente
    // Cada um vê sua versão
}
```

---

### Fase 3: Análise (Semanas 5-6)

#### 3.1 Divergências

**Arquivos:** `analysis/divergence.go`

**Contrato:**
- Implementar JSD, KL, Hellinger
- Interface DivergenceFunc satisfeita
- Distribuições normalizadas antes do cálculo
- Tratamento de zeros (epsilon)

**Testes:**
```go
func TestJSD(t *testing.T) {
    // JSD(p, p) = 0
    // JSD(p, q) = JSD(q, p)  // simétrico
    // JSD(p, q) >= 0
    // JSD(p, q) <= log(2)
}

func TestKL(t *testing.T) {
    // KL(p, p) = 0
    // KL(p, q) != KL(q, p) em geral  // assimétrico
    // KL(p, q) >= 0
}

func TestDivergenceWithZeros(t *testing.T) {
    // Distribuição com zeros não causa NaN/Inf
}
```

#### 3.2 Cálculo de Tensão

**Arquivos:** `analysis/tension.go`

**Contrato:**
- Para cada vizinho, calcular distribuição de pesos
- Simular remoção (zerar aresta para o nó alvo)
- Calcular divergência entre original e perturbada
- Agregar divergências dos vizinhos
- Normalizar resultado em [0, 1]

**Testes:**
```go
func TestTensionCalculation(t *testing.T) {
    // Nó isolado tem tensão 0
    // Nó com um vizinho tem tensão calculável
    // Hub tem tensão > nó periférico (em geral)
}

func TestTensionNormalization(t *testing.T) {
    // Resultado sempre em [0, 1]
}
```

**Armadilhas:**
- Vizinho com apenas uma aresta (para o nó alvo) -> distribuição trivial
- NaN/Inf propagando

#### 3.3 Curvatura

**Arquivos:** `analysis/curvature.go`

**Contrato:**
- Implementar Ollivier-Ricci
- Para aresta (x,y): distribuições em vizinhanças de x e y
- Distância de Wasserstein entre distribuições
- κ(x,y) = 1 - W(μ_x, μ_y) / d(x,y)

**Testes:**
```go
func TestOllivierRicci(t *testing.T) {
    // Aresta em grafo completo -> curvatura positiva
    // Aresta "ponte" entre clusters -> curvatura negativa
}
```

**Nota:** Wasserstein pode usar aproximação (Sinkhorn) para performance.

#### 3.4 Integração da Análise

**Contrato:**
- `Snapshot.Analyze()` usa divergência + curvatura configurados
- `TensionResult` preenchido completamente
- `Results` agregado com estatísticas

**Testes:**
```go
func TestSnapshotAnalyze(t *testing.T) {
    // Retorna resultado para todos os nós
    // Anomalies filtrado por threshold
    // Stats calculados corretamente
}

func TestAnalyzeNode(t *testing.T) {
    // Retorna tensão de um nó específico
    // Nó inexistente -> erro
}

func TestAnalyzeRegion(t *testing.T) {
    // Analisa subconjunto de nós
    // Estatísticas agregadas corretas
}
```

---

### Fase 4: MVCC Avançado (Semana 7)

#### 4.1 Garbage Collector

**Arquivos:** `mvcc/gc.go`

**Contrato:**
- Executar periodicamente
- Remover versões com RefCount=0
- Respeitar timeout para snapshots abandonados
- Invocar callback OnGC

**Testes:**
```go
func TestGCRemovesOrphanVersions(t *testing.T) {
    // Versão sem snapshots é removida
    // Versão com snapshot ativo não é removida
}

func TestGCTimeout(t *testing.T) {
    // Snapshot não fechado por muito tempo -> warning
    // Após timeout -> força close
}
```

**Armadilhas:**
- GC não deve bloquear ingestão
- Race condition entre GC e novo snapshot

#### 4.2 Base + Overlay

**Arquivos:** `graph/view.go`

**Contrato:**
- View unificada transparente
- GetNode busca em Overlay, fallback para Base
- Arestas: merge de pesos quando em ambos
- Iteração percorre ambos sem duplicatas

**Testes:**
```go
func TestViewUnification(t *testing.T) {
    // Nó só no Base -> encontrado
    // Nó só no Overlay -> encontrado
    // Nó em ambos -> Overlay vence
}

func TestViewEdgeMerge(t *testing.T) {
    // Aresta só no Base -> peso do Base
    // Aresta em ambos -> pesos somados
}
```

#### 4.3 Compactação

**Arquivos:** `compact/strategy.go`, `compact/merge.go`

**Contrato:**
- Estratégias: ByVolume, ByTime, Manual
- Merge cria novo Base, reseta Overlay
- Snapshots existentes não são afetados
- Callback OnCompact invocado

**Testes:**
```go
func TestCompactByVolume(t *testing.T) {
    // Atingir threshold -> compactação automática
}

func TestCompactManual(t *testing.T) {
    // engine.Compact() força compactação
}

func TestCompactPreservesSnapshots(t *testing.T) {
    // Snapshot antes de compact continua válido
}
```

---

### Fase 5: Callbacks e Exportação (Semana 8)

#### 5.1 Sistema de Callbacks

**Contrato:**
- Callbacks invocados na goroutine do worker
- Panic em callback é recuperado e logado
- Callback lento atrasa processamento

**Testes:**
```go
func TestOnChangeCallback(t *testing.T) {
    // Callback chamado após cada mudança
    // Delta contém informação correta
}

func TestOnAnomalyCallback(t *testing.T) {
    // Callback chamado quando tensão > threshold
}

func TestCallbackPanicRecovery(t *testing.T) {
    // Panic em callback não derruba engine
    // Erro é logado
}
```

#### 5.2 Exportação

**Contrato:**
- Export para JSON, GraphML, GEXF, DOT
- Snapshot inteiro serializado
- Formato válido e parseável

**Testes:**
```go
func TestExportJSON(t *testing.T) {
    // Output é JSON válido
    // Contém todos os nós e arestas
}

func TestExportGraphML(t *testing.T) {
    // Output é XML válido
    // Schema GraphML correto
}
```

---

### Fase 6: Polimento (Semana 9)

#### 6.1 Logging

**Contrato:**
- Logger opcional
- Níveis: Debug, Info, Warn, Error
- SDK silenciosa se logger nil

**Testes:**
```go
func TestLoggingOptional(t *testing.T) {
    // Engine sem logger funciona
    // Nenhum output para stdout/stderr
}

func TestLoggingLevels(t *testing.T) {
    // Eventos corretos em cada nível
}
```

#### 6.2 Performance Tuning

- Profiling com pprof
- Identificar hotspots
- sync.Pool para alocações frequentes
- Benchmark suite

**Testes:**
```go
func BenchmarkAddEvent(b *testing.B) {
    // Medir throughput de ingestão
}

func BenchmarkAnalyze(b *testing.B) {
    // Medir tempo de análise por tamanho de grafo
}

func BenchmarkConcurrentAddEvent(b *testing.B) {
    // Medir escalabilidade com múltiplas goroutines
}
```

---

## 4. Checklist de Validação

### 4.1 Funcionalidade

- [ ] Builder cria engine com defaults
- [ ] AddEvent aceita eventos válidos
- [ ] AddEvent rejeita eventos inválidos
- [ ] Snapshot captura estado imutável
- [ ] Analyze retorna tensões corretas
- [ ] Anomalies filtrado por threshold
- [ ] Callbacks são invocados
- [ ] GC remove versões órfãs
- [ ] Compactação merge Base+Overlay
- [ ] Export gera formatos válidos

### 4.2 Concorrência

- [ ] AddEvent é thread-safe
- [ ] Múltiplos snapshots simultâneos funcionam
- [ ] GC não causa race conditions
- [ ] Compactação não corrompe dados

### 4.3 Robustez

- [ ] Eventos inválidos não corrompem estado
- [ ] Panic em callback é recuperado
- [ ] Snapshot fechado retorna erro em operações
- [ ] Engine parada rejeita novos eventos

### 4.4 Performance

- [ ] Ingestão >= 15k eventos/segundo
- [ ] Snapshot é O(1)
- [ ] Análise escala linearmente com nós
- [ ] Memória não cresce indefinidamente

---

## 5. Armadilhas Conhecidas

### 5.1 MVCC

| Armadilha | Consequência | Prevenção |
|-----------|--------------|-----------|
| Ponteiro compartilhado entre versões | Mutação afeta versões antigas | Deep copy sempre |
| RefCount race condition | Versão removida com snapshot ativo | Operações atômicas |
| GC muito agressivo | Versão removida antes de snapshot usar | Verificar refcount > 0 |
| GC muito lento | Memory leak | Timeout + GC periódico |

### 5.2 Concorrência

| Armadilha | Consequência | Prevenção |
|-----------|--------------|-----------|
| Canal cheio bloqueia indefinidamente | Deadlock | Timeout ou política de drop |
| Callback lento | Backpressure na ingestão | Documentar, não resolver |
| Stop() durante processamento | Eventos perdidos | Drain do canal antes de fechar |

### 5.3 Análise

| Armadilha | Consequência | Prevenção |
|-----------|--------------|-----------|
| Divisão por zero em divergência | NaN/Inf | Epsilon em denominadores |
| Distribuição vazia | Panic | Verificar len > 0 |
| Overflow em soma de pesos | Valores incorretos | Usar float64, verificar Inf |

### 5.4 API

| Armadilha | Consequência | Prevenção |
|-----------|--------------|-----------|
| Snapshot não fechado | Memory leak | Timeout + warning |
| Usar snapshot fechado | Resultado incorreto | Retornar erro |
| Configuração conflitante | Comportamento indefinido | Validar em Build() |

---

## 6. Testes de Integração

### 6.1 Cenário: Streaming Contínuo

```go
func TestStreamingScenario(t *testing.T) {
    engine := itt.NewBuilder().
        OnAnomaly(func(r itt.TensionResult) {
            // verificar
        }).
        Build()
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    engine.Start(ctx)
    
    // Gerar 100k eventos em 10 goroutines
    // Verificar todos processados
    // Verificar anomalias detectadas
    // Verificar sem memory leak
}
```

### 6.2 Cenário: Análise Durante Ingestão

```go
func TestAnalysisDuringIngestion(t *testing.T) {
    engine := itt.NewBuilder().Build()
    engine.Start(context.Background())
    
    // Goroutine 1: ingestão contínua
    go func() {
        for i := 0; i < 100000; i++ {
            engine.AddEvent(generateEvent())
        }
    }()
    
    // Goroutine 2: análises periódicas
    go func() {
        for i := 0; i < 100; i++ {
            snap := engine.Snapshot()
            snap.Analyze()
            snap.Close()
            time.Sleep(100 * time.Millisecond)
        }
    }()
    
    // Verificar sem race conditions
    // Verificar resultados consistentes
}
```

### 6.3 Cenário: Compactação Sob Carga

```go
func TestCompactionUnderLoad(t *testing.T) {
    engine := itt.NewBuilder().
        CompactionStrategy(itt.CompactByVolume).
        CompactionThreshold(10000).
        Build()
    
    // Ingerir 50k eventos (deve triggerar 5 compactações)
    // Verificar Base cresce
    // Verificar Overlay reseta
    // Verificar análise funciona durante/após
}
```

---

## 7. Métricas de Sucesso

### 7.1 Performance Mínima

| Métrica | Target | Como Medir |
|---------|--------|------------|
| Ingestão | >= 15k eventos/s | BenchmarkAddEvent |
| Snapshot | < 1μs | BenchmarkSnapshot |
| Análise 1k nós | < 100ms | BenchmarkAnalyze |
| Análise 100k nós | < 10s | BenchmarkAnalyzeLarge |
| Memória por versão | < 1.5x tamanho do grafo | Memory profiling |

### 7.2 Corretude

| Métrica | Target | Como Medir |
|---------|--------|------------|
| Race conditions | 0 | go test -race |
| Cobertura | > 80% | go test -cover |
| Testes passando | 100% | CI |

---

## 8. Referências da PoC

A PoC existente (`itt-poc-master`) contém implementações de referência:

| Componente | Arquivo na PoC | Notas |
|------------|----------------|-------|
| Grafo | `pkg/graph/graph.go` | Adaptável para imutável |
| Divergência | `pkg/entropy/divergence.go` | JSD/KL implementados |
| Curvatura | `pkg/curvature/ollivier_ricci.go` | Implementação completa |
| Tensão | `pkg/tension/analyze.go` | Lógica de cálculo |
| Validação | `pkg/validation/theorems.go` | ComputeNodeTension |

**Recomendação:** Usar como referência conceitual, não copiar diretamente. A SDK tem requisitos diferentes (MVCC, thread-safety, etc).

---

## 9. Cronograma Sugerido

| Semana | Fase | Entregável |
|--------|------|------------|
| 1-2 | Fundação | Estruturas, grafo imutável, MVCC básico |
| 3-4 | Engine Core | Builder, Engine lifecycle, ingestão, snapshot |
| 5-6 | Análise | Divergências, tensão, curvatura |
| 7 | MVCC Avançado | GC, Base+Overlay, compactação |
| 8 | Callbacks/Export | Sistema de callbacks, exportação |
| 9 | Polimento | Logging, performance, documentação |

**Total estimado:** 9 semanas para implementação completa.

**MVP (mínimo viável):** Semanas 1-6 (análise funcional, sem compactação avançada).
