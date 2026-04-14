# Analytics PAL Spec

Status: experimental candidate  
Updated: 2026-04-14

## Thesis

Needle-X already does much more than a generic scraper or search wrapper, but most of that value is invisible.

Today the product exposes:
1. proof
2. trace
3. compact packets
4. local memory
5. semantic reranking
6. seedless recovery

But it does not yet expose a durable, queryable accounting of:
1. what work was avoided
2. what work was compressed
3. what work was reused
4. where time was spent
5. where quality was won or lost
6. how much public bootstrap was avoided
7. how much burden was removed from the downstream LLM

This spec proposes an `Analytics PAL`:
1. a platform abstraction layer for telemetry and measurement
2. persistent and local-first
3. product-facing rather than debugging-only
4. fast enough to run on every request
5. trustworthy enough to drive claims, benchmarks, and UX

The goal is not vanity metrics.
The goal is to make Needle-X legible as a system.

## Product Facade First

The analytics system should have two faces:

1. a **front-of-house** layer for product perception and user delight
2. a **back-of-house** layer for engineering truth, diagnostics, and optimization

The front face is not optional.
Needle-X needs visible, legible, high-signal numbers that create immediate product understanding and an honest "wow effect".

That means the analytics PAL should deliberately support:
1. **marketing-facing counters**
2. **user-visible leverage metrics**
3. **deep maintainer metrics**

In that order of presentation.

The correct pattern is:
1. headline numbers first
2. decompositions second
3. internal diagnostics third

Not the reverse.

## Front-of-House Metrics

These are the numbers that should appear first in CLI dashboards, MCP summaries, README visuals, and product demos.

They are allowed to be emotionally legible and product-oriented.
They are **not** allowed to be fake, ungrounded, or impossible to explain.

### Headline metrics

Recommended top-level "wow" metrics:

1. **Chars Saved for the Agent**
   - how many raw characters Needle-X prevented from reaching the downstream LLM
   - this is one of the cleanest visible value metrics

2. **Context Compression Ratio**
   - percentage reduction from raw fetched content to final compact packet
   - strong product signal when paired with proof usability

3. **Bootstrap Avoided**
   - how many queries were resolved from local memory / same-site recovery without public bootstrap
   - this communicates compounding product intelligence

4. **Proof-Backed Answers Delivered**
   - number or rate of packets with actionable proof references
   - this is a trust metric, not just a quality metric

5. **Web Work Avoided**
   - pages, links, and candidate branches Needle-X did not need to explore because local/semantic recovery succeeded
   - makes the hidden planning work visible

6. **Agent Time Saved**
   - estimated latency or downstream work avoided compared to a raw-page or public-bootstrap path
   - must be presented as estimate, not fake certainty

7. **Topic Roots Recovered**
   - how often Needle-X returned the correct overview/root instead of a too-specific leaf
   - especially important now that `topic-first` memory exists

8. **Warm-State Lift**
   - quality and latency lift from accumulated memory over cold state
   - this is one of Needle-X's signature product differentiators

### Facade principles

The front metrics should be:
1. compact
2. cumulative over time
3. session-visible
4. emotionally legible
5. numerically defensible

The front metrics should not be:
1. raw debug counters dumped without framing
2. implementation-detail heavy
3. optimized independently from correctness

### The right kind of "marketing numbers"

The system should explicitly support metrics that feel big and impressive, for example:

1. `total_raw_chars_processed`
2. `total_agent_chars_saved`
3. `total_proof_backed_packets`
4. `total_public_bootstraps_avoided`
5. `total_topic_root_corrections`
6. `total_hosts_understood`
7. `total_links_explored`
8. `total_memory_reuse_events`

But each one must have:
1. a canonical event basis
2. a derivation rule
3. a bounded semantic meaning

That is the difference between:
1. product metrics
2. vanity theater

## Anti-Vanity Rule

Needle-X should absolutely expose marketing-facing numbers.

But every headline metric must satisfy all three:

1. **derivable**
   - computed from canonical events
2. **auditable**
   - explainable through lower-level counters
3. **paired**
   - shown together with at least one quality/trust counter where relevant

Examples:

1. `Chars Saved` should be paired with:
   - `Proof-Backed Rate`
   - or `Packet Usability`

2. `Compression Ratio` should be paired with:
   - `Summary Present`
   - `Proof Usable`

3. `Bootstrap Avoided` should be paired with:
   - `Warm Success Rate`

This keeps the front face impressive without becoming dishonest.

## Product Goal

The analytics substrate should let Needle-X answer questions such as:

1. how many raw characters were ingested vs how many useful characters were emitted?
2. how much packet reduction did Needle-X achieve?
3. how often did local memory avoid public bootstrap?
4. how much latency was spent in fetch, reduce, rerank, proof, and memory?
5. which hosts produce the most useful evidence per unit cost?
6. how often does seedless succeed cold vs warm?
7. how often do topic nodes beat flat page retrieval?
8. how much value is the agent getting from Needle-X over direct web search or raw page reads?

And at the product surface it should immediately answer:

1. how much work did Needle-X save the agent?
2. how much web work did Needle-X avoid?
3. how much content did Needle-X compress?
4. how much trust did Needle-X preserve through proof?

## Design Requirements

The analytics layer must be:

1. append-friendly
2. crash-safe
3. low-overhead
4. queryable locally
5. exportable
6. privacy-conscious
7. derived-metric friendly
8. replay-compatible with trace/proof state

The layer must not:

1. require external infrastructure
2. block the hot path on expensive aggregation
3. depend on opaque hosted observability products
4. become a second trace system with duplicated semantics

## Core Idea

Needle-X should treat analytics as a first-class state plane.

Not:
1. scattered counters in ad hoc structs
2. benchmark-only measurement
3. best-effort debug logs

But:
1. canonical events
2. canonical dimensions
3. derived materializations
4. stable semantics across CLI, MCP, benchmark, and runtime

## Architectural Position

Target layering:

1. `trace/proof` = causal evidence of what happened in one run
2. `memory` = reusable semantic state extracted from prior runs
3. `analytics PAL` = quantitative substrate that measures system behavior over time

Analytics is not proof.
Analytics is not discovery memory.
Analytics sits beside them and consumes their outputs.

## Scientific and Product Rationale

Needle-X is moving from a utility into an agent infrastructure product.
That requires measurements at three levels:

1. `mechanistic`
   - bytes, chars, timings, retries, memory growth
2. `retrieval quality`
   - local vs public provider activation, family recovery, proof usability
3. `agent leverage`
   - packet reduction, context compression, work avoided for downstream LLMs

Without all three, the system can look either:
1. technically sophisticated but product-opaque
2. product-promising but scientifically ungrounded

The PAL is meant to close that gap.

## Recommended Storage Shape

### Canonical store

Use SQLite as the canonical analytics store.

Reasons:
1. already aligned with the project's local-first philosophy
2. operationally simple
3. durable and inspectable
4. fast enough for append-heavy usage
5. supports JSON, indexes, and derived tables without infrastructure

### Why not a pure time-series engine?

Not recommended as the first implementation.

Reasons:
1. Needle-X needs mixed relational + event + rollup queries
2. dimensions such as host, provider, topic family, lane, proof usability, and memory source are relational, not just time-series labels
3. SQLite is sufficient until proven otherwise

### Why not DuckDB as primary SSOT?

DuckDB is attractive for analytics scans but is not the best first canonical store for hot-path append event logging.

The correct ordering is:
1. SQLite as canonical event store
2. optional DuckDB export or mirror later for heavy offline analysis

## Data Model

### 1. `analytics_runs`

One row per top-level operation.

Examples:
1. `read`
2. `query`
3. `crawl`
4. `discover_web`
5. MCP tool invocation

Suggested fields:

```json
{
  "run_id": "run_...",
  "started_at": "2026-04-14T12:00:00Z",
  "completed_at": "2026-04-14T12:00:01Z",
  "operation": "query",
  "surface": "cli",
  "profile": "browser_like_semantic",
  "goal_hash": "...",
  "goal_length_chars": 74,
  "discovery_mode": "web_search",
  "seed_present": false,
  "selected_url": "https://developer.mozilla.org/en-US/docs/Web/JavaScript",
  "success": true,
  "trace_id": "trace_..."
}
```

### 2. `analytics_stage_events`

One row per stage in a run.

Examples:
1. fetch
2. reduce
3. discover
4. semantic_rerank
5. memory_search
6. proof_assembly
7. packet_finalize

Fields:
1. `run_id`
2. `stage`
3. `started_at`
4. `completed_at`
5. `latency_ms`
6. `status`
7. `metadata_json`

### 3. `analytics_fetch_events`

Per HTTP acquisition attempt or browserlike acquisition.

Fields should include:
1. `run_id`
2. `url`
3. `host`
4. `fetch_mode`
5. `fetch_profile`
6. `retry_profile`
7. `content_type`
8. `status_code`
9. `bytes_received`
10. `chars_received`
11. `latency_ms`
12. `retry_count`
13. `retry_sleep_ms`
14. `host_pacing_ms`
15. `blocked_class`
16. `cache_hit`
17. `success`

### 4. `analytics_reduce_events`

Per compiled page/document.

Fields:
1. `run_id`
2. `document_url`
3. `raw_chars`
4. `reduced_chars`
5. `reduction_ratio`
6. `chunk_count`
7. `link_count`
8. `resource_class`
9. `substrate_class`
10. `proof_chunk_count`
11. `summary_chars`
12. `packet_chars`

### 5. `analytics_discovery_events`

Per discovery decision.

Fields:
1. `run_id`
2. `provider`
3. `cold_or_warm`
4. `memory_candidates_count`
5. `public_candidates_count`
6. `selected_candidate_count`
7. `rewrite_applied`
8. `selected_url`
9. `selected_family_host`
10. `selected_reason_json`
11. `topic_node_used`
12. `same_site_recovery_used`
13. `public_bootstrap_used`
14. `success`
15. `failure_class`

### 6. `analytics_memory_events`

Per interaction with Discovery Memory.

Fields:
1. `run_id`
2. `memory_query_kind`
3. `document_hits`
4. `topic_node_hits`
5. `graph_hits`
6. `host_hits`
7. `selected_from_memory`
8. `memory_provider`
9. `topic_node_count_seen`
10. `topic_node_selected`
11. `memory_db_size_bytes`
12. `document_count`
13. `embedding_count`
14. `topic_node_count`

### 7. `analytics_packet_events`

Per final answer packet.

Fields:
1. `run_id`
2. `packet_bytes`
3. `packet_chars`
4. `summary_chars`
5. `chunk_count`
6. `proof_ref_count`
7. `proof_usable`
8. `estimated_llm_context_saved_chars`
9. `estimated_llm_context_saved_ratio`
10. `raw_source_chars_total`
11. `final_context_chars`

### 8. `analytics_agent_events`

Per MCP and agent-facing usage.

Fields:
1. `run_id`
2. `surface`
3. `tool_name`
4. `tool_schema_version`
5. `compact_first`
6. `structured_content_present`
7. `error_class`
8. `error_guided`
9. `tool_round_trip_ms`
10. `client_visible_payload_bytes`

## Derived Metrics: What Actually Matters

The most important insight from this design is that not everything worth tracking should be shown to users.

Needle-X should separate:
1. core internal metrics
2. product proof metrics
3. operator diagnostics

### A. Core product proof metrics

These are the metrics that justify Needle-X as a product.

#### 1. `public_bootstrap_avoidance_rate`

How often local memory / same-site / topic-first retrieval avoided public bootstrap.

This is one of the strongest product metrics in the system.

#### 2. `context_reduction_ratio`

How much raw source content was reduced into compact context.

Formula:

`1 - final_context_chars / raw_source_chars_total`

#### 3. `agent_work_avoidance_index`

Synthetic metric estimating how much work was removed from the downstream LLM.

Potential components:
1. chars not sent downstream
2. sources disambiguated before final packet
3. proof-backed chunk count
4. local retrieval replacing web bootstrap
5. final packet coherence

This should not be a vanity score.
It should be explainable as a weighted decomposition.

#### 4. `proof_backed_answer_rate`

How often final packets contain actionable proof references.

#### 5. `warm_state_lift`

Improvement in success and latency from warm local state versus cold state.

This is a flagship Needle-X metric.

### B. Operator metrics

These are essential for runtime tuning.

1. `fetch_latency_p50/p95`
2. `reduce_latency_p50/p95`
3. `memory_query_latency_p50/p95`
4. `topic_node_hit_rate`
5. `provider_block_rate`
6. `retry_rate`
7. `content_type_trap_rate`
8. `memory_growth_rate`
9. `db_size_growth_rate`
10. `per-host usefulness score`

### C. Transparency metrics for users

These are the ones suitable for CLI/MCP surfaces.

1. raw chars processed
2. final chars delivered
3. packet reduction percentage
4. sources visited
5. links explored
6. public bootstrap avoided or not
7. proof present or not
8. local memory used or not
9. latency total and by phase
10. memory footprint

## Metrics That Matter More Than The User's Example List

The examples you gave are directionally right, but not sufficient.
The optimum set should include at least the following higher-value metrics.

### 1. `semantic_disambiguation_work_done`

Count and measure disambiguation effort Needle-X performed before finalizing the packet.

Examples:
1. candidate families considered
2. topic nodes considered
3. mirrors suppressed
4. false branches pruned

Why it matters:
Because much of Needle-X's value is not just compression but removal of ambiguity.

### 2. `granularity_correction_rate`

How often the system avoided a too-specific page and returned the correct topic root or intermediate node.

Why it matters:
This is exactly the class of value exposed by topic-first retrieval.

### 3. `semantic_memory_reuse_rate`

How often the final answer depended on previously observed local semantic state.

This is stronger than generic cache-hit metrics.

### 4. `quality_per_byte`

How much useful proof-bearing output was produced per byte fetched or parsed.

This is a high-signal metric for host usefulness and pipeline efficiency.

### 5. `host_yield`

Per host:
1. fetch success
2. proof usability
3. packet usefulness
4. latency cost
5. local reuse value over time

This can eventually guide host-level policies.

## Counter-Metrics: What Not To Optimize Blindly

A good analytics layer must also know which metrics are dangerous if optimized directly.

### 1. total pages visited

More is not better.
It may just mean the system is wandering.

### 2. total chars parsed

Bigger is not better.
It can indicate inefficiency.

### 3. packet smallness alone

A tiny packet can be bad if it loses evidence.
Compression must be tied to proof usability and correctness.

### 4. local memory hits alone

A local hit can still be wrong-granularity or wrong-family.

### 5. latency alone

Fast and wrong is not better than slower and correct.

## The Right Accounting Model

The PAL should support three kinds of accounting.

### 1. Resource accounting

1. bytes fetched
2. chars parsed
3. chars reduced
4. packet bytes
5. db size
6. embedding count
7. topic node count

### 2. Effort accounting

1. provider attempts
2. candidate counts
3. disambiguation passes
4. rewrite passes
5. recovery passes
6. proof assembly work

### 3. Value accounting

1. bootstrap avoided
2. proof-backed packet delivered
3. context saved for downstream model
4. local memory reused
5. correct topic/root selected
6. failure class avoided compared to cold path

## Proposed DB Schema

Minimum viable canonical schema:

1. `analytics_runs`
2. `analytics_stage_events`
3. `analytics_fetch_events`
4. `analytics_reduce_events`
5. `analytics_discovery_events`
6. `analytics_memory_events`
7. `analytics_packet_events`
8. `analytics_agent_events`
9. `analytics_rollups_daily`
10. `analytics_rollups_host`
11. `analytics_rollups_provider`

Optional later:
12. `analytics_rollups_topic_family`
13. `analytics_rollups_surface`
14. `analytics_failure_edges`

## Event Semantics

Events should be:
1. append-only
2. timestamped in UTC
3. linked by `run_id`
4. versioned by schema version

Avoid mutable cumulative counters as the primary truth.

Derived rollups should be rebuildable from event truth.

## Runtime Integration Points

### 1. `acquire`

Emit:
1. attempt started
2. attempt finished
3. bytes/chars/status/retry/blocking info

### 2. `reduce`

Emit:
1. raw chars
2. reduced chars
3. chunk count
4. link count
5. summary chars

### 3. `discover/query`

Emit:
1. candidate counts
2. provider chain
3. rewrite info
4. local-memory vs public-bootstrap
5. topic-node use
6. final selection correctness if benchmark mode knows expected target

### 4. `memory`

Emit:
1. document hits
2. topic hits
3. graph expansion hits
4. selected source kind
5. memory DB stats snapshot

### 5. `transport`

Emit:
1. CLI surface
2. MCP tool name
3. payload size
4. guided error class
5. compact-first compliance

## Derived UX Surfaces

### CLI

Potential commands:

1. `needlex analytics stats --json`
2. `needlex analytics recent --limit 50`
3. `needlex analytics hosts --top 20`
4. `needlex analytics providers`
5. `needlex analytics value-report`
6. `needlex analytics export --out DIR`
7. `needlex analytics rebuild-rollups`

### MCP

Potential tools:

1. `analytics_stats`
2. `analytics_recent_runs`
3. `analytics_hosts`
4. `analytics_value_report`
5. `analytics_export`

### Packet-level transparency

Optionally expose a compact analytics footer in responses such as:

```json
{
  "analytics": {
    "raw_chars": 84231,
    "final_chars": 3120,
    "reduction_ratio": 0.9629,
    "sources_visited": 3,
    "links_explored": 17,
    "public_bootstrap_avoided": true,
    "memory_used": true,
    "proof_backed": true,
    "latency_ms": 418
  }
}
```

This should be optional and compact.

## DB Technology Recommendation

### Primary recommendation

**SQLite with append-only event tables plus rebuildable rollups.**

Why:
1. fits project philosophy
2. integrates with existing local stores
3. durable and inspectable
4. low operational burden
5. enough performance for current scale

### Optional advanced layer later

If offline analytics grows substantially:
1. export SQLite events into DuckDB for heavy scans
2. or mirror daily rollups into Parquet

But not as the first SSOT.

## Privacy and Sensitivity Boundaries

Analytics must not blindly persist:
1. full goals in plaintext forever
2. secrets from fetched pages
3. authorization headers
4. raw credentials or tokens

Prefer:
1. hashes
2. host-level aggregation
3. controlled sampling
4. redaction of obvious secrets

## What Would Make This Scientifically Strong

The PAL becomes scientifically strong when it supports:

1. cold vs warm comparisons
2. ablations
3. before/after change comparisons
4. host-level yield curves
5. topic-first vs flat-page retrieval comparisons
6. packet reduction and proof usability measured together

This matters because it lets Needle-X make defensible claims such as:

1. public bootstrap reduced by X%
2. downstream context reduced by Y%
3. warm-state retrieval improved by Z%
4. topic-first retrieval improves overview accuracy on dense doc families by N points

## Devil's Advocate

### 1. Risk: telemetry bloat

If everything is tracked, nothing is legible.

Mitigation:
1. strict canonical schema
2. separate raw events from derived rollups
3. keep product metrics opinionated

### 2. Risk: hot-path slowdown

Too much synchronous logging can harm latency.

Mitigation:
1. append-only writes
2. batch inserts where possible
3. bounded metadata payloads
4. rollups out of band

### 3. Risk: wrong metrics drive wrong optimization

Teams optimize what they can see.
If the wrong numbers are prominent, the product will drift.

Mitigation:
1. foreground value metrics, not vanity counters
2. always pair compression with proof/correctness
3. always pair speed with success rate

### 4. Risk: duplicate semantics with trace/proof

If analytics duplicates trace in another form, complexity rises without value.

Mitigation:
1. analytics stores quantitative event facts
2. trace stores causal execution details
3. proof stores evidence lineage

## Recommended Implementation Order

### Phase 1: Canonical event substrate

1. `analytics.db`
2. event tables
3. append hooks in fetch, reduce, discover, memory, packet finalize
4. `analytics stats`

### Phase 2: Derived rollups

1. daily rollups
2. host/provider rollups
3. value report
4. public-bootstrap avoidance metrics

### Phase 3: Product-facing transparency

1. compact analytics footer in CLI/MCP responses
2. MCP analytics tools
3. host usefulness inspection
4. discovery-memory value dashboard

### Phase 4: Scientific measurement layer

1. ablation-friendly comparisons
2. change-over-time trend reports
3. topic-first vs flat-page delta reports
4. warm-state leverage reporting

## Recommended First Build

If implemented now, the minimum serious cut should include:

1. SQLite analytics DB
2. append-only tables:
   - runs
   - fetch_events
   - reduce_events
   - discovery_events
   - memory_events
   - packet_events
3. CLI:
   - `analytics stats`
   - `analytics recent`
   - `analytics value-report`
4. derived metrics:
   - raw chars processed
   - final chars delivered
   - reduction ratio
   - public bootstrap avoided rate
   - local memory reuse rate
   - proof-backed packet rate
   - avg latency by phase
   - sources visited
   - links explored
   - memory DB size

This would already make Needle-X much more transparent without overbuilding.

## Final Position

The right analytics system for Needle-X is not a debug log and not a vanity dashboard.

It should be a local, durable, queryable measurement substrate that can prove:
1. what Needle-X did
2. what Needle-X avoided
3. how much work it saved for the agent
4. where the system is winning
5. where the system is still wasting work

That is the correct role of an `Analytics PAL`.
