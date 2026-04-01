# Real-World Read Validation

Date: `2026-03-28`

This document captures real `needle read` trials executed on live websites.

It exists for two reasons:
1. preserve concrete evidence of current runtime behavior
2. derive the next improvement backlog from real failures and partial wins

## Scope

Sites tested:
1. `https://carratellire.com/`
2. `https://www.cnfsrl.it/`
3. `https://halfpocket.net/`

## Automated Regression Suite

A repeatable evaluator is now available:

```bash
./scripts/run_live_read_eval.sh
```

Outputs:
1. `improvements/live-read-latest.json`
2. regression checks against `improvements/live-read-baseline.json` when present

To refresh the baseline after intentional improvements:

```bash
./scripts/run_live_read_eval.sh --update-baseline
```

Exit behavior:
1. `0` no regression
2. `2` regression detected vs baseline

## Commands Used

### Default timeout

```bash
go run ./cmd/needle read https://carratellire.com/
go run ./cmd/needle read https://www.cnfsrl.it/
go run ./cmd/needle read https://halfpocket.net/
```

### Extended timeout

```bash
NEEDLEX_RUNTIME_TIMEOUT_MS=10000 go run ./cmd/needle read https://carratellire.com/
NEEDLEX_RUNTIME_TIMEOUT_MS=10000 go run ./cmd/needle read https://www.cnfsrl.it/
```

## Results Summary

| Site | Default | Extended Timeout | Outcome | Main Issue |
| --- | --- | --- | --- | --- |
| `carratellire.com` | fetch timeout | fetch ok, segmentation failed | failed | JS-heavy app shell with content embedded in scripts/JSON |
| `www.cnfsrl.it` | fetch timeout | success | partial success | live content extracted, but spam/SEO contamination leaked into chunks |
| `halfpocket.net` | success | not needed | success | minor chunk duplication / hierarchical repetition |

## Detailed Outcomes

### 1. `https://carratellire.com/`

Default run:

```text
read failed: fetch page: Get "https://carratellire.com/": context deadline exceeded
```

Extended-timeout run:

```text
read failed: no segments produced
```

Observed page shape from raw HTML inspection:
1. HTTP `200 OK`
2. app shell mounted into `<app-root></app-root>`
3. heavy inline CSS and scripts
4. large business and listing content embedded in `window._a2s = {...}`
5. useful text is present, but mostly inside JSON/script payloads, not clean semantic content blocks

Assessment:
1. fetch path is recoverable with higher timeout
2. reducer/segmenter cannot currently promote embedded structured payloads into readable content
3. this is a real blocker for JS-heavy commercial sites with server-delivered bootstrap state

### 2. `https://www.cnfsrl.it/`

Default run:

```text
read failed: fetch page: Get "https://www.cnfsrl.it/": context deadline exceeded
```

Extended-timeout run:

```text
Title: Homepage - Cnf
URL: https://www.cnfsrl.it/
Chunks: 6
Profile: standard
Proof Records: 6
Stages: 4
Latency: 4680ms
Trace ID: trace_07f2fa6aafbb5ad0
Trace Path: .needlex/traces/trace_07f2fa6aafbb5ad0.json
Proof Path: .needlex/proofs/trace_07f2fa6aafbb5ad0.json
Fingerprint Path: .needlex/fingerprints/trace_07f2fa6aafbb5ad0.json
```

Positive extraction signals:
1. title and company positioning captured correctly
2. intralogistics messaging extracted
3. product/service sections extracted
4. runtime completed end-to-end and persisted trace/proof/fingerprint

Negative extraction signals:
1. unrelated gambling/spam text leaked into chunks
2. ranking kept contaminated content in the final pack
3. current pruning does not aggressively suppress injected SEO or compromised page sections

Assessment:
1. runtime is already usable on this site class
2. the main gap is quality, not basic reachability
3. this is the clearest current example for anti-spam pruning work

### 3. `https://halfpocket.net/`

Default run:

```text
Title: Half Pocket - Sviluppo siti web, app, hardware e software per il business
URL: https://www.halfpocket.net/
Chunks: 6
Profile: standard
Proof Records: 6
Stages: 4
Latency: 1349ms
Trace ID: trace_0dc65bb06e9c9634
Trace Path: .needlex/traces/trace_0dc65bb06e9c9634.json
Proof Path: .needlex/proofs/trace_0dc65bb06e9c9634.json
Fingerprint Path: .needlex/fingerprints/trace_0dc65bb06e9c9634.json
```

Positive extraction signals:
1. title captured correctly
2. main proposition extracted
3. service areas extracted
4. no timeout tuning required
5. output is coherent and usable as compact business context

Minor weaknesses:
1. repeated hierarchy prefixes like `> GR > Design emozionale`
2. some repeated semantic content across nearby chunks
3. chunk naming can still be cleaner

Assessment:
1. this is a strong positive case for the current runtime
2. it proves the core path already works on real business websites with mostly semantic content

## What These Tests Prove

The runtime already handles three different realities:
1. standard business landing pages: good
2. slower websites with usable semantic HTML: acceptable with timeout tuning
3. app-like pages with structured payloads in scripts: not solved yet

This means the product is no longer hypothetical.
It also means the main gaps are now specific and testable.

## External Comparison Meter

Optional external baseline is supported through:

```bash
export NEEDLEX_EXTERNAL_BASELINE_CMD=".venv/bin/python scripts/external_baselines/trafilatura_stdin.py"
./scripts/run_live_read_eval.sh --out improvements/live-read-latest-with-external.json
```

This compares Needle-X coverage/noise metrics with text extracted by an external reader.
Current default adapter uses `trafilatura` through `scripts/external_baselines/trafilatura_stdin.py`.

## Priority Improvement Backlog

## Implemented Since This Validation

1. Embedded payload fallback in reducer (`2026-03-28`, post-validation)
   - when semantic DOM extraction is sparse, the reducer now scans script payloads
   - it extracts high-signal fields (`title`, `description`, `subtitle`, `name`, `business_name`)
   - it converts HTML fragments inside JSON strings into plain text nodes
   - validated with unit tests and a live re-run on `https://carratellire.com/`

2. Contamination-aware ranking penalty in pack stage (`2026-03-28`, post-validation)
   - applies a score/ confidence penalty to contamination-prone segments for non-gambling objectives
   - adds `contamination_risk` in proof flags when applicable
   - measured effect on live suite:
     - `cnf` noise hits: `4 -> 0`
     - `halfpocket` remained stable with `noise=0` and full keyword coverage

3. Adaptive acquire retry on timeout (`2026-03-28`, post-validation)
   - one bounded retry is now attempted when the first fetch fails with deadline exceeded
   - retry timeout is expanded from the original timeout and capped
   - no retry is performed for non-timeout fetch errors
   - covered by dedicated acquire test and validated on live suite without quality regression

4. WebIR live regression automation (`2026-03-28`, post-realignment)
   - live evaluator now records `web_ir_version`, `web_ir_node_count`, and core IR signals
   - live regression compare now fails on:
     - IR version drift
     - IR node collapse (strong drop vs baseline)
     - IR node count reaching zero
   - external comparison remains supported through `trafilatura`
   - validation commands:
     - `go test ./benchmarks/live_read_eval/runner ./internal/core/service`
     - `./scripts/run_live_read_eval.sh --out improvements/live-read-latest.json`
     - `NEEDLEX_EXTERNAL_BASELINE_CMD=".venv/bin/python scripts/external_baselines/trafilatura_stdin.py" ./scripts/run_live_read_eval.sh --out improvements/live-read-latest-with-external.json`

5. Local candidate memory + query auto-seed (`2026-03-28`, post-realignment)
   - runtime now persists URL candidates from `read`, `query`, and `crawl` into `.needlex/candidates/index.json`
   - `query` without `seed_url` can bootstrap from local candidate memory before external discovery
   - auto-seed is intentionally disabled when `discovery=off`
   - covered by new tests in:
     - `internal/store/candidates_test.go`
     - `internal/transport/cli_test.go`

6. Runtime-native domain hint reranking (`2026-03-28`, post-realignment)
   - query now carries `domain_hints` into `same_site_links` and `web_search` discovery scoring
   - discovery scoring adds explicit `domain_hint_match` reason when candidate host matches local hints
   - runtime populates hints from candidate memory and current seed host
   - covered by tests in:
     - `internal/core/service/discover_test.go`
     - `internal/core/service/query_test.go`
     - `internal/transport/cli_test.go`

7. Native domain graph bootstrap (`2026-03-28`, post-realignment)
   - runtime now stores domain-to-domain transitions in `.needlex/domain_graph/index.json`
   - query expands local `domain_hints` via graph edges before discovery execution
   - graph edges are observed from query candidate exploration (`seed -> candidate`, `selected -> candidate`)
   - covered by tests in:
     - `internal/store/domain_graph_test.go`
     - `internal/transport/cli_test.go`

8. Architecture + noise guardrails (`2026-03-28`, post-realignment)
   - graph-based domain expansion now applies a minimum score gate to reduce one-off noisy expansions
   - transport adapter guard tests now fail package-wide if transport entry `.go` files directly reintroduce candidate/domain-graph/genome state orchestration
   - additional guardrails enforce scoped `internal/store` imports and forbid state-logic symbol definitions in transport files
   - covered by tests in:
      - `internal/core/service/query_state_test.go`
      - `internal/transport/architecture_guard_test.go`

9. WebIR-backed ranking substrate (`2026-03-28`, post-realignment)
   - `pack` ranking now consumes explicit `WebIR` evidence derived from node paths
   - ranking gets additional signal from IR kind coherence, embedded provenance, and shallow-node structural support
   - query compiler now records observed `WebIR` node/embedded counts in plan decisions
   - covered by tests in:
     - `internal/core/service/pack_test.go`
     - `internal/core/service/query_test.go`

10. WebIR-backed planning evidence (`2026-03-28`, post-realignment)
   - web candidate probe now materializes `WebIR` metadata during discovery
   - probed candidate metadata contributes to reranking and is carried into compiler decisions
   - query plans now record `WebIR` evidence for selected discovery candidates before final page read
   - covered by tests in:
     - `internal/core/service/discover_test.go`
     - `internal/core/service/query_test.go`

11. Query compiler reason-family expansion (`2026-03-28`, post-realignment)
   - query plans now emit explicit reason codes for provider fallback, graph/domain evidence, and basic selection risk gates
   - selected candidate decisions now retain score metadata, making plan diffs more informative
   - this moves `QueryPlan` closer to a real planner artifact instead of a thin execution log
   - covered by tests in:
     - `internal/core/service/query_test.go`

12. First fingerprint graph substrate (`2026-03-28`, post-realignment)
   - local store now persists per-URL latest chunk-fingerprint snapshots and cross-run delta history
   - `read/query/crawl` observers now update retained/added/removed chunk-fingerprint relationships automatically
   - this creates the first deterministic base for future delta-aware retrieval and graph-guided dedup
   - covered by tests in:
     - `internal/store/fingerprint_graph_test.go`
     - `internal/core/service/read_crawl_state_test.go`
     - `internal/core/service/query_state_test.go`

13. First fingerprint-graph lookup path (`2026-03-28`, post-realignment)
   - `read` now loads previous fingerprint snapshots from local state and exposes stable-vs-novel chunk counts in pack trace metadata
   - this is the first runtime consumer of the fingerprint graph, making change-awareness visible before any ranking or dedup policy uses it
   - covered by tests in:
     - `internal/core/service/read_crawl_state_test.go`
     - `internal/core/service/service_test.go`

14. Guarded graph-aware dedup for `tiny` profile (`2026-03-28`, post-realignment)
   - near-duplicate compaction now prefers novel chunks over stable ones, but only in `tiny` profile
   - this keeps aggressive minimal-token packing aligned with change-awareness without degrading standard-profile fidelity on real sites
   - validated against live sites after an initial regression on `halfpocket`, which was fixed by scoping the policy to `tiny`
   - covered by tests in:
     - `internal/core/service/pack_cleanup_test.go`

15. First ambiguity validation suite (`2026-03-28`, post-realignment)
   - added a repeatable lane-behavior suite covering:
     - deterministic lane 0 docs flow
     - ambiguity-triggered escalation
     - forced lane 3 full-stack execution
   - this creates a concrete gate for future SLM advantage work without adding production runtime weight
   - covered by tests in:
     - `internal/core/service/ambiguity_suite_test.go`

16. First hard-case benchmark suite (`2026-03-29`, post-realignment)
   - added explicit hard cases for:
     - embedded app-shell extraction under forced lane 3
     - troubleshooting/forum replay investigation under forced lane 2
     - embedded-state extraction without forced escalation
   - the suite adds concrete expectations on lane ceiling, model invocation count, and extracted text
   - benchmark entrypoints were added for the lane 2 and lane 3 hard cases
   - covered by tests in:
     - `internal/core/service/hard_case_suite_test.go`

17. Hard-case comparative matrix (`2026-03-29`, post-realignment)
   - added a comparative matrix between default lane behavior and forced lane `2/3` behavior on controlled hard inputs
   - the matrix intentionally measures:
     - retained anchor fidelity
     - expected-signal density
     - objective-focus density
     - token compactness for `tiny`
   - this closes an important prerequisite before real SLM work because it creates an A/B gate for hard-case value claims without changing runtime code
   - covered by tests in:
     - `internal/core/service/hard_case_matrix_test.go`

18. Versioned hard-case corpus + exportable score table (`2026-03-29`, post-realignment)
   - added `benchmarks/corpora/hard-case-corpus-v1.json` as the first versioned hard-case corpus
   - added benchmark-only export infrastructure under `benchmarks/hard_case_matrix/runner`
   - export is intentionally implemented as test-only infrastructure to avoid increasing production LOC
   - `./scripts/run_hard_case_matrix.sh --update-baseline` now produces:
     - `improvements/hard-case-matrix-latest.json`
     - `improvements/hard-case-matrix-baseline.json`
   - the exported table includes:
     - baseline vs compare lane metrics
     - packed text
     - pass/fail reasons
     - regression checks against the prior baseline

19. Hard-case corpus v2 + grounded objective scoring (`2026-03-29`, post-realignment)
   - added `benchmarks/corpora/hard-case-corpus-v2.json`
   - expanded the hard-case matrix from 3 to 6 benchmark cases
   - introduced explicit `objective_terms` so abstract intents like `company profile` or `capability summary` no longer collapse to zero-value objective scores
   - validated and frozen via:
     - `go test ./benchmarks/hard_case_matrix/runner`
     - `./scripts/run_hard_case_matrix.sh --update-baseline`

20. Lossiness-risk axis + family aggregation (`2026-03-29`, post-realignment)
   - each hard-case corpus entry now declares a `family` such as `embedded`, `forum`, `tiny`, or `compaction`
   - exported matrix rows now include:
     - `lossiness_risk`
     - `lossiness_level`
   - exported reports now also include `family_summary` with average signal/objective improvements and average/max lossiness per family
   - current reading from the fresh baseline:
     - `forum` has the highest average lossiness risk because it benefits from aggressive focus compaction
     - `tiny` currently stays at zero measured lossiness on the curated cases
     - `compaction` currently improves objective focus while staying low-risk

21. Family-threshold guardrails (`2026-03-29`, post-realignment)
   - the hard-case corpus now contains `family_thresholds`
   - benchmark export now fails if a whole family exceeds its allowed average or max lossiness risk
   - current thresholds were set from the observed stable baseline:
     - `embedded`: strict low-lossiness envelope
     - `tiny`: near-zero-lossiness envelope
     - `forum`: looser envelope because this family intentionally trades coverage for sharper focus
     - `compaction`: moderate envelope

22. Fingerprint graph enters query planning (`2026-03-29`, post-realignment)
   - `PrepareQueryRequestWithLocalState` now loads seed-side fingerprint evidence from local state
   - `QueryCompiler` now emits explicit reason codes when local fingerprint history shows:
     - stable region bias
     - novelty bias
     - delta risk
   - this is the first step where fingerprint memory is no longer only stored or shown in traces, but starts shaping planner reasoning directly
   - validated by:
     - `internal/core/service/query_test.go`
     - `internal/core/service/query_state_test.go`

23. First graph-aware ranking bias in query discovery (`2026-03-29`, post-realignment)
   - query discovery now uses seed-side fingerprint evidence not only for planner reasons, but also for candidate reranking
   - current policy is intentionally narrow:
     - stable unchanged seed gets a small penalty
     - novel/changed seed gets a small bias
   - this is the first runtime behavior where fingerprint memory changes candidate ordering, not just annotations
   - validated by:
     - `internal/core/service/query_test.go`

### P0

1. Embedded structured payload extraction
   - detect high-signal JSON/script payloads like `window.__STATE__`, `window._a2s`, `__NEXT_DATA__`
   - safely extract business descriptions, listings, titles, and article content from embedded state
   - fallback only when semantic HTML segmentation is weak or empty

2. Contamination and spam pruning
   - suppress chunks with gambling/adult/pharma/SEO-spam signatures when they conflict with site topic
   - add domain-topic coherence checks
   - down-rank isolated off-topic sections even if they contain dense text

3. Better timeout strategy
   - keep default timeout lean
   - add adaptive retry policy for slow-but-valid pages
   - avoid forcing the user to manually raise `NEEDLEX_RUNTIME_TIMEOUT_MS` for common cases

### P1

1. Chunk deduplication
   - remove near-duplicate chunks after packing
   - reduce repeated hierarchy prefixes and repeated body text

2. Better section-title shaping
   - normalize breadcrumb-like prefixes
   - shorten heading chains when they do not add retrieval value

3. Failure-mode classification
   - distinguish:
     - `fetch_timeout`
     - `empty_semantic_dom`
     - `script_embedded_content_detected`
     - `contaminated_page_detected`
   - this will make debugging and roadmap prioritization cleaner

### P2

1. Domain genome upgrades from live tests
   - remember slow domains
   - remember contamination-prone domains
   - remember script-heavy domains that require embedded payload extraction

2. Targeted profile selection
   - use more conservative packing when contamination risk is high
   - bias toward deeper inspection when semantic DOM is sparse

## Recommended Engineering Sequence

The next implementation order should be:
1. add embedded payload detection and extraction
2. add contamination pruning and off-topic down-ranking
3. add adaptive retry/timeout behavior
4. add post-pack dedup and heading normalization
5. feed these outcomes into the domain genome

## Product Reading

If judged only on these three live reads:
1. one site is already good
2. one site is usable but noisy
3. one site exposes a real capability gap

This is a useful distribution.
It shows the runtime is already real, but not yet robust enough for broad unattended market usage.
