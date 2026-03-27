# Needle-X

Needle-X is a local-first, serverless, single-binary Go runtime that compiles noisy web pages into compact, high-signal, proof-carrying context for AI agents.

The design target is aggressive but explicit:
1. Full strategic scope on Day 1.
2. Minimal codebase, minimal package count, minimal dependency surface.
3. Deterministic core by default, SLM activation only on measured ambiguity.

## Status

This repository is in active implementation mode. The runtime is still incomplete, but the deterministic core path is real and executable.

Current implemented baseline:
1. `go.mod` with one runtime dependency for HTML parsing (`golang.org/x/net/html`)
2. Canonical core contracts in `internal/core`
3. JSON-first config loading with env overrides in `internal/config`
4. Deterministic `Acquire`, `Reduce`, and `Segment` in `internal/pipeline`
5. Local `intel` policy engine with ambiguity scoring, reason codes, lane escalation, domain force-lane hints, deterministic extractor, and constrained formatter
6. `ProofRecord`, `RunTrace`, `Recorder`, and `DiffReport` in `internal/proof`
7. End-to-end deterministic `Read`, `Query`, and `Crawl` orchestration in `internal/core/service`, with query strategies `off|same_site_links` for measurable discovery-vs-seed comparison
8. Thin CLI and MCP transport surface in `cmd/needle` and `internal/transport`, including `query` and `crawl`
9. Local state persistence in `.needlex/{traces,proofs,fingerprints,genome}` via `internal/store`, with genome-driven `force_lane`, `preferred_profile`, `pruning_profile`, and `render_hint`
10. Versioned schema files in `schemas/`
11. Golden end-to-end fixtures and benchmark coverage for `read`, `query`, and `crawl`
12. Contract tests for CLI and MCP transport shape
13. Golden NFR gates for determinism, fidelity, and `tiny` compression >= `3x`
14. Tests and budget enforcement passing
15. Lane `2/3` transform chain recorded in proofs and traces through local `extract_slm` and `formatter` policy stages

## Current CLI

```bash
go run ./cmd/needle crawl https://example.com --max-pages 3 --max-depth 1 --same-domain
go run ./cmd/needle query https://example.com --goal "proof replay deterministic"
go run ./cmd/needle query https://example.com --goal "proof replay deterministic" --discovery off
go run ./cmd/needle read https://example.com
go run ./cmd/needle read https://example.com --json
go run ./cmd/needle read https://example.com --profile tiny
go run ./cmd/needle read https://example.com --profile deep --json
go run ./cmd/needle replay trace_1
go run ./cmd/needle diff trace_a trace_b --json
go run ./cmd/needle proof trace_1 --json
go run ./cmd/needle proof chk_1
go run ./cmd/needle prune --older-than-hours 24
go run ./cmd/needle mcp
```

## Working Documents

- [idea.md](/home/jose/hpdev/Libraries/needlex/idea.md)
- [spec.md](/home/jose/hpdev/Libraries/needlex/spec.md)
- [architecture.md](/home/jose/hpdev/Libraries/needlex/docs/architecture.md)
- [folder-tree.md](/home/jose/hpdev/Libraries/needlex/docs/folder-tree.md)
- [development-plan.md](/home/jose/hpdev/Libraries/needlex/docs/development-plan.md)

## Repo Rules

1. No empty architectural stubs.
2. No duplicate logic between CLI and MCP.
3. No feature lands without proof, replay, and budget awareness.
4. No merge if code budget or determinism gates fail.

## Budget Check

Use:

```bash
bash scripts/check_budget.sh .
```

Current hard targets:
1. Production LOC <= 8000
2. Internal packages <= 10
3. Runtime dependencies <= 8
4. Max file size <= 400 LOC
