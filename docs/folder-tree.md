# Folder Tree

## Principle

The tree is designed before code, but directories are created only when real code exists. No empty folders are committed to "reserve" architecture.

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
    development-plan.md
    folder-tree.md
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
