# SLM Strategic Revision

Status: active doctrine for macrostep `6. Real SLM Integration`

Primary references:
- `docs/vademecum.md`
- `docs/project-context.md`
- `docs/archive/intelligence-failure-classes.md`

## Purpose

This document redefines how Needle-X should use SLMs if the goal is not merely to add local models, but to build a product that feels category-breaking.

The core thesis is strict:

Needle-X does not become disruptive by "having local SLMs".
Needle-X becomes disruptive only if it uses small models in ways that conventional readers, scrapers, and search wrappers do not.

## Strategic Position

Needle-X must not become:
1. a scraper with a local LLM bolt-on
2. a prettier wrapper around Ollama
3. a conventional RAG preprocessor with better tracing

Needle-X should become:
1. a local-first web context compiler
2. an evidence-native runtime where models operate on structured uncertainty, not raw pages
3. a system that uses SLMs as constrained micro-solvers inside a deterministic substrate

If the future product can be summarized as "we use Qwen locally", the strategy has failed.

## What Is Actually Disruptive

The disruptive move is not:
1. `HTML -> model -> text`

The disruptive move is:
1. `HTML -> WebIR`
2. `WebIR -> evidence graph`
3. `evidence graph -> policy-selected ambiguity payload`
4. `ambiguity payload -> typed SLM delta`
5. `typed delta -> proof-carrying context pack`

The product advantage comes from:
1. lower inference surface
2. stronger auditability
3. lower token waste
4. deterministic replay around model intervention
5. model specialization by failure class

## Non-Negotiable SLM Rules

These rules are stricter than "best practice". They are identity constraints.

1. Models do not read raw pages first.
2. Models do not produce the final product payload directly.
3. Models do not emit unconstrained prose as runtime output.
4. Models must return typed, schema-validated deltas.
5. Every model call must have a named failure class or policy reason.
6. Every model call must be replay-visible and proof-visible.
7. Deterministic fallback must remain usable when all model backends are disabled.

If any future integration violates one of these, it is strategically wrong even if benchmark quality rises.

## The Right SLM Roles

The system should not think of "one model" or "a general assistant".

It should think in terms of very narrow solver roles.

### 1. Ambiguity Resolver

Input:
1. conflicting segment candidates
2. objective terms
3. local IR evidence
4. trace risk flags

Output:
1. chosen candidate ids
2. rejection rationale codes
3. confidence score

This is not summarization. This is evidence arbitration.

### 2. Embedded State Interpreter

Input:
1. extracted JSON blobs
2. embedded script fragments
3. nearby heading/section context

Output:
1. normalized structured fields
2. typed evidence patch
3. uncertainty tags

This is one of the most important non-obvious roles because many modern sites hide the useful payload in app state, not visible text.

### 3. Selector Drift Diagnostician

Input:
1. prior trace
2. current WebIR
3. retained/removed fingerprint graph deltas
4. failed selector/evidence pairs

Output:
1. failure diagnosis class
2. likely drift region
3. retry/reduce/render recommendation

This is strategically stronger than a generic extractor because it turns models into maintenance intelligence for the runtime itself.

### 4. Evidence Graph Repairer

Input:
1. fragmented segments
2. low-confidence heading relationships
3. embedded + visible content disagreement

Output:
1. merge/split decisions
2. graph edge patch
3. provenance retention map

This is the role where Needle-X can move beyond "main content extraction" and toward compiler-like structural repair.

### 5. Compression Guardian

Input:
1. candidate compact pack
2. expected objective terms
3. proof/risk metadata

Output:
1. safe removals
2. protected facts
3. lossiness warnings

This is a non-standard role that can make `tiny` mode substantially more powerful without turning compression into silent damage.

## The Most Unconventional Opportunity

The most disruptive use of SLMs is not better extraction.
It is making the model operate as a `typed patch engine` on Needle-X internal representations.

This means:
1. the model never "answers the user"
2. the model proposes patches to `WebIR`, evidence graph, or chunk selection
3. the deterministic runtime validates, accepts, or rejects those patches

This is the key market distinction.

Most systems do:
1. unstructured model generation
2. weak post-processing

Needle-X should do:
1. deterministic state construction
2. very small, high-value model intervention
3. typed patch validation
4. proof-carrying merge into final output

That is a very different product category.

## Product Thesis That Feels "Two Generations Ahead"

Needle-X should aim to be recognized as:

`the first local web context compiler where models patch structured uncertainty instead of generating answers from raw pages`

That claim is strong, coherent, and defensible if the runtime actually behaves that way.

## Architecture Revision For Macrostep 6

Macrostep 6 should not be executed as "add Ollama support".

It should be executed as five specific blocks.

### Block A: Single Adapter Boundary

Deliver:
1. one `ModelRuntime` interface
2. one request/response schema shared by all backends
3. one place where token/latency/model identity is recorded

Constraint:
No backend-specific logic may leak into pack/judge/transport.

### Block B: Typed Task Layer

Deliver:
1. `ResolveAmbiguity`
2. `InterpretEmbeddedState`
3. `DiagnoseDrift`
4. `RepairEvidenceGraph`
5. `GuardCompression`

Constraint:
Each task has its own JSON schema and validator.

### Block C: Policy Engine Expansion

Deliver:
1. failure-class to task mapping
2. domain-genome overrides for allowed tasks
3. explicit budget ceilings per task
4. deterministic fallback route per task

Constraint:
No generic "ask model for help" path is allowed.

### Block D: Patch Validation Layer

Deliver:
1. typed patch validators
2. confidence gating
3. contradiction detection against source spans and graph state
4. reject/accept/partial-merge policy

Constraint:
The runtime owns truth; the model proposes deltas.

### Block E: Real Benchmark Closure

Deliver:
1. backend A/B against deterministic baseline
2. per-task win-rate measurement
3. token and latency cost measurement
4. failure-class specific dashboards

Constraint:
A model backend that does not win on a named task should not stay in the product path.

## Model Selection Strategy

Model choice must follow task roles, not hype.

Use a portfolio approach.

### Class 1: Ultra-small CPU Router

Purpose:
1. cheap task classification
2. ambiguity/failure routing
3. patch admissibility hints

Desired profile:
1. `0.5B` to `2B`
2. strong CPU viability
3. fast JSON-formatted output

Examples to evaluate:
1. `Qwen3.5-0.8B`
2. `Qwen3.5-2B`
3. `Llama 3.2 1B/3B` if JSON discipline is competitive

### Class 2: Mid-size Structured Solver

Purpose:
1. ambiguity resolution
2. embedded-state interpretation
3. graph repair

Desired profile:
1. `4B` to `14B`
2. reliable schema following
3. strong long-context behavior on structured inputs

Examples to evaluate:
1. `Qwen3.5-4B`
2. `Qwen3.5-9B`
3. `Phi-4`
4. `Gemma 3 4B/12B`

### Class 3: Specialist Reader

Purpose:
1. HTML/reader-specific comparison path
2. stress-test against specialized extraction models

Examples to evaluate:
1. `ReaderLM-v2`

Important:
Specialist readers should be treated as task baselines or specialist modules, not default truth sources.

## What The Models Should Never Do

Needle-X should never let real SLMs:
1. rewrite the entire page into prose as a primary path
2. replace proof generation
3. bypass source span grounding
4. produce final chunks without deterministic validation
5. silently merge contradictory evidence

The moment this happens, Needle-X stops being a compiler and starts becoming a flavored reader.

## Market-Level Differentiators To Pursue

If the goal is market disruption, these are the differentiators worth building toward.

1. `Patch-level inference`
   Models propose structural deltas, not free-form answers.

2. `Delta-aware web memory`
   Model intervention is conditioned by prior runs and only activates on changed/uncertain regions.

3. `Proof-carrying SLM intervention`
   Each accepted patch is stored with origin, reason, confidence, and rejection alternatives.

4. `Domain-native runtime adaptation`
   Different sites accumulate local handling memory without becoming hand-coded scrapers.

5. `Low-token operational context`
   The final product is not verbose extraction. It is compact operational memory for agents.

This combination is much rarer than "local model + browser".

## Decision Test For Future SLM Work

Before implementing any real model feature, answer these questions:

1. Does this make the model operate on structured uncertainty instead of raw page text?
2. Does it produce a typed patch or a typed decision?
3. Is there a deterministic validator after the model call?
4. Is the task linked to a named failure class?
5. Can the task be benchmarked independently?
6. Would this still feel novel if the model name were removed from the demo?

If question 6 is weak, the feature is probably not strategically strong enough.

## Immediate Consequence

The next implementation burst should not be:
1. integrate Ollama
2. wire generic prompts
3. expose a `--model` flag and call it progress

The next implementation burst should be:
1. define the adapter boundary
2. define typed task contracts
3. define patch schemas
4. define validation and rejection logic
5. only then integrate the first backend

That order preserves the philosophy and keeps the project on the "two generations ahead" line.
