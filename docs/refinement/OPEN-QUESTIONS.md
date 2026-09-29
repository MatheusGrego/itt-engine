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

## Q4 (E1/ADR-0002, 2026-09-29): arestas que só aparecem depois de t0

- **Fato:** no M3/M4 do harness, uma aresta sem nenhum evento em [0, t0) tem `e = 0`. O plano manda aplicar o floor de 0.5 só em arestas que existiam antes, então para `o > 0` o desvio fica `o·ln(o/0)` = infinito.
- **Decidido:** essas arestas ficam fora de S e de B (`harness.Deviance`). Isso não mexe no S (com `e = 0` não existe `o < e`), só no B: aresta nova não conta como burst.
- **Por quê:** é a leitura literal do plano sem produzir infinito. Um B infinito empataria no topo todo nó que ganhou uma aresta por acaso (com λ ≈ 0.3, uma aresta real fica 8 janelas zerada com chance de ~9%).
- **Pergunta:** aresta nova deveria contar como burst (ex.: `e` = floor de 0.5 também para ela, ou um prior por nó)? Isso é decisão da ADR-0003 (modelo nulo).

## Q5 (ADR-0002/0004, 2026-09-29): o limiar em bits não segura o FP sob o nulo

- **Fato:** no harness, com a regra do plano (S/2 > ln(1/α) + ln N, α = 0.05), o silence deviance alarma em 8.35% dos nós por réplica sob o S0 e o FWER é 1.00. O desvio médio por aresta sob o nulo é 1.54 (esperado 1 para χ²₁ com `e` conhecido): o `e` estimado de 8 janelas soma a variância dele à do `o`, e (T − t0)/t0 = 0.5 prevê exatamente esse fator de ~1.5. Além disso, S(v) soma todas as arestas do nó, então cresce com o grau (grau médio 28.2 nos nós em alarme contra 9.4 no geral).
- **Decidido:** nada mudou no método nem na regra. Os números entram como resultado na ADR-0002 e na ADR-0006, sem mexer em status.
- **Pergunta (para a ADR-0004):** qual calibração? Opções que vi, sem testar:
  1. comparar S(v) com o quantil de χ² com k = número de arestas do nó (lidando com o fato de S só somar o lado o < e);
  2. inflar o esperado pelo ruído do baseline (fator 1 + (T − t0)/t0) ou usar um teste de razão entre dois Poisson (antes contra depois) em vez de tratar `e` como conhecido;
  3. nulo empírico: calibrar o limiar por grau no próprio S0 ou por permutação de janelas.

## Q6 (ADR-0002/0004, fase 2b T2, 2026-09-29): variante por aresta do M5/M6

- **Fato:** a regra do plano (FWER mais perto de α no S0 default, menor perda de AUC) empatou entre o signed root e o mid-p. A distância média ao α foi 0.0125 nas duas, com IC de ±0.03 em R = 200, e a perda de AUC foi ≈ 0. A calibração por nó (10⁵ amostras) mostrou que o signed root tem viés negativo que cresce com o grau (b·√k no Stouffer), e em pouca contagem o FWER de silêncio dele é 0.130. Os números estão em `docs/refinement/results/fase2b-discreteness.md`.
- **Decidido:** o default do M5/M6 no harness passa a ser o **mid-p exato**, a opção conservadora e a única com FWER ≤ α nas duas condições. O signed root e o n_min = 3 continuam como opções (`ZConfig`).
- **Custo conhecido:** com pouca contagem o mid-p é sub-disperso (var Z = 0.81), então perde poder (S1 0.49 contra 0.90).
- **Pergunta:** vale padronizar cada aresta pela média e pelo desvio exatos do mid-p sob H0 dado n? Dá para calcular pela binomial, e isso devolve var 1 sem viés. Não testei, porque muda a estatística e não estava no plano.
