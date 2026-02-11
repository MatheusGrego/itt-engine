# ARGOS — Fontes de Dados

## 1. SEC EDGAR API (Primária)

### Base
```
URL:            https://data.sec.gov
Auth:           Nenhuma (API pública)
Rate Limit:     10 requests/segundo
User-Agent:     OBRIGATÓRIO — "AppName/Version (contact@email.com)"
Response:       JSON ou XML dependendo do endpoint
```

### 1.1 Company Search (Ticker → CIK)

**Endpoint**: `https://efts.sec.gov/LATEST/search-index?q={ticker}&dateRange=custom&startdt=2024-01-01&enddt=2024-12-31&forms=4`

**Alternativa (mais confiável)**: Download do mapping completo:
```
https://www.sec.gov/files/company_tickers.json
```

**Response (company_tickers.json)**:
```json
{
  "0": {"cik_str": 320193, "ticker": "AAPL", "title": "Apple Inc."},
  "1": {"cik_str": 789019, "ticker": "MSFT", "title": "MICROSOFT CORP"},
  ...
}
```

**Uso**: Cachear localmente. Atualiza raramente (novas IPOs).

---

### 1.2 Submissions API (Filings por CIK)

**Endpoint**: `https://data.sec.gov/submissions/CIK{cik_padded}.json`

**CIK padding**: CIK deve ser zero-padded para 10 dígitos. Ex: `320193` → `CIK0000320193`

**Response** (simplificada):
```json
{
  "cik": "320193",
  "entityType": "operating",
  "name": "Apple Inc.",
  "tickers": ["AAPL"],
  "exchanges": ["Nasdaq"],
  "filings": {
    "recent": {
      "accessionNumber": ["0001234-24-000001", ...],
      "filingDate": ["2024-06-15", ...],
      "form": ["4", "10-K", "8-K", ...],
      "primaryDocument": ["xslForm4X01/primary_doc.xml", ...],
      "primaryDocDescription": ["FORM 4", ...],
      ...
    },
    "files": [
      {"name": "CIK0000320193-submissions-001.json", "filingCount": 1000}
    ]
  }
}
```

**Uso**: Filtrar por `form == "4"` ou `form == "8-K"`. Usar `accessionNumber` para construir URL do filing.

**Pagination**: `files[]` contém JSONs adicionais para empresas com muitos filings.

---

### 1.3 Form 4 Individual Filing (XML)

**URL**: `https://www.sec.gov/Archives/edgar/data/{cik}/{accession_no_dashes}/{primary_doc}`

**Construção da URL**:
```
accessionNumber = "0001234-24-000001"
accession_no_dashes = "000123424000001"  // remove hífens
primary_doc = "xslForm4X01/primary_doc.xml"

URL = https://www.sec.gov/Archives/edgar/data/320193/000123424000001/xslForm4X01/primary_doc.xml
```

**XML Structure** (campos relevantes):
```xml
<ownershipDocument>
  <issuer>
    <issuerCik>0000320193</issuerCik>
    <issuerName>Apple Inc.</issuerName>
    <issuerTradingSymbol>AAPL</issuerTradingSymbol>
  </issuer>
  
  <reportingOwner>
    <reportingOwnerId>
      <rptOwnerCik>0001234567</rptOwnerCik>
      <rptOwnerName>COOK TIMOTHY D</rptOwnerName>
    </reportingOwnerId>
    <reportingOwnerRelationship>
      <isDirector>1</isDirector>
      <isOfficer>1</isOfficer>
      <officerTitle>Chief Executive Officer</officerTitle>
      <isTenPercentOwner>0</isTenPercentOwner>
    </reportingOwnerRelationship>
  </reportingOwner>
  
  <nonDerivativeTable>
    <nonDerivativeTransaction>
      <securityTitle><value>Common Stock</value></securityTitle>
      <transactionDate><value>2024-06-10</value></transactionDate>
      <transactionCoding>
        <transactionCode>S</transactionCode>  <!-- P=Purchase, S=Sale, M=Exercise, A=Award, G=Gift -->
      </transactionCoding>
      <transactionAmounts>
        <transactionShares><value>50000</value></transactionShares>
        <transactionPricePerShare><value>195.50</value></transactionPricePerShare>
        <transactionAcquiredDisposedCode><value>D</value></transactionAcquiredDisposedCode>  <!-- A=Acquired, D=Disposed -->
      </transactionAmounts>
      <postTransactionAmounts>
        <sharesOwnedFollowingTransaction><value>3340611</value></sharesOwnedFollowingTransaction>
      </postTransactionAmounts>
      <ownershipNature>
        <directOrIndirectOwnership><value>D</value></directOrIndirectOwnership>  <!-- D=Direct, I=Indirect -->
      </ownershipNature>
    </nonDerivativeTransaction>
  </nonDerivativeTable>
  
  <derivativeTable>
    <!-- Similar structure for options, warrants, etc. -->
  </derivativeTable>
  
  <remarks>
    <!-- Footnotes, 10b5-1 plan references -->
  </remarks>
</ownershipDocument>
```

**Parsing Rules**:
- Um Form 4 pode conter MÚLTIPLAS transações (nonDerivativeTransaction)
- Um Form 4 pode conter MÚLTIPLOS reportingOwners (filing conjunto)
- `transactionCode`: P=Purchase, S=Sale, M=Option Exercise, A=Award/Grant, F=Tax Withhold, D=Disposition to Issuer, G=Gift
- `remarks` pode mencionar "Rule 10b5-1" — flag para atenuação de peso
- `derivativeTable` contém opções — tratar como `transactionCode=M` quando exercidas

---

### 1.4 Form 8-K (Corporate Events)

**Acesso**: Via Submissions API (filtrar `form == "8-K"`), depois baixar o filing.

**Classificação por Item Number** (no header do 8-K):
| Item | Tipo de Evento |
|---|---|
| 1.01 | Entry into Material Agreement |
| 1.02 | Termination of Material Agreement |
| 2.01 | Completion of Acquisition/Disposition |
| 2.02 | Results of Operations (Earnings) |
| 5.01 | Changes in Control |
| 5.02 | Departure/Appointment of Officers |
| 8.01 | Other Events |

**Parsing**: Plain text ou HTML. Extrair item numbers do header para classificar automaticamente.

---

### 1.5 Form 13F (Institutional Holdings)

**Endpoint**: Via Submissions API (filtrar `form == "13F-HR"`).

**Estrutura**: XML com tabela de holdings.

**Campos por holding**:
```xml
<infoTable>
  <nameOfIssuer>APPLE INC</nameOfIssuer>
  <titleOfClass>COM</titleOfClass>
  <cusip>037833100</cusip>
  <value>1234567</value>  <!-- em milhares de dólares -->
  <sshPrnamt>50000</sshPrnamt>  <!-- shares -->
  <sshPrnamtType>SH</sshPrnamtType>
  <investmentDiscretion>SOLE</investmentDiscretion>
</infoTable>
```

**Uso**: Comparar quarter-over-quarter para detectar posições novas ou aumentos significativos.

---

## 2. Yahoo Finance API (Secundária)

### Endpoint (Informal, gratuita)
```
https://query1.finance.yahoo.com/v8/finance/chart/{ticker}?interval=1d&range=3mo
```

**Dados**: OHLCV (Open, High, Low, Close, Volume) diários.

**Uso**:
- Verificar se preço subiu após compra anômala de insider → reforça suspeição
- Earnings dates para correlação temporal
- Volume de mercado para contextualizar volume de insider

**Alternativa**: `yfinance` Python library ou Alpha Vantage API (free tier: 25 req/dia).

---

## 3. Bulk Download (Dados Históricos)

Para análise histórica em massa:

```
https://www.sec.gov/files/structureddata/data/insider-transactions-data-sets/
```

Contém datasets completos de insider transactions em CSV, organizados por trimestre. Ideal para:
- Validação contra casos históricos
- Construção de baselines estatísticas
- Treinamento inicial do MAD calibrator

---

## 4. Litigation Releases (Ground Truth)

```
https://www.sec.gov/litigation/litreleases.htm
```

Lista de enforcement actions da SEC. Filtrar por insider trading. Usar para validação:
1. Extrair CIKs dos indivíduos condenados
2. Extrair data do evento e da ação
3. Rodar ARGOS nos dados da época
4. Verificar se o sistema teria detectado

### Casos de Referência para Validação

| Caso | Ano | Tipo | Valor | Detalhes |
|---|---|---|---|---|
| Martha Stewart / ImClone | 2001 | Tippee sells | $228k | FDA denial + tip do CEO |
| Raj Rajaratnam / Galleon | 2009 | Hedge fund ring | $63M | Rede de tippees em tech |
| SAC Capital / Cohen | 2013 | Institutional | $1.8B | Expert network + trading |
| Chris Collins (congressista) | 2017 | Tippee network | $768k | Innate Immunotherapeutics |
| Panuwat / Medivation | 2021 | Shadow trading | $117k | Compra de empresa similar antes de M&A |

**Nota sobre "shadow trading" (caso Panuwat)**: Insider comprou ações de OUTRA empresa (concorrente) antes da M&A da sua. Isso é detectável pela ITT via co-insider graph se ambas empresas compartilham insiders ou setor.
