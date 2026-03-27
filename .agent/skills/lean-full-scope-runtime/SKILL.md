---
name: lean-full-scope-runtime
description: Formalize and enforce a full-scope Day 1, minimal-code architecture for next-generation runtimes. Use when defining specs, planning implementation, reviewing pull requests, or coding features where all critical capabilities must exist from Day 1 while minimizing files, LOC, dependencies, and maintenance risk.
---

# Lean Full Scope Runtime

Apply this skill to ship disruptive functionality without code bloat.

## Mission

Keep two constraints true at the same time:
1. Ship full strategic scope on Day 1 (no stubs, no empty versions).
2. Keep the codebase minimal, legible, and replay-safe.

## Non-Negotiables

1. Keep one runtime process and one primary execution pipeline.
2. Reuse shared primitives before creating modules.
3. Keep deterministic core as default path.
4. Trigger SLM only on explicit ambiguity/conflict conditions.
5. Emit proof and trace artifacts on every run.
6. Reject placeholder endpoints and partial feature surfaces.

## Workflow

### 1) Lock hard constraints first
Define and freeze:
- max production LOC
- max internal package count
- max runtime dependencies
- max file length

Load [minimal-code-charter.md](references/minimal-code-charter.md) and copy thresholds into spec/config before implementation.

### 2) Collapse architecture aggressively
Design one core pipeline and attach features as stages.
Do not create parallel pipelines for CLI and MCP. Use one shared core API.

### 3) Implement Day 1 complete surface
Ship all strategic capabilities at launch:
- deterministic extraction
- ambiguity-routed SLM lanes
- proof-carrying output
- replay and diff
- CLI and MCP parity

Do not defer strategic features to "v2".

### 4) Enforce anti-bloat during implementation
Before merge, run:
```bash
bash scripts/check_budget.sh .
```

If any budget fails, refactor before proceeding.

### 5) Verify quality gates
Run deterministic replay tests and extraction golden tests.
Reject changes that increase ambiguity calls without measurable fidelity gain.

### 6) Review using explicit release gate
Load [release-gate.md](references/release-gate.md) and check every item.
No exceptions for "quick wins" that violate architecture invariants.

## Implementation Rules

1. Create interfaces only when at least two concrete implementations exist.
2. Prefer config-driven behavior over feature-specific branching.
3. Share one model adapter for router/judge/formatter.
4. Emit proof and trace from one shared event surface.
5. Keep runtime dependencies minimal and justified.

## Pull Request Requirements

1. Include LOC delta and rationale.
2. List any new dependency and why stdlib is insufficient.
3. Report budget check output from `scripts/check_budget.sh`.
4. Confirm replay/proof behavior unchanged or improved.

## Resources

- [minimal-code-charter.md](references/minimal-code-charter.md): hard budgets and design constraints.
- [release-gate.md](references/release-gate.md): Day 1 completeness and merge gate checklist.
- `scripts/check_budget.sh`: automated budget enforcement.
