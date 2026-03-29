# Folder Tree

## Principle

The tree is designed before code, but directories are created only when real code exists. No empty folders are committed to "reserve" architecture.

Currently materialized:
1. `cmd/needle`
2. `internal/config`
3. `internal/core`
4. `internal/intel`
5. `internal/pipeline`
6. `internal/proof`
7. `internal/store`
8. `internal/transport`
9. `schemas`
10. `scripts`
11. `testdata/golden`
12. `scripts/external_baselines`
13. `improvements`
14. `scripts/live_read_eval`
15. `scripts/hard_case_matrix`
16. `testdata/benchmark`

## Planned Tree

```text
needlex/
  .agent/
    skills/
      lean-full-scope-runtime/
  cmd/
    needle/
  docs/
    architecture.md
    benchmark-report.md
    development-plan.md
    folder-tree.md
    project-context.md
  improvements/
    live-read-baseline.json
    live-read-latest.json
    read-validation-2026-03-28.md
  internal/
    config/
    core/
    intel/
    pipeline/
    proof/
    store/
    transport/
  schemas/
    mcp/
    proof.schema.json
    resultpack.schema.json
  scripts/
    check_budget.sh
    external_baselines/
    hard_case_matrix/
      doc.go
    live_read_eval/
      main.go
    run_hard_case_matrix.sh
    run_live_read_eval.sh
  testdata/
    benchmark/
    golden/
  README.md
  idea.md
  spec.md
```

## Package Responsibilities

`cmd/needle`
Build entrypoint only. No business logic.

`internal/config`
Config loading and runtime policy values.

`internal/core`
Canonical types, run context, service orchestration, and transport-neutral API.

`internal/core/service`
End-to-end orchestration for deterministic read flow. It composes config, pipeline, and proof without introducing transport logic.

`internal/pipeline`
Deterministic extraction stages and shared stage contracts.

`internal/intel`
Model-backed routing, judging, formatting, and ambiguity handling.

`internal/proof`
Proof artifacts, trace events, replay, diff, and validation helpers.

`internal/store`
Local persistence for traces, fingerprints, cache, and domain genome.

`internal/transport`
CLI and MCP handlers wired to `core`.

## What Is Explicitly Not Allowed

1. Separate package per stage on Day 1.
2. Separate package for CLI and MCP business logic.
3. Parallel data models for transport and core.
4. Empty placeholder directories committed only for future use.
