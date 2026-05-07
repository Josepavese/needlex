# Issue 01: Local Late-Interaction Reranker

Status: implemented as bounded dense-semantic candidate-stack reranker; benchmark gate still required for release claims
Date: 2026-05-04
Priority: P0 if seedless quality remains the main product bottleneck
Surface: seedless discovery, Discovery Memory ranking, MCP `web_query`

## Objective

Add a local, no-key, multilingual late-interaction reranker that reranks the final candidate pool before fetch selection.

This directly attacks the current dominant seedless failure class:

1. `right_family_not_selected = 34/100`
2. `wrong_family_selected = 8/100`
3. `runtime_error = 2/100`

The expected family is often already in the pool, but the current ranker cannot reliably select the best representative. A single-vector embedding score is too coarse for this; the system needs token/passage-level semantic interaction.

## Semantic Contract

This project must be:

1. embeddings-first
2. multilingual by default
3. local inference only
4. zero API key
5. zero site-specific behavior
6. zero lexical-primary scoring

Allowed signals:

1. multilingual dense vectors
2. multi-vector late interaction
3. candidate context embeddings
4. semantic role embeddings
5. entity-family graph priors
6. structural provenance as metadata

Not allowed as primary ranking signals:

1. exact word overlap
2. language-specific keyword lists
3. path-name special casing
4. provider-name boosts
5. hardcoded domains or sites

URL, host, title, and path may remain provenance and safety metadata. They must not become the primary ranking surface.

## Research Basis

Primary references:

1. [ColBERT: Efficient and Effective Passage Search via Contextualized Late Interaction over BERT](https://arxiv.org/abs/2004.12832)
2. [ColBERTv2: Effective and Efficient Retrieval via Lightweight Late Interaction](https://arxiv.org/abs/2112.01488)
3. [PLAID: An Efficient Engine for Late Interaction Retrieval](https://arxiv.org/abs/2205.09707)
4. [Jina-ColBERT-v2: A General-Purpose Multilingual Late Interaction Retriever](https://arxiv.org/abs/2408.16672)
5. [BGE M3-Embedding: Multi-Lingual, Multi-Functionality, Multi-Granularity Text Embeddings](https://arxiv.org/abs/2402.03216)
6. [BEIR: A Heterogeneous Benchmark for Zero-shot Evaluation of Information Retrieval Models](https://arxiv.org/abs/2104.08663)

Interpretation for Needle-X:

1. Late interaction is the right architecture when candidates are related but fine-grained ranking fails.
2. Multilingual late-interaction models now exist and are aligned with Needle-X's global retrieval goal.
3. BEIR warns that dense retrievers vary under domain shift; this supports a reranker stage instead of relying only on a first-stage dense score.
4. PLAID-style compression is relevant later, but the first product step should rerank only a small candidate pool, not build a large full-corpus multi-vector engine.

## Product Hypothesis

If the candidate pool already contains the correct entity family, a local late-interaction reranker should convert many `right_family_not_selected` failures into selected-pass successes.

Target impact:

1. seedless 100-case pass rate: from `0.56` to at least `0.66`
2. stretch target: `0.72`
3. no regression in runtime success below `0.95`
4. seeded 100-domain pass remains `>= 0.99`
5. warm Discovery Memory remains `>= 0.94`

Shipping gate:

1. at least `+10` percentage points on the 100-case seedless lane
2. no deterministic suite regression
3. p95 additional rerank latency below `1500 ms` on CPU for the default pool
4. no API key or remote model inference requirement

## Architecture

Add a semantic reranking layer behind an interface:

```go
type LateInteractionReranker interface {
    Available(ctx context.Context) bool
    ModelID() string
    Score(ctx context.Context, query SemanticQuery, candidates []CandidateBundle) ([]LateInteractionScore, error)
}
```

Core data model:

```go
type SemanticQuery struct {
    Goal string
    IntentEmbedding []float32
    RoleEmbedding []float32
    TargetKindEmbedding []float32
    LanguageAgnosticHints []EmbeddingSpan
}

type CandidateBundle struct {
    URL string
    Title string
    Summary string
    SourceContext string
    StructuralContext string
    SemanticRole string
    ResourceClass string
    FamilyEvidence []FamilyEvidence
}

type LateInteractionScore struct {
    URL string
    Score float64
    Confidence float64
    ModelID string
    Explanation []SemanticReason
}
```

The default ranking path becomes:

1. gather candidates from memory and no-key public bootstrap
2. compute existing structural/context features
3. compute candidate-intelligence role/family features
4. call local late-interaction reranker on top `N` candidates
5. combine with graph activation and provider consensus as secondary signals
6. select final representative
7. fetch selected page

## Model Strategy

Phase 1 should support one local multilingual model path:

1. model bundle stored under PAL model cache
2. model manifest with SHA256, license, dimensions, max tokens, supported runtime
3. no remote inference
4. optional download only if explicitly enabled
5. fallback to current ranker if model is unavailable

Recommended initial candidates:

1. Jina-ColBERT-v2 family for multilingual late interaction
2. BGE-M3 multi-vector mode if a practical local runtime path is easier

Avoid:

1. English-only MS MARCO rerankers as default
2. hosted rerank APIs
3. lexical sparse head as primary ranker

## Implementation Plan

Phase 0: feasibility spike

1. create `internal/core/semanticrank` package
2. add a deterministic fake late-interaction backend for tests
3. define model manifest format under PAL
4. add CLI diagnostic `doctor` visibility for model availability
5. prove reranker can run in shadow mode without changing selections

Phase 1: product integration

1. insert reranker after candidate-intelligence and before final limiting
2. add config flags for `semantic_rerank=off|shadow|on`
3. default to `shadow` until benchmark gate passes
4. record score, confidence, model ID, and reason metadata into candidate diagnostics
5. add analytics fields for reranker usage and latency

Phase 2: local model runtime

1. add ONNX Runtime or equivalent backend behind PAL
2. implement CPU-only default path
3. add model cache verification
4. add bounded memory use and timeout guard
5. add fallback on model load failure

Phase 3: release gate

1. run deterministic suites
2. run 100-case seedless with shadow comparison
3. run 100-case seedless with reranker enabled
4. compare failure taxonomy before and after
5. release only if pass-rate gain exceeds gate and latency stays acceptable

## Tests

Unit tests:

1. reranker unavailable fallback
2. stable score ordering for fake backend
3. no panic on empty snippets or missing source context
4. multilingual candidate text bundle construction
5. diagnostics include model and score metadata

Integration tests:

1. `web_query` seedless with fake reranker promoting semantically correct candidate
2. fallback path when model missing
3. PAL model cache corruption detection
4. timeout path does not fail the query

Benchmark tests:

1. 100-case seedless no-key browser-like semantic lane
2. warm memory 100-case lane
3. seeded unique-source 100-case lane
4. multilingual live-semantic-global suite

Ablations:

1. baseline current ranker
2. late interaction only
3. graph memory only
4. late interaction plus graph memory
5. late interaction plus semantic quorum

## Metrics

Primary:

1. selected pass rate
2. `right_family_not_selected` reduction
3. top-1 semantic representative accuracy
4. p50/p95 rerank latency
5. local model failure rate

Secondary:

1. candidate pool contains expected family
2. reranker promoted selected candidate
3. reranker confidence calibration
4. memory bytes used
5. model load time

## Devil's Advocate

Objection: late interaction is too heavy for a CLI/MCP tool.

Response: rerank only the top 8 to 20 candidates. Do not use it as first-stage retrieval. Use timeouts and fallback. If p95 latency exceeds the gate, keep shadow mode only.

Objection: multilingual rerankers may still underperform low-resource languages.

Response: keep a global evaluation lane and track language clusters. Do not assume one model solves all languages. Allow future model adapters without changing ranking architecture.

Objection: model bundling increases installer complexity.

Response: do not bundle large models into the core binary. Use PAL model cache with explicit manifest and optional acquisition. The core product remains usable without the model.

Objection: late interaction can overfit benchmark candidates.

Response: split benchmark by entity family and domain. Evaluate on unseen domains and multilingual pages. Require failure-taxonomy improvement, not only pass-rate improvement.

Objection: using titles and URLs in candidate text reintroduces literal bias.

Response: title and URL are allowed only as provenance fields or weak context. The score must be dominated by semantic embeddings over summary, source context, role, and family evidence.

## Acceptance Criteria

1. `semanticrank` package exists with adapter interface and fake backend.
2. `web_query` can run late-interaction rerank in shadow mode.
3. Candidate diagnostics expose reranker metadata compactly.
4. No API key is required.
5. 100-case seedless pass rate improves by at least `+10` points before enabling by default.
6. No deterministic benchmark regression.
7. No site-specific or language-specific ranking rule is introduced.
