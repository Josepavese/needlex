# AGENTS.md

This file codifies the durable engineering philosophy and operating rules for Needle-X.

It is not a release note and not a user guide.
It exists to preserve the project's intent as the codebase evolves.

## Mission

Needle-X is a production retrieval tool for AI agents.

Its job is not to look plausible.
Its job is to return the right source, page, or evidence path reliably enough that an agent can complete real work.

Two product surfaces are first-class:
1. compact proof-carrying compilation of agent-selected URLs
2. MCP usability for agents

## Core Philosophy

### Embeddings-first, semantic-first

The system must prefer:
- multilingual embedding similarity
- context
- meaning
- semantic relatedness
- entity/family coherence
- structural evidence

The system must avoid depending on:
- monolingual search-term heuristics
- surface-form overlap as a retrieval strategy
- exact surface-form preservation as a primary retrieval strategy
- ad hoc provider-specific hacks

Surface-form matching must not be used as a ranking surface. If text affects ranking, it must pass through semantic/embedding alignment. URL and protocol strings may be parsed only as identity, provenance, or transport constraints.

### Multilingual by design

Needle-X is not an English-only system.

Discovery and ranking should work across languages and naming variations.
When a design choice trades semantic generality for monolingual surface-form convenience, prefer semantic generality.

### Context over string matching

Queries, candidates, and pages should be interpreted through:
- semantic grounding
- family/host relationships
- resource class
- cluster structure
- evidence provenance

Do not optimize around “matching the right word”.
Optimize around identifying the right entity, family, document, or endpoint.

### No simulated semantics

A semantic ranking decision requires dense embedding vectors.

Needle-X has no valid production mode without dense embeddings.
The installed runtime must read a PAL-home SSOT config that points to a local/no-key embedding endpoint and a durable vector-space identity.
If semantic config is missing or disabled, operational commands must fail rather than silently degrading into lexical retrieval.
Do not replace missing embeddings with character n-grams, token overlap, edit distance, language-specific word lists, or any other surface-similarity substitute.

## Discovery Principles

### Seedless discovery is experimental

Stable agent workflows must obtain candidate URLs through the host agent's search tool and use Needle-X to compile every URL the agent chooses to analyze.
Needle-X must not prescribe a candidate count; research breadth belongs to the agent.

Seedless retrieval remains an explicit experimental runtime surface.
It must never be selected implicitly, advertised in the stable skill or README, or presented as a production reliability claim.
Keep its implementation measurable and inspectable so it can improve without contaminating the stable contract.

Changes to discovery should be evaluated for:
1. pass rate
2. latency
3. stability across runs
4. failure mode mix

### Prefer semantic grounding before aggressive probing

When bootstrap candidates are noisy, use semantic reranking and candidate intelligence early enough to affect probe ordering.

Do not rely on surface-form overlap to decide what deserves probing first.

### Entity/family recovery beats search-term expansion

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
- render_degraded
- network_body_missing
- network_truncated

Interpret benchmark results through this taxonomy before changing ranking logic.

## Render and Application-Data Delivery Principles

### Rendering exists to deliver application data, not only the DOM

JavaScript rendering is the last escalation step, and it is not satisfied by a DOM dump.
Its purpose is to materialize the content an application builds after load: `fetch`/XHR bodies, SSE messages, received WebSocket frames, and other textual application payloads.

A render that produces a DOM but drops the application data has not delivered the page.

### Budgets must follow the transport, not a constant

Render waiting is bounded by `render.timeout_ms` and by the remaining operation deadline, never by a fixed cap that makes the documented target case structurally unreachable.

Termination is driven by observed activity:
- a quiet page stops as soon as the page and its streams go idle
- a page with active application requests or open streams keeps waiting while they produce
- a stream cut while still open is reported as truncated, with the reason, instead of being presented as a complete capture

If a budget is ever reached, the result must say so.

### Degradation is reported, never silent

A fallback is a measured event, not an invisible one.

When the capture path degrades, the run must record it:
- the render path that actually ran (`cdp` versus `dump_dom`)
- how many relevant resources were observed, retained, and unreadable
- whether streams were still open at snapshot time
- why waiting stopped

Reporting degradation is part of the product contract because an agent that cannot tell “no data exists” from “data was not captured” will make a wrong decision.

### Evidence hygiene at the transport boundary

Transport-level decoding such as gzip unwrapping is allowed and expected at the capture boundary; it recovers application data that is otherwise unreadable.

Payloads that remain non-textual after decoding must not enter semantic evidence. This is an evidence-scoped exclusion, not a global resource hard drop: the resource stays observed and reported, it simply does not become text.

### Render escalation is semantic, not only structural

Render escalation triggers on structural weakness (thin, navigation-like, or client-rendered surface) and on semantic grounding: when the static surface does not cover the objective, the objective-to-surface similarity decides.

Calibrated thresholds must be measured against the local embedding runtime, documented with the samples behind them, and recorded per run so they can be re-tuned from field data instead of intuition.

Non-HTML responses are never rendered in `auto` mode. There is no JavaScript to run in them, and paying a browser launch for a JSON, CSS, or plain-text response is waste.

## MCP and Agent UX Principles

### MCP is an agent-facing product surface

If an AI agent predictably misuses a tool, that is usually a tool UX problem first, not an agent problem first.

Tool schemas, examples, errors, and help must actively guide correct usage.

### Compact-first outputs

For agent-facing MCP responses:
- `content.text` should present the compact, immediately useful summary first
- rich diagnostics may remain in structured payloads

Do not force agents to parse a giant diagnostic blob before they can see the useful result.

Compact output must still answer where the content came from.
For reads that involved rendering, the packet reports the content source (rendered DOM, or rendered DOM plus captured application data) and whether the capture was truncated or degraded, so an agent can trust the result or escalate without opening full diagnostics.

### Installed agent guidance must be able to go stale visibly

Agent skills and similar guidance are installed as snapshots. They do not refresh themselves, and a host agent cannot see that its local copy documents an older contract than the running binary.

Therefore:
- shipped agent guidance declares the release version it documents
- `needlex doctor` reports the installed guidance version against the running build and reports drift explicitly
- the installer refreshes guidance it previously installed, while backing up the previous copy and restoring it if the refresh fails

Silent drift is the failure mode to avoid: an agent following an outdated contract is indistinguishable from an agent misusing the tool unless the drift is reported.

### Be explicit about strict modes

Strict options such as `discovery_mode=off` and `render=required` must be documented as strict.

If a strict mode requires:
- an exact canonical page
- a verified seed URL
- no discovery expansion
- a browser read even for non-HTML content

that requirement must be stated in:
- schema descriptions
- examples
- error messages

## Rewrite and Semantic Escalation Principles

### Rewrite is for semantic retrieval, not surface-form copying

Query rewriting should preserve intent and entity identity semantically.
It should not require verbatim repetition of a canonical entity string in every rewrite.

If rewrite escalation happens, it should be because the current leader is not semantically grounded, not because a surface-form overlap threshold failed.

### Semantic signals must be durable and inspectable

When semantic reranking influences a candidate, preserve enough metadata and reasons to make that influence inspectable in code and tests.

The same rule applies to render escalation: when an objective-to-surface similarity decides that a page must be rendered, that similarity value and its reason code belong in the run record.

## Benchmarking Principles

### Benchmarks must not lie

A benchmark with unrealistic timeout budgets is misleading.
If a profile is cut off artificially, that is a benchmark design problem before it is a ranking conclusion.

### Multi-run evaluation beats single noisy runs

For noisy providers and experimental seedless discovery, require:
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
- shipped agent guidance aligned to the released version

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
1. semantic/contextual models over surface-form rules
2. structural/contextual annotations over hard exclusions
3. narrow, testable improvements over sprawling heuristics
4. explicit diagnostics over hidden magic
5. budgets derived from measured transport reality over fixed constants

Avoid:
1. provider-name hacks
2. single-case patches disguised as general logic
3. monolingual search-term filters as primary ranking logic
4. benchmark conclusions drawn from unstable runs without taxonomy
5. silent fallbacks that trade captured data for a green status
6. leaving a shipped contract and its installed guidance version out of sync

## Practical Checklist For Future Contributors

Before landing a change, ask:
1. Did this move the system toward semantics and context, or back toward surface-form hacks?
2. Does this remain multilingual in principle?
3. Is the effect measurable through tests or benchmarks?
4. Does it degrade another product surface such as known-URL compilation, seeded routing, MCP, render delivery, or install/runtime behavior?
5. Is the new behavior understandable from metadata, reasons, and tests?
6. If the change touches capture or waiting, can an operator tell from the run record what was captured, what was lost, and why?
7. If the change narrows a budget, was the previous value derived from a measurement rather than a guess?
8. If the change alters a public contract, does shipped agent guidance carry it and can installed copies be detected as stale?

If the answer is weak on those points, the change is probably not mature enough.
