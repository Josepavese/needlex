# AGENTS.md

This file codifies the durable engineering philosophy and operating rules for Needle-X.

It is not a release note and not a user guide.
It exists to preserve the project's intent as the codebase evolves.

## Mission

Needle-X is a production retrieval tool for AI agents.

Its job is not to look plausible.
Its job is to return the right source, page, or evidence path reliably enough that an agent can complete real work.

Two product surfaces are first-class:
1. seedless discovery
2. MCP usability for agents

## Core Philosophy

### Semantic-first, not lexical-first

The system must prefer:
- context
- meaning
- semantic relatedness
- entity/family coherence
- structural evidence

The system must avoid depending on:
- monolingual keyword heuristics
- brittle literal token overlap
- exact surface-form preservation as a primary retrieval strategy
- ad hoc provider-specific hacks

Literal and lexical signals may exist as weak legacy hints, but they must not be the main decision surface.

### Multilingual by design

Needle-X is not an English-only system.

Discovery and ranking should work across languages and naming variations.
When a design choice trades semantic generality for monolingual literal convenience, prefer semantic generality.

### Context over string matching

Queries, candidates, and pages should be interpreted through:
- semantic grounding
- family/host relationships
- resource class
- cluster structure
- evidence provenance

Do not optimize around “matching the right word”.
Optimize around identifying the right entity, family, document, or endpoint.

## Discovery Principles

### Seedless discovery is a first-class product concern

Seedless retrieval is not a side experiment.
It is a primary runtime surface and must be treated as such in design, benchmarking, and regression analysis.

Changes to discovery should be evaluated for:
1. pass rate
2. latency
3. stability across runs
4. failure mode mix

### Prefer semantic grounding before aggressive probing

When bootstrap candidates are noisy, use semantic reranking and candidate intelligence early enough to affect probe ordering.

Do not rely on lexical overlap to decide what deserves probing first.

### Entity/family recovery beats keyword expansion

For hard cases, recover the right entity or official family through:
- semantic clustering
- candidate family modeling
- representative selection
- contextual evidence propagation

Do not fall back to monolingual “official/auth/docs/pricing” style heuristics as the main mechanism.

### Annotate, do not prematurely discard

Resource classes and host classes should be used as contextual signals.
Avoid global hard drops that can remove valid targets such as CSS, images, JSON, raw text, or files.

## Fetch and Reliability Principles

### Reliability matters; evasion is not the design goal

Needle-X should maximize lawful real-world reliability using:
- provider diversity
- pacing
- backoff
- jitter
- cooldowns
- provider health memory
- caching
- robust fallback order

Do not build the project around stealth or anti-bot evasion claims.
The correct direction is production-grade resilience, not “guaranteed undetectability”.

### Measure failures precisely

Do not collapse all failures into “search is bad”.
Distinguish at least:
- ranking_miss
- provider_blocked
- benchmark_timeout
- unsupported_content_type
- empty_candidates
- unavailable_upstream

Interpret benchmark results through this taxonomy before changing ranking logic.

## MCP and Agent UX Principles

### MCP is an agent-facing product surface

If an AI agent predictably misuses a tool, that is usually a tool UX problem first, not an agent problem first.

Tool schemas, examples, errors, and help must actively guide correct usage.

### Compact-first outputs

For agent-facing MCP responses:
- `content.text` should present the compact, immediately useful summary first
- rich diagnostics may remain in structured payloads

Do not force agents to parse a giant diagnostic blob before they can see the useful result.

### Be explicit about strict modes

Strict options such as `discovery_mode=off` must be documented as strict.

If a strict mode requires:
- an exact canonical page
- a verified seed URL
- no discovery expansion

that requirement must be stated in:
- schema descriptions
- examples
- error messages

## Rewrite and Semantic Escalation Principles

### Rewrite is for semantic retrieval, not surface-form copying

Query rewriting should preserve intent and entity identity semantically.
It should not require verbatim repetition of a canonical entity string in every rewrite.

If rewrite escalation happens, it should be because the current leader is not semantically grounded, not because a lexical overlap threshold failed.

### Semantic signals must be durable and inspectable

When semantic reranking influences a candidate, preserve enough metadata and reasons to make that influence inspectable in code and tests.

## Benchmarking Principles

### Benchmarks must not lie

A benchmark with unrealistic timeout budgets is misleading.
If a profile is cut off artificially, that is a benchmark design problem before it is a ranking conclusion.

### Multi-run evaluation beats single noisy runs

For noisy providers and seedless discovery, prefer:
- multiple runs
- majority/median interpretation
- per-profile failure taxonomy

Do not overfit the system to a single volatile run.

### Artifact discipline

Benchmark JSON outputs under `improvements/` are local analysis artifacts unless explicitly intended for commit.
Do not commit them by accident.

## Release and Process Discipline

### Release workflow is mandatory

When preparing a public release, follow:
- [release-workflow.md](.agent/workflows/release-workflow.md)

This includes:
- full test suite
- budget check
- local install validation
- CLI smoke tests
- MCP smoke tests
- release asset verification

### Public behavior changes require release discipline

If a change affects:
- installed CLI behavior
- MCP transport or tool UX
- seedless discovery behavior
- installer/runtime behavior

do not assume `main` is enough.
The installed public channel only changes once a proper release is published.

## Code Change Heuristics

When making changes, prefer:
1. semantic/contextual models over lexical rules
2. structural/contextual annotations over hard exclusions
3. narrow, testable improvements over sprawling heuristics
4. explicit diagnostics over hidden magic

Avoid:
1. provider-name hacks
2. single-case patches disguised as general logic
3. monolingual keyword filters as primary ranking logic
4. benchmark conclusions drawn from unstable runs without taxonomy

## Practical Checklist For Future Contributors

Before landing a discovery change, ask:
1. Did this move the system toward semantics and context, or back toward lexical hacks?
2. Does this remain multilingual in principle?
3. Is the effect measurable through tests or benchmarks?
4. Does it degrade another product surface such as seedless, MCP, or install/runtime behavior?
5. Is the new behavior understandable from metadata, reasons, and tests?

If the answer is weak on those points, the change is probably not mature enough.
