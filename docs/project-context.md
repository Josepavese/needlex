# Project Context

Primary execution doctrine: see `docs/vademecum.md`.

This is the living context document for Needle-X.

Use it to answer four questions fast:
1. What Needle-X is.
2. What is already real in the repo.
3. What is still missing.
4. What the next engineering moves should be.

## In Simple Terms

Needle-X is not a search engine yet.

Today it is:
1. a local-first web context runtime
2. a deterministic extraction and compression engine
3. a proof-carrying retrieval layer for agents

It reads web pages, removes noise, selects the useful parts, compresses them, and produces traceable output with proof, replay, and diff support.

The strategic point is this:
Needle-X is not trying to win by indexing more pages or wrapping third-party search.
It is trying to win by producing higher-fidelity, auditable context packs with lower token cost and better debugging.

## Product Shape Today

The current product is real, but it is still a technical product rather than the final market-facing product.

What exists today:
1. `read` on real web pages
2. `query` starting from a seed URL
3. `query` without seed URL via web bootstrap discovery (`web_search`)
4. `crawl` on linked pages
5. local proof, replay, diff, and prune
6. CLI and MCP over the same core path
7. local intelligence lanes `0/1/2/3`
8. local domain genome that influences runtime behavior
9. local candidate memory (`.needlex/candidates/index.json`) with goal-token reranking
10. query auto-seeding from local candidate memory when seed URL is absent and discovery is enabled

What does not exist yet:
1. a first-class Web IR surfaced as a stable artifact and optimization substrate
2. a real retrieval compiler that converts goals into explicit execution plans
3. a Needle-native discovery substrate beyond provider bootstrap
4. an internal fingerprint graph with delta-aware retrieval logic
5. a final market-facing product shape

## Core Philosophy

These are the non-negotiable design rules.

1. Deterministic first.
2. Model activation only when policy says it is needed.
3. Every useful output must be auditable.
4. Local-first state is a feature, not a temporary implementation detail.
5. Minimal code is a product advantage.
6. CLI and MCP are adapters, not places for business logic.
7. Bootstrap integrations are scaffolding, not product identity.

In plain language:
Needle-X should not be a wrapper around external search or a pile of agent glue. It should become its own retrieval runtime with its own logic.

## What Is Real Versus Tactical

The project now has two different layers, and they must not be confused.

### Strategic Core

These are the parts that define the category and the moat:
1. deterministic `read` pipeline
2. proof-carrying chunks
3. trace, replay, and diff
4. local-first persistence
5. policy-gated intelligence lanes
6. domain genome adaptation
7. budget-constrained architecture

If these improve, Needle-X becomes more unique.

### Tactical Scaffolding

These are useful, but they are not the identity of the product:
1. provider-bootstrap `web_search`
2. same-site link discovery
3. external `trafilatura` comparison path
4. temporary heuristics added only to stabilize live reads

If these expand too much, Needle-X risks looking like a conventional search/scraping utility with extra layers on top.

## What The Runtime Can Do Right Now

### `read`

Given a real URL, Needle-X can:
1. fetch the page
2. reduce boilerplate deterministically
3. segment content semantically
4. rank and pack compact chunks
5. attach proof records
6. save trace artifacts locally

### `query`

Given a goal and an optional seed URL, Needle-X can:
1. stay on the seed page with `discovery=off`
2. do same-site candidate discovery with `discovery=same_site_links`
3. do a first bootstrap web search with `discovery=web_search`
4. merge bootstrap candidates across multiple providers
5. probe top web candidates locally and expand the best same-host child links
6. pick the better candidate deterministically
7. read and pack the selected page

### `crawl`

Given a seed URL, Needle-X can:
1. follow links
2. enforce `max_pages`, `max_depth`, `same_domain`
3. read each visited page through the same core path

### Proof And Debug

Needle-X can:
1. emit proof per chunk
2. save trace per run
3. replay trace status
4. diff two traces

## Current Architecture State

The repo already has the core packages we wanted:
1. `internal/config`
2. `internal/core`
3. `internal/intel`
4. `internal/pipeline`
5. `internal/proof`
6. `internal/store`
7. `internal/transport`

This matters because the architecture is no longer speculative. It is implemented and under budget.

## Intelligence State

The intelligence system is present and real.

Current lanes:
1. `Lane 0`: deterministic only
2. `Lane 1`: router + judge policy
3. `Lane 2`: local extractor
4. `Lane 3`: local constrained formatter

Important constraint:
these are policy-gated and proof-traced. Needle-X is not delegating the main path to an LLM by default.

Important clarification:
the current lane system is strategically correct, but the SLM layer is still early.
Today the main innovation is the policy and proof surface, not yet a fully differentiated model capability.

## Domain Genome State

The domain genome is no longer passive storage.

Today it can influence:
1. `force_lane`
2. `preferred_profile`
3. `pruning_profile`
4. `render_hint`

Meaning:
the runtime now adapts by domain based on local history.

## Strategic Gaps

These are the gaps that matter most if the goal is to stay "next-gen" rather than drift into something conventional.

### 1. Web IR Is Not First-Class Yet

The reducer and segmenter already produce a usable internal substrate, but Needle-X still does not expose or operate around a first-class Web IR artifact.

This matters because Web IR is supposed to be the "compiler layer" of the product, not an implementation detail.

### 2. Retrieval Compiler Is Still Thin

`QueryPlan` exists, but it is still closer to a thin execution record than a real compiler plan.

We still need:
1. explicit planning decisions
2. cost-quality tradeoff encoding
3. lane and discovery choices as machine-readable policy outputs
4. re-runnable planning independent from transport

### 3. Native Discovery Does Not Exist Yet

The current query-only path works, but it is still bootstrap-based.

This is acceptable as scaffolding, but not as the final strategic direction.
If overbuilt, it would pull the product toward "search wrapper" territory.

### 4. Fingerprint Graph Is Not Materialized Yet

Fingerprints exist on chunks and local state exists in store, but the graph-level intelligence described in the original thesis is not yet real.

Without this, Needle-X cannot yet become a delta-aware retrieval engine.

### 5. SLM Advantage Is Architectural More Than Functional

The current system correctly gates model-assisted lanes by policy and records proof of activation.

That is good and important.
But the real moat is not "we have SLM hooks".
The moat will exist only when local small-model decisions materially outperform deterministic-only fallback on hard ambiguity cases without breaking budget discipline.

## Quality State

The repo already enforces a serious baseline.

Current quality gates include:
1. end-to-end golden tests
2. replay determinism checks
3. fidelity checks
4. `tiny` compression `>= 3x`
5. query strategy comparison
6. comparison against a naive plain-text baseline
7. comparison against a reduced deterministic baseline
8. code budget checks

Current budget status:
1. production LOC under `8000`
2. internal packages under `10`
3. runtime dependencies under `8`
4. max file size under `400 LOC`

## Current Phase

The project is in:
`runtime foundation complete + strategic realignment around moat-building`

In practical terms:
1. the runtime foundation is mostly in place
2. validation is already happening against benchmarks and baselines
3. a tactical web discovery path exists and is good enough for experimentation
4. the next frontier is not "more discovery features", but turning the runtime into the category promised by the original thesis

In sharper terms:
the foundation is no longer the bottleneck.
The bottleneck is now moat realization.
If the next work does not strengthen `WebIR`, retrieval compilation, or graph-native retrieval, it is probably side work.

## What We Must Protect

These things must stay true even under delivery pressure:
1. no architecture sprawl
2. no business logic in transports
3. no feature work that weakens determinism, proof, or replay
4. no search-provider dependency becoming the center of the product
5. no SLM usage without explicit policy reason and artifact trail
6. no code growth that is not justified by a moat-building capability

## Priority Order From Now On

The work should now be ordered by strategic leverage, not by ease of implementation.

### Priority 1: Make Web IR First-Class

Deliver:
1. stable IR structure and schema/versioning
2. explicit provenance and noise signals in the IR
3. IR inspection/debug surface
4. pack/proof logic that clearly derives from IR states

Why this is first:
without first-class IR, Needle-X remains a strong extractor runtime, but not yet a web compiler.

Current reading:
1. `WebIR` exists and is versioned in outputs
2. it is validated and tracked in live regression
3. `WebIR` contributes explicit evidence to segment ranking and query compiler observation
4. pack selection now applies explicit IR policy (embedded/heading/noise swap), and proof chains include IR provenance markers
5. CLI/debug surfaces now expose IR selection diagnostics from pack trace metadata

Definition of done for this priority:
1. ranking inputs are visibly derived from `WebIR` signals
2. proof chains reference IR provenance, not only late-stage chunk transforms
3. `QueryPlan` can explain which IR evidence shaped the plan
4. IR inspection is a first-class debugging path, not just output decoration

### Priority 2: Turn `QueryPlan` Into A Real Retrieval Compiler

Deliver:
1. explicit planning stages
2. plan decisions with reason codes
3. lane/discovery/profile/budget selection as compiler outputs
4. serialized plan that can be inspected, replayed, and diffed

Why this is second:
this is the bridge from "tool call" to "runtime with its own reasoning model".

Current reading:
1. `QueryPlan` and compiler reason codes exist
2. planning now records `WebIR` evidence both after final read and during probed candidate selection
3. compiler decisions now cover graph evidence, provider fallback rationale, and basic risk gating
4. planning now also consumes seed fingerprint evidence from local state and emits explicit stability/novelty/delta-risk reasons
5. query discovery now applies a first seed-side graph-aware ranking bias, demoting unchanged stable seeds and preserving novel seeds when appropriate
6. current compiler is still closer to a structured execution log than a true planner, but the plan is materially more explanatory than before
7. planning still under-expresses lane rationale and richer quality-risk tradeoffs

Definition of done for this priority:
1. compiler decisions cover seed strategy, graph expansion, provider fallback, lane policy, and risk gating
2. every major query decision has a stable reason code family
3. the plan is replayable and diffable as a standalone artifact
4. query execution looks like "plan then execute", not "execute while annotating"

### Priority 3: Build Native Discovery Substrate Beyond Provider Bootstrap

Deliver:
1. domain-to-domain expansion logic not anchored only to provider SERPs
2. local candidate memory
3. stronger reranking using runtime-native signals
4. first seedless flow that is not defined by external search HTML

Why this is third:
native discovery matters, but if built too early or too broadly it can swallow the roadmap and turn the product into a commodity search layer.

Status:
1. local candidate memory is now active in runtime (`read/query/crawl` observations + query auto-seed)
2. query now propagates local `domain_hints` into discovery ranking with explicit `domain_hint_match` signals
3. a first native domain graph (`domain_graph`) now expands domain hints from local transition history
4. query/read/crawl local-state orchestration (auto-seed/hints/graph/genome/observe) has been moved to `internal/core/service` to reduce business logic in transports
5. remaining work is stronger graph-driven expansion policy and quality gates on expansion noise
6. architecture guard tests now enforce package-wide that transport `.go` entry files do not directly own candidate/domain-graph/genome state orchestration, including scoped `internal/store` imports and forbidden state-logic symbols

Definition of done for this priority:
1. seedless query can succeed on a meaningful subset of goals without provider bootstrap
2. graph expansion is confidence-aware and auditable
3. provider bootstrap is an explicit fallback stage, not the default identity path
4. discovery quality can be benchmarked separately from page reading quality

### Priority 4: Materialize The Fingerprint Graph

Deliver:
1. cross-run chunk relationships
2. dedup that uses graph evidence rather than local heuristics only
3. delta extraction and re-read minimization
4. change-aware retrieval primitives

Why this is fourth:
this is one of the clearest long-term moats in the original thesis and one of the least commoditized components.

Current reading:
1. fingerprints are persisted
2. a first local fingerprint graph now persists per-URL latest snapshots and cross-run delta history
3. retained/added/removed chunk fingerprints are now observable across repeated reads
4. `read` now loads prior fingerprint snapshots and exposes stable-vs-novel chunk counts in pack trace metadata
5. `tiny` profile now uses fingerprint-graph-aware dedup as a guarded compaction aid, preferring novel near-duplicates over stable ones
6. `query` planning now consumes seed-side fingerprint evidence and emits `stable_region_bias`, `novelty_bias`, and `delta_risk` reason codes
7. query discovery now uses that same seed-side evidence for a first graph-aware ranking bias on the seed candidate
8. graph intelligence is still early: it does not yet drive standard-profile read ranking, cross-document planning, or re-read minimization decisions

Definition of done for this priority:
1. chunk relationships exist across runs and documents
2. dedup uses graph evidence rather than local string heuristics only
3. re-read can skip or minimize stable regions
4. change-aware retrieval becomes a real product primitive

### Priority 5: Make SLM Use Earn Its Place

Deliver:
1. harder ambiguity benchmarks
2. measurable quality wins for lane `2/3`
3. stronger local small-model integration only where deterministic methods actually plateau
4. strict proof and budget accounting for every invocation

Why this is fifth:
SLM usage should be a scalpel, not branding.
If it does not create measurable fidelity gains on hard cases, it is noise.

Current reading:
1. the architecture is correct
2. proof and policy surfaces are correct
3. measurable superiority on hard ambiguity cases is not yet established

### Priority 6: Tighten Validation And Performance Around The Core

Deliver:
1. fairer external comparisons
2. broader real-site validation
3. profiling on probe-heavy flows
4. cost-quality reporting tied to lane decisions

Why this is sixth:
performance matters, but acceleration without moat clarity is optimization in the wrong direction.

## Strategic Milestones

From this point forward, the roadmap should be read as three milestone blocks, not as a flat feature list.

### Milestone A: Web Compiler Core

This milestone is complete only when:
1. `WebIR` is first-class in debugging and decision-making
2. `QueryPlan` becomes a real compiler artifact
3. proof chains and plan reasons clearly reference IR evidence

What this milestone changes:
Needle-X stops looking like a strong extractor and starts looking like a compiler for web context.

### Milestone B: Native Retrieval Substrate

This milestone is complete only when:
1. local candidate memory and domain graph are no longer just helpers
2. seedless flow can work from local substrate before provider fallback
3. fingerprint graph enables delta-aware retrieval

What this milestone changes:
Needle-X stops depending conceptually on external discovery scaffolding and starts owning a retrieval substrate.

### Milestone C: Measured Intelligence Advantage

This milestone is complete only when:
1. lane `2/3` wins are measured on hard ambiguity suites
2. SLM use remains budget-disciplined and fully auditable
3. the market-facing claim is backed by benchmark evidence, not architecture alone

What this milestone changes:
Needle-X stops being "well-architected for future intelligence" and becomes provably better on the cases that matter.

Current reading:
1. hard-case matrix now exports a finalized acceptance gate
2. acceptance covers global pass-rate, lane-lift-rate, objective-lift average, and risk ceiling
3. failure classes are explicitly mapped to future SLM rollout blocking policy

## Immediate Next Steps

The next engineering steps should now be:
1. implement a single real-SLM adapter boundary
2. integrate one real backend under strict policy gating
3. validate real-SLM lane effects against the new acceptance gate
4. keep deterministic-first default with full proof/trace model-call accounting

Anything outside these four lines should be treated as a conscious exception.

## What To Deprioritize

The following work is allowed only if it clearly supports one of the priorities above:
1. adding more search bootstrap providers
2. broadening generic crawl scope without new retrieval logic
3. transport-surface growth
4. UI/dashboard work
5. cloud/service integrations
6. overfitting heuristics for isolated sites without reusable runtime value

## What Needle-X Is And Is Not Becoming

Needle-X should become:
1. a verified web context runtime
2. a retrieval compiler for agent objectives
3. a local-first proof-carrying retrieval substrate

Needle-X should not become:
1. a thin wrapper around external search
2. a generic scraper toolkit
3. a prompt-heavy agent orchestration layer
4. a feature-bloated browser automation product

## How To Use This Document

If you need fast context before editing the repo:
1. read this file first
2. then read `README.md`
3. then read `spec.md`
4. then inspect the relevant package

This file should be updated whenever one of these changes:
1. the phase changes
2. the product boundary changes
3. a major capability becomes real
4. a previously claimed capability is proven false or incomplete
