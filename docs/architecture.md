# Architecture

## Goal

Needle-X must deliver a full Day 1 runtime without turning into a sprawling codebase. The architecture is therefore constrained before implementation starts.

## Runtime Topology

1. One public command: `needlex`
2. Internal entrypoint: `cmd/needle`
2. One runtime process
3. One core execution pipeline
4. One transport-neutral service API reused by CLI and MCP

Canonical flow:

```text
Acquire -> AgentReadableResolve -> Reduce -> RenderIfNeeded -> Segment -> ExtractDet -> SemanticGate -> (+ResolveAmbiguity) -> ChunkRank -> Pack -> Proof -> Trace
```

## Internal Package Plan

The repo should converge on these `internal/` packages and stay below the package budget:

1. `config`
Load config, defaults, policy thresholds, and runtime budgets.

2. `core`
Own canonical domain types and the main application service entrypoints.

3. `pipeline`
Own deterministic stages: acquire, reduce, segment, extract_det, chunk, rank, pack.

4. `intel`
Own model adapter, router, judge, formatter, and ambiguity policy logic.

5. `proof`
Own proof artifacts, trace events, replay, and diff.

6. `store`
Own local persistence for traces, fingerprints, cache, and domain genome.

7. `transport`
Own thin CLI and MCP adapters that call `core`.

## Design Invariants

1. Deterministic pipeline is the default path and must stay operable offline.
2. Model calls are policy-gated, benchmark-backed, and must emit explicit reason codes.
3. Every successful run emits proof and trace artifacts.
4. CLI and MCP never implement business logic directly.
5. Domain genome and fingerprint graph are storage concerns, not new execution pipelines.
6. The default transport contract must be the compact agent-facing packet, not the diagnostic envelope.

## Retrieval Layering

Needle-X retrieval is organized as replaceable layers:

1. `transport` adapters parse CLI/MCP input and render compact-first output
2. `core/service` coordinates product flow but should not own provider-specific logic
3. `core/discovery` owns structural candidate scoring and same-site context priors
4. `core/webdiscover` owns web bootstrap family/entity recovery helpers
5. `memory` owns local semantic recall, topic nodes, graph expansion, import/export, and pruning
6. `intel` owns dense embedding adapters, semantic alignment, and model runtimes
7. `analytics` owns value/reporting state, not retrieval decisions

Replacement rule:
1. change a provider by replacing an adapter
2. change semantic implementation inside `intel`
3. change persistence inside `memory`, `store`, or `analytics`
4. do not push adapter concerns back into ranking or transport code

Experimental seedless discovery order:
1. local Discovery Memory and topic/family recovery
2. same-site recovery when a local family seed exists
3. provider-health ordered public bootstrap
4. rewrite escalation only when the leader is not semantically grounded

Stable agent workflow:
1. the host agent obtains candidate URLs with its own search tool
2. the host agent chooses the breadth without a Needle-X candidate cap
3. Needle-X compiles each selected URL independently through `web_read`
4. seeded `web_query` may route within a known site or documentation family

Seedless execution requires explicit `web_search` selection and is not an automatic transport or service fallback.

## Agent-Readable Source Resolution

Before rendering JavaScript, `core/service` attempts same-origin agent-readable sources with explicit provenance.

Resolution order:
1. declared resources from HTTP `Link` headers
2. declared resources from HTML `<link>` elements
3. shared conventions when the static page is weak: Markdown content negotiation, same-path `.md`/`.mdx`, `/llms.txt`, `/llms-full.txt`, `/.well-known/api-catalog`, OpenAPI/Swagger well-known paths
4. `robots.txt` `Sitemap:` entries and conventional sitemap files as candidate indexes
5. linked resources inside API catalogs, including `service-desc`, `service-doc`, `service-meta`, `describedby`, and OpenAPI/Swagger/AsyncAPI descriptions
6. JavaScript rendering as the final escalation path, including rendered DOM plus same-origin textual network evidence from fetch/XHR bodies, SSE messages, and received WebSocket frames

Rules:
1. candidates must stay same-origin
2. conventional and sitemap-derived probes are filtered through same-origin `robots.txt` policy when robots is available
3. accepted agent-readable candidates are selected semantically when an objective and dense semantic runtime are available
4. protocol and URL strings may classify resource identity, not ranking relevance
5. arbitrary application state locations such as framework globals or site-specific `window.*` payloads are not extraction sources
6. non-JSON-LD `<script>` payloads remain ignored by the reducer
7. JSON-LD is the only script-based structured-data extraction path
8. renderer network payloads are evidence captured through browser/CDP transport provenance, not provider-specific ranking signals

## Render Budget and Network Evidence

Rendering runs only in `auto` or `required` mode, and only for HTML-like content in `auto`.

Escalation in `auto` mode is structural and semantic:
1. structural reasons: thin surface, navigation-like surface, or a client-rendered substrate
2. semantic reason: the objective-to-surface similarity falls below a calibrated threshold, recorded as `semantic_gap_similarity` together with the `semantic_coverage_gap` reason code

Waiting is activity-driven and bounded:
1. a render may wait up to `render.timeout_ms` and never past the remaining operation deadline
2. quiet pages settle as soon as the page and its streams go idle
3. active application requests and open streams keep the render alive while they produce
4. a stream still open at snapshot time is reported as truncated instead of being presented as complete

Network evidence handling:
1. relevant textual resources are retained within `render.network_*` budgets; gzipped payloads are decoded at the transport boundary before they become evidence
2. payloads that remain non-textual after decoding are observed and reported but never enter semantic evidence
3. the DOM plus the synthesized network text is reduced into one page substrate, so captured application data reaches segments, chunks, and the packet

Trace and packet contract:
1. the `render` stage records `render_path`, `network_observed`, `network_resources`, `network_body_missing` with a bounded URL sample, `network_streams_open`, `network_bytes`, `network_truncated`, `network_idle_reason`, and `render_degraded` when the CDP path failed
2. `render_path=dump_dom` means no application data was captured, and that degradation is explicit rather than silent
3. the compact packet exposes `signals.content_source` (`dom` or `dom+network`) with network counters, so an agent can tell rendered DOM from delivered application data

## Runtime State Layout

Runtime-generated local state should live outside source packages, under the platform state root resolved by `NEEDLEX_HOME` or the OS default application data directory.

Default roots:
- Linux: `$XDG_DATA_HOME/needlex` or `~/.local/share/needlex`
- macOS: `~/Library/Application Support/NeedleX`
- Windows: `%LOCALAPPDATA%\NeedleX`

Canonical layout:

```text
<state-root>/
  analytics/
  browsers/
  candidates/
  discovery/
    discovery.db
    providers/
  domain_graph/
  fingerprint_graph/
  fingerprints/
  genome/
  logs/
    needlex.jsonl
  proofs/
  traces/
```

This keeps the repository clean, makes CLI and MCP share one durable PAL home, and preserves replayability, analytics, runtime diagnostics, provider health memory, and local-first discovery behavior across processes.

Diagnostics policy:
1. CLI and MCP write detailed runtime diagnostics to the PAL log, not ad hoc stderr/stdout streams
2. analytics stores aggregate value and failure facts
3. `doctor` correlates the two planes for local inspection
4. `support bundle` exports a portable maintainer snapshot without traces, proofs, fingerprints, or source files by default
