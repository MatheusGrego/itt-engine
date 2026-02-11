# Fio 2 — Loss Function / Mapa (Ideia Arquivada)

**Status**: Pausado. Não necessário para SEC. Revisitar se integrar ML à ITT.

## O que é

Uma Loss Function proposta pelo Gemini que poderia servir como "mapa" da ITT:

$$L_{total} = \lambda_1 L_{struct} + \lambda_2 L_{semant} + \lambda_3 L_{crit}$$

- **L_struct** = (τ_calc - √JSD)² — calibração da tensão vs divergência real
- **L_semant** = 1 - cos(S⃗, Φ⃗) — alinhamento semântico (domain-specific, não universal)
- **L_crit** = exp(τ_obs / (Υ - τ_obs)) — penalidade exponencial perto do Yharim Limit

## O que vale extrair

**Só o L_crit é genuinamente útil.** Ele define um "campo gravitacional" do Yharim:

$$\Psi(\tau) = \exp\left(\frac{\tau}{\Upsilon - \tau}\right)$$

- ≈ 1 para τ ≈ 0 (sem urgência)
- → ∞ para τ → Υ (horizonte de detecção)

Poderia virar multiplicador de urgência no CPS: `CPS_enhanced = CPS · Ψ(τ_max)`

## Problemas identificados

- **L_struct é circular**: τ já é baseado em JSD, minimizar a diferença não faz sentido
- **L_semant sai do escopo**: vincula ITT a NLP, quebrando domain-agnosticism
- **L_total como loss** só faz sentido se houver um modelo treinável (NN) — a ITT engine calcula analiticamente

## Quando revisitar

- Se integrar neural network pra prever tensão (τ_pred vs τ_real)
- Se quiser um "risk score" contínuo mais sofisticado que CPS
- Se quiser visualizar a "landscape" de detectabilidade como superfície 3D

## Frase guardada

> "É como medir um buraco negro não pelo buraco negro em si, mas pela curvatura do espaço ao redor."

Potencial POC com domínio astronômico usando essa analogia.
