# Benchmark Report

This is the shortest honest read of Needle-X today.

Rule:
1. only report what was actually run
2. separate quality metrics from advantage metrics
3. do not market weak or still-uncalibrated signals

## What The Benchmarks Already Support

1. compact agent-facing output
2. proof-carrying context
3. benchmark-backed narrow model use
4. semantic context alignment on multilingual pages
5. strong warm-state local retrieval through `Discovery Memory`

## What They Do Not Support Yet

1. broad market-superiority claims
2. cold-state seedless open-web strength
3. surface-form metrics as proxies for meaning
4. reopening specialist model tasks in the active core

## Live Advantage Metrics

Source run:
1. `Needle-X`
2. `Jina`
3. `Firecrawl`
4. `Tavily`

| Metric | Needle-X | Tavily | Jina | Firecrawl |
| --- | ---: | ---: | ---: | ---: |
| Avg packet bytes | 4436 | 6975 | 30565 | 72166 |
| Claim-to-source steps | 1 | 2 | 2 | 2 |
| Post-processing burden | 0.25 | 1.92 | 1.86 | 2.50 |
| Proof usability | 1.0 | 0 | 0 | 0 |

Extra read:
1. Needle-X is about `85.5%` smaller than the `Jina` baseline on packet size
2. Needle-X reaches the source in half the steps of the others in this live run
3. Needle-X imposes much less cleanup on the next agent in the loop

Interpretation:
1. these are advantage metrics
2. they are strong enough for public storytelling
3. they are not broad quality-superiority claims

## Discovery Memory Benchmark

Latest 100-case warm-state result:
1. `case_count = 100`
2. `warm_runtime_success_rate = 1`
3. `warm_selected_pass_rate = 0.94`
4. `warm_local_provider_rate = 1`
5. `warm_memory_provider_rate = 0.02`
6. `improvement_rate = 0.94`
7. `avg_warm_latency_ms = 455`

Read it correctly:
1. local warm-state retrieval is strong on a broad seeded corpus
2. repeated use materially improves retrieval even when public bootstrap is noisy
3. remaining misses are mostly canonical-home/list-page intent problems
4. this does not prove cold-state open-web seedless performance

## 100-Case Seeded Benchmarks

Latest seeded corpus result:
1. `case_count = 100`
2. `runtime_success_rate = 1`
3. `pass_rate = 0.99`
4. `selected_url_pass_rate = 0.99`
5. `proof_usability_rate = 1`
6. `avg_latency_ms = 528`
7. `avg_packet_bytes = 5360`

Latest unique-source seeded result:
1. `case_count = 100`
2. `unique_expected_domains = 100`
3. `runtime_success_rate = 1`
4. `pass_rate = 1`
5. `selected_url_pass_rate = 1`
6. `proof_usability_rate = 1`
7. `avg_latency_ms = 588`
8. `avg_packet_bytes = 4343`

Read it correctly:
1. the curated seeded corpus is stable at `99/100`
2. the broader 100-domain corpus is currently `100/100`
3. the improvement came from generic acquisition fixes and canonical corpus cleanup, not single-domain ranking rules
4. this is the right direction for product-quality measurement because every case targets a different expected domain

## Semantic-First Validation

Latest local validation run: `2026-05-04`.

Deterministic suites:
1. `discovery_eval`: `8/8` pass
2. `hard_case_matrix`: `6/6` pass
3. `hard_case_matrix.avg_lossiness = 0.098`

Live 100-case seedless no-key provider run:
1. `case_count = 100`
2. `runner_runs = 1`
3. `runner_profiles = ["browser_like_semantic"]`
4. `runner_provider_chains = ["ddg-bing"]`
5. provider chain: `lite.duckduckgo.com`, `html.duckduckgo.com`, `www.bing.com`
6. `profile_pass_rates = {"browser_like_semantic": 0.31}`
7. `profile_runtime_rates = {"browser_like_semantic": 0.94}`
8. `error_kinds = {"ranking_miss": 63, "runtime_error": 5, "benchmark_timeout": 1}`

Interpretation:
1. semantic-first changes did not regress deterministic discovery quality
2. seedless runtime reliability is mostly stable with only no-key public HTML providers
3. open-web seedless failures are mostly ranking/candidate-source failures, not provider blocking
4. the seedless runner now supports explicit profile selection and multiple named provider chains
5. broad cold-state seedless quality is not yet a market claim; the next measured target is semantic/family reranking against mirrors and aggregators

## Quality Interpretation Rule

Keep these axes separate:
1. `runtime_success_rate`
2. `quality_pass_rate`
3. `advantage metrics`

If these are collapsed into one leaderboard, the report becomes misleading.

## Competitive Discipline

Direct references:
1. `Firecrawl`
2. `Tavily`
3. `Exa`
4. `Brave Search API`

Simple baseline:
1. `Jina Reader` / raw-page readers

Adjacent reference:
1. `Vercel Browser Agent`

Important:
1. `Vercel Browser Agent` is not an isomorphic comparator for compact proof-carrying packet quality
2. it is mainly useful on seeded routing and browsing tasks

## Where To Look Next

1. [seeded-benchmark-spec.md](seeded-benchmark-spec.md)
2. [competitive-benchmark-protocol.md](competitive-benchmark-protocol.md)
3. [seeded-benchmark-latest.json](../improvements/seeded-benchmark-latest.json)
4. [competitive-benchmark-latest.json](../improvements/competitive-benchmark-latest.json)
5. [discovery-memory-benchmark-latest.json](../improvements/discovery-memory-benchmark-latest.json)
