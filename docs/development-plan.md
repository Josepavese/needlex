# Development Plan

## Rule

This is not a versioned rollout plan. All strategic capabilities are required before the first public release.

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

## Recommended Execution Order

1. Core Contract
2. Deterministic Pipeline
3. Proof And Replay Plane
4. Intelligence Plane
5. Local State Plane
6. Transport Surface
7. Quality Gate

This is an implementation order, not a release sequence. Release still requires all workstreams complete.
