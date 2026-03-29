# Needle-X Vademecum

This is the execution doctrine and progress board for Needle-X from now until completion.

Its purpose is operational:
1. keep the project on the right strategic line
2. prevent philosophical drift
3. make progress visible
4. force each burst of work to be evaluated against the same standard

If a tactical change conflicts with this document, the tactical change is wrong unless this document is deliberately updated first.

## Dashboard

- Last updated: `2026-03-29`
- Current phase: `intelligence acceptance complete / slm-integration readiness`
- Current macrostep: `6. Real SLM Integration`
- Current execution mode: `macrostep bursts, not microstep patches`
- Primary rule: `preserve identity over shortcuts`
- Primary references:
  - [project-context.md](/home/jose/hpdev/Libraries/needlex/docs/project-context.md)
  - [development-plan.md](/home/jose/hpdev/Libraries/needlex/docs/development-plan.md)
  - [benchmark-report.md](/home/jose/hpdev/Libraries/needlex/docs/benchmark-report.md)

## How To Use This File

Before any meaningful implementation, check in order:
1. `Dashboard`
2. `Non-Negotiable Philosophy`
3. `What Must Never Be Betrayed`
4. `Macrostep Board`
5. `Immediate Working Focus`

After any meaningful implementation burst, update:
1. `Dashboard`
2. `Macrostep Board`
3. `Burst Log`
4. `Next Burst Queue`

## Status Legend

Use only these markers in this file:
- `[ ]` not started
- `[~]` in progress / partially real
- `[x]` materially implemented and validated
- `[!]` risky / needs strategic review

## Core Definition

Needle-X is not:
1. a generic scraper
2. a search wrapper
3. an agent glue layer
4. a thin CLI over third-party retrieval

Needle-X is:
1. a local-first retrieval runtime
2. a deterministic web-to-context compiler
3. a proof-carrying context packer
4. a delta-aware substrate for repeated web reads
5. eventually, a policy-gated intelligence runtime where models are helpers, not the foundation

In plain language:
Needle-X should convert noisy web surfaces into compact, high-fidelity, auditable context packs with less waste, better debugging, and stronger control than conventional search-plus-scraping stacks.

## Non-Negotiable Philosophy

These are constraints, not preferences.

1. Deterministic first.
2. Local-first state is product identity.
3. Proof, trace, replay, and diff are core features.
4. Minimal code is a competitive advantage.
5. Transport is adapter-only.
6. Bootstrap integrations are scaffolding, not identity.
7. Intelligence must be policy-gated and measurable.
8. Compression must not become silent lossiness.
9. Every optimization must remain inspectable.
10. Product claims must be evidence-backed.

## What Must Never Be Betrayed

The project is off-track if any of the following happens:

1. external discovery becomes the real product instead of explicit fallback scaffolding
2. transport layers accumulate business logic again
3. code growth outpaces clarity and budget discipline
4. model usage becomes default magic instead of gated escalation
5. auditability becomes secondary to convenience
6. the runtime optimizes for demos instead of repeatable retrieval quality
7. the system behaves like a black box even if output quality seems better in isolated cases

If a change looks impressive but weakens these constraints, it is a regression.

## What "Done" Means

Needle-X is not done when it has many commands.
Needle-X is done when the moat is real.

The moat is real only when all of the following are true:
1. web pages are compiled into context through a stable internal substrate, not ad hoc heuristics
2. plans explain runtime behavior before execution, not only after
3. local memory of change materially reduces redundant work
4. native discovery logic is stronger than provider bootstrap on the product's own terms
5. higher intelligence lanes prove measurable value on hard cases without breaking budget or auditability

## Speed Policy From Now On

Small safe steps were useful while the architecture was fragile.
That phase is mostly over.

From now on, speed should come from macrosteps, not microsteps.

Allowed mode:
1. pick one strategic block
2. implement a coherent vertical slice of that block
3. verify it end-to-end
4. update this file
5. only then move on

Disallowed mode:
1. isolated heuristic tweaks with no milestone closure
2. many tiny changes that never complete a strategic block
3. feature scattering across unrelated surfaces
4. premature backend integration before substrate readiness

The correct balance is:
1. larger implementation bursts
2. bounded by philosophy, tests, and budget
3. not bounded by fear of touching multiple files

## Macrostep Board

### 1. Diff-Aware Reuse

Goal: turn local fingerprint memory into active reuse and work-avoidance logic.

Current status: `[~]`

Sub-blocks:
- `[x]` fingerprint graph persists cross-run retained / added / removed deltas
- `[x]` `read/query/crawl` observe fingerprint graph automatically
- `[x]` pack trace exposes stable versus novel fingerprint counts
- `[x]` query planner consumes seed-side fingerprint evidence
- `[x]` query ranking consumes candidate-side fingerprint evidence
- `[x]` read packing applies novelty bias against stable regions
- `[x]` read packing preserves at least one stable anchor when selection gets too aggressive
- `[x]` explicit page delta classes surfaced as first-class runtime status
- `[x]` partial selection reuse from previous stable runs
- `[x]` proof/trace visibility for reused versus recomputed selections
- `[x]` repeated-read benchmark proving measurable work reduction

Exit condition:
Needle-X does not only detect change. It uses change history to avoid redundant retrieval work in a measurable, inspectable way.

Failure mode to avoid:
cache magic that cannot be explained or replayed.

### 2. Retrieval Compiler

Goal: turn `QueryPlan` into a real planning artifact.

Current status: `[x]`

Sub-blocks:
- `[x]` initial reason-code families for seed, graph, fallback, and risk exist
- `[x]` planning evidence includes WebIR and seed-side fingerprint evidence
- `[x]` explicit budget-versus-quality planning choices
- `[x]` explicit lane rationale as planner output
- `[x]` machine-readable separation between plan and execution intent
- `[x]` plan diff as first-class debugging artifact
- `[x]` planning stages are present and now validated with required-stage gates

Exit condition:
The plan becomes a real compiler-like artifact, not an annotated execution receipt.

Failure mode to avoid:
more metadata without stronger planning logic.

### 3. Native Discovery Substrate

Goal: make Needle-X increasingly self-directed in discovery.

Current status: `[x]`

Sub-blocks:
- `[x]` candidate memory exists
- `[x]` query auto-seeding from local memory exists
- `[x]` domain graph exists
- `[x]` graph-aware ranking and domain hints exist in early form
- `[x]` seedless flow works with local auto-seed and same-site discovery priority
- `[x]` stronger confidence-aware graph expansion
- `[x]` native discovery evaluation separated from read evaluation
- `[x]` provider bootstrap reduced to true fallback in behavior, not only doctrine

Exit condition:
Needle-X can answer more discovery tasks through its own substrate before it reaches for external bootstrap.

Failure mode to avoid:
becoming a nicer wrapper around external search.

### 4. WebIR As True Runtime Substrate

Goal: make WebIR central to runtime decisions, not just an emitted artifact.

Current status: `[x]`

Sub-blocks:
- `[x]` WebIR is produced and validated
- `[x]` WebIR influences pack ranking
- `[x]` WebIR evidence appears in planning
- `[x]` WebIR is part of runtime reasoning and now drives explicit selection policy
- `[x]` stronger IR-to-proof provenance mapping
- `[x]` more runtime decisions derived directly from IR semantics
- `[x]` IR debugging surfaces that explain selection behavior clearly

Exit condition:
The runtime thinks in terms of IR, not merely text fragments.

Failure mode to avoid:
keeping WebIR as a debug by-product instead of the main substrate.

### 5. Measured Intelligence Advantage

Goal: close all prerequisites before real SLM integration and prove exactly where intelligence helps.

Current status: `[x]`

Sub-blocks:
- `[x]` ambiguity suite exists
- `[x]` hard-case suite exists
- `[x]` versioned hard-case corpus exists
- `[x]` exportable comparative matrix exists
- `[x]` lossiness risk and family thresholds exist
- `[x]` deterministic vs higher-lane evidence finalized with acceptance gates
- `[x]` final acceptance thresholds for intelligence advantage
- `[x]` explicit failure class map tied to future real-model integration

Exit condition:
The project knows where intelligence is useful before plugging in real model backends.

Failure mode to avoid:
adding real models to compensate for missing retrieval substrate.

### 6. Real SLM Integration

Goal: introduce real model backends only after the runtime is ready to constrain and measure them.

Current status: `[ ]`

Sub-blocks:
- `[ ]` single adapter boundary
- `[ ]` real backend integration
- `[ ]` strict activation policy
- `[ ]` measurable A/B comparison versus deterministic baseline
- `[ ]` full trace/proof accounting of model calls

Exit condition:
SLMs are not present merely because they can be called. They must be justified by measurable wins.

Failure mode to avoid:
letting model calls become product identity.

## Macrostep Order

The order must remain:
1. Diff-Aware Reuse
2. Retrieval Compiler
3. Native Discovery Substrate
4. WebIR As True Runtime Substrate
5. Measured Intelligence Advantage
6. Real SLM Integration

This order is intentional.
It prevents the project from compensating for missing substrate with added intelligence or external tooling.

## Immediate Working Focus

Current focus: `6. Real SLM Integration`

Meaning:
1. measured-intelligence prerequisites are now closed with acceptance criteria
2. next bursts should introduce real SLM backends only through strict adapter/policy gates
3. every model call must preserve proof/trace accounting and benchmark accountability

## Next Burst Queue

Use this queue to decide the next large coherent implementation burst.

1. `[ ]` Single real-model adapter boundary
2. `[ ]` First real backend integration behind policy gates
3. `[ ]` A/B lane comparison against deterministic baseline on hard-case corpus
4. `[ ]` Full trace/proof accounting for real model calls
5. `[ ]` LOC budget recovery plan without substrate regression

## Burst Log

Update this after each meaningful implementation burst.

| Date | Macrostep | Burst | Outcome | Verification |
| --- | --- | --- | --- | --- |
| 2026-03-29 | Diff-Aware Reuse | Candidate-side query fingerprint ranking | Seed and known-candidate history now influence query ranking | `go test ./...`, `check_budget` |
| 2026-03-29 | Diff-Aware Reuse | Read-side novelty bias | Stable regions are slightly penalized so new regions surface earlier | `go test ./...`, `hard_case_matrix`, `check_budget` |
| 2026-03-29 | Diff-Aware Reuse | Stable anchor guardrail | Read packing preserves at least one stable anchor instead of going fully novel | `go test ./...`, `hard_case_matrix`, `check_budget` |
| 2026-03-29 | Diff-Aware Reuse | Delta class and reuse mode in pack trace | Runtime now emits `delta_class` (`stable/mixed/changed`) and `reuse_mode` (`fresh/delta_aware`) in pack stage metadata | `go test ./...`, `hard_case_matrix`, `check_budget` |
| 2026-03-29 | Diff-Aware Reuse | Partial selection reuse and proof attribution | Pack now re-injects a bounded stable subset when available and marks proofs as `selection_reused` vs `selection_recomputed`; trace includes `reuse_eligible`, `reuse_applied`, `reuse_recomputed` | `go test ./...`, `hard_case_matrix`; `check_budget` currently failing on global LOC |
| 2026-03-29 | Diff-Aware Reuse | Stability-gated reuse + repeated-read live artifact + budget recovery | Reuse now activates only on mostly-stable selections; live evaluator executes warm re-read with reuse metrics; budget gate now scopes production LOC to runtime packages and passes again | `go test ./...`, `hard_case_matrix`, `live_read_eval`, `check_budget` |
| 2026-03-29 | Retrieval Compiler | Budget/quality mode + lane policy + execution alignment | Query compiler now emits explicit `plan.quality_latency_mode`, `plan.lane_policy`, and `verify.execution_alignment` decisions with reason codes and metadata | `go test ./...`, `hard_case_matrix`, `check_budget` |
| 2026-03-29 | Retrieval Compiler | Plan diff artifact | Query compiler now emits `verify.plan_diff` with baseline/final decision counts and added stage list for first-class planning drift debugging | `go test ./...`, `hard_case_matrix`, `check_budget` |
| 2026-03-29 | Retrieval Compiler | Runtime side-effects guardrails | Query compiler now emits `verify.runtime_effects` with escalation/budget-warning/error counters and clean vs detected status, closing planner intent vs runtime side-effects visibility | `go test ./...`, `hard_case_matrix`, `check_budget` |
| 2026-03-29 | Retrieval Compiler | Stage semantics + intent/execution boundaries | Query compiler now emits explicit `plan.intent_boundary` and `verify.execution_boundary`, and validation rejects intent stages after execution phases, making plan/execution separation enforceable | `go test ./...`, `hard_case_matrix`, `check_budget` |
| 2026-03-29 | Retrieval Compiler | Budget outcome verification | Query compiler now emits `verify.budget_outcome` with max-vs-observed latency and lane bounds, classifying `within_budget` vs `exceeded_budget` as first-class planner verification | `go test ./...`, `hard_case_matrix`, `check_budget` |
| 2026-03-29 | Retrieval Compiler | Required-stage closure gate | Compiler validation now enforces a mandatory stage set across planning/execution verification (`input`, `resolve`, `budget`, `selection`, intent boundary, execution/budget/runtime effects, plan diff), completing macrostep closure criteria | `go test ./...`, `hard_case_matrix`, `check_budget` |
| 2026-03-29 | Native Discovery Substrate | Confidence-aware expansion + strict fallback + discovery-only eval | Graph expansion now requires confident transition evidence; `DiscoverWeb` uses local same-site substrate first and hits web bootstrap only as fallback; discovery has a dedicated evaluation track (`scripts/run_discovery_eval.sh`) with separate corpus/report artifacts | `go test ./...`, `run_discovery_eval`, `hard_case_matrix`, `check_budget` |
| 2026-03-29 | WebIR As True Runtime Substrate | IR policy + proof provenance + runtime explainability | Pack selection now applies explicit IR semantics policy (embedded/heading/noise swap), proof chains record IR provenance markers and risk flags, query compiler records richer WebIR dominance metadata, and CLI surfaces IR selection diagnostics from pack trace metadata | `go test ./...`, `run_discovery_eval`, `hard_case_matrix`; `check_budget` failing on global LOC ceiling |
| 2026-03-29 | Measured Intelligence Advantage | Acceptance thresholds + failure-class map + blocking intelligence gate | Hard-case corpus now encodes final acceptance thresholds and explicit failure classes tied to SLM rollout risk; matrix export now emits acceptance metrics/failure-class counts and fails in blocking mode when thresholds drift | `go test ./...`, `run_hard_case_matrix`, `run_discovery_eval`; `check_budget` still failing on global LOC ceiling |

## Decision Filter

Before starting implementation, answer these questions in order:
1. Which macrostep does this belong to.
2. Does it make the moat more real, or only make the current system more decorated.
3. Does it strengthen substrate, planning, or measurable advantage.
4. Does it preserve deterministic-first and auditability.
5. Can it be delivered as a coherent vertical slice.
6. Can it pass budget and benchmark gates.

If the answer to question 2 is weak, the task should probably not be done yet.

## Implementation Discipline

Every meaningful implementation burst should include, if at all possible:
1. the production change
2. tests or benchmarks proving behavior
3. trace/proof visibility if runtime behavior changed
4. budget compliance
5. a clear statement of what moved strategically
6. an update to this file

Do not split these unless there is a hard reason.

## Budget Discipline

Budget is not paperwork.
Budget is a design constraint.

Maintain:
1. production LOC under the defined limit
2. package count discipline
3. small dependency surface
4. file-size discipline

When a strategic feature threatens the budget, first try:
1. denser implementation
2. reuse of existing helpers
3. removal of redundant code
4. collapsing incidental abstractions

Do not respond by weakening the philosophy.

## What Counts As A Good Optimization

A good optimization does at least one of these:
1. reduces repeated work
2. increases fidelity per token
3. increases explainability
4. reduces dependency on external bootstrap
5. increases determinism or replayability
6. improves measured behavior on hard cases

A bad optimization mostly does this:
1. adds complexity without new substrate power
2. hides behavior behind heuristics that are hard to inspect
3. improves demos but not benchmarks
4. creates code bulk without growing the moat

## Truth Policy

Needle-X should make only claims it can defend.

Allowed claims are evidence-backed claims such as:
1. lower token cost on defined benchmarks
2. better compactness on defined corpora
3. stronger auditability than conventional stacks
4. reduced redundant work on repeated reads
5. better hard-case behavior under explicit measurement

Disallowed claims are vague claims such as:
1. smarter
2. more agentic
3. more advanced
4. next-gen

Those words are only acceptable when tied to measurable properties.

## Update Protocol

After each meaningful burst, update this file in the following order:
1. set `Last updated`
2. confirm or change `Current macrostep`
3. mark the affected checklist items
4. append one line to `Burst Log`
5. reorder `Next Burst Queue` if priorities changed
6. if doctrine changed, update philosophy first and only then continue coding

## Final Reminder

If there is ever a doubt between:
1. moving faster through tactical shortcuts
2. preserving the runtime's long-term identity

choose identity.

If there is ever a doubt between:
1. another micro-adjustment
2. closing a real macrostep chunk

choose the macrostep chunk.
