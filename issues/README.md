# Issues

This folder tracks engineering issues and product-development project plans that are too large or strategic to live as transient benchmark artifacts.

Rules:
1. Keep plans implementation-oriented.
2. State non-goals and failure risks explicitly.
3. Preserve the Needle-X semantic-first, multilingual, embeddings-first, zero-literal philosophy.
4. Do not place generated benchmark JSON artifacts here.
5. Do not use provider-specific or site-specific recipes as product architecture.

Maintenance review: 2026-11-15, against v0.1.52. This index is the status
surface; the individual files retain their original plans and evidence.
Closed files are kept for provenance and must not be deleted merely because
they are old.

## Active

1. [Local late-interaction reranker](2026-05-04-01-local-late-interaction-reranker.md) — implemented as a bounded reranker; the `+10` benchmark gate and active-release decision remain open.
2. [Trace-driven semantic calibration](2026-05-04-03-trace-driven-semantic-calibration.md) — feature registry and static calibrator implemented; the trace-trained artifact remains pending.
3. [Local HNSW vector index](2026-05-04-04-local-hnsw-vector-index.md) — exact vector-index PAL implemented; the HNSW backend remains intentionally unshipped pending portability and recall gates.
4. [Brevo web_query selected unrelated MiniMax docs](2026-05-25-brevo-web-query-selected-minimax.md) — still reproducible on v0.1.52; a Brevo goal selected `https://www.icloud.com.cn/find` instead of Brevo sources.

## Closed

1. [Core embeddings runtime](2026-05-07-core-embeddings-runtime.md) — closed in v0.1.27; current CLI/MCP/install gates still exercise the embeddings-first contract.
2. [PAL embedding cache](2026-05-08-pal-embedding-cache.md) — closed; the current runtime includes the documented cache, eviction, observability, and prune paths.
3. [Entity-family graph memory](2026-05-04-02-entity-family-graph-memory.md) — closed in v0.1.52 with persistence, active recall, diagnostics, export, and import tests.
4. [Semantic quorum provider fusion](2026-05-04-05-semantic-quorum-provider-fusion.md) — closed in v0.1.52 with provider observation and semantic cluster quorum tests.
5. [Renderer network-aware application data](2026-06-04-renderer-network-aware-application-data.md) — implemented and smoke-tested; the delivery gaps found afterwards are tracked separately.
6. [Renderer network delivery](2026-09-17-renderer-network-delivery.md) — closed in v0.1.36; the residual risk about empty trace metadata is explicitly retained in the document.
7. [External-agent compact output and evidence coverage](2026-10-15-external-agent-compact-output-and-evidence-coverage.md) — closed in v0.1.46.
8. [Brevo CLI reference header-only read](2026-05-25-brevo-cli-reference-header-only-read.md) — closed as of the 2026-11-15 check against v0.1.52; the reader now returns the Markdown body instead of page chrome.

The two v0.1.52 closures retain their benchmark claim as release-level residual
evidence: the implementation is closed, but a future deterministic benchmark
regression reopens the issue.

## Historical / superseded

1. [Semantic Jump Development Plan](2026-05-04-semantic-jump-index.md) — historical plan; the five individual issues above are authoritative and this file is not an active work queue.
2. [v0.1.27 post-release performance baseline](2026-05-08-v0.1.27-post-release-performance-baseline.md) — historical measurement snapshot, not evidence for the current v0.1.52 release.
