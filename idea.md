# NEEDLE-X
## Cognitive Web Retrieval Runtime with Surgical SLM Integration

## Core Idea
"Build a local-first, serverless, single-binary Go runtime that transforms noisy web pages into compact, high-signal context fragments using deterministic pruning and selectively activated small language models for ambiguity resolution, ensuring minimal token usage and maximal extraction fidelity."

Documenti operativi:
- `spec.md`
- `README.md`
- `docs/architecture.md`
- `docs/folder-tree.md`
- `docs/development-plan.md`

## 1) Executive Thesis (2 Generazioni Avanti)
Needle-X non deve essere solo un altro scraper per LLM.
Deve creare una nuova categoria: **Verified Web Context Runtime (VWCR)**.

Tesi di rottura:
1. Il web non e' un input testuale, e' un sistema rumoroso da compilare.
2. Il contesto per agenti deve essere verificabile, ripetibile e con costo predicibile.
3. I modelli grandi non sono il motore: sono edge functions rare per risolvere ambiguita'.
4. Il vantaggio competitivo non e' "piu' pagine", ma **piu' fedelta' per token**.
5. L'output primario non deve essere il bundle diagnostico del runtime, ma il **contesto compilato piu' piccolo e utile possibile**.

Posizionamento:
- "Needle-X compila il web in contesto macchina verificabile."

## 2) Research Snapshot (Stato del Mercato e Gap Reale)

### 2.1 Segnali da benchmark agentici
1. WebArena mostra gap enorme tra agenti e umani (GPT-4-based 14.41% vs umani 78.24%): il web reale resta fragile per agenti.
2. WebVoyager alza la performance (59.1%), ma conferma che affidabilita' e valutazione robusta restano problemi centrali.
3. Mind2Web formalizza la necessita' di agenti generalisti web-aware, ma non risolve il layer di "context fidelity" in produzione.

### 2.2 Segnali dal mercato prodotti
1. Firecrawl eccelle nel "turn any URL into clean data" ma resta API-first cloud-centric.
2. Browserbase risolve browser infrastructure scalabile, non la prova di fedelta' del contenuto estratto.
3. Exa introduce ricerca+verifica per dataset strutturati, ma non e' focalizzato su runtime locale deterministic-first.
4. Diffbot costruisce knowledge graph massive-scale, ma con logica piattaforma centralizzata.

### 2.3 Segnali tecnologici utili
1. MCP sta emergendo come standard d'interoperabilita' tool/agent.
2. Estrattori euristici solidi (es. Trafilatura) confermano valore del deterministic-first.
3. SLM specialistici (es. ReaderLM-v2) mostrano che piccoli modelli verticali possono battere pipeline generaliste nel task HTML->Markdown/JSON.

Conclusione research:
- Lo spazio e' saturo su "estrarre".
- Lo spazio e' ancora aperto su **estrarre con garanzie formali, replay e proof artifacts**.

## 3) Categoria Nuova: Verified Web Context Runtime (VWCR)
Needle-X deve definire lo standard di categoria con 4 pilastri:
1. **Deterministic Core**: stessa pagina + stessa policy = stesso output.
2. **Proof-Carrying Context**: ogni chunk porta traccia di origine e trasformazioni.
3. **Selective Intelligence**: SLM attivati solo su ambiguita' misurata.
4. **Economic Control Plane**: budget token/latenza/qualita' governati da policy.

## 4) Funzionalita' di Rottura (Fuori dagli Schemi)

### 4.1 Web IR (Intermediate Representation)
Formato canonico intermedio del web, tipo "LLVM del web context":
- nodi semantici normalizzati
- provenienza DOM
- segnali di rumore
- ancore strutturali

Effetto:
- strategia extraction indipendente dal sito
- debugging e replay reali
- base per ottimizzazioni automatiche

### 4.2 Proof-Carrying Chunks
Ogni chunk include:
- `source_span` (xpath/css/range)
- `transform_chain` (regole applicate)
- `confidence`
- `fingerprint`
- `risk_flags`

Effetto:
- agenti che possono fidarsi e citare
- auditability enterprise-grade

### 4.3 Deterministic Replay Engine
Comando `needle replay <trace_id>`:
- ricostruisce ogni stage
- diff stage-by-stage
- evidenzia regressioni extraction

Effetto:
- debug da software engineering, non da prompt tweaking

### 4.4 Ambiguity-Triggered SLM
SLM invocato solo con trigger espliciti:
- conflitto fra estrattori
- entropy score alto
- perdita strutturale oltre soglia

Effetto:
- costo basso
- comportamento prevedibile
- piu' robustezza operativa

### 4.5 Domain Genome Learning
Profilo per dominio/sito aggiornato automaticamente:
- pattern boilerplate
- necessita' JS rendering
- lane preferita
- failure signatures

Effetto:
- runtime che migliora nel tempo senza retraining pesante

### 4.6 Extraction Fingerprint Graph
Fingerprint cross-run/cross-query per:
- dedup intelligente
- cache semantica
- delta extraction nel tempo

Effetto:
- meno fetch inutili
- meno token
- timeline dei cambiamenti reali

### 4.7 Retrieval Compiler
L'agente dichiara obiettivo, Needle-X compila piano:
- quali pagine
- quale profondita'
- quale lane
- quale budget

Effetto:
- da tool-call imperative a execution plan ottimizzato

### 4.8 Constraint-First Model Output
Quando il modello e' usato, output sempre vincolato:
- JSON schema
- tag set chiuso
- no long-form libero se non richiesto

Effetto:
- integrazione software affidabile
- minor rischio output malformati

## 5) Obiettivo Prodotto
Trasformare il web da input entropico a **contesto macchina verificabile** per agenti e sistemi RAG con:
- minimal token usage
- maximal extraction fidelity
- deterministic behavior by default

Conseguenza di prodotto:
1. `read/query/crawl` devono esporre di default payload compatti agent-facing
2. proof, trace, replay e artifact completi restano first-class, ma in modalita' esplicita
3. Needle-X non vince se produce piu' JSON dell'HTML originale; vince se consegna meno token con piu' segnale e provenance sufficiente

## 6) Architettura Next-Gen (Minimal Basecode)

### 6.1 Design constraints
1. Local-first
2. Single binary Go
3. Zero mandatory external services
4. Deterministic-by-default
5. Plug-in points minimi e stabili

### 6.2 Microkernel Runtime
Core minimale (obbligatorio):
1. Fetcher
2. DOM reducer
3. Segmenter
4. Deterministic extractor
5. Chunker + ranker
6. Packer
7. Trace/proof writer

Moduli avanzati (inclusi dal Day 1, attivati via policy):
1. SLM Router/Judge
2. Formatter
3. JS rendering adapter
4. Domain genome trainer

### 6.2.1 Topologia Minima del Repo
Per mantenere il codice leggero, Needle-X deve convergere su:
1. un solo binary
2. un solo core runtime
3. meno di 10 package interni
4. nessuna duplicazione logica tra CLI e MCP
5. strumenti di controllo budget codice fin dal primo commit

### 6.3 Pipeline
`Acquire -> Prune -> Segment -> Extract(A/B) -> Judge -> Chunk -> Rank -> Pack -> Proof`

### 6.4 Intelligence Lanes
Lane 0: deterministic-only
Lane 1: router+judge
Lane 2: SLM extraction
Lane 3: formatter
Lane 4: remote burst (eccezione)

Policy default:
- prefer lane piu' basso
- escalate solo con evidenza
- ritorno forzato a deterministic quando possibile

## 7) API Surface Minima
CLI:
- `needle read <url>`
- `needle query "<goal>" --seed <url>`
- `needle crawl <url>`
- `needle replay <trace_id>`
- `needle diff <trace_a> <trace_b>`

Regola di superficie:
1. default = contesto compilato compatto
2. full diagnostics = opt-in esplicito
3. l'agente deve poter navigare rapidamente senza dover filtrare trace, proof store e WebIR completi

MCP:
- `web_read`
- `web_query`
- `web_crawl`
- `web_expand`
- `web_replay`
- `web_diff`
- `web_proof`

## 8) Modello Dati (V2)

`Document`:
- `id, url, final_url, title, fetched_at, fetch_mode, raw_hash`

`Chunk`:
- `id, doc_id, text, heading_path, score, fingerprint, confidence`

`Proof`:
- `chunk_id, source_span, transform_chain[], lane, model_invocations[], risk_flags[]`

`ResultPack`:
- `query, objective, chunks[], sources[], proof_refs[], cost_report`

## 9) KPI di Frontiera
1. Fidelity@k (aderenza al contenuto sorgente verificato)
2. Token Compression Ratio
3. Determinism Score (run-to-run invariance)
4. Cost per successful context pack
5. Escalation Rate by lane
6. Replay Debug Time (MTTR extraction regressions)

## 10) Moat Strategico
Moat tecnici cumulativi:
1. Web IR proprietario
2. Proof corpus storico
3. Domain genome database
4. Replay traces e regressions suite

Moat di distribuzione:
1. MCP-native
2. local-first per compliance/privacy
3. single-binary install friction minima

## 11) Day 1 Full-Scope Launch Plan (No Stubs, No Empty Versions)
Obiettivo: rilasciare al Giorno 1 un sistema completo, coerente e production-grade in tutte le capability strategiche.

### 11.1 Scope obbligatorio al Day 1
1. Deterministic core completo (Acquire/Prune/Segment/Extract/Chunk/Rank/Pack)
2. Web IR operativo con schema versionato
3. Proof-carrying chunks completi (`source_span`, `transform_chain`, `confidence`, `risk_flags`)
4. SLM Router/Judge con policy di attivazione ambiguita'-driven
5. Formatter vincolato (JSON schema/tag-set)
6. Replay engine + diff stage-by-stage
7. Domain genome v1 funzionante
8. Extraction fingerprint graph + dedup/cache
9. Retrieval compiler v1 (goal -> execution plan con budget)
10. CLI e MCP completi (`read/query/crawl/replay/diff/proof`)
11. Security guardrails hard-enforced
12. Observability end-to-end (trace, motivi escalation, cost report)

### 11.2 Definition of Done (vincolante)
Una release e' \"Day 1 complete\" solo se:
1. Ogni feature sopra ha test automatici unitari + integrazione + regressione
2. Ogni lane (0-4) ha benchmark e threshold di accettazione dichiarati
3. Replay produce output ripetibile con varianza entro soglia definita
4. Fidelity benchmark pubblico e riproducibile
5. Nessun comando MCP/CLI risulta placeholder o \"not implemented\"
6. Ogni output include proof artifacts validabili

### 11.3 Piano di esecuzione parallelo (stessa finestra temporale)
Track A - Runtime Core:
- fetcher, reducer, segmenter, extractor, ranker, packer

Track B - Verification Plane:
- Web IR, proof schema, replay/diff, fingerprinting

Track C - Intelligence Plane:
- router/judge, formatter vincolato, retrieval compiler

Track D - Reliability & Security:
- guardrails, timeout/budget caps, anti-regression suite

Track E - Product Surface:
- CLI completa, MCP completa, docs tecniche + benchmark report

Regola: i track avanzano in parallelo, ma il launch avviene solo con completezza di tutti i track.

### 11.4 Launch Gate Unico
Go-live consentito solo con:
1. KPI minimi raggiunti (Fidelity@k, determinism score, token compression ratio, p95 latency)
2. Nessun debito funzionale critico rimandato
3. Nessuna capability strategica posticipata a \"v2\"

## 12) Rischi e Mitigazioni
1. Rischio: complessita' eccessiva
- Mitigazione: core microkernel + feature flags

2. Rischio: performance su siti JS-heavy
- Mitigazione: rendering adapter opzionale + domain genome routing

3. Rischio: claim "fidelity" non difendibile
- Mitigazione: benchmark pubblico, golden set, replay deterministico

4. Rischio: copia funzionale dei competitor
- Mitigazione: puntare su proof/replay/IR standard, non solo scraping quality

## 13) Basecode Minimo (Pragmatico)
Per rimanere leggeri:
1. Dipendenze ridotte al minimo (HTTP, HTML parsing, scoring, logging)
2. Nessun DB obbligatorio: storage locale append-only + files index
3. Nessun orchestratore esterno
4. Config singolo file + env overrides
5. Modello plugin semplice (interfacce Go, no framework complessi)

Target runtime:
- cold start quasi istantaneo
- RAM contenuta
- modalita' offline per deterministic lanes

## 14) Decisione Strategica Finale
Se vuoi davvero rottura mercato:
- non vendere "scraping migliore"
- vendi "contesto verificabile come infrastruttura critica per agenti"

Frase di categoria:
**Needle-X e' il compilatore del web in contesto affidabile per AI agents.**

## 15) Riferimenti di Ricerca e Mercato
- WebArena paper: https://arxiv.org/abs/2307.13854
- Mind2Web paper: https://arxiv.org/abs/2306.06070
- WebVoyager paper: https://arxiv.org/abs/2401.13919
- ReaderLM-v2 paper: https://arxiv.org/abs/2503.01151
- MCP specification: https://modelcontextprotocol.io/specification/versioning
- Firecrawl Scrape docs: https://docs.firecrawl.dev/features/scrape
- Browserbase overview: https://www.browserbase.com/
- Exa Websets/API overview: https://docs.exa.ai/websets/api/overview
- Diffbot platform overview: https://www.diffbot.com/
- Trafilatura usage docs: https://trafilatura.readthedocs.io/en/latest/usage-python.html
