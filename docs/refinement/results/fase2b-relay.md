# Tarefa 7: relay latente (S6)

Gerado por `go run ./cmd/itt-harness -exp relay -r 100`.

Parâmetros: N = 500 (menos o relay latente), T = 12, t0 = 8, R = 100 réplicas por linha, α = 0.05. Exploratório.

Modelo: o relay v (grau mediano) é latente: v e as arestas dele nunca aparecem nos dados. Cada par (n1 → v, v → n2) do grafo base vira um fluxo ponta a ponta n1 → n2, registrado sem o salto, com taxa lognormal da qual uma fração ρ passa por v. A partir de t0, v some e essa fração é perdida (não é redistribuída). O alvo é o conjunto dos vizinhos de v. No controle, v não some.

| ρ | v some? | método | AUC vizinhos × resto | vizinhos em alarme | outros em alarme | réplicas com alarme fora dos vizinhos [IC 95%] | nº vizinhos | nº fluxos | grau médio dos vizinhos |
|---|---|---|---|---|---|---|---|---|---|
| 0.2 | sim | silence-z-2pois-midp | 0.672 ± 0.115 | 0.4% | 0.006% | 0.03 [0.01, 0.08] | 6.8 | 9.4 | 19.5 |
| 0.2 | sim | silence-z-2pois-midp-qhat-phibands | 0.671 ± 0.114 | 0.4% | 0.006% | 0.03 [0.01, 0.08] | 6.8 | 9.4 | 19.5 |
| 0.5 | sim | silence-z-2pois-midp | 0.821 ± 0.118 | 5.1% | 0.006% | 0.03 [0.01, 0.08] | 6.8 | 9.4 | 19.5 |
| 0.5 | sim | silence-z-2pois-midp-qhat-phibands | 0.819 ± 0.119 | 5.2% | 0.006% | 0.03 [0.01, 0.08] | 6.8 | 9.4 | 19.5 |
| 0.8 | sim | silence-z-2pois-midp | 0.901 ± 0.118 | 27.1% | 0.006% | 0.03 [0.01, 0.08] | 6.8 | 9.4 | 19.5 |
| 0.8 | sim | silence-z-2pois-midp-qhat-phibands | 0.900 ± 0.119 | 26.6% | 0.006% | 0.03 [0.01, 0.08] | 6.8 | 9.4 | 19.5 |
| 0.5 | não (controle) | silence-z-2pois-midp | 0.516 ± 0.121 | 0.0% | 0.006% | 0.03 [0.01, 0.08] | 6.8 | 9.4 | 19.5 |
| 0.5 | não (controle) | silence-z-2pois-midp-qhat-phibands | 0.516 ± 0.122 | 0.0% | 0.006% | 0.03 [0.01, 0.08] | 6.8 | 9.4 | 19.5 |

## Leitura (escrita à mão; a tabela acima é gerada)

- **A ausência de um nó nunca observado deixa rastro quando existe dependência entre arestas.** No controle (v não some), a AUC dos vizinhos é 0.52 ± 0.12, no nível do chute. Com a queda:

  | ρ | AUC | vizinhos em alarme |
  |---|---|---|
  | 0.2 | 0.67 | 0.4% |
  | 0.5 | 0.82 | 5% |
  | 0.8 | 0.90 | 27% |

  Os falsos alarmes fora dos vizinhos ficam no nível do nulo (réplicas com alarme 0.03).
- **O rastro é difuso.** O M5 aponta os vizinhos de v, porque o v não está nos dados. Cada vizinho perde só os fluxos que passavam por v: ~9 fluxos divididos entre ~7 vizinhos, com grau médio ~19. É o mesmo problema do silêncio parcial da Tarefa 5, a diluição por √k. Mesmo com ρ = 0.8, só 27% dos vizinhos alarmam.
- **Localizar o v exige um segundo passo que o M5 não faz.** Seria inferir que os vizinhos em silêncio têm fluxos entre si (n1 → n2 caindo juntos), ou seja, testar arestas e não nós, e procurar o "buraco" comum. É o que o H-LoG do backlog propõe, e ele depende de um campo de silêncio confiável.
- As correções (q̂, φ̂) não mudam nada aqui (gerador Poisson).
- A interpretação do modelo de relay é minha (Q9).
