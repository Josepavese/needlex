# Agentic Decision Plane Spec

Status: Draft for later phase
Owner: Needle-X core doctrine
Phase: Post-wave benchmark / future macrostep

Related docs:
- [project-context.md](/home/jose/hpdev/Libraries/needlex/docs/project-context.md)
- [vademecum.md](/home/jose/hpdev/Libraries/needlex/docs/vademecum.md)
- [slm-strategic-revision.md](/home/jose/hpdev/Libraries/needlex/docs/archive/slm-strategic-revision.md)
- [slm-execution-plan.md](/home/jose/hpdev/Libraries/needlex/docs/archive/slm-execution-plan.md)
- [first-real-slm-test.md](/home/jose/hpdev/Libraries/needlex/docs/archive/first-real-slm-test.md)
- [spec.md](/home/jose/hpdev/Libraries/needlex/spec.md)

## Purpose

This document defines a future Needle-X subsystem: the `Agentic Decision Plane`.

It is not a plan to turn Needle-X into a generic agent framework.
It is a plan to introduce bounded, typed, local micro-agency inside a deterministic web context compiler.

The goal is to create a new class of runtime behavior:
1. deterministic by default
2. locally stateful
3. proof-carrying
4. selectively agentic only where uncertainty is structurally high

This is intended as a later-phase capability, after the current SLM benchmark and acceptance work has stabilized.

## Strategic Thesis

Needle-X should not compete as:
1. a browser agent
2. a search wrapper with LLM post-processing
3. a scraper with a local model attached

Needle-X should compete as:
1. a local-first web context compiler
2. a proof-carrying retrieval runtime
3. a deterministic system with bounded decision-time intelligence

The `Agentic Decision Plane` is the layer that gives Needle-X controlled agency without sacrificing auditability.

## Core Idea

The system should not give a model control over the whole workflow.
It should ask a model to solve one narrow decision at a time, on top of structured runtime state.

The canonical pattern is:

`runtime state -> bounded micro-task -> typed decision or patch -> deterministic validation -> apply or reject -> proof`

This means:
1. no free-form browsing
2. no open-ended planning loop
3. no direct final-answer generation from raw HTML
4. no silent model authority over runtime outcome

## Non-Negotiable Rules

1. The deterministic path remains the primary path.
2. The model never owns the end-to-end loop.
3. The model should read `WebIR`, pack state, chunk sets, evidence graphs, drift deltas, or embedded summaries before it ever reads raw page state.
4. Every output must be schema-locked.
5. Every output must be groundable against runtime evidence.
6. Every output may be rejected.
7. The proof surface must expose why a micro-agent was called, what it proposed, and what was accepted or rejected.
8. Latency and token budgets are part of the contract, not post-hoc concerns.

## Why This Exists

Conventional systems usually choose one of two bad extremes:
1. deterministic only, which breaks on hard ambiguity and drift
2. LLM-first, which destroys auditability and cost discipline

Needle-X should occupy the third position:
1. deterministic substrate for most work
2. small local agentic interventions for hard uncertainty
3. full proof and validation around every intervention

This is the market-disruptive position.

## Scope

### In Scope

1. bounded decision tasks
2. typed repair tasks
3. drift explanation tasks
4. query decomposition tasks
5. budget-aware activation policy
6. deterministic validation and proof integration

### Out Of Scope

1. autonomous browsing loops
2. browser control as a general runtime default
3. arbitrary tool-using agent chains
4. open-ended web exploration
5. direct model authorship of final read/query output

## Product-Level Outcomes

If implemented correctly, the `Agentic Decision Plane` should allow Needle-X to:
1. reduce ambiguity failures without making the whole runtime opaque
2. diagnose why a site changed instead of only failing on the new version
3. propose typed repairs instead of overfitting heuristics directly into the core path
4. decompose complex multi-page goals into bounded read units
5. preserve low-token, proof-carrying output while increasing robustness

## Architectural Position

The future architecture should be understood as four layers.

### Layer 1. Deterministic Runtime

This layer remains primary.
It includes:
1. acquire
2. reduce
3. WebIR
4. segment
5. evidence extraction
6. pack
7. proof
8. trace
9. domain genome
10. local state and replay

### Layer 2. Agentic Decision Plane

This is the new layer defined by this document.
It includes:
1. bounded task router
2. task-specific micro-agents
3. typed outputs
4. budget-aware activation policy

### Layer 3. Validation Plane

This layer protects the runtime from uncontrolled model behavior.
It includes:
1. schema validation
2. grounding validation
3. risk validation
4. delta validation
5. acceptance or rejection recording

### Layer 4. Memory And Replay Plane

This layer turns one-off model decisions into inspectable long-lived system knowledge.
It includes:
1. previous proofs
2. drift comparison
3. failure memory
4. per-domain intervention history
5. patch acceptance history

## Canonical Micro-Agent Tasks

The first implementation wave should focus on a small set of high-leverage tasks.

### 1. Ambiguity Arbiter

Goal:
Choose the best evidence set when deterministic scoring yields multiple plausible alternatives.

Input:
1. `objective`
2. `page_type`
3. `lane`
4. `candidate_clusters[]`
5. `chunk_snippets[]`
6. `web_ir_summary`
7. `risk_flags[]`
8. `existing_pack_summary`

Output:
1. `selected_cluster_id`
2. `rejected_cluster_ids[]`
3. `confidence`
4. `reason_code`
5. `notes`

When to activate:
1. chunk score gap is below threshold
2. multiple candidate clusters compete for the same objective
3. selection risk is medium or high

What it must never do:
1. invent new evidence
2. write final output text
3. re-read the full page freely

Expected value:
1. better arbitration on forum-like pages
2. better handling of hero/content ambiguity
3. less heuristic proliferation in selection logic

### 2. Embedded Worthiness Judge

Goal:
Decide whether interpreting embedded state is worth the latency and risk for this page.

Input:
1. `page_type`
2. `visible_signal_density`
3. `visible_context_alignment`
4. `embedded_paths[]`
5. `embedded_summaries[]`
6. `script_counts`
7. `web_ir_summary`
8. `existing_pack_summary`

Output:
1. `should_interpret_embedded`
2. `priority_paths[]`
3. `expected_signal_type`
4. `budget_class`
5. `reason_code`

When to activate:
1. app-shell or JS-heavy page shape
2. visible content appears shallow or fragmented
3. embedded payloads appear rich and semantically promising

What it must never do:
1. parse the whole embedded payload itself as final extraction
2. replace the extractor
3. trigger unbounded escalation

Expected value:
1. fewer useless embedded calls
2. lower CPU latency
3. more selective activation of `interpret_embedded_state`

### 3. Drift Diagnostician

Goal:
Explain why a previously stable extraction path changed.

Input:
1. `previous_proof_summary`
2. `current_web_ir_summary`
3. `previous_selected_paths[]`
4. `current_candidate_paths[]`
5. `diff_markers[]`
6. `pack_regression_summary`
7. `domain_genome_summary`

Output:
1. `drift_class`
2. `probable_cause`
3. `recommended_repair_type`
4. `confidence`
5. `notes`

Canonical drift classes:
1. `hero_expansion`
2. `embedded_migration`
3. `navigation_dominance`
4. `selector_decay`
5. `content_fragmentation`
6. `template_shift`

When to activate:
1. previously stable fingerprint regions disappear from pack
2. pack regression is detected on repeated reads
3. deterministic replay diverges beyond tolerance

Expected value:
1. better repeated-site resilience
2. more interpretable drift handling
3. foundation for typed repairs instead of heuristic sprawl

### 4. Pack Repair Proposer

Goal:
Propose a minimal typed patch to pack policy when deterministic output is structurally wrong.

Input:
1. `current_pack_summary`
2. `rejected_alternatives[]`
3. `risk_flags[]`
4. `objective`
5. `web_ir_summary`
6. `drift_diagnosis`

Output:
1. `repair_type`
2. `repair_patch`
3. `expected_effect`
4. `confidence`
5. `reason_code`

Canonical repair types:
1. `prefer_heading_family`
2. `demote_cta_cluster`
3. `expand_body_merge`
4. `preserve_embedded_section`
5. `relax_short_text_filter`
6. `tighten_noise_filter`

When to activate:
1. deterministic pack is near-correct but consistently excludes the wrong material
2. drift diagnosis suggests a local policy change
3. a bounded repair exists without changing core architecture

What it must never do:
1. rewrite the whole result pack from scratch
2. bypass deterministic validation
3. create opaque per-domain hacks without proof

### 5. Query Decomposer

Goal:
Split a complex user goal into bounded Needle-native read units.

Input:
1. `user_goal`
2. `seed_url_optional`
3. `candidate_pages[]`
4. `domain_graph_summary`
5. `local_candidate_memory_summary`
6. `budget_class`

Output:
1. `read_units[]`
2. `priority_order[]`
3. `per_unit_objective[]`
4. `per_unit_lane_suggestion[]`
5. `stop_condition`

When to activate:
1. user intent is clearly multi-faceted
2. single-page reads are unlikely to satisfy the goal
3. enough local or discovered candidate pages already exist

Expected value:
1. stronger multi-page retrieval planning
2. less brute-force crawl behavior
3. more explicit objective decomposition

## Task Schema Requirements

Each micro-agent task must define:
1. strict task name
2. versioned input schema
3. versioned output schema
4. max input token class
5. max output token class
6. validator contract
7. acceptance policy
8. proof mapping

### Required Output Envelope

Every micro-agent output should fit a shared envelope like:

```json
{
  "task": "ambiguity_arbiter",
  "version": "v1",
  "decision_id": "dec_...",
  "confidence": 0.0,
  "reason_code": "...",
  "payload": {},
  "notes": []
}
```

The `payload` is task-specific.

## Validation Plane Requirements

Every task must pass at least five validation layers.

### 1. Schema Validation

The payload must be valid JSON and match the task schema exactly.

### 2. Grounding Validation

All references in the payload must map to real runtime entities:
1. chunk IDs must exist
2. embedded paths must exist
3. cluster IDs must exist
4. repair targets must be valid

### 3. Budget Validation

The output must not imply a larger action than the route allowed.

Examples:
1. a `worth_it=false` decision cannot trigger the extractor
2. a lane-1 arbiter cannot emit a lane-3 repair

### 4. Risk Validation

The proposed action must be checked against:
1. lossiness risk
2. contamination risk
3. contradiction risk
4. token inflation risk

### 5. Delta Validation

The proposed change must improve or safely preserve the current pack state.

If no safe improvement is proven, the runtime should reject the decision.

## Acceptance Semantics

Each micro-agent invocation must end in one of these outcomes:
1. `accepted`
2. `selection_rejected`
3. `validation_rejected`
4. `runtime_error`
5. `budget_rejected`
6. `no_op`

These outcomes must be visible in:
1. proof
2. trace
3. benchmark artifacts

## Activation Policy

The `Agentic Decision Plane` must never activate by taste.
It must activate by policy.

### Shared Activation Conditions

A micro-agent call is allowed only if all are true:
1. deterministic path has already produced a state snapshot
2. an uncertainty condition is present
3. the budget class allows the task
4. the task is benchmark-backed for this route
5. the transport path can surface proof of the intervention

### Shared Blocking Conditions

A micro-agent call is blocked if any are true:
1. deterministic output is already strong enough
2. latency budget is too tight
3. task acceptance rate is below rollout threshold
4. the page or query type is outside the task's tested family
5. the same task already failed recently on this domain and no new evidence exists

## Budget Classes

The decision plane should classify tasks by cost.

### Class A. Micro Decision

Examples:
1. ambiguity arbitration
2. worthiness judgment

Constraints:
1. very small context window
2. short output
3. low latency target

### Class B. Structured Analysis

Examples:
1. drift diagnosis
2. bounded query decomposition

Constraints:
1. medium context
2. structured output
3. moderate latency target

### Class C. Typed Repair

Examples:
1. pack repair proposal
2. graph repair proposal

Constraints:
1. restricted to rare cases
2. higher review burden
3. stronger validator requirements

## Proof Requirements

Every agentic decision must extend the existing proof system with:
1. `task`
2. `decision_id`
3. `input_class`
4. `backend`
5. `latency_ms`
6. `validator_outcome`
7. `patch_effect`
8. `affected_entities[]`
9. `reason_code`
10. `rejection_cause_optional`

This ensures the decision plane never becomes a black box.

## Benchmark Requirements

No micro-agent task may graduate to default policy without passing both controlled and live benchmarks.

### Controlled Benchmarks

The task must show measurable value in versioned corpora.

Metrics should include:
1. pass rate
2. objective lift
3. fidelity preservation
4. backend acceptance rate
5. latency overhead
6. runtime error rate

### Live Benchmarks

The task must be tested on real sites from multiple families.

Families should include:
1. corporate brochure
2. docs or reference
3. app-shell or JS-heavy
4. forum or support-like
5. editorial or mixed longform

### Promotion Rule

A task is promoted only if:
1. it improves a real failure family
2. it does not explode latency or runtime errors
3. its accepted decisions are meaningfully non-zero
4. it remains explainable in proof

## Rollout Order

This future phase should be implemented in this order.

### Wave 1. Decision Tasks

1. `embedded_worthiness_judge`
2. `ambiguity_arbiter`

Reason:
They are cheap, bounded, and immediately useful.

### Wave 2. Diagnosis Tasks

1. `drift_diagnostician`
2. `query_decomposer`

Reason:
They increase system intelligence without handing over extraction authority.

### Wave 3. Repair Tasks

1. `pack_repair_proposer`

Reason:
Repairs are powerful but riskier.
They should come only after strong validators exist.

## Data Structures To Add Later

These are suggested future runtime types.

### `DecisionState`

A compact runtime snapshot prepared for a micro-agent.

Fields should include:
1. task name
2. route class
3. objective
4. web_ir summary
5. candidate entities
6. current pack summary
7. risk summary
8. budget summary

### `DecisionResult`

Canonical model output plus validation metadata.

Fields should include:
1. task
2. payload
3. confidence
4. validator outcome
5. patch effect
6. latency
7. backend

### `DecisionHistory`

Per-domain memory of prior decisions.

Fields should include:
1. task family
2. domain
3. success or rejection rate
4. last drift class
5. last accepted repair type

## Failure Modes To Avoid

1. adding a general agent loop because it feels powerful
2. letting micro-agents read too much raw content
3. introducing free-form text outputs that bypass validators
4. treating model suggestions as truth instead of proposals
5. growing a patch zoo with no benchmark proof
6. using agentic logic to hide deterministic weaknesses instead of fixing them

## Relationship To Current Macrostep

This document is intentionally future-oriented.

It does not change the current execution priority.
Current priority remains:
1. CPU benchmark closure
2. candidate selection for current SLM roles
3. acceptance improvement on existing typed tasks

Only after the current model-selection phase stabilizes should this decision-plane expansion start.

## Exit Criteria For This Future Phase

The `Agentic Decision Plane` can be considered materially real only when all are true:
1. at least two micro-agent tasks are active in production policy
2. both tasks have non-trivial accepted decision rates
3. both tasks improve benchmark results on their target families
4. proof shows accepted and rejected decisions clearly
5. deterministic fallback remains strong when the plane is disabled
6. the system still looks like a compiler with bounded intelligence, not an agent wrapper

## Summary

The purpose of this spec is not to make Needle-X more fashionable.
It is to make Needle-X more structurally powerful.

The correct endpoint is:
1. deterministic substrate first
2. local bounded agency second
3. proof and validation always

If built this way, Needle-X gains something most systems do not have:
controlled intelligence without surrendering the runtime.
