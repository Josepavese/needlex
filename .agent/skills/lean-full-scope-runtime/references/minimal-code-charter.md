# Minimal Code Charter

Use this charter as hard constraints while designing and reviewing changes.

## Budget Targets

1. Production LOC: <= 8000 (exclude tests/fixtures).
2. Internal packages: <= 10.
3. Runtime third-party dependencies: <= 8.
4. Max file size: <= 400 LOC.

## Design Constraints

1. One binary, one process, one main pipeline.
2. Deterministic-first path must stay dominant.
3. SLM activation must be policy-gated and explainable.
4. CLI and MCP must reuse the same core runtime API.
5. Proof + trace must be produced for every run.

## Anti-Bloat Rules

1. No new module unless existing module cannot safely absorb behavior.
2. No new dependency without documented stdlib alternative analysis.
3. Refactor if complexity rises faster than capability gain.
4. Reject duplicated data models across transport layers.
