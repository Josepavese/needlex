# Benchmark Report

This document tracks the current measurable state of Needle-X against simple baselines.

It is intentionally conservative:
1. only report numbers we actually ran
2. keep the methodology explicit
3. avoid marketing claims that the benchmarks do not support

Operational artifact policy:
1. active benchmark outputs live in `improvements/`
2. only current `baseline` and `latest` style reports should remain in `improvements/` root
3. old waves, provider-specific experiments, and one-off empirical captures belong in `improvements/archive/`
4. paid-provider benchmark reuse cache lives locally in `.needlex/competitive-benchmark-cache.json`

## Scope

Current benchmark coverage focuses on:
1. deterministic `read`
2. query strategy comparison
3. Needle-X versus a naive plain-text baseline
4. Needle-X versus a reduced deterministic baseline
5. optional external deterministic baseline adapter
6. hard-case lane behavior on ambiguity-style pages
7. comparative hard-case matrix between default lane behavior and forced lane `2/3` behavior
8. live semantic context evaluation on multilingual pages

## Semantic Context Regime

Meaning-sensitive evaluation is now semantic-first.

This means:
1. `context_alignment` is the primary signal for context quality in live evaluation
2. lexical overlap is not treated as a primary meaning metric
3. hard-case exports declare their metric regime explicitly

Current semantic gate baseline:
1. backend: `openai-embeddings`
2. model: `intfloat/multilingual-e5-small`

## Current Quality Gates

The repo currently enforces:
1. replay determinism on golden tests
2. fidelity checks on golden tests
3. `tiny` compression ratio `>= 3.0`
4. query improvement through same-site discovery
5. Needle-X signal-quality win over naive and reduced deterministic baselines
6. hard-case matrix checks that elevated lanes improve or preserve useful signal under controlled difficult inputs
7. hard-case acceptance checks enforce final intelligence-readiness thresholds
8. live semantic evaluation on multilingual pages
9. discovery memory cold-vs-warm evaluation
10. structural budget checks

Benchmark interpretation rule:
1. seeded benchmark reports must separate execution reliability from product quality
2. `runtime_success_rate` tracks whether a case completed at all
3. `quality_pass_rate` tracks whether Needle-X passed the product criteria on completed cases
4. competitive comparisons are invalid if these two axes are collapsed into a single pass rate

## Current Quality Conclusions

What the benchmarks support today:
1. Needle-X produces more concentrated context than simpler baselines
2. the active CPU model path is real and benchmark-backed
3. `Gemma 3 1B` is the current CPU baseline
4. semantic context alignment is measurable and works on multilingual pages where lexical overlap is blind
5. the active model task set is intentionally narrow: `resolve_ambiguity` only
6. warm-state `Discovery Memory` can dominate local retrieval on the active benchmark corpus

What the benchmarks do not support yet:
1. a broad market-superiority claim
2. reopening specialist model tasks in the active core
3. equating lexical overlap with context understanding
4. claiming cold-state open-web seedless strength from the discovery-memory benchmark

## Discovery Memory Benchmark

Active artifact:
- [discovery-memory-benchmark-latest.json](/home/jose/hpdev/Libraries/needlex/improvements/discovery-memory-benchmark-latest.json)

Current measured summary:
1. `case_count = 30`
2. `cold_selected_pass_rate = 0`
3. `warm_selected_pass_rate = 1`
4. `warm_memory_provider_rate = 1`
5. `improvement_rate = 1`

Correct interpretation:
1. the benchmark measures a local warm-state retrieval regime
2. once prior evidence is observed, Needle-X can reuse it extremely well on the active corpus
3. this benchmark does not prove that cold-state seedless discovery is solved

Competitive benchmark scope now distinguishes:
1. direct market references:
   - Firecrawl
   - Tavily
   - Exa
   - Brave Search API
2. simple baseline references:
   - raw page / Jina Reader style readers
3. adjacent browser-agent references:
   - Vercel Browser Agent

Competitive reporting must also distinguish:
1. `quality metrics`
2. `advantage metrics`

Recommended advantage metrics:
1. token/character reduction versus baselines
2. hop count to target page or answer
3. tool calls to target
4. time to verifiable claim
5. proof/audit leverage
6. cached reuse savings on paid providers

Default comparative rule:
1. token reduction should default to comparison against `Jina Reader` when that baseline is present in the same report

Rule:
1. Vercel Browser Agent is not treated as a fully isomorphic competitor to Needle-X
2. it is valid mainly on seeded routing and browsing tasks
3. it is not a fair primary comparator for proof-carrying compact packet quality
4. competitive quality must include `fact_coverage_rate` over `must_contain_facts`
5. otherwise raw-text baselines can appear artificially strong
6. in this repo, Vercel Browser Agent enters the benchmark through a bridge endpoint contract, not a single official public comparator API
7. advantage metrics are allowed for product storytelling, but they cannot replace quality metrics

Next benchmark protocol reference:
1. [seeded-benchmark-spec.md](/home/jose/hpdev/Libraries/needlex/docs/seeded-benchmark-spec.md)
2. [seeded-corpus-v1.json](/home/jose/hpdev/Libraries/needlex/benchmarks/corpora/seeded-corpus-v1.json)
3. active seeded benchmark artifact:
   [seeded-benchmark-latest.json](/home/jose/hpdev/Libraries/needlex/improvements/seeded-benchmark-latest.json)
4. active discovery memory benchmark artifact:
   [discovery-memory-benchmark-latest.json](/home/jose/hpdev/Libraries/needlex/improvements/discovery-memory-benchmark-latest.json)
5. competitive benchmark protocol:
   [competitive-benchmark-protocol.md](/home/jose/hpdev/Libraries/needlex/docs/competitive-benchmark-protocol.md)
6. competitive corpus:
   [competitive-corpus-v1.json](/home/jose/hpdev/Libraries/needlex/benchmarks/corpora/competitive-corpus-v1.json)
