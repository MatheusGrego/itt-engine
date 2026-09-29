# Refinamento ITT: backlog priorizado

Data: 2026-09-29. Base: auditoria de `analysis/`, `graph/`, `engine.go`, paper core v3 e roadmap v2, com testes rodados na engine (Go 1.25, `CGO_ENABLED=0`).

## Resumo em 5 linhas

1. O τ do código mede **importância contrafactual** ("quanto estrago v causaria se sumisse"), não **ausência** ("algo que existia sumiu"). Um nó já suprimido dá τ = 0.
2. No leave-one-out, qualquer divergência vira uma função crescente só de p = w(n→v)/W_out(n). JSD, Hellinger e KL dão o mesmo ranking. A escolha da divergência é decorativa.
3. O τ ignora volume (1 evento e 10k eventos na mesma proporção dão o mesmo valor) e não existe P_exp no código.
4. A detectabilidade (SNR e Yharim) está matematicamente frágil no código e no paper.
5. A performance é O(E²) na ingestão por causa do deep copy do grafo a cada evento. Só vale otimizar depois de redefinir o τ, porque o cálculo muda.

## Ordem de ataque

A ordem é **avaliar, depois consertar o que mede, depois acelerar**. Sem um bench com supressões plantadas não dá pra saber se nenhuma mudança melhora algo. Otimizar o τ atual antes de trocar o τ é trabalho jogado fora (o kernel da GPU, por exemplo, implementa o τ velho).

---

## P0: fundação (bugs rápidos + bench)

| ID | Problema | Evidência | Ação | Esforço |
|----|----------|-----------|------|---------|
| B1 | Bug p=1: quando o único destino de n é v, o vetor perturbado zera, `Normalize` devolve uniforme = original, e o τ sai 0 (o limite correto é 1) | `analysis/tension.go`, laço de `Calculate` + `Normalize` fallback | Tratar soma perturbada == 0 como divergência máxima | 30 min |
| B2 | O exemplo do README detecta 0 anomalias; charlie (peso 50) tem τ = 0 | rodado: db = api = 0.1556, resto 0 | Corrigir B1 e reescrever o exemplo com um caso que a engine realmente pega | 1 h |
| B3 | Build sem cgo quebra: o stub não tem `dispatch`, `release`, `info` | `gpu/gpu_dispatch_nocgo.go` | Adicionar os stubs (o patch já foi testado localmente) | 15 min |
| B4 | O benchmark de 131 ns/op mede só o enqueue no channel | `AddEvent` → channel; o processamento é assíncrono | Bench end-to-end: AddEvent + drenar + versão publicada | 1 h |
| E1 | Não existe critério objetivo de "funciona" | nenhum teste com ground truth | Harness sintético com supressões plantadas (ver Fase 1, ADR-0006) | 1 a 2 dias |

## P1: semântica e estatística do τ (o coração da teoria)

| ID | Problema | Evidência | Proposta | ADR |
|----|----------|-----------|----------|-----|
| T1 | τ mede importância, não ausência | nó removido → τ = 0; o paper simulou a extinção de um nó ainda presente (Claytonia) | τ passa a comparar o presente com o esperado (o passado do próprio nó) | 0001 |
| T2 | Divergência irrelevante: τ = f(p) | 4 distribuições diferentes com p = 0.3 dão a mesma JSD = 0.169195 | Estatística orientada à ausência: desvio de Poisson com sinal (silêncio vs burst) | 0002 |
| T3 | Ignora volume | escala ×1 e ×10000 dão o mesmo τ = 0.3113 | Estatística sobre contagens, não sobre vetores normalizados | 0002 |
| T4 | Não existe P_exp no código | `tension.go` só faz leave-one-out | Modelo nulo explícito e plugável; o default é o baseline temporal | 0003 |
| T5 | Diluição: vizinhos que não apontam pra v entram na média com 0 | `count++` para todo vizinho com out-edges | Some sozinho na nova estatística (soma por aresta, não média por vizinho) | 0002 |
| D1 | O SNR do código é `mean/std·√n` das tensões, um t-stat contra zero que cresce com √n (grafo grande sempre "detectável") | `analysis/yharim.go` | Detectabilidade em bits de evidência | 0004 |
| D2 | Derivação do Yharim: √(2 ln 1/α) é cauda gaussiana, não Gumbel; o máximo de uma variável limitada cai no domínio Weibull | paper v3, teorema "JSD boundedness" | Reinterpretar o Yharim via Sanov: N·KL > ln(1/α). A fórmula sobrevive, a justificativa muda | 0004 |

## P2: performance e revisão do paper

| ID | Problema | Evidência | Proposta | ADR |
|----|----------|-----------|----------|-----|
| F1 | Deep copy do grafo inteiro a cada evento: O(V+E) por evento, O(E²) na ingestão | `graph/immutable.go` `WithEvent` → `deepCopyGraph` | Estrutura persistente (structural sharing) ou snapshot por lote em vez de por evento | 0005 |
| F2 | Analyze superlinear: 475 µs (100 nós) → 277 ms (1k nós) | tabela do README | Estatística aditiva por aresta → atualização incremental (Dirty set) | 0005 |
| F3 | Duas alocações de slice por vizinho em cada `Calculate` | `tension.go` | Some com a nova estatística; se sobrar, usar buffers reaproveitados | 0005 |
| F4 | O kernel GPU implementa o τ velho | `gpu/jsd_kernel*.go`, `jsd_tension.wgsl` | Congelar a GPU até a ADR-0002 ser aceita | 0005 |
| X1 | O τ da Marak = 0.311 = f(½) exatamente, provável artefato de d = 2 | f(0.5) = 0.3113 | Refazer a análise com a estatística nova e com volume | Fase 5 |
| X2 | P_eq uniforme ≈ função da entropia, o que derruba o H3 (irredutibilidade) | roadmap v2 | Trocar o candidato de P_eq pelo nulo da ADR-0003 | Fase 5 |
| X3 | O SNR do paper (d/σ_d·√C) ≠ o SNR do código | paper v3 vs `yharim.go` | Unificar numa só definição (ADR-0004) | 0004 |
| X4 | Validações do paper medem importância (remoção simulada), não detecção | seção de validação cross-domain | Revalidar no harness e depois em um caso real com antes/depois | Fase 5 |

## P3: hipóteses em gelo (só depois da Fase 2 provar valor)

| ID | Hipótese | Por que esperar |
|----|----------|-----------------|
| H-LoG | Filtro multi-escala sobre o campo de silêncio pra achar *onde* está o buraco, *de que tamanho* e *quão antigo* | Depende de existir um campo de silêncio confiável (Fase 2). Ver explicação abaixo |
| H-Ω | Recalcular o Ω (concealment) em cima do silêncio S em vez do τ atual | Mesma dependência |

**H-LoG em português simples.** Pinga tinta num papel molhado: ela espalha. A tua equação de difusão (∂τ/∂t = −αLτ + S) é exatamente essa tinta espalhando pelas arestas do grafo. Agora compara "espalhou um pouco" com "espalhou bastante" e subtrai um do outro. O que sobra destaca manchas de um tamanho específico. Repetindo isso pra vários tamanhos, dá pra saber em qual tamanho a mancha de silêncio responde mais forte. Isso diz *onde* está o buraco e *quão grande* ele é, e o tamanho, pela tua velocidade do silêncio, vira uma estimativa de *idade*. É só isso. É útil, mas só depois que o silêncio medido na base for confiável.

---

## Planos por fase

### Fase 0: higiene (1 dia)
- B1, B3, B4 no código; B2 no README.
- Pronto quando: `CGO_ENABLED=0 go test ./...` passa; o README tem números end-to-end; o exemplo detecta algo.

### Fase 1: harness de avaliação (1 a 2 dias), ADR-0006
- Gerador: grafo com pesos Poisson ao longo de T janelas de tempo (configuration model ou blocos), com semente fixa.
- Cenários plantados:
  - S1: remoção total de um nó na janela t0.
  - S2: afinamento, com as arestas de um nó caindo a 30%.
  - S3: "sniper", um nó de grau baixo ligado a hubs.
  - S4: controles, com um nó que nunca existiu e um nó naturalmente isolado.
  - S5: burst, com excesso em vez de falta.
- Métricas:
  - AUC e precision@k pra achar o nó plantado.
  - Taxa de falso positivo sob o nulo puro, que precisa ficar ≤ α.
- Baseline: o τ atual (com B1 corrigido).
- Pronto quando: um comando roda todos os cenários e imprime uma tabela comparável.

### Fase 2: silence deviance v0, ADRs 0001 a 0003
- Nova função de tensão plugável em `analysis/`, sem remover a JSD ainda (as duas rodam lado a lado no harness).
- O nulo é o baseline temporal por aresta (média móvel ou EWMA das janelas anteriores).
- Pronto quando: a AUC em S1/S2/S3 é maior que a do τ atual, e o FP sob o nulo fica ≤ α.
- Se perder: a ADR vira "Rejeitado" e a gente registra por quê (resultado negativo também é resultado).

### Fase 3: detectabilidade honesta, ADR-0004
- Limiar em bits: silêncio > ln(1/α) + correção de múltiplos testes (ln n).
- Remover o SNR `mean/std·√n` ou rebaixá-lo a um descritor.
- Pronto quando: sob o nulo puro, a fração de alarmes bate com α no harness.

### Fase 4: performance, ADR-0005
- Ingestão sem deep copy por evento.
- Análise incremental: só recalcular as arestas que mudaram.
- Reavaliar a GPU só depois disso.
- Pronto quando: um bench end-to-end com 100k eventos e 10k nós mostra ingestão ~linear.

### Fase 5: revisão do paper
- X1 a X4, com a nova estatística e com os números do harness.
- Reescrever o teorema do Yharim via Sanov.

---

## Glossário

- **f-divergência:** família de medidas de diferença entre distribuições (KL, JSD, Hellinger, χ²...). Todas dependem só da razão P/Q ponto a ponto, e por isso no leave-one-out todas viram função de p.
- **Leave-one-out / contrafactual:** "e se eu tirasse v?". Mede importância, não ausência.
- **Modelo nulo (P_exp):** o que você esperaria ver se nada estranho tivesse acontecido. Sem ele, "anomalia" não tem referência.
- **Baseline temporal:** usar o comportamento passado da própria aresta ou do próprio nó como esperado.
- **Desvio de Poisson (deviance):** `d = 2[o·ln(o/e) − (o − e)]` com o = observado e e = esperado; quando o = 0, d = 2e. Mede, em unidades de log-verossimilhança, o quanto o observado é improvável sob o esperado. Leva o volume em conta.
- **Silêncio / burst:** a soma do desvio só nas arestas onde o < e (falta) ou o > e (excesso).
- **Sanov:** a probabilidade de ver uma distribuição empírica P̂ quando a verdade é Q cai como e^(−N·KL(P̂‖Q)). Liga "divergência" a "probabilidade", com o N (volume) dentro.
- **χ² (qui-quadrado):** distribuição que o desvio total segue sob o nulo. Dá o limiar a partir da teoria, sem warmup.
- **AUC:** probabilidade de o método ranquear um nó plantado acima de um nó normal. 0.5 = chute, 1.0 = perfeito.
- **Laplaciano (L):** matriz que diz "quanto cada nó difere dos vizinhos". É o motor da difusão no grafo.
- **Heat kernel e^(−tL):** "a tinta depois de espalhar por um tempo t".
- **LoG (Laplacian of Gaussian):** "espalhou pouco" menos "espalhou muito"; realça manchas de um tamanho específico.
- **Structural sharing:** uma versão nova do grafo reaproveita as partes que não mudaram, em vez de copiar tudo.
