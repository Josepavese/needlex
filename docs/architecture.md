# Architecture

## Goal

Needle-X must deliver a full Day 1 runtime without turning into a sprawling codebase. The architecture is therefore constrained before implementation starts.

## Runtime Topology

1. One binary: `cmd/needle`
2. One runtime process
3. One core execution pipeline
4. One transport-neutral service API reused by CLI and MCP

Canonical flow:

```text
Acquire -> Reduce -> Segment -> ExtractDet -> SemanticGate -> (+ResolveAmbiguity) -> ChunkRank -> Pack -> Proof -> Trace
```

## Internal Package Plan

The repo should converge on these `internal/` packages and stay below the package budget:

1. `config`
Load config, defaults, policy thresholds, and runtime budgets.

2. `core`
Own canonical domain types and the main application service entrypoints.

3. `pipeline`
Own deterministic stages: acquire, reduce, segment, extract_det, chunk, rank, pack.

4. `intel`
Own model adapter, router, judge, formatter, and ambiguity policy logic.

5. `proof`
Own proof artifacts, trace events, replay, and diff.

6. `store`
Own local persistence for traces, fingerprints, cache, and domain genome.

7. `transport`
Own thin CLI and MCP adapters that call `core`.

## Design Invariants

1. Deterministic pipeline is the default path and must stay operable offline.
2. Model calls are policy-gated, benchmark-backed, and must emit explicit reason codes.
3. Every successful run emits proof and trace artifacts.
4. CLI and MCP never implement business logic directly.
5. Domain genome and fingerprint graph are storage concerns, not new execution pipelines.
6. The default transport contract must be the compact agent-facing packet, not the diagnostic envelope.

## Local Artifact Layout

Runtime-generated local state should live outside source packages, under an untracked working directory:

```text
.needlex/
  runs/
  traces/
  genome/
  cache/
```

This keeps the repository clean while preserving replayability and local-first behavior.
