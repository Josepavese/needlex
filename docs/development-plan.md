# Development Plan

## Rule

This is not a versioned rollout plan. All strategic capabilities are required before the first public release.

This document now serves a stricter purpose:
it defines the remaining execution order after the runtime foundation became real.
From this point on, the goal is not breadth.
The goal is to complete the moat.

## Workstreams

### 1. Core Contract

Deliver:
1. Canonical types for `Document`, `Chunk`, `Proof`, `ResultPack`
2. `RunContext` and `CostReport`
3. Config loading and budget thresholds

Acceptance:
1. Schemas validate
2. Core types are transport-neutral

### 2. Deterministic Pipeline

Deliver:
1. Acquire
2. Reduce
3. Segment
4. Extract deterministic
5. Chunk, rank, and pack

Acceptance:
1. Golden tests pass on article, docs, forum, ecommerce
2. Baseline compression target is met

### 3. Proof And Replay Plane

Deliver:
1. Proof artifact generation
2. Trace event stream
3. Replay
4. Diff

Acceptance:
1. Replay determinism >= 0.98
2. Diff reports stage and chunk deltas clearly

### 4. Intelligence Plane

Deliver:
1. Router
2. Judge
3. Formatter
4. Ambiguity policy and escalation thresholds

Acceptance:
1. Every model call is reason-coded
2. SLM usage is measurable and budget-aware

### 5. Local State Plane

Deliver:
1. Fingerprint cache
2. Domain genome store
3. Local run and trace persistence

Acceptance:
1. State is local-first
2. Storage can be pruned without corrupting source docs

### 6. Transport Surface

Deliver:
1. CLI parity
2. MCP parity
3. Shared invocation path through `core`

Acceptance:
1. No transport-specific business logic
2. CLI and MCP expose the same core operations

### 7. Quality Gate

Deliver:
1. Benchmark suite
2. Golden datasets
3. Budget checker
4. Release checklist

Acceptance:
1. `bash scripts/check_budget.sh .` passes
2. No placeholder endpoints remain
3. Release gate is fully green

## Current Reading

The earlier foundational workstreams are no longer the main constraint.
The remaining work is concentrated in four strategic gaps:
1. `WebIR` is present but not yet the center of runtime decisions
2. `QueryPlan` exists but is not yet a true retrieval compiler
3. native retrieval substrate exists in early form but not yet as product identity
4. fingerprint graph and measurable SLM advantage are still missing

## Remaining Execution Order

### 1. Web Compiler Core

Deliver:
1. `WebIR` as a first-class, versioned runtime substrate
2. ranking and proof logic that explicitly derive from `WebIR`
3. IR inspection/debug surfaces that explain selection behavior
4. compiler-visible IR evidence

Acceptance:
1. `read/query` outputs contain stable `WebIR`
2. plan and proof can point to IR-derived evidence
3. regressions on IR quality fail automatically

### 2. Retrieval Compiler

Deliver:
1. explicit planning stages
2. reason-code families for seed, graph, fallback, lane, budget, and risk
3. serialized plan artifact suitable for replay/diff
4. clear separation between planning and execution

Acceptance:
1. `QueryPlan` explains why execution happened, not only what happened
2. compiler output is machine-readable and testable
3. query strategy shifts are visible in plan diffs

### 3. Native Retrieval Substrate

Deliver:
1. stronger domain graph policy
2. seedless flow that prioritizes local substrate before provider fallback
3. separate evaluation of discovery quality vs read quality
4. confidence-aware graph expansion and anti-noise gating

Acceptance:
1. meaningful seedless queries succeed without provider bootstrap in benchmark cases
2. provider bootstrap remains fallback, not identity path
3. discovery regressions are testable independently

### 4. Fingerprint Graph

Deliver:
1. cross-run fingerprint relationships
2. graph-guided dedup
3. delta-aware re-read minimization
4. change-aware retrieval primitives

Acceptance:
1. stable content is detected across runs
2. redundant extraction decreases on repeated reads
3. graph evidence is visible in retrieval decisions

Current status:
1. first fingerprint graph data model is implemented
2. `read/query/crawl` now observe per-URL chunk snapshots and append cross-run retained/added/removed deltas
3. `read` now consumes prior fingerprint snapshots and emits stable/novel fingerprint counts in pack trace metadata
4. `tiny` profile now uses graph-aware dedup as a guarded compaction aid without changing standard-profile extraction
5. `query` planning now consumes seed-side fingerprint evidence and emits explicit stable/novel/delta risk reasons
6. query discovery now applies a first graph-aware ranking bias on the seed candidate using stability/novelty evidence
7. graph evidence is not yet consumed by standard-profile read ranking, broader candidate ranking, or re-read minimization, so moat realization here is still partial

### 5. Measured Intelligence Advantage

Deliver:
1. hard ambiguity benchmark suite
2. explicit lane `2/3` quality measurements
3. budget-aware evidence for when SLM use helps

Acceptance:
1. SLM calls show measurable wins on defined hard cases
2. budget and trace surfaces remain intact
3. deterministic-first remains default

Current status:
1. a first ambiguity validation suite now exercises deterministic lane 0, ambiguity-triggered escalation, and forced lane 3 full-stack behavior
2. this gives the roadmap a repeatable gate for lane behavior before broader hard-case benchmarking is built
3. a first hard-case suite now covers embedded-state extraction and troubleshooting-style forum cases with explicit lane/invocation expectations
4. a first comparative hard-case matrix now measures default-vs-forced lane behavior using fidelity, expected-signal density, objective focus, and token compactness on controlled difficult inputs
5. the hard-case matrix is now backed by a versioned corpus and exportable JSON artifacts, making lane-value claims inspectable outside raw test output
6. the current corpus `v2` expands to six cases and uses explicit `objective_terms`, reducing false-zero scores on abstract goals like company profile or capability summary
7. the export now includes per-case `lossiness_risk` and per-family aggregates, making compression tradeoffs visible by benchmark family instead of only per test row
8. family-level thresholds are now encoded in the corpus, so benchmark failure can be triggered by systemic degradation in a benchmark family even when no single case fully collapses
9. final intelligence acceptance thresholds are now encoded in the corpus (`min_pass_rate`, `min_lane_lift_rate`, `min_objective_lift_avg`, `max_medium_or_high_risk_rate`)
10. failure-class mapping tied to future real-SLM rollout risk is now encoded and exported with acceptance output
11. hard-case matrix export now blocks on acceptance failure in addition to per-row regression and family-threshold drift

### 6. Final Quality Consolidation

Deliver:
1. broader real-site validation
2. performance profiling on probe-heavy paths
3. external comparisons framed fairly
4. release-level quality checklist

Acceptance:
1. benchmark, budget, and live validation gates are green
2. no tactical scaffolding is mistaken for the product moat
3. public release claims are evidence-backed

## Immediate Next Build Sequence

The next concrete build order should be:
1. introduce a single real-model adapter boundary
2. wire one real backend behind strict activation policy
3. execute A/B validation versus deterministic baseline using the hard-case acceptance gate
4. preserve full proof/trace accounting of model calls

This is the shortest path from measured readiness to controlled real-SLM integration.
