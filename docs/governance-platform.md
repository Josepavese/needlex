# Governance Platform

This document defines Needle-X code governance as a product-quality platform, not a cosmetic lint bundle.

## Goals

Needle-X governance exists to do four things:
1. block regressions that increase bug risk or maintenance drag
2. keep code concentration visible and measurable
3. make release quality reproducible
4. pressure the codebase toward smaller, clearer, safer units without amputating real product substrates

## Governance Model

Governance has three layers:
1. `hard gates`
   Hard failures that must block merges and releases.
2. `target pressure`
   Non-blocking warnings that identify where the repo still needs reduction.
3. `advisory pressure`
   Rich lint signals used to pick the next refactor fronts.

This distinction is intentional. Needle-X should not pretend a healthy repo baseline does not exist. It should ratchet down honestly from where the code is today.

## Hard Gates

Current hard gates are enforced by:
- [check_governance.sh](/home/jose/hpdev/Libraries/needlex/scripts/check_governance.sh)
- [check_budget.sh](/home/jose/hpdev/Libraries/needlex/scripts/check_budget.sh)
- [budget.env](/home/jose/hpdev/Libraries/needlex/governance/budget.env)
- [golangci.yml](/home/jose/hpdev/Libraries/needlex/.golangci.yml)

They include:
- full test suite
- `gofumpt`
- `go vet`
- `staticcheck`
- structural lint hard set via `golangci-lint`
- hard ceilings on:
  - total production LOC
  - average file LOC
  - internal package count
  - runtime dependency count
  - count of files over 300/350 LOC
  - largest file LOC
  - largest package LOC

## Target Pressure

Target ceilings are warning-only.

They exist to answer one question:
- if the repo passes today, where should the next reduction work land?

Warnings are printed in the budget report but do not fail the run.

## Linter Policy

### Hard lint set

The hard lint set must stay high-signal and low-theater.

Today it includes:
- `bodyclose`
- `errorlint`
- `govet`
- `ineffassign`
- `misspell`
- `unconvert`

Why these:
- they catch correctness, concentration, and maintenance risk
- they do not impose stylistic preferences as primary policy

### Advisory lint set

The advisory lint set exists to surface real next-step refactor fronts without blocking the repo.

Today it adds pressure around:
- `errcheck`
- `funlen`
- `gocyclo`
- `errorlint`
- `bodyclose`

Advisory lint should never be ignored forever, but it should not be promoted to hard gate until the repo can pass honestly.

## Baseline Philosophy

The baseline in [budget.env](/home/jose/hpdev/Libraries/needlex/governance/budget.env) is not aspirational fiction.

Rules:
1. hard limits must be above the current measured baseline unless there is immediate cleanup in the same burst
2. target limits should be meaningfully tighter than hard limits
3. tightening must follow actual wins, not wishful thinking
4. loosening a hard limit requires an explicit architectural reason

## Processes

### Standard local check

Run:

```bash
bash scripts/check_governance.sh .
```

### Governance recalibration

Use:
- [code-governance-workflow.md](/home/jose/hpdev/Libraries/needlex/.agent/workflows/code-governance-workflow.md)

### Release

Public release must still follow:
- [release-workflow.md](/home/jose/hpdev/Libraries/needlex/.agent/workflows/release-workflow.md)

Releases should use the governance script rather than ad hoc subsets of checks.

## Ratchet Policy

A governance change is valid when it does at least one of these:
1. reduces hard failures without hiding them
2. converts vague maintenance risk into inspectable signal
3. makes CI catch a real class of bug earlier
4. reduces hotspot concentration

A governance change is invalid when it only:
1. adds noise
2. encodes taste
3. breaks flow without improving safety
4. replaces hard reasoning with ceremonial policy

## Repo-Managed Memory

Governance is part of repo memory.

The durable sources are:
- [AGENTS.md](/home/jose/hpdev/Libraries/needlex/AGENTS.md)
- [budget.env](/home/jose/hpdev/Libraries/needlex/governance/budget.env)
- [code-governance-workflow.md](/home/jose/hpdev/Libraries/needlex/.agent/workflows/code-governance-workflow.md)
- [SKILL.md](/home/jose/hpdev/Libraries/needlex/.agent/skills/code-governance/SKILL.md)

This keeps governance reproducible across future contributors and future model sessions.
