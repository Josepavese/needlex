# Competitive Benchmark Protocol

Questo documento definisce come eseguire il primo confronto competitivo seeded di Needle-X.

Non ridefinisce il seeded benchmark generale.
Specifica:
1. competitor iniziali
2. task comparabili
3. fairness rules
4. corpus competitivo iniziale
5. configurazione operativa del runner

## Competitor Set

### Product Under Test
1. `Needle-X`

### Direct Market References
1. `Firecrawl`
2. `Tavily`
3. `Exa`
4. `Brave Search API`

### Simple Baseline
1. `Jina Reader`
2. raw-page baseline equivalente, se necessario

### Adjacent Reference
1. `Vercel Browser Agent`

## Comparator Categories

### Direct
Usare per:
1. seeded `read`
2. seeded `query` quando supportata
3. compact context extraction

Categoria:
1. `Firecrawl`
2. `Tavily`
3. `Exa`
4. `Brave Search API` on search-first tasks

### Simple Baseline
Usare per:
1. misurare il vantaggio Needle-X su un reader minimale
2. capire il valore del packet compatto e della provenance

Categoria:
1. `Jina Reader`

### Adjacent Browser Agent
Usare per:
1. same-site routing
2. docs navigation
3. browser-guided finding of the right page from a seed

Categoria:
1. `Vercel Browser Agent`

Hard rule:
1. non trattare `Vercel Browser Agent` come competitor isomorfo su proof, compact packet efficiency o deterministic compilation

## Task Matrix

### same_site_query_routing
Needle-X:
1. yes

Firecrawl:
1. adapter target

Tavily:
1. adapter target

Exa:
1. adapter target

Brave Search API:
1. adapter target

Jina Reader:
1. no

Vercel Browser Agent:
1. yes

### read_page_understanding
Needle-X:
1. yes

Firecrawl:
1. yes

Tavily:
1. yes

Exa:
1. yes

Brave Search API:
1. no

Jina Reader:
1. yes

Vercel Browser Agent:
1. low priority

### read_then_answer
Needle-X:
1. yes

Firecrawl:
1. yes

Tavily:
1. yes

Exa:
1. yes

Brave Search API:
1. no

Jina Reader:
1. yes, weak baseline

Vercel Browser Agent:
1. low priority unless browsing is required

## Initial Corpus

Path:
1. [competitive-corpus-v1.json](/home/jose/hpdev/Libraries/needlex/benchmarks/corpora/competitive-corpus-v1.json)

Shape:
1. `12` casi
2. include same-site query hard cases
3. include docs, corporate e multilingual
4. e' abbastanza piccolo per audit manuale

## Metrics

Per tutti i competitor:
1. `runtime_success_rate`
2. `quality_pass_rate`
3. `selected_url_pass_rate`
4. `proof_usability_rate` quando applicabile
5. `fact_coverage_rate`
6. `avg_latency_ms`
7. `avg_packet_bytes`

## Advantage Metrics

Queste metriche non sostituiscono la quality evaluation.
Servono a raccontare dove Needle-X e' forte in modo spendibile e difendibile.

### Token Efficiency
1. `avg_packet_bytes`
2. `packet_reduction_vs_baseline`
3. `chars_per_successful_case`

Baseline rule:
1. the default baseline for `packet_reduction_vs_baseline` is `Jina Reader` when present in the same report

Interpretazione:
1. Needle-X deve mostrare vantaggio quando restituisce meno caratteri/token utili per arrivare alla stessa informazione
2. questa e' una metrica di costo operativo per agenti, non solo di stile output

### Navigation Efficiency
1. `hop_count_to_target`
2. `tool_calls_to_target`
3. `time_to_target_ms`

Interpretazione:
1. Needle-X deve essere misurato anche su quanti passaggi servono per arrivare alla pagina o informazione giusta
2. questa metrica e' particolarmente importante contro browser agents e search-first tools
3. `hop_count_to_target` deve essere calcolato solo sui casi arrivati davvero al target

### Proof And Audit Advantage
1. `proof_usability_rate`
2. `time_to_verifiable_claim`
3. `claim_to_source_steps`

Interpretazione:
1. Needle-X e' forte se un agente arriva piu' in fretta a una claim verificabile
2. qui il confronto non e' isomorfo con tutti i competitor, ma il vantaggio va comunque mostrato

### Reuse And Cost Advantage
1. `cached_reuse_rate`
2. `cached_reuse_count`

Interpretazione:
1. queste metriche mostrano quante API calls esterne sono state evitate nei rerun
2. sono particolarmente importanti per provider a pagamento o rate-limited

### Marketing Rule
1. `quality metrics` e `advantage metrics` devono stare nello stesso report ma in sezioni separate
2. non usare `advantage metrics` per mascherare fallimenti reali di quality
3. usare `advantage metrics` per mostrare costo, ergonomia e verification leverage

Fairness rule:
1. `proof_usability_rate` resta una metrica separata
2. l'assenza di `proof_ref` non deve azzerare `quality_pass_rate` per comparator non-isomorfi
3. `Needle-X` resta confrontato anche sulla provenance, ma `Jina`, `Firecrawl`, `Tavily` e `Vercel Browser Agent` non vengono penalizzati come se dovessero esporre lo stesso contratto di proof
4. `quality_pass_rate` include anche `fact coverage` sui `must_contain_facts`, per evitare che testo generico o troppo lungo sembri competitivo senza coprire i fatti attesi
5. `Vercel Browser Agent` e' integrato tramite un bridge endpoint deployato su Vercel; non esiste qui un endpoint pubblico ufficiale equivalente a Firecrawl/Tavily/Brave

Interpretazione:
1. `Needle-X` deve essere forte soprattutto su provenance e packet quality
2. `Jina Reader` serve da baseline minima
3. `Vercel Browser Agent` serve soprattutto per task di routing/navigation

## Runner

Path:
1. `benchmarks/competitive/runner/main.go`

Default competitors:
1. `needlex`
2. `jina`
3. `firecrawl`
4. `tavily`
5. `exa`
6. `brave-search`
7. `vercel-browser-agent`

Default output:
1. `improvements/competitive-benchmark-latest.json`

Default local cache:
1. `.needlex/competitive-benchmark-cache.json`
2. external provider results are reused automatically when the same competitor and case are rerun with the same corpus version and inputs
3. this cache exists to reduce paid API calls and rate-limited calls

Example:
```bash
go run ./benchmarks/competitive/runner \
  --cases benchmarks/corpora/competitive-corpus-v1.json \
  --out improvements/competitive-benchmark-latest.json
```

## Environment

Current adapter gating:
1. `Needle-X`: no extra env
2. `Jina Reader`: no extra env
3. `Firecrawl`: `FIRECRAWL_API_KEY`
4. `Tavily`: `TAVILY_API_KEY`
5. `Exa`: `EXA_API_KEY`
6. `Brave Search API`: `BRAVE_SEARCH_API_KEY`
7. `Vercel Browser Agent`: `VERCEL_BROWSER_AGENT_ENDPOINT`
8. `Vercel Browser Agent` optional auth: `VERCEL_BROWSER_AGENT_TOKEN`

Current implementation state:
1. `Needle-X`: implemented
2. `Jina Reader`: implemented for `read`-style tasks only
3. `Firecrawl`: implemented, env-gated
4. `Tavily`: implemented, env-gated
5. `Exa`: implemented, env-gated
6. `Brave Search API`: implemented, env-gated for routing tasks
7. `Vercel Browser Agent`: implemented via env-gated bridge endpoint

Expected bridge request shape:
1. `POST` JSON with:
   - `id`
   - `family`
   - `language`
   - `seed_url`
   - `task_type`
   - `goal`
   - `expected_domain`
   - `must_contain_facts`

Expected bridge response shape:
1. `url` or `selected_url`
2. `summary`
3. optional `text`
4. optional `chunks[].text`
5. optional `latency_ms`

Reference:
1. [vercel-browser-agent-bridge.md](/home/jose/hpdev/Libraries/needlex/docs/vercel-browser-agent-bridge.md)

## Decision Rule

Questo runner e' pronto a:
1. benchmarkare subito Needle-X
2. benchmarkare subito Jina Reader sui task `read`
3. accogliere adapter veri per Firecrawl, Tavily, Exa, Brave Search API e Vercel Browser Agent senza cambiare il protocollo

Questa e' la soglia corretta prima dell'integrazione completa dei competitor.
