# Seeded Benchmark Spec

Questo documento definisce il benchmark seeded che Needle-X deve usare prima di qualsiasi claim competitivo serio.

Non e' una nota esplorativa.
E' il protocollo che decide:
1. cosa misuriamo
2. contro chi confrontiamo il prodotto
3. come evitiamo benchmark cosmetici o non comparabili

## Obiettivo

Misurare Needle-X sul suo path forte reale:
1. `read` seeded
2. `query` seeded same-site
3. `query -> proof` end-to-end

Non misuriamo qui:
1. seedless public-web search generico
2. browser automation
3. crawling ampio come prodotto di search

## Regola Madre

Needle-X va confrontato dove il suo contratto attivo e' forte:
1. pagina seedata
2. goal esplicito
3. contesto compatto per AI
4. provenance verificabile

Se il benchmark misura soprattutto discovery web aperta, il benchmark e' fuori fuoco rispetto al prodotto attuale.

## Benchmark Regimes

### Regime A: Internal Product Validation

Scopo:
1. capire se Needle-X funziona bene da solo
2. chiudere gap di output
3. calibrare rubric e dataset

Dimensione:
1. `30-50` casi seeded reali curati manualmente

Uso:
1. hardening pre-release
2. tuning di packet, proof ergonomics, summary quality

### Regime B: Scaled Seeded Evaluation

Scopo:
1. misurare Needle-X su una base piu' ampia
2. stimare stabilita' e failure classes

Dimensione:
1. `100-200` casi seeded reali

Uso:
1. quality gate serio
2. regressioni di prodotto
3. report pre-release

### Regime C: Competitive Seeded Benchmark

Scopo:
1. confrontare Needle-X con uno o due riferimenti di mercato
2. solo dopo che A e B sono stabili

Dimensione:
1. usa il dataset del Regime B
2. subset manualmente auditato da `30-50` casi per scoring profondo

Uso:
1. benchmark story pubblica
2. differenziazione difendibile

## Non-Goals

Questo benchmark non deve:
1. incoronare un “leader del mercato” in astratto
2. usare un competitor scelto per convenienza
3. confondere search bootstrap con context compilation
4. usare metriche solo lessicali
5. usare un dataset enorme prima che esista una rubric seria

## Candidate Competitor Classes

Il confronto competitivo deve avvenire contro classi precise.

### Classe 1: Raw Page Baseline

Forma:
1. HTML o text extraction semplice
2. prompt standard verso un LLM esterno o locale

Serve per misurare:
1. vantaggio di compilazione
2. vantaggio di packet structure

### Classe 2: Deterministic Reader Baseline

Forma:
1. reader strutturale senza proof forte
2. chunking/ranking semplice

Serve per misurare:
1. vantaggio del ranking Needle-X
2. vantaggio del packet AI-first

### Classe 3: Market Reference

Forma:
1. prodotto noto per web reading / context extraction
2. stesso input seeded
3. stessa domanda finale

Regola:
1. massimo `1-2` riferimenti
2. il confronto deve essere replicabile
3. niente competitor scelto solo perche' facile da battere

### Classe 4: Adjacent Browser Agent Reference

Forma:
1. agente browser-first o browser automation-first
2. forte su navigazione, click, form filling, browsing interattivo
3. non perfettamente isomorfo a Needle-X come context compiler

Serve per misurare:
1. se Needle-X perde o vince quando il task richiede routing same-site reale
2. quanto conta avere un browser agent rispetto a un compiler seeded
3. dove il confronto non e' equo e va dichiarato

Regola:
1. questa classe non sostituisce il market reference principale
2. va usata come comparator adiacente
3. non va usata per sminuire o gonfiare Needle-X su task non comparabili

## Recommended Competitive Order

Ordine corretto:
1. Needle-X vs raw page baseline
2. Needle-X vs deterministic reader baseline
3. Needle-X vs one market reference
4. Needle-X vs one adjacent browser agent reference

Non partire dal punto 4.

## Dataset Shape

Ogni caso seeded deve avere questo shape minimo:

```json
{
  "id": "zai-coding-plan",
  "family": "docs",
  "language": "en",
  "seed_url": "https://docs.z.ai/guides/overview/quick-start",
  "task_type": "same_site_query",
  "goal": "coding plan",
  "expected_url": "https://docs.z.ai/devpack/overview",
  "expected_domain": "docs.z.ai",
  "must_contain_facts": [
    "supports coding tools",
    "lists supported models or plans"
  ],
  "must_expose_proof": true,
  "notes": "same-site routing from a docs landing page"
}
```

## Case Families

Il dataset deve coprire almeno:
1. `docs`
2. `corporate`
3. `multilingual`
4. `legacy_homepage`
5. `same_site_query`
6. `list_like_content`

Vincoli:
1. nessuna famiglia oltre il `35%`
2. almeno `20%` non-English o multilingual
3. almeno `20%` same-site query routing

## Task Types

Ogni caso deve appartenere a uno di questi task:
1. `read_page_understanding`
2. `same_site_query_routing`
3. `proof_lookup`
4. `read_then_answer`

Task composti consentiti:
1. `same_site_query_routing + proof_lookup`
2. `read_page_understanding + proof_lookup`

## Primary Metrics

Prima di leggere le metriche di qualita', il report deve distinguere due assi:
1. `runtime_success_rate`
2. `quality_pass_rate`

Interpretazione:
1. `runtime_success_rate` misura se il runner e il sito hanno completato il task
2. `quality_pass_rate` misura la qualita' del prodotto solo sui casi eseguiti

Regola:
1. un confronto competitivo non e' valido se confonde timeout di rete con failure di routing o packet quality
2. i report devono classificare almeno:
   - `network_timeout`
   - `network_tls_error`
   - `network_connect_error`
   - `runtime_error`

### 1. Selected URL Correctness

Per `query` seeded:
1. `pass` se `selected_url == expected_url`
2. `near-pass` se dominio corretto e pagina semanticamente equivalente

### 2. Summary Usefulness

Valutazione umana o rubric-guided:
1. `0`: fuorviante o inutile
2. `1`: parziale ma utile
3. `2`: buona sintesi operativa

### 3. Chunk Utility

Per i primi `N` chunk del packet:
1. `%` di chunk utili alla risposta
2. `%` di chunk rumorosi
3. `%` di chunk ridondanti

### 4. Proof Usability

Misura:
1. il packet espone `proof_ref`
2. `needle proof <proof_ref>` funziona
3. il proof contiene selector e transform chain utili

Scoring:
1. `pass/fail`

### 5. Uncertainty Calibration

Misura:
1. casi buoni con `uncertainty=low`
2. casi fragili/list-based/ambiguous con `medium/high` quando appropriato

Non si misura con parole chiave.
Si misura contro audit umano del caso.

### 6. Token Footprint

Misura:
1. dimensione del compact packet
2. numero chunk
3. lunghezza summary

Scopo:
1. confrontare utilita' per token, non solo correttezza

### 7. Latency

Misura:
1. `latency_ms`
2. opzionalmente p50/p95 per il set

## Secondary Metrics

1. `candidate quality` per query
2. `selection_why usefulness`
3. `proof round-trip time`
4. `compact chunk diversity`

## Scoring Protocol

### Machine-Scorable

Automatizzabili:
1. `selected_url_correctness`
2. `proof_lookup_pass`
3. `latency_ms`
4. `compact_packet_size`
5. `chunk_count`

### Human-Scored

Richiedono audit umano:
1. `summary_usefulness`
2. `chunk_utility`
3. `uncertainty_calibration`
4. `near-pass` su selected URL

## Ground Truth Discipline

Ogni caso deve avere:
1. `expected_url` o `expected_domain`
2. `must_contain_facts`
3. una nota breve su cosa conta come successo

Regola:
1. nessun caso entra nel benchmark senza ground truth esplicito

## Release Benchmarks

### Pre-Release Gate

Prima release pubblica:
1. almeno `30-50` casi seeded
2. report stable
3. almeno un audit manuale completo

### Post-Release Expansion

Dopo release:
1. estensione a `100-200` casi
2. solo dopo introduzione confronto competitivo

### 1000-Case Rule

Non si va a `1000` casi finche':
1. rubric non e' stabile
2. failure classes non sono chiare
3. competitive protocol non e' fissato

Un benchmark grande con rubric debole e' peggio di un benchmark piccolo ma auditabile.

## Competitive Protocol

Per ogni competitor/reference:
1. stesso `seed_url`
2. stesso `goal`
3. stessa richiesta finale
4. stesso criterio di scoring
5. stesso limite operativo ragionevole

Va sempre documentato:
1. versione/tool/date
2. come e' stato eseguito
3. cosa non e' comparabile

### Initial Competitive Set

Il set iniziale raccomandato e':
1. `Firecrawl`
2. `Tavily`
3. `Exa`
4. `Brave Search API`
5. `Jina Reader` o raw-page baseline equivalente
6. `Vercel Browser Agent` come adjacent browser agent reference

Classificazione:
1. `Firecrawl`: market reference orientato a scrape/extract/crawl
2. `Tavily`: market reference orientato a search/extract per agenti
3. `Exa`: market reference orientato a neural/AI search
4. `Brave Search API`: market reference orientato a Web search con indice proprio
5. `Jina Reader`: baseline semplice URL-to-LLM
6. `Vercel Browser Agent`: competitor adiacente, browser-first

### Fairness Rules For Vercel Browser Agent

Il confronto con `Vercel Browser Agent` e' valido soprattutto su:
1. `same_site_query_routing`
2. `find the right page from a seed`
3. docs navigation o browsing guidato

Il confronto con `Vercel Browser Agent` non va trattato come isomorfo su:
1. compact packet token efficiency
2. proof/provenance usability
3. deterministic page compilation

Quindi ogni report deve dichiarare:
1. task confrontati
2. task esclusi
3. perche' il confronto e' adiacente e non perfettamente speculare

### Advantage Metrics

Oltre alle metriche di quality, Needle-X dovrebbe misurare anche metriche di vantaggio competitivo.

Metriche raccomandate:
1. `packet_reduction_vs_baseline`
2. `hop_count_to_target`
3. `tool_calls_to_target`
4. `time_to_verifiable_claim`
5. `proof_usability_rate`

Regola:
1. queste metriche servono a mostrare dove Needle-X fa risparmiare lavoro a un agente
2. non sostituiscono `quality_pass_rate`

## Reporting Format

Il report deve avere:
1. `case_count`
2. `runtime_success_rate`
3. `quality_pass_rate`
4. `family_breakdown`
5. `task_breakdown`
6. `selected_url_pass_rate`
7. `proof_usability_pass_rate`
8. `summary_usefulness_avg`
9. `chunk_utility_avg`
10. `uncertainty_calibration_avg`
11. `avg_latency_ms`
12. `avg_packet_bytes`
13. `residual_failure_classes`

## Failure Classes

Classi iniziali da tracciare:
1. wrong-page selection
2. good-page weak-summary
3. good-page noisy-tail
4. proof not actionable
5. uncertainty under-signaled
6. uncertainty over-signaled
7. list-page under-explained
8. multilingual context weakness

## Acceptance Thresholds

Per il Regime A:
1. `selected_url_pass_rate >= 0.80` sui casi query
2. `proof_usability_pass_rate = 1.00`
3. `summary_usefulness_avg >= 1.4 / 2.0`
4. `chunk_utility_avg >= 0.70`

Per il Regime B:
1. soglie da fissare solo dopo stabilizzazione del Regime A

## Implementation Plan

Ordine corretto:
1. creare corpus `50` casi seeded
2. definire runner e schema report
3. fare audit umano su subset
4. chiudere gap prodotto
5. estendere a `100-200`
6. solo dopo aggiungere competitor

## Initial Corpus

Current seeded corpus path:
1. [seeded-corpus-v1.json](/home/jose/hpdev/Libraries/needlex/benchmarks/corpora/seeded-corpus-v1.json)

Current shape:
1. `30` casi
2. famiglie bilanciate
3. same-site query coverage presente
4. multilingual coverage presente

## Decision Rule

Se il benchmark seeded dice:
1. Needle-X e' forte su `read/query/proof`
2. il packet e' utile
3. la provenance e' davvero usabile

allora il confronto competitivo ha senso.

Se queste cose non sono ancora vere internamente, il confronto col mercato e' prematuro.
