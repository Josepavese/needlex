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
    fetch-profiles.md
    folder-tree.md
    go-to-market.md
    governance-platform.md
    install.md
    model-baseline.md
    operator-guide.md
    project-context.md
    seeded-benchmark-spec.md
    semantic-alignment-gate.md
    tool-calling.md
    vademecum.md
    vercel-browser-agent-bridge.md
    assets/
    experimental/
    roadmap/
    wiki/
  governance/
    budget.env
    golangci.advisory.yml
  improvements/
    README.md
  install/
    install.sh
    install.ps1
  issues/
  benchmarks/
    README.md
    corpora/
    competitive/runner/
    discovery_eval/runner/
    discovery_memory/runner/
    hard_case_matrix/runner/
    internal/
    live_read_eval/runner/
    native_api_endpoint/runner/
    seeded/runner/
    seedless_ddg/runner/
  internal/
    analytics/
    config/
    core/
    intel/
    memory/
    observability/
    pipeline/
    platform/
    proof/
    rendering/
    store/
    transport/
  schemas/
  scripts/
    external_baselines/
    lib/
    release/
    check_budget.sh
    check_governance.sh
    check_semantic_guard.sh
    check_skills.sh
    run_*.sh
  skills/
    needlex-web-retrieval/
  testdata/
    golden/
```

Note on `improvements/`: benchmark JSON outputs are local analysis artifacts and are not tracked; the directory keeps its README plus the artifacts currently under analysis.

## Responsibilities

`internal/`
Product code only.

`benchmarks/`
Reproducible evaluation harness only.

`scripts/`
Thin operator wrappers and helper scripts only.

`improvements/`
Active benchmark outputs only.

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
5. [Agent Answer Packet](agent-answer-packet.md)
6. [Install](install.md)
7. [Governance Platform](governance-platform.md)
