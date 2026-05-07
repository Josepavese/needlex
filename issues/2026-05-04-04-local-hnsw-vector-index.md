# Issue 04: Local HNSW Vector Index

Status: exact vector index PAL implemented; HNSW backend intentionally not shipped until portability and recall gates pass
Date: 2026-05-04
Priority: P1 infrastructure, P0 once memory grows beyond exact-scan comfort
Surface: Discovery Memory, graph memory, semantic candidate recall, local analytics

## Objective

Add a local vector index PAL that can scale Discovery Memory and entity-family graph retrieval without degrading latency as embeddings grow.

The current memory can remain exact-scan while small. The jump happens when Needle-X has thousands to millions of resource, topic, entity, role, and evidence embeddings. At that point, high-quality approximate nearest-neighbor search becomes core infrastructure.

## Semantic Contract

This issue is infrastructure for embeddings-first retrieval.

It must support:

1. multilingual dense embeddings
2. entity/topic/resource embeddings
3. graph node embeddings
4. exact fallback for correctness checks
5. local-only operation
6. no API keys
7. PAL abstraction so the backend can be replaced

It must not introduce:

1. keyword indices as primary retrieval
2. external vector database dependency
3. hosted vector search
4. provider-specific retrieval semantics
5. benchmark-only shortcuts

## Research Basis

Primary references:

1. [Efficient and robust approximate nearest neighbor search using HNSW](https://arxiv.org/abs/1603.09320)
2. [Billion-scale similarity search with GPUs](https://arxiv.org/abs/1702.08734)
3. [ONNX Runtime official docs](https://onnxruntime.ai/docs)
4. [ONNX Runtime execution providers](https://onnxruntime.ai/docs/execution-providers)
5. [Hubness Reduction Improves Sentence-BERT Semantic Spaces](https://arxiv.org/abs/2311.18364)
6. [Multilingual E5 Text Embeddings](https://arxiv.org/abs/2402.05672)

Interpretation for Needle-X:

1. HNSW is a proven graph-based ANN method with strong practical performance.
2. Exact scan is simpler and safer for small corpora; HNSW becomes necessary when memory grows.
3. Embedding spaces can suffer hubness, so ANN needs quality monitoring and not only speed metrics.
4. Runtime/model infrastructure should be PAL-based because local CPU/GPU capabilities vary by platform.

## Product Hypothesis

A local vector index will let Needle-X preserve strong semantic recall while memory grows, making Discovery Memory a durable product advantage instead of a small-cache optimization.

Target impact:

1. memory search p95 below `50 ms` at `100k` embeddings on commodity CPU
2. recall@20 above `0.97` compared to exact scan on sampled audits
3. no seedless quality regression
4. graph activation latency remains bounded
5. startup remains fast by lazy-loading index metadata

## Architecture

Add a vector index PAL:

```go
type VectorIndex interface {
    Name() string
    Dimension() int
    Upsert(ctx context.Context, item VectorItem) error
    Delete(ctx context.Context, id string) error
    Search(ctx context.Context, query []float32, opts SearchOptions) ([]VectorHit, error)
    Stats(ctx context.Context) (VectorIndexStats, error)
    AuditRecall(ctx context.Context, sample RecallAuditSample) (RecallAuditResult, error)
}
```

Backends:

1. `exact`: pure Go exact cosine/dot search for small corpora and tests
2. `hnsw`: Go-native or embedded native backend behind adapter
3. `shadow`: queries both exact sample and ANN to measure recall drift

Persistence:

1. SQLite remains SSOT for metadata
2. index files live under PAL `discovery/vector_index/`
3. embeddings have model ID and dimension
4. index is rebuildable from SQLite embeddings
5. index metadata includes parameters and corpus fingerprint

## HNSW Parameters

Expose conservative settings:

1. `M`
2. `efConstruction`
3. `efSearch`
4. distance metric
5. quantization mode if used
6. recall-audit sample rate

Do not expose these as casual MCP parameters. They are maintainer/runtime config.

## Hubness and Quality Controls

Potential controls:

1. normalize embeddings consistently
2. track hub frequency by vector ID
3. apply score margin checks before trusting top-1
4. sample exact recall audits
5. support CSLS or mutual-proximity style damping later if hubness is observed
6. avoid single nearest-neighbor decisions; retrieve top-k then rerank semantically

## Implementation Plan

Phase 0: PAL and exact backend

1. create `internal/core/vectorindex`
2. implement exact backend
3. add SQLite embedding reader/writer abstraction
4. add stats and recall audit structs
5. integrate memory search through interface without behavior change

Phase 1: HNSW backend spike

1. evaluate Go-native HNSW libraries and embedded options
2. require Linux/macOS/Windows support
3. require deterministic persistence/rebuild
4. benchmark insert/search/delete behavior
5. choose backend only if it satisfies release portability

Phase 2: shadow mode

1. run exact and ANN on sample queries
2. report recall loss
3. record latency and memory use
4. keep exact fallback for small memory
5. add `doctor` visibility

Phase 3: active mode

1. enable HNSW above configurable corpus size
2. keep exact rerank of top candidates if needed
3. rebuild index on model version changes
4. add corruption detection and automatic rebuild
5. add backup/restore integration with PAL

## Tests

Unit tests:

1. exact backend returns stable nearest neighbors
2. vector dimension mismatch fails cleanly
3. model ID mismatch prevents accidental mixed-space search
4. delete/upsert idempotency
5. stats reflect count and dimensions

Integration tests:

1. memory search uses vector index interface
2. index rebuild from SQLite works
3. corrupt index file triggers rebuild
4. shadow recall audit records results
5. cross-platform path handling under PAL

Benchmark tests:

1. `1k`, `10k`, `100k` synthetic multilingual embeddings
2. exact vs HNSW recall@k
3. latency p50/p95
4. memory footprint
5. seedless and warm-memory pass rates before/after

## Metrics

Primary:

1. recall@k against exact scan
2. p50/p95 search latency
3. memory footprint
4. rebuild time
5. index corruption/rebuild count

Semantic quality:

1. selected pass rate
2. right-family candidate recall
3. hub frequency distribution
4. low-resource language recall
5. graph activation latency

## Devil's Advocate

Objection: ANN can miss the exact best candidate.

Response: use top-k retrieval and semantic rerank. Run exact recall audits. Keep exact backend for small datasets and tests.

Objection: native HNSW dependencies complicate releases.

Response: the interface lands first with exact backend. HNSW backend ships only if it satisfies current release matrix.

Objection: vector index speed does not improve quality by itself.

Response: correct. This is infrastructure. Its product value comes from allowing graph memory and local semantic recall to scale.

Objection: hubness can make vector search misleading.

Response: track hubness explicitly. Do not select directly from ANN top-1. Use graph, reranker, and confidence margins.

Objection: multiple embedding models create incompatible vector spaces.

Response: every vector stores model ID, dimension, normalization, and schema version. Mixed-space search is rejected.

## Acceptance Criteria

1. Vector index PAL exists with exact backend.
2. Memory search goes through the PAL without behavior regression.
3. HNSW backend is selected only after portability benchmark.
4. Recall audit exists and is visible in diagnostics.
5. No external vector DB or API key is required.
6. No lexical primary retrieval is introduced.
