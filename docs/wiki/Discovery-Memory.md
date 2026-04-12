# Discovery Memory

Discovery Memory is Needle-X's local retrieval layer.

It is:
1. SQLite-backed
2. local-first
3. proof-aware
4. warm-state oriented
5. already partially absorbed by the runtime

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

What remains experimental is the broader strategic shape:
1. wider memory leverage across seedless retrieval
2. stronger long-horizon local accumulation
3. more autonomous decisioning over when and how memory should dominate bootstrap search

## Current Claim

Warm-state benchmark result:
1. `30/30` selected-url correctness
2. `30/30` `discovery_memory` provider selection

Guardrail:
1. this is a warm-state local retrieval claim
2. it is not a blanket cold-state open-web superiority claim
3. seedless discovery is a first-class product surface, but it is still noisier than warm-state retrieval

## Operator Surface

```bash
needlex memory stats --json
needlex memory search "pricing" --json
needlex memory prune --json
```

## Next

1. [Benchmarks](./Benchmarks.md)
2. [CLI](./CLI.md)

## Full Reference

1. [Benchmark Report](../benchmark-report.md)
2. [Discovery Memory Spec](../experimental/discovery-memory-spec.md)
3. [Archived Seedless Discovery Strategy](../archive/seedless-discovery-strategy.md)
