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
8. `internal/memory`
9. `internal/transport`
10. `schemas`
11. `scripts`
12. `testdata/golden`
13. `scripts/external_baselines`
14. `improvements`
15. `benchmarks`
16. `benchmarks/corpora`
17. `benchmarks/seeded/runner`
18. `benchmarks/competitive/runner`
19. `benchmarks/discovery_memory/runner`
20. `benchmarks/live_read_eval/runner`
21. `benchmarks/discovery_eval/runner`
22. `benchmarks/hard_case_matrix/runner`
23. `docs/archive`
24. `docs/experimental`
25. `.needlex`

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
    competitive-benchmark-protocol.md
    development-plan.md
    folder-tree.md
    go-to-market.md
    model-baseline.md
    operator-guide.md
    project-context.md
    semantic-alignment-gate.md
    vercel-browser-agent-bridge.md
    vademecum.md
    archive/
    experimental/
      agentic-decision-plane-spec.md
      discovery-memory-spec.md
      seedless-discovery-strategy.md
  improvements/
    README.md
    archive/
    live-read-baseline.json
    live-read-latest.json
    hard-case-matrix-baseline.json
    hard-case-matrix-latest.json
    discovery-eval-latest.json
    seeded-benchmark-latest.json
    competitive-benchmark-latest.json
    discovery-memory-benchmark-latest.json
  benchmarks/
    README.md
    corpora/
      hard-case-corpus-benchmark-v1.json
      hard-case-corpus-smoke-v1.json
      hard-case-corpus-v1.json
      hard-case-corpus-v2.json
      live-sites-cpu-v1.json
      live-sites-market-v2.json
      live-sites-semantic-eval-v1.json
      live-sites-semantic-global-v1.json
      model-candidates-cpu-v1.json
      seeded-corpus-v1.json
      competitive-corpus-v1.json
      discovery-corpus-v1.json
    competitive/
      runner/
        main.go
        main_test.go
        vercel_browser_agent_bridge_example.ts
    discovery_eval/
      runner/
        main_test.go
    discovery_memory/
      runner/
        main.go
    hard_case_matrix/
      runner/
        doc.go
        main_test.go
    live_read_eval/
      runner/
        main.go
        main_test.go
    seeded/
      runner/
        main.go
        main_test.go
  internal/
    config/
    core/
    intel/
    memory/
    pipeline/
    proof/
    store/
    transport/
  schemas/
    mcp/
    proof.schema.json
    resultpack.schema.json
  scripts/
    archive/
    check_budget.sh
    external_baselines/
    lib/
    run_cpu_baseline_matrix.sh
    run_cpu_model_benchmark.sh
    run_discovery_eval.sh
    run_hard_case_matrix.sh
    run_live_read_eval.sh
    run_live_semantic_eval.sh
    run_qwen35_cpu_matrix.sh
    run_semantic_embed_upstream.py
    run_semantic_gate_smoke.sh
  testdata/
    golden/
  .needlex/
    competitive-benchmark.env
    competitive-benchmark-cache.json
    discovery/
      discovery.db
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
Model-backed ambiguity handling, semantic context alignment, and runtime policy logic.

`internal/memory`
Local discovery memory, embeddings persistence, vector recall, and bounded pruning.

`internal/proof`
Proof artifacts, trace events, replay, diff, and validation helpers.

`internal/store`
Local persistence for traces, fingerprints, cache, and domain genome.

`internal/transport`
CLI and MCP handlers wired to `core`.

`benchmarks`
Standalone evaluation harness, corpora, and competitor runners.

`scripts`
Thin operator wrappers and helper scripts only. Benchmark harness code does not live here anymore.

## What Is Explicitly Not Allowed

1. Separate package per stage on Day 1.
2. Separate package for CLI and MCP business logic.
3. Parallel data models for transport and core.
4. Empty placeholder directories committed only for future use.
