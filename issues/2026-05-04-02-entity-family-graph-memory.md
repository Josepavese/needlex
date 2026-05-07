# Issue 02: Entity-Family Graph Memory

Status: implemented for persistence, active recall, diagnostics, export, and import
Date: 2026-05-04
Priority: P0 after late-interaction shadow mode
Surface: Discovery Memory, seedless discovery, provider fallback, analytics

## Objective

Transform Discovery Memory from a useful local cache into a semantic entity-family graph that learns which resources belong to the same conceptual family and which resource should represent that family for a given intent.

The current weak point is not only finding candidates. It is selecting the correct representative inside a candidate family. This graph should make Needle-X progressively better as agents use it.

## Semantic Contract

This project must be:

1. semantic graph first
2. embedding-backed
3. multilingual
4. provenance-aware
5. explainable
6. reversible

It must not become:

1. a host allowlist
2. a domain-specific rule table
3. a keyword taxonomy
4. a permanent cache of wrong selections
5. a provider-specific recovery hack

Host, URL, redirect, canonical, and HTTP metadata are evidence properties. They are not the graph's meaning layer.

## Research Basis

Primary references:

1. [GRAG: Graph Retrieval-Augmented Generation](https://arxiv.org/abs/2405.16506)
2. [Graph Retrieval-Augmented Generation: A Survey](https://arxiv.org/abs/2408.08921)
3. [Microsoft GraphRAG project](https://www.microsoft.com/en-us/research/project/graphrag/)
4. [GraphRAG local search documentation](https://microsoft.github.io/graphrag/query/local_search/)
5. [When to use Graphs in RAG: A Comprehensive Analysis](https://arxiv.org/abs/2506.05690)
6. [Knowledge Graph-Guided Retrieval Augmented Generation](https://aclanthology.org/2025.naacl-long.449.pdf)
7. [Dense Hierarchical Retrieval for Open-Domain QA](https://arxiv.org/abs/2110.15439)

Interpretation for Needle-X:

1. Graphs help when relationships matter, not when a single chunk is enough.
2. Seedless discovery is relationship-heavy: origin, mirror, derivative, distribution, social context, docs page, API endpoint, asset, canonical record.
3. Graph retrieval can fail if graph construction is noisy. The Needle-X graph must be evidence-weighted and reversible.
4. Local graph search can combine structured graph context and unstructured page evidence without requiring hosted services.

## Product Hypothesis

A persistent semantic entity-family graph will improve seedless and warm-state retrieval by recovering the right family before public bootstrap noise dominates.

Target impact:

1. warm Discovery Memory selected pass: from `0.94` to `>= 0.97`
2. seedless selected pass: from `0.56` to `>= 0.65` without late interaction, or `>= 0.75` combined with Issue 01
3. reduce `wrong_family_selected`
4. reduce repeated public bootstrap on already known entity families
5. improve analytics transparency for why a candidate was selected

## Graph Model

Core node types:

1. `entity`: conceptual entity, product, organization, standard, library, public service, dataset, authority, document family
2. `resource`: concrete URL or file-like resource
3. `host`: provenance host, not semantic identity by itself
4. `topic`: embedding-derived concept cluster
5. `role`: semantic role profile such as custodian origin, custodian record, derivative representation, distribution node, social context
6. `evidence`: observed trace, proof, redirect, canonical, successful read, failed selection

Core edge types:

1. `represents`: resource represents entity
2. `belongs_to_family`: entity/resource belongs to semantic family
3. `derives_from`: derivative resource points toward origin family
4. `distributed_by`: distribution node exposes family resource
5. `mentions_or_contextualizes`: social/context resource discusses family
6. `canonicalizes_to`: observed canonical or redirect relationship
7. `co_selected_with`: observed candidate cluster relationship
8. `contradicts`: negative evidence from failed benchmark or trace

Every edge stores:

1. confidence
2. evidence count
3. last observed time
4. source operation
5. source trace/proof ID
6. embedding model ID
7. decay policy

## Retrieval Flow

Seedless query flow:

1. embed query intent
2. search graph entities/topics by vector similarity
3. activate neighboring family nodes with bounded propagation
4. propose representative resources from the activated family
5. merge with public no-key candidate pool
6. rerank candidates semantically
7. select final resource
8. write new evidence back to graph

Seeded query flow:

1. embed seed page context
2. attach resource to existing entity family if confidence is high
3. add resource as isolated candidate if confidence is low
4. never force merge based on host string alone

## Implementation Plan

Phase 0: graph schema and migration

1. add graph tables to Discovery Memory DB or adjacent PAL DB
2. keep SQLite as SSOT
3. define node and edge schemas with evidence provenance
4. add migration versioning
5. add export/import for graph state

Phase 1: passive graph builder

1. observe successful reads and queries
2. create resource nodes
3. embed resource context
4. infer semantic role with existing candidate intelligence
5. create low-confidence entity/topic nodes
6. record evidence without influencing live ranking

Phase 2: family clustering

1. cluster resource nodes by embedding similarity and role compatibility
2. prevent hub resources from absorbing unrelated families
3. use evidence decay for stale or contradicted links
4. track family representative candidates
5. expose graph diagnostics in `memory stats` and `doctor`

Phase 3: active retrieval

1. consult graph before public bootstrap
2. use graph activation as a semantic prior
3. merge graph candidates with live candidates
4. emit compact MCP diagnostics only when useful
5. record whether graph memory changed selected result

Phase 4: correction loop

1. use benchmark failures as negative evidence
2. downgrade incorrect family links
3. add contradicted edges for wrong representative choices
4. allow one-time graph rebuild from canonical traces without exposing a user-facing rebuild command unless needed for maintenance

## Tests

Unit tests:

1. graph schema migration
2. evidence insertion idempotency
3. confidence decay
4. family merge threshold behavior
5. contradiction lowers selection confidence

Integration tests:

1. successful read creates resource and evidence nodes
2. seedless query consults graph and returns known family resource
3. wrong benchmark selection creates negative edge
4. graph activation does not override high-confidence direct fetch evidence

Benchmark tests:

1. warm memory 100-case lane
2. seedless 100-case lane with graph disabled/enabled
3. multilingual global suite
4. replay of prior `right_family_not_selected` failures

## Metrics

Primary:

1. warm selected pass rate
2. seedless selected pass rate
3. graph-assisted selection rate
4. graph-assisted success rate
5. wrong-family reduction

Safety metrics:

1. graph false merge rate
2. contradicted edge count
3. stale edge count
4. hubness concentration by entity node
5. graph DB size growth

Product metrics:

1. public bootstrap avoided count
2. average latency saved by memory-first retrieval
3. agent chars saved through graph-assisted reads
4. repeated-query improvement rate

## Devil's Advocate

Objection: graphs are often expensive and noisy.

Response: build a small evidence graph, not a full LLM-extracted knowledge graph. Use local embeddings, observed traces, role profiles, canonical/redirect evidence, and negative benchmark evidence.

Objection: a graph can lock in wrong beliefs.

Response: every edge is evidence-weighted, time-decayed, and reversible. Wrong benchmark selections create negative evidence.

Objection: entity-family detection might become lexical via host names.

Response: host identity is provenance only. Family merge requires semantic embedding proximity, role compatibility, and evidence support.

Objection: GraphRAG helps reasoning, but Needle-X is retrieval.

Response: Needle-X does not need generation-oriented GraphRAG. It needs graph-assisted representative selection, which is narrower and easier to evaluate.

Objection: low-resource languages may fragment families.

Response: store multilingual embeddings with model ID and allow cross-lingual family linking only above higher confidence. Track language-family fragmentation as a metric.

## Acceptance Criteria

1. Discovery Memory has graph tables with migrations.
2. Reads and queries passively populate graph evidence.
3. Graph search runs in shadow mode and reports candidate impact.
4. Active graph retrieval improves warm memory or seedless pass rate without deterministic regression.
5. Graph links are explainable and reversible.
6. No hardcoded site or monolingual keyword taxonomy is introduced.
