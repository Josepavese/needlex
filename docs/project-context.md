# Project Context

Primary execution doctrine: see `docs/vademecum.md`.
Future semantic context policy: see `docs/semantic-alignment-gate.md`.
Future local seedless discovery substrate: see `docs/experimental/discovery-memory-spec.md`.
Future seedless discovery strategy: see `docs/experimental/seedless-discovery-strategy.md`.
Historical documents live under `docs/archive/`.
Strategically interesting but not active future specs live under `docs/experimental/`.

This is the living context document for Needle-X.

CPU model baseline SSOT: see `docs/model-baseline.md`.

## In Simple Terms

Needle-X is:
1. a local-first web context runtime
2. a deterministic extraction and compression engine
3. a proof-carrying retrieval layer for agents
4. a semantic context compiler rather than a lexical matcher

It reads web pages, removes noise, selects the useful parts, compresses them, and produces traceable output with proof, replay, and diff support.

Output doctrine:
1. the default surface must be compact compiled context
2. proof and trace are core capabilities, but not the default payload shape
3. the product should minimize downstream token cost, not just extraction cost

## Product Shape Today

What exists today:
1. `read` on real web pages
2. `query` starting from a seed URL
3. `query` without seed URL via bootstrap discovery (`web_search`)
4. `crawl` on linked pages
5. local proof, replay, diff, and prune
6. CLI and MCP over the same core path
7. local intelligence lanes `0/1/2/3`
8. local domain genome that influences runtime behavior
9. local candidate memory (`.needlex/candidates/index.json`)
10. semantic context alignment for meaning-sensitive evaluation and routing support

What does not exist yet:
1. a finished market-facing contract
2. broad enough live validation to support a wider release claim
3. operator docs polished for external adopters
4. a local discovery memory that can reduce dependence on public bootstrap search

## Core Philosophy

1. Deterministic first.
2. Model activation only when policy says it is needed.
3. Every useful output must be auditable.
4. Local-first state is a feature, not a temporary implementation detail.
5. Minimal code is a product advantage.
6. CLI and MCP are adapters, not places for business logic.
7. Bootstrap integrations are scaffolding, not product identity.
8. Meaning-sensitive routing prefers semantics over lexical coincidence.

## Strategic Core

These are the parts that define the category and the moat:
1. deterministic `read` pipeline
2. proof-carrying chunks
3. trace, replay, and diff
4. local-first persistence
5. policy-gated intelligence lanes
6. domain genome adaptation
7. budget-constrained architecture
8. semantic context alignment on a multilingual web
9. future client-local discovery memory for infrastructure-free seedless retrieval reuse

## Tactical Scaffolding

These are useful, but they are not the identity of the product:
1. provider-bootstrap `web_search`
2. same-site link discovery
3. external `trafilatura` comparison path
4. temporary heuristics added only to stabilize live reads

Important extension:
public search bootstrap is useful but not reliable enough to be product identity for a repo-only distribution model.
The long-term correction path is local `Discovery Memory`, not mandatory infrastructure.

Important correction:
lexical heuristics are not a strategically sufficient context gate.
They remain useful only as auxiliary signals and cheap pattern detectors.

## Intelligence State

Current runtime reality:
1. `Lane 0`: deterministic only
2. model-assisted runtime is active only for `resolve_ambiguity`
3. the active CPU baseline is `Gemma 3 1B`
4. multilingual semantic alignment is real and benchmarked for context gating
5. the semantic provider/model pair is SSOT-driven exactly like the main runtime baseline

Important clarification:
Needle-X is not trying to add more model roles right now.
The active direction is:
1. broaden live trust
2. keep meaning-sensitive routing semantic-first
3. narrow the market-facing contract around benchmark-backed behavior

## Strategic Gaps

### 1. Live Trust Is Still Narrow

The active runtime is coherent, but live validation is still limited compared with a true public-web surface.

### 2. Product Contract Is Still Too Implicit

The architecture is now sharper than the product story.
The next risk is not technical confusion; it is market confusion.

### 3. Operator UX Needs Closure

Proof and trace are strong, but they should be consumed explicitly. The default operator and agent path should stay compact and low-friction.

## Current Phase

The project is in:
`runtime trust closure complete + live/product/operator closure phase`

In practical terms:
1. the runtime foundation is real
2. the active intelligence profile is benchmark-backed
3. the bottleneck is release credibility, not feature existence
