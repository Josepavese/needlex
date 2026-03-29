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
6. hard-case lane behavior on embedded-state and troubleshooting-style pages
7. comparative hard-case matrix between default lane behavior and forced lane `2/3` behavior

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
3. it is currently exercised through a local repo-scoped `.venv` and measured in the benchmark suite

## Test Corpus

Current fixtures:
1. `testdata/golden/article.html`
2. `testdata/golden/forum.html`

Current dynamic query benchmark fixture:
1. seed page with docs and blog candidates
2. docs candidate that is more relevant than the seed page

Current hard-case matrix corpus:
1. `testdata/benchmark/hard-case-corpus-v2.json`
2. exported via `./scripts/run_hard_case_matrix.sh`
3. written to:
   - `improvements/hard-case-matrix-latest.json`
   - `improvements/hard-case-matrix-baseline.json`
4. now includes explicit `objective_terms` so abstract goals are measured against grounded domain terms
5. each case now carries a `family` label and each export includes `family_summary` aggregates
6. the corpus now also defines `family_thresholds`, so benchmark export can fail on aggregate family risk, not only on per-case regressions
7. the corpus now defines final `acceptance` thresholds (pass-rate, lane-lift-rate, objective-lift average, risk ceiling) and a failure-class map linked to future SLM rollout gates

## Active Quality Gates

The repo currently enforces:
1. replay determinism on golden tests
2. fidelity checks on golden tests
3. `tiny` compression ratio `>= 3.0`
4. query improvement through same-site discovery
5. Needle-X signal-density win over a naive plain-text baseline
6. Needle-X signal-density win over a reduced deterministic baseline
7. hard-case matrix checks that elevated lanes improve or preserve useful signal under controlled difficult inputs
8. hard-case acceptance checks enforce final intelligence-readiness thresholds and classify failures by integration risk class

Hard-case matrix export command:

```bash
./scripts/run_hard_case_matrix.sh --update-baseline
```

Blocking intelligence gate command:

```bash
./scripts/run_intelligence_gate.sh
```

## Latest Measured Results

### 1. Read Baseline

Command used:

```bash
go test ./internal/core/service -run '^$' -bench 'BenchmarkReadGoldenArticle$' -benchmem -count=3
```

Observed:
1. `337089 ns/op`
2. `427261 ns/op`
3. `326834 ns/op`
4. memory around `84656-84837 B/op`
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
1. `23100 ns/op`
2. `19190 ns/op`
3. `21231 ns/op`
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
1. `32035 ns/op`
2. `32426 ns/op`
3. `32031 ns/op`
4. memory `21530-21531 B/op`
5. `263 allocs/op`

Reading:
1. this baseline is still much cheaper than Needle-X
2. it is a more serious comparison than naive plain-text extraction
3. Needle-X still wins on objective signal density and compression under the current golden article test

### 5. Needle-X Versus External `trafilatura` Baseline

Command used:

```bash
export NEEDLEX_EXTERNAL_BASELINE_CMD=".venv/bin/python scripts/external_baselines/trafilatura_stdin.py"
go test ./internal/core/service -run '^$' -bench 'Benchmark(ReadGoldenArticle|NaiveBaselineGoldenArticle|ReducedBaselineGoldenArticle|ExternalBaselineGoldenArticle)$' -benchmem -count=3
```

Observed external baseline:
1. `396708940 ns/op`
2. `393493016 ns/op`
3. `397309425 ns/op`
4. memory around `65850-65856 B/op`
5. `115 allocs/op`

Reading:
1. this adapter path is far slower than Needle-X in the current local setup because each benchmark iteration spawns a Python process
2. the result is useful for end-to-end reproducibility of the adapter path, not for a fair in-process extractor comparison
3. the next stronger comparison should embed or isolate parser cost separately from process-launch overhead

## Current Quality Conclusions

What the benchmarks support today:
1. Needle-X can produce more concentrated context than a naive baseline
2. Needle-X can produce more concentrated context than a reduced deterministic baseline
3. the optional external baseline adapter path is real and runnable
4. same-site discovery improves query quality in the golden scenario
5. the first bootstrap `web_search` path is implemented, multi-provider capable, and test-covered
6. `tiny` reaches the compression target while remaining traceable
7. the runtime is still deterministic under replay-oriented checks
8. lane behavior on the first hard cases is now test-gated for deterministic lane 0, forced lane 2, and forced lane 3 flows
9. a comparative hard-case matrix now checks that forced lane `2/3` paths either improve objective focus and signal density or preserve tiny-profile compactness under the same input
10. the hard-case benchmark story is now exportable as a versioned JSON artifact, not only as test assertions
11. the current hard-case corpus covers six cases across embedded app-shell, forum remediation, pricing compaction, and tiny capability summaries
12. the report now exposes `lossiness_risk`, making compaction-vs-fidelity tradeoffs visible instead of implicit
13. aggregate family thresholds are now enforced for `embedded`, `forum`, `tiny`, and `compaction`
14. final intelligence-acceptance thresholds are enforced globally (`pass_rate`, `lane_lift_rate`, `objective_lift_avg`, `medium/high risk rate`)
15. acceptance failures are now classified through an explicit failure-class map tied to SLM rollout blocking policy

What the benchmarks do not support yet:
1. that Needle-X is faster than simple baselines
2. that Needle-X beats established external deterministic readers in a fair like-for-like in-process comparison
3. that Needle-X is ready for web-scale discovery claims
4. that selective SLM activation beats deterministic-only behavior on a broad hard-case corpus

## Current Gaps In The Benchmark Story

The main missing pieces are:
1. a fairer external baseline comparison without process-spawn overhead dominating results
2. more fixture diversity
3. a persistent benchmark script or command wrapper
4. profiling-backed optimization data
5. stronger open-web search benchmarks after the current two-stage bootstrap `web_search`
6. `lossiness_risk` is still heuristic and should later be validated against human-reviewed cases

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
