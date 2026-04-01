# Refactor Plan

This document defines the current code refactor sequence for Needle-X.

Goal:
1. reduce duplication
2. increase reuse
3. remove accidental growth
4. keep only code aligned with the real product moat

## Rules

1. No refactor is valid if it weakens proof, trace, replay, diff, or deterministic behavior.
2. No refactor is valid if it expands bootstrap scaffolding faster than native substrate.
3. Shared code is preferred only when it reduces real duplication, not when it hides different behaviors behind vague abstractions.
4. Future-oriented code is allowed only when it is already part of the active architecture.

## Work Order

### 1. State Plane Consolidation

Status: `[x]`

Target:
1. unify repeated read/query/crawl state observation
2. unify genome and fingerprint loading helpers
3. keep transport free of state logic

Acceptance:
1. same behavior as before
2. fewer duplicated store interactions

### 2. Discovery Substrate Consolidation

Status: `[x]`

Target:
1. unify candidate trimming and same-site discovery flow
2. reduce duplicate request normalization and response shaping
3. preserve local-first discovery priority

Acceptance:
1. `Discover` and `DiscoverWeb` still pass current tests
2. candidate scoring logic stays in one substrate

### 3. Pack Engine Consolidation

Status: `[~]`

Target:
1. reduce orchestration bloat in `pack.go`
2. turn ranking/cleanup/intel/proof assembly into clearer passes

### 4. Validation And Render Consolidation

Status: `[~]`

Target:
1. reduce repeated validation boilerplate in `core` and `proof`
2. reduce repeated rendering boilerplate in `transport`

### 5. Strategic Prune

Status: `[~]`

Target:
1. remove code that is neither active moat nor benchmark-backed future path
2. keep only holdout intelligence tasks that remain part of the typed-patch roadmap

Current reading:
1. state-plane duplication has been reduced materially
2. discovery flow is more reusable than before
3. pack and validation/render are partially consolidated
4. the remaining problem is not correctness but total code surface
5. the concrete substrate-level consolidation for candidate handling and pack common rules is tracked in `docs/archive/unification-plan.md`
