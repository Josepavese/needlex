# Folder Tree

## Principle

The repository reflects the current runtime, not a speculative architecture.
Only real code and active docs are materialized.

## Current Tree

```text
needlex/
  cmd/
    needle/
  docs/
    agent-answer-packet.md
    architecture.md
    benchmark-report.md
    competitive-benchmark-protocol.md
    folder-tree.md
    go-to-market.md
    model-baseline.md
    operator-guide.md
    project-context.md
    seeded-benchmark-spec.md
    semantic-alignment-gate.md
    vademecum.md
    vercel-browser-agent-bridge.md
    archive/
    experimental/
    wiki/
  improvements/
    README.md
    archive/
    competitive-benchmark-latest.json
    discovery-eval-latest.json
    discovery-memory-benchmark-latest.json
    hard-case-matrix-baseline.json
    hard-case-matrix-latest.json
    live-read-baseline.json
    live-read-latest.json
    live-semantic-eval-latest.json
    live-semantic-global-eval-latest.json
    live-validation-closure-latest.json
    seeded-benchmark-latest.json
  benchmarks/
    README.md
    corpora/
    competitive/runner/
    discovery_eval/runner/
    discovery_memory/runner/
    hard_case_matrix/runner/
    live_read_eval/runner/
    seeded/runner/
  internal/
    config/
    core/
    evalutil/
    intel/
    memory/
    pipeline/
    proof/
    store/
    transport/
  schemas/
  scripts/
    archive/
    external_baselines/
    lib/
    check_budget.sh
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
```

## Responsibilities

`internal/`
Product code only.

`benchmarks/`
Reproducible evaluation harness only.

`scripts/`
Thin operator wrappers and helper scripts only.

`improvements/`
Active benchmark outputs only.

`docs/archive/`
Historical material only.

`docs/experimental/`
Strategic but non-active specs only.

## Rules

1. No empty placeholder directories.
2. No benchmark harness code under `internal/`.
3. No active docs with historical planning content in root `docs/`.
4. No one-off reports in root `improvements/`.

## Docs Surface

Primary entrypoints:
1. [README](../README.md)
2. [Wiki Home](wiki/Home.md)
3. [Operator Guide](operator-guide.md)
4. [Tool Calling](tool-calling.md)
