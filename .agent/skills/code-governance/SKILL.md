---
name: code-governance
description: Redesign or evolve Needle-X code governance honestly: recalibrate hard and target limits, adopt high-signal linters, maintain CI/release gates, and keep durable governance memory in sync.
---

# Code Governance

Use this skill when changing how the repository governs code quality.

## Objective

Improve code quality and bug resistance without introducing ceremony or false constraints.

## Core Rules

1. Start from the real repo baseline.
2. Separate `hard gates` from `target pressure` and `advisory pressure`.
3. Do not adopt a linter unless it leads to concrete action.
4. Prefer repo-managed configuration over hidden shell defaults.
5. Keep governance memory durable in docs, workflows, and skills.

## Required Files

When doing governance work, inspect and update as needed:
- `governance/budget.env`
- `.golangci.yml`
- `governance/golangci.advisory.yml`
- `scripts/check_budget.sh`
- `scripts/check_governance.sh`
- `.github/workflows/governance.yml`
- `docs/governance-platform.md`
- `.agent/workflows/code-governance-workflow.md`

## Workflow

1. Collect baseline with `bash scripts/check_budget.sh .`.
2. Confirm hotspot files and hotspot packages.
3. Decide what belongs in:
   - hard gate
   - target warnings
   - advisory lint
4. Update repo-managed config.
5. Validate with `bash scripts/check_governance.sh .`.
6. Update docs and workflow memory in the same burst.

## Anti-Patterns

Reject these moves:
- lowering pressure by silently raising hard limits without explanation
- adding style-only linters as blockers
- pretending advisory output is actionable when it is not
- leaving CI and local scripts out of sync

## Success Condition

A governance change is complete only when:
1. local governance checks pass honestly
2. CI uses the same policy
3. docs explain the model clearly
4. future contributors can repeat the process without reconstructing intent
