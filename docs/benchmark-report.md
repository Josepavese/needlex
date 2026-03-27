# Benchmark Report

This document tracks the current measurable state of Needle-X against simple baselines.

It is intentionally conservative:
1. only report numbers we actually ran
2. keep the methodology explicit
3. avoid marketing claims that the benchmarks do not support

## Scope

Current benchmark coverage focuses on:
1. deterministic `read`
2. query strategy comparison
3. Needle-X versus a naive plain-text baseline
4. Needle-X versus a reduced deterministic baseline
5. optional external deterministic baseline adapter

This is not yet a full public benchmark suite.

## External Baseline Adapter

The repo now includes an optional external baseline adapter:

- `scripts/external_baselines/trafilatura_stdin.py`

Contract:
1. set `NEEDLEX_EXTERNAL_BASELINE_CMD`
2. the command reads HTML from stdin
3. the command writes extracted text to stdout

Example:

```bash
export NEEDLEX_EXTERNAL_BASELINE_CMD="python3 scripts/external_baselines/trafilatura_stdin.py"
```

Important:
1. this is benchmark-only support
2. it adds no runtime dependency to Needle-X
3. it was not executed in the current environment because `trafilatura` is not installed

## Test Corpus

Current fixtures:
1. `testdata/golden/article.html`
2. `testdata/golden/forum.html`

Current dynamic query benchmark fixture:
1. seed page with docs and blog candidates
2. docs candidate that is more relevant than the seed page

## Active Quality Gates

The repo currently enforces:
1. replay determinism on golden tests
2. fidelity checks on golden tests
3. `tiny` compression ratio `>= 3.0`
4. query improvement through same-site discovery
5. Needle-X signal-density win over a naive plain-text baseline
6. Needle-X signal-density win over a reduced deterministic baseline

## Latest Measured Results

### 1. Read Baseline

Command used:

```bash
go test ./internal/core/service -run '^$' -bench 'BenchmarkReadGoldenArticle$' -benchmem -count=3
```

Observed:
1. `392443 ns/op`
2. `376720 ns/op`
3. `360696 ns/op`
4. memory around `84783-84867 B/op`
5. `606 allocs/op`

Practical reading:
1. stable sub-millisecond local benchmark
2. not optimized for raw speed yet
3. acceptable for current validation phase

### 2. Query Strategy Comparison

Command used:

```bash
go test ./internal/core/service -run '^$' -bench 'BenchmarkQuery(SeedOnly|DiscoverFirst)$' -benchmem -count=3
```

Observed `seed-only`:
1. `234842 ns/op`
2. `192041 ns/op`
3. `186824 ns/op`
4. memory around `43235-43253 B/op`
5. `387 allocs/op`

Observed `discover-first`:
1. `309028 ns/op`
2. `339751 ns/op`
3. `321344 ns/op`
4. memory around `68904-68950 B/op`
5. `605 allocs/op`

Reading:
1. `discover-first` is materially more expensive
2. `discover-first` improves target quality on the golden query scenario
3. this is an explicit quality-vs-cost tradeoff, not a free win

### 3. Needle-X Versus Naive Baseline

Command used:

```bash
go test ./internal/core/service -run '^$' -bench 'Benchmark(ReadGoldenArticle|NaiveBaselineGoldenArticle)$' -benchmem -count=3
```

Observed Needle-X:
1. `392443 ns/op`
2. `376720 ns/op`
3. `360696 ns/op`
4. memory around `84783-84867 B/op`
5. `606 allocs/op`

Observed naive baseline:
1. `23367 ns/op`
2. `22721 ns/op`
3. `24031 ns/op`
4. memory `14144 B/op`
5. `127 allocs/op`

Reading:
1. the naive baseline is much faster
2. the naive baseline is much lighter
3. Needle-X currently wins on output quality and retrieval usefulness, not on raw cost

### 4. Needle-X Versus Reduced Deterministic Baseline

Command used:

```bash
go test ./internal/core/service -run '^$' -bench 'Benchmark(ReadGoldenArticle|NaiveBaselineGoldenArticle|ReducedBaselineGoldenArticle)$' -benchmem -count=3
```

Observed reduced deterministic baseline:
1. `36696 ns/op`
2. `35724 ns/op`
3. `34122 ns/op`
4. memory `21531 B/op`
5. `263 allocs/op`

Reading:
1. this baseline is still much cheaper than Needle-X
2. it is a more serious comparison than naive plain-text extraction
3. Needle-X still wins on objective signal density and compression under the current golden article test

## Current Quality Conclusions

What the benchmarks support today:
1. Needle-X can produce more concentrated context than a naive baseline
2. Needle-X can produce more concentrated context than a reduced deterministic baseline
3. same-site discovery improves query quality in the golden scenario
4. the first bootstrap `web_search` path is implemented and test-covered
5. `tiny` reaches the compression target while remaining traceable
6. the runtime is still deterministic under replay-oriented checks

What the benchmarks do not support yet:
1. that Needle-X is faster than simple baselines
2. that Needle-X beats established external deterministic readers
3. that Needle-X is ready for web-scale discovery claims

## Current Gaps In The Benchmark Story

The main missing pieces are:
1. actually running the optional external deterministic baseline adapter and capturing numbers
2. more fixture diversity
3. a persistent benchmark script or command wrapper
4. profiling-backed optimization data
5. stronger open-web search benchmarks after bootstrap `web_search`

## Recommended Next Benchmarking Steps

1. add a stronger external deterministic baseline
2. add one or two more golden fixture families
3. create a reproducible benchmark runner script
4. track benchmark history over time

## Notes

This report should be updated when:
1. benchmark methodology changes
2. benchmark numbers are rerun and materially different
3. a stronger baseline is added or upgraded
4. `discover_web` becomes real
