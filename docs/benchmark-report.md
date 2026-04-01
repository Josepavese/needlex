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
3. lexical metrics as proxies for meaning
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

Active warm-state result:
1. `case_count = 30`
2. `warm_selected_pass_rate = 1`
3. `warm_memory_provider_rate = 1`
4. `improvement_rate = 1`

Read it correctly:
1. local warm-state retrieval is strong
2. repeated use materially improves retrieval
3. this does not prove cold-state open-web seedless performance

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

1. [seeded-benchmark-spec.md](/home/jose/hpdev/Libraries/needlex/docs/seeded-benchmark-spec.md)
2. [competitive-benchmark-protocol.md](/home/jose/hpdev/Libraries/needlex/docs/competitive-benchmark-protocol.md)
3. [seeded-benchmark-latest.json](/home/jose/hpdev/Libraries/needlex/improvements/seeded-benchmark-latest.json)
4. [competitive-benchmark-latest.json](/home/jose/hpdev/Libraries/needlex/improvements/competitive-benchmark-latest.json)
5. [discovery-memory-benchmark-latest.json](/home/jose/hpdev/Libraries/needlex/improvements/discovery-memory-benchmark-latest.json)
