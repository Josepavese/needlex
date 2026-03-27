# Project Context

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

## Product Shape Today

The current product is real, but it is still a technical product rather than the final market-facing product.

What exists today:
1. `read` on real web pages
2. `query` starting from a seed URL
3. `crawl` on linked pages
4. local proof, replay, diff, and prune
5. CLI and MCP over the same core path
6. local intelligence lanes `0/1/2/3`
7. local domain genome that influences runtime behavior

What does not exist yet:
1. true open-web search without a seed URL
2. cross-site discovery and ranking at web scope
3. an internal web index or candidate corpus
4. a final market-facing search product

## Core Philosophy

These are the non-negotiable design rules.

1. Deterministic first.
2. Model activation only when policy says it is needed.
3. Every useful output must be auditable.
4. Local-first state is a feature, not a temporary implementation detail.
5. Minimal code is a product advantage.
6. CLI and MCP are adapters, not places for business logic.

In plain language:
Needle-X should not be a wrapper around external search or a pile of agent glue. It should become its own retrieval runtime with its own logic.

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

Given a goal and a seed URL, Needle-X can:
1. stay on the seed page with `discovery=off`
2. do same-site candidate discovery with `discovery=same_site_links`
3. pick the better candidate deterministically
4. read and pack the selected page

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

## Domain Genome State

The domain genome is no longer passive storage.

Today it can influence:
1. `force_lane`
2. `preferred_profile`
3. `pruning_profile`
4. `render_hint`

Meaning:
the runtime now adapts by domain based on local history.

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
`runtime foundation complete + comparative validation + pre-search expansion`

In practical terms:
1. the runtime foundation is mostly in place
2. validation is already happening against benchmarks and baselines
3. the next major frontier is true search and discovery beyond seeded retrieval

## What We Have Not Built Yet

These are the main missing pieces.

### 1. True Web Search

We still do not have:
1. query-only discovery without a seed URL
2. cross-site candidate expansion
3. web-scale ranking and reranking
4. a Needle-native search graph or local web index

### 2. Stronger External Baselines

We currently compare against a naive baseline.

We still need:
1. a stronger external deterministic reader baseline
2. a more realistic retrieval comparison suite

### 3. Performance Work

Needle-X currently wins on output quality and auditability, not on raw speed.

That is acceptable for now, but we still need:
1. profiling
2. targeted performance tightening
3. better cost/quality tradeoff reporting

## What Needle-X Is Not Yet

Needle-X is not yet:
1. a Google replacement
2. a consumer search engine
3. a finished agent platform
4. a broad web index

It is the engine that could become the retrieval core of that product.

## Recommended Next Moves

The best next steps are:
1. create a persistent benchmark report in the repo
2. add a stronger external deterministic baseline beyond the in-repo reduced baseline
3. design the first true `discover_web` path
4. expand `query` from same-site discovery to multi-source discovery

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
