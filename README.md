# Needle-X

Needle-X is a local-first, serverless, single-binary Go runtime that compiles noisy web pages into compact, high-signal, proof-carrying context for AI agents.

The design target is aggressive but explicit:
1. Full strategic scope on Day 1.
2. Minimal codebase, minimal package count, minimal dependency surface.
3. Deterministic core by default, SLM activation only on measured ambiguity.

## Status

This repository is in architecture and planning mode. There is no placeholder application code yet by design.

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
