# Project Context

Primary execution doctrine:
1. [vademecum.md](vademecum.md)

Baseline SSOT:
1. [model-baseline.md](model-baseline.md)
2. [semantic-alignment-gate.md](semantic-alignment-gate.md)

Strategic but non-active specs:
1. [discovery-memory-spec.md](experimental/discovery-memory-spec.md)
2. [agentic-decision-plane-spec.md](experimental/agentic-decision-plane-spec.md)
3. [seedless-discovery.md](experimental/seedless-discovery.md)

## Product Shape

Needle-X is:
1. a local-first web context compiler
2. a deterministic substrate reducer
3. a proof-carrying retrieval layer for agents
4. an AI-first runtime whose default output is a compact answer packet

Needle-X is not:
1. a browser agent
2. a search engine
3. a generic hosted scraping API
4. an infra-heavy pipeline

## Active Runtime

What is strong today:
1. `read`
2. seeded `query`
3. compact default output
4. proof / trace / replay
5. local discovery memory in warm-state flows

What remains narrow:
1. experimental seedless open-web discovery
2. broad market claim beyond seeded and warm-state paths

Stable research workflow:
1. the host agent obtains and selects candidate URLs with its own search tool
2. Needle-X compiles every URL selected by the agent
3. Needle-X does not impose a candidate count or research-breadth policy

## Active Technical Doctrine

1. deterministic first
2. semantic signals for meaning-sensitive decisions
3. no linguistic heuristics as primary logic
4. local-first state is part of the product
5. compact output is the default contract
6. diagnostics are explicit, not default

## Active Surface

Current public-facing surfaces:
1. `README.md`
2. `spec.md`
3. operator docs under `docs/`
4. benchmark harness under `benchmarks/`
5. active reports under `improvements/`
