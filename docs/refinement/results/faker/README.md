# Tarefa 8: caso real faker.js / colors.js (não executada)

**Status: bloqueada. Nenhum dado foi baixado, processado ou inventado.**

- Em 2026-09-29, o proxy de rede do ambiente de nuvem recusou o host `data.gharchive.org` (`CONNECT tunnel failed, response 403`, em `https://data.gharchive.org/2022-01-06-15.json.gz`). Uma segunda tentativa foi barrada pelo classificador de permissões do ambiente antes de sair. Não houve medição de vazão, então também não há projeção de tempo nem de volume.
- **Por isso, este diretório não tem:**
  - CSV agregado;
  - top 20 do M5/M6;
  - posição do `Marak`;
  - contagem de alarmes;
  - φ̂;
  - τ-jsd.
- **O que está pronto para rodar quando o host for liberado:**
  - `EdgeZ` com `CorrectedZConfig()` e o `ScanZ` das Tarefas 4 e 6 aceitam qualquer `Dataset`;
  - o `estimatePhi` devolve o φ̂ por faixa.
  - **Falta** o leitor de GH Archive: streaming por arquivo, filtro por `repo.name`, agregação ator→repo por dia, saída em `.cache/`.
- **Ecológico (Claytonia):** fora de escopo, como o plano pede. O plano registra que o arquivo `itt-poc/data/ecology/interactions.csv` é uma página HTML de anti-bot, não dado. O repositório `itt-poc` não existe neste container, então não consegui confirmar isso.

Para liberar, é preciso incluir `data.gharchive.org` nos domínios permitidos da rede do ambiente e permitir o `curl` para esse host. Ver Q8 em `docs/refinement/OPEN-QUESTIONS.md`.
