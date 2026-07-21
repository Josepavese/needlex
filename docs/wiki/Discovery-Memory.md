# Discovery Memory

Discovery Memory is Needle-X's local retrieval layer.

It is:
1. SQLite-backed
2. local-first
3. proof-aware
4. warm-state oriented
5. enabled by default
6. already absorbed by the runtime

## What It Means

1. first run observes and compiles
2. later runs reuse local evidence
3. repeated use improves retrieval without hosted infra

## Current State

Today this is not just a future idea or a speculative design.

Needle-X already ships:
1. local discovery state
2. provider health memory
3. warm-state reuse through `discovery.db`
4. semantic reranking and family recovery in the discovery path
5. dense embedding vectors from the configured semantic endpoint

Needle-X does not synthesize memory vectors from surface or sub-token overlap.
If no semantic endpoint is configured, the runtime is invalid; Needle-X must fail rather than silently storing lexical-only memory.

What remains experimental is the broader strategic shape:
1. stronger long-horizon local accumulation
2. more autonomous decisioning over when and how memory should dominate bootstrap search
3. richer cross-host family recovery from accumulated evidence

## Runtime Behavior

Successful `read`, `query`, and `crawl` runs automatically observe:
1. canonical page URL and title
2. compact semantic summary
3. proof references
4. discovered links and same-host edges
5. topic nodes and host/family expansions
6. semantic family graph evidence and membership edges

Experimental seedless `web_query` consults this local substrate before public bootstrap. This path runs only after explicit `discovery_mode="web_search"` opt-in. Stable agent workflows use host-agent search followed by `web_read` for every URL the agent selects.

## Current Claim

Verified seeded benchmark result:
1. `100/100` selected-url correctness on `seeded-corpus-v2`
2. `100/100` proof usability
3. `100/100` runtime success

Guardrail:
1. this is a seeded-runtime claim
2. it is not a blanket cold-state open-web superiority claim
3. Discovery Memory warm-state stress is tracked separately from the seeded runtime score
4. seedless discovery is experimental and outside the stable agent workflow

## Operator Surface

```bash
needlex memory stats --json
needlex memory search "pricing" --json
needlex memory prune --json
needlex memory export --out /tmp/needlex-memory
needlex memory import --in /tmp/needlex-memory
needlex memory rebuild-index --json
```

`memory stats` and `doctor` expose the local semantic substrate:
1. document, edge, embedding, topic, family, and family-member counts
2. vector engine identity
3. stored embedding dimensions
4. last rebuild timestamp

`memory export` and `memory import` include the semantic family graph, not only documents and page embeddings.

## Next

1. [Benchmarks](./Benchmarks.md)
2. [CLI](./CLI.md)

## Full Reference

1. [Benchmark Report](../benchmark-report.md)
2. [Discovery Memory Spec](../experimental/discovery-memory-spec.md)
