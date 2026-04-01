# NEEDLE-X SPEC
Version: 0.2
Status: Active product/runtime contract
Language: IT/EN technical

Related docs:
- `README.md`
- `docs/architecture.md`
- `docs/agent-answer-packet.md`
- `docs/folder-tree.md`
- `docs/development-plan.md`
- `docs/semantic-alignment-gate.md`
- `docs/model-baseline.md`
- `docs/experimental/discovery-memory-spec.md`

## 1. Scopo
Questo documento definisce la specifica tecnica e di prodotto di Needle-X come:
- runtime Go local-first
- serverless
- single-binary
- deterministic-first
- SLM policy-gated per risoluzione ambiguita'

Obiettivo: trasformare superfici web rumorose in contesto compatto, verificabile e semanticamente coerente per agenti, senza dipendere da matching lessicale come giudice principale del significato.

Principio di output:
1. la superficie primaria del prodotto deve essere il contesto compilato compatto
2. artifact diagnostici e di audit devono restare disponibili ma espliciti
3. il runtime non deve costringere agenti o operatori a consumare bundle completi quando basta il contesto finale

## 2. Obiettivi e Non-Obiettivi
### 2.1 Obiettivi
1. Ridurre token inviati ai modelli senza perdere informazione critica.
2. Massimizzare extraction fidelity con prove verificabili.
3. Garantire replay deterministico e debug stage-by-stage.
4. Offrire surface minima ma completa: CLI + MCP.
5. Supportare escalation modellata solo dove esiste vantaggio benchmark-backed.
6. Compilare contesto web multilingua sulla base del significato, non della sola coincidenza lessicale.

### 2.2 Non-Obiettivi
1. Non essere un motore di ricerca globale.
2. Non sostituire un browser general purpose.
3. Non fare automazione auth/form submit by default.
4. Non dipendere da servizi cloud obbligatori.
5. Non diventare un wrapper sottile di provider search esterni.

### 2.3 Priorita' Strategiche (Execution Order)

Current active runtime contract:
1. deterministic substrate
2. semantic context gate
3. bounded ambiguity solver only when benchmark-backed

1. Rendere `WebIR` un artifact first-class e versionato.
2. Evolvere `QueryPlan` in retrieval compiler con decisioni esplicite e reason code.
3. Costruire discovery nativa oltre il bootstrap provider-based.
3b. Costruire `Discovery Memory` client-local come substrate seedless senza infrastruttura obbligatoria.
4. Materializzare fingerprint graph per dedup/delta retrieval cross-run.
5. Usare modello locale solo dove fornisce vantaggio misurabile su casi ad alta ambiguita'.
6. Ottimizzare performance e benchmark senza compromettere il moat tecnico.

## 3. Definizioni
1. Web IR: rappresentazione intermedia canonica del contenuto web.
2. Proof artifact: metadati verificabili sulla provenienza e trasformazioni di un chunk.
3. Lane: livello di computazione da deterministic-only a model-assisted.
4. Ambiguity score: indice di incertezza che guida escalation lane.
5. Fidelity@k: percentuale di chunk top-k aderenti alla fonte reale.
6. Semantic alignment: misura di vicinanza contestuale tra obiettivo e chunk, anche cross-lingua.
7. Discovery Memory: memoria locale di pagine e segnali gia' osservati, usata per retrieval seedless prima del bootstrap pubblico.

## 4. Product Requirements (FR)
### 4.1 Acquisition
- FR-001: il sistema deve supportare fetch HTTP/HTTPS con redirect tracking.
- FR-002: il sistema deve registrare `final_url`, headers principali, status code e fetch_mode.
- FR-003: il sistema deve supportare render adapter attivabile via policy per siti JS-heavy.

### 4.2 DOM Reduction e Segmentazione
- FR-004: il sistema deve applicare pruning deterministico configurabile per rimuovere boilerplate.
- FR-005: il sistema deve produrre `SimplifiedDOM` con mapping al DOM originale.
- FR-005b: il sistema deve materializzare un `WebIR` canonico e versionato derivato dal DOM ridotto.
- FR-006: il sistema deve segmentare in regioni semantiche (heading, paragraph, list, code, table).

### 4.3 Estrazione
- FR-007: il sistema deve eseguire estrazione deterministica come percorso primario.
- FR-008: il sistema deve supportare escalation modellata solo con trigger policy.
- FR-008b: il sistema deve usare il modello locale come solver di task tipizzati e non come reader libero della pagina.
- FR-009: il sistema deve poter confrontare percorso deterministico e percorso con escalation senza confondere i due come identita' di prodotto.

### 4.4 Routing e Judge
- FR-010: il runtime deve classificare `page_type`, `difficulty`, `noise_level` e substrate utili al routing.
- FR-011: le decisioni di escalation devono avere motivazione esplicita.
- FR-012: ogni decisione context-sensitive deve essere tracciata con reason code.
- FR-012b: ogni task modello deve emettere output schema-locked validabile come decisione tipizzata.
- FR-012c: le decisioni context-sensitive devono usare segnali semantici come gate primario quando il significato non e' affidabile via matching lessicale.
- FR-012d: il matching lessicale puo' restare solo come segnale ausiliario, fallback cheap o euristica di rumore.

### 4.5 Chunking, Ranking, Packing
- FR-013: il sistema deve generare chunk con `id`, `text`, `heading_path`, `score`.
- FR-014: ranking deve usare segnali multipli configurabili (density, heading match, position, link context).
- FR-015: packing deve supportare profili `tiny`, `standard`, `deep`.

### 4.6 Proof-Carrying Context
- FR-016: ogni chunk deve avere proof artifact associato.
- FR-017: proof deve includere `source_span`, `transform_chain`, `lane`, `risk_flags`.
- FR-018: proof deve essere serializzabile e validabile con schema versionato.

### 4.7 Replay e Diff
- FR-019: il runtime deve salvare trace stage-by-stage per ogni esecuzione.
- FR-020: `replay` deve ricostruire deterministically gli step usando trace e input congelati.
- FR-021: `diff` deve mostrare differenze per stage e per chunk.

### 4.8 Domain Genome
- FR-022: il sistema deve mantenere profili dominio aggiornabili (`domain genome`).
- FR-023: profilo dominio deve influenzare lane preference, render need e pruning profile.
- FR-024: aggiornamenti profilo devono essere auditabili e reversibili.

### 4.9 Retrieval Compiler
- FR-025: input obiettivo utente deve essere tradotto in execution plan.
- FR-026: plan deve includere budget costo/latenza/qualita'.
- FR-027: plan deve essere serializzabile in formato machine-readable.
- FR-027b: il compiler deve supportare objective-to-chunk semantic alignment su web multilingua.
- FR-027c: il compiler deve poter consultare un substrate locale di `Discovery Memory` prima di usare bootstrap pubblico, quando disponibile.

### 4.10 Interfaccia
- FR-028: CLI deve includere `read`, `query`, `crawl`, `replay`, `diff`, `proof`.
- FR-029: MCP deve esporre tools equivalenti con schema I/O stabile.
- FR-030: output deve includere sempre `sources` e `cost_report`.
- FR-030b: output di `read/query` deve includere `web_ir` come artifact ispezionabile.
- FR-030c: output default di `read/query/crawl` deve privilegiare un payload compatto agent-facing ottimizzato per navigazione rapida e basso consumo token.
- FR-030d: payload diagnostici completi devono richiedere una modalita' esplicita, senza sostituire il contratto compatto di default.
- FR-030e: il payload compatto default deve essere ordinato per consumo AI-first e includere almeno locator primario, summary, uncertainty sintetica, evidenza minima inline, alternative di selezione quando rilevanti, segnali sintetici e costo.

### 4.11 Sicurezza e Guardrail
- FR-031: enforcement su `max_pages`, `max_depth`, `max_bytes`, `timeout`.
- FR-032: blocco default di form submit e azioni con side-effect.
- FR-033: no credential handling automatico senza policy esplicita.

### 4.12 Osservabilita'
- FR-034: metriche runtime p50/p95/p99 per stage.
- FR-035: log strutturati con `run_id`, `trace_id`, `stage`, `reason_code`.
- FR-036: export tracing OpenTelemetry-compatible opzionale.

### 4.13 Compatibilita'
- FR-037: single-binary Go per Linux/macOS/Windows.
- FR-038: modalita' offline supportata per lane deterministic.
- FR-039: config file unico con override env vars.

### 4.14 Qualita'
- FR-040: benchmark fidelity pubblico e riproducibile.

## 5. Non-Functional Requirements (NFR)
- NFR-001: percorso semplice <200ms p50 su hardware target.
- NFR-002: percorso complesso <2s p95 su hardware target.
- NFR-003: memoria resident contenuta (<512MB profilo standard).
- NFR-004: deterministic score >= 0.98 su suite replay.
- NFR-005: crash-free runs >= 99.9% in regression suite.
- NFR-006: token compression ratio minimo 3x baseline raw HTML.

## 6. Architettura
## 6.1 Componenti
1. `acquire`: fetch, redirect, content-type checks, optional render.
2. `reduce`: pruning boilerplate e normalizzazione DOM.
3. `segment`: region building semantico.
4. `extract_det`: estrazione deterministic.
5. `extract_slm`: estrazione assistita modello (policy-gated).
5b. `patch_validate`: validazione deterministica dei delta proposti dal modello.
6. `semantic_gate`: allineamento semantico multilingua per decisioni meaning-sensitive.
7. `judge`: compare outputs e decisione.
8. `chunk_rank`: chunking + scoring + selection.
9. `pack`: costruzione ResultPack.
10. `proof`: produzione artifact verificabili.
11. `trace`: event timeline + replay store.
12. `genome`: profili dominio.
13. `planner`: retrieval compiler.

### 6.2 Flusso
`Acquire -> Reduce -> Segment -> Extract_det -> SemanticGate -> (+ResolveAmbiguity) -> ChunkRank -> Pack -> Proof -> Trace`

### 6.3 Policy Engine
Input policy:
- quality target
- cost budget
- latency budget
- domain hints

Output policy:
- lane selection
- escalation thresholds
- allowed model calls

## 7. Lane System
### 7.1 Lanes
- Lane 0: deterministic-only
- Lane 1: deterministic + policy analysis
- Lane 2: typed local ambiguity solver
- Lane 3: reserved for future bounded expansion only if benchmark-backed
- Lane 4: not part of the active product contract

### 7.2 Trigger escalation
Escalation consentita se almeno una condizione:
1. `ambiguity_score > threshold_ambiguity`
2. `domain_profile.force_lane >= n`
3. il semantic gate non riesce a chiudere il caso con sufficiente dominanza contestuale
4. il top candidate non e' abbastanza supportato strutturalmente

### 7.3 De-escalation
Ritorno a lane inferiore quando:
1. qualità sufficiente raggiunta
2. budget residuo insufficiente
3. confidenza > threshold_confidence

## 8. Data Model (Canonical)
## 8.1 Document
```json
{
  "id": "doc_...",
  "url": "https://...",
  "final_url": "https://...",
  "title": "...",
  "fetched_at": "2026-03-27T20:00:00Z",
  "fetch_mode": "http",
  "raw_hash": "sha256:..."
}
```

### 8.2 Chunk
```json
{
  "id": "chk_...",
  "doc_id": "doc_...",
  "text": "...",
  "heading_path": ["..."],
  "score": 0.0,
  "fingerprint": "fp_...",
  "confidence": 0.0
}
```

### 8.3 Proof
```json
{
  "chunk_id": "chk_...",
  "source_span": {
    "selector": "css/xpath",
    "char_start": 0,
    "char_end": 128
  },
  "transform_chain": ["prune:v3", "segment:v2", "extract_det:v4"],
  "lane": 1,
  "model_invocations": [
    {
      "model": "local-slm-router",
      "purpose": "route",
      "tokens_in": 210,
      "tokens_out": 32,
      "latency_ms": 18
    }
  ],
  "risk_flags": ["partial_table", "possible_truncation"]
}
```

### 8.4 ResultPack
```json
{
  "query": "...",
  "objective": "...",
  "profile": "standard",
  "chunks": [],
  "sources": [],
  "outline": [],
  "links": [],
  "proof_refs": [],
  "cost_report": {
    "latency_ms": 0,
    "token_in": 0,
    "token_out": 0,
    "lane_path": [0, 1]
  }
}
```

## 9. CLI Specification
## 9.1 `needle read`
`needle read <url> [--profile tiny|standard|deep] [--json] [--json-mode compact|full]`

Output:
- contesto compilato compatto
- top chunks
- proof summary
- cost report

JSON modes:
- `--json` => payload compatto default
- `--json --json-mode full` => envelope diagnostico completo

Default compact contract:
- `Agent Answer Packet v1`

### 9.2 `needle query`
`needle query [seed-url] --goal "<goal>" [--budget-ms 1200] [--budget-tokens 8000] [--json] [--json-mode compact|full]`

Output:
- result pack orientato al goal
- motivazione ranking

JSON modes:
- `--json` => payload compatto default
- `--json --json-mode full` => envelope diagnostico completo

Default compact contract:
- `Agent Answer Packet v1`

### 9.3 `needle crawl`
`needle crawl <url> [--max-pages 20] [--max-depth 2] [--same-domain] [--json] [--json-mode compact|full]`

Output:
- summary compatto del crawl
- documenti compilati essenziali
- stored run ids

JSON modes:
- `--json` => payload compatto default
- `--json --json-mode full` => envelope completo quando richiesto

### 9.4 `needle replay`
`needle replay <trace_id> [--json] [--stage all|name]`

### 9.5 `needle diff`
`needle diff <trace_a> <trace_b> [--stage all|name] [--json]`

### 9.6 `needle proof`
`needle proof <chunk_id|proof_id|trace_id> [--json]`

## 10. MCP Tool Contracts
### 10.1 `web_read`
Input:
```json
{
  "url": "https://...",
  "profile": "standard",
  "lane_max": 2
}
```
Output:
```json
{
  "document": {},
  "web_ir": {},
  "chunks": [],
  "proof_refs": [],
  "cost_report": {}
}
```

### 10.2 `web_query`
Input:
```json
{
  "goal": "...",
  "seed_url": "https://... (optional)",
  "budget": {"ms": 1200, "tokens": 8000}
}
```
Output:
```json
{
  "plan": {},
  "document": {},
  "web_ir": {},
  "result_pack": {}
}
```

### 10.3 `web_crawl`
Input:
```json
{
  "seed_url": "https://...",
  "max_pages": 20,
  "max_depth": 2,
  "same_domain": true
}
```
Output:
```json
{
  "documents": [],
  "summary": {}
}
```

### 10.4 `web_replay`
Input:
```json
{
  "trace_id": "tr_..."
}
```
Output:
```json
{
  "replay_report": {}
}
```

### 10.5 `web_diff`
Input:
```json
{
  "trace_a": "tr_...",
  "trace_b": "tr_..."
}
```
Output:
```json
{
  "diff_report": {}
}
```

### 10.6 `web_proof`
Input:
```json
{
  "trace_id": "trace_...",
  "proof_id": "proof_...",
  "chunk_id": "chk_..."
}
```
Output:
```json
{
  "proof": {}
}
```

## 11. Error Model
Formato errore canonico:
```json
{
  "error": {
    "code": "NX_TIMEOUT",
    "message": "Timeout exceeded at stage extract_det",
    "stage": "extract_det",
    "retryable": true,
    "details": {}
  }
}
```

Codici minimi:
- `NX_INVALID_INPUT`
- `NX_FETCH_FAILED`
- `NX_RENDER_FAILED`
- `NX_TIMEOUT`
- `NX_BUDGET_EXCEEDED`
- `NX_POLICY_BLOCKED`
- `NX_MODEL_UNAVAILABLE`
- `NX_REPLAY_NOT_FOUND`

## 12. Sicurezza e Compliance
1. URL allow/deny lists supportate.
2. Strict timeout enforcement per stage.
3. Byte caps hard stop.
4. No side-effect HTTP methods by default (solo GET/HEAD).
5. Redaction opzionale di PII nei log.
6. Storage locale cifrabile opzionale.

## 13. Observability
Metriche minime:
- `run_latency_ms{stage}`
- `lane_escalations_total`
- `token_usage_total`
- `fidelity_score`
- `determinism_score`

Log event schema:
- `ts`, `run_id`, `trace_id`, `stage`, `event`, `reason_code`, `duration_ms`

Trace event types:
- `stage_started`
- `stage_completed`
- `escalation_triggered`
- `budget_warning`
- `error`

## 14. Benchmark e Qualita'
Suite benchmark minima Day 1:
1. News/article
2. Developer docs
3. Forum/thread
4. Ecommerce product pages
5. JS-heavy pages

Metriche:
1. Fidelity@k
2. Compression ratio
3. p50/p95 latency
4. escalation rate
5. replay determinism

Acceptance thresholds iniziali:
- Fidelity@5 >= 0.85
- Determinism >= 0.98
- Compression >= 3x
- p95 standard profile <= 2s

## 15. Test Strategy
1. Unit tests per stage core.
2. Contract tests per CLI/MCP schema.
3. Integration tests end-to-end.
4. Golden tests su dataset statici.
5. Replay regression tests run-to-run.
6. Chaos tests su network failures/timeouts.

## 16. Day 1 Execution Plan (Tutto Subito)
Track A - Core Runtime:
- acquire/reduce/segment/extract_det/chunk_rank/pack

Track B - Verification Plane:
- Web IR + proof + trace + replay + diff

Track C - Intelligence Plane:
- router/judge + extract_slm + formatter + lane policy

Track D - Product Interface:
- CLI complete + MCP complete + config system

Track E - Quality Gate:
- benchmark suite + acceptance thresholds + release checks

Regola di rilascio:
- no track incomplete
- no placeholder endpoint
- no TODO critici nei percorsi runtime

## 17. Repository Layout Proposto
```text
needlex/
  .agent/
    skills/
      lean-full-scope-runtime/
  cmd/needle/
  docs/
    architecture.md
    folder-tree.md
    development-plan.md
  internal/
    config/
    core/
    intel/
    pipeline/
    proof/
    store/
    transport/
  schemas/
    proof.schema.json
    resultpack.schema.json
    mcp/
  scripts/
    check_budget.sh
  testdata/
    benchmark/
    golden/
  README.md
  idea.md
  spec.md
```

Regole vincolanti:
1. I package `internal/` devono restare <= 10.
2. `pipeline/` ospita gli stage deterministici; non si spezza in package separati finche' non esistono due motivazioni forti: complessita' ciclomatica e ownership distinta.
3. `intel/` centralizza router, judge, formatter e adapter modello.
4. `transport/` contiene CLI e MCP come adapter sottili che chiamano lo stesso core service.
5. Directory vuote non vanno create solo per "prenotare" spazio architetturale; si creano quando esiste codice reale da ospitare.

## 18. Config Spec (esempio)
Prima implementazione: JSON-first per rimanere stdlib-only e non introdurre dipendenze di parsing premature.

```json
{
  "runtime": {
    "max_pages": 20,
    "max_depth": 2,
    "timeout_ms": 2000,
    "max_bytes": 4000000,
    "lane_max": 3
  },
  "policy": {
    "threshold_conflict": 0.42,
    "threshold_ambiguity": 0.37,
    "threshold_coverage": 0.15,
    "threshold_confidence": 0.78
  },
  "budget": {
    "max_tokens": 8000,
    "max_latency_ms": 1800
  },
  "models": {
    "router": "local-slm-router",
    "judge": "local-slm-judge",
    "extractor": "local-slm-extractor",
    "formatter": "local-slm-formatter"
  }
}
```

## 19. Decisioni Architetturali Vincolanti
1. Tutti gli output user-facing passano da schema validation.
2. Tutte le decisioni non deterministiche devono avere reason code.
3. Ogni run deve produrre trace+proof per debug/audit.
4. Ogni escalation lane deve essere spiegabile e budget-aware.

## 20. Minimal Code Charter (Vincolante)
Obiettivo: mantenere il runtime completo Day 1, ma con basecode minima, leggibile e difficile da rompere.

Budget hard:
1. Production LOC target: <= 8k (esclusi test e fixture).
2. Numero package `internal/` target: <= 10.
3. Dipendenze third-party runtime target: <= 8.
4. Nessun file oltre 400 LOC (salvo eccezioni motivate).

Regole di design:
1. Un solo binary (`cmd/needle`) e un solo processo runtime.
2. Nessun microservizio interno, nessun orchestratore.
3. Nessuna interfaccia se non esistono almeno 2 implementazioni reali.
4. Niente reflection o metaprogramming se evitabile.
5. Config-driven behavior prima di nuova logica hardcoded.
6. Reuse massimo di primitive comuni attive (`Budget`, `ProofEvent`, packet/trace contracts).

Regole di implementazione:
1. Ogni nuova feature deve riusare la pipeline esistente, non creare pipeline parallela.
2. Router/Judge/Formatter devono condividere lo stesso adapter modello.
3. Proof e Trace devono essere emessi da un unico event bus interno.
4. CLI e MCP devono chiamare lo stesso core API, zero duplicazione logica.

Regole anti-bloat:
1. Ogni PR deve dichiarare delta LOC e motivazione.
2. Ogni PR che introduce una dependency deve indicare alternativa stdlib e tradeoff.
3. Ogni PR che supera 300 LOC netti richiede split o giustificazione tecnica.
4. Refactor obbligatorio se una feature aumenta complessita' ciclomatica oltre soglia.

Quality guardrails:
1. Determinism tests obbligatori su ogni change al core.
2. Golden tests obbligatori su estrazione e ranking.
3. No merge se `replay` o `proof` regressano.

## 21. Open Decisions
1. Selezione SLM locali per router/judge/extractor/formatter.
2. Scelta libreria HTML parsing principale in Go.
3. Politica storage trace (solo locale vs plugin backend).
4. Formato finale Web IR (JSON vs binary packed).

## 22. Exit Criteria
La versione e' pronta se e solo se:
1. FR-001..FR-040 implementati e verificati.
2. NFR-001..NFR-006 rispettati in benchmark.
3. CLI e MCP complete senza endpoint stub.
4. Replay/diff/proof funzionanti su suite golden.
5. Documentazione operativa sufficiente per deploy locale.
