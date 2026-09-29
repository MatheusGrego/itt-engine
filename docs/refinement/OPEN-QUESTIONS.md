# Perguntas abertas e decisões tomadas sem o Matheus

Regra (CLAUDE.md): quando uma decisão trava e muda a teoria, fica a opção mais conservadora, registrada aqui, e o trabalho segue. Cada item diz o que foi decidido, por quê e o que falta o Matheus responder.

Formato: `Q<n> (ID do backlog, data)`.

---

## Q1 (B1, 2026-09-29): KL no caso p = 1

- **Decidido:** o teto da KL (ilimitada) é `log2(1/epsilon)` ≈ 39.86, com `epsilon = 1e-12` de `analysis/divergence.go`. Divergências customizadas sem `MaxValue()` são avaliadas em duas massas pontuais disjuntas, que é o supremo de qualquer f-divergência.
- **Por quê:** o plano pediu esse teto. Ele coincide com o que a própria `klDiv` devolve para massas disjuntas, então não cria uma escala nova.
- **Pergunta:** vale manter a KL como opção de tensão? Com esse teto, um único vizinho com p = 1 domina a média (39.86 contra valores < 1 nos outros).

## Q2 (B1, 2026-09-29): alertas em tempo real no aquecimento

- **Fato:** depois do B1, o `OnAnomaly` (que roda por evento sobre o grafo parcial) dispara com τ = 1 para o destino do primeiro evento de toda fonte nova, porque nesse momento a fonte manda 100% do peso pra ele. Antes do B1 isso dava 0 por causa do bug.
- **Decidido:** nada muda no código (fora de escopo e mudaria default). O README agora avisa disso na seção de limitações.
- **Pergunta:** o callback em tempo real deveria exigir um suporte mínimo (ex.: W_out(n) ou número de eventos da fonte) antes de avaliar? Isso é da Fase 2/3, quando o τ virar evidência com volume.

## Q3 (B2, 2026-09-29): o exemplo do README "detectava", mas pelo motivo errado

- **Fato:** depois do B1, o exemplo antigo acha 2 anomalias: `service:api` e `service:db`, ambos com τ = 0.6556. O nó comentado como `// suspicious` (`user:charlie`, peso 50) continua com τ = 0, e o peso 50 não muda nada (com peso 1 o resultado é o mesmo, T3).
- **Decidido:** reescrevi o exemplo mesmo assim. O plano só pedia reescrita se ele não detectasse nada, mas o exemplo antigo sugeria que a engine pega o charlie, o que é falso. O novo cenário sinaliza `service:legacy` (τ = 0.7744) porque dois usuários dependem dele, que é o que o τ mede hoje. O exemplo roda como `ExampleEngine_readme` (`example_test.go`) com a saída conferida.
- **Também:** as seções novas do README estão em inglês, porque o README inteiro é em inglês (a regra de PT-BR vale para docs novos). A tabela de divergências dizia JSD em [0, ln2]; o código usa log2, então corrigi para [0, 1].
- **Pergunta:** nenhuma bloqueante. Se preferir o README em PT-BR, é só trocar.
