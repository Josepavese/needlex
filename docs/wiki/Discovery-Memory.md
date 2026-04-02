# Discovery Memory

Discovery Memory is Needle-X's local retrieval layer.

It is:
1. SQLite-backed
2. local-first
3. proof-aware
4. warm-state oriented

## What It Means

1. first run observes and compiles
2. later runs reuse local evidence
3. repeated use improves retrieval without hosted infra

## Current Claim

Warm-state benchmark result:
1. `30/30` selected-url correctness
2. `30/30` `discovery_memory` provider selection

Guardrail:
1. this is a warm-state local retrieval claim
2. it is not a cold-state open-web seedless claim

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
