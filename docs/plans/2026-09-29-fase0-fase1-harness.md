# Plano: Fase 0 (higiene) + Fase 1 (harness) + Fase 2 v0 (stretch)

- Branch: `refinement/fase0-fase1`
- Referências: `docs/refinement/BACKLOG.md`, ADR-0001, ADR-0002, ADR-0003 e ADR-0006.
- Objetivo da noite: deixar a engine compilando/honesta e ter **uma tabela que compara métodos em supressões plantadas**. Nada de mudar a teoria nem os defaults.

Execute as tarefas em ordem. Cada tarefa = 1 commit. Ao final, faça o relatório (Tarefa 9) e abra o PR.

---

## Tarefa 0: baseline

1. `CGO_ENABLED=0 go build ./...`. É esperado que **falhe** no pacote `gpu` (é o B3). Anote o erro.
2. Anote no relatório quais testes falham antes de qualquer mudança (rode depois da Tarefa 1 se o build impedir antes).

## Tarefa 1: B3, build sem cgo

Arquivo: `gpu/gpu_dispatch_nocgo.go`. O stub não tem o que `gpu/gosl_backend.go` usa. Adicione:

```go
type gpuPipeline struct{ info DeviceInfo }

func (p *gpuPipeline) dispatch(
	csrRowPtr []int32, csrColIdx []int32, csrValues []float32,
	cscColPtr []int32, cscRowIdx []int32,
	numNodes int,
) ([]float32, error) {
	return nil, errors.New("gpu: not available without cgo")
}

func (p *gpuPipeline) release() {}
```

(Confira as assinaturas reais em `gpu/gpu_dispatch.go` e copie-as exatamente.)

Pronto quando: `CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./...` passam. Rode `CGO_ENABLED=0 go test ./...` e registre o resultado.

## Tarefa 2: B1, bug p=1

**Bug:** em `analysis/tension.go` (`Calculate`), quando todo o peso de saída do vizinho n vai para v, `perturbedRaw` fica todo zero, `Normalize` devolve uniforme (que é igual ao original quando n tem 1 destino) e a divergência sai 0. O limite correto de JSD(P, P∖v) quando p → 1 é 1 (log2).

**Teste primeiro** (`analysis/tension_test.go`):
- grafo `a→v (w=5)` e só isso: τ(v) com JSD deve ser 1.0 (tolerância 1e-9).
- grafo `a→v (w=1)`, `a→x (w=1)`: contribuição do vizinho a = f(0.5) = 0.311278 (tolerância 1e-5), onde `f(p) = 0.5*(p + (1-p)*log2(2(1-p)/(2-p)) + log2(2/(2-p)))`.
- Teste de propriedade: para vários vetores com a mesma fração p para v, JSD(P, P∖v) é igual a f(p) (documenta a descoberta T2).

**Fix:** se a soma de `perturbedRaw` for 0, a contribuição é o máximo da divergência:
- se ela implementa `BoundedDivergence` e é limitada: JSD → 1.0, Hellinger → 1.0. Crie o método `MaxValue() float64` nas divergências limitadas, ou trate com um switch por `Name()`, o que ficar mais limpo;
- KL (ilimitada): use `math.Log2(1/epsilon)` como teto e documente no comentário.

**Paridade GPU:** aplique a mesma regra em `gpu/jsd_kernel.go`, em `gpu/jsd_kernel_f32.go` (referência CPU) e em `gpu/shaders/jsd_tension.wgsl`. Os testes com cgo não rodam na nuvem, então diga isso no PR.

Atualize testes pré-existentes que assertavam o comportamento bugado, com um comentário `// B1:` explicando.

## Tarefa 3: B2, exemplo do README

1. Crie `example_test.go` com `ExampleEngine_readme()` reproduzindo o exemplo do README (use `engine.Stop()` para drenar em vez de `time.Sleep`, se `Stop` drenar; confira em `engine.go`). Imprima τ por nó em ordem alfabética e a contagem de anomalias. Use `// Output:` com os valores reais após o B1.
2. Se o exemplo continuar sem detectar anomalia, **não force**. Reescreva o exemplo do README para um cenário em que a engine de fato sinaliza algo e explique em 1 frase o que o τ mede hoje ("quanto os vizinhos dependem do nó"). Adicione no README uma seção curta "Estado atual / limitações conhecidas" apontando para `docs/refinement/BACKLOG.md`.

## Tarefa 4: B4, benchmark honesto

Em `benchmark_test.go`, adicione `BenchmarkIngestEndToEnd` com sub-benchmarks para 1k, 5k, 20k e 50k eventos sobre ~1k nós (fonte e destino aleatórios com semente fixa). Cada iteração: build da engine → AddEvent × N → `Stop()` (drena). Reporte ns/evento com `b.ReportMetric`.

Rode e cole os números no relatório. Se o custo por evento crescer com N, isso confirma o F1 (deep copy por evento). **Não otimize nada agora.**

No README, na tabela de performance: marque `AddEvent ~131 ns/op` como "só enqueue" e adicione a linha end-to-end com os números medidos (CPU do container informada).

## Tarefa 5: harness, gerador (ADR-0006)

Pacote `harness/` (raiz do módulo) + `cmd/itt-harness/main.go`. Só stdlib, `math/rand/v2` com PCG e semente fixa.

**Modelo:**
- N nós (default 500) e T janelas (default 12). A supressão começa em t0 (default 8): janelas [0, t0) são "antes" e [t0, T) são "depois".
- Grau de saída por nó: heavy-tailed truncado (ex.: `min(50, floor(2 * pareto(α=1.5)))`, mínimo 2).
- Destinos escolhidos com probabilidade proporcional a uma fitness lognormal (gera hubs de entrada). Sem self-loops nem duplicatas.
- Taxa por aresta: λ_ij ~ lognormal(μ=ln 4, σ=0.8). Contagem por janela: Poisson(λ_ij · mult_ij(t)).
- `mult_ij(t) = 1`, exceto quando o cenário altera, e aí só para t ≥ t0.

**Cenários (o alvo é sempre escolhido com a semente, nunca à mão):**

| ID | Nome | Alteração em t ≥ t0 | Alvo |
|----|------|---------------------|------|
| S0 | nulo | nenhuma | nenhum (mede falso positivo) |
| S1 | remoção | mult = 0 em todas as arestas de entrada e saída do alvo | nó com grau total próximo da mediana |
| S2 | afinamento | mult = 0.3 nas arestas do alvo | idem |
| S3 | sniper | plante um nó extra com 2 a 3 arestas de saída para hubs (top 10% em fitness), com λ normal; depois mult = 0 | o nó plantado |
| S4 | eremita | nenhuma | nó de menor grau total (controle: não deveria subir no ranking) |
| S5 | burst | mult = 3 nas arestas do alvo | nó com grau próximo da mediana |

API sugerida:

```go
type Scenario string
type Dataset struct {
    N, T, T0 int
    Target   string              // "" em S0
    Counts   []map[[2]string]int // Counts[t][{from,to}]
}
func Generate(s Scenario, cfg Config, seed uint64) Dataset
```

Testes: mesma semente → mesmo dataset; em S1 o alvo tem contagem 0 em todas as arestas para t ≥ t0; em S0 a média das contagens ≈ a média de λ (tolerância larga).

## Tarefa 6: harness, métodos e métricas

**Métodos** (interface `Method { Name() string; Scores(d Dataset) map[string]float64 }`):

- **M1 `tau-jsd`** (baseline): monta um `graph.Graph` com os pesos = soma das contagens nas janelas [t0, T) e roda `analysis.NewTensionCalculator(analysis.JSD{}).CalculateAll`. Nó sem arestas no depois recebe 0 (é exatamente o problema T1, e o harness deve mostrar isso).
- **M2 `tau-jsd-cumulative`**: igual, mas com os pesos somando todas as janelas [0, T), que é como a engine acumula hoje.
- **M3 `silence-deviance-v0`** (ADR-0002/0003): para cada aresta ij existente em qualquer janela:
  - `e = mean(counts[0:t0]) * (T - t0)`, com floor `e = max(e, 0.5)` só se a aresta existia antes;
  - `o = sum(counts[t0:T])`;
  - `d = 2*(o*ln(o/e) - (o-e))`, com `d = 2e` quando o = 0.
  - `S(v)` = soma de d nas arestas incidentes a v (entrada e saída) com o < e; `B(v)` = a mesma soma com o > e.
  - Score de M3 = S(v). Exponha também B.
- **M4 `burst-deviance-v0`**: score = B(v). Só pra checar o S5.

**Métricas** por (cenário, método), com R réplicas (default 20, sementes 1..R):

- `AUC`: fração de nós não-alvo com score menor que o do alvo, com empate contando 0.5. Média ± desvio.
- `hit@10`: fração das réplicas com o alvo no top 10.
- S0, M1/M2: fração de nós com τ > 0.2 (threshold default da engine).
- S0, M3: FWER, a fração de réplicas com algum nó tendo `S(v)/2 > ln(1/α) + ln(N)` com α = 0.05. Deve ser ≤ ~0.05. Se passar muito, registre (é informação útil sobre sobredispersão, não é bug a esconder).
- S4: AUC do eremita (quanto menor, melhor; ~0.5 ou menos é ok).

Testes: AUC de um score perfeito = 1; AUC de um score constante = 0.5.

## Tarefa 7: harness, CLI e saída

`go run ./cmd/itt-harness [-n 500 -t 12 -t0 8 -r 20 -alpha 0.05 -out docs/refinement/results]`

- Imprime uma tabela markdown (linhas = cenário, colunas = método, células = `AUC ± sd / hit@10`) e as linhas de falso positivo do S0.
- Grava `results.csv` e `results.md` em `-out`.
- Pronto quando: roda em < 1 min com os defaults no container.

## Tarefa 8: anexar resultados aos ADRs (sem mudar status)

Em ADR-0002 e ADR-0006, adicione a seção `## Resultados (harness 2026-09-29)` com a tabela e 2 a 3 frases factuais: o que ganhou onde e o que surpreendeu. **Status continua `Proposto`.**

Se o M3 perder para o M1 em S1/S2, escreva isso sem rodeio. Resultado negativo é resultado.

## Tarefa 9: relatório e PR

Crie `docs/refinement/RUN-2026-09-29.md` com:
- o que foi feito (tarefas + commits);
- testes antes/depois e o que não rodou (GPU/cgo);
- números do bench end-to-end;
- a tabela do harness;
- decisões que você tomou sozinho e perguntas abertas (também em `OPEN-QUESTIONS.md`);
- o próximo passo sugerido (em uma linha).

Abra o PR `refinement/fase0-fase1 → master` com título `Refinamento: Fase 0 + harness (B1-B4, E1)` e o relatório como descrição. Se não conseguir abrir o PR, deixe a branch pushada e diga isso no relatório.

## Fora de escopo (não faça)

- Mudar o default de divergência, o `TensionCalculator` além do B1, ou a API pública.
- Otimizar performance (F1 a F4).
- Integrar o silence deviance na engine (isso é da Fase 2, depois do Matheus aceitar a ADR-0002).
- Mexer no paper, em `demo/` ou no ARGOS.
