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
- shipped-contract guards:
  - `check_skills.sh`: every shipped skill declares `name`, `description`, and the release `version` it documents
  - `check_semantic_guard.sh`: semantic-first doctrine is present and banned surface-form retrieval residues stay out
  - `check_skill_refresh.sh`: the installer's host-skill refresh backs up the previous copy, restores it on failure, and never installs a skill that was not already present
- hard ceilings on:
  - total production LOC, excluding benchmark runners
  - average file LOC
  - internal package count
  - runtime dependency count
  - count of files over 300/350 LOC
  - largest file LOC
  - largest package LOC

Benchmark LOC is reported separately as `BENCHMARK_LOC`.
Benchmarks keep their own advisory lint lane because they are operational evidence tooling, not installed runtime product code.

## Target Pressure

Target ceilings are warning-only.

They exist to answer one question:
- if the repo passes today, where should the next reduction work land?

Warnings are printed in the budget report but do not fail the run.

## Linter Policy

### Hard lint set

The hard lint set must stay high-signal and low-theater.

Today it includes:
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

Advisory pressure is split into two lanes:
1. product advisory
   - `internal`, `cmd`, `schemas`
   - this is the main backlog signal for runtime quality work
2. benchmark advisory
   - `benchmarks`
   - this is visible on purpose, but separated so benchmark debt does not drown product debt

`bodyclose` is not in the hard or advisory set right now.

That is intentional:
- it produced low-confidence results in this repo
- it was not a trustworthy enough signal to justify gate pressure

If it becomes high-signal later, it can return through the normal ratchet process.

Advisory lint should never be ignored forever, but it should not be promoted to hard gate until the repo can pass honestly.

## Baseline Philosophy

The baseline in [budget.env](/home/jose/hpdev/Libraries/needlex/governance/budget.env) is not aspirational fiction.

Rules:
1. hard limits must be above the current measured baseline unless there is immediate cleanup in the same burst
2. target limits should be meaningfully tighter than hard limits
3. tightening must follow actual wins, not wishful thinking
4. loosening a hard limit requires an explicit architectural reason

Current calibration:
1. hard LOC gates still protect against uncontrolled growth
2. target LOC pressure is aligned to the current semantic, analytics, memory, provider-diversity, and discovery-diagnostics production surface
3. future LOC wins should ratchet `TARGET_MAX_PROD_LOC` and `TARGET_MAX_PACKAGE_LOC` down after measurable cleanup

### v0.1.33 strategic recalibration

The v0.1.33 baseline was recalibrated only after a structural reduction pass.
The production surface now includes agent-readable standards, network-aware PAL rendering, and semantic query review. Removing those substrates merely to recover the former total-LOC ceiling would reduce product behavior.

The compensating structural results are measured rather than assumed:
- average Go file size fell from 225 to 192 LOC
- files over 300 LOC fell from 37 to 27
- files over 350 LOC fell from 28 to 22
- the largest file fell from 1,895 to 798 LOC
- the largest package fell from 8,605 to 6,650 LOC
- deprecated WebSocket usage was removed and `staticcheck` is green

### v0.1.34 network-aware delivery recalibration

The v0.1.33 renderer admitted network capture in code but could not deliver it: the
whole retrieval substrate had 150 lines of headroom, so closing measured capture gaps
(adaptive render budget, streaming fetch bodies, observable degradation, semantic render
escalation) would have meant deleting diagnostics instead of fixing behavior.

The total ceiling moved once, and file-size discipline tightened in the same burst:
- `HARD_MAX_PROD_LOC` 34,500 to 34,900, `TARGET_MAX_PROD_LOC` 34,000 to 34,500
- `HARD_MAX_AVG_FILE_LOC` 235 to 210, `TARGET_MAX_AVG_FILE_LOC` 210 to 200
- `HARD_MAX_FILES_OVER_300` 30 to 29
- `HARD_MAX_FILES_OVER_350` 24 to 23

Measured baseline at that decision: production LOC 34,560, average file 194 LOC,
28 files over 300 LOC, 22 files over 350 LOC, largest file 798 LOC, largest package
6,650 LOC. Every tightened threshold passes with the raised total, so the new volume
cannot become bigger files or new hotspots.

Follow-up in the same release cycle: the later capture work pushed
`internal/rendering/network_state.go` and `internal/core/sourceresolution/robots.go`
past the file thresholds, so both were decomposed along cohesion lines
(`network_evidence.go` for body decoding and evidence hygiene, `render_policy.go` for
escalation and budget policy). That restored slack instead of relaxing the new limits:
28 files over 300 LOC, 22 files over 350 LOC.

Environment note: `staticcheck`, advisory lint and structure lint failed on this machine
because the installed lint binaries could not read the export data of the local Go
toolchain, which was newer than the module minimum. The same gates were already failing
on the pristine pre-change tree for the same reason, so the failures were attributable to
tooling mismatch, not to the recalibration.

That mismatch was removed in v0.1.37: the module now targets Go 1.27, the lint pins
(`gofumpt v0.12.0`, `staticcheck 2026.2.1`, `golangci-lint v2.13.2`) are installed from the
same toolchain CI resolves from `go.mod`, and local and CI gate results agree. A developer
whose local Go is newer than the module minimum must set `GOTOOLCHAIN` to the version in
`go.mod` before trusting a local gate result, because lint binaries built with the older
toolchain cannot read newer export data.

Accordingly, only two hard limits changed:
- production LOC: `30,500` to `34,500`, with target pressure at `34,000`
- direct runtime dependencies: `4` to `5`, with target pressure at `4`

The dependency count is deliberately truthful. `go mod tidy` classified every imported module as direct; the runtime uses HTTP, HTML, SQLite memory, YAML standards parsing, and maintained WebSocket/CDP transport. Marking an imported dependency as indirect to satisfy the old gate would hide architecture rather than improve it.

Zero-legacy cleanup accompanied the recalibration: the unused remote-CDP configuration field and its environment/CLI aliases were removed, the deprecated WebSocket module was replaced, and extracted packages expose their real APIs without service-layer compatibility shims.

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
