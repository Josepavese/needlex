# Semantic Jump Development Plan

Status: proposed
Date: 2026-05-04
Scope: five high-impact projects to move Needle-X from semantic ranking improvements to a durable semantic retrieval platform.

## Current Baseline

Latest accepted 100-case seedless no-key browser-like semantic lane:

1. pass rate: `0.56`
2. runtime rate: `0.98`
3. failure taxonomy: `right_family_not_selected = 34`, `wrong_family_selected = 8`, `runtime_error = 2`

Latest implementation checkpoint from `2026-05-09`:

1. pass rate: `0.68`
2. runtime rate: `0.96`
3. timeout rate: `0.04`
4. failure taxonomy: `benchmark_timeout = 4`, `right_family_not_selected = 26`, `wrong_family_selected = 2`
5. command: `go run ./benchmarks/seedless_ddg/runner -cases benchmarks/corpora/unique-sources-corpus-v1.json -profiles browser_like_semantic -runs 1 -timeout-ms 30000 -out /tmp/needlex-seedless-100-near-tie-restricted.json`
6. implemented components: semantic family evidence mass, restricted near-tie provenance review, compact semantic payloads, batch cross-similarity scoring

Interpretation:

1. provider blocking is no longer the dominant measured issue on this lane
2. many failures are selection failures inside an already visible family
3. semantic representative selection has produced a measurable jump, but the remaining dominant class is still `right_family_not_selected`
4. the next large jump should come from stronger semantic family recovery and calibration, not from lexical query hacks

## Non-Negotiable Product Constraints

1. embeddings-first
2. semantic-first
3. multilingual/global by design
4. zero literal-primary ranking
5. no API keys required
6. no site-specific rules
7. no provider-specific hacks
8. compact-first MCP output
9. inspectable diagnostics
10. release-gated behavior changes

## Recommended Execution Order

1. [Issue 01: Local Late-Interaction Reranker](2026-05-04-01-local-late-interaction-reranker.md)
2. [Issue 02: Entity-Family Graph Memory](2026-05-04-02-entity-family-graph-memory.md)
3. [Issue 05: Semantic Quorum Provider Fusion](2026-05-04-05-semantic-quorum-provider-fusion.md)
4. [Issue 04: Local HNSW Vector Index](2026-05-04-04-local-hnsw-vector-index.md)
5. [Issue 03: Trace-Driven Semantic Calibration](2026-05-04-03-trace-driven-semantic-calibration.md)

Reason:

1. late interaction directly attacks `right_family_not_selected`
2. graph memory turns repeated use into compounding product advantage
3. semantic quorum reduces public bootstrap volatility without stealth architecture
4. HNSW becomes necessary as graph/memory grows
5. calibration is most valuable after richer semantic signals exist

## Combined Target Architecture

Retrieval path:

1. normalize goal into semantic intent
2. query entity-family graph memory
3. query no-key provider PAL with health/cooldown/cache
4. convert provider observations into semantic candidate bundles
5. cluster observations by semantic family
6. retrieve vector neighbors from local index
7. rerank representatives with late interaction
8. apply trace-trained semantic calibrator when available
9. fetch selected representative
10. write proof, analytics, and memory evidence back to PAL

## Combined Benchmark Target

Conservative target after Issues 01, 02, and 05:

1. seedless 100-case pass: `>= 0.75`
2. seedless runtime: `>= 0.96`
3. warm memory 100-case selected pass: `>= 0.97`
4. seeded unique-source 100-case pass: `>= 0.99`
5. proof usability: `>= 0.98`

Stretch target after all five:

1. seedless 100-case pass: `>= 0.82`
2. seedless runtime: `>= 0.98`
3. `right_family_not_selected` below `10/100`
4. `wrong_family_selected` below `4/100`
5. no deterministic regression

## Research Sources

Late interaction and multilingual reranking:

1. [ColBERT](https://arxiv.org/abs/2004.12832)
2. [ColBERTv2](https://arxiv.org/abs/2112.01488)
3. [PLAID](https://arxiv.org/abs/2205.09707)
4. [Jina-ColBERT-v2](https://arxiv.org/abs/2408.16672)
5. [BGE-M3](https://arxiv.org/abs/2402.03216)

Dense and zero-shot retrieval:

1. [Dense Passage Retrieval](https://arxiv.org/abs/2004.04906)
2. [Dense Hierarchical Retrieval](https://arxiv.org/abs/2110.15439)
3. [BEIR](https://arxiv.org/abs/2104.08663)
4. [Multilingual E5](https://arxiv.org/abs/2402.05672)
5. [Low-resource cross-lingual DPR limits](https://arxiv.org/abs/2408.11942)

Graph and memory retrieval:

1. [GRAG](https://arxiv.org/abs/2405.16506)
2. [GraphRAG survey](https://arxiv.org/abs/2408.08921)
3. [Microsoft GraphRAG](https://www.microsoft.com/en-us/research/project/graphrag/)
4. [GraphRAG local search](https://microsoft.github.io/graphrag/query/local_search/)
5. [Knowledge Graph-Guided RAG](https://aclanthology.org/2025.naacl-long.449.pdf)

Vector indexing and scalability:

1. [HNSW](https://arxiv.org/abs/1603.09320)
2. [FAISS billion-scale similarity search](https://arxiv.org/abs/1702.08734)
3. [ONNX Runtime docs](https://onnxruntime.ai/docs)
4. [ONNX Runtime execution providers](https://onnxruntime.ai/docs/execution-providers)
5. [Hubness reduction in Sentence-BERT spaces](https://arxiv.org/abs/2311.18364)

Learning and fusion:

1. [Unbiased Learning-to-Rank](https://arxiv.org/abs/1608.04468)
2. [Counterfactual Online Learning to Rank](https://arvinzhuang.github.io/files/arvin2020counterfactual.pdf)
3. [Adversarial Retriever-Ranker](https://arxiv.org/abs/2110.03611)
4. [Reciprocal Rank Fusion](https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf)
5. [Sparse Meets Dense](https://arxiv.org/abs/2401.04055)

## Devil's Advocate Summary

Risk: local models add latency and packaging complexity.

Mitigation: shadow mode first, PAL model cache, CPU timeout, fallback path, no model bundled into core binary unless proven small enough.

Risk: graph memory can lock in wrong beliefs.

Mitigation: evidence-weighted, time-decayed, contradicted edges, benchmark negative evidence, reversible rebuild.

Risk: trace learning can overfit current bugs.

Mitigation: family/domain holdouts, feature registry, no lexical-primary features, shadow evaluation.

Risk: approximate vector search can lose recall.

Mitigation: exact scan path, recall audit, top-k plus semantic rerank, exact scan mode for small datasets.

Risk: provider fusion can amplify correlated wrong results.

Mitigation: semantic clusters, graph validation, late-interaction representative selection, provider health memory, no rank-only decision.

## Single Best Bet

If only one large bet can be executed first, choose Issue 01.

Reason:

1. current failure taxonomy points to representative selection
2. late interaction directly improves fine-grained semantic ranking
3. it can be added in shadow mode without destabilizing acquisition
4. it composes cleanly with graph memory, provider fusion, and calibration

If two large bets can run together, choose Issue 01 plus Issue 02.

Reason:

1. Issue 01 improves per-query selection
2. Issue 02 compounds knowledge across runs
3. together they attack both cold and warm seedless failure modes

## Implementation Slice 2026-05-04

Implemented in code:

1. `internal/core/semanticrank`: local late-interaction reranker interface and exact tests
2. `internal/memory`: semantic family graph tables, passive family evidence writes, graph recall search
3. `internal/core/providerfusion`: provider observation annotation and semantic-cluster quorum primitives
4. `internal/core/vectorindex`: exact vector index PAL used by Discovery Memory vector search
5. `internal/core/semanticcalibrate`: semantic-only calibration registry with lexical-primary feature rejection

Runtime promotion decision:

1. graph memory and exact vector PAL are active because tests pass and they do not add public bootstrap calls
2. provider observation metadata is active because it preserves evidence without changing score by itself
3. late-interaction and calibration are not active in cold seedless ranking by default because full 100-case runs did not beat the accepted gate

Measured rejection:

1. accepted baseline: `0.56` seedless pass, `0.98` runtime
2. active late-interaction/calibration: `0.55` pass, `0.96` runtime
3. shadow late-interaction/calibration: `0.48` pass, `0.93` runtime
4. current implementation constrains late-interaction/calibration to the bounded final candidate stack pending a fresh 100-case release gate

Next hard requirement:

1. batch or cache candidate-intelligence embeddings before activating late interaction
2. use a cheaper local multi-vector backend before adding extra semantic calls to cold seedless
3. promote only after the full 100-case lane beats the accepted baseline, not after a partial smoke
