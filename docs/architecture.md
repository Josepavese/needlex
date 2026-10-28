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

## Current Internal Responsibilities

The runtime has twelve top-level `internal/` areas. Subpackages in `core`
separate retrieval policies and pure transformations from service orchestration.

| Area | Responsibility |
| --- | --- |
| `config` | Configuration, defaults, policy thresholds, budgets, and model baseline |
| `core` | Domain types, read/query/crawl orchestration, source resolution, discovery, ranking, and WebIR |
| `pipeline` | HTTP acquisition, content reduction, and segmentation |
| `intel` | Dense embeddings, semantic alignment, bounded model tasks, and model adapters |
| `rendering` | Headless browser execution, CDP, and application-data capture |
| `proof` | Proof artifacts, trace recording, replay, and diff |
| `store` | Domain validation and persistence rules for traces, proofs, fingerprints, candidates, graphs, genomes, and provider health |
| `memory` | Semantic discovery memory and its SQLite adapter |
| `analytics` | Run observations, aggregate reports, and their SQLite adapter |
| `observability` | Runtime diagnostic logging |
| `platform` | Platform state layout, JSON document backend, and shared native primitives |
| `transport` | CLI/MCP input, shared runtime invocation, persistence wiring, and packet presentation |

Analytics consumers use `analytics.Reports` for views and exports and
`analytics.Recorder` for observations. The Runner composition point assembles
the SQLite adapter for CLI, MCP, doctor, support, and runtime observations;
those consumers do not instantiate or depend on its concrete type. Instances
are synchronous operation-scoped handles; transactions and connection cleanup
belong to the adapter.

Host, provider, and daily aggregates share `RollupMetrics`, one SQL metric
projection and decoder, and one textual rendering of those metrics. Host and
provider grouping share their query; daily grouping keeps its time ordering.
JSON fields remain flat and the database schema is unchanged. Analytics CLI
commands share one argument parser, with their supported flags and validation
preserved; CLI and MCP use the same client limit defaults.

Local-state preparation and observation consume the candidate, domain-graph,
genome and fingerprint repository contracts in `store.LocalState`. Semantic
recall consumes `memory.Searcher`; runtime observations and embedding refresh
consume `memory.Runtime`; maintenance consumes `memory.LocalStore`. The Runner
composes these adapters once per operation boundary. State policies do not
construct stores or embedding clients. The JSON repositories remain active,
including recovery paths when SQLite discovery memory is disabled.
Candidate and graph observations initialize missing files only. Invalid or
unreadable existing state returns an error and is preserved; observation never
silently replaces it with an empty repository.

All memory commands share CLI parsing and configuration loading. Across CLI
families, interspersed argument normalization reads value-taking and boolean
options from the registered `flag.FlagSet`, with no parallel flag-name lists.
It preserves assigned values, negative numeric values, and the `--` terminator.
This also fixes previously documented value flags being reordered incorrectly
when followed by boolean flags, such as `--config path --json`.

JSON domain stores consume the `platform.JSONDocuments` contract. Their
factories compose the filesystem backend using directories from `StateLayout`;
they do not perform native file I/O or JSON serialization themselves. The
backend owns file naming, indented encoding, directory creation, reads, writes,
and ordered enumeration. Schema validation and domain-specific missing-record
errors stay in each store. Both proof-ID and chunk-ID lookup use one validated,
ordered scan of the proof collection.

This consolidation preserves existing JSON schemas, filenames, permissions,
and diagnostic context. Reads do not create state; writes remain synchronous
and require the same caller coordination as before. No data conversion is
needed. State pruning is a separate existing filesystem operation.

Dense alignment, exact vector search, and semantic memory share the pure
`core/vectorindex.Cosine` operation. It accumulates float32 vector products in
float64 and leaves interpretation of negative scores to the caller. Memory
family policies and cross-alignment therefore keep their own domain decisions
without separate implementations of vector arithmetic.

Provider-chain usage is classified in `core/discovery`, shared by analytics
and compact packets. This classification describes provenance only and never
affects relevance ranking.

Structural text measurements (average trimmed nonempty line length in runes,
and sentence punctuation count) belong to `core` and are reused by selection
and packet presentation. Each consumer retains its own thresholds. Fingerprint
annotations clone candidate metadata before writing; discovery metadata merge
keeps its distinct provider-provenance aggregation and blank-value policy.
Validation uses `errors.Join`; integer bounds and map copying use the standard
operations rather than parallel implementations.

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

MCP owns a lazy rendering runtime across read, query and crawl operations.
It reuses one Chromium process, with a fresh CDP browser context per render.
Cookies, storage, caches and service workers belong to that isolated context;
the context is disposed before releasing its browser lease. Each render keeps
its own CDP connection and network collector. Configuration changes replace
the runtime between operations. EOF, transport failure and shutdown close it.

`config.DefaultRenderConfig` owns lifecycle defaults. `render.max_concurrency`
bounds active contexts; `render.browser_idle_timeout_ms` retires an unused
browser; `render.browser_max_uses` bounds its total acquisitions before recycle.
These limits must be positive. Network/content/time budgets still apply to
every operation. They bound process count, concurrency, payloads and lifetime;
they are not an operating-system limit on the browser's total resident memory.
The degraded dump-DOM fallback reserves the same native process slot exclusively.

The PAL owns native launch, endpoint discovery, bounded logs, scheduling,
process/profile retirement and process-group cleanup where supported. The
renderer consumes that resource contract and owns the CDP capture protocol.
CLI services use the same capture implementation with operation-scoped browsers
and retain their service-wide concurrency gate. No browser singleton or second
capture pipeline is introduced. See [reuse qualification](chromium-reuse.md).

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
  configs/
    needlex.json
  data/
    embeddings/
      cache/
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
