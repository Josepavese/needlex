# Go-To-Market Readiness

This document defines the narrow market-facing package for Needle-X.

## One-Line Definition

Needle-X is a local-first web context compiler for AI agents that turns noisy pages into compact, proof-carrying context using deterministic structure and semantic context alignment.

## Benchmark Story

What we can say today, and defend:
1. Needle-X compiles web pages into concentrated context rather than passing raw HTML or plain text downstream.
2. The active runtime is benchmark-backed with one narrow model role only: `resolve_ambiguity`.
3. Semantic context alignment works on multilingual pages where lexical overlap is effectively blind.
4. The product emits proof, trace, replay, and diff artifacts as first-class runtime outputs.
5. Warm-state `Discovery Memory` turns repeated use into strong local retrieval.
6. The active path is operationally stable.

What is especially marketable because it is already measured:
1. Needle-X is much smaller than extraction-heavy competitors on agent-facing output
2. Needle-X is the only measured runtime in the live comparison with usable proof-carrying output
3. Needle-X reduces claim-to-source distance and post-processing burden for downstream agents

What we must not say yet:
1. that Needle-X is a broad web-search replacement
2. that it is a browser agent
3. that model escalation is broadly superior across all live cases
4. that it has solved the general web extraction problem
5. that it beats a market leader before a seeded competitive benchmark exists
6. that cold-state seedless discovery is solved

## Differentiation Story

Needle-X is different from scraper-plus-LLM stacks because:
1. it is deterministic-first, not model-first
2. it preserves provenance through proof records and traces
3. it uses semantic context to judge meaning on a multilingual web
4. it keeps model usage bounded and benchmark-backed
5. it remains local-first and operator-inspectable

In practical terms:
1. commodity stacks read pages and ask a model to improvise
2. Needle-X compiles pages into auditable context before the agent consumes them
3. with warm local memory, Needle-X can route and recall from prior verified work without requiring hosted search infrastructure

Current live competitive advantage metrics:
1. average packet size:
   - `Needle-X`: `4436`
   - `Tavily`: `6975`
   - `Jina`: `30565`
   - `Firecrawl`: `72166`
2. claim-to-source steps:
   - `Needle-X`: `1`
   - others in the live run: `2`
3. average post-processing burden:
   - `Needle-X`: `0.25`
   - `Jina`: `1.86`
   - `Tavily`: `1.92`
   - `Firecrawl`: `2.5`
4. proof usability:
   - `Needle-X`: `1.0`
   - others in the live run: `0`

## Discovery Memory Story

The strongest new story is not “Needle-X is now a search engine”.

It is this:
1. the first run observes and compiles the web
2. later runs can reuse local verified evidence
3. repeated use improves retrieval without requiring infrastructure

Current benchmark-backed statement:
1. on the active warm-state discovery-memory benchmark, Needle-X reached `30/30` selected-url correctness
2. in those warm-state cases, `discovery_memory` was the selected provider in `30/30` cases

Guardrail:
1. this is a warm-state local retrieval claim
2. it is not a cold-state open-web seedless claim

## Ideal Early User

Needle-X is currently best suited for:
1. agent builders who need traceable web context
2. local-first operators who cannot rely on opaque hosted pipelines
3. teams that need replay, diff, and provenance for debugging extraction quality
4. multilingual web workflows where lexical matching is too weak

It is not yet aimed at:
1. broad consumer search
2. fully autonomous browsing products
3. high-scale hosted extraction as a service

## Supported Product Surface

The package we can support today is:
1. `read`
2. `query`
3. `crawl`
4. `proof`
5. `replay`
6. `diff`

The active runtime contract behind that surface is:
1. deterministic substrate
2. semantic context alignment
3. bounded ambiguity solving only when benchmark-backed

## Release Checklist

The product is ready for early users only if all of the following are true:
1. `go test ./... -count=1` passes
2. `bash scripts/check_budget.sh .` passes
3. README, spec, operator guide, and vademecum are aligned
4. baseline commands run on the maintainer machine:
   - `./scripts/run_cpu_baseline_matrix.sh`
   - `NEEDLEX_LIVE_READ_USE_BASELINE_MODELS=1 NEEDLEX_LIVE_READ_OUT=improvements/live-read-baseline-cpu-compare.json ./scripts/run_live_read_eval.sh`
   - `NEEDLEX_LIVE_READ_CASES=benchmarks/corpora/live-sites-semantic-global-v1.json NEEDLEX_LIVE_READ_OUT=improvements/live-semantic-global-eval-latest.json ./scripts/run_live_semantic_eval.sh`
5. the active runtime contract remains:
   - CPU baseline `Gemma 3 1B`
   - semantic baseline `multilingual-e5-small`
   - active task `resolve_ambiguity`
6. no unsupported capability is described as release-grade

## Early-User Package

The minimum package for an early user is:
1. README
2. operator guide
3. benchmark report
4. model baseline doc
5. semantic alignment doc
6. release checklist from this document

Recommended handoff order:
1. explain the one-line definition
2. show `read --json`
3. show `proof` and `replay`
4. show one multilingual semantic example
5. only then show `query` and `crawl`

## Positioning Guardrails

Do not position Needle-X as:
1. "AI that browses the web for you"
2. "semantic search engine"
3. "drop-in replacement for search"
4. "universal extractor"

Position it as:
1. local-first context compiler
2. proof-carrying retrieval runtime for agents
3. semantic web-context layer for debugging-sensitive systems

## Exit Condition

Go-to-market readiness is complete when:
1. the narrow claim is stable across docs and demos
2. the benchmark story can be told without caveat sprawl
3. the early-user package is enough to evaluate the product honestly
