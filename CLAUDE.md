# CLAUDE.md: itt-engine

SDK em Go de detecção de anomalias baseado na Informational Tension Theory (ITT). O dono é o Matheus (PT-BR casual, direto, sem enrolação).

## Estado atual (leia antes de mexer)

O projeto está em **refinamento**. A auditoria de 2026-09-29 achou problemas de teoria, estatística e performance. Fonte da verdade:

- `docs/refinement/BACKLOG.md`: problemas priorizados com IDs (B, T, D, F, X, E), planos por fase e glossário.
- `docs/adr/`: decisões em formato MADR enxuto. Todas estão `Proposto`.
- `docs/plans/`: planos de implementação executáveis. O plano da vez é o arquivo mais recente.

Resumo em uma linha: o τ atual (`analysis/tension.go`) é leave-one-out, então mede importância contrafactual e não ausência; ele é uma função só de p = w(n→v)/W_out(n), e a JSD é intercambiável com qualquer f-divergência.

## Comandos

```bash
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go test -run XXX -bench . -benchmem ./...
```

Use sempre `CGO_ENABLED=0` fora da máquina do Matheus: o backend GPU (`gpu/`, cogentcore/webgpu + glfw) exige cgo + headers X11 e não compila em container. Testes de GPU (`gpu_integration_test.go`, `gpu/*_test.go` com cgo) não rodam na nuvem. Diga isso no PR em vez de fingir que rodou.

## Regras

- **Nunca** commitar na `master`. Trabalhe na branch indicada no plano e abra PR.
- Um commit por tarefa do plano, mensagem `tipo(escopo): descrição (ID)`, ex.: `fix(analysis): p=1 gives max divergence (B1)`.
- TDD quando der: teste falhando → fix → teste passando.
- **Não mudar o status de ADR.** Só o Matheus aceita ou rejeita. Você pode anexar resultados ao ADR na seção "Resultados" (crie se não houver).
- **Não remover a JSD nem mudar o default da engine.** Métodos novos entram lado a lado, plugáveis.
- **Não mexer** em `docs/theory/` (paper), `demo/`, nem em `argos/` (se existir localmente).
- `gpu/`: só mudanças de paridade exigidas pelo plano (B1) e os stubs de build (B3). Todo o resto da GPU está congelado (ADR-0005).
- Core sem dependências externas novas. Só a stdlib (`math/rand/v2` pode).
- Se um teste pré-existente falhar antes de você mexer, registre no relatório e siga. Não "conserte" o teste pra passar.
- Se travar numa decisão que muda a teoria, escolha a opção mais conservadora, registre em `docs/refinement/OPEN-QUESTIONS.md` e siga.
- Docs novos em PT-BR. Código e comentários de código em inglês.

## Arquitetura (curta)

`types` → `graph` (mutable + ImmutableGraph COW) → `mvcc` → `analysis` (divergence, tension, curvature, calibrator, concealment, yharim, temporal, laplacian) → `engine` / `snapshot` → `builder`. `graph.Graph` já implementa `analysis.GraphView`.
