# Benchmarks

Needle-X benchmarks should be read in two groups:
1. quality metrics
2. advantage metrics

## What Already Holds

Current advantage metrics support:
1. smaller packets
2. direct proof usability
3. shorter claim-to-source path
4. lower post-processing burden
5. strong warm-state local retrieval

## Live Snapshot

Compared with `Tavily`, `Jina`, and `Firecrawl`:
1. avg packet bytes: `4436`
2. claim-to-source steps: `1`
3. post-processing burden: `0.25`
4. proof usability: `1.0`

## Important Guardrail

Do not read the benchmark story as:
1. broad market-superiority
2. cold-state open-web dominance

Read it as:
1. compact proof-carrying output
2. efficient agent-facing retrieval
3. strong warm-state local reuse
4. a product that treats seedless retrieval as first-class, while still measuring it separately from warm-state performance

## Reading Seedless Correctly

Seedless discovery is part of the product surface now.

That does not mean:
1. every seedless benchmark should be mixed into the warm-state story
2. noisy provider behavior should be confused with retrieval quality

It does mean:
1. seedless pass rate matters
2. provider reliability matters
3. semantic grounding and family recovery matter
4. warm-state and seedless should be evaluated as different modes with different failure profiles

## Next

1. [Discovery Memory](./Discovery-Memory.md)
2. [MCP And Tool Calling](./MCP-And-Tool-Calling.md)

## Full Reference

1. [Benchmark Report](../benchmark-report.md)
2. [Competitive Benchmark Protocol](../competitive-benchmark-protocol.md)
3. [Seeded Benchmark Spec](../seeded-benchmark-spec.md)
