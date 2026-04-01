# Discovery Memory Spec

Status: future capability specification  
Version: `0.3`  
Scope: local-first seedless discovery support without mandatory infrastructure

Primary references:
- [project-context.md](/home/jose/hpdev/Libraries/needlex/docs/project-context.md)
- [vademecum.md](/home/jose/hpdev/Libraries/needlex/docs/vademecum.md)
- [semantic-alignment-gate.md](/home/jose/hpdev/Libraries/needlex/docs/semantic-alignment-gate.md)
- [model-baseline.md](/home/jose/hpdev/Libraries/needlex/docs/model-baseline.md)
- [spec.md](/home/jose/hpdev/Libraries/needlex/spec.md)
- SQLite `vec1`: <https://sqlite.org/vec1>
- `sqlite-vec`: <https://github.com/asg017/sqlite-vec>
- DuckDB `vss`: <https://duckdb.org/docs/stable/core_extensions/vss.html>
- LanceDB docs: <https://docs.lancedb.com/>
- Qdrant docs: <https://qdrant.tech/documentation/>

## 1. Purpose

`Discovery Memory` is the local-first memory layer that allows Needle-X to improve seedless discovery without:
1. mandatory infrastructure
2. per-client full-web indexing
3. hard dependence on public search engines

It is not a global search engine.
It is a local, growing, retrieval-oriented memory of already observed web evidence.

## 2. Product Thesis

Needle-X should not try to become:
1. Google
2. a hosted search engine
3. a browser automation vendor
4. a vector database product

Needle-X should instead:
1. compile pages better than generic scrapers
2. preserve local knowledge from previous reads, queries, and crawls
3. reuse validated web evidence before calling unstable public bootstrap search
4. accumulate retrieval leverage over time without requiring infrastructure

`Discovery Memory` is the mechanism that turns previous verified work into future seedless retrieval leverage.

## 3. Core Definition

`Discovery Memory` is a client-local indexed store of:
1. observed URLs
2. normalized titles
3. host and path metadata
4. compact semantic summaries
5. semantic embeddings
6. outbound link evidence
7. proof/trace/fingerprint references
8. optional entity and locality hints

Its role is:
1. retrieve plausible candidates for a new goal
2. rank them cheaply before public bootstrap
3. reduce dependence on external search providers
4. preserve proof-bearing prior work as future discovery substrate

## 4. Non-Goals

`Discovery Memory` must not:
1. crawl the open web autonomously by default
2. require a daemon or shared server
3. become a global inverted index
4. replace `read/query/crawl`
5. invent entities or canonical sites
6. turn Needle-X into a standalone retrieval platform

## 5. Position in Runtime

Future seedless query flow:

`Goal -> QueryRewrite -> DiscoveryMemory -> PublicBootstrap(best effort) -> SemanticRerank -> Read -> Proof`

More precisely:
1. query arrives without seed
2. `query_rewrite` may generate bounded search variants
3. Needle-X searches local `Discovery Memory`
4. if local recall is sufficient, it stays local
5. if local recall is insufficient, it tries public bootstrap search
6. all candidates then go through semantic/structural reranking

Important:
`Discovery Memory` is the first seedless discovery substrate.
Public search becomes fallback, not primary identity.

## 6. Product Shape

The correct shape is:
1. local-first
2. append-friendly
3. inspectable
4. semantically searchable
5. proof-aware
6. rebuildable without infrastructure

The wrong shape is:
1. opaque vector-first persistence
2. heavyweight ANN stack before product proof
3. service/daemon dependency
4. embeddings as the only truth

## 7. Data Model

### 7.1 DiscoveryDocument

```json
{
  "url": "https://example.com/about",
  "final_url": "https://www.example.com/about",
  "host": "www.example.com",
  "path": "/about",
  "title": "About Example",
  "observed_at": "2026-03-31T10:00:00Z",
  "last_trace_id": "trace_...",
  "proof_refs": ["chk_...", "chk_..."],
  "semantic_summary": "Example is a design studio focused on ...",
  "embedding_ref": "emb_...",
  "language": "it",
  "locality_hints": ["Turin"],
  "entity_hints": ["Example Studio"],
  "category_hints": ["design studio", "branding"],
  "stable_ratio": 0.82,
  "novelty_ratio": 0.05,
  "changed_recently": false,
  "source_kind": "read"
}
```

### 7.2 DiscoveryEdge

```json
{
  "source_url": "https://example.com",
  "target_url": "https://example.com/services",
  "anchor_text": "Services",
  "same_host": true,
  "observed_at": "2026-03-31T10:00:00Z",
  "trace_ref": "trace_..."
}
```

### 7.3 DiscoveryEmbedding

```json
{
  "embedding_ref": "emb_...",
  "document_url": "https://example.com/about",
  "model": "intfloat/multilingual-e5-small",
  "backend": "openai-embeddings",
  "input_text": "About Example\nExample is a design studio focused on ...",
  "dimension": 384,
  "created_at": "2026-03-31T10:00:00Z"
}
```

### 7.4 DiscoveryCandidate

```json
{
  "url": "https://example.com/services",
  "title": "Services",
  "score": 1.74,
  "reasons": [
    "semantic_goal_alignment",
    "entity_hint_match",
    "recent_local_evidence"
  ],
  "trace_ref": "trace_...",
  "proof_ref": "chk_..."
}
```

## 8. Storage Architecture

### 8.1 Recommendation

Recommended implementation path:
1. SQLite as the local store
2. `documents`, `edges`, and `embeddings` in the same local database
3. vector search via embedded SQLite vector support
4. no server, no daemon, no external DB requirement

This is the recommended path because it preserves:
1. local-first discipline
2. inspectability
3. append/update simplicity
4. metadata + vector retrieval in one place
5. low operational surface

### 8.2 SSOT Rule

The persistence truth must remain local and rebuildable.

The correct SSOT is:
1. document metadata
2. edge metadata
3. embedding metadata
4. state/config metadata

Embeddings are part of the memory, but vector indexes are derived artifacts.

The system must preserve this hierarchy:
1. `documents` and `edges` are canonical truth
2. `embeddings` are canonical semantic payloads
3. ANN/vector index structures are rebuildable acceleration artifacts

`Discovery Memory` must never become dependent on a non-rebuildable opaque index.

### 8.3 Storage Tiers

#### Tier A: Canonical local store
Preferred form:
1. SQLite database at `.needlex/discovery/discovery.db`

Minimum tables:
1. `documents`
2. `edges`
3. `embeddings`
4. `memory_state`

#### Tier B: Rebuildable vector acceleration
Possible forms:
1. SQLite `vec1`
2. `sqlite-vec`
3. linear scan fallback if vector extension is unavailable

#### Tier C: Export and inspection
Optional export artifacts:
1. `.needlex/discovery/export/documents.jsonl`
2. `.needlex/discovery/export/edges.jsonl`
3. `.needlex/discovery/export/embeddings.jsonl`

These are for debugging, export, and migration.
They are not required as the primary runtime store if SQLite is adopted.

## 9. Technology Recommendation

### 9.1 Final recommendation: SQLite + vector search embedded

This is the final recommended storage stack.

Why:
1. embedded
2. local
3. no infra
4. easy to ship with Go
5. supports metadata and vector retrieval together
6. inspectable enough for operator-grade debugging

Default final shape:
1. SQLite as canonical store
2. `sqlite-vec` as the initial embedded vector engine
3. linear scan fallback preserved for correctness
4. exports optional, not primary runtime storage
5. migration path kept open toward `vec1` when maturity and Go integration justify it

### 9.2 `vec1` vs `sqlite-vec`

#### `vec1`
Pros:
1. official SQLite direction
2. embedded ANN search
3. strong long-term architectural fit

Tradeoff:
1. newer and lower-level
2. integration path may be less ergonomic initially

#### `sqlite-vec`
Pros:
1. practical and widely used in local-AI workflows
2. easy path to vector similarity in SQLite
3. good fit for local-first experiments and productization

Tradeoff:
1. pre-v1 maturity
2. should be treated as an implementation choice, not product identity

Decision rule:
1. initial implementation uses `sqlite-vec`
2. `vec1` remains the preferred long-term migration target if it becomes cleaner in Go and operationally mature enough
3. preserve linear scan fallback so correctness never depends on vector acceleration availability

### 9.3 Initial engine decision

Needle-X should implement `Discovery Memory` first with:
1. SQLite as canonical store
2. `sqlite-vec` as the active vector engine
3. linear scan fallback always available

Reason:
1. `sqlite-vec` has a more practical adoption path now
2. it is easier to productize quickly in a local-first Go repo
3. it preserves the correct architecture without waiting on the official long-term path

Migration rule:
1. the runtime must hide vector search behind an internal adapter
2. no service code should depend directly on a specific SQLite vector extension
3. migration from `sqlite-vec` to `vec1` must be a backend swap, not a product rewrite

### 9.4 Rejected or deferred options

#### DuckDB + VSS
Good technology, but not recommended as primary storage.

Reason:
1. more analytics-oriented than memory-store-oriented
2. `vss` is still experimental
3. heavier conceptual surface than SQLite for this use case

#### LanceDB
Interesting and modern, but not the recommended default.

Reason:
1. stronger fit for retrieval engine products than for a local-first Go utility
2. bigger stack than needed
3. Go path is not the cleanest primary path

#### Qdrant local / vector DB products
Not recommended.

Reason:
1. too database-product-shaped for the problem
2. less aligned with zero-infra repo usage
3. unnecessary complexity for local warm-state retrieval

## 10. Population Policy

Memory should be populated only from validated observation paths:
1. successful `read`
2. successful `query`
3. successful `crawl`

Population sources:
1. selected URL
2. title and final URL
3. top proof-carrying chunks
4. top outbound same-site or cross-site links
5. semantic summary
6. embedding of `title + semantic_summary`

Do not populate from:
1. failed fetches
2. provider SERP pages
3. rejected ambiguity patches
4. low-confidence or empty reads

## 11. Embedding Policy

Embeddings are required for `Discovery Memory` to be strategically strong.

Without embeddings:
1. memory is inspectable
2. memory is reusable
3. recall is limited by lexical and metadata overlap

With embeddings:
1. local semantic recall becomes viable
2. multilingual recall becomes stronger
3. seedless warm-state discovery becomes materially better

Final embedding input should be bounded and stable:
1. `title + semantic_summary`
2. optional selected entity/locality/category hints

Do not embed:
1. full raw pages
2. boilerplate dumps
3. provider SERP content
4. arbitrary chunk floods

## 12. Retrieval Policy

When a seedless query arrives, retrieval should happen in this order:

### 12.1 Phase 1: Local semantic recall
Use:
1. goal embedding
2. query rewrite variants
3. similarity against stored `title + semantic_summary`

Output:
top `k` local candidates

### 12.2 Phase 2: Graph expansion
From top local candidates:
1. follow recorded edges
2. promote neighboring pages on same host
3. promote frequently co-observed cross-site targets

### 12.3 Phase 3: Freshness and change bias
Adjust scores using:
1. `stable_ratio`
2. `novelty_ratio`
3. `changed_recently`

Bias should be:
1. stable pages downweighted for change-seeking goals
2. novel or recently changed pages slightly upweighted

### 12.4 Phase 4: Best-effort public bootstrap
Only if local candidate quality is insufficient:
1. use configured external provider chain
2. merge with local candidates
3. rerank again semantically and structurally

## 13. Scoring Signals

Primary signals:
1. semantic goal alignment
2. entity hint match
3. locality hint match
4. host/domain recurrence in prior successful runs
5. proof-backed density of the remembered page
6. recency and novelty bias
7. graph proximity to previously trusted pages

Auxiliary only:
1. lexical overlap in titles
2. path token coincidence

Meaning must stay semantic-first.
No linguistic heuristics as primary decision logic.

## 14. Query Rewrite Interaction

`query_rewrite` and `Discovery Memory` are complementary.

`query_rewrite` does:
1. bounded reformulation
2. entity preservation
3. locality/category hint extraction

`Discovery Memory` does:
1. local recall
2. local graph reuse
3. candidate reuse before public bootstrap

The combined effect is:
1. better recall without infrastructure
2. lower dependence on fragile public search
3. stronger local-first moat

## 15. Final Implementation Shape

### 15.1 Final runtime commitments

The implementation must include all of the following:
1. canonical SQLite store
2. embeddings persisted locally
3. vector retrieval embedded when available
4. correctness-preserving linear scan fallback
5. automatic population from validated `read/query/crawl`
6. seedless lookup against memory before public bootstrap
7. bounded pruning and operator controls

### 15.2 Final SQLite schema

Canonical file:
1. `.needlex/discovery/discovery.db`

Required tables:

```sql
CREATE TABLE documents (
  url TEXT PRIMARY KEY,
  final_url TEXT NOT NULL,
  host TEXT NOT NULL,
  path TEXT NOT NULL,
  title TEXT NOT NULL,
  semantic_summary TEXT NOT NULL,
  language TEXT,
  locality_hints_json TEXT NOT NULL,
  entity_hints_json TEXT NOT NULL,
  category_hints_json TEXT NOT NULL,
  proof_refs_json TEXT NOT NULL,
  last_trace_id TEXT NOT NULL,
  source_kind TEXT NOT NULL,
  stable_ratio REAL NOT NULL,
  novelty_ratio REAL NOT NULL,
  changed_recently INTEGER NOT NULL,
  observed_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX idx_documents_host ON documents(host);
CREATE INDEX idx_documents_observed_at ON documents(observed_at);
```

```sql
CREATE TABLE edges (
  source_url TEXT NOT NULL,
  target_url TEXT NOT NULL,
  anchor_text TEXT NOT NULL,
  same_host INTEGER NOT NULL,
  trace_ref TEXT NOT NULL,
  observed_at TEXT NOT NULL,
  PRIMARY KEY (source_url, target_url, anchor_text)
);

CREATE INDEX idx_edges_source_url ON edges(source_url);
CREATE INDEX idx_edges_target_url ON edges(target_url);
```

```sql
CREATE TABLE embeddings (
  embedding_ref TEXT PRIMARY KEY,
  document_url TEXT NOT NULL UNIQUE,
  model TEXT NOT NULL,
  backend TEXT NOT NULL,
  input_text TEXT NOT NULL,
  dimension INTEGER NOT NULL,
  vector BLOB NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

```sql
CREATE TABLE memory_state (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

Optional acceleration objects:
1. vector virtual table or index for `embeddings.vector`
2. rebuildable from canonical `embeddings` rows

### 15.3 Final internal interfaces

Required internal interfaces:

```go
type MemoryStore interface {
    UpsertDocument(ctx context.Context, doc DiscoveryDocument) error
    UpsertEdges(ctx context.Context, edges []DiscoveryEdge) error
    UpsertEmbedding(ctx context.Context, emb DiscoveryEmbedding, vector []float32) error
    SearchByVector(ctx context.Context, vector []float32, limit int) ([]DiscoveryCandidate, error)
    ExpandNeighbors(ctx context.Context, urls []string, limit int) ([]DiscoveryCandidate, error)
    GetStats(ctx context.Context) (MemoryStats, error)
    Prune(ctx context.Context, policy PrunePolicy) error
    RebuildIndex(ctx context.Context) error
}
```

```go
type MemoryIndexer interface {
    BuildInput(doc DiscoveryDocument) string
    EmbedGoal(ctx context.Context, goal string) ([]float32, error)
    EmbedDocument(ctx context.Context, text string) ([]float32, error)
}
```

```go
type MemoryService interface {
    PopulateFromRead(ctx context.Context, result ReadResult) error
    PopulateFromQuery(ctx context.Context, result QueryResult) error
    PopulateFromCrawl(ctx context.Context, result CrawlResult) error
    Search(ctx context.Context, goal string, opts MemorySearchOptions) ([]DiscoveryCandidate, error)
}
```

### 15.4 Final seedless query flow

The runtime must execute this order:
1. receive seedless goal
2. compute query rewrite variants if allowed
3. embed goal and variants
4. search `Discovery Memory`
5. expand graph neighbors from top local results
6. score and filter local candidates
7. if local quality is sufficient, stop before public bootstrap
8. otherwise call configured provider chain
9. merge all candidates and rerank semantically/structurally
10. continue normal `read -> proof` path

### 15.5 Explicitly deferred even in final product
1. daemonized memory service
2. remote shared memory
3. complex graph ranking
4. chunk-level multi-vector indexing by default
5. full local web indexing

## 16. Acceptance Conditions

The feature is successful only if it improves seedless discovery without breaking product discipline.

Required outcomes:
1. improves seedless correctness after warm-up
2. reduces public provider activation rate
3. preserves explainability and provenance
4. does not require background infrastructure
5. keeps storage understandable and rebuildable

Non-acceptable outcomes:
1. hidden autonomous crawling
2. opaque ranking without proof or reasons
3. large cold-start cost that hurts local usability
4. memory growth without pruning discipline
5. vector infrastructure that is harder than the product itself

## 17. Pruning and Hygiene

Memory must be bounded.

Required controls:
1. max stored documents
2. max embeddings
3. stale-entry pruning
4. host-level cap
5. duplicate URL collapse
6. rebuild vector index from canonical rows

Suggested retention policy:
1. keep recent successful pages
2. keep semantically central pages
3. drop low-value duplicates and boilerplate hubs

## 18. Operator Controls

Future CLI should expose:
1. `needle memory stats`
2. `needle memory search --goal "..."`
3. `needle memory prune`
4. `needle memory export`
5. `needle memory import`
6. `needle memory rebuild-index`

Future MCP should expose equivalent tools.

## 19. Benchmark Plan

The feature must be benchmarked in two regimes.

### 19.1 Cold start
No prior memory.

Measures:
1. no-regression versus current seedless behavior
2. public bootstrap fallback still works when available

### 19.2 Warm state
Local memory already contains previously visited relevant pages.

Measures:
1. seedless correctness rate
2. public provider activation rate
3. latency delta
4. candidate quality
5. proof-bearing final selection rate
6. warm-state local recall rate

## 20. SSOT Requirements

If implemented, `Discovery Memory` must be SSOT-backed like models and discovery providers.

Minimum manifest fields:
1. `memory.enabled`
2. `memory.backend`
3. `memory.path`
4. `memory.max_documents`
5. `memory.max_edges`
6. `memory.max_embeddings`
7. `memory.embedding_model`
8. `memory.embedding_backend`
9. `memory.vector_mode`
10. `memory.vector_engine`
11. `memory.prune_policy`

No hardcoded memory behavior in service constructors.

## 21. Failure Classes

Expected failure classes:
1. cold-start empty memory
2. stale memory bias
3. entity drift
4. host overfitting
5. duplicate result inflation
6. memory pollution from weak pages
7. vector index stale versus canonical rows
8. embedding backend mismatch after migration

The system must classify these explicitly.

## 22. Strategic Value

If successful, `Discovery Memory` becomes:
1. a local-first search substrate
2. a client-side moat
3. a seedless discovery amplifier
4. a way to keep Needle-X useful without mandatory infrastructure
5. a semantic memory layer that compounds with product usage

This is not a generic search engine strategy.
It is a product-consistent local knowledge strategy.

## 23. Exit Criteria For Future Implementation

The implementation phase is complete when:
1. local memory populates automatically from validated reads/queries/crawls
2. seedless query checks local memory before public bootstrap
3. warm-state benchmark shows meaningful gain
4. operator controls exist for inspect/prune/export
5. no infrastructure is required to benefit from the feature
6. vector acceleration is optional or rebuildable, never mandatory for correctness
