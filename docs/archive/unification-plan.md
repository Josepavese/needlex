# Unification Plan

This document defines the next consolidation pass for Needle-X.

Goal:
1. reduce duplicated decision centers
2. replace scattered helper logic with shared data structures
3. keep modularity only where behavior is truly different
4. preserve deterministic-first runtime and proof integrity

## Principles

1. Shared structures must model real product concepts, not generic utilities.
2. One competence should have one substrate.
3. Orchestration files should compose passes, not re-implement collection logic.
4. Future-facing code stays only if it is already routed by current architecture.

## Bursts

### 1. Candidate Substrate

Status: `[x]`

Target:
1. unify candidate merge/filter/select/url projection
2. remove discovery/query drift around candidate handling
3. make discovery state a reusable structure instead of loose helper functions

Acceptance:
1. `Discover`, `DiscoverWeb`, and `Query` use the same candidate substrate
2. no separate ad-hoc candidate merge/filter/select helpers remain

### 2. Pack Common Surface

Status: `[x]`

Target:
1. move pack-wide common rules out of the orchestration center
2. keep `pack.go` focused on pass sequencing
3. reduce largest-file pressure without weakening the pack engine

Acceptance:
1. `pack.go` stays orchestration-first
2. profile/objective/reuse common rules live in shared pack support code

### 3. Strategic Follow-up

Status: `[~]`

Target:
1. unify transport text rendering around shared section writers
2. collapse query/discovery request shaping further if LOC pressure continues
3. prune holdout surfaces only if they are not benchmark-backed roadmap items

Reading:
1. this pass is about shared product structures, not cosmetic file shuffling
2. after this pass, the remaining debt should be mostly surface area, not duplicated competence
